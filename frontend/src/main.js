import { createApp } from 'vue'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import { applyTheme, currentTheme } from './theme'
import './style.css'

// 按用户选择（localStorage，默认 dark）应用主题，替代旧版无条件深色
applyTheme(currentTheme.value)

const app = createApp(App)

app.mount('#app')
