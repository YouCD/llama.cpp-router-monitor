<template>
  <div class="daily-chart">
    <div class="chart-section">
      <h3 class="chart-title">{{ t('dailyTokenTitle') }}</h3>
      <div ref="tokenEl" class="chart-box"></div>
    </div>
    <div class="chart-section">
      <h3 class="chart-title">{{ t('dailyStatusTitle') }}</h3>
      <div ref="statusEl" class="chart-box"></div>
    </div>
    <div class="chart-section">
      <h3 class="chart-title">{{ t('dailyRequestsTitle') }}</h3>
      <div ref="requestsEl" class="chart-box"></div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { t } from '../i18n'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps({
  items: { type: Array, default: () => [] },
})

const tokenEl = ref(null)
const statusEl = ref(null)
const requestsEl = ref(null)
let tokenChart = null
let statusChart = null
let requestsChart = null

const DATE_COLORS = {
  prompt: '#409eff',
  completion: '#67c23a',
  total: '#e6a23c',
}

const statusColor = (v) => {
  if (v <= 399) return '#67c23a'
  if (v < 500) return '#e6a23c'
  return '#f56c6c'
}

function render() {
  const items = props.items || []
  const dates = items.map((d) => d.date || '')
  const labels = {
    totalTokens: t('chartTotalTokens'),
    promptTokens: t('chartPromptTokens'),
    completionTokens: t('chartCompletionTokens'),
    ok: t('chartOk'),
    err4xx: t('chartErr4xx'),
    err5xx: t('chartErr5xx'),
    totalReq: t('chartTotalRequests'),
  }

  renderToken(dates, items, labels)
  renderStatus(dates, items, labels)
  renderRequests(dates, items, labels)
}

function renderToken(dates, items, labels) {
  if (!tokenChart) return
  tokenChart.setOption({
    tooltip: { trigger: 'axis' },
    legend: {
      data: [labels.promptTokens, labels.completionTokens, labels.totalTokens],
      bottom: 0,
      itemWidth: 14,
      itemHeight: 8,
      textStyle: { color: '#909399' },
    },
    grid: { left: 10, right: 10, top: 20, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: dates,
      boundaryGap: false,
      axisLabel: { color: '#909399' },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#909399' },
      splitLine: { lineStyle: { color: 'rgba(144,147,153,0.15)' } },
    },
    series: [
      {
        name: labels.totalTokens,
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: items.map((d) => d.total_tokens || 0),
        lineStyle: { width: 2, color: DATE_COLORS.total },
        itemStyle: { color: DATE_COLORS.total },
      },
      {
        name: labels.promptTokens,
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: items.map((d) => d.prompt_tokens || 0),
        lineStyle: { width: 2, color: DATE_COLORS.prompt },
        itemStyle: { color: DATE_COLORS.prompt },
      },
      {
        name: labels.completionTokens,
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: items.map((d) => d.completion_tokens || 0),
        lineStyle: { width: 2, color: DATE_COLORS.completion },
        itemStyle: { color: DATE_COLORS.completion },
      },
    ],
  })
}

function renderStatus(dates, items, labels) {
  if (!statusChart) return
  statusChart.setOption({
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: {
      data: [labels.ok, labels.err4xx, labels.err5xx],
      bottom: 0,
      itemWidth: 14,
      itemHeight: 8,
      textStyle: { color: '#909399' },
    },
    grid: { left: 10, right: 10, top: 20, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: dates,
      axisLabel: { color: '#909399' },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#909399' },
      splitLine: { lineStyle: { color: 'rgba(144,147,153,0.15)' } },
    },
    series: [
      {
        name: labels.ok,
        type: 'bar',
        stack: 'status',
        data: items.map((d) => d.ok_requests || 0),
        itemStyle: { color: statusColor(200) },
      },
      {
        name: labels.err4xx,
        type: 'bar',
        stack: 'status',
        data: items.map((d) => d.err4xx || 0),
        itemStyle: { color: statusColor(404) },
      },
      {
        name: labels.err5xx,
        type: 'bar',
        stack: 'status',
        data: items.map((d) => d.err5xx || 0),
        itemStyle: { color: statusColor(500) },
      },
    ],
  })
}

function renderRequests(dates, items, labels) {
  if (!requestsChart) return
  requestsChart.setOption({
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 10, right: 10, top: 20, bottom: 30, containLabel: true },
    xAxis: {
      type: 'category',
      data: dates,
      boundaryGap: false,
      axisLabel: { color: '#909399' },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#909399' },
      splitLine: { lineStyle: { color: 'rgba(144,147,153,0.15)' } },
    },
    series: [
      {
        name: labels.totalReq,
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: items.map((d) => d.total_requests || 0),
        lineStyle: { width: 2, color: DATE_COLORS.total },
        itemStyle: { color: DATE_COLORS.total },
      },
    ],
  })
}

function resizeAll() {
  tokenChart?.resize()
  statusChart?.resize()
  requestsChart?.resize()
}

watch(() => props.items, render, { deep: true })

onMounted(() => {
  tokenChart = echarts.init(tokenEl.value)
  statusChart = echarts.init(statusEl.value)
  requestsChart = echarts.init(requestsEl.value)
  window.addEventListener('resize', resizeAll)
  render()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeAll)
  tokenChart?.dispose()
  statusChart?.dispose()
  requestsChart?.dispose()
  tokenChart = null
  statusChart = null
  requestsChart = null
})
</script>

<style scoped>
.daily-chart {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-top: 4px;
}

@media (max-width: 900px) {
  .daily-chart {
    grid-template-columns: 1fr;
  }
}

.chart-section {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 10px;
  padding: 16px;
}

.chart-title {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.chart-box {
  width: 100%;
  height: 260px;
}
</style>