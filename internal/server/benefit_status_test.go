package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codearts2api/internal/auth"
	"codearts2api/internal/pool"
	"codearts2api/internal/upstream"
)

// 余额查询失败时不能静默：benefitResult 必须报出失败原因且 has_balance=false，
// 否则面板把它当 0 显示，读起来像「额度已用尽」，还会被折进合计行。
func TestBenefitStatusReportsBalanceFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/benefit/claim" {
			// 签到状态成功，create_time=0 表示从未领取。
			_, _ = w.Write([]byte(`{"error_code":"0000","result":{"create_time":0}}`))
			return
		}
		// 余额接口失败。
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error_msg":"balance backend down","error_code":"APIG.9999"}`))
	}))
	defer srv.Close()

	a := auth.New("benefit-account", "tester", "domain", "token", "ak", "sk",
		time.Now().Add(time.Hour).Format(time.RFC3339), "", "")
	p, err := pool.New([]*auth.Auth{a}, pool.Config{MaxConcurrent: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	c := upstream.New(5 * time.Second)
	c.SetModelHostsForTest(srv.URL, srv.URL)
	h := NewHandler(Config{Pool: p, Upstream: c, APIKey: "secret-key", MaxRotate: 1})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/benefit/status", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Results []struct {
			UID        string `json:"uid"`
			OK         bool   `json:"ok"`
			Message    string `json:"message"`
			HasBalance bool   `json:"has_balance"`
			Total      int64  `json:"total"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results = %d, want 1; body=%s", len(payload.Results), rec.Body.String())
	}
	got := payload.Results[0]
	if got.HasBalance {
		t.Error("余额查询失败时 has_balance 必须为 false，否则面板会把 0 当成真实额度")
	}
	if got.Message == "" {
		t.Error("余额查询失败必须带 message，不能静默")
	}
	if got.Total != 0 {
		t.Errorf("余额失败时不应有额度数字，got total=%d", got.Total)
	}
}
