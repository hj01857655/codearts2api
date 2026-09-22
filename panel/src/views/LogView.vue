<script setup lang="ts">
import { computed, ref } from "vue";
import { usePanelStore } from "../stores/panel";

const store = usePanelStore();
const filter = ref<"all" | "op" | "err">("all");
const rows = computed(() =>
  store.log.filter((l) => filter.value === "all" || l.ch === filter.value).slice().reverse());
function setFilter(f: "all" | "op" | "err") { filter.value = f; }
</script>

<template>
  <section class="view">
    <div class="box">
      <header>
        <h3>操作记录 <span class="hint">本页发出的动作与结果</span></h3>
        <span class="grow" />
        <span id="logChips" class="chips">
          <button class="xs chip" :class="{ on: filter === 'all' }" @click="setFilter('all')">全部</button>
          <button class="xs chip" :class="{ on: filter === 'op' }" @click="setFilter('op')">操作</button>
          <button class="xs chip" :class="{ on: filter === 'err' }" @click="setFilter('err')">错误</button>
        </span>
        <button class="xs" @click="store.log = []">清空</button>
      </header>
      <pre id="logBox"><span v-if="!rows.length" class="ln">暂无记录</span><span v-for="(l, i) in rows" :key="i" class="ln" :class="{ e: l.ch === 'err' }"><span class="ts">{{ l.t.toLocaleTimeString() }}</span>{{ l.text }}</span></pre>
    </div>
  </section>
</template>
