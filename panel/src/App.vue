<script setup lang="ts">
import { usePanelStore } from "./stores/panel";
import ToastHost from "./components/ToastHost.vue";
import LoginView from "./views/LoginView.vue";
import AppShell from "./views/AppShell.vue";
import { config } from "./config";

const store = usePanelStore();

// 启动时若有已存密钥则直接尝试加载（经 hub 反代时 hub 注入的 api_key 也在
// localStorage 流程里）；失败回登录表单。
void store.login(localStorage.getItem("codearts2api_panel_api_key") || "");
document.title = config.serviceTitle + " · 控制台";
</script>

<template>
  <ToastHost />
  <LoginView v-if="!store.authed" />
  <AppShell v-else />
</template>
