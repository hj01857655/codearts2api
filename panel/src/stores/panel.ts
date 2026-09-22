import { defineStore } from "pinia";
import {
  api, getKey, setKey, clearKey, ApiError,
  type Account, type BenefitResult, type ServerConfig, type ActionResponse,
} from "../api";

export interface Overview {
  accounts: Account[];
  models: { id: string }[];
  version?: string;
  region?: string;
  schedule?: Record<string, unknown>;
  [k: string]: unknown;
}

interface Toast { id: number; msg: string; ok: boolean }

let toastSeq = 0;

export const usePanelStore = defineStore("panel", {
  state: () => ({
    authed: false,
    loginErr: "",
    overview: null as Overview | null,
    accounts: [] as Overview["accounts"],
    benefit: null as { results: BenefitResult[] } | null,
    loadingOverview: false,
    refreshing: false,
    lastRefreshed: "",
    toasts: [] as Toast[],
    // 操作记录
    log: [] as { t: Date; ch: string; text: string }[],
  }),
  getters: {
    healthy: (s): number => s.accounts.filter((a: Account) => !a.disabled && !a.cooling).length,
    cooling: (s): number => s.accounts.filter((a: Account) => a.cooling && !a.disabled).length,
    disabled: (s): number => s.accounts.filter((a: Account) => a.disabled).length,
    poolState(): string {
      if (!this.accounts.length) return "empty";
      if (!this.healthy) return this.disabled === this.accounts.length ? "bad" : "warn";
      return "ok";
    },
  },
  actions: {
    toast(msg: string, ok = true) {
      const id = ++toastSeq;
      this.toasts.push({ id, msg, ok });
      window.setTimeout(() => {
        this.toasts = this.toasts.filter((t: Toast) => t.id !== id);
      }, 3600);
    },
    logLine(ch: string, text: string) {
      this.log.push({ t: new Date(), ch, text });
      if (this.log.length > 500) this.log.shift();
    },
    async fetchConfig(): Promise<ServerConfig> {
      return api<ServerConfig>("/admin/api/config");
    },
    async saveConfig(body: ServerConfig): Promise<ActionResponse> {
      return api<ActionResponse>("/admin/api/config", { method: "PUT", body: JSON.stringify(body) });
    },
    async login(key: string) {
      if (!key.trim()) { this.loginErr = "请输入 API Key"; return; }
      setKey(key.trim());
      try {
        await this.loadOverview();
        this.authed = true;
        this.loginErr = "";
      } catch (e: any) {
        clearKey();
        this.loginErr = (e instanceof ApiError && e.status === 401 ? "需要 API Key：" : "") + e.message;
      }
    },
    logout() {
      clearKey();
      this.benefit = null;
      this.accounts = [];
      this.logLine("op", "退出登录");
      this.authed = false;
      this.loginErr = "已退出登录，请输入 API Key 重新进入。";
    },
    unauthorized() {
      clearKey();
      this.authed = false;
      this.loginErr = "密钥不正确，请重试。";
    },
    async loadOverview() {
      if (this.refreshing) return this.overview;
      this.refreshing = true;
      try {
        const data = await api<Overview>("/admin/api/overview");
        this.overview = data;
        this.accounts = data.accounts || [];
        this.lastRefreshed = new Date().toLocaleTimeString();
        return data;
      } finally { this.refreshing = false; }
    },
    async loadBenefit(silent = false) {
      try {
        const data = await api<{ results: BenefitResult[] }>("/admin/api/benefit/status");
        this.benefit = data;
        if (!silent) this.toast("额度已刷新");
        this.logLine("op", "福利额度查询 · " + (data.results || []).length + " 个账号");
      } catch (e: any) {
        this.toast(e.message, false);
        this.logLine("err", "福利额度查询 · " + e.message);
      }
    },
    // 清空操作记录：状态变更集中在 store，组件不再直接赋值。
    clearLog() {
      this.log = [];
    },
    // 用本轮领取结果刷新福利表（比再拉一次 benefit/status 少一次上游往返）。
    applyBenefitResults(results: BenefitResult[]) {
      this.benefit = { results };
    },
    async runAction(path: string, body: unknown, label: string): Promise<ActionResponse | null> {
      try {
        const data = await api<ActionResponse>(path, { method: "POST", body: JSON.stringify(body || {}) });
        const msg = data.message || (data.ok ? "完成" : "完成（有失败）");
        this.toast(msg, !!data.ok);
        this.logLine(data.ok ? "op" : "err", label + " · " + msg);
        await this.loadOverview();
        return data;
      } catch (e: any) {
        this.toast(e.message, false);
        this.logLine("err", label + " · " + e.message);
        return null;
      }
    },
  },
});