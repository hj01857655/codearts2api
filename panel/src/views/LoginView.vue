<script setup lang="ts">
import { ref } from "vue";
import { usePanelStore } from "../stores/panel";
import { config } from "../config";

const store = usePanelStore();
const key = ref("");

// 登录是网络往返：无进行中状态时，慢网络下连点会并发发多次请求，
// loginErr 还会相互覆盖。与其它视图的 refreshing/checking 守卫同一约定。
const busy = ref(false);

async function submit() {
  if (busy.value) return;
  busy.value = true;
  try { await store.login(key.value); } finally { busy.value = false; }
}
</script>

<template>
  <div class="login">
    <div class="login-box">
      <div class="brand">
        <div class="logo">{{ config.logo }}</div>
        <div>
          <h1>{{ config.serviceTitle }}</h1>
          <p>管理控制台 · 使用 API Key 登录</p>
        </div>
      </div>
      <label class="fld"><span class="lb">API Key</span>
        <input v-model="key" type="password" placeholder="config.json 中的 api_key"
          autocomplete="current-password" :disabled="busy" @keydown.enter="submit" />
        <span class="hint">密钥仅保存在本机浏览器 localStorage，请求以 Authorization: Bearer 发送。</span>
      </label>
      <button class="primary" style="width:100%" :disabled="busy" @click="submit">
        {{ busy ? "验证中…" : "进入控制台" }}
      </button>
      <div v-if="store.loginErr" class="state err" role="alert" style="margin-top:12px">{{ store.loginErr }}</div>
    </div>
  </div>
</template>
