// 运行时注入量：后端 panel.go 在 HTML 里做字符串替换。
// dev 模式下没有后端，走这里的兜底值。
export interface RuntimeConfig {
  serviceName: string;
  serviceTitle: string;
  logo: string;
  accent: string;
}

declare global {
  interface Window {
    __PANEL_CONFIG__?: Partial<RuntimeConfig>;
  }
}

export const config: RuntimeConfig = {
  serviceName: "__SERVICE_NAME__",
  serviceTitle: "__SERVICE_TITLE__",
  logo: "__LOGO__",
  accent: "__ACCENT__",
  ...(typeof window !== "undefined" ? window.__PANEL_CONFIG__ : {}),
};

// dev（vite serve）下占位符不会被后端替换，这里替换成可用的兜底值。
const isPlaceholder = (v: string) => /^__[A-Z_]+__$/.test(v);
if (isPlaceholder(config.serviceName)) config.serviceName = "codearts2api";
if (isPlaceholder(config.serviceTitle)) config.serviceTitle = "CodeArts2API";
if (isPlaceholder(config.logo)) config.logo = "CA";
if (isPlaceholder(config.accent)) config.accent = "#0284c7";
