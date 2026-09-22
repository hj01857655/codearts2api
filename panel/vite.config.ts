import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { viteSingleFile } from "vite-plugin-singlefile";
import { resolve } from "node:path";

// 面板构建产物必须是「单个 index.html」：Go 侧 //go:embed 整目录嵌入，
// servePanel 在运行时注入 __SERVICE_*__ 变量。viteSingleFile 把 JS/CSS
// 全部内联进 index.html，不产生外链资源（embed 之外无静态路由）。
export default defineConfig({
  plugins: [vue(), viteSingleFile({ removeViteModuleLoader: true })],
  build: {
    outDir: resolve(__dirname, "../internal/server/panel-dist"),
    emptyOutDir: true,
  },
});
