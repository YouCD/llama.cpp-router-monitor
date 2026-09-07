<template>
  <div class="panel">
    <div class="section-title">{{ t('filters') }}</div>
    <div class="filter-bar">
      <el-input :model-value="f.q" :placeholder="t('filterSearch')" clearable @update:model-value="v => (f.q = v)" @keyup.enter="apply" />
      <el-input :model-value="f.path" :placeholder="t('filterPath')" clearable @update:model-value="v => (f.path = v)" @keyup.enter="apply" />
      <el-select :model-value="f.model" :placeholder="t('filterModel')" filterable clearable @update:model-value="v => (f.model = v)" style="width: 180px">
        <el-option v-for="m in models" :key="m" :label="m" :value="m" />
      </el-select>
      <el-select :model-value="f.backend" :placeholder="t('filterBackend')" filterable clearable @update:model-value="v => (f.backend = v)" style="width: 220px">
        <el-option v-for="b in backends" :key="b" :label="b" :value="b" />
      </el-select>
      <el-select :model-value="f.method" :placeholder="t('filterMethod')" clearable @update:model-value="v => (f.method = v)" style="width: 110px">
        <el-option v-for="m in ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']" :key="m" :label="m" :value="m" />
      </el-select>
      <el-input :model-value="f.status" placeholder="200" clearable @update:model-value="v => (f.status = v)" style="width: 90px" />
      <el-input :model-value="f.since" placeholder="24" clearable @update:model-value="v => (f.since = v)" style="width: 100px" />
      <el-select :model-value="f.stream" :placeholder="t('filterStream')" clearable @update:model-value="v => (f.stream = v)" style="width: 130px">
        <el-option :label="t('streaming')" value="true" />
        <el-option :label="t('nonStreaming')" value="false" />
      </el-select>
      <div class="filter-check">
        <el-checkbox :model-value="f.errors_only" @update:model-value="v => (f.errors_only = v)">{{ t('filterErrorsOnly') }}</el-checkbox>
        <el-checkbox :model-value="f.with_tokens" @update:model-value="v => (f.with_tokens = v)">{{ t('filterWithTokens') }}</el-checkbox>
        <el-checkbox :model-value="f.chat_only" @update:model-value="v => (f.chat_only = v)">{{ t('filterChatOnly') }}</el-checkbox>
      </div>
      <div>
        <el-button type="primary" @click="apply">{{ t('apply') }}</el-button>
        <el-button @click="reset">{{ t('reset') }}</el-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { reactive } from 'vue'
import { t } from '../i18n'

const props = defineProps({
  models: { type: Array, default: () => [] },
  backends: { type: Array, default: () => [] },
})
const emit = defineEmits(['apply', 'reset'])

const f = reactive({
  q: '',
  path: '',
  model: '',
  backend: '',
  method: '',
  status: '',
  since: '',
  stream: '',
  errors_only: false,
  with_tokens: false,
  chat_only: false,
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

function reset() {
  f.q = ''
  f.path = ''
  f.model = ''
  f.backend = ''
  f.method = ''
  f.status = ''
  f.since = ''
  f.stream = ''
  f.errors_only = false
  f.with_tokens = false
  f.chat_only = false
  emit('apply', {})
}

function setBackend(url) {
  f.backend = url
}

defineExpose({ collect, setBackend })
</script>
