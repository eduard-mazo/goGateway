<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import type { TSDBBackendStatus } from '@/api'
import SparkLine from './SparkLine.vue'

const props = defineProps<{
  backend: TSDBBackendStatus
  retryDepth: number
}>()

const history = ref<number[]>([])
watch(
  () => props.backend.writeRate,
  v => {
    history.value.push(v)
    if (history.value.length > 40) history.value.shift()
  },
)

const sparkColor = computed(() =>
  props.backend.circuitOpen ? '#f44336' : props.backend.healthy ? '#4caf50' : '#ff9800',
)

function fmtBytes(b: number) {
  if (b > 1e9) return `${(b / 1e9).toFixed(1)} GB`
  if (b > 1e6) return `${(b / 1e6).toFixed(1)} MB`
  return `${(b / 1e3).toFixed(1)} KB`
}
</script>

<template>
  <div
    class="rounded-md border bg-card text-card-foreground p-4 flex flex-col gap-3"
    :class="backend.circuitOpen ? 'border-destructive bg-destructive/5' : 'border-border'"
  >
    <!-- Header -->
    <div class="flex items-center gap-2 min-w-0">
      <span
        class="shrink-0 inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide"
        :class="backend.type === 'victoriametrics'
          ? 'bg-blue-700 text-white'
          : 'bg-emerald-700 text-white'"
      >{{ backend.type }}</span>
      <span
        class="shrink-0 h-2 w-2 rounded-full"
        :class="backend.healthy ? 'bg-green-500' : 'bg-red-500'"
      />
      <span class="truncate text-sm font-medium">{{ backend.name }}</span>
      <span
        v-if="backend.circuitOpen"
        class="ml-auto shrink-0 text-[10px] font-bold text-destructive uppercase"
      >Circuit Open</span>
    </div>

    <!-- Metrics grid -->
    <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
      <div class="flex justify-between">
        <dt class="text-muted-foreground">Write</dt>
        <dd class="font-mono tabular-nums">{{ backend.writeRate.toFixed(1) }}/s</dd>
      </div>
      <div class="flex justify-between">
        <dt class="text-muted-foreground">Errors</dt>
        <dd
          class="font-mono tabular-nums"
          :class="backend.errorRate > 0 ? 'text-destructive' : ''"
        >{{ backend.errorRate }}</dd>
      </div>
      <div class="flex justify-between">
        <dt class="text-muted-foreground">Retry queue</dt>
        <dd
          class="font-mono tabular-nums"
          :class="retryDepth > 100 ? 'text-orange-400' : ''"
        >{{ retryDepth }}</dd>
      </div>
      <div class="flex justify-between">
        <dt class="text-muted-foreground">Sent</dt>
        <dd class="font-mono tabular-nums">{{ fmtBytes(backend.bytesSent) }}</dd>
      </div>
    </dl>

    <!-- Last error -->
    <p
      v-if="backend.lastError"
      class="truncate text-[11px] text-orange-400 font-mono"
      :title="backend.lastError"
    >{{ backend.lastError }}</p>

    <!-- Sparkline -->
    <SparkLine :data="history" :color="sparkColor" />
  </div>
</template>
