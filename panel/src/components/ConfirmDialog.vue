<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { useModalDialog } from "../composables/useModalDialog";

// 危险/不可逆操作的二次确认对话框。
//
// 与 VersionBadge 里那个更新确认不同，这是通用组件：账号禁用、批量领取这类
// 「点下去就改上游状态」的操作走它。判定标准是后果是否需要用户再操作一次才能
// 复原——只读或可轻易重来的操作不套确认，否则确认框会被点成条件反射而失效。
const props = withDefaults(
  defineProps<{
    title: string;
    message: string;
    confirmLabel?: string;
    cancelLabel?: string;
    danger?: boolean;
    busy?: boolean;
  }>(),
  { confirmLabel: "确定", cancelLabel: "取消", danger: false, busy: false },
);

const emit = defineEmits<{ confirm: []; cancel: [] }>();

const panel = ref<HTMLElement | null>(null);
const cancelBtn = ref<HTMLElement | null>(null);

// 组件由 v-if 挂载，恒为「已打开」；ESC 等同取消，执行中不可关闭。
useModalDialog(() => true, () => { if (!props.busy) emit("cancel"); }, panel);

// 焦点默认落在「取消」而非主操作：危险操作按回车应当是取消。
// useModalDialog 先把焦点给面板，这里在其后的 nextTick 里覆盖为取消按钮。
onMounted(() => { void nextTick(() => cancelBtn.value?.focus()); });
</script>

<template>
  <div class="veil on" @click.self="!busy && emit('cancel')">
    <div
      ref="panel"
      class="dlg"
      role="alertdialog"
      aria-modal="true"
      :aria-label="title"
      tabindex="-1"
      style="width:420px"
    >
      <header>
        <h3>{{ title }}</h3>
        <div class="hint">{{ message }}</div>
      </header>
      <footer>
        <button ref="cancelBtn" :disabled="busy" @click="emit('cancel')">{{ cancelLabel }}</button>
        <span class="grow" />
        <button :class="danger ? 'danger-solid' : 'primary'" :disabled="busy" @click="emit('confirm')">
          {{ busy ? "处理中…" : confirmLabel }}
        </button>
      </footer>
    </div>
  </div>
</template>
