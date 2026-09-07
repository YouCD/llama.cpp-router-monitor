import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: '/_monitor/ui/',
  plugins: [vue()],
  build: {
    outDir: '../web',
    emptyOutDir: true,
    assetsDir: 'assets',
  },
  server: {
    port: 5173,
    proxy: {
      '/_monitor': {
        target: 'http://localhost:8000',
        changeOrigin: true,
      },
    },
  },
})
