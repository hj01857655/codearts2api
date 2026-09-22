package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 签到按天发放：「今天是否已签到」必须按自然日判定，不能只看 create_time 非 0。
// 否则昨天签过的账号会被判成「今天已签到」而跳过签到。
func TestClaimedTodayUsesCalendarDay(t *testing.T) {
	now := time.Date(2026, 9, 22, 15, 0, 0, 0, time.Local)
	cases := []struct {
		name string
		ct   int64
		want bool
	}{
		{"从未签到（0）", 0, false},
		{"今天早些时候签过", now.Add(-3 * time.Hour).UnixMilli(), true},
		{"今天刚过零点签的", time.Date(2026, 9, 22, 0, 5, 0, 0, time.Local).UnixMilli(), true},
		{"昨天签的", now.Add(-24 * time.Hour).UnixMilli(), false},
		{"上月同一天号", time.Date(2026, 8, 22, 15, 0, 0, 0, time.Local).UnixMilli(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClaimedToday(tc.ct, now); got != tc.want {
				t.Errorf("ClaimedToday(%d) = %t, want %t", tc.ct, got, tc.want)
			}
		})
	}
}

// 上游已签到过时同样返回 error_code=0000 且 create_time 保持不动。
// 实测 2026-09-21 抓包：POST 发生在 2026-09-22 00:43Z，返回的 create_time
// 仍是 2026-09-17 13:51:40Z（早 4.45 天）。因此「本次是否新签」只能靠
// 调用前后的 create_time 比较。
func TestSigninOutcomeOfWithTrustedBaseline(t *testing.T) {
	now := time.Date(2026, 9, 22, 15, 0, 0, 0, time.Local)
	earlier := now.Add(-2 * time.Hour).UnixMilli()
	stale := now.Add(-5 * 24 * time.Hour).UnixMilli() // 几天前，非今天

	cases := []struct {
		name      string
		before    int64
		after     ClaimResult
		wantNew   bool
		wantToday bool
		wantCount bool
	}{
		{
			name:      "本次真的新签（时间被刷新到今天）",
			before:    stale,
			after:     ClaimResult{CreateTime: now.UnixMilli()},
			wantNew:   true,
			wantToday: true,
			wantCount: true,
		},
		{
			name:      "今天已签过，本次是空转（时间未变）",
			before:    earlier,
			after:     ClaimResult{CreateTime: earlier},
			wantNew:   false,
			wantToday: true,
			wantCount: true,
		},
		{
			name:      "从未签到，本次新签",
			before:    0,
			after:     ClaimResult{CreateTime: now.UnixMilli()},
			wantNew:   true,
			wantToday: true,
			wantCount: true,
		},
		{
			name:      "上游没回时间 = 无法判断",
			before:    earlier,
			after:     ClaimResult{CreateTime: 0},
			wantNew:   false,
			wantToday: false,
			wantCount: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SigninOutcomeOf(tc.before, true, tc.after, now)
			if got.Counted != tc.wantCount {
				t.Errorf("Counted = %t, want %t", got.Counted, tc.wantCount)
			}
			if got.NewlyClaimed != tc.wantNew {
				t.Errorf("NewlyClaimed = %t, want %t", got.NewlyClaimed, tc.wantNew)
			}
			if got.Today != tc.wantToday {
				t.Errorf("Today = %t, want %t", got.Today, tc.wantToday)
			}
		})
	}
}

// 基线缺失时绝不断言「本次新签」：这正是「每次点都成功」的漏洞来源。
// 此前实现把基线取失败降级为 before=0，而 0 恰好是「从未签到」的合法取值，
// 于是 create_time != before 恒成立，今天早已签过的账号会被报成签到成功。
func TestSigninOutcomeOfWithoutBaselineNeverClaimsNew(t *testing.T) {
	now := time.Date(2026, 9, 22, 15, 0, 0, 0, time.Local)

	// 今天早已签过（create_time 就是今天），但基线查询失败。
	got := SigninOutcomeOf(0, false, ClaimResult{CreateTime: now.Add(-time.Hour).UnixMilli()}, now)
	if got.NewlyClaimed {
		t.Error("基线缺失时不得断言本次新签，否则每次点击都会报成功")
	}
	if !got.Today {
		t.Error("create_time 落在今天，Today 应为 true（结论可信，但不等于本次领到）")
	}
	if !got.Counted {
		t.Error("上游返回了时间，结论应标记为可信")
	}

	// 时间戳在几天前，同样不得断言新签。
	got = SigninOutcomeOf(0, false, ClaimResult{CreateTime: now.Add(-5 * 24 * time.Hour).UnixMilli()}, now)
	if got.NewlyClaimed {
		t.Error("基线缺失时不得断言本次新签")
	}
	if got.Today {
		t.Error("时间戳不在今天，Today 应为 false")
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
