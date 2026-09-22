<script setup lang="ts">
import { usePanelStore } from "../stores/panel";

const store = usePanelStore();
</script>

<template>
  <!-- 提示对视觉用户是浮层，对屏幕阅读器是状态播报：错误用 alert（打断式），
       成功用 status（礼貌式）。缺了它整块提示对读屏器等于不存在。 -->
  <div id="toasts" aria-live="polite">
    <div
      v-for="t in store.toasts"
      :key="t.id"
      class="tst"
      :class="t.ok ? 'ok' : 'err'"
      :role="t.ok ? 'status' : 'alert'"
    >{{ t.msg }}</div>
  </div>
</template>