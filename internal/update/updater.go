package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// State is the updater's progress.
type State string

const (
	StateIdle        State = "idle"     // not checked yet
	StateChecking    State = "checking" // fetching the manifest
	StateUpToDate    State = "upToDate"
	StateAvailable   State = "available" // a newer version exists
	StateDownloading State = "downloading"
	StateReady       State = "ready"      // package downloaded and verified
	StateInstalling  State = "installing" // handed over to the helper; the program is exiting
)

// Status is a snapshot of the updater.
type Status struct {
	State      State
	Latest     *Manifest // the newer version, when there is one
	Downloaded int64
	Total      int64
	Speed      int64 // bytes per second while downloading
	Error      string
	CheckedAt  time.Time
}

// Config is where updates come from.
type Config struct {
	Source string // "" for DefaultSource
	Proxy  string // "" for the system proxy
}

// Options configure an Updater.
type Options struct {
	Current string // the running version
	Mode    Mode
	Dir     string // download directory
	Keys    []ed25519.PublicKey
	// OnChange is called after every status change, outside the lock.
	OnChange func()
}

const (
	maxManifestSize = 1 << 20
	stallTimeout    = 60 * time.Second
	emitInterval    = 300 * time.Millisecond
)

// Updater checks for and downloads updates. It is safe for concurrent use.
type Updater struct {
	opts Options

	mu       sync.Mutex
	cfg      Config
	st       Status
	pkg      string // verified package, when ready
	cancel   context.CancelFunc
	lastEmit time.Time
	lastSize int64
}

// New returns an Updater.
func New(opts Options) *Updater {
	if opts.OnChange == nil {
		opts.OnChange = func() {}
	}
	return &Updater{opts: opts, st: Status{State: StateIdle}}
}

// CanVerify reports whether the program has a key to verify updates with;
// development builds usually do not.
func (u *Updater) CanVerify() bool { return len(u.opts.Keys) > 0 }

// Mode returns how this copy was installed.
func (u *Updater) Mode() Mode { return u.opts.Mode }

// SetConfig changes the source and proxy for later requests.
func (u *Updater) SetConfig(c Config) {
	u.mu.Lock()
	u.cfg = c
	u.mu.Unlock()
}

// Config returns the current configuration.
func (u *Updater) Config() Config {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cfg
}

// Status returns a snapshot of the updater.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

// idleState is the state to return to when nothing is in progress.
func (u *Updater) idleState() State {
	switch {
	case u.st.Latest != nil && u.pkg != "":
		return StateReady
	case u.st.Latest != nil:
		return StateAvailable
	case !u.st.CheckedAt.IsZero():
		return StateUpToDate
	}
	return StateIdle
}

// Check fetches the newest manifest. It does nothing while a check,
// download or install is in progress.
func (u *Updater) Check(ctx context.Context) error {
	u.mu.Lock()
	switch u.st.State {
	case StateChecking, StateDownloading, StateInstalling:
		u.mu.Unlock()
		return nil
	}
	u.st.State, u.st.Error = StateChecking, ""
	cfg := u.cfg
	u.mu.Unlock()
	u.opts.OnChange()

	m, err := u.fetchManifest(ctx, cfg)

	u.mu.Lock()
	u.st.CheckedAt = time.Now()
	switch {
	case err != nil:
		err = fmt.Errorf("检查更新失败：%w", err)
		u.st.Error = err.Error()
	case Compare(m.Version, u.opts.Current) <= 0:
		u.st.Latest, u.pkg = nil, ""
	case u.st.Latest != nil && u.st.Latest.Version == m.Version && u.pkg != "":
		u.st.Latest = m // already downloaded
	default:
		u.st.Latest, u.pkg = m, ""
		if a, ok := m.Assets[u.opts.Mode]; ok {
			if p := u.packagePath(m, a); verifyFile(p, a) == nil {
				u.pkg = p
			}
		}
	}
	u.st.State = u.idleState()
	keep := ""
	if u.st.Latest != nil {
		keep = u.st.Latest.Version
	}
	u.mu.Unlock()
	if err == nil {
		u.prune(keep)
	}
	u.opts.OnChange()
	return err
}

func (u *Updater) fetchManifest(ctx context.Context, cfg Config) (*Manifest, error) {
	if len(u.opts.Keys) == 0 {
		return nil, errUnsigned
	}
	client, err := newHTTPClient(cfg.Proxy)
	if err != nil {
		return nil, err
	}
	data, err := fetchSmall(ctx, client, latestURL(cfg.Source, ManifestName), maxManifestSize)
	if err != nil {
		return nil, err
	}
	sig, err := fetchSmall(ctx, client, latestURL(cfg.Source, SignatureName), 4096)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data, sig, u.opts.Keys)
}

func fetchSmall(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return nil, connError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("更新源中没有找到已发布的版本（HTTP 404）")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新服务器返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, connError(err)
	}
	if int64(len(b)) > limit {
		return nil, errors.New("更新服务器返回的内容过大")
	}
	return b, nil
}

func connError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("连接更新服务器超时")
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("无法连接更新服务器（%v）", err)
}

// packagePath is where the package of release m is stored.
func (u *Updater) packagePath(m *Manifest, a Asset) string {
	return filepath.Join(u.opts.Dir, m.Version, a.Name)
}

// prune removes downloads of versions other than keep.
func (u *Updater) prune(keep string) {
	entries, err := os.ReadDir(u.opts.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != keep && ValidVersion(e.Name()) {
			_ = os.RemoveAll(filepath.Join(u.opts.Dir, e.Name()))
		}
	}
}

// Download starts downloading the package of the newer version in the
// background; progress is reported through OnChange.
func (u *Updater) Download() error {
	u.mu.Lock()
	switch u.st.State {
	case StateDownloading, StateReady:
		u.mu.Unlock()
		return nil
	case StateAvailable:
	default:
		u.mu.Unlock()
		return errors.New("没有可下载的新版本")
	}
	m := u.st.Latest
	a, ok := m.Assets[u.opts.Mode]
	if !ok {
		u.mu.Unlock()
		return errors.New("该版本没有适用于当前安装方式的更新包，请到发布页面手动下载")
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	u.st.State, u.st.Error = StateDownloading, ""
	u.st.Downloaded, u.st.Total, u.st.Speed = 0, a.Size, 0
	u.lastEmit, u.lastSize = time.Now(), 0
	cfg := u.cfg
	u.mu.Unlock()
	u.opts.OnChange()

	go func() {
		path, err := u.download(ctx, cfg, m, a)
		u.mu.Lock()
		u.cancel = nil
		if err == nil {
			u.pkg = path
		} else if !errors.Is(err, context.Canceled) {
			u.st.Error = "下载失败：" + err.Error()
		}
		u.st.Speed = 0
		u.st.State = u.idleState()
		u.mu.Unlock()
		cancel()
		u.opts.OnChange()
	}()
	return nil
}

// Cancel stops a running download; the partial file is kept so the next
// download resumes it.
func (u *Updater) Cancel() {
	u.mu.Lock()
	if u.cancel != nil {
		u.cancel()
	}
	u.mu.Unlock()
}

func (u *Updater) progress(n int64) {
	u.mu.Lock()
	u.st.Downloaded = n
	now := time.Now()
	emit := now.Sub(u.lastEmit) >= emitInterval
	if emit {
		u.st.Speed = int64(float64(n-u.lastSize) / now.Sub(u.lastEmit).Seconds())
		u.lastEmit, u.lastSize = now, n
	}
	u.mu.Unlock()
	if emit {
		u.opts.OnChange()
	}
}

// download fetches asset a of release m, resuming a partial download, and
// returns the verified file.
func (u *Updater) download(ctx context.Context, cfg Config, m *Manifest, a Asset) (string, error) {
	final := u.packagePath(m, a)
	if verifyFile(final, a) == nil {
		return final, nil
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return "", err
	}
	part := final + ".part"
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	have, err := io.Copy(h, f)
	if err != nil {
		return "", err
	}
	restart := func() error {
		h.Reset()
		have = 0
		if err := f.Truncate(0); err != nil {
			return err
		}
		_, err := f.Seek(0, io.SeekStart)
		return err
	}
	if have >= a.Size {
		if err := restart(); err != nil {
			return "", err
		}
	}

	client, err := newHTTPClient(cfg.Proxy)
	if err != nil {
		return "", err
	}
	reqCtx, cancelReq := context.WithCancel(ctx)
	defer cancelReq()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, assetURL(cfg.Source, m.Tag, a.Name), nil)
	if err != nil {
		return "", err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", connError(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0 && hasRangeStart(resp.Header.Get("Content-Range"), have):
	case resp.StatusCode == http.StatusOK:
		if err := restart(); err != nil {
			return "", err
		}
	case resp.StatusCode == http.StatusNotFound:
		return "", errors.New("更新源中没有找到安装包（HTTP 404）")
	default:
		return "", fmt.Errorf("更新服务器返回 HTTP %d", resp.StatusCode)
	}

	// A connection that stops delivering data is abandoned after a while.
	var stalled atomic.Bool
	stall := time.AfterFunc(stallTimeout, func() { stalled.Store(true); cancelReq() })
	defer stall.Stop()
	u.progress(have)
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			stall.Reset(stallTimeout)
			if have+int64(n) > a.Size {
				_ = restart()
				return "", errors.New("下载的数据比更新清单中的大小多")
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			h.Write(buf[:n])
			have += int64(n)
			u.progress(have)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if stalled.Load() {
				return "", fmt.Errorf("%d 秒内没有收到数据，请稍后重试（已下载的部分会保留）", int(stallTimeout.Seconds()))
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("连接中断（%v），重试时会从断点继续", rerr)
		}
	}
	if have != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		_ = restart()
		return "", errors.New("安装包校验失败（大小或 SHA-256 与更新清单不一致），请重新下载")
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(part, final); err != nil {
		return "", err
	}
	return final, nil
}

func hasRangeStart(contentRange string, start int64) bool {
	var from, to, total int64
	_, err := fmt.Sscanf(contentRange, "bytes %d-%d/%d", &from, &to, &total)
	return err == nil && from == start
}

// Prepared returns the downloaded package after checking it once more.
func (u *Updater) Prepared() (string, *Manifest, Asset, error) {
	u.mu.Lock()
	if u.st.State != StateReady {
		u.mu.Unlock()
		return "", nil, Asset{}, errors.New("更新包尚未下载完成")
	}
	m, p := u.st.Latest, u.pkg
	a := m.Assets[u.opts.Mode]
	u.mu.Unlock()
	if err := verifyFile(p, a); err != nil {
		u.mu.Lock()
		if u.pkg == p {
			u.pkg = ""
			u.st.Error = err.Error() + "，请重新下载"
			u.st.State = u.idleState()
		}
		u.mu.Unlock()
		u.opts.OnChange()
		return "", nil, Asset{}, err
	}
	return p, m, a, nil
}

// SetInstalling marks the update as handed over to the helper.
func (u *Updater) SetInstalling() {
	u.mu.Lock()
	u.st.State, u.st.Error = StateInstalling, ""
	u.mu.Unlock()
	u.opts.OnChange()
}

// PruneInstalled removes downloads of versions not newer than the running
// one (left over after an update).
func (u *Updater) PruneInstalled() {
	entries, err := os.ReadDir(u.opts.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && ValidVersion(e.Name()) && Compare(e.Name(), u.opts.Current) <= 0 {
			_ = os.RemoveAll(filepath.Join(u.opts.Dir, e.Name()))
		}
	}
}
