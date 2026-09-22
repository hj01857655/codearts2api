package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// testAsset 发布的归档名与 internal/update 的匹配规则一致，测试里直接复用该函数。
func testAssetName() string { return archiveName() + ".tar.gz" }

// buildArchive 生成一个 .tar.gz，内含名为 codearts2api 的可执行文件。
func buildArchive(t *testing.T, binaryContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// 故意放在子目录里：解包按基名匹配，应当仍能取到。
	body := []byte(binaryContent)
	hdr := &tar.Header{Name: "codearts2api/codearts2api", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fakeGitHub 起一个假 GitHub API + 下载服务（TLS，使下载 URL 为 https，
// 与生产「仅允许 https」的规则一致）。badChecksum=true 时 checksums.txt 里放
// 错误摘要，用于验证「校验失败必须中止」。
func fakeGitHub(t *testing.T, archive []byte, badChecksum bool) *httptest.Server {
	t.Helper()
	sum := sha256Hex(archive)
	if badChecksum {
		sum = strings.Repeat("0", 64)
	}
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v9.9.9",
			"body":     "test release",
			"html_url": base + "/releases/v9.9.9",
			"assets": []map[string]any{
				{"name": testAssetName(), "browser_download_url": base + "/dl/archive"},
				{"name": "checksums.txt", "browser_download_url": base + "/dl/checksums"},
			},
		})
	})
	mux.HandleFunc("/dl/archive", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/dl/checksums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", sum, testAssetName())
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	base = srv.URL
	return srv
}

// newTestService 构造一个指向假 GitHub、可执行文件在临时目录的服务。
// 白名单只放开测试服务器主机；生产默认仍只允许 https + GitHub 系域名。
func newTestService(t *testing.T, srv *httptest.Server, current string) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "codearts2api")
	if err := os.WriteFile(exe, []byte("OLD-BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New("test/repo", current,
		WithExePath(exe),
		WithAPIBase(srv.URL),
		WithHTTPClient(srv.Client()),
		WithAllowedHosts("127.0.0.1"),
	)
	// 测试环境是 Windows/容器时也要能跑替换逻辑，因此放开平台检查。
	s.supportedFn = func() bool { return true }
	return s, exe
}

func TestApplyReplacesBinaryAndKeepsBackup(t *testing.T) {
	archive := buildArchive(t, "NEW-BINARY")
	srv := fakeGitHub(t, archive, false)
	s, exe := newTestService(t, srv, "v1.0.0")

	st, err := s.Apply(context.Background())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !st.HasUpdate || st.Latest != "9.9.9" {
		t.Fatalf("status=%+v", st)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW-BINARY" {
		t.Fatalf("binary not replaced: %q", got)
	}
	backup, err := os.ReadFile(exe + ".backup")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != "OLD-BINARY" {
		t.Fatalf("backup content=%q", backup)
	}
	// 执行位只在 Unix 上有意义：Windows 的 Chmod 不设 Unix 权限位，
	// 而自更新本来也只支持 Linux。
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(exe); err != nil || fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("replaced binary is not executable: %v %v", fi, err)
		}
	}
}

func TestApplyAbortsOnChecksumMismatch(t *testing.T) {
	archive := buildArchive(t, "EVIL-BINARY")
	srv := fakeGitHub(t, archive, true)
	s, exe := newTestService(t, srv, "v1.0.0")

	if _, err := s.Apply(context.Background()); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	// 关键：现有二进制必须原封不动，且不得留下备份造成误回滚。
	if got, _ := os.ReadFile(exe); string(got) != "OLD-BINARY" {
		t.Fatalf("existing binary was touched: %q", got)
	}
	if _, err := os.Stat(exe + ".backup"); err == nil {
		t.Fatal("checksum failure must not create a backup")
	}
}

func TestApplyRefusesReleaseWithoutChecksums(t *testing.T) {
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v9.9.9",
			"assets": []map[string]any{
				{"name": testAssetName(), "browser_download_url": base + "/dl/archive"},
			},
		})
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	base = srv.URL
	s, exe := newTestService(t, srv, "v1.0.0")

	_, err := s.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("expected refusal without checksums, got %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD-BINARY" {
		t.Fatalf("binary touched: %q", got)
	}
}

func TestApplyReportsMissingPlatformAsset(t *testing.T) {
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v9.9.9",
			"assets": []map[string]any{
				{"name": "codearts2api_windows_386.tar.gz", "browser_download_url": base + "/a"},
				{"name": "checksums.txt", "browser_download_url": base + "/c"},
			},
		})
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	base = srv.URL
	s, _ := newTestService(t, srv, "v1.0.0")

	_, err := s.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no compatible release asset") {
		t.Fatalf("expected clear missing-asset error, got %v", err)
	}
}

func TestRollbackRestoresBackup(t *testing.T) {
	archive := buildArchive(t, "NEW-BINARY")
	srv := fakeGitHub(t, archive, false)
	s, exe := newTestService(t, srv, "v1.0.0")

	if _, err := s.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := s.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD-BINARY" {
		t.Fatalf("rollback did not restore: %q", got)
	}
	if err := s.Rollback(); err == nil {
		t.Fatal("second rollback should fail (backup consumed)")
	}
}

func TestApplySkipsWhenUpToDate(t *testing.T) {
	archive := buildArchive(t, "NEW-BINARY")
	srv := fakeGitHub(t, archive, false)
	s, exe := newTestService(t, srv, "v9.9.9")

	_, err := s.Apply(context.Background())
	if err != ErrNoUpdate {
		t.Fatalf("expected ErrNoUpdate, got %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "OLD-BINARY" {
		t.Fatalf("binary touched: %q", got)
	}
}

func TestValidateDownloadURLRejectsUntrustedHosts(t *testing.T) {
	s := New("test/repo", "v1")
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com/a/b/releases/download/v1/x.tar.gz", true},
		{"https://objects.githubusercontent.com/x", true},
		{"https://release-assets.githubusercontent.com/x", true},
		{"http://github.com/a", false},          // 非 https
		{"https://evil.example.com/x", false},   // 非白名单
		{"https://github.com.evil.io/x", false}, // 后缀伪装
		{"file:///etc/passwd", false},
	}
	for _, c := range cases {
		err := s.validateDownloadURL(c.url)
		if c.want && err != nil {
			t.Errorf("validateDownloadURL(%q) = %v, want ok", c.url, err)
		}
		if !c.want && err == nil {
			t.Errorf("validateDownloadURL(%q) = nil, want error", c.url)
		}
	}
	// 自建镜像：显式放开白名单后应当可用，但不影响默认拒绝。
	mirror := New("test/repo", "v1", WithAllowedHosts("mirror.internal"))
	if err := mirror.validateDownloadURL("https://mirror.internal/x.tar.gz"); err != nil {
		t.Fatalf("explicit allowlist not honored: %v", err)
	}
}

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v1.2.3", "v1.2.2", true},
		{"v1.2.2", "v1.2.2", false},
		{"v1.3.0", "v1.2.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.2.3", "dev", true}, // 开发构建总是可更新
		{"v1.2.3", "", true},    // 无版本号同样提示
		{"v1.2.3-rc1", "v1.2.2", true},
		{"", "v1.2.2", false}, // 无 tag 不提示
	}
	for _, c := range cases {
		if got := newerVersion(c.tag, c.current); got != c.want {
			t.Errorf("newerVersion(%q,%q)=%v want %v", c.tag, c.current, got, c.want)
		}
	}
}

// TestConcurrentApplyDoesNotCorrupt 验证并发 Apply 不会把二进制换坏。
// 串行化后每次都可能成功（当前版本号不会变），所以这里断言的是不变量：
// 结束时 exe 必须是完好的新二进制，而不是被两次 rename 交叉后的残缺状态。
func TestConcurrentApplyDoesNotCorrupt(t *testing.T) {
	srv := fakeGitHub(t, buildArchive(t, "NEW-BINARY"), false)
	s, exe := newTestService(t, srv, "v1.0.0")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 只关心不崩、不写坏；错误在下面用文件内容判定。
			_, _ = s.Apply(context.Background())
		}()
	}
	wg.Wait()

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("executable vanished after concurrent apply: %v", err)
	}
	if string(got) != "NEW-BINARY" {
		t.Fatalf("executable corrupted by concurrent apply: %q", got)
	}
}

func TestChecksumForParsesGoreleaserFormats(t *testing.T) {
	content := "abc  file1.tar.gz\ndef *file2.tar.gz\n" + strings.Repeat("a", 64) + "  " + testAssetName() + "\n"
	got := checksumFor(content, testAssetName())
	if got != strings.Repeat("a", 64) {
		t.Fatalf("checksumFor=%q", got)
	}
	if checksumFor(content, "missing.tar.gz") != "" {
		t.Fatal("expected empty for missing entry")
	}
	// 二进制模式（* 前缀）与回车换行都要能解析（用合法长度摘要避免误判格式）。
	sum := strings.Repeat("b", 64)
	if got := checksumFor(sum+" *file2.tar.gz\r\n", "file2.tar.gz"); got != sum {
		t.Fatalf("binary-mode/CRLF parsing failed: %q", got)
	}
}
