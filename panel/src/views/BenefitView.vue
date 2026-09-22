<script setup lang="ts">
import { computed, ref } from "vue";
import { usePanelStore } from "../stores/panel";
import ConfirmDialog from "../components/ConfirmDialog.vue";

const store = usePanelStore();
const checking = ref(false);
const claiming = ref(false);
const confirmingClaim = ref(false);

function nameOf(uid: string): string {
  const a = store.accounts.find((x) => (x.uid || x.name) === uid);
  return a ? (a.nickname || a.name || uid) : uid;
}
function fmtTime(t?: number | string): string {
  if (!t) return "从未签到";
  const d = new Date(typeof t === "number" ? new Date(t).toISOString() : t);
  return isNaN(d.getTime()) ? String(t) : d.toLocaleString();
}
function pctColor(pct: number, has: boolean): string {
  if (!has) return "var(--ink-3)";
  return pct > 50 ? "var(--ok)" : pct > 20 ? "var(--warn)" : "var(--bad)";
}
// 今天是否已签到。额度按天发放，「上次签到」只有落在今天才算已签。
// 与后端 upstream.ClaimedToday 同一判据：按本地自然日比较，不只看非 0。
function signedToday(t?: number | string): boolean {
  if (!t) return false;
  const d = new Date(typeof t === "number" ? t : new Date(t).getTime());
  if (isNaN(d.getTime())) return false;
  const now = new Date();
  return d.getFullYear() === now.getFullYear()
    && d.getMonth() === now.getMonth()
    && d.getDate() === now.getDate();
}

const rows = computed(() => {
  const results = store.benefit?.results || [];
  return results.map((r) => {
    const has = !!r.has_balance;
    const total = Number(r.total || 0), used = Number(r.used || 0), remain = Number(r.remain || 0);
    const pct = has && total > 0 ? Math.round((remain / total) * 100) : 0;
    return { r, has, total, used, remain, pct, color: pctColor(pct, has), today: signedToday(r.create_time) };
  });
});
const totals = computed(() => {
  const results = store.benefit?.results || [];
  let totalSum = 0, usedSum = 0, remainSum = 0, monthlySum = 0, counted = 0;
  for (const r of results) {
    if (!r.has_balance) continue;
    totalSum += Number(r.total || 0); usedSum += Number(r.used || 0);
    remainSum += Number(r.remain || 0); monthlySum += Number(r.monthly_used || 0);
    counted++;
  }
  const pct = totalSum > 0 ? Math.round((remainSum / totalSum) * 100) : 0;
  return { totalSum, usedSum, remainSum, monthlySum, counted, pct, color: pctColor(pct, counted > 0), show: counted > 0 };
});
const note = computed(() => {
  const results = store.benefit?.results || [];
  if (!results.length) return store.benefit ? "无数据" : "未查询";
  const c = totals.value.counted;
  return c === results.length ? results.length + " 个账号" : c + " / " + results.length + " 个账号有余额";
});

// 全部账号今日均已签到：此时再点「全部签到」只会得到「未重复领取」，
// 故置灰。仅在结论可信时才置灰——没查过、上游没返回时间、或任一账号查询
// 失败都不算，否则会把「不知道」当成「已签到」而挡住真正需要的签到。
const allSignedToday = computed(() => {
  const results = store.benefit?.results || [];
  if (!results.length) return false;
  return results.every((r) => r.ok && signedToday(r.create_time));
});

async function refresh() {
  checking.value = true;
  try { await store.loadBenefit(false); } finally { checking.value = false; }
}
async function claimAll() {
  confirmingClaim.value = false;
  claiming.value = true;
  try {
    const d = await store.runAction("/admin/api/checkin", {}, "全部签到");
    if (d && d.results) store.applyBenefitResults(d.results);
  } finally { claiming.value = false; }
}
</script>

<template>
  <section class="view">
    <div class="box">
      <header>
        <h3>福利额度 <span class="hint">上游限时福利签到与余额</span></h3>
        <span class="grow" />
        <span class="note">{{ note }}</span>
        <button class="xs" :disabled="checking || claiming" @click="refresh">{{ checking ? "查询中…" : "刷新额度" }}</button>
        <button
          class="xs primary"
          :disabled="claiming || checking || allSignedToday"
          :title="allSignedToday ? '所有账号今日均已签到，无需重复操作' : ''"
          @click="confirmingClaim = true"
        >
          {{ claiming ? "签到中…" : allSignedToday ? "今日已全部签到" : "全部签到" }}
        </button>
      </header>
      <div class="tbl-wrap">
        <table class="acc">
          <thead><tr>
            <th class="mark" aria-hidden="true" />
            <th>账号</th><th>上次签到</th><th>今日额度</th><th>今日已用</th><th>今日剩余</th><th>剩余占比</th><th>本月已用</th>
          </tr></thead>
          <tbody>
            <tr v-if="!rows.length">
              <td colspan="8" class="empty">{{ store.benefit ? "上游未返回数据" : "点击「刷新额度」查询上游签到状态与余额" }}</td>
            </tr>
            <tr v-for="row in rows" :key="row.r.uid" :class="row.r.ok ? '' : 'off'">
              <td class="mark" aria-hidden="true"><i /></td>
              <td class="who">
                <div class="nm">{{ nameOf(row.r.uid) }}</div>
                <div class="id" :title="row.r.uid">{{ row.r.uid }}</div>
                <div v-if="row.r.message" class="muted" style="font-size:11.5px">{{ row.r.message }}</div>
              </td>
              <td class="num">
                <span v-if="!row.r.ok" class="muted">查询失败</span>
                <template v-else>{{ fmtTime(row.r.create_time) }}</template>
                <span v-if="row.r.ok" class="tag" :class="row.today ? 'ok' : 'mute'">
                  {{ row.today ? "今日已签" : "今日未签" }}
                </span>
              </td>
              <td class="num">{{ row.has ? row.total.toLocaleString() : "—" }}</td>
              <td class="num muted">{{ row.has ? row.used.toLocaleString() : "—" }}</td>
              <td class="num" :style="{ color: row.color }">{{ row.has ? row.remain.toLocaleString() : "—" }}</td>
              <td>
                <template v-if="row.has">
                  <div class="bar"><i :style="{ width: row.pct + '%', background: row.color }" /></div>
                  <span class="num muted" style="font-size:11px">{{ row.pct }}%</span>
                </template>
                <span v-else class="muted">—</span>
              </td>
              <td class="num muted">{{ row.has ? Number(row.r.monthly_used || 0).toLocaleString() : "—" }}</td>
            </tr>
          </tbody>
          <tfoot v-if="totals.show">
            <tr>
              <td class="mark" />
              <td class="who"><div class="nm">合计</div></td>
              <td class="num" />
              <td class="num">{{ totals.totalSum.toLocaleString() }}</td>
              <td class="num muted">{{ totals.usedSum.toLocaleString() }}</td>
              <td class="num" :style="{ color: totals.color }">{{ totals.remainSum.toLocaleString() }}</td>
              <td>
                <div class="bar"><i :style="{ width: totals.pct + '%', background: totals.color }" /></div>
                <span class="num muted" style="font-size:11px">{{ totals.pct }}%</span>
              </td>
              <td class="num muted">{{ totals.monthlySum.toLocaleString() }}</td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>

    <ConfirmDialog
      v-if="confirmingClaim"
      :busy="claiming"
      title="全部签到"
      message="将对账号池内全部账号发起今日签到。福利额度按日发放，需每天签到才能拿到当日额度；同一天内重复执行不会重复领取，但会逐个账号访问上游。"
      confirm-label="全部签到"
      @confirm="claimAll"
      @cancel="confirmingClaim = false"
    />
  </section>
</template>
