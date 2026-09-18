<template>
  <div class="panel">
    <div class="section-title">{{ t('byBackend') }}</div>
    <div class="backend-list" v-if="items.length">
      <div class="backend-bar" v-for="b in bars" :key="b.url" @click="$emit('select', b.url)">
        <span class="backend-dot" :class="b.dot" :title="b.dotTitle"></span>
        <div class="backend-id">
          <span class="backend-name" :title="b.url">{{ b.name }}</span>
          <div class="backend-meta" v-if="b.model || b.tags.length">
            <span class="backend-model" v-if="b.model" :title="b.model">{{ b.model }}</span>
            <span class="backend-tag" v-for="tag in b.tags" :key="tag" :title="'路由池: ' + tag">{{ tag }}</span>
          </div>
        </div>

        <div class="backend-load">
          <div class="load-track">
            <div class="load-fill" :class="b.loadTone" :style="{ width: b.loadPct + '%' }"></div>
          </div>
        </div>

        <span class="load-label">{{ t('backendRequests') }} {{ fmtNum(b.requests) }}</span>

        <div class="backend-metrics">
          <span class="metric-cell" :title="t('colTtft')"><strong class="mono">{{ fmtDuration(b.avgFirstByte) }}</strong> TTFT</span>
          <span class="metric-cell" :title="t('tokPerSec')"><strong class="mono">{{ b.tps.toFixed(1) }}</strong> Token/s</span>
          <span class="metric-cell" :title="fmtNum(b.totalTokens)"><strong class="mono">{{ fmtCompact(b.totalTokens) }}</strong> {{ t('metricTotalTokens') }}</span>
          <span class="metric-cell" :title="t('metricErrorRate')">
            <strong class="mono" :class="b.errTone">{{ fmtPercent(b.errorRate) }}</strong> {{ t('metricErrorRate') }}
          </span>
        </div>
      </div>
    </div>
    <div class="empty-backend" v-else>{{ t('noBackendData') }}</div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { t } from '../i18n'
import {
  fmtNum, fmtCompact, fmtPercent, fmtDuration,
  shortenBackendUrl as shorten,
} from '../utils'

const props = defineProps({
  items: { type: Array, default: () => [] },
  windowSec: { type: Number, default: 3600 },
})
defineEmits(['select'])

const bars = computed(() => {
  const list = props.items || []
  const maxReqs = Math.max(1, ...list.map((b) => b.requests || 0))
  const secs = props.windowSec || 1
  return list.map((b) => {
    const requests = b.requests || 0
    const errorRate = b.error_rate || 0
    // 用 completion_tokens（输出 token）计算，与“生成速度”口径一致；
    // total_tokens 里 prompt 占大头（可达 98%），会把数值虚高成 prompt 处理速率
    const tps = secs > 0 ? (b.completion_tokens || 0) / secs : 0
    const pct = Math.max(4, Math.round((requests / maxReqs) * 100))
    const dot = errorRate > 0 ? 'dot-warn' : 'dot-ok'
    const dotTitle = errorRate > 0
      ? t('backendErrTip', { n: fmtPercent(errorRate) })
      : t('backendHealthy')
    const errTone = errorRate > 0 ? 'tone-warm' : 'tone-good'
    let loadTone = 'fill-low'
    if (list.length > 1) {
      if (pct > 80) loadTone = 'fill-high'
      else if (pct > 50) loadTone = 'fill-mid'
    }
    return {
      url: b.backend_url,
      name: shorten(b.backend_url),
      model: b.model || '',
      tags: b.tags || [],
      requests,
      totalTokens: b.total_tokens || 0,
      errorRate,
      avgFirstByte: b.avg_first_byte_ms || 0,
      tps,
      dot,
      dotTitle,
      errTone,
      loadTone,
      loadPct: pct,
    }
  })
})
</script>
