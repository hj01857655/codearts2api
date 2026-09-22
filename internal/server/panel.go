package server

import (
	"embed"
	"net/http"
	"strings"
)

// 面板有两层来源：
//   dist/ —— panel/ 下 Vue3+Vite 工程的构建产物（bun run build），
//                  vite-plugin-singlefile 把 JS/CSS 全部内联进一个 index.html，
//                  因此嵌入整个目录即可，无需静态资源路由。
//   panel.html —— 旧版单文件面板，仅在 dist 缺失（未执行前端构建）时
//                  兜底，保证 clone 后直接 go build 不依赖 Node。
// 两者的 __SERVICE_*__ / __LOGO__ / __ACCENT__ 占位符在 servePanel 里统一注入。
var (
	//go:embed dist
	panelDist embed.FS

	//go:embed panel.html
	panelHTMLRaw string
)

func panelHTML() []byte {
	if b, err := panelDist.ReadFile("dist/index.html"); err == nil && len(b) > 0 {
		return b
	}
	return []byte(panelHTMLRaw)
}

func (h *Handler) servePanel(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/admin" && r.URL.Path != "/panel" && r.URL.Path != "/panel/" {
		http.NotFound(w, r)
		return
	}
	html := string(panelHTML())
	html = strings.ReplaceAll(html, "__SERVICE_NAME__", "codearts2api")
	html = strings.ReplaceAll(html, "__SERVICE_TITLE__", "CodeArts2API")
	html = strings.ReplaceAll(html, "__LOGO__", "CA")
	html = strings.ReplaceAll(html, "__ACCENT__", "#0284c7")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
