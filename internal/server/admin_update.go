package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"codearts2api/internal/update"
)

// updateTimeout 一次完整更新的上限：下载可达数十 MB。
const updateTimeout = 15 * time.Minute

// updaterOrErr 返回更新服务；未配置时给出可读原因。
//
// 这里的文案会原样进入面板 toast 与操作记录（四个更新端点都复用它），
// 所以用中文；保留 update_repo 字样，用户才知道该改哪个配置项。
func (h *Handler) updaterOrErr() (*update.Service, error) {
	if h.updater == nil {
		return nil, errors.New("未配置在线更新：请在 config.json 里设置 update_repo（owner/name）")
	}
	return h.updater, nil
}

// detachedUpdateContext 把长耗时更新与请求上下文解耦：客户端点完就走、连接断开
// 都不应中断正在进行的下载与替换，否则会留下半截状态。
func detachedUpdateContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), updateTimeout)
}

// adminUpdateCheck 检测是否有新版本。只读，不获取 updateMu。
func (h *Handler) adminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	up, err := h.updaterOrErr()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false, "message": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	st, cerr := up.Check(ctx)
	if cerr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "message": cerr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"status":     st,
		"message":    updateMessage(st),
	})
}

func updateMessage(st *update.Status) string {
	switch {
	case !st.Supported:
		if st.Reason != "" {
			return st.Reason
		}
		return "当前环境不支持在线更新"
	case st.HasUpdate:
		return "发现新版本 " + st.Latest + "（当前 " + st.Current + "）"
	default:
		return "已是最新版本（" + st.Current + "）"
	}
}

// adminUpdateApply 下载并替换二进制，随后需要重启才生效。
func (h *Handler) adminUpdateApply(w http.ResponseWriter, r *http.Request) {
	up, err := h.updaterOrErr()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	if !h.updateMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "message": "另一个更新操作正在进行"})
		return
	}
	defer h.updateMu.Unlock()

	ctx, cancel := detachedUpdateContext()
	defer cancel()
	st, aerr := up.Apply(ctx)
	if aerr != nil {
		msg := aerr.Error()
		code := http.StatusBadGateway
		switch {
		case errors.Is(aerr, update.ErrNoUpdate):
			msg, code = "已是最新版本（"+up.Current()+"）", http.StatusOK
		case errors.Is(aerr, update.ErrUnsupported):
			msg, code = "当前环境不支持在线更新，请改用镜像或手动替换", http.StatusBadRequest
		}
		if code == http.StatusOK {
			writeJSON(w, code, map[string]any{"ok": true, "need_restart": false, "status": st, "message": msg})
			return
		}
		log.Printf("online update failed: %v", aerr)
		writeJSON(w, code, map[string]any{"ok": false, "message": msg})
		return
	}
	log.Printf("online update applied: %s -> %s", st.Current, st.Latest)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"need_restart": true,
		"status":       st,
		"message":      "已下载并替换 " + st.Latest + "，重启后生效",
	})
}

// adminUpdateRollback 用备份换回上一版。
func (h *Handler) adminUpdateRollback(w http.ResponseWriter, r *http.Request) {
	up, err := h.updaterOrErr()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	if !h.updateMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "message": "另一个更新操作正在进行"})
		return
	}
	defer h.updateMu.Unlock()

	if rerr := up.Rollback(); rerr != nil {
		msg := rerr.Error()
		if errors.Is(rerr, update.ErrNoBackup) {
			msg = "没有可回滚的备份（未执行过在线更新）"
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "message": msg})
		return
	}
	log.Printf("online update rolled back to backup")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"need_restart": true,
		"message":      "已回滚到上一版本，重启后生效",
	})
}

// adminUpdateRestart 退出进程，由 systemd 的 Restart=always 拉起新版本。
//
// 不用 systemctl（那需要 sudo）：只做优雅退出，重启交给 unit。延迟 500ms 是
// 了让本次 HTTP 响应先发出去。
func (h *Handler) adminUpdateRestart(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "正在重启，稍后自动恢复"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Printf("restart requested from panel: exiting for systemd Restart=always")
		os.Exit(0)
	}()
}
