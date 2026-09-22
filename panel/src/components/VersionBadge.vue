<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { usePanelStore } from "../stores/panel";
import { api, type UpdateCheckResult, type UpdateActionResult } from "../api";
import { useModalDialog } from "../composables/useModalDialog";

const store = usePanelStore();
const open = ref(false);
const badge = ref<HTMLElement | null>(null);
const showConfirm = ref(false);

// 更新流程状态是本组件独有的状态机，不占全局 store
const version = ref(store.overview?.version || "");
const latestVersion = ref("");
const hasUpdate = ref(false);
const updateChecked = ref(false);
const updateSupported = ref(true);
const updateReason = ref("");
const canRollback = ref(false);
const updateChecking = ref(false);
const updating = ref(false);
const needRestart = ref(false);
const updateMsg = ref("");
const updateFailed = ref(false);
const restarting = ref(false);

const label = computed(() =>
  updateMsg.value || (hasUpdate.value ? "可更新到 v" + latestVersion.value : "已是最新"));

async function check() {
  updateChecking.value = true;
  updateMsg.value = ""; updateFailed.value = false;
  try {
    const d = await api<UpdateCheckResult>("/admin/api/update/check");
    if (d.configured === false) {
      updateMsg.value = d.message || "未配置在线更新";
      updateFailed.value = true;
      return;
    }
    const st = d.status;
    updateChecked.value = true;
    version.value = st?.current || version.value;
    latestVersion.value = st?.latest || "";
    hasUpdate.value = !!st?.has_update;
    updateSupported.value = st?.supported !== false;
    updateReason.value = st?.reason || "";
    canRollback.value = !!st?.can_rollback;
  } catch (e: any) {
    updateMsg.value = "检测失败：" + e.message;
    updateFailed.value = true;
  } finally { updateChecking.value = false; }
}

async function apply() {
  showConfirm.value = false;
  updating.value = true;
  applyStart = Date.now();
  tickApply();
  applyTimer = setInterval(tickApply, 1000);
  updateMsg.value = "正在从 GitHub 下载更新…";
  updateFailed.value = false;
  try {
    const d = await api<UpdateActionResult>("/admin/api/update/apply", { method: "POST", body: "{}" });
    if (d.status) {
      version.value = d.status.current || version.value;
      latestVersion.value = d.status.latest || latestVersion.value;
      canRollback.value = !!d.status.can_rollback;
    }
    hasUpdate.value = false;
    needRestart.value = !!d.need_restart;
    updateMsg.value = d.message || "已更新";
    store.logLine("op", "在线更新 · " + updateMsg.value);
  } catch (e: any) {
    updateMsg.value = e.message;
    updateFailed.value = true;
    store.logLine("err", "在线更新 · " + e.message);
  } finally {
    updating.value = false;
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
  updating.value = true;
  try {
    const d = await api<UpdateActionResult>("/admin/api/update/rollback", { method: "POST", body: "{}" });
    needRestart.value = !!d.need_restart;
    updateMsg.value = d.message || "已回滚";
    hasUpdate.value = false;
    store.logLine("op", "在线更新 · " + updateMsg.value);
  } catch (e: any) {
    updateMsg.value = e.message; updateFailed.value = true;
    store.logLine("err", "在线更新 · " + e.message);
  } finally { updating.value = false; }
}

const countdown = ref(0);
let timer: ReturnType<typeof setInterval> | null = null;
async function restart() {
  restarting.value = true;
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
      // 只要 fetch 得到响应就说明服务已起来，不按状态码判断：/healthz 在账号池
      // 无可用账号时返回 503（见后端 handler.healthz），那是健康状态而非「未就绪」，
      // 用 r.ok 会把这种情况误判成没起来，白等到循环耗尽。
      await fetch((location.pathname.replace(/\/(admin|panel)\/?$/, "") || "") + "/healthz", { cache: "no-cache" });
      window.location.reload();
      return;
    } catch { /* 连接被拒 = 还没起来，继续等 */ }
    await new Promise((r) => setTimeout(r, 1000));
  }
  window.location.reload();
}

function docClick(e: MouseEvent) {
  if (badge.value && !badge.value.contains(e.target as Node)) open.value = false;
}
onMounted(() => document.addEventListener("click", docClick));
onBeforeUnmount(() => { document.removeEventListener("click", docClick); if (timer) clearInterval(timer); });

// 确认框是对话框（模态，开跑前必须明确选一个出口），ESC 可关闭；
// 更新途中不可关闭，避免用户以为中止了实际仍在替换二进制。
const confirmPanel = ref<HTMLElement | null>(null);
function closeConfirm() { if (!updating.value) showConfirm.value = false; }
useModalDialog(() => showConfirm.value, closeConfirm, confirmPanel);

// store.overview 是异步加载的：挂载时可能还没到，版本号要跟着它补上。
watch(
  () => store.overview?.version,
  (v) => { if (v && !version.value) version.value = v; },
);
</script>

<template>
  <div ref="badge" class="vbadge">
    <button :class="hasUpdate ? 'hasupdate' : 'uptodate'" :title="label" :aria-label="label"
      aria-haspopup="dialog" :aria-expanded="open"
      @click.stop="open = !open; if (open && !updateChecked) void check()">
      <span>v{{ version || "-" }}</span>
      <span v-if="hasUpdate" class="updot"><i /><b /></span>
    </button>

    <div v-if="open" class="vdrop">
      <div class="vh">
        <span>软件更新</span>
        <button :disabled="updateChecking" title="刷新" aria-label="重新检测更新" @click="check">
          <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" :class="{ spin: updateChecking }" style="animation:none" stroke-linecap="round" stroke-linejoin="round"><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9"/><path d="M13.5 2.5v3h-3"/></svg>
        </button>
      </div>

      <div class="vsub">{{ hasUpdate ? "发现新版本 v" + latestVersion : (updateChecked ? "已是最新" : "点击刷新图标检测更新") }}</div>

      <template v-if="updateFailed">
        <div class="card err">
          <div class="ico">✕</div>
          <div class="grow">
            <div style="font-weight:550">更新失败</div>
            <div style="opacity:.75;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{ updateMsg }}</div>
          </div>
        </div>
        <button class="wbtn retry" :disabled="updating" @click="check">重试</button>
      </template>

      <template v-else-if="needRestart">
        <div class="card ok">
          <div class="ico">✓</div>
          <div class="grow">
            <div style="font-weight:550">更新完成</div>
            <div style="opacity:.75">需要重启服务生效</div>
          </div>
        </div>
        <button class="wbtn re" :disabled="restarting" @click="restart">
          <span v-if="restarting">重启中{{ countdown > 0 ? " (" + countdown + "s)" : "" }}</span>
          <span v-else>立即重启</span>
        </button>
      </template>

      <template v-else-if="hasUpdate && updateSupported">
        <div class="card warn">
          <div class="ico">↓</div>
          <div class="grow">
            <div style="font-weight:550">发现新版本</div>
            <div style="opacity:.75">v{{ latestVersion }}</div>
          </div>
        </div>
        <template v-if="updating">
          <div class="card">
            <div class="spin" />
            <div class="grow">
              <div style="font-weight:550">下载中（已用时 {{ elapsedText() }}）</div>
            </div>
          </div>
        </template>
        <button v-else class="wbtn go" @click="showConfirm = true">立即更新</button>
      </template>

      <template v-else-if="hasUpdate && !updateSupported">
        <div class="card warn">
          <div class="ico">↓</div>
          <div class="grow">
            <div style="font-weight:550">发现新版本 v{{ latestVersion }}</div>
            <div style="opacity:.75">{{ updateReason || "当前环境不支持在线更新" }}</div>
          </div>
        </div>
      </template>

      <template v-if="canRollback && !needRestart">
        <button class="wbtn" :disabled="updating" @click="rollback">
          <span v-if="updating">回滚中…</span><span v-else>回滚上一版</span>
        </button>
      </template>
    </div>

    <!-- 更新确认对话框：下载是长事务，开跑前须明确确认 -->
    <div v-if="showConfirm" class="veil on" @click.self="closeConfirm">
      <div ref="confirmPanel" class="dlg" role="dialog" aria-modal="true" aria-label="确认更新" tabindex="-1" style="width:420px">
        <header>
          <h3>确认更新</h3>
          <div class="hint">将下载并安装 v{{ latestVersion }}，完成后需重启服务生效；下载期间请保持页面打开。</div>
        </header>
        <footer>
          <button :disabled="updating" @click="closeConfirm">取消</button>
          <span class="grow" />
          <button class="primary" :disabled="updating" @click="apply">开始更新</button>
        </footer>
      </div>
    </div>
  </div>
</template>