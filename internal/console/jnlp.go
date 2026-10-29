package console

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Java remote consoles.
//
// Many BMCs (AMI MegaRAC firmware such as Inspur's, older Supermicro, Dell
// and Lenovo) start their remote console as a Java Web Start application:
// the web page downloads a JNLP file naming the jars, the main class and the
// arguments, including a one-time KVM session token. Java Web Start no
// longer exists and the machine may have no Java at all, so the console
// window runs these applications itself with the Java runtime shipped with
// this program: the proxy recognises the JNLP the BMC returns, downloads the
// jars from the BMC (pinned certificate) and starts the main class.

// jnlpFile is the part of a JNLP file that is needed to start the program.
type jnlpFile struct {
	Codebase  string          `xml:"codebase,attr"`
	Title     string          `xml:"information>title"`
	Resources []jnlpResources `xml:"resources"`
	App       *jnlpApp        `xml:"application-desc"`
	Applet    *struct{}       `xml:"applet-desc"`
}

type jnlpResources struct {
	OS         string         `xml:"os,attr"`
	Arch       string         `xml:"arch,attr"`
	Jars       []jnlpRef      `xml:"jar"`
	NativeLibs []jnlpRef      `xml:"nativelib"`
	Properties []jnlpProperty `xml:"property"`
}

type jnlpRef struct {
	Href string `xml:"href,attr"`
	Main string `xml:"main,attr"`
}

type jnlpProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type jnlpApp struct {
	MainClass string   `xml:"main-class,attr"`
	Args      []string `xml:"argument"`
}

// javaPlan is what to download and run for one JNLP.
type javaPlan struct {
	Title      string
	Jars       []*url.URL // the main jar first
	NativeLibs []*url.URL // jars holding native libraries for this platform
	Properties []jnlpProperty
	// MainClass may be empty: it then comes from the main jar's manifest.
	MainClass string
	Args      []string
}

// javaPlatform is how the bundled Java runtime describes the system, which
// is what JNLP os/arch attributes are matched against.
type javaPlatform struct{ OSName, Arch string }

func platformFor(goos, goarch string) javaPlatform {
	p := javaPlatform{OSName: goos, Arch: goarch}
	switch goos {
	case "windows":
		p.OSName = "Windows 10"
	case "linux":
		p.OSName = "Linux"
	case "darwin":
		p.OSName = "Mac OS X"
	}
	switch goarch {
	case "386":
		p.Arch = "x86"
	case "arm64":
		p.Arch = "aarch64"
	}
	return p
}

// matchesPrefix applies the JNLP rule for os/arch attributes: they list
// prefixes, and an empty attribute matches everything.
func matchesPrefix(attr, value string) bool {
	fields := strings.Fields(attr)
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		if len(value) >= len(f) && strings.EqualFold(value[:len(f)], f) {
			return true
		}
	}
	return false
}

// parseJNLP reads a JNLP file served at src for the given platform.
func parseJNLP(data []byte, src *url.URL, plat javaPlatform) (*javaPlan, error) {
	var f jnlpFile
	dec := xml.NewDecoder(bytes.NewReader(data))
	// JNLP files are ASCII in practice, whatever encoding they declare.
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("无法解析 JNLP 文件: %w", err)
	}
	if f.App == nil {
		if f.Applet != nil {
			return nil, errors.New("该 BMC 的 Java 控制台是 Applet 形式，暂不支持")
		}
		return nil, errors.New("JNLP 文件中没有可启动的程序")
	}
	// Hrefs are relative to the codebase, a directory.
	base := src.ResolveReference(&url.URL{Path: "./"})
	if f.Codebase != "" {
		cb, err := src.Parse(f.Codebase)
		if err != nil {
			return nil, fmt.Errorf("JNLP 文件的 codebase 无效: %w", err)
		}
		if !strings.HasSuffix(cb.Path, "/") {
			cb.Path += "/"
		}
		base = cb
	}
	plan := &javaPlan{Title: strings.TrimSpace(f.Title), MainClass: strings.TrimSpace(f.App.MainClass)}
	for _, a := range f.App.Args {
		plan.Args = append(plan.Args, strings.TrimSpace(a))
	}
	resolve := func(href string) (*url.URL, error) {
		u, err := base.Parse(strings.TrimSpace(href))
		if err != nil || u.Scheme != "https" && u.Scheme != "http" {
			return nil, fmt.Errorf("JNLP 文件中的程序包地址无效: %q", href)
		}
		return u, nil
	}
	for _, res := range f.Resources {
		if !matchesPrefix(res.OS, plat.OSName) || !matchesPrefix(res.Arch, plat.Arch) {
			continue
		}
		for _, j := range res.Jars {
			u, err := resolve(j.Href)
			if err != nil {
				return nil, err
			}
			if strings.EqualFold(j.Main, "true") {
				plan.Jars = append([]*url.URL{u}, plan.Jars...)
			} else {
				plan.Jars = append(plan.Jars, u)
			}
		}
		for _, n := range res.NativeLibs {
			u, err := resolve(n.Href)
			if err != nil {
				return nil, err
			}
			plan.NativeLibs = append(plan.NativeLibs, u)
		}
		plan.Properties = append(plan.Properties, res.Properties...)
	}
	if len(plan.Jars) == 0 {
		return nil, errors.New("JNLP 文件中没有程序包")
	}
	return plan, nil
}

// looksLikeJNLP tells a JNLP response from other responses. A ".jnlp" path
// that returns a web page (e.g. "session expired") is left to the browser.
func looksLikeJNLP(contentType, path string) bool {
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(ct, "application/x-java-jnlp-file") {
		return true
	}
	return strings.HasSuffix(strings.ToLower(path), ".jnlp") && !strings.Contains(ct, "html") && !strings.Contains(ct, "json")
}

// mapToBMC points the JNLP at the BMC. Some firmware builds the JNLP from
// the address the page was loaded from, which in a console window is the
// local proxy (proxyHost, host:port). Jars are only taken from the BMC
// itself (bmc, its base URL) and always over its pinned HTTPS connection:
// they are code this program runs.
func (p *javaPlan) mapToBMC(proxyHost string, bmc *url.URL) error {
	for _, list := range [][]*url.URL{p.Jars, p.NativeLibs} {
		for _, u := range list {
			if !strings.EqualFold(u.Host, proxyHost) && !strings.EqualFold(u.Hostname(), bmc.Hostname()) {
				return fmt.Errorf("JNLP 文件要求从其他服务器（%s）下载程序，已拒绝", u.Host)
			}
			u.Scheme, u.Host = bmc.Scheme, bmc.Host
		}
	}
	proxyName := strings.Split(proxyHost, ":")[0]
	proxyOrigin := "https://" + proxyHost
	for i, a := range p.Args {
		switch {
		case a == proxyName || strings.EqualFold(a, "localhost"):
			p.Args[i] = bmc.Hostname()
		case strings.HasPrefix(a, proxyOrigin):
			p.Args[i] = bmc.Scheme + "://" + bmc.Host + a[len(proxyOrigin):]
		}
	}
	return nil
}
