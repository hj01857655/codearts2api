<script setup lang="ts">
import { usePanelStore } from "../stores/panel";

const store = usePanelStore();
</script>

<template>
  <div id="toasts">
    <div v-for="t in store.toasts" :key="t.id" class="tst" :class="t.ok ? 'ok' : 'err'">{{ t.msg }}</div>
  </div>
</template>