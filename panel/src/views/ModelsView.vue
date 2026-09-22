<script setup lang="ts">
import { computed, ref } from "vue";
import { usePanelStore } from "../stores/panel";
import { BASE } from "../api";

const store = usePanelStore();
const refreshing = ref(false);

const models = computed(() => (store.overview?.models as { id: string }[] | undefined) || []);
const baseUrl = computed(() => location.origin + BASE + "/v1");

async function refresh() {
  // 后端 /v1/models 有 1h 缓存，必须打 models/refresh 才能真重发现。
  refreshing.value = true;
  try {
    const d = await store.runAction("/admin/api/models/refresh", {}, "重新获取模型");
    if (d) store.toast(d.message || "模型已更新", d.ok !== false);
  } finally { refreshing.value = false; }
}
async function copy(id: string) {
  try { await navigator.clipboard.writeText(id); store.toast("已复制 " + id); }
  catch { store.toast("复制失败：" + id, false); }
}
</script>

<template>
  <section class="view">
    <div class="box">
      <header>
        <h3>可用模型 <span class="hint">已过滤已知不可用条目；点击标签复制模型 ID</span></h3>
        <span class="grow" />
        <span class="note">{{ models.length }} 个</span>
        <button class="xs" :disabled="refreshing" @click="refresh">{{ refreshing ? "获取中…" : "重新获取" }}</button>
      </header>
      <div class="pad">
        <div class="chips">
          <template v-if="models.length">
            <button v-for="m in models" :key="m.id" class="chip" :title="'点击复制模型 ID：' + m.id" :aria-label="'复制模型 ID ' + m.id" @click="copy(m.id)">{{ m.id }}</button>
          </template>
          <span v-else class="muted">暂无模型</span>
        </div>
      </div>
    </div>
    <div class="box">
      <header><h3>接入信息</h3></header>
      <div class="pad">
        <div class="kv">
          <div class="cell"><div class="k">OpenAI 兼容 base</div><div class="v">{{ baseUrl }}</div></div>
          <div class="cell"><div class="k">对话端点</div><div class="v">/v1/chat/completions</div></div>
          <div class="cell"><div class="k">模型列表</div><div class="v">/v1/models</div></div>
        </div>
      </div>
    </div>
  </section>
</template>
