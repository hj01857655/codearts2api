// API 基址：面板可挂在根路径或 /panel /admin 前缀下，API 路径按当前挂载点拼。
// 挂载点必须从 BASE 去掉：API 路径本身以 /admin/api 开头（见 handler 路由），
// 只刨 /panel 时在 /admin 下会拼成 /admin/admin/api/... 而 404。
export const BASE = (() => {
  let p = location.pathname.replace(/\/+$/, "") || "";
  for (const mount of ["/panel", "/admin"]) {
    if (p.endsWith(mount)) { p = p.slice(0, -mount.length); break; }
  }
  return p === "/" ? "" : p;
})();

export interface Account {
  uid?: string; name?: string; nickname?: string;
  disabled?: boolean; cooling?: boolean; until?: string;
  default_model?: string; token_remaining?: string; expires_at?: string;
  active_concurrent?: number; max_concurrent?: number;
  err_count?: number; reason?: string; last_error?: string;
}

export interface BenefitResult {
  uid: string; ok: boolean; has_balance: boolean;
  total?: number; used?: number; remain?: number; monthly_used?: number;
  create_time?: number; message?: string;
}

export interface UpdateStatus {
  current: string; latest?: string; has_update: boolean;
  can_rollback: boolean; supported: boolean; reason?: string;
  notes?: string; html_url?: string;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) { super(message); this.status = status; }
}

const KEY_STORE = "codearts2api_panel_api_key";
export const getKey = () => localStorage.getItem(KEY_STORE) || "";
export const setKey = (k: string) => localStorage.setItem(KEY_STORE, k);
export const clearKey = () => localStorage.removeItem(KEY_STORE);

export async function api<T = any>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json", ...((opts.headers as Record<string, string>) || {}) };
  const key = getKey();
  if (key) headers["Authorization"] = "Bearer " + key;
  const res = await fetch(BASE + path, { ...opts, headers });
  const text = await res.text();
  let data: any = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) {
    const err = data && data.error;
    const msg = (err && (err.message || err.code)) || (data && (data.message || data.error)) || text || res.statusText;
    throw new ApiError(typeof msg === "string" ? msg : JSON.stringify(msg), res.status);
  }
  return data as T;
}
