// 配置端点：面板「配置」页的读写接口。
//
// 只暴露白名单字段（巡检保活、单账号并发、福利自动领取、更新代理）。
// 保存不回写 config.json——那份文件是用户手工维护的（可能带 // 注释，JSON
// 解析后无法原样写回），改动持久化到 data/settings.json 覆盖层，重启时在
// config.json 之上合并。福利自动领取是 upstream.Client 上的开关，保存后
// 立即生效，无需重启；其余字段重启后由启动时合并覆盖层生效。
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// settingsOverlay data/settings.json 的结构；只含白名单字段，保存即整份覆盖。
type settingsOverlay struct {
	Watch struct {
		Enabled           *bool `json:"enabled,omitempty"`
		PollMinutes       *int  `json:"poll_minutes,omitempty"`
		RefreshSkewM      *int  `json:"refresh_skew_minutes,omitempty"`
		KeepaliveInterval *int  `json:"keepalive_interval_minutes,omitempty"`
	} `json:"watch,omitempty"`

	KeepaliveWindow  *string `json:"keepalive_window,omitempty"`
	MaxConcurrent    *int    `json:"max_concurrent,omitempty"`
	BenefitAutoClaim *bool   `json:"benefit_auto_claim,omitempty"`
	UpdateProxy      *string `json:"update_proxy,omitempty"`
}

// settingsPathFor 覆盖层文件路径：与 state 文件同目录（默认 data/settings.json）。
func settingsPathFor(cfg Config) string {
	if cfg.ConvStateFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.ConvStateFile), "settings.json")
}

// settingsStore 配置页的运行时状态：初始化时读已有覆盖层，PUT 后内存与文件同步更新。
type settingsStore struct {
	mu      sync.Mutex
	overlay settingsOverlay
	path    string // data/settings.json；空表示不持久化（测试场景）
}

// newSettingsStore 初始化：优先读已有覆盖层，没有则用空覆盖（首次保存前属正常）。
func newSettingsStore(path string) *settingsStore {
	s := &settingsStore{path: path}
	if path == "" {
		return s
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(raw, &s.overlay)
	return s
}

// boolOf 从 WatchInfo 快照取布尔值，缺失时返回 false。
func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}

// intOf 从 WatchInfo 快照取整数，缺失或类型不符时返回默认值。
func intOf(v any, def int) int {
	if n, ok := v.(int); ok {
		return n
	}
	return def
}

// stringOf 从 WatchInfo 快照取字符串，缺失时返回空。
func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

// mergedWatch 合并后的巡检参数：覆盖层优先，回退启动快照（WatchInfo）。
func (h *Handler) mergedWatch() (enabled bool, poll, skew, keepalive int) {
	enabled = boolOf(h.cfg.WatchInfo["enabled"])
	poll = intOf(h.cfg.WatchInfo["poll_minutes"], 30)
	skew = intOf(h.cfg.WatchInfo["refresh_skew_minutes"], 30)
	keepalive = intOf(h.cfg.WatchInfo["keepalive_interval"], 15)
	if h.settings == nil {
		return
	}
	h.settings.mu.Lock()
	defer h.settings.mu.Unlock()
	o := h.settings.overlay
	if o.Watch.Enabled != nil {
		enabled = *o.Watch.Enabled
	}
	if o.Watch.PollMinutes != nil {
		poll = *o.Watch.PollMinutes
	}
	if o.Watch.RefreshSkewM != nil {
		skew = *o.Watch.RefreshSkewM
	}
	if o.Watch.KeepaliveInterval != nil {
		keepalive = *o.Watch.KeepaliveInterval
	}
	return
}

// mergedScalar 合并后的标量配置：覆盖层优先，回退启动快照。
func (h *Handler) mergedScalar() (maxConcurrent int, keepaliveWindow string, benefitClaim bool, updateProxy string) {
	maxConcurrent = intOf(h.cfg.WatchInfo["max_concurrent"], 1)
	keepaliveWindow = stringOf(h.cfg.WatchInfo["keepalive_window"])
	if v, ok := h.cfg.WatchInfo["benefit_auto_claim"].(bool); ok {
		benefitClaim = v
	}
	updateProxy = h.cfg.UpdateProxy
	if h.settings == nil {
		return
	}
	h.settings.mu.Lock()
	defer h.settings.mu.Unlock()
	o := h.settings.overlay
	if o.MaxConcurrent != nil {
		maxConcurrent = *o.MaxConcurrent
	}
	if o.KeepaliveWindow != nil {
		keepaliveWindow = *o.KeepaliveWindow
	}
	if o.BenefitAutoClaim != nil {
		benefitClaim = *o.BenefitAutoClaim
	}
	if o.UpdateProxy != nil {
		updateProxy = *o.UpdateProxy
	}
	return
}

// adminGetConfig GET /admin/api/config：返回合并后的当前生效值。
func (h *Handler) adminGetConfig(w http.ResponseWriter, r *http.Request) {
	enabled, poll, skew, keepalive := h.mergedWatch()
	maxConcurrent, keepaliveWindow, benefitClaim, updateProxy := h.mergedScalar()
	writeJSON(w, http.StatusOK, map[string]any{
		"watch": map[string]any{
			"enabled":                    enabled,
			"poll_minutes":               poll,
			"refresh_skew_minutes":       skew,
			"keepalive_interval_minutes": keepalive,
		},
		"max_concurrent":     maxConcurrent,
		"keepalive_window":   keepaliveWindow,
		"benefit_auto_claim": benefitClaim,
		"update_proxy":       updateProxy,
	})
}

// configPayload PUT 请求体：表单整份提交，nil 表示该字段未提交（保持不变）。
type configPayload struct {
	Watch struct {
		Enabled           *bool `json:"enabled"`
		PollMinutes       *int  `json:"poll_minutes"`
		RefreshSkewM      *int  `json:"refresh_skew_minutes"`
		KeepaliveInterval *int  `json:"keepalive_interval_minutes"`
	} `json:"watch"`
	MaxConcurrent    *int    `json:"max_concurrent"`
	KeepaliveWindow  *string `json:"keepalive_window"`
	BenefitAutoClaim *bool   `json:"benefit_auto_claim"`
	UpdateProxy      *string `json:"update_proxy"`
}

// validate 校验白名单字段的取值，返回首个错误。
func (p *configPayload) validate() error {
	if p.Watch.PollMinutes != nil && *p.Watch.PollMinutes <= 0 {
		return fmt.Errorf("轮询间隔必须大于 0 分钟")
	}
	if p.Watch.RefreshSkewM != nil && *p.Watch.RefreshSkewM <= 0 {
		return fmt.Errorf("提前刷新窗口必须大于 0 分钟")
	}
	if p.Watch.KeepaliveInterval != nil && *p.Watch.KeepaliveInterval <= 0 {
		return fmt.Errorf("保活间隔必须大于 0 分钟")
	}
	if p.MaxConcurrent != nil && *p.MaxConcurrent <= 0 {
		return fmt.Errorf("单账号最大并发必须大于 0")
	}
	if p.KeepaliveWindow != nil {
		if _, err := time.ParseDuration(*p.KeepaliveWindow); err != nil {
			return fmt.Errorf("保活窗口格式无效（示例 10m / 1h30m）: %v", err)
		}
	}
	if p.UpdateProxy != nil && *p.UpdateProxy != "" {
		u, err := url.Parse(*p.UpdateProxy)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
			return fmt.Errorf("更新代理仅支持 http/https/socks5 地址")
		}
	}
	return nil
}

// apply 把请求体写入覆盖层并原子落盘，同步 WatchInfo 快照供 GET/overview 读取；
// 福利自动领取额外调 upstream 的开关即时生效。
func (h *Handler) apply(p *configPayload) {
	h.settings.mu.Lock()
	o := &h.settings.overlay
	if p.Watch.Enabled != nil {
		o.Watch.Enabled = p.Watch.Enabled
	}
	if p.Watch.PollMinutes != nil {
		o.Watch.PollMinutes = p.Watch.PollMinutes
	}
	if p.Watch.RefreshSkewM != nil {
		o.Watch.RefreshSkewM = p.Watch.RefreshSkewM
	}
	if p.Watch.KeepaliveInterval != nil {
		o.Watch.KeepaliveInterval = p.Watch.KeepaliveInterval
	}
	if p.MaxConcurrent != nil {
		o.MaxConcurrent = p.MaxConcurrent
	}
	if p.KeepaliveWindow != nil {
		o.KeepaliveWindow = p.KeepaliveWindow
	}
	if p.BenefitAutoClaim != nil {
		o.BenefitAutoClaim = p.BenefitAutoClaim
	}
	if p.UpdateProxy != nil {
		o.UpdateProxy = p.UpdateProxy
	}
	raw, _ := json.MarshalIndent(o, "", "  ")
	path := h.settings.path
	h.settings.mu.Unlock()

	if p.BenefitAutoClaim != nil {
		h.cfg.Upstream.SetBenefitAutoClaim(*p.BenefitAutoClaim) // 热生效，无需重启
	}

	// 原子写：同目录临时文件 + rename，写失败保留旧覆盖层。
	if path != "" {
		dir := filepath.Dir(path)
		if mkErr := os.MkdirAll(dir, 0o700); mkErr == nil {
			tmp, tmpErr := os.CreateTemp(dir, ".settings-*")
			if tmpErr == nil {
				_, wErr := tmp.Write(raw)
				tmp.Close()
				if wErr == nil {
					_ = os.Rename(tmp.Name(), path)
				}
			}
		}
	}

	// 同步 WatchInfo 快照（map 引用，overview 与配置页 GET 都读这里）。
	if o.Watch.Enabled != nil {
		h.cfg.WatchInfo["enabled"] = *o.Watch.Enabled
	}
	if o.Watch.PollMinutes != nil {
		h.cfg.WatchInfo["poll_minutes"] = *o.Watch.PollMinutes
	}
	if o.Watch.RefreshSkewM != nil {
		h.cfg.WatchInfo["refresh_skew_minutes"] = *o.Watch.RefreshSkewM
	}
	if o.Watch.KeepaliveInterval != nil {
		h.cfg.WatchInfo["keepalive_interval"] = *o.Watch.KeepaliveInterval
	}
	if o.MaxConcurrent != nil {
		h.cfg.WatchInfo["max_concurrent"] = *o.MaxConcurrent
	}
	if o.KeepaliveWindow != nil {
		h.cfg.WatchInfo["keepalive_window"] = *o.KeepaliveWindow
	}
	if o.BenefitAutoClaim != nil {
		h.cfg.WatchInfo["benefit_auto_claim"] = *o.BenefitAutoClaim
	}
	if o.UpdateProxy != nil {
		h.cfg.UpdateProxy = *o.UpdateProxy
	}
}

// registerConfigRoutes 注册 GET/PUT /admin/api/config（与其它 admin 路由同一处注册模式）。
func (h *Handler) registerConfigRoutes() {
	h.mux.HandleFunc("GET /admin/api/config", h.withAuth(h.adminGetConfig))
	h.mux.HandleFunc("PUT /admin/api/config", h.withAuth(h.adminPutConfig))
}

// adminPutConfig PUT /admin/api/config：校验 → 写覆盖层 → 内存同步 → 返回。
func (h *Handler) adminPutConfig(w http.ResponseWriter, r *http.Request) {
	var p configPayload
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "请求体无效: " + err.Error()}})
		return
	}
	if err := p.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	h.apply(&p)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "已保存；巡检与并发参数重启服务后生效，福利自动领取即时生效",
	})
}
