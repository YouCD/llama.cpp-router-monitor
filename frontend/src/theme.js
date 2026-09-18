import { ref } from 'vue'

export const THEME_KEY = 'llama-proxy.theme'

// 默认深色（与历史行为一致），用户选择后持久化
export const currentTheme = ref(localStorage.getItem(THEME_KEY) || 'dark')

export function applyTheme(theme) {
  currentTheme.value = theme === 'light' ? 'light' : 'dark'
  localStorage.setItem(THEME_KEY, currentTheme.value)
  document.documentElement.classList.toggle('dark', currentTheme.value === 'dark')
}

export function toggleTheme() {
  applyTheme(currentTheme.value === 'dark' ? 'light' : 'dark')
}
