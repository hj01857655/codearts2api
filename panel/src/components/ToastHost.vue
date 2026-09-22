<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from "vue";

const items = ref<{ id: number; msg: string; ok: boolean }[]>([]);
let seq = 0;
let off: (() => void) | null = null;

onMounted(() => {
  const handler = (e: Event) => {
    const d = (e as CustomEvent).detail as { msg: string; ok: boolean };
    const id = ++seq;
    items.value.push({ id, msg: d.msg, ok: d.ok });
    setTimeout(() => { items.value = items.value.filter((x) => x.id !== id); }, 3600);
  };
  window.addEventListener("panel:toast", handler);
  off = () => window.removeEventListener("panel:toast", handler);
});
onBeforeUnmount(() => { off?.(); });
</script>

<template>
  <div id="toasts">
    <div v-for="t in items" :key="t.id" class="tst" :class="t.ok ? 'ok' : 'err'">{{ t.msg }}</div>
  </div>
</template>
