<template>
  <div class="panel">
    <div class="section-title">{{ t('byBackend') }}</div>
    <div class="backend-list" v-if="items.length">
      <div class="backend-bar" v-for="b in bars" :key="b.url" @click="$emit('select', b.url)">
        <span class="backend-dot" :class="b.dot" :title="b.dotTitle"></span>
        <div class="backend-id">
          <div class="backend-line">
            <span class="backend-name" :title="b.url">{{ b.name }}</span>
            <span
              v-for="tag in b.tags"
              :key="tag"
              class="backend-tag"
              :title="'路由池: ' + tag"
            >{{ tag }}</span>
          </div>
          <div class="backend-model" v-if="b.model" :title="b.model">{{ b.model }}</div>
        </div>

        <div class="backend-load">
          <div class="load-track">
            <!-- 成功率细条：长度 = 100% − 错误率；颜色 = 错误率分级（蓝/琥珀/红） -->
            <div class="load-fill" :class="'fill-' + b.barTone" :style="{ width: b.successPct + '%' }"></div>
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
  fmtNum, fmtCompact, fmtPercent, fmtDuration, errRateTone,
  shortenBackendUrl as shorten,
} from '../utils'

const props = defineProps({
  items: { type: Array, default: () => [] },
  windowSec: { type: Number, default: 3600 },
})
defineEmits(['select'])

const bars = computed(() => {
  const list = props.items || []
  const secs = props.windowSec || 1
  return list.map((b) => {
    const requests = b.requests || 0
    const errorRate = b.error_rate || 0
    // 用 completion_tokens（输出 token）计算，与“生成速度”口径一致；
    // total_tokens 里 prompt 占大头（可达 98%），会把数值虚高成 prompt 处理速率
    const tps = secs > 0 ? (b.completion_tokens || 0) / secs : 0
    // 成功率细条：100% − 错误率（不再用请求数比例，降低大面积色块噪声）
    const successPct = Math.max(0, Math.round((1 - errorRate) * 10000) / 100)
    // 状态分级：<1% 正常 / 1–5% 警告 / >5% 异常，与错误率文字颜色同一套语义
    const barTone = errorRate < 0.01 ? 'ok' : errorRate <= 0.05 ? 'warn' : 'bad'
    const dot = errorRate < 0.01 ? 'dot-ok' : errorRate <= 0.05 ? 'dot-warn' : 'dot-bad'
    const dotTitle = errorRate > 0
      ? t('backendErrTip', { n: fmtPercent(errorRate) })
      : t('backendHealthy')
    const errTone = errRateTone(errorRate)
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
      barTone,
      successPct,
    }
  })
})
</script>
