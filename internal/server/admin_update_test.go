package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codearts2api/internal/auth"
	"codearts2api/internal/pool"
	"codearts2api/internal/upstream"
)

// newUpdateTestHandler 构造一个带账号池的 handler，用于验证更新端点的鉴权与降级行为。
func newUpdateTestHandler(t *testing.T, version, updateRepo string) *Handler {
	t.Helper()
	a := auth.New("update-account", "tester", "domain", "token", "ak", "sk",
		time.Now().Add(time.Hour).Format(time.RFC3339), "", "")
	p, err := pool.New([]*auth.Auth{a}, pool.Config{MaxConcurrent: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	p.Accounts()[0].Client = upstream.New(5 * time.Second)
	return NewHandler(Config{
		Pool:         p,
		APIKey:       "secret-key",
		MaxRotate:    1,
		DefaultModel: "glm-5.2",
		Version:      version,
		UpdateRepo:   updateRepo,
	})
}

// 更新端点必须鉴权：匿名可触发等于给了远程替换二进制的入口。
func TestUpdateEndpointsRequireAuth(t *testing.T) {
	h := newUpdateTestHandler(t, "v1.0.0", "test/repo")
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/admin/api/update/check"},
		{http.MethodPost, "/admin/api/update/apply"},
		{http.MethodPost, "/admin/api/update/rollback"},
		{http.MethodPost, "/admin/api/update/restart"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without key = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

// 未配置 update_repo 时检测端点要明确说「未配置」，而不是报错或假装没有更新。
func TestUpdateCheckReportsUnconfigured(t *testing.T) {
	h := newUpdateTestHandler(t, "v1.0.0", "")
	req := httptest.NewRequest(http.MethodGet, "/admin/api/update/check", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["configured"] != false {
		t.Fatalf("configured=%v want false; body=%s", got["configured"], rec.Body.String())
	}
	if msg, _ := got["message"].(string); !strings.Contains(msg, "update_repo") {
		t.Fatalf("message should point at update_repo: %q", msg)
	}
}

// 未配置时执行更新要给出可读原因，而不是 500。
func TestUpdateApplyUnconfigured(t *testing.T) {
	h := newUpdateTestHandler(t, "v1.0.0", "")
	req := httptest.NewRequest(http.MethodPost, "/admin/api/update/apply", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "update_repo") {
		t.Fatalf("body should mention update_repo: %s", rec.Body.String())
	}
}

// /status 要带版本，便于远程确认当前跑的是哪一版。
func TestStatusIncludesVersion(t *testing.T) {
	h := newUpdateTestHandler(t, "v1.2.3", "")
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"version":"v1.2.3"`) {
		t.Fatalf("status body=%s", rec.Body.String())
	}
}

// /admin/api/overview 带版本，供面板设置菜单显示。
func TestOverviewIncludesVersion(t *testing.T) {
	h := newUpdateTestHandler(t, "v9.8.7", "")
	req := httptest.NewRequest(http.MethodGet, "/admin/api/overview", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "v9.8.7") {
		t.Fatalf("overview body=%s", rec.Body.String())
	}
}
