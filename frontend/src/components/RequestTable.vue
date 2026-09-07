<template>
  <div class="panel">
    <div class="section-title">{{ t('requestTape') }}</div>
    <el-table
      :data="items"
      :highlight-current-row="true"
      size="small"
      height="560"
      @row-click="onRowClick"
    >
      <el-table-column :label="t('colTime')" min-width="110">
        <template #default="{ row }">
          <div class="mono">{{ fmtTime(row.created_at) }}</div>
          <div class="cell-subtle">{{ row.client_ip || t('unknownClient') }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colRequest')" min-width="220">
        <template #default="{ row }">
          <div>
            <span class="method-badge">{{ row.method || '-' }}</span>
            <span class="mono">{{ row.path || '-' }}</span>
          </div>
          <div class="cell-subtle">{{ row.query || t('noQuery') }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colStatus')" width="80" align="center">
        <template #default="{ row }">
          <span :class="'status-tag ' + statusClass(row.status_code || 0)">
            {{ statusText(row) }}
          </span>
        </template>
      </el-table-column>
      <el-table-column :label="t('colModel')" min-width="150">
        <template #default="{ row }">
          <div>{{ row.model || '-' }}</div>
          <div class="cell-subtle">{{ rowState(row) }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colTtft')" min-width="90" align="right">
        <template #default="{ row }">
          <span :class="latencyTone(row.first_byte_ms)">{{ fmtMs(row.first_byte_ms) }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('colTotal')" min-width="90" align="right">
        <template #default="{ row }">
          <div>{{ fmtMs(row.total_ms) }}</div>
          <div class="cell-subtle">{{ !completed(row) ? t('running') : t('chunks', { n: fmtNum(row.chunks_count || 0) }) }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colPrompt')" min-width="70" align="right">
        <template #default="{ row }">{{ fmtNum(row.prompt_tokens || 0) }}</template>
      </el-table-column>
      <el-table-column :label="t('colCompletion')" min-width="90" align="right">
        <template #default="{ row }">{{ fmtNum(row.completion_tokens || 0) }}</template>
      </el-table-column>
      <el-table-column :label="t('colTotalTok')" min-width="80" align="right">
        <template #default="{ row }">{{ fmtNum(row.total_tokens || 0) }}</template>
      </el-table-column>
      <el-table-column :label="t('colCache')" min-width="100" align="right">
        <template #default="{ row }">
          <span :class="cacheTone(row.cache_hit_pct || 0)">
            {{ fmtPct(row.cache_hit_pct || 0) }}
          </span>
          <div class="cell-subtle">
            {{ row.cached_prompt_tokens > 0 ? t('cached', { n: fmtNum(row.cached_prompt_tokens) }) : t('noCache') }}
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colPromptPerSec')" min-width="90" align="right">
        <template #default="{ row }">
          <span :class="rateTone(row.decode_tok_per_sec || 0)">
            {{ fmtRate(row.decode_tok_per_sec || 0) }}
          </span>
          <div class="cell-subtle">{{ t('promptPrefix', { n: fmtRate(row.prompt_tok_per_sec || 0) }) }}</div>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="hasMore" style="text-align: center; margin-top: 12px">
      <el-button :loading="loadingMore" @click="$emit('loadMore')">
        {{ loadingMore ? t('loading') : t('loadMore', { n: pageSize }) }}
      </el-button>
    </div>

    <el-empty v-if="items.length === 0" :description="t('emptyTitle')">
      <p>{{ t('emptyDesc') }}</p>
    </el-empty>
  </div>
</template>

<script setup>
import { t } from '../i18n'
import {
  fmtTime, fmtNum, fmtMs, fmtRate, fmtPercent,
  isCompleted as completed, statusClass, latencyTone, rateTone, cacheTone,
} from '../utils'

defineProps({
  items: { type: Array, default: () => [] },
  hasMore: { type: Boolean, default: false },
  loadingMore: { type: Boolean, default: false },
  pageSize: { type: Number, default: 100 },
})
const emit = defineEmits(['loadMore', 'select'])

function statusText(row) {
  return completed(row) ? String(row.status_code || 0) : t('live')
}
function fmtPct(v) {
  return fmtPercent(v / 100)
}
function rowState(row) {
  if (!completed(row)) return t('inProgress')
  return row.is_streaming ? t('streaming') : t('standard')
}
function onRowClick(row) {
  if (completed(row)) emit('select', row)
}
</script>
