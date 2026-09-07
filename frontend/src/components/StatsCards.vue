<template>
  <div class="stats-grid">
    <div class="stat-card" v-for="c in cards" :key="c.key">
      <div class="label">{{ c.label }}</div>
      <div class="value">
        <AnimatedNumber :value="c.value" :format="c.format" />
      </div>
      <div class="foot">{{ c.foot }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import AnimatedNumber from './AnimatedNumber.vue'
import { t } from '../i18n'
import { fmtNum, fmtPercent, fmtMs } from '../utils'

const props = defineProps({
  stats: { type: Object, default: () => ({}) },
  hasFilters: { type: Boolean, default: false },
  outputSec: { type: Number, default: 0 },
})

const cards = computed(() => {
  const s = props.stats || {}
  const totalMatching = props.hasFilters
    ? (s.matching_total_requests || 0)
    : (s.lifetime_total_requests || 0)
  const totalTokens = props.hasFilters
    ? (s.matching_total_tokens || 0)
    : (s.lifetime_total_tokens || 0)
  return [
    { key: 'active', label: t('metricActive'), value: s.active_connections || 0, format: fmtNum, foot: t('metricInFlight') },
    { key: 'reqhour', label: t('metricReqHour'), value: Math.round((s.requests_per_minute || 0) * 60), format: fmtNum, foot: t('metricRolling1h') },
    { key: 'total', label: t('metricTotalReq'), value: totalMatching, format: fmtNum, foot: t('metricRetained') },
    { key: 'output', label: t('metricOutputSec'), value: props.outputSec, format: (v) => v.toFixed(1), foot: t('metricShownRows') },
    { key: 'tokens', label: t('metricTotalTok'), value: totalTokens, format: fmtNum, foot: t('metricRetained') },
    { key: 'ttft', label: t('metricAvgTtft'), value: s.avg_first_byte_ms || 0, format: fmtMs, foot: t('metricFirstToken') },
    { key: 'err', label: t('metricErrorRate'), value: (s.error_rate || 0) * 100, format: (v) => `${v.toFixed(2)}%`, foot: t('metric4xx') },
  ]
})
</script>
