<template>
  <span>{{ format(display) }}</span>
</template>

<script setup>
import { ref, watch, onUnmounted } from 'vue'

const props = defineProps({
  value: { type: Number, default: 0 },
  duration: { type: Number, default: 600 },
  format: { type: Function, default: (v) => String(v) },
})

const display = ref(props.value)
let raf = null
let start = null

watch(
  () => props.value,
  (newVal) => {
    if (raf) cancelAnimationFrame(raf)
    const from = display.value
    start = null
    const step = (ts) => {
      if (start === null) start = ts
      const progress = Math.min((ts - start) / props.duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      display.value = from + (newVal - from) * eased
      if (progress < 1) {
        raf = requestAnimationFrame(step)
      } else {
        display.value = newVal
      }
    }
    raf = requestAnimationFrame(step)
  },
  { immediate: true },
)

onUnmounted(() => {
  if (raf) cancelAnimationFrame(raf)
})
</script>
