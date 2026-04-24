<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  state?: 'ok' | 'warn' | 'fault' | 'idle'
  label?: string
  value?: string
  pulse?: boolean
}>(), { state: 'idle', pulse: true })

const tone = computed(() => {
  switch (props.state) {
    case 'ok': return 'text-[color:var(--signal-ok)]'
    case 'warn': return 'text-[color:var(--signal-warn)]'
    case 'fault': return 'text-[color:var(--signal-fault)]'
    default: return 'text-muted-foreground'
  }
})

const bgTint = computed(() => {
  switch (props.state) {
    case 'ok': return 'bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)] border-[color:color-mix(in_srgb,var(--epm-bosque)_30%,transparent)]'
    case 'warn': return 'bg-[color:color-mix(in_srgb,var(--signal-warn)_15%,transparent)] border-[color:color-mix(in_srgb,var(--signal-warn)_35%,transparent)]'
    case 'fault': return 'bg-[color:color-mix(in_srgb,var(--signal-fault)_14%,transparent)] border-[color:color-mix(in_srgb,var(--signal-fault)_35%,transparent)]'
    default: return 'bg-card border-border'
  }
})
</script>

<template>
  <div
    class="inline-flex items-center gap-2 rounded-sm border px-3 py-1 backdrop-blur transition-colors"
    :class="bgTint"
  >
    <span :class="[tone]">
      <span :class="pulse && state === 'ok' ? 'status-dot' : 'status-dot-static'" />
    </span>
    <span
      v-if="label"
      class="text-[10px] uppercase tracking-[0.2em] font-bold"
      :class="state === 'idle' ? 'text-muted-foreground' : tone"
    >
      {{ label }}
    </span>
    <span v-if="value" class="font-mono text-xs text-foreground/90 truncate max-w-[220px]">{{ value }}</span>
  </div>
</template>
