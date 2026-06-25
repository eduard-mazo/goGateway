<script setup lang="ts">
// HwIdentity shows the device-reported hardware identity (ICR edges publish
// these as Device/* string metrics in node System telemetry; the gateway stores
// them on the node's equipo). Renders nothing when no field is populated.
import { computed } from 'vue'
import { Cpu } from 'lucide-vue-next'
import type { SSFVEquipo } from '@/api'

const props = defineProps<{ equipo: SSFVEquipo }>()

const fields = computed(() => ([
  { label: 'Part Number',  value: props.equipo.hw_part_number },
  { label: 'Product Type', value: props.equipo.hw_product_type },
  { label: 'Product Name', value: props.equipo.hw_product_name },
  { label: 'Firmware',     value: props.equipo.hw_firmware },
  { label: 'Serial',       value: props.equipo.hw_serial },
  { label: 'UUID',         value: props.equipo.hw_uuid },
] as { label: string; value?: string }[]).filter(f => f.value))

const reportedAt = computed(() =>
  props.equipo.hw_reported_at ? new Date(props.equipo.hw_reported_at).toLocaleString() : '')
</script>

<template>
  <div v-if="fields.length" class="rounded-sm border border-border/60 bg-muted/10 px-3 py-2">
    <div class="flex items-center gap-1.5 mb-2">
      <Cpu class="h-3 w-3 text-[color:var(--epm-citrico)]" />
      <span class="text-[10px] uppercase tracking-[0.16em] font-semibold text-muted-foreground">
        Identidad de hardware
      </span>
      <span v-if="reportedAt" class="text-[10px] text-muted-foreground/70 ml-auto" :title="'Última actualización: ' + reportedAt">
        {{ reportedAt }}
      </span>
    </div>
    <dl class="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
      <div v-for="f in fields" :key="f.label" class="min-w-0">
        <dt class="text-[10px] uppercase tracking-wide text-muted-foreground">{{ f.label }}</dt>
        <dd class="font-mono text-xs truncate" :title="f.value">{{ f.value }}</dd>
      </div>
    </dl>
  </div>
</template>
