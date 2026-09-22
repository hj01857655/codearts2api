// 主题管理：data-theme 始终显式落地并指向四套板之一，不做系统跟随。
export const THEMES: [string, string][] = [
  ["graphite", "石墨"],
  ["midnight", "午夜"],
  ["light", "浅色"],
  ["sand", "暖沙"],
];
const THEME_IDS = THEMES.map(([id]) => id);
const THEME_STORE = "codearts2api_panel_theme";

export function currentTheme(): string {
  const t = localStorage.getItem(THEME_STORE) ?? "";
  return THEME_IDS.includes(t) ? t : "graphite";
}
export function applyTheme(t: string): void {
  if (!THEME_IDS.includes(t)) t = "graphite";
  localStorage.setItem(THEME_STORE, t);
  document.documentElement.setAttribute("data-theme", t);
}
// 启动即同步一次，避免首帧闪默认色。
document.documentElement.setAttribute("data-theme", currentTheme());
