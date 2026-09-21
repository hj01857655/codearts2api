package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"codearts2api/internal/auth"
	"codearts2api/internal/upstream"
)

// adminOverview 面板总览。
func (h *Handler) adminOverview(w http.ResponseWriter, r *http.Request) {
	total, healthy, disabled, cooling, credits := h.cfg.Pool.Stats()
	models := h.modelList()
	ids := make([]map[string]any, 0, len(models))
	for _, m := range models {
		ids = append(ids, map[string]any{"id": m["id"]})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "codearts2api",
		"region":  "cn",
		"version": h.cfg.Version,
		"stats": map[string]any{
			"total":    total,
			"healthy":  healthy,
			"disabled": disabled,
			"cooling":  cooling,
			"credits":  credits,
		},
		"accounts": h.cfg.Pool.List(),
		"models":   ids,
		"schedule": map[string]any{
			"watch": h.cfg.WatchInfo,
		},
	})
}

type uidBody struct {
	UID     string `json:"uid"`
	Account string `json:"account"`
	Reason  string `json:"reason"`
}

func readUIDBody(r *http.Request) (uidBody, error) {
	var b uidBody
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return b, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return b, nil
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	if b.UID == "" {
		b.UID = b.Account
	}
	return b, nil
}

type actionResult struct {
	UID     string `json:"uid"`
	OK      bool   `json:"ok"`
	Credits int64  `json:"credits,omitempty"`
	Message string `json:"message,omitempty"`
}

// adminCredits CodeArts 无积分；复用为 Validate/刷新状态。
func (h *Handler) adminCredits(w http.ResponseWriter, r *http.Request) {
	body, err := readUIDBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "bad json: " + err.Error()})
		return
	}
	targets := h.pickTargets(body.UID)
	results := make([]actionResult, 0, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for _, uid := range targets {
		uid := uid
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			acct := h.cfg.Pool.Get(uid)
			res := actionResult{UID: uid}
			if acct == nil {
				res.Message = "no account"
			} else if ok, err := h.cfg.Pool.Validate(acct); err != nil {
				res.Message = err.Error()
			} else if !ok {
				res.Message = "token invalid"
			} else {
				res.OK = true
				res.Message = "ok"
			}
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}()
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": allOK(results), "message": summaryMsg("刷新状态", results), "results": results,
	})
}

// benefitResult 单账号福利操作结果。
//
// 额度字段按上游的两个维度分组：daily_* 是当日、monthly_* 是本月。
// 上游的 total_quota/total_balance/used_amount 只是日维度的别名（实测
// total_quota == daily_token_limit），旧前端读的是这三个；面板显示必须带
// 时间维度，否则「本用 1009 万」会被读成「总共用了 549」。
type benefitResult struct {
	UID        string `json:"uid"`
	OK         bool   `json:"ok"`
	Message    string `json:"message,omitempty"`
	CreateTime int64  `json:"create_time,omitempty"` // 上次签到时间（毫秒）
	Total      int64  `json:"total,omitempty"`       // 当日额度（上游 total_quota 别名）
	Remain     int64  `json:"remain,omitempty"`      // 当日剩余
	Used       int64  `json:"used,omitempty"`        // 当日已用
	// DailyLimit/DailyUsed/MonthlyLimit/MonthlyUsed 是上游的原始维度字段。
	// MonthlyLimit=0 表示未设月度上限（不是「额度用尽」），前端按此区分。
	DailyLimit   int64 `json:"daily_limit,omitempty"`
	DailyUsed    int64 `json:"daily_used,omitempty"`
	MonthlyLimit int64 `json:"monthly_limit,omitempty"`
	MonthlyUsed  int64 `json:"monthly_used,omitempty"`
	UpdateTime   int64 `json:"update_time,omitempty"`
	// HasBalance 标记上述额度字段是上游真实返回的。没有它就无法区分
	// 「额度就是 0」与「余额没查到」，面板会把后者当 0 显示并计入合计。
	HasBalance bool `json:"has_balance"`
}

// fillBalance 把一次余额查询的结果写进 benefitResult。
func fillBalance(res *benefitResult, info upstream.BenefitBalanceInfo) {
	res.Total, res.Remain, res.Used = info.TotalQuota, info.TotalBalance, info.UsedAmount
	res.DailyLimit, res.DailyUsed = info.DailyLimit, info.DailyUsed
	res.MonthlyLimit, res.MonthlyUsed = info.MonthlyLimit, info.MonthlyUsed
	res.UpdateTime = info.UpdateTime
	res.HasBalance = true
}

// adminCheckin 真正领取限时福利（POST /api/v1/benefit/claim）并返回余额。
func (h *Handler) adminCheckin(w http.ResponseWriter, r *http.Request) {
	body, err := readUIDBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "bad json: " + err.Error()})
		return
	}
	targets := h.pickTargets(body.UID)
	results := make([]benefitResult, 0, len(targets))
	for _, uid := range targets {
		acct := h.cfg.Pool.Get(uid)
		res := benefitResult{UID: uid}
		if acct == nil || acct.Auth == nil {
			res.Message = "no account"
			results = append(results, res)
			continue
		}
		token, ak, sk := acct.Auth.Credentials()
		cred := upstream.SignCredential{AccessKeyID: ak, SecretAccessKey: sk, SecurityToken: token}
		// 先领取（幂等）
		if err := h.cfg.Upstream.ClaimBenefit(cred); err != nil {
			res.Message = "claim: " + err.Error()
			results = append(results, res)
			continue
		}
		// 再查余额
		if info, berr := h.cfg.Upstream.BenefitBalanceDetail(cred); berr != nil {
			res.OK = true
			res.Message = "claimed, balance: " + berr.Error()
		} else {
			res.OK = true
			res.Message = "claimed"
			fillBalance(&res, info)
		}
		results = append(results, res)
	}
	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": okCount == len(results), "message": summaryMsg("签到", toActionResults(results)), "results": results,
	})
}

// adminBenefitStatus 查询全部账号福利签到状态 + 余额（不领取）。
func (h *Handler) adminBenefitStatus(w http.ResponseWriter, r *http.Request) {
	targets := h.pickTargets("")
	results := make([]benefitResult, 0, len(targets))
	for _, uid := range targets {
		acct := h.cfg.Pool.Get(uid)
		res := benefitResult{UID: uid}
		if acct == nil || acct.Auth == nil {
			res.Message = "no account"
			results = append(results, res)
			continue
		}
		token, ak, sk := acct.Auth.Credentials()
		cred := upstream.SignCredential{AccessKeyID: ak, SecretAccessKey: sk, SecurityToken: token}
		// 签到状态
		ct, serr := h.cfg.Upstream.BenefitStatus(cred)
		if serr != nil {
			res.Message = "status: " + serr.Error()
		} else {
			res.OK = true
			res.CreateTime = ct
		}
		// 余额
		if info, berr := h.cfg.Upstream.BenefitBalanceDetail(cred); berr == nil {
			fillBalance(&res, info)
		} else {
			// 余额失败原先完全静默：面板把 0 当成真实额度显示，还折进合计行。
			res.Message = appendMsg(res.Message, "balance: "+berr.Error())
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// appendMsg 拼接同一账号的多条失败原因：状态查询与余额查询可能各自失败，
// 只留最后一条会丢掉先发生的那个。
func appendMsg(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "; " + add
}

// toActionResults 把 benefitResult 转成 actionResult 供 summaryMsg 复用。
func toActionResults(brs []benefitResult) []actionResult {
	out := make([]actionResult, 0, len(brs))
	for _, br := range brs {
		out = append(out, actionResult{UID: br.UID, OK: br.OK, Message: br.Message})
	}
	return out
}

// adminKeepalive 刷新即将过期的 token。
func (h *Handler) adminKeepalive(w http.ResponseWriter, r *http.Request) {
	body, err := readUIDBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "bad json: " + err.Error()})
		return
	}
	targets := h.pickTargets(body.UID)
	results := make([]actionResult, 0, len(targets))
	for _, uid := range targets {
		acct := h.cfg.Pool.Get(uid)
		res := actionResult{UID: uid}
		if acct == nil {
			res.Message = "no account"
			results = append(results, res)
			continue
		}
		// Validate 内部会在临近过期时自动 Refresh
		ok, verr := h.cfg.Pool.Validate(acct)
		if verr != nil {
			res.Message = verr.Error()
		} else if !ok {
			res.Message = "token invalid"
		} else {
			res.OK = true
			res.Message = "refreshed/validated"
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": allOK(results), "message": summaryMsg("保活", results), "results": results,
	})
}

func (h *Handler) adminReload(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AuthDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "auth_dir 未配置"})
		return
	}
	auths, err := auth.LoadDir(h.cfg.AuthDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	h.cfg.Pool.SyncToDir(auths)
	total, healthy, _, _, _ := h.cfg.Pool.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "message": "已重载 auths", "loaded": len(auths), "total": total, "healthy": healthy,
	})
}

func (h *Handler) adminEnable(w http.ResponseWriter, r *http.Request) {
	body, err := readUIDBody(r)
	if err != nil || body.UID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "uid required"})
		return
	}
	if !h.cfg.Pool.Enable(body.UID) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "message": "account not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "已启用 " + body.UID})
}

func (h *Handler) adminDisable(w http.ResponseWriter, r *http.Request) {
	body, err := readUIDBody(r)
	if err != nil || body.UID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "uid required"})
		return
	}
	reason := body.Reason
	if reason == "" {
		reason = "manual disable"
	}
	h.cfg.Pool.Disable(body.UID, reason)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "已禁用 " + body.UID})
}

func (h *Handler) adminClearCooldown(w http.ResponseWriter, r *http.Request) {
	body, err := readUIDBody(r)
	if err != nil || body.UID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": "uid required"})
		return
	}
	if !h.cfg.Pool.ClearCooldown(body.UID) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "message": "account not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "已清冷却 " + body.UID})
}

func (h *Handler) pickTargets(uid string) []string {
	if uid != "" {
		if h.cfg.Pool.Get(uid) == nil {
			return nil
		}
		return []string{uid}
	}
	out := make([]string, 0)
	for _, st := range h.cfg.Pool.List() {
		if disabled, _ := st["disabled"].(bool); disabled {
			continue
		}
		if u, ok := st["uid"].(string); ok && u != "" {
			out = append(out, u)
		}
	}
	return out
}

func allOK(results []actionResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}

func summaryMsg(action string, results []actionResult) string {
	if len(results) == 0 {
		return action + ": 无目标账号"
	}
	ok, fail := 0, 0
	for _, r := range results {
		if r.OK {
			ok++
		} else {
			fail++
		}
	}
	if fail == 0 {
		return action + "完成: " + itoa(ok) + " 成功"
	}
	return action + "完成: " + itoa(ok) + " 成功 / " + itoa(fail) + " 失败"
}

// adminModelsRefresh 强制重发现模型目录（POST /admin/api/models/refresh）。
//
// 面板「重新获取」原先只是重读一次 overview，而 /v1/models 有 1h 内存缓存 +
// 账号目录各 1h TTL，用户刚领完福利再点也看不到新模型——按钮名不副实。
// 这里绕过两层缓存直打上游，并回写磁盘缓存，使结果对 /v1/models 立即生效。
func (h *Handler) adminModelsRefresh(w http.ResponseWriter, r *http.Request) {
	infos := h.refreshDynamicModels()
	if len(infos) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "message": "模型发现失败，已沿用旧列表（可能是上游不可达或账号不可用）",
		})
		return
	}
	ids := make([]map[string]any, 0, len(infos))
	for _, m := range infos {
		ids = append(ids, map[string]any{"id": m.ID})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "message": "已重新发现 " + itoa(len(infos)) + " 个模型", "models": ids,
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
