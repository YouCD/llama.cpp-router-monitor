<template>
  <el-drawer :model-value="visible" :title="drawerTitle" size="520px" @update:model-value="v => $emit('update:visible', v)" @closed="reset">
    <div v-if="request">
      <el-descriptions :column="2" border size="small" style="margin-bottom: 16px">
        <el-descriptions-item :label="t('reqId')">{{ request.id || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('backend')">{{ request.backend_url || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('client')">{{ request.client_ip || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('dateTime')">{{ fmtDateTime(request.created_at) }}</el-descriptions-item>
        <el-descriptions-item :label="t('path')" :span="2">{{ request.method || '-' }} {{ request.path || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('query')" :span="2">{{ request.query || '-' }}</el-descriptions-item>
        <el-descriptions-item :label="t('error')" :span="2" v-if="request.error_text">
          <span class="tone-critical">{{ request.error_text }}</span>
        </el-descriptions-item>
      </el-descriptions>

      <div class="detail-metrics">
        <div class="mini-stat">
          <span>{{ t('colStatus') }}</span><strong :class="'status-tag ' + statusClass(request.status_code || 0)">{{ statusText }}</strong>
        </div>
        <div class="mini-stat"><span>{{ t('colTtft') }}</span><strong>{{ fmtMs(request.first_byte_ms) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colTotal') }}</span><strong>{{ fmtMs(request.total_ms) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colPromptPerSec') }}</span><strong>{{ fmtRate(request.prompt_tok_per_sec || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('metricOutputSec') }}</span><strong>{{ fmtRate(request.decode_tok_per_sec || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colPrompt') }}</span><strong>{{ fmtNum(request.prompt_tokens || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colCache') }}</span><strong>{{ fmtNum(request.cached_prompt_tokens || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colCompletion') }}</span><strong>{{ fmtNum(request.completion_tokens || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colTotalTok') }}</span><strong>{{ fmtNum(request.total_tokens || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colTotal') }} bytes</span><strong>{{ fmtBytes(request.request_bytes || 0) }}</strong></div>
        <div class="mini-stat"><span>{{ t('colPromptPerSec') }} (out)</span><strong>{{ fmtBytes(request.response_bytes || 0) }}</strong></div>
      </div>

      <el-tabs v-model="activeTab" style="margin-top: 8px">
        <el-tab-pane :label="t('tabRawReq')" name="request">
          <pre class="raw-view">{{ rawReq }}</pre>
        </el-tab-pane>
        <el-tab-pane :label="t('tabStructured')" name="structured">
          <pre class="raw-view">{{ rawResp }}</pre>
        </el-tab-pane>
        <el-tab-pane :label="t('tabRawResp')" name="response">
          <pre class="raw-view">{{ rawResp }}</pre>
        </el-tab-pane>
      </el-tabs>

      <div style="margin-top: 16px">
        <el-button type="danger" :disabled="!canDelete" @click="onDelete">{{ t('deleteRequest') }}</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { t } from '../i18n'
import { fetchRaw, deleteRequest } from '../api'
import {
  fmtMs, fmtNum, fmtBytes, fmtRate, fmtDateTime,
  isCompleted, statusClass, statusText as _st,
} from '../utils'

const props = defineProps({
  visible: { type: Boolean, default: false },
  request: { type: Object, default: null },
})
const emit = defineEmits(['update:visible', 'deleted'])

const activeTab = ref('request')
const rawReq = ref(t('noSelection'))
const rawResp = ref(t('noSelection'))

const statusText = computed(() => _st(props.request))
const canDelete = computed(() => isCompleted(props.request))

watch(
  () => [props.visible, props.request],
  async ([visible, req]) => {
    if (visible && req) {
      rawReq.value = t('loadingReqPayload')
      rawResp.value = t('loadingRespPayload')
      const [rr, rp] = await Promise.all([
        fetchRaw(req.id, 'request'),
        fetchRaw(req.id, 'response'),
      ])
      rawReq.value = rr || '-'
      rawResp.value = rp || '-'
    }
  },
  { immediate: true },
)

function reset() {
  activeTab.value = 'request'
  rawReq.value = t('noSelection')
  rawResp.value = t('noSelection')
}

async function onDelete() {
  const ok = await ElMessageBox.confirm(
    t('confirmDelete', { id: props.request.id }),
    t('deleteRequest'),
    { type: 'warning', confirmButtonText: t('deleteRequest'), cancelButtonText: t('close') },
  ).catch(() => false)
  if (!ok) return
  await deleteRequest(props.request.id)
  ElMessage.success('OK')
  emit('deleted')
  emit('update:visible', false)
}
</script>

<style scoped>
.detail-metrics {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-bottom: 8px;
}
.mini-stat {
  display: flex;
  flex-direction: column;
  background: var(--app-panel);
  border: 1px solid var(--app-line);
  border-radius: 6px;
  padding: 8px 10px;
}
.mini-stat span {
  color: var(--app-muted);
  font-size: 10px;
  text-transform: uppercase;
}
.mini-stat strong {
  font-size: 13px;
  margin-top: 2px;
  font-variant-numeric: tabular-nums;
}
.raw-view {
  background: #0d1117;
  border: 1px solid var(--app-line);
  border-radius: 6px;
  padding: 12px;
  max-height: 380px;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
