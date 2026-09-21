package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"codearts2api/internal/auth"
	"codearts2api/internal/pool"
	"codearts2api/internal/upstream"
)

// 强制刷新必须真的重打上游：内存缓存与账号目录 TTL 都不能挡住它，
// 否则面板「重新获取」只是把 1h 前的旧列表再显示一遍。
func TestModelsRefreshBypassesCaches(t *testing.T) {
	// dynamicModelsCache 是包级变量：本测试要写入它，必须原值归还，
	// 否则同包后续读 /v1/models 的测试会拿到这里发现的模型。
	dynamicModelsCache.Lock()
	prevIDs, prevFetched, prevFail := dynamicModelsCache.ids, dynamicModelsCache.fetched, dynamicModelsCache.lastFail
	dynamicModelsCache.Unlock()
	defer func() {
		dynamicModelsCache.Lock()
		dynamicModelsCache.ids, dynamicModelsCache.fetched, dynamicModelsCache.lastFail = prevIDs, prevFetched, prevFail
		dynamicModelsCache.Unlock()
	}()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 只让内置来源成功（其余来源失败不影响计数）：模型发现能走通，
		// 且成功的来源不进入退避，第二次强制刷新必定再打一次上游。
		if r.URL.Path == "/v1/model/builtin" {
			atomic.AddInt32(&hits, 1)
			_, _ = io.WriteString(w, `{"builtinModels":[{"model_id":"OpenPangu-2.0-Pro","context_window":524288}]}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	a := auth.New("refresh-account", "tester", "domain", "token", "ak", "sk",
		time.Now().Add(time.Hour).Format(time.RFC3339), "", "")
	p, err := pool.New([]*auth.Auth{a}, pool.Config{MaxConcurrent: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	acct := p.Accounts()[0]
	c := upstream.New(5 * time.Second)
	c.SetModelHostsForTest(srv.URL, srv.URL)
	acct.Client = c

	// 模型发现走 handler 的 Config.Upstream，不是账号上的 client，两处都要换。
	h := NewHandler(Config{Pool: p, Upstream: c, APIKey: "secret-key", MaxRotate: 1, DefaultModel: "glm-5.2"})
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/models/refresh", nil)
		req.Header.Set("Authorization", "Bearer secret-key")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := call(); rec.Code != http.StatusOK {
		t.Fatalf("refresh = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	first := atomic.LoadInt32(&hits)
	if first == 0 {
		t.Fatal("强制刷新没有打上游")
	}
	// 紧接着再刷一次：缓存已新鲜，但 force 必须仍然重打。
	if rec := call(); rec.Code != http.StatusOK {
		t.Fatalf("second refresh = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if second := atomic.LoadInt32(&hits); second <= first {
		t.Errorf("第二次强制刷新被缓存挡住：上游命中数仍为 %d", second)
	}
}
