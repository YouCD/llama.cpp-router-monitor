import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

const root = process.cwd()

export default defineConfig(({ command, mode }) => {
  const isDev = command === 'serve'
  const env = loadEnv(mode, root, '')
  const apiTarget = env.VITE_APP_API_URL || 'http://localhost:9091'

  return {
    envPrefix: 'VITE_',
    base: isDev ? '/' : '/_proxy/ui/',
    define: {
      'API_PROXY': JSON.stringify('/_proxy/'),
    },
    plugins: [
      vue(),
      AutoImport({
        resolvers: [ElementPlusResolver()],
        dts: false,
      }),
      Components({
        resolvers: [ElementPlusResolver()],
        dts: false,
      }),
    ],
    build: {
      outDir: '../web',
      emptyOutDir: true,
      assetsDir: 'assets',
      rollupOptions: {
        output: {
          manualChunks: {
            echarts: ['echarts/core', 'echarts/charts', 'echarts/components', 'echarts/renderers'],
          },
        },
      },
    },
    server: {
      port: 5173,
      proxy: {
        '/_proxy': {
          target: apiTarget,
          changeOrigin: true,
          proxyTimeout: 60000,
          pathRewrite: {
            '^/_proxy': '/_proxy',
          },
        },
      },
    },
  }
})
