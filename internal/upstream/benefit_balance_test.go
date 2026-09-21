package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 上游 /api/v1/user/tokens/balance 同时给日、月两个维度，且 total_quota /
// used_amount 只是日维度的别名（夹具取自 2026-09-21 的真实响应）：
// daily_token_limit == total_quota == 10000000，daily_tokens_used == used_amount == 549，
// 而 monthly_tokens_used == 10091038。只解析别名那三个数，面板就会把「本月用了
// 1009 万」显示成「已用 549」。
func TestBenefitBalanceDetailKeepsBothDimensions(t *testing.T) {
	const body = `{"error_code":"0000","error_msg":"success","result":{` +
		`"channel":"codearts","create_time":1789653100754,` +
		`"daily_token_limit":10000000,"daily_tokens_used":549,` +
		`"domain_id":"d","expire_time":0,` +
		`"monthly_token_limit":0,"monthly_tokens_used":10091038,` +
		`"total_balance":9999451,"total_quota":10000000,` +
		`"update_time":1789653100754,"used_amount":549,` +
		`"user_id":"u"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EpBenefitBalance {
			t.Errorf("path = %s, want %s", r.URL.Path, EpBenefitBalance)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New(5 * time.Second)
	c.snapBase = srv.URL
	c.benefitBase = srv.URL

	info, err := c.BenefitBalanceDetail(SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK", SecurityToken: "ST"})
	if err != nil {
		t.Fatal(err)
	}
	if info.DailyLimit != 10000000 || info.DailyUsed != 549 {
		t.Errorf("日维度解析错: limit=%d used=%d", info.DailyLimit, info.DailyUsed)
	}
	if info.MonthlyLimit != 0 || info.MonthlyUsed != 10091038 {
		t.Errorf("月维度解析错: limit=%d used=%d", info.MonthlyLimit, info.MonthlyUsed)
	}
	if info.TotalQuota != 10000000 || info.TotalBalance != 9999451 || info.UsedAmount != 549 {
		t.Errorf("别名字段解析错: quota=%d balance=%d used=%d", info.TotalQuota, info.TotalBalance, info.UsedAmount)
	}

	// 兼容入口仍返回日维度三数（cmd/benefit 之外的旧调用方靠它）。
	total, remain, used, err := c.BenefitBalance(SignCredential{AccessKeyID: "AK", SecretAccessKey: "SK", SecurityToken: "ST"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 10000000 || remain != 9999451 || used != 549 {
		t.Errorf("BenefitBalance 兼容入口错: total=%d remain=%d used=%d", total, remain, used)
	}
}
