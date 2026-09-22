import { defineStore } from "pinia";
import { api, getKey, setKey, clearKey, ApiError, type Account, type BenefitResult } from "../api";

export interface Overview {
  accounts: Account[];
  models: { id: string }[];
  version?: string;
  region?: string;
  schedule?: { watch?: Record<string, unknown> };
  [k: string]: unknown;
}

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
    // 版本徽标（对齐 sub2api VersionBadge 的状态机）
    version: "",
    latestVersion: "",
    hasUpdate: false,
    updateChecked: false,
    updateSupported: true,
    updateReason: "",
    canRollback: false,
    updateChecking: false,
    updating: false,
    needRestart: false,
    updateMsg: "",
    updateFailed: false,
    restarting: false,
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
      window.dispatchEvent(new CustomEvent("panel:toast", { detail: { msg, ok } }));
    },
    logLine(ch: string, text: string) {
      this.log.push({ t: new Date(), ch, text });
      if (this.log.length > 500) this.log.shift();
    },
    async fetchConfig() {
      return api("/admin/api/config");
    },
    async saveConfig(body: any) {
      return api("/admin/api/config", { method: "PUT", body: JSON.stringify(body) });
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
        if (data.version && !this.updateChecked) this.version = data.version;
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
        if (e instanceof ApiError && e.status === 401) this.unauthorized();
      }
    },
    async runAction(path: string, body: unknown, label: string) {
      try {
        const data = await api(path, { method: "POST", body: JSON.stringify(body || {}) });
        const msg = data.message || (data.ok ? "完成" : "完成（有失败）");
        this.toast(msg, !!data.ok);
        this.logLine(data.ok ? "op" : "err", label + " · " + msg);
        await this.loadOverview();
        return data;
      } catch (e: any) {
        this.toast(e.message, false);
        this.logLine("err", label + " · " + e.message);
        if (e instanceof ApiError && e.status === 401) this.unauthorized();
        return null;
      }
    },
  },
});
