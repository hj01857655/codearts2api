<script setup lang="ts">
import { computed } from "vue";
import { usePanelStore } from "../stores/panel";

const store = usePanelStore();
const watch = computed(() => (store.overview?.schedule as any)?.watch as Record<string, any> | undefined);
const mins = (m: unknown) => (m === 0 || m === undefined || m === null) ? "—" : m + " 分钟";
const rows = computed(() => {
  const w = watch.value;
  if (!w) return [];
  return [
    { k: "后台巡检", v: w.enabled ? "已开启" : "已关闭", cls: w.enabled ? "color:var(--ok)" : "color:var(--warn)" },
    { k: "轮询间隔", v: mins(w.poll_minutes), cls: "" },
    { k: "提前刷新窗口", v: mins(w.refresh_skew_minutes), cls: "" },
    { k: "保活间隔", v: mins(w.keepalive_interval), cls: "" },
    { k: "保活窗口", v: w.keepalive_window || "—", cls: "" },
    { k: "单账号最大在途", v: w.max_concurrent ?? "—", cls: "" },
  ];
});
</script>

<template>
  <section class="view">
    <div class="box">
      <header>
        <h3>后台巡检与保活</h3>
        <span class="grow" />
        <button class="xs" @click="store.loadOverview()">刷新</button>
      </header>
      <div class="pad">
        <div v-if="!rows.length" class="muted">未上报调度信息</div>
        <div v-else class="kv">
          <div v-for="r in rows" :key="r.k" class="cell">
            <div class="k">{{ r.k }}</div>
            <div class="v" :style="r.cls ? r.cls : undefined">{{ r.v }}</div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
