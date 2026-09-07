<template>
  <el-config-provider :locale="locale">
    <div class="app-shell">
      <header class="app-header">
        <div>
          <h1 class="title">{{ t('appTitle') }}</h1>
          <p class="subtitle">{{ t('heroText') }}</p>
        </div>
        <div class="header-actions">
          <span :class="'live-dot ' + liveState"></span>
          <span>{{ liveText }}</span>
          <span class="cell-subtle">{{ t('lastUpdate') }} {{ lastUpdated }}</span>
          <span style="display: inline-flex; align-items: center; gap: 6px">
            <el-switch v-model="autoRefresh" />
            <span class="cell-subtle">{{ t('autoRefresh') }}</span>
          </span>
          <el-button :icon="Refresh" @click="refreshNow" round>{{ t('refresh') }}</el-button>
          <el-button :icon="Switch" @click="toggleLang" round>
            {{ currentLang === 'zh' ? 'EN' : '中文' }}
          </el-button>
        </div>
      </header>

      <StatsCards :stats="stats" :has-filters="hasFilters" :output-sec="outputSec" />

      <BackendStats :items="backendStats" @select="onBackendSelect" />

      <FilterPanel ref="filterPanel" :models="models" :backends="backends" @apply="onApply" @reset="onApply" />

      <RequestTable
        :items="items"
        :has-more="hasMore"
        :loading-more="loadingMore"
        :page-size="pageSize"
        @load-more="loadMore"
        @select="openDetails"
      />

      <RequestDrawer
        v-model:visible="drawerVisible"
        :request="selectedRequest"
        @deleted="onDeleted"
      />
    </div>
  </el-config-provider>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { Refresh, Switch } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import StatsCards from './components/StatsCards.vue'
import BackendStats from './components/BackendStats.vue'
import FilterPanel from './components/FilterPanel.vue'
import RequestTable from './components/RequestTable.vue'
import RequestDrawer from './components/RequestDrawer.vue'
import { currentLang, elementLocale, t, toggleLang, setLang } from './i18n'
import {
  fetchStats, fetchRequests, fetchRequest, fetchModels, fetchBackends,
  fetchStatsByBackend, queryString,
} from './api'
import { fmtDate, isCompleted } from './utils'

const pageSize = 100
const locale = computed(() => elementLocale())

const filters = reactive({})
const stats = reactive({})
const items = ref([])
const backendStats = ref([])
const models = ref([])
const backends = ref([])
const outputSec = ref(0)
const hasMore = ref(false)
const loadingMore = ref(false)
const autoRefresh = ref(true)
const lastUpdated = ref('-')
const liveState = ref('retry')
const liveText = ref(t('connecting'))
const drawerVisible = ref(false)
const selectedRequest = ref(null)
const filterPanel = ref(null)

const hasFilters = computed(() => Object.keys(filters).length > 0)

let refreshTimer = null
let eventSource = null
let pollTimers = []

function setLive(mode, text) {
  liveState.value = mode
  liveText.value = text
}

function setFilters(f) {
  Object.keys(filters).forEach((k) => delete filters[k])
  Object.assign(filters, f)
}

async function loadStats() {
  try {
    const data = await fetchStats({ ...filters })
    Object.keys(stats).forEach((k) => delete stats[k])
    Object.assign(stats, data)
    lastUpdated.value = fmtDate(new Date().toISOString())
  } catch (err) {
    setLive('error', t('refreshFailed', { msg: err.message }))
  }
}

async function loadRequests() {
  loadingMore.value = false
  try {
    const data = await fetchRequests(items.value.length || pageSize, 0, { ...filters })
    items.value = data.items || []
    hasMore.value = (data.items || []).length >= pageSize
    const rates = items.value
      .map((it) => Number(it.decode_tok_per_sec || 0))
      .filter((v) => Number.isFinite(v) && v > 0)
    outputSec.value = rates.length ? rates.reduce((a, b) => a + b, 0) / rates.length : 0
  } catch (err) {
    setLive('error', t('refreshFailed', { msg: err.message }))
  }
}

async function loadMore() {
  if (loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const data = await fetchRequests(pageSize, items.value.length, { ...filters })
    items.value = items.value.concat(data.items || [])
    hasMore.value = (data.items || []).length >= pageSize
  } catch (err) {
    ElMessage.error(t('loadMoreFailed', { msg: err.message }))
  } finally {
    loadingMore.value = false
  }
}

async function loadBackendStats() {
  try {
    const hours = Number.parseInt(filters.since_hours || '1', 10) || 1
    backendStats.value = await fetchStatsByBackend(hours)
  } catch {
    /* ignore */
  }
}

async function loadOptions() {
  try {
    models.value = await fetchModels()
  } catch { /* ignore */ }
  try {
    backends.value = await fetchBackends()
  } catch { /* ignore */ }
}

async function refreshAll() {
  await Promise.all([loadStats(), loadRequests(), loadBackendStats()])
}

function onApply(f) {
  setFilters(f)
  refreshAll().catch(() => {})
}

function onBackendSelect(url) {
  if (filterPanel.value) {
    filterPanel.value.setBackend(url)
  }
  filters.backend = url
  refreshAll().catch(() => {})
}

async function openDetails(row) {
  try {
    selectedRequest.value = await fetchRequest(row.id)
  } catch {
    selectedRequest.value = row
  }
  drawerVisible.value = true
}

function onDeleted() {
  refreshAll().catch(() => {})
}

function scheduleEventRefresh() {
  if (!autoRefresh.value) return
  if (refreshTimer) clearTimeout(refreshTimer)
  refreshTimer = setTimeout(() => {
    refreshAll().catch(() => {})
  }, 180)
}

function connectEvents() {
  eventSource = new EventSource('/_monitor/events')
  eventSource.onopen = () => setLive('live', t('liveConnected'))
  eventSource.onerror = () => setLive('retry', t('reconnecting'))
  eventSource.addEventListener('request', scheduleEventRefresh)
}

function refreshNow() {
  refreshAll().catch((err) => setLive('error', t('refreshFailed', { msg: err.message })))
}

onMounted(async () => {
  setLang(currentLang.value)
  connectEvents()
  await refreshAll()
  await loadOptions()
  pollTimers = [
    setInterval(() => { if (autoRefresh.value) loadStats().catch(() => {}) }, 5000),
    setInterval(() => { if (autoRefresh.value) loadRequests().catch(() => {}) }, 9000),
    setInterval(() => { loadOptions().catch(() => {}) }, 30000),
  ]
})

onUnmounted(() => {
  if (eventSource) eventSource.close()
  if (refreshTimer) clearTimeout(refreshTimer)
  pollTimers.forEach((timer) => clearInterval(timer))
})
</script>
