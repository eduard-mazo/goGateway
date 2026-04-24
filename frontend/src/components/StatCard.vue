<script setup lang="ts">
defineProps<{
  label: string
  value: string | number
  hint?: string
  accent?: boolean
  icon?: unknown
}>()
</script>

<template>
  <div
    class="group relative overflow-hidden card-soft transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg"
    :class="accent ? 'ring-2 ring-offset-2 ring-offset-background' : ''"
    :style="accent ? { '--tw-ring-color': 'var(--epm-citrico)' } : {}"
  >
    <!-- Accent strip top: citrico → bosque gradient -->
    <div
      v-if="accent"
      class="absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-[color:var(--epm-citrico)] to-[color:var(--epm-bosque)]"
    />

    <div class="px-5 py-5">
      <div class="flex items-center justify-between">
        <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">
          {{ label }}
        </div>
        <component
          v-if="icon"
          :is="icon"
          class="h-4 w-4 text-[color:var(--epm-bosque)] opacity-70"
        />
      </div>
      <div
        class="mt-3 font-sans text-[34px] font-black tabular leading-none tracking-tight"
        :class="accent ? 'text-[color:var(--epm-bosque)]' : 'text-foreground'"
      >
        {{ value }}
      </div>
      <div v-if="hint" class="mt-2 text-xs text-muted-foreground">{{ hint }}</div>
    </div>

    <!-- Soft decorative blob bottom-right -->
    <div
      class="pointer-events-none absolute -bottom-8 -right-8 h-24 w-24 rounded-sm opacity-0 group-hover:opacity-20 transition-opacity"
      :style="{ background: accent ? 'var(--epm-citrico)' : 'var(--epm-gris)' }"
    />
  </div>
</template>
