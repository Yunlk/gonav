import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

// 构建产物直接落到 Go 的 embed 目录里，go build 时自动打包进二进制。
export default defineConfig({
  base: "/navkit/",
  plugins: [react()],
  build: {
    outDir: fileURLToPath(new URL("../internal/web/assets/navkit", import.meta.url)),
    emptyOutDir: true,
    sourcemap: false,
    target: "es2020",
    chunkSizeWarningLimit: 900,
  },
});
