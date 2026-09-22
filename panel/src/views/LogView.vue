<script setup lang="ts">
import { computed, ref } from "vue";
import { usePanelStore } from "../stores/panel";
import ConfirmDialog from "../components/ConfirmDialog.vue";

const store = usePanelStore();
const filter = ref<"all" | "op" | "err">("all");
const rows = computed(() =>
  store.log.filter((l) => filter.value === "all" || l.ch === filter.value).slice().reverse());
function setFilter(f: "all" | "op" | "err") { filter.value = f; }

// 清空不可撤销（记录无法找回），故走确认；与账号禁用、全部领取同一套判定。
const confirming = ref(false);
function doClear() { store.clearLog(); confirming.value = false; }
</script>

<template>
  <section class="view">
    <div class="box">
      <header>
        <h3>操作记录 <span class="hint">本页发出的动作与结果</span></h3>
        <span class="grow" />
        <span class="chips log-chips">
          <button class="xs chip" :class="{ on: filter === 'all' }" @click="setFilter('all')">全部</button>
          <button class="xs chip" :class="{ on: filter === 'op' }" @click="setFilter('op')">操作</button>
          <button class="xs chip" :class="{ on: filter === 'err' }" @click="setFilter('err')">错误</button>
        </span>
        <button class="xs" @click="confirming = true">清空</button>
      </header>
      <pre class="log-box"><span v-if="!rows.length" class="ln">暂无记录</span><span v-for="(l, i) in rows" :key="i" class="ln" :class="{ e: l.ch === 'err' }"><span class="ts">{{ l.t.toLocaleTimeString() }}</span>{{ l.text }}</span></pre>
    </div>

    <ConfirmDialog
      v-if="confirming"
      title="清空操作记录"
      message="本页记录将被清空且无法恢复。此操作只影响本地浏览记录，不改动服务端。"
      confirm-label="清空"
      danger
      @confirm="doClear"
      @cancel="confirming = false"
    />
  </section>
</template>
