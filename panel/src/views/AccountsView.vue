<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { usePanelStore } from "../stores/panel";
import { BASE, type Account } from "../api";
import ConfirmDialog from "../components/ConfirmDialog.vue";

const store = usePanelStore();

// 待确认的危险操作。仅「禁用」与「重载 auths」入列：前者要手动启用才能恢复，
// 后者会重建整池账号对象；「刷新」「保活」可重复且无副作用，不套确认，
// 否则确认框会被点成条件反射而失效。
const pending = ref<{ run: () => void; title: string; message: string; label: string } | null>(null);
function askDisable(a: Account) {
  pending.value = {
    title: "禁用账号",
    message: "「" + nameOf(a) + "」将被移出可用池，需手动启用才能恢复。",
    label: "禁用",
    run: () => act("/admin/api/accounts/disable", { uid: uidOf(a), reason: "manual disable from panel" }, a, "禁用"),
  };
}
function askReload() {
  pending.value = {
    title: "重载 auths",
    message: "将按 auths/ 目录重新载入全部账号并重建账号池，进行中的请求不受影响。",
    label: "重载",
    run: () => store.runAction("/admin/api/reload", {}, "重载 auths"),
  };
}
function confirmPending() {
  const p = pending.value;
  pending.value = null;
  p?.run();
}

const stats = computed(() => {
  let inflight = 0, errs = 0;
  for (const a of store.accounts) {
    inflight += Number(a.active_concurrent || 0);
    errs += Number(a.err_count || 0);
  }
  return [
    { v: store.accounts.length, k: "账号总数", cls: "" },
    { v: store.healthy, k: "可用", cls: "good" },
    { v: store.cooling, k: "冷却中", cls: "warn" },
    { v: store.disabled, k: "已禁用", cls: "bad" },
    { v: inflight, k: "在途会话", cls: "" },
    { v: errs, k: "故障计数合计", cls: "" },
  ];
});
const note = computed(() =>
  store.accounts.length
    ? store.healthy + " 可用 · " + store.cooling + " 冷却 · " + store.disabled + " 禁用"
    : "空池");

function nameOf(a: Account) { return a.nickname || a.name || "未命名"; }
function uidOf(a: Account) { return a.uid || a.name || ""; }
function rowClass(a: Account) { return a.disabled ? "off" : (a.cooling ? "cool" : ""); }

/* 冷却倒计时：每秒重算剩余时间；有账号到期后补拉一次总览。 */
const now = ref(Date.now());
let tick: ReturnType<typeof setInterval> | null = null;
let reloaded = false;
onMounted(() => {
  tick = setInterval(() => {
    now.value = Date.now();
    const anyExpired = store.accounts.some((a) => {
      if (!a.cooling || !a.until) return false;
      const until = new Date(a.until).getTime();
      return !isNaN(until) && until - now.value <= 0;
    });
    if (anyExpired && !reloaded) {
      reloaded = true;
      setTimeout(async () => { try { await store.loadOverview(); } catch {} reloaded = false; }, 1200);
    }
  }, 1000);
});
onBeforeUnmount(() => { if (tick) clearInterval(tick); });
function countdown(a: Account): string {
  if (!a.until) return "冷却中";
  const until = new Date(a.until).getTime();
  if (isNaN(until)) return "冷却中";
  const left = until - now.value;
  if (left <= 0) return "恢复中";
  const s = Math.max(0, Math.floor(left / 1000));
  const p2 = (n: number) => String(n).padStart(2, "0");
  const txt = s < 60 ? s + "s"
    : s < 3600 ? Math.floor(s / 60) + "m" + p2(s % 60) + "s"
    : Math.floor(s / 3600) + "h" + p2(Math.floor((s % 3600) / 60)) + "m";
  return "剩 " + txt;
}

async function act(path: string, body: unknown, a: Account, verb: string) {
  await store.runAction(path, body, nameOf(a) + " " + verb);
}
</script>

<template>
  <section class="view">
    <div class="stats">
      <div v-for="s in stats" :key="s.k" class="stat" :class="s.cls">
        <div class="v">{{ s.v }}</div><div class="k">{{ s.k }}</div>
      </div>
    </div>

    <div class="box">
      <header>
        <h3>账号池</h3>
        <span class="grow" />
        <span class="note">{{ note }}</span>
        <button class="xs" @click="store.runAction('/admin/api/credits', {}, '全员刷新状态')">全员刷新状态</button>
        <button class="xs" @click="store.runAction('/admin/api/keepalive', {}, '全员保活')">全员保活</button>
        <button class="xs" @click="askReload">重载 auths</button>
        <button class="xs primary" @click="$emit('add')">＋ 授权登录</button>
      </header>
      <div class="tbl-wrap">
        <table class="acc">
          <thead><tr>
            <th class="mark" aria-hidden="true" />
            <th>账号</th><th>状态</th><th>默认模型</th>
            <th>Token 剩余</th><th>在途</th><th>故障 / 原因</th><th class="c-acts" />
          </tr></thead>
          <tbody>
            <tr v-if="!store.accounts.length">
              <td colspan="8" class="empty">
                <div class="big">暂无账号</div>
                点击右上角「＋ 授权登录」添加，或在服务器运行 login.sh
              </td>
            </tr>
            <tr v-for="a in store.accounts" :key="uidOf(a)" :class="rowClass(a)">
              <td class="mark" aria-hidden="true"><i /></td>
              <td class="who">
                <div class="nm">{{ nameOf(a) }}</div>
                <div class="id" :title="uidOf(a)">{{ uidOf(a) }}</div>
              </td>
              <td>
                <span v-if="a.disabled" class="tag bad">已禁用</span>
                <span v-else-if="a.cooling" class="tag warn">冷却中</span>
                <span v-else class="tag ok">可用</span>
                <div v-if="a.cooling" class="cd">{{ countdown(a) }}</div>
              </td>
              <td class="num">{{ a.default_model || "默认" }}</td>
              <td class="num" :title="a.expires_at || ''">{{ a.token_remaining || "—" }}</td>
              <td class="num">{{ Number(a.active_concurrent || 0) }} / {{ Number(a.max_concurrent || 0) }}</td>
              <td class="reason" :title="a.last_error || a.reason || ''">
                <span v-if="a.err_count" class="tag mute">err {{ a.err_count }}</span>
                {{ a.reason || "—" }}
              </td>
              <td class="c-acts">
                <div class="acts">
                  <button @click="act('/admin/api/credits', { uid: uidOf(a) }, a, '刷新状态')">刷新</button>
                  <button @click="act('/admin/api/keepalive', { uid: uidOf(a) }, a, '保活')">保活</button>
                  <button v-if="a.disabled" @click="act('/admin/api/accounts/enable', { uid: uidOf(a) }, a, '启用')">启用</button>
                  <button v-else class="danger" @click="askDisable(a)">禁用</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <ConfirmDialog
      v-if="pending"
      :title="pending.title"
      :message="pending.message"
      :confirm-label="pending.label"
      danger
      @confirm="confirmPending"
      @cancel="pending = null"
    />
  </section>
</template>
