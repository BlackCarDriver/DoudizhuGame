import { defineConfig } from 'vite'

// 开发时：npm run dev → http://127.0.0.1:5173
// 把 /ws、/api、/exchange 代理到本机 Go 服务（默认 :8090）。
// 构建时：base 用相对路径，部署在 /doudizhu/ 子路径下也能正确加载资源。
export default defineConfig({
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2020',
  },
  server: {
    port: 5173,
    host: '127.0.0.1',
    proxy: {
      '/ws': { target: 'ws://127.0.0.1:8090', ws: true },
      '/api': { target: 'http://127.0.0.1:8090', changeOrigin: true },
      '/exchange': { target: 'http://127.0.0.1:8090', changeOrigin: true },
    },
  },
})
