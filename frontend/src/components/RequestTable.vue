<template>
  <div class="panel">
    <div class="section-title">{{ t('requestTape') }}</div>
    <el-table
      :data="rows"
      :highlight-current-row="true"
      size="small"
      height="560"
      :row-class-name="rowClassName"
      @row-click="onRowClick"
    >
      <el-table-column :label="t('colTime')" min-width="118">
        <template #default="{ row }">
          <div class="mono">{{ row._time }}</div>
          <div class="cell-subtle">{{ row._rel }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colRequest')" min-width="200">
        <template #default="{ row }">
          <div class="req-line">
            <span :class="'method-badge ' + methodClass(row.method)">{{ row.method || '-' }}</span>
            <span class="mono req-path" :title="reqTitle(row)">{{ row.path || '-' }}</span>
            <span class="cell-subtle req-ip">{{ row.client_ip || '' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colStatus')" width="76" align="center">
        <template #default="{ row }">
          <span :class="'status-tag ' + statusClass(row.status_code || 0)">
            {{ statusText(row) }}
          </span>
        </template>
      </el-table-column>
      <el-table-column :label="t('colModel')" min-width="140">
        <template #default="{ row }">
          <div class="model-cell">
            <span :title="row.model || ''" class="model-name">{{ row.model || '-' }}</span>
            <span v-if="row.is_streaming" class="stream-tag">{{ t('streaming') }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colLatency')" min-width="120" align="right">
        <template #default="{ row }">
          <span :class="latencyTone(row.first_byte_ms)">{{ fmtLatency(row.first_byte_ms, row.total_ms, t('live')) }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('colTokens')" min-width="100" align="right">
        <template #default="{ row }">
          <div class="mono">{{ fmtTokens(row.prompt_tokens, row.completion_tokens) }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('colCache')" min-width="76" align="right">
        <template #default="{ row }">
          <el-tooltip :content="cacheTip(row)" placement="top">
            <span :class="cacheTone(row.cache_hit_pct || 0)">
              <span class="cache-dot" :class="cacheDotClass(row)"></span>
              {{ fmtCache(row.cache_hit_pct || 0) }}
            </span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column :label="t('colPromptPerSec')" min-width="80" align="right">
        <template #default="{ row }">
          <span :class="rateTone(row.decode_tok_per_sec || 0)">{{ fmtRate(row.decode_tok_per_sec || 0) }}</span>
        </template>
      </el-table-column>
      <el-table-column width="84" fixed="right" align="center">
        <template #default="{ row }">
          <div class="row-actions">
            <el-tooltip :content="t('copyRequest')" placement="top">
              <el-button text :icon="CopyDocument" size="small" @click.stop="onCopy(row)" />
            </el-tooltip>
            <el-tooltip :content="t('viewDetail')" placement="top">
              <el-button text :icon="View" size="small" :disabled="!completed(row)" @click.stop="onRowClick(row)" />
            </el-tooltip>
          </div>
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
      <p class="empty-idle"><span class="live-dot live"></span>{{ t('emptyIdle') }}</p>
    </el-empty>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { CopyDocument, View } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { t } from '../i18n'
import {
  fmtTime, fmtNum, fmtRate, fmtPercent, fmtLatency, fmtTokens, shortQuery,
  isCompleted as completed, statusClass, latencyTone, rateTone, cacheTone, methodClass,
} from '../utils'

const props = defineProps({
  items: { type: Array, default: () => [] },
  hasMore: { type: Boolean, default: false },
  loadingMore: { type: Boolean, default: false },
  pageSize: { type: Number, default: 100 },
})
const emit = defineEmits(['loadMore', 'select'])

const BREAK_MS = 30 * 60 * 1000

const rows = computed(() => {
  const now = Date.now()
  let prevTs = null
  return props.items.map((it) => {
    const ts = new Date(it.created_at).getTime()
    let breakLine = false
    if (prevTs !== null && Number.isFinite(ts) && Number.isFinite(prevTs)) {
      breakLine = Math.abs(ts - prevTs) > BREAK_MS
    }
    prevTs = Number.isFinite(ts) ? ts : prevTs
    return {
      ...it,
      _time: fmtTime(it.created_at),
      _rel: fmtRelative(it.created_at, now),
      _break: breakLine,
    }
  })
})

function statusText(row) {
  return completed(row) ? String(row.status_code || 0) : t('live')
}
function fmtCache(v) {
  return `${Math.round(v || 0)}%`
}
function cacheTip(row) {
  return row.cached_prompt_tokens > 0
    ? t('cached', { n: fmtNum(row.cached_prompt_tokens) })
    : t('noCache')
}
function cacheDotClass(row) {
  return (row.cache_hit_pct || 0) > 0 ? 'hit' : 'miss'
}
function reqTitle(row) {
  return row.query ? `${row.path || ''}?${row.query}` : (row.path || '')
}
function fmtRelative(value, now) {
  const ts = new Date(value).getTime()
  if (!Number.isFinite(ts)) return ''
  const diff = Math.max(0, now - ts)
  const sec = Math.floor(diff / 1000)
  if (sec < 5) return t('relJustNow')
  if (sec < 60) return t('relSec', { n: sec })
  const min = Math.floor(sec / 60)
  if (min < 60) return t('relMin', { n: min })
  const h = Math.floor(min / 60)
  if (h < 24) return t('relHour', { n: h })
  return t('relDay', { n: Math.floor(h / 24) })
}
function rowClassName({ row }) {
  return row._break ? 'row-break' : ''
}
function onRowClick(row) {
  if (completed(row)) emit('select', row)
}
async function onCopy(row) {
  const q = row.query ? `?${row.query}` : ''
  try {
    await navigator.clipboard.writeText(`${row.method || ''} ${row.path || ''}${q}`)
    ElMessage.success(t('copied'))
  } catch {
    /* clipboard unavailable */
  }
}
</script>
