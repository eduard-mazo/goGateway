<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  data: number[]
  color?: string
  height?: number
}>()

const color  = computed(() => props.color ?? '#4caf50')
const h      = computed(() => props.height ?? 32)
const W      = 120

const path = computed(() => {
  const pts = props.data
  if (pts.length < 2) return ''
  const max  = Math.max(...pts) || 1
  const step = W / (pts.length - 1)
  return pts
    .map((v, i) => {
      const x = i * step
      const y = h.value - (v / max) * h.value
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
})
</script>

<template>
  <svg
    v-if="path"
    :width="W"
    :height="h"
    class="block overflow-visible"
    aria-hidden="true"
  >
    <path
      :d="path"
      fill="none"
      :stroke="color"
      stroke-width="1.5"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
  </svg>
</template>
