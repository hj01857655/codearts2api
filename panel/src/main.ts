import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { setUnauthorizedHandler } from "./api";
import { usePanelStore } from "./stores/panel";
import "./assets/main.css";
import "./config";

// 401 统一处理：任何带 key 的请求返回 401 即清除本地密钥并回登录页，
// 各调用点无需再各自判断（见 api.ts）。
setUnauthorizedHandler(() => usePanelStore().unauthorized());

createApp(App).use(createPinia()).mount("#app");