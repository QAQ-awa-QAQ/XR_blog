import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    // 生产环境由同容器内的 Nginx 托管静态文件
    assetsDir: 'assets',
    sourcemap: false,
  },
  server: {
    port: 5173,
    // 开发时把 /api 转发到本机 Go 服务
    proxy: {
      '/api': 'http://127.0.0.1:8081',
    },
  },
})
