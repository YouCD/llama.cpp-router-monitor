<template>
  <CountUp :end-val="value" :start-val="from" :duration="durationSec" :options="options" />
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import CountUp from 'vue-countup-v3'

const props = defineProps({
  value: { type: Number, default: 0 },
  duration: { type: Number, default: 600 },
  step: { type: Number, default: 0 },
  format: { type: Function, default: (v) => String(v) },
})

const from = ref(props.value)

watch(
  () => props.value,
  (newVal, oldVal) => {
    from.value = oldVal
  },
)

const durationSec = computed(() => Math.max(props.duration / 1000, 0.001))
const options = computed(() => ({
  duration: durationSec.value,
  decimalPlaces: props.step > 0 ? 0 : 2,
  formattingFn: (n) => props.format(n),
}))
</script>
