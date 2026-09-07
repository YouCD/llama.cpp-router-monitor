import { createApp } from 'vue'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import './style.css'

document.documentElement.classList.add('dark')

const app = createApp(App)

app.mount('#app')
