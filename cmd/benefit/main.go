// benefit 工具：限时福利签到 / 状态 / 余额查询，对齐 Python 版 benefit.py。
//
// 子命令：
//   status   查询签到状态（上次签到时间）
//   claim    签到领取当日免费额度（幂等）+ 余额
//   balance  查询免费额度余额
//   models   查询免费模型列表
//   （默认） 签到 + 状态 + 余额
//
// 用法：go run ./cmd/benefit [-auth-dir ./auths] [status|claim|balance|models]
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"codearts2api/internal/auth"
	"codearts2api/internal/upstream"
)

func main() {
	authDir := flag.String("auth-dir", "./auths", "auth dir")
	flag.Parse()
	action := "default"
	if flag.NArg() > 0 {
		action = flag.Arg(0)
	}

	auths, err := auth.LoadDir(*authDir)
	if err != nil {
		log.Fatalf("load auths: %v", err)
	}
	if len(auths) == 0 {
		log.Fatalf("no accounts in %s", *authDir)
	}

	c := upstream.New(60 * time.Second)

	for _, a := range auths {
		cred := upstream.SignCredential{
			AccessKeyID:     a.AccessKeyID,
			SecretAccessKey: a.SecretAccessKey,
			SecurityToken:   a.CloudDragonTok,
		}
		fmt.Printf("== %s (%s)\n", a.UserID, a.UserName)

		switch action {
		case "status":
			ct, err := c.BenefitStatus(cred)
			if err != nil {
				fmt.Printf("  签到状态: 失败: %v\n", err)
			} else if ct == 0 {
				fmt.Println("  从未签到")
			} else {
				fmt.Printf("  最近签到: %s\n", time.UnixMilli(ct).Format("2006-01-02 15:04:05"))
			}

		case "claim":
			claimLine(c, cred)
			showBalance(c, cred)

		case "balance":
			showBalance(c, cred)

		case "models":
			infos, err := c.FetchModels(a)
			if err != nil {
				fmt.Printf("  模型查询失败: %v\n", err)
				continue
			}
			for _, m := range infos {
				tag := "builtin"
				if m.Benefit {
					tag = "benefit"
				}
				fmt.Printf("  [%-7s] %s (ctx=%d max=%d)\n", tag, m.ID, m.ContextWindow, m.MaxTokens)
			}

		default:
			// 签到 + 状态 + 余额
			claimLine(c, cred)
			showBalance(c, cred)
		}
	}
}

// claimLine 签到并报出结论。
//
// 签到按天发放，已签过时上游同样返回 error_code=0000 且 create_time 不变，
// 所以「签到成功」这种写法会把「今天已签过」当成「刚签到」。拿调用前后的
// create_time 对比才能说清，且基线缺失时不得断言成功。
func claimLine(c *upstream.Client, cred upstream.SignCredential) {
	before, err := c.BenefitStatus(cred)
	baselineOK := err == nil
	if !baselineOK {
		fmt.Printf("  签到前状态查询失败: %v\n", err)
	} else if upstream.ClaimedToday(before, time.Now()) {
		fmt.Printf("  今天已签到（%s），本次不重复领取\n",
			time.UnixMilli(before).Format("2006-01-02 15:04:05"))
		return
	}
	claim, err := c.ClaimBenefit(cred)
	if err != nil {
		fmt.Printf("  签到失败: %v\n", err)
		return
	}
	out := upstream.SigninOutcomeOf(before, baselineOK, claim, time.Now())
	switch {
	case !out.Counted:
		fmt.Println("  签到调用已发出（上游未返回 create_time，无法确认本次是否新领）")
	case out.NewlyClaimed:
		fmt.Printf("  签到成功: 本次已领到（%s）\n", time.UnixMilli(claim.CreateTime).Format("2006-01-02 15:04:05"))
	case out.AlreadyToday:
		fmt.Printf("  未重复发放: 今天已签过，create_time 仍为 %s\n",
			time.UnixMilli(claim.CreateTime).Format("2006-01-02 15:04:05"))
	default:
		fmt.Println("  签到调用已发出（未取得签到前状态，无法确认本次是否新领）")
	}
}

func showBalance(c *upstream.Client, cred upstream.SignCredential) {
	info, err := c.BenefitBalanceDetail(cred)
	if err != nil {
		fmt.Printf("  余额查询失败: %v\n", err)
		return
	}
	pct := 0.0
	if info.TotalQuota > 0 {
		pct = float64(info.TotalBalance) / float64(info.TotalQuota) * 100
	}
	// 上游给的是日/月两个维度：只报日额度会让人把「本用 1000 万」读成总共用了 549。
	fmt.Printf("  今日额度: %d / %d（已用 %d，剩余 %.1f%%）\n",
		info.TotalBalance, info.TotalQuota, info.UsedAmount, pct)
	if info.MonthlyLimit > 0 {
		fmt.Printf("  本月已用: %d / %d\n", info.MonthlyUsed, info.MonthlyLimit)
	} else {
		fmt.Printf("  本月已用: %d（未设月度上限）\n", info.MonthlyUsed)
	}
}
