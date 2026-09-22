package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codearts2api/internal/auth"
	"codearts2api/internal/pool"
	"codearts2api/internal/upstream"
)

// 容器 healthcheck 与反代按 /healthz 的状态码决定是否摘实例。账号池全挂时
// 仍回 200，会把请求持续打到必然失败的实例上。
func TestHealthzReportsNoHealthyAccount(t *testing.T) {
	p, err := pool.New([]*auth.Auth{{UserID: "u1", UserName: "u1"}}, pool.Config{}, "")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Config{Pool: p, Upstream: upstream.New(time.Second)})

	rec := httptest.NewRecorder()
	h.healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("有健康账号时应 200，got=%d", rec.Code)
	}

	p.Disable("u1", "test")
	rec = httptest.NewRecorder()
	h.healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("无健康账号时应 503，got=%d", rec.Code)
	}
}

// 黏性路由必须跨重启保留：convAcct 是请求期就读的状态，只留在内存里意味着
// 重启后多轮会话被切到别的账号（上游并发会话槽位也随之翻倍占用）。
func TestConvStatePersistsStickyRoute(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json.chats.json")
	h := NewHandler(Config{ConvStateFile: stateFile, Upstream: upstream.New(time.Second)})
	h.convMu.Lock()
	h.chats["acct-a"] = "chatid-1"
	h.convAcct["conv-1"] = convRoute{Account: "acct-a", At: time.Now().Unix()}
	h.convMu.Unlock()
	h.saveChats()

	reloaded := NewHandler(Config{ConvStateFile: stateFile, Upstream: upstream.New(time.Second)})
	reloaded.convMu.Lock()
	route := reloaded.convAcct["conv-1"].Account
	chatID := reloaded.chats["acct-a"]
	reloaded.convMu.Unlock()
	if route != "acct-a" {
		t.Errorf("重启后黏性路由应保留，got=%q", route)
	}
	if chatID != "chatid-1" {
		t.Errorf("重启后 chat_id 应保留，got=%q", chatID)
	}
}

// 过期路由要淘汰，否则 convAcct 随 conversation_id 数量无限增长。
func TestConvStatePrunesExpiredRoutes(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json.chats.json")
	h := NewHandler(Config{ConvStateFile: stateFile, Upstream: upstream.New(time.Second)})
	stale := time.Now().Add(-convStateTTL - time.Hour).Unix()
	h.convMu.Lock()
	h.convAcct["conv-old"] = convRoute{Account: "acct-a", At: stale}
	h.convAcct["conv-new"] = convRoute{Account: "acct-a", At: time.Now().Unix()}
	h.convMu.Unlock()
	h.saveChats()

	if _, ok := h.convAcct["conv-old"]; ok {
		t.Error("落盘时应顺手淘汰过期路由")
	}
	reloaded := NewHandler(Config{ConvStateFile: stateFile, Upstream: upstream.New(time.Second)})
	if _, ok := reloaded.convAcct["conv-old"]; ok {
		t.Error("旧格式/过期路由不应在重启后复活")
	}
	if _, ok := reloaded.convAcct["conv-new"]; !ok {
		t.Error("未过期路由应保留")
	}
}

// 旧版本写的是扁平 account→chat_id map，升级后仍要能读。
func TestConvStateReadsLegacyFlatFormat(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json.chats.json")
	if err := os.WriteFile(stateFile, []byte(`{"acct-a":"chatid-legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Config{ConvStateFile: stateFile, Upstream: upstream.New(time.Second)})
	if h.chats["acct-a"] != "chatid-legacy" {
		t.Errorf("旧格式 chats 应可读，got=%q", h.chats["acct-a"])
	}
}
