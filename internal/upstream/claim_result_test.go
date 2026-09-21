package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 领取幂等：上游已领过时同样返回 error_code=0000，且 create_time 保持不动。
// 实测 2026-09-21 抓包：POST 发生在 2026-09-22 00:43:12Z，返回的 create_time
// 仍是 1789653100754（2026-09-17 13:51:40Z，早 4.45 天）。因此「本次是否新领到」
// 只能靠调用前后的 create_time 比较，不能靠 HTTP 方法或响应形状。
func TestClaimResultStatusDistinguishesIdempotentCall(t *testing.T) {
	const claimed = int64(1789653100754)

	tests := []struct {
		name   string
		result ClaimResult
		before int64
		want   ClaimStatus
	}{
		{
			name:   "此前就有记录且未变 = 幂等空转",
			result: ClaimResult{CreateTime: claimed},
			before: claimed,
			want:   ClaimExisting,
		},
		{
			name:   "从未领取过、这次拿到时间 = 新领到",
			result: ClaimResult{CreateTime: claimed},
			before: 0,
			want:   ClaimNew,
		},
		{
			name:   "时间被刷新 = 新领到",
			result: ClaimResult{CreateTime: claimed},
			before: claimed - 86400000,
			want:   ClaimNew,
		},
		{
			name:   "上游没回时间 = 无法判断",
			result: ClaimResult{CreateTime: 0},
			before: claimed,
			want:   ClaimUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.Status(tc.before); got != tc.want {
				t.Errorf("Status(%d) = %v, want %v", tc.before, got, tc.want)
			}
		})
	}
}

// ClaimBenefit 不能丢弃响应体：create_time 是判定领取结论的唯一依据，
// 夹具直接取自真实抓包的 POST 响应。
func TestClaimBenefitReturnsCreateTime(t *testing.T) {
	const body = `{"error_code":"0000","error_msg":"success","result":{` +
		`"channel":"codearts","create_time":1789653100754,` +
		`"domain_id":"086b0271090025320f7cc013fac68b40","expire_time":0,` +
		`"update_time":1789653100754,` +
		`"user_id":"086b0271c580f3ee1f88c01314631a3a","user_name":"hj01857655"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("claim 必须是 POST，got %s", r.Method)
		}
		if r.URL.Path != EpBenefitClaim {
			t.Errorf("path = %s, want %s", r.URL.Path, EpBenefitClaim)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New(5 * time.Second)
	c.snapBase = srv.URL
	c.benefitBase = srv.URL

	got, err := c.ClaimBenefit(SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK", SecurityToken: "ST"})
	if err != nil {
		t.Fatal(err)
	}
	if got.CreateTime != 1789653100754 {
		t.Errorf("CreateTime = %d, want 1789653100754", got.CreateTime)
	}
	if got.UserName != "hj01857655" {
		t.Errorf("UserName = %q, want hj01857655", got.UserName)
	}
}
