package console

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// javaHomeEnv overrides the Java runtime, for development builds and tests.
const javaHomeEnv = "MDBMC_JAVA_HOME"

// bundledJavaHome is the Java runtime shipped next to the executable.
func bundledJavaHome() string {
	if d := os.Getenv(javaHomeEnv); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "jre")
}

// javaSecurity re-enables the TLS versions and ciphers that current Java
// disables but old BMC firmware still needs for an encrypted KVM session.
// It applies only to the console's own Java process.
const javaSecurity = `# Written by MDBMC for BMC remote consoles.
jdk.tls.disabledAlgorithms=SSLv3, RC4, DES, MD5withRSA, anon, NULL
jdk.certpath.disabledAlgorithms=MD2
`

// javaStatus is shown on the console window's status page.
type javaStatus struct {
	State  string `json:"state"` // idle, preparing, running, exited, failed
	Detail string `json:"detail"`
}

// javaLauncher runs the Java consoles of one console window.
type javaLauncher struct {
	javaHome string
	cacheDir string       // downloaded jars and native libraries
	client   *http.Client // to the BMC, pinned certificate
	report   func(name, detail string)
	// start starts the process and ties its lifetime to the window.
	start func(*exec.Cmd) error

	mu     sync.Mutex
	status javaStatus
}

func newJavaLauncher(cacheDir string, client *http.Client, report func(name, detail string)) *javaLauncher {
	return &javaLauncher{
		javaHome: bundledJavaHome(),
		cacheDir: cacheDir,
		client:   client,
		report:   report,
		start:    startTied,
		status:   javaStatus{State: "idle"},
	}
}

func (l *javaLauncher) setStatus(state, detail string) {
	l.mu.Lock()
	l.status = javaStatus{State: state, Detail: detail}
	l.mu.Unlock()
	if l.report != nil {
		l.report("java-"+state, detail)
	}
}

func (l *javaLauncher) Status() javaStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

func (l *javaLauncher) javaExe() (string, error) {
	name := "java"
	if runtime.GOOS == "windows" {
		name = "javaw.exe" // no console window
	}
	exe := filepath.Join(l.javaHome, "bin", name)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("未找到随软件安装的 Java 运行环境（%s），请重新安装本软件", filepath.Join(l.javaHome, "bin"))
	}
	return exe, nil
}

// Launch downloads what plan needs and starts it. cookie is the BMC's
// Cookie header, for firmware that only serves the jars to a session.
func (l *javaLauncher) Launch(ctx context.Context, plan *javaPlan, cookie string) {
	cmd, err := l.prepare(ctx, plan, cookie)
	if err != nil {
		l.setStatus("failed", err.Error())
		return
	}
	tail := &tailBuffer{max: 8 << 10}
	cmd.Stdout, cmd.Stderr = tail, tail
	if err := l.start(cmd); err != nil {
		l.setStatus("failed", "启动 Java 失败: "+err.Error())
		return
	}
	l.setStatus("running", plan.Title)
	go func() {
		err := cmd.Wait()
		detail := "已关闭"
		if err != nil {
			detail = err.Error()
		}
		if out := strings.TrimSpace(tail.String()); out != "" {
			detail += "\n" + lastLines(out, 12)
		}
		l.setStatus("exited", detail)
	}()
}

func (l *javaLauncher) prepare(ctx context.Context, plan *javaPlan, cookie string) (*exec.Cmd, error) {
	java, err := l.javaExe()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.cacheDir, 0o700); err != nil {
		return nil, err
	}
	var jars []string
	for i, u := range plan.Jars {
		l.setStatus("preparing", fmt.Sprintf("正在从 BMC 下载 %s（%d/%d）", path.Base(u.Path), i+1, len(plan.Jars)+len(plan.NativeLibs)))
		p, err := l.fetch(ctx, u, cookie)
		if err != nil {
			return nil, err
		}
		jars = append(jars, p)
	}
	var libDirs []string
	for i, u := range plan.NativeLibs {
		l.setStatus("preparing", fmt.Sprintf("正在从 BMC 下载 %s（%d/%d）", path.Base(u.Path), len(plan.Jars)+i+1, len(plan.Jars)+len(plan.NativeLibs)))
		p, err := l.fetch(ctx, u, cookie)
		if err != nil {
			return nil, err
		}
		dir := strings.TrimSuffix(p, filepath.Ext(p)) + ".native"
		if err := extractNatives(p, dir); err != nil {
			return nil, fmt.Errorf("解压 %s 失败: %w", path.Base(u.Path), err)
		}
		libDirs = append(libDirs, dir)
	}
	main := plan.MainClass
	if main == "" {
		if main, err = manifestMainClass(jars[0]); err != nil {
			return nil, err
		}
	}
	secFile := filepath.Join(l.cacheDir, "java.security")
	if err := os.WriteFile(secFile, []byte(javaSecurity), 0o600); err != nil {
		return nil, err
	}
	args := []string{"-Djava.security.properties=" + secFile}
	if len(libDirs) > 0 {
		args = append(args, "-Djava.library.path="+strings.Join(libDirs, string(os.PathListSeparator)))
	}
	for _, p := range plan.Properties {
		if p.Name != "" {
			args = append(args, "-D"+p.Name+"="+p.Value)
		}
	}
	args = append(args, "-cp", strings.Join(jars, string(os.PathListSeparator)), main)
	args = append(args, plan.Args...)
	cmd := exec.Command(java, args...)
	cmd.Dir = l.cacheDir
	return cmd, nil
}

// fetch downloads u into the cache, revalidating a cached copy with the
// validators the BMC sent for it.
func (l *javaLauncher) fetch(ctx context.Context, u *url.URL, cookie string) (string, error) {
	sum := sha256.Sum256([]byte(u.String()))
	name := path.Base(u.Path)
	if name == "." || name == "/" || strings.ContainsAny(name, `\:`) {
		name = "file.jar"
	}
	dst := filepath.Join(l.cacheDir, "jars", hex.EncodeToString(sum[:6])+"-"+name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	validators, _ := os.ReadFile(dst + ".etag")
	if _, err := os.Stat(dst); err == nil && len(validators) > 0 {
		etag, modified, _ := strings.Cut(string(validators), "\n")
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		if modified != "" {
			req.Header.Set("If-Modified-Since", modified)
		}
	}
	res, err := l.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("从 BMC 下载 %s 失败: %w", name, err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotModified:
		return dst, nil
	case http.StatusOK:
	default:
		return "", fmt.Errorf("从 BMC 下载 %s 失败: HTTP %d", name, res.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, io.LimitReader(res.Body, 256<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("从 BMC 下载 %s 失败: %w", name, err)
	}
	_ = os.WriteFile(dst+".etag", []byte(res.Header.Get("ETag")+"\n"+res.Header.Get("Last-Modified")), 0o600)
	return dst, nil
}

// extractNatives copies the native libraries in a jar into dir. Only file
// names are kept, so entries cannot escape dir.
func extractNatives(jarPath, dir string) error {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := path.Base(f.Name)
		switch strings.ToLower(path.Ext(name)) {
		case ".dll", ".so", ".dylib", ".jnilib":
		default:
			continue
		}
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "META-INF/") {
			continue
		}
		if err := extractFile(f, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, dst string) error {
	if fi, err := os.Stat(dst); err == nil && fi.Size() == int64(f.UncompressedSize64) {
		return nil // a running console may hold the library open
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(r, 256<<20))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// manifestMainClass reads Main-Class from a jar's manifest.
func manifestMainClass(jarPath string) (string, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return "", fmt.Errorf("无法读取 %s: %w", filepath.Base(jarPath), err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, "META-INF/MANIFEST.MF") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		defer r.Close()
		// Manifest lines continue on lines that start with a space.
		var lines []string
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if strings.HasPrefix(line, " ") && len(lines) > 0 {
				lines[len(lines)-1] += line[1:]
				continue
			}
			lines = append(lines, line)
		}
		for _, line := range lines {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Main-Class") {
				return strings.TrimSpace(v), nil
			}
		}
	}
	return "", errors.New("JNLP 文件未指定主类，程序包里也没有 Main-Class")
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// launchTimeout bounds downloading the jars from a slow BMC.
const launchTimeout = 3 * time.Minute
