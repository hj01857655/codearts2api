// Package update 实现「检测新版本 → 下载校验 → 原子替换二进制 → 优雅退出重启」
// 的自更新链路，只用标准库：本服务进程持有华为云凭证，外部依赖越少越可控。
//
// 设计说明见 docs/online-update.md。三个不可动摇的约束：
//  1. 替换必须是原子的（同目录 rename），绝不能原地写正在运行的文件。
//  2. 校验和不匹配一律中止，且不动现有二进制——绝不做「没校验就放过」的降级。
//  3. 重启不调 systemctl（需要 sudo），而是退出进程，交给守护方拉起：
//     systemd 的 Restart=always 或 Docker 的 restart: unless-stopped。
//     容器内自更新也因此成立：替换的是容器可写层里的文件，容器重启不清空
//     可写层，新二进制在重启后依然有效（做法对齐 sub2api）。
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// newProxyTransport 按代理 URL 构造 HTTP 传输层。http/https 走 CONNECT 隧道；
// socks5 由 http.Transport.Proxy 原生支持。仅用于更新流量，不碰业务上游。
func newProxyTransport(proxyURL string) (*http.Transport, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5":
		// http.Transport.Proxy 原生支持这三种 scheme。
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (want http/https/socks5)", u.Scheme)
	}
	return &http.Transport{Proxy: http.ProxyURL(u)}, nil
}

// maxArchiveSize 归档体积上限，防止超大响应打爆磁盘。
const maxArchiveSize = 200 << 20 // 200 MiB

// checksumAssetName 发布产物里校验和文件的固定名（见 .goreleaser.yaml）。
const checksumAssetName = "checksums.txt"

// ErrNoUpdate 已是最新版本。
var ErrNoUpdate = errors.New("already up to date")

// ErrNoBackup 没有可回滚的备份。
var ErrNoBackup = errors.New("no backup binary found")

// ErrUnsupported 当前部署形态不支持自更新（如 Windows 的文件锁）。
var ErrUnsupported = errors.New("self-update is not supported in this environment")

// Release 一次发布。
type Release struct {
	Version     string    `json:"version"`
	TagName     string    `json:"tag_name"`
	Notes       string    `json:"notes,omitempty"`
	HTMLURL     string    `json:"html_url,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	Assets      []Asset   `json:"assets,omitempty"`
}

// Asset 发布产物中的一个文件。
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
}

// Status 检测结果，直接作为面板的数据源。
type Status struct {
	Current     string `json:"current"`
	Latest      string `json:"latest,omitempty"`
	HasUpdate   bool   `json:"has_update"`
	Notes       string `json:"notes,omitempty"`
	HTMLURL     string `json:"html_url,omitempty"`
	CanRollback bool   `json:"can_rollback"`
	Supported   bool   `json:"supported"`
	Reason      string `json:"reason,omitempty"`
}

// 默认放行的下载域名：GitHub 的 release 资源会重定向到这些主机。
var defaultAllowedHosts = []string{
	"github.com",
	"objects.githubusercontent.com",
	"release-assets.githubusercontent.com",
	"api.github.com",
}

// Service 更新服务。零值不可用，必须经 New 构造。
type Service struct {
	repo        string // owner/name
	current     string // 当前版本，如 v1.2.3 或 dev
	client      *http.Client
	apiBase     string // 覆盖 GitHub API 基址（测试用）
	exePath     string // 覆盖可执行文件路径（测试用）
	lock        sync.Mutex
	supportedFn func() bool
	// allowedHosts 下载域名白名单；默认见 defaultAllowedHosts。
	// 下载白名单：默认只允许 GitHub 系域名；http 与未知主机一律拒绝。
	// 测试用 httptest 时会显式放开 127.0.0.1。
	allowedHosts []string
	// proxyURL 更新流量的代理地址（http/https/socks5）；空为直连。
	// 境内服务器直连 GitHub 下载域名常被断连，走代理是 sub2api 同款解法。
	proxyURL string
}

// Option 构造选项。
type Option func(*Service)

// WithHTTPClient 注入 HTTP 客户端（测试用）。
func WithHTTPClient(c *http.Client) Option { return func(s *Service) { s.client = c } }

// WithAPIBase 覆盖 API 基址（测试用，需与 WithHTTPClient 配合指向 httptest）。
func WithAPIBase(base string) Option { return func(s *Service) { s.apiBase = base } }

// WithExePath 覆盖被替换的可执行文件路径（测试用，正常应留空取自 os.Executable）。
func WithExePath(p string) Option { return func(s *Service) { s.exePath = p } }

// WithAllowedHosts 覆盖下载域名白名单。默认只允许 GitHub 系域名；
// 自建镜像（如内网 Gitea/GitLab 转发 release 资源）或测试场景可用它放开。
func WithAllowedHosts(hosts ...string) Option {
	return func(s *Service) { s.allowedHosts = hosts }
}

// WithProxy 为更新流量配置代理（对齐 sub2api 的 update.proxy_url：境内服务器
// 直连 GitHub 的下载域名常被断连，走代理是官方解决方案）。支持
// http/https/socks5 协议；空串恢复直连。仅在构造期生效。
func WithProxy(proxyURL string) Option {
	return func(s *Service) { s.proxyURL = strings.TrimSpace(proxyURL) }
}

// New 构造更新服务。repo 形如 "owner/name"，version 为当前版本。
func New(repo, version string, opts ...Option) *Service {
	s := &Service{
		repo:         strings.TrimSpace(repo),
		current:      strings.TrimSpace(version),
		client:       &http.Client{Timeout: 10 * time.Minute},
		apiBase:      "https://api.github.com",
		supportedFn:  defaultSupported,
		allowedHosts: defaultAllowedHosts,
	}
	for _, o := range opts {
		o(s)
	}
	if s.proxyURL != "" {
		tr, err := newProxyTransport(s.proxyURL)
		if err != nil {
			log.Printf("update: proxy %q init failed, using direct connection: %v", s.proxyURL, err)
		} else {
			s.client.Transport = tr
		}
	}
	return s
}

// defaultSupported 判断当前环境是否支持自更新。
//
// Linux 一律支持，容器内也不例外（对齐 sub2api）：替换的是容器可写层里的
// 二进制，退出后由 restart 策略拉起，新文件仍在；唯一注意点是下次
// `docker compose up --build` 重建镜像会盖掉它，文档已说明。非 Linux 平台
// 无法自替换正在运行的二进制（Windows 文件锁），拒绝。
func defaultSupported() bool {
	return runtime.GOOS == "linux"
}

// Detected 报告该环境是否支持自更新，附带原因。
func (s *Service) Detected() (bool, string) {
	if s.supportedFn() {
		return true, ""
	}
	return false, "仅 Linux 支持在线更新（systemd 与容器部署均可，当前 " + runtime.GOOS + "）"
}

// Current 当前版本。
func (s *Service) Current() string { return s.current }

// executablePath 解析真实可执行文件路径（穿透符号链接，保证 rename 落在同一文件系统）。
func (s *Service) executablePath() (string, error) {
	if s.exePath != "" {
		return s.exePath, nil
	}
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		// 符号链接解析失败不致命，退回去用原始路径。
		return p, nil
	}
	return resolved, nil
}

// backupPath 备份文件路径。
func backupPath(exe string) string { return exe + ".backup" }

// Check 查询最新版本并与当前版本比较。
func (s *Service) Check(ctx context.Context) (*Status, error) {
	supported, reason := s.Detected()
	st := &Status{
		Current:   s.current,
		Supported: supported,
		Reason:    reason,
	}
	if exe, err := s.executablePath(); err == nil {
		if _, err := os.Stat(backupPath(exe)); err == nil {
			st.CanRollback = true
		}
	}
	if !supported {
		return st, nil
	}
	rel, err := s.latestRelease(ctx)
	if err != nil {
		return st, err
	}
	st.Latest = rel.Version
	st.Notes = rel.Notes
	st.HTMLURL = rel.HTMLURL
	st.HasUpdate = newerVersion(rel.TagName, s.current)
	return st, nil
}

// Apply 下载并替换可执行文件。成功后需要由调用方触发重启。
//
// 全程持锁：并发 Apply 会让两次 rename 交叠（A 已把 exe 改名成备份、B 又去
// 改名就会失败或留下错乱状态），必须串行。handler 层还有一层 TryLock 用于
// 给用户返回 409，这里是保证正确性的那一道。
func (s *Service) Apply(ctx context.Context) (*Status, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	st, err := s.Check(ctx)
	if err != nil {
		return st, err
	}
	if !st.Supported {
		return st, ErrUnsupported
	}
	if !st.HasUpdate {
		return st, ErrNoUpdate
	}
	rel, err := s.latestRelease(ctx)
	if err != nil {
		return st, err
	}
	if err := s.applyRelease(ctx, rel); err != nil {
		return st, err
	}
	st.CanRollback = true
	return st, nil
}

// latestRelease 取最新 release。
func (s *Service) latestRelease(ctx context.Context) (*Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", s.apiBase, s.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch latest release: http %d", res.StatusCode)
	}
	var raw struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	rel := &Release{
		Version:     strings.TrimPrefix(raw.TagName, "v"),
		TagName:     raw.TagName,
		Notes:       raw.Body,
		HTMLURL:     raw.HTMLURL,
		PublishedAt: raw.PublishedAt,
	}
	for _, a := range raw.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, DownloadURL: a.BrowserDownloadURL})
	}
	return rel, nil
}

// archiveName 当前平台对应的归档名（与 .goreleaser.yaml 的 name_template 对应）。
func archiveName() string {
	return fmt.Sprintf("codearts2api_%s_%s", runtime.GOOS, runtime.GOARCH)
}

// pickAssets 在发布产物里挑出当前平台的归档与校验和文件。
func pickAssets(assets []Asset) (archive, checksum Asset, err error) {
	want := archiveName()
	for _, a := range assets {
		switch {
		case a.Name == checksumAssetName:
			checksum = a
		case strings.Contains(a.Name, want) && !strings.HasSuffix(a.Name, ".txt"):
			archive = a
		}
	}
	if archive.DownloadURL == "" {
		return archive, checksum, fmt.Errorf("no compatible release asset for %s/%s (want %s*)", runtime.GOOS, runtime.GOARCH, want)
	}
	return archive, checksum, nil
}

// applyRelease 下载 → 校验 → 解包 → 原子替换。
func (s *Service) applyRelease(ctx context.Context, rel *Release) error {
	archive, checksum, err := pickAssets(rel.Assets)
	if err != nil {
		return err
	}
	if err := s.validateDownloadURL(archive.DownloadURL); err != nil {
		return err
	}
	if checksum.DownloadURL == "" {
		// 宁可失败也不装未校验的二进制：它随后要持有账号凭证。
		return errors.New("release has no checksums.txt; refusing to install an unverified binary")
	}
	if err := s.validateDownloadURL(checksum.DownloadURL); err != nil {
		return err
	}

	exe, err := s.executablePath()
	if err != nil {
		return err
	}
	// 临时文件必须与可执行文件同目录：rename 只在同一文件系统内才是原子的。
	tmpDir, err := os.MkdirTemp(filepath.Dir(exe), ".update-*")
	if err != nil {
		return fmt.Errorf("create temp dir next to executable: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	if err := s.download(ctx, archive.DownloadURL, archivePath); err != nil {
		return err
	}

	// 用 asset 名而非下载 URL 末段来比对校验和：URL 末段并不保证等于文件名
	// （GitHub 会重定向，自建镜像也可能改写路径）。
	wantSum, err := s.fetchChecksum(ctx, checksum.DownloadURL, archive.Name)
	if err != nil {
		return err
	}
	if err := verifySHA256(archivePath, wantSum); err != nil {
		return err
	}

	newExe := filepath.Join(tmpDir, "codearts2api.new")
	if err := extractBinary(archivePath, newExe); err != nil {
		return err
	}
	if err := os.Chmod(newExe, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}
	// 先把新二进制刷到盘上再换名：rename 只保证目录项原子可见，不保证文件内容
	// 已落盘。换名后紧跟断电/被 kill -9 时会留下指向空文件的目录项——上一版还在
	// 备份里，但重启拉起的进程起不来，对用户就是彻底不可用。
	if err := syncFile(newExe); err != nil {
		return fmt.Errorf("flush new binary: %w", err)
	}

	return replaceExecutable(exe, newExe)
}

// syncFile 把文件内容刷盘（fsync）。
//
// 用 O_RDWR 而非只读句柄：Windows 的 FlushFileBuffers 要求句柄带写权限，
// 只读句柄会被拒（Access is denied），而本函数就发生在自己的临时文件上。
func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// syncDir 刷目录项，让 rename 的结果真正落到盘上。
// 目录 fsync 在部分平台（Windows）不被支持，失败不致命，忽略即可。
func syncDir(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	defer f.Close()
	_ = f.Sync()
}

// replaceExecutable 原子替换：旧版先改名成备份，再让新版就位；第二步失败则回滚。
//
// 不先删旧备份：rename 在 POSIX 上是原子覆盖目标，旧备份被覆盖的同一瞬间新备份
// 就已经就位，中间不存在「没有备份」的窗口（原实现在这里留了一个）。备份是回滚
// 与失败恢复的唯一凭据，不能提前删。
func replaceExecutable(exe, newExe string) error {
	backup := backupPath(exe)
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	syncDir(filepath.Dir(exe))
	if err := os.Rename(newExe, exe); err != nil {
		if rerr := os.Rename(backup, exe); rerr != nil {
			// 已无路可退：明确告知用户手工恢复的命令。
			return fmt.Errorf("install new binary: %w (and restore failed: %v; recover manually: mv %s %s)", err, rerr, backup, exe)
		}
		return fmt.Errorf("install new binary: %w", err)
	}
	syncDir(filepath.Dir(exe))
	return nil
}

// Rollback 用备份换回上一版。同样持锁，避免与 Apply 并发。
func (s *Service) Rollback() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	exe, err := s.executablePath()
	if err != nil {
		return err
	}
	backup := backupPath(exe)
	if _, err := os.Stat(backup); err != nil {
		return ErrNoBackup
	}
	if err := os.Rename(backup, exe); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}

// download 下载到 dest，带体积上限。
func (s *Service) download(ctx context.Context, rawURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download: http %d", res.StatusCode)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(res.Body, maxArchiveSize+1))
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > maxArchiveSize {
		return fmt.Errorf("download exceeded %d bytes", int64(maxArchiveSize))
	}
	return nil
}

// fetchChecksum 拉 checksums.txt 并取出 target 的期望值。
func (s *Service) fetchChecksum(ctx context.Context, rawURL, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch checksums: http %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	sum := checksumFor(string(body), target)
	if sum == "" {
		return "", fmt.Errorf("checksums.txt has no entry for %s", target)
	}
	return sum, nil
}

// checksumFor 从 goreleaser 风格的 checksums.txt 中取文件名对应的 sha256。
// 兼容两种行格式："<sum>  <name>" 与 "<sum> *<name>"（二进制模式）。
func checksumFor(content, target string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) != target {
			continue
		}
		if _, err := hex.DecodeString(fields[0]); err != nil || len(fields[0]) != 64 {
			continue
		}
		return strings.ToLower(fields[0])
	}
	return ""
}

// verifySHA256 校验文件摘要。
func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, want)
	}
	return nil
}

// extractBinary 从 .tar.gz 里取出名为 codearts2api 的可执行文件。
func extractBinary(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		// 归档里可能含子目录，按基名匹配，且只接受普通文件。
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "codearts2api" {
			continue
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		// 限制单文件体积，避免 tar 头声明的尺寸被滥用。
		if _, err := io.Copy(out, io.LimitReader(tr, maxArchiveSize)); err != nil {
			out.Close()
			return fmt.Errorf("extract binary: %w", err)
		}
		return out.Close()
	}
	return errors.New("archive does not contain codearts2api binary")
}

// validateDownloadURL 只放行 HTTPS + 白名单域名，防止 SSRF 与投毒。
func (s *Service) validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("only https downloads are allowed, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range s.allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return fmt.Errorf("download host %q is not allowed", host)
}

// newerVersion 判断 tag 是否比 current 新。无法解析时退化为字符串不等判断：
// 宁可让用户看到「有更新」再自行决定，也不要因为解析失败而永远不提示。
func newerVersion(tag, current string) bool {
	cur := strings.TrimPrefix(strings.TrimSpace(current), "v")
	next := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if next == "" {
		return false
	}
	if cur == "" || cur == "dev" {
		// 开发构建：凡有正式 tag 都算可更新。
		return true
	}
	// 带预发布后缀（如 1.2.3-rc1）一律视为不自动升级。
	if strings.ContainsAny(next, "-+") {
		return next != cur
	}
	cmp, ok := compareSemver(next, cur)
	if !ok {
		return next != cur
	}
	return cmp > 0
}

// compareSemver 比较点分数字版本。返回 -1/0/1；解析失败返回 ok=false。
func compareSemver(a, b string) (int, bool) {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			n, err := strconv.Atoi(strings.TrimSpace(as[i]))
			if err != nil {
				return 0, false
			}
			av = n
		}
		if i < len(bs) {
			n, err := strconv.Atoi(strings.TrimSpace(bs[i]))
			if err != nil {
				return 0, false
			}
			bv = n
		}
		if av != bv {
			if av > bv {
				return 1, true
			}
			return -1, true
		}
	}
	return 0, true
}
