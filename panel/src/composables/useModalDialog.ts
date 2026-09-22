// 模态对话框（.veil 遮罩 + .dlg 面板）的行为。
//
// 面板里有两类浮层，语义不同，不要混用：
//   对话框 —— .veil + .dlg：模态、居中，处理完才能继续（授权登录、确认更新）；
//   弹窗   —— .menu / .vdrop：锚定触发器的非模态浮层，点击外部即关。
// 后者的关闭路径放在各自组件里（AppShell 的菜单、VersionBadge 的下拉）。
// 对话框另外需要 ESC 关闭、背景滚动锁定与焦点移入，集中在这一点实现，
// 避免两个对话框各写一遍。
//
// 两个对话框可能同时开着（侧边栏的「确认更新」与账号池的「授权登录」），
// 因此不能简单地开时写 overflow、关时清空：后关的那个会把仍开着的对话框
// 一起解锁。滚动锁定用计数，ESC 只交给栈顶那一个。
import { onBeforeUnmount, onMounted, nextTick, watch, type Ref } from "vue";

// 打开中的对话框栈，后进先出；栈顶接收 ESC 与 Tab 圈定。
const stack: Array<{ close: () => void; panel: HTMLElement | null }> = [];
let locks = 0;
let listening = false;

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusable(el: HTMLElement): HTMLElement[] {
  return Array.from(el.querySelectorAll<HTMLElement>(FOCUSABLE))
    .filter((n) => n.offsetParent !== null || n === document.activeElement);
}

function onKeydown(e: KeyboardEvent) {
  if (stack.length === 0) return;
  const top = stack[stack.length - 1];
  if (e.key === "Escape") {
    // 捕获阶段拦下：ESC 属于最上层的对话框，不该同时触发下层的菜单/下拉关闭。
    e.stopPropagation();
    top.close();
    return;
  }
  // focus trap：Tab / Shift+Tab 在最顶层对话框的可聚焦元素里循环，出不去。
  if (e.key === "Tab" && top.panel) {
    const list = focusable(top.panel);
    if (list.length === 0) return;
    const first = list[0];
    const last = list[list.length - 1];
    const active = document.activeElement;
    const inside = top.panel.contains(active);
    if (e.shiftKey && (active === first || !inside)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && (active === last || !inside)) {
      e.preventDefault();
      first.focus();
    }
  }
}

function refreshListener() {
  const want = stack.length > 0;
  if (want === listening) return;
  listening = want;
  if (want) document.addEventListener("keydown", onKeydown, true);
  else document.removeEventListener("keydown", onKeydown, true);
}

function acquire() {
  locks++;
  if (locks === 1) document.body.style.overflow = "hidden";
}

function release() {
  if (--locks > 0) return;
  locks = 0;
  document.body.style.overflow = "";
}

/**
 * @param isOpen 对话框是否打开（组件常驻时用它变化，v-if 挂载的传 () => true）
 * @param close  关闭回调（ESC 与清理都走它）
 * @param panel  对话框面板元素；传入则打开时把焦点移进去，键盘用户不必从背景开始 Tab
 */
export function useModalDialog(
  isOpen: () => boolean,
  close: () => void,
  panel?: Ref<HTMLElement | null>,
): void {
  let opened = false;
  let prevFocus: HTMLElement | null = null;

  function sync() {
    if (isOpen() === opened) return;
    opened = !opened;
    if (opened) {
      prevFocus = document.activeElement as HTMLElement | null;
      stack.push({ close, panel: panel?.value ?? null });
      acquire();
      void nextTick(() => panel?.value?.focus());
    } else {
      drop();
    }
    refreshListener();
  }

  function drop() {
    const i = stack.findIndex((s) => s.close === close);
    if (i >= 0) stack.splice(i, 1);
    release();
    // 焦点归还给打开前的触发元素（触发弹窗的按钮），键盘用户回到原链路。
    if (prevFocus) { prevFocus.focus(); prevFocus = null; }
  }

  onMounted(sync);
  watch(isOpen, sync);
  onBeforeUnmount(() => {
    if (!opened) return;
    opened = false;
    drop();
    refreshListener();
  });
}
