import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { viteSingleFile } from "vite-plugin-singlefile";

// 面板构建产物必须是「单个 index.html」：Go 侧 //go:embed 整目录嵌入，
// servePanel 在运行时注入 __SERVICE_*__ 变量。viteSingleFile 把 JS/CSS
// 全部内联进 index.html，不产生外链资源（embed 之外无静态路由）。
export default defineConfig({
  plugins: [vue(), viteSingleFile({ removeViteModuleLoader: true })],
  build: {
    // outDir 相对项目根（即 panel/）解析，等价于原先的 resolve(__dirname, ...)，
    // 但不依赖 node:path 与 __dirname——那属于 Node 全局，会把这个配置文件挤出
    // 类型检查（缺 @types/node 时 __dirname 报错）。目标在根之外，必须显式
    // emptyOutDir，否则 Vite 拒绝清空而遗留旧产物。
    outDir: "../internal/server/dist",
    emptyOutDir: true,
  },
});
