<template>
  <div class="stats-groups">
    <div class="stats-grid">
      <div
        class="stat-card"
        :class="{ featured: c.featured, [c.tone]: !!c.tone }"
        v-for="c in allCards"
        :key="c.key"
      >
        <div class="label">{{ c.label }}</div>
        <div class="value">
          <AnimatedNumber :value="c.value" :format="c.format" :step="c.step || 0" />
        </div>
        <div class="foot" :title="c.foot">{{ c.foot }}</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import AnimatedNumber from './AnimatedNumber.vue'
import { t } from '../i18n'
import { fmtNum, fmtMs, fmtPctNum, errRateTone } from '../utils'

const props = defineProps({
  stats: { type: Object, default: () => ({}) },
  llmStats: { type: Object, default: () => ({}) },
  hasFilters: { type: Boolean, default: false },
  outputSec: { type: Number, default: 0 },
  hours: { type: Number, default: 1 },
})

const groups = computed(() => {
  const s = props.stats || {}
  const llm = props.llmStats || {}
  const hours = props.hours || 1
  const windowLabel = t('metricRecentHours', { n: hours })
  const totalMatching = props.hasFilters
    ? (s.matching_total_requests || 0)
    : (s.lifetime_total_requests || 0)
  const totalTokens = props.hasFilters
    ? (s.matching_total_tokens || 0)
    : (s.lifetime_total_tokens || 0)

  const llmError = (llm.error_rate || 0) * 100
  const totalError = (s.error_rate || 0) * 100
  const llmTone = errRateTone(llm.error_rate || 0)
  const errFoot = totalError > 0 && Math.abs(totalError - llmError) > 0.001
    ? t('metricErrProbeTip', { pct: fmtPctNum(totalError) })
    : windowLabel

  return [
    {
      title: t('groupFlow'),
      cards: [
        { key: 'active', label: t('metricActive'), value: s.active_connections || 0, format: fmtNum, foot: t('metricInFlight'), step: 1 },
        { key: 'reqhour', label: t('metricReqHour'), value: Math.round((s.requests_per_minute || 0) * 60), format: fmtNum, foot: windowLabel, step: 1 },
        { key: 'output', label: t('metricOutputSec'), value: props.outputSec, format: (v) => Math.round(v).toString(), foot: t('metricShownRows'), featured: true },
      ],
    },
    {
      title: t('groupQuality'),
      cards: [
        { key: 'ttft', label: t('metricAvgTtft'), value: s.avg_first_byte_ms || 0, format: fmtMs, foot: t('metricFirstToken'), featured: true },
        { key: 'err', label: t('metricErrorRateLLM'), value: llmError, format: fmtPctNum, tone: llmTone, foot: errFoot },
      ],
    },
    {
      title: t('groupResource'),
      cards: [
        { key: 'total', label: t('metricTotalReq'), value: totalMatching, format: fmtNum, foot: windowLabel, step: 1 },
        { key: 'tokens', label: t('metricTotalTok'), value: totalTokens, format: fmtNum, foot: windowLabel, step: 1 },
      ],
    },
  ]
})

const allCards = computed(() => groups.value.flatMap(g => g.cards))
</script>

<style scoped>
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
}
</style>
