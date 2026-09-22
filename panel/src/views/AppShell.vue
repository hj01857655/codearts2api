<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { usePanelStore } from "../stores/panel";
import { config } from "../config";
import { THEMES, currentTheme, applyTheme } from "../composables/useTheme";
import VersionBadge from "../components/VersionBadge.vue";
import AccountsView from "./AccountsView.vue";
import BenefitView from "./BenefitView.vue";
import ModelsView from "./ModelsView.vue";
import ScheduleView from "./ScheduleView.vue";
import LogView from "./LogView.vue";
import OauthDialog from "../components/OauthDialog.vue";

const store = usePanelStore();

const VIEWS: { id: string; label: string; sub: string }[] = [
  { id: "accounts", label: "账号池", sub: "账号健康 · Token · 在途" },
  { id: "benefit", label: "福利额度", sub: "限时福利签到与余额" },
  { id: "models", label: "模型", sub: "上游可用模型与接入地址" },
  { id: "schedule", label: "配置", sub: "巡检保活与服务参数" },
  { id: "log", label: "操作记录", sub: "本页动作流水" },
];
const valid = VIEWS.map((v) => v.id);
const view = ref("accounts");
const title = computed(() => VIEWS.find((v) => v.id === view.value)?.label || "账号池");
const sub = computed(() => VIEWS.find((v) => v.id === view.value)?.sub || "");

function switchTo(v: string, keepHash = false) {
  if (!valid.includes(v)) v = "accounts";
  view.value = v;
  if (!keepHash && location.hash.slice(1) !== v) location.hash = v;
  // 福利额度是唯一要打上游取数的视图，进页面就得自己拉一次（旧版面板
  // switchView 同款）：只放在 onMounted 时，从导航切进来永远是空表。
  if (v === "benefit" && !store.benefit) void store.loadBenefit(true);
}
function onHash() { switchTo(location.hash.slice(1), true); }
onMounted(() => {
  window.addEventListener("hashchange", onHash);
  switchTo(location.hash.slice(1) || "accounts", true);
});
onBeforeUnmount(() => window.removeEventListener("hashchange", onHash));

const settingsOpen = ref(false);
const userOpen = ref(false);
const oauthOpen = ref(false);
const theme = ref(currentTheme());
function toggleSettings() { settingsOpen.value = !settingsOpen.value; userOpen.value = false; }
function toggleUser() { userOpen.value = !userOpen.value; settingsOpen.value = false; }
function pickTheme(t: string) { applyTheme(t); theme.value = t; settingsOpen.value = false; store.toast("主题：" + (THEMES.find(([id]) => id === t)?.[1] || t)); }
function docClick(e: MouseEvent) {
  if (!(e.target as HTMLElement).closest(".menu-wrap")) { settingsOpen.value = false; userOpen.value = false; }
}
function esc() { settingsOpen.value = false; userOpen.value = false; }
onMounted(() => { document.addEventListener("click", docClick); document.addEventListener("keydown", esc); });
onBeforeUnmount(() => { document.removeEventListener("click", docClick); document.removeEventListener("keydown", esc); });

const hasKey = computed(() => !!localStorage.getItem("codearts2api_panel_api_key"));
const pulseClass = computed(() => {
  if (store.poolState === "bad") return "pulse bad";
  if (store.poolState === "warn") return "pulse warn";
  if (store.poolState === "ok") return "pulse";
  return "pulse";
});
const navState = computed(() =>
  store.accounts.length ? store.healthy + " 可用 / " + store.accounts.length + " 账号" : "账号池为空");
const navBase = computed(() => (String(store.overview?.region || "cn")) + " · " + location.host);

async function refresh() {
  try { await store.loadOverview(); store.toast("已刷新"); store.logLine("op", "刷新总览"); }
  catch (e: any) { store.toast(e.message, false); store.logLine("err", "刷新总览 · " + e.message); }
}
</script>

<template>
  <div class="shell">
    <nav class="nav">
      <div class="brand">
        <div class="name">{{ config.serviceTitle }}</div>
        <div class="sub">{{ sub || "控制台" }}</div>
        <VersionBadge />
      </div>
      <ul>
        <li v-for="v in VIEWS" :key="v.id">
          <a href="#" :class="{ on: view === v.id }" @click.prevent="switchTo(v.id)">
            <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4">
              <template v-if="v.id === 'accounts'"><circle cx="8" cy="5.5" r="2.6"/><path d="M2.8 13.5c.6-2.6 2.7-4 5.2-4s4.6 1.4 5.2 4"/></template>
              <template v-else-if="v.id === 'benefit'"><path d="M2 13.5h12"/><path d="M4.5 13.5V8.2M8 13.5V3.5M11.5 13.5v-3"/></template>
              <template v-else-if="v.id === 'models'"><path d="M8 1.8 14 5v6L8 14.2 2 11V5z"/><path d="M2 5l6 3.2L14 5M8 8.2v6"/></template>
              <template v-else-if="v.id === 'schedule'"><circle cx="8" cy="8" r="5.6"/><path d="M8 4.6V8l2.4 1.6"/></template>
              <template v-else><path d="M2.5 3.5h11M2.5 8h11M2.5 12.5h7"/></template>
            </svg>
            {{ v.label }}
          </a>
        </li>
      </ul>
      <div class="sig">
        <div class="row"><span :class="pulseClass" /><span>{{ navState }}</span></div>
        <div class="row" style="margin-top:5px"><span class="mono">{{ navBase }}</span></div>
      </div>
    </nav>

    <div class="main">
      <div class="topbar">
        <h2>{{ title }}</h2>
        <span class="sep">/</span>
        <span class="meta">更新于 {{ store.lastRefreshed || "-" }}</span>
        <div class="grow" />
        <button class="icon ghost" title="刷新" aria-label="刷新" @click="refresh">
          <svg viewBox="0 0 16 16" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9"/><path d="M13.5 2.5v3h-3"/></svg>
        </button>
        <div class="menu-wrap">
          <button class="icon ghost" title="设置" aria-label="设置" aria-haspopup="menu" :aria-expanded="settingsOpen" @click.stop="toggleSettings">
            <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3.2"/><path d="M19.6 14.4a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.03 1.55v.17a2 2 0 1 1-4 0v-.09a1.7 1.7 0 0 0-1.11-1.55 1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.55-1.03h-.17a2 2 0 1 1 0-4h.09a1.7 1.7 0 0 0 1.55-1.11 1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34h.08a1.7 1.7 0 0 0 1.03-1.55v-.17a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1.03 1.55 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87v.08a1.7 1.7 0 0 0 1.55 1.03h.17a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.55 1.03z"/></svg>
          </button>
          <div v-if="settingsOpen" class="menu" role="menu">
            <div class="grp">主题</div>
            <button v-for="[id, label] in THEMES" :key="id" role="menuitem" :class="{ on: theme === id }" @click="pickTheme(id)">
              <span class="sw" :style="{ background: 'linear-gradient(135deg,var(--surface) 45%,var(--accent))' }" />{{ label }}<span class="tick">✓</span>
            </button>
          </div>
        </div>
        <div class="menu-wrap">
          <button title="账户" aria-haspopup="menu" :aria-expanded="userOpen" @click.stop="toggleUser">
            <span class="uinfo">
              <span class="uavatar" :class="{ off: !hasKey }">{{ hasKey ? "A" : "?" }}</span>
              <span>{{ hasKey ? "已登录" : "未设置密钥" }}</span>
              <svg class="uchev" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4.5 6.5 8 10l3.5-3.5"/></svg>
            </span>
          </button>
          <div v-if="userOpen" class="menu" role="menu">
            <div class="grp">账户</div>
            <div class="who">{{ hasKey ? "API Key 已保存（仅本机浏览器）" : "未设置 API Key" }}</div>
            <div class="who">v{{ store.overview?.version || "-" }}</div>
            <button class="danger" role="menuitem" @click="store.logout(); userOpen = false">退出登录</button>
          </div>
        </div>
      </div>

      <AccountsView v-show="view === 'accounts'" @add="oauthOpen = true" />
      <BenefitView v-show="view === 'benefit'" />
      <ModelsView v-show="view === 'models'" />
      <ScheduleView v-show="view === 'schedule'" />
      <LogView v-show="view === 'log'" />

      <footer class="foot">
        <span>{{ config.serviceTitle }}</span>
        <span class="grow" />
        <a href="https://github.com/hj01857655" target="_blank" rel="noopener noreferrer">@hj01857655</a>
      </footer>
    </div>

    <OauthDialog v-if="oauthOpen" @close="oauthOpen = false" />
  </div>
</template>
