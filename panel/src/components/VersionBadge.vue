<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { usePanelStore } from "../stores/panel";
import { api, ApiError } from "../api";

const store = usePanelStore();
const open = ref(false);
const badge = ref<HTMLElement | null>(null);
const showConfirm = ref(false);

const hasUpdate = computed(() => store.hasUpdate);
const label = computed(() =>
  store.updateMsg || (store.hasUpdate ? "可更新到 v" + store.latestVersion : "已是最新"));

async function check() {
  store.updateChecking = true;
  store.updateMsg = ""; store.updateFailed = false;
  try {
    const d = await api<any>("/admin/api/update/check");
    if (d.configured === false) {
      store.updateMsg = d.message || "未配置在线更新";
      store.updateFailed = true;
      return;
    }
    const st = d.status;
    store.updateChecked = true;
    store.version = st.current || store.version;
    store.latestVersion = st.latest || "";
    store.hasUpdate = !!st.has_update;
    store.updateSupported = !!st.supported;
    store.updateReason = st.reason || "";
    store.canRollback = !!st.can_rollback;
  } catch (e: any) {
    store.updateMsg = "检测失败：" + e.message;
    store.updateFailed = true;
    if (e instanceof ApiError && e.status === 401) store.unauthorized();
  } finally { store.updateChecking = false; }
}

async function apply() {
  showConfirm.value = false;
  store.updating = true;
  applyStart = Date.now();
  tickApply();
  applyTimer = setInterval(tickApply, 1000);
  store.updateMsg = "正在从 GitHub 下载更新（境内服务器直连可能较慢，最长约 10 分钟）…";
  store.updateFailed = false;
  try {
    const d = await api<any>("/admin/api/update/apply", { method: "POST", body: "{}" });
    if (d.status) {
      store.version = d.status.current || store.version;
      store.latestVersion = d.status.latest || store.latestVersion;
      store.canRollback = !!d.status.can_rollback;
    }
    store.hasUpdate = false;
    store.needRestart = !!d.need_restart;
    store.updateMsg = d.message || "已更新";
    store.logLine("op", "在线更新 · " + store.updateMsg);
  } catch (e: any) {
    store.updateMsg = e.message;
    store.updateFailed = true;
    store.logLine("err", "在线更新 · " + e.message);
    if (e instanceof ApiError && e.status === 401) store.unauthorized();
  } finally {
    store.updating = false;
    if (applyTimer) { clearInterval(applyTimer); applyTimer = null; }
  }
}

/* apply 是长事务且服务端无进度上报，用已用时计时给用户持续反馈。 */
let applyTimer: ReturnType<typeof setInterval> | null = null;
let applyStart = 0;
const elapsed = ref(0);
function tickApply() { elapsed.value = Math.floor((Date.now() - applyStart) / 1000); }
const elapsedText = () => {
  const s = elapsed.value;
  return s < 60 ? s + "s" : Math.floor(s / 60) + "m" + String(s % 60).padStart(2, "0") + "s";
};

async function rollback() {
  store.updating = true;
  try {
    const d = await api<any>("/admin/api/update/rollback", { method: "POST", body: "{}" });
    store.needRestart = !!d.need_restart;
    store.updateMsg = d.message || "已回滚";
    store.hasUpdate = false;
    store.logLine("op", "在线更新 · " + store.updateMsg);
  } catch (e: any) {
    store.updateMsg = e.message; store.updateFailed = true;
    store.logLine("err", "在线更新 · " + e.message);
  } finally { store.updating = false; }
}

const countdown = ref(0);
let timer: ReturnType<typeof setInterval> | null = null;
async function restart() {
  store.restarting = true;
  countdown.value = 8;
  try { await api("/admin/api/update/restart", { method: "POST", body: "{}" }); }
  catch { /* 进程退出断开连接是预期行为 */ }
  timer = setInterval(() => {
    countdown.value--;
    if (countdown.value <= 0) { clearInterval(timer!); timer = null; void probeAndReload(); }
  }, 1000);
}
async function probeAndReload() {
  for (let i = 0; i < 30; i++) {
    try {
      const r = await fetch((location.pathname.replace(/\/(admin|panel)\/?$/, "") || "") + "/healthz", { cache: "no-cache" });
      if (r.ok) { window.location.reload(); return; }
    } catch { /* 未就绪继续等 */ }
    await new Promise((r) => setTimeout(r, 1000));
  }
  window.location.reload();
}

function docClick(e: MouseEvent) {
  if (badge.value && !badge.value.contains(e.target as Node)) open.value = false;
}
onMounted(() => document.addEventListener("click", docClick));
onBeforeUnmount(() => { document.removeEventListener("click", docClick); if (timer) clearInterval(timer); });
</script>

<template>
  <div ref="badge" class="vbadge">
    <button :class="hasUpdate ? 'hasupdate' : 'uptodate'" :title="label"
      @click.stop="open = !open; if (open && !store.updateChecked) void check()">
      <span>v{{ store.version || "-" }}</span>
      <span v-if="hasUpdate" class="updot"><i /><b /></span>
    </button>

    <div v-if="open" class="vdrop">
      <div class="vh">
        <span>软件更新</span>
        <button :disabled="store.updateChecking" title="刷新" @click="check">
          <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" :class="{ spin: store.updateChecking }" style="animation:none" stroke-linecap="round" stroke-linejoin="round"><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9"/><path d="M13.5 2.5v3h-3"/></svg>
        </button>
      </div>

      <div class="vsub">{{ hasUpdate ? "发现新版本 v" + store.latestVersion : (store.updateChecked ? "已是最新" : "点击刷新图标检测更新") }}</div>

      <template v-if="store.updateFailed">
        <div class="card err">
          <div class="ico">✕</div>
          <div class="grow">
            <div style="font-weight:550">更新失败</div>
            <div style="opacity:.75;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{ store.updateMsg }}</div>
          </div>
        </div>
        <button class="wbtn retry" :disabled="store.updating" @click="check">重试</button>
      </template>

      <template v-else-if="store.needRestart">
        <div class="card ok">
          <div class="ico">✓</div>
          <div class="grow">
            <div style="font-weight:550">更新完成</div>
            <div style="opacity:.75">需要重启服务生效</div>
          </div>
        </div>
        <button class="wbtn re" :disabled="store.restarting" @click="restart">
          <span v-if="store.restarting">重启中{{ countdown > 0 ? " (" + countdown + "s)" : "" }}</span>
          <span v-else>立即重启</span>
        </button>
      </template>

      <template v-else-if="hasUpdate && store.updateSupported">
        <div class="card warn">
          <div class="ico">↓</div>
          <div class="grow">
            <div style="font-weight:550">发现新版本</div>
            <div style="opacity:.75">v{{ store.latestVersion }}</div>
          </div>
        </div>
        <template v-if="store.updating">
          <div class="card">
            <div class="spin" />
            <div class="grow">
              <div style="font-weight:550">下载中（已用时 {{ elapsedText() }}）</div>
              <div style="opacity:.75">境内直连 GitHub 可能较慢，请保持页面打开</div>
            </div>
          </div>
        </template>
        <button v-else class="wbtn go" @click="showConfirm = true">立即更新</button>
      </template>

      <template v-else-if="hasUpdate && !store.updateSupported">
        <div class="card warn">
          <div class="ico">↓</div>
          <div class="grow">
            <div style="font-weight:550">发现新版本 v{{ store.latestVersion }}</div>
            <div style="opacity:.75">{{ store.updateReason || "当前环境不支持在线更新" }}</div>
          </div>
        </div>
      </template>

      <template v-if="store.canRollback && !store.needRestart">
        <button class="wbtn" :disabled="store.updating" @click="rollback">
          <span v-if="store.updating">回滚中…</span><span v-else>回滚上一版</span>
        </button>
      </template>
    </div>

    <!-- 更新确认弹窗：下载是长事务，开跑前须明确确认 -->
    <div v-if="showConfirm" class="veil on" @click.self="showConfirm = false">
      <div class="dlg" style="width:420px">
        <header>
          <h3>确认更新</h3>
          <div class="hint">将下载并安装 v{{ store.latestVersion }}，完成后需重启服务生效；下载期间请保持页面打开。</div>
        </header>
        <footer>
          <button :disabled="store.updating" @click="showConfirm = false">取消</button>
          <span class="grow" />
          <button class="primary" :disabled="store.updating" @click="apply">开始更新</button>
        </footer>
      </div>
    </div>
  </div>
</template>
