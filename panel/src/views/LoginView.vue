<script setup lang="ts">
import { ref } from "vue";
import { usePanelStore } from "../stores/panel";
import { config } from "../config";

const store = usePanelStore();
const key = ref("");

function submit() { void store.login(key.value); }
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
          autocomplete="current-password" @keydown.enter="submit" />
        <span class="hint">密钥仅保存在本机浏览器 localStorage，请求以 Authorization: Bearer 发送。</span>
      </label>
      <button class="primary" style="width:100%" @click="submit">进入控制台</button>
      <div v-if="store.loginErr" class="state err" style="margin-top:12px">{{ store.loginErr }}</div>
    </div>
  </div>
</template>
