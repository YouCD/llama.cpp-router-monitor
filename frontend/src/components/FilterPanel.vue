<template>
  <div class="panel">
    <div class="section-title">{{ t('filters') }}</div>

    <div class="quick-tags">
      <button
        v-for="tag in tags"
        :key="tag.key"
        class="quick-tag"
        :class="{ active: isActive(tag.key) }"
        @click="applyQuick(tag.key)"
      >
        <span class="quick-dot" :class="tag.dot"></span>
        {{ tag.label }}
        <span class="quick-count">{{ tag.count }}</span>
      </button>
    </div>

    <div class="filter-bar">
      <el-input
        :model-value="f.q"
        :placeholder="t('filterSearch')"
        clearable
        @update:model-value="v => (f.q = v)"
        @clear="reset"
        @keyup.enter="apply"
      />
      <el-input
        :model-value="f.status"
        :placeholder="t('filterStatus')"
        clearable
        @update:model-value="v => (f.status = v)"
        @keyup.enter="apply"
        style="width: 110px"
      />
      <el-input
        :model-value="f.since"
        clearable
        @update:model-value="v => (f.since = v)"
        @keyup.enter="apply"
        style="width: 96px"
      >
        <template #suffix><span class="since-unit">h</span></template>
      </el-input>
      <el-button
        text
        :icon="Filter"
        class="adv-toggle"
        :class="{ active: showAdv }"
        @click="showAdv = !showAdv"
      >
        {{ t('filterAdvanced') }}
      </el-button>
    </div>

    <div v-show="showAdv" class="filter-bar adv-panel">
      <el-input :model-value="f.path" :placeholder="t('filterPath')" clearable @update:model-value="v => (f.path = v)" @keyup.enter="apply" />
      <el-select :model-value="f.model" :placeholder="t('filterModel')" filterable clearable @update:model-value="v => (f.model = v)">
        <el-option v-for="m in models" :key="m" :label="m" :value="m" />
      </el-select>
      <el-select :model-value="f.backend" :placeholder="t('filterBackend')" filterable clearable @update:model-value="v => (f.backend = v)">
        <el-option v-for="b in backends" :key="b" :label="b" :value="b" />
      </el-select>
      <el-select :model-value="f.method" :placeholder="t('filterMethod')" clearable @update:model-value="v => (f.method = v)" style="width: 130px">
        <el-option v-for="m in ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']" :key="m" :label="m" :value="m" />
      </el-select>
      <el-select :model-value="f.stream" :placeholder="t('filterStream')" clearable @update:model-value="v => (f.stream = v)" style="width: 150px">
        <el-option :label="t('streaming')" value="true" />
        <el-option :label="t('nonStreaming')" value="false" />
      </el-select>
      <div class="filter-check">
        <el-checkbox :model-value="f.errors_only" @update:model-value="v => (f.errors_only = v)">{{ t('filterErrorsOnly') }}</el-checkbox>
        <el-checkbox :model-value="f.with_tokens" @update:model-value="v => (f.with_tokens = v)">{{ t('filterWithTokens') }}</el-checkbox>
        <el-checkbox :model-value="f.chat_only" @update:model-value="v => (f.chat_only = v)">{{ t('filterChatOnly') }}</el-checkbox>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, reactive, watch, onUnmounted } from 'vue'
import { Filter } from '@element-plus/icons-vue'
import { t } from '../i18n'

const props = defineProps({
  models: { type: Array, default: () => [] },
  backends: { type: Array, default: () => [] },
  quickCounts: { type: Object, default: () => ({ ok: 0, err4xx: 0, err5xx: 0, stream: 0 }) },
})
const emit = defineEmits(['apply'])

const f = reactive({
  q: '',
  path: '',
  model: '',
  backend: '',
  method: '',
  status: '',
  since: '1',
  stream: '',
  errors_only: false,
  with_tokens: false,
  chat_only: false,
})
const showAdv = ref(false)
const lastQuick = ref('all')

let debounceTimer = null
let suppressWatch = false

const tags = computed(() => [
  { key: 'all', label: t('quickAll'), dot: 'dot-all', count: '' },
  { key: 'ok', label: t('quickSuccess'), dot: 'dot-ok', count: props.quickCounts.ok },
  { key: '4xx', label: t('quick4xx'), dot: 'dot-warn', count: props.quickCounts.err4xx },
  { key: '5xx', label: t('quick5xx'), dot: 'dot-bad', count: props.quickCounts.err5xx },
  { key: 'stream', label: t('quickStreaming'), dot: 'dot-cyan', count: props.quickCounts.stream },
])

watch(f, () => {
  if (suppressWatch) {
    suppressWatch = false
    return
  }
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(apply, 300)
})

function collect() {
  const filters = {}
  if (f.q) filters.q = f.q.trim()
  if (f.path) filters.path = f.path.trim()
  if (f.model) filters.model = f.model
  if (f.backend) filters.backend = f.backend
  if (f.method) filters.method = f.method
  if (f.status) filters.status = f.status.trim()
  if (f.since) filters.since_hours = f.since.trim()
  if (f.stream !== '') filters.stream = f.stream
  if (f.errors_only) filters.errors_only = 'true'
  if (f.with_tokens) filters.with_tokens = 'true'
  if (f.chat_only) filters.chat_completions_only = 'true'
  return filters
}

function apply() {
  emit('apply', collect())
}

function clearFilters() {
  f.q = ''
  f.path = ''
  f.model = ''
  f.backend = ''
  f.method = ''
  f.status = ''
  f.since = '1'
  f.stream = ''
  f.errors_only = false
  f.with_tokens = false
  f.chat_only = false
}

function reset() {
  if (debounceTimer) clearTimeout(debounceTimer)
  suppressWatch = true
  clearFilters()
  lastQuick.value = 'all'
  emit('apply', {})
}

function applyQuick(kind) {
  if (debounceTimer) clearTimeout(debounceTimer)
  suppressWatch = true
  lastQuick.value = kind
  if (kind === 'all') {
    clearFilters()
    emit('apply', {})
  } else if (kind === 'ok') {
    f.status = '200'
    f.errors_only = false
    f.stream = ''
    emit('apply', collect())
  } else if (kind === '4xx' || kind === '5xx') {
    f.status = ''
    f.errors_only = true
    f.stream = ''
    emit('apply', collect())
  } else if (kind === 'stream') {
    f.status = ''
    f.errors_only = false
    f.stream = 'true'
    emit('apply', collect())
  }
}

function isActive(kind) {
  if (kind === 'all') return !f.status && !f.errors_only && f.stream === ''
  if (kind === 'ok') return f.status === '200'
  if (kind === '4xx') return f.errors_only && lastQuick.value === '4xx'
  if (kind === '5xx') return f.errors_only && lastQuick.value === '5xx'
  if (kind === 'stream') return f.stream === 'true'
  return false
}

function setBackend(url) {
  if (debounceTimer) clearTimeout(debounceTimer)
  suppressWatch = true
  f.backend = url
}

defineExpose({ collect, setBackend })

onUnmounted(() => {
  if (debounceTimer) clearTimeout(debounceTimer)
})
</script>
