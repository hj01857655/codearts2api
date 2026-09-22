<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from "vue";
import { api } from "../api";
import { usePanelStore } from "../stores/panel";

const emit = defineEmits<{ close: [] }>();
const store = usePanelStore();

const sessionId = ref("");
const authUrl = ref("");
const ready = ref(false);
const load = ref(false);
const done = ref("");
const err = ref("");
const pollMsg = ref("");
const pollSpin = ref(false);
const callbackUrl = ref("");
const busy = ref(false);

let timer: ReturnType<typeof setInterval> | null = null;
function stopPoll() { if (timer) { clearInterval(timer); timer = null; } }

function setPoll(msg: string, spin: boolean) { pollMsg.value = msg; pollSpin.value = spin; }

async function start() {
  if (busy.value) return;
  busy.value = true;
  load.value = true;
  err.value = "";
  try {
    const d = await api<any>("/admin/api/oauth/start", { method: "POST", body: "{}" });
    if (!d.ok || !d.auth_url) throw new Error(d.message || "发起失败");
    sessionId.value = d.session_id;
    authUrl.value = d.auth_url;
    ready.value = true;
    setPoll("已生成链接，请在浏览器完成登录，正在自动检测…", true);
    store.logLine("op", "授权登录 · 已生成授权链接");
    try { window.open(d.auth_url, "_blank", "noopener,noreferrer"); } catch {}
    stopPoll();
    timer = setInterval(() => void poll(true), 2500);
  } catch (e: any) {
    err.value = "发起失败：" + e.message;
    store.logLine("err", "授权登录 · " + e.message);
  } finally {
    load.value = false;
    busy.value = false;
  }
}

async function importCallback() {
  const url = callbackUrl.value.trim();
  if (!sessionId.value || !url) {
    err.value = "请先发起授权，再粘贴浏览器地址栏中的完整回调地址";
    return;
  }
  busy.value = true;
  setPoll("正在中转回调…", true);
  try {
    const d = await api<any>("/admin/api/oauth/import-callback", {
      method: "POST",
      body: JSON.stringify({ session_id: sessionId.value, callback_url: url }),
    });
    if (!d.ok) throw new Error(d.message || "回调中转失败");
    callbackUrl.value = "";
    setPoll(d.next_url ? "回调已接收，继续等待授权结果…" : (d.message || "回调已接收"), false);
    if (d.next_url) {
      authUrl.value = d.next_url;
      try { window.open(d.next_url, "_blank", "noopener,noreferrer"); } catch {}
    }
    setTimeout(() => void poll(true), 600);
  } catch (e: any) {
    setPoll("回调中转失败：" + e.message, false);
    store.logLine("err", "授权登录 · 回调中转 " + e.message);
  } finally { busy.value = false; }
}

async function poll(silent: boolean) {
  if (!sessionId.value || busy.value) return;
  busy.value = true;
  if (!silent) setPoll("正在确认授权结果…", true);
  try {
    const d = await api<any>("/admin/api/oauth/poll", {
      method: "POST",
      body: JSON.stringify({ session_id: sessionId.value }),
    });
    if (d.status === "pending") {
      if (!silent) setPoll(d.message || "等待浏览器完成授权…", true);
      return;
    }
    stopPoll();
    if (d.status === "done" && d.ok) {
      sessionId.value = "";
      pollMsg.value = ""; pollSpin.value = false;
      done.value = "✓ " + (d.message || "登录成功");
      store.toast(d.message || "登录成功");
      store.logLine("op", "授权登录 · " + (d.message || "成功"));
      await store.loadOverview();
      setTimeout(() => emit("close"), 900);
      return;
    }
    setPoll("✗ " + (d.message || "授权失败"), false);
    store.toast(d.message || "授权失败", false);
    store.logLine("err", "授权登录 · " + (d.message || "失败"));
  } catch (e: any) {
    if (!silent) setPoll("查询失败：" + e.message, false);
    store.logLine("err", "授权登录 · 查询 " + e.message);
  } finally { busy.value = false; }
}

async function copyUrl() {
  const u = authUrl.value.trim();
  try { await navigator.clipboard.writeText(u); store.toast("已复制授权链接"); }
  catch {
    const ta = document.createElement("textarea");
    ta.value = u; document.body.appendChild(ta); ta.select();
    try { document.execCommand("copy"); store.toast("已复制授权链接"); } catch { store.toast("复制失败", false); }
    ta.remove();
  }
}
function openUrl() {
  const u = authUrl.value.trim();
  if (u && u.startsWith("http")) window.open(u, "_blank", "noopener,noreferrer");
}
function close() {
  stopPoll();
  sessionId.value = "";
  emit("close");
}
function veilClick(e: MouseEvent) { if (e.target === e.currentTarget) close(); }

onBeforeUnmount(stopPoll);
</script>

<template>
  <div class="veil on" @click="veilClick">
    <div class="dlg">
      <header>
        <h3>授权登录</h3>
        <div class="hint">浏览器完成上游账号登录，网关自动载入账号池，无需重启。</div>
      </header>
      <div class="body">
        <div class="steps" style="margin-bottom:12px">
          <div><b>1.</b> 点「发起授权」获取链接</div>
          <div><b>2.</b> 在浏览器打开并完成登录</div>
          <div><b>3.</b> 若浏览器停在 127.0.0.1，把地址栏完整回调地址粘贴到下方中转</div>
        </div>
        <div v-if="load" class="state"><span class="spin" />正在获取授权链接</div>
        <div v-if="ready">
          <div class="state">在浏览器打开以下链接并登录：</div>
          <div class="url">{{ authUrl }}</div>
          <div v-if="pollMsg" class="state">
            <span v-if="pollSpin" class="spin" />
            <span :class="pollMsg.startsWith('✗') ? 'err' : ''" style="color:inherit">{{ pollMsg }}</span>
          </div>
        </div>
        <div v-if="done" class="state ok">{{ done }}</div>
        <div v-if="err" class="state err">{{ err }}</div>
        <label v-if="ready" class="fld" style="margin:14px 0 0"><span class="lb">远程浏览器回调中转</span>
          <input v-model="callbackUrl" type="url" placeholder="http://127.0.0.1:7866/oauth/callback?..." autocomplete="off" />
        </label>
      </div>
      <footer>
        <span class="grow" />
        <button class="primary" :disabled="busy" @click="start">发起授权</button>
        <button v-if="ready" @click="copyUrl">复制链接</button>
        <button v-if="ready" @click="openUrl">浏览器打开</button>
        <button v-if="ready" :disabled="busy" @click="importCallback">提交回调地址</button>
        <button @click="close">关闭</button>
      </footer>
    </div>
  </div>
</template>
