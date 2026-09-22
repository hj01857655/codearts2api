<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { usePanelStore } from "../stores/panel";

const store = usePanelStore();

const form = reactive({
  watch: {
    enabled: true,
    poll_minutes: 30,
    refresh_skew_minutes: 30,
    keepalive_interval_minutes: 15,
  },
  keepalive_window: "10m",
  max_concurrent: 1,
  benefit_auto_claim: true,
  update_proxy: "",
});

const saved = ref("");
const dirty = computed(() => JSON.stringify(form) !== saved.value);

function fillForm(cfg: any) {
  form.watch.enabled = !!cfg.watch?.enabled;
  form.watch.poll_minutes = cfg.watch?.poll_minutes ?? 30;
  form.watch.refresh_skew_minutes = cfg.watch?.refresh_skew_minutes ?? 30;
  form.watch.keepalive_interval_minutes = cfg.watch?.keepalive_interval_minutes ?? 15;
  form.keepalive_window = cfg.keepalive_window || "10m";
  form.max_concurrent = cfg.max_concurrent ?? 1;
  form.benefit_auto_claim = cfg.benefit_auto_claim !== false;
  form.update_proxy = cfg.update_proxy || "";
  saved.value = JSON.stringify(form);
}

async function load() {
  try {
    fillForm(await store.fetchConfig());
  } catch (e: any) {
    store.toast("加载配置失败: " + (e?.message || e), false);
  }
}

async function save() {
  try {
    const res = await store.saveConfig(JSON.parse(JSON.stringify(form)));
    store.toast(res?.message || "已保存", true);
    saved.value = JSON.stringify(form);
  } catch (e: any) {
    store.toast("保存失败: " + (e?.message || e), false);
  }
}

onMounted(load);
</script>

<template>
  <section class="view">
    <div class="box">
      <header>
        <h3>后台巡检与保活</h3>
        <span class="grow" />
        <button class="xs" :disabled="!dirty" @click="load">还原</button>
        <button class="xs" :disabled="!dirty" @click="save">保存</button>
      </header>
      <div class="pad">
        <label class="fld"><span>后台巡检</span>
          <input type="checkbox" v-model="form.watch.enabled" />
        </label>
        <label class="fld"><span>轮询间隔（分钟）</span>
          <input type="number" min="1" v-model.number="form.watch.poll_minutes" />
        </label>
        <label class="fld"><span>提前刷新窗口（分钟）</span>
          <input type="number" min="1" v-model.number="form.watch.refresh_skew_minutes" />
        </label>
        <label class="fld"><span>保活间隔（分钟）</span>
          <input type="number" min="1" v-model.number="form.watch.keepalive_interval_minutes" />
        </label>
        <label class="fld"><span>保活窗口（如 10m / 1h30m）</span>
          <input type="text" v-model="form.keepalive_window" />
        </label>
      </div>
    </div>
    <div class="box">
      <header><h3>其他配置</h3></header>
      <div class="pad">
        <label class="fld"><span>单账号最大并发</span>
          <input type="number" min="1" v-model.number="form.max_concurrent" />
        </label>
        <label class="fld"><span>福利自动领取</span>
          <input type="checkbox" v-model="form.benefit_auto_claim" />
        </label>
        <label class="fld"><span>更新代理（http/https/socks5，留空直连）</span>
          <input type="text" v-model="form.update_proxy" placeholder="http://127.0.0.1:7890" />
        </label>
        <p class="muted">巡检与并发参数重启服务后生效；福利自动领取即时生效。</p>
      </div>
    </div>
  </section>
</template>
