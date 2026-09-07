<template>
  <div class="panel">
    <div class="section-title">{{ t('byBackend') }}</div>
    <div class="backend-grid" v-if="items.length">
      <div class="backend-card" v-for="b in items" :key="b.backend_url" @click="$emit('select', b.backend_url)">
        <div class="head">
          <span class="name" :title="b.backend_url">{{ shorten(b.backend_url) }}</span>
          <span class="err-rate" :class="errClass(b.error_rate)">{{ fmtPercent(b.error_rate || 0) }}</span>
        </div>
        <div class="metrics">
          <div class="metric"><span>{{ t('rows') }}</span><strong>{{ fmtNum(b.requests || 0) }}</strong></div>
          <div class="metric"><span>{{ t('colTotalTok') }}</span><strong>{{ fmtNum(b.total_tokens || 0) }}</strong></div>
          <div class="metric"><span>{{ t('colTtft') }}</span><strong>{{ fmtMs(b.avg_first_byte_ms || 0) }}</strong></div>
          <div class="metric"><span>{{ t('colTotal') }}</span><strong>{{ fmtMs(b.avg_total_ms || 0) }}</strong></div>
        </div>
      </div>
    </div>
    <div class="empty-backend" v-else>{{ t('noBackendData') }}</div>
  </div>
</template>

<script setup>
import { t } from '../i18n'
import { fmtNum, fmtPercent, fmtMs, shortenBackendUrl as shorten } from '../utils'

defineProps({
  items: { type: Array, default: () => [] },
})
defineEmits(['select'])

function errClass(rate) {
  if (rate >= 0.5) return 'tone-critical'
  if (rate > 0) return 'tone-hot'
  return 'tone-good'
}
</script>
