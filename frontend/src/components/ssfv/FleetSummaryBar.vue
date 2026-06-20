<script setup lang="ts">
// FleetSummaryBar — the SSFV plantas fleet KPI strip. Pure presentational:
// the parent computes the aggregate, this only renders it.
interface FleetSummary {
  total: number
  activas: number
  kwp: number
  equipos: number
  senales: number
  alarmas: number
  live: number
}
defineProps<{ summary: FleetSummary }>()
</script>

<template>
  <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
    <div class="card-soft p-3 space-y-1">
      <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">Plantas</div>
      <div class="font-mono text-lg font-bold leading-none">{{ summary.activas }}<span class="text-xs font-normal text-muted-foreground"> / {{ summary.total }} activas</span></div>
    </div>
    <div class="card-soft p-3 space-y-1">
      <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">Capacidad</div>
      <div class="font-mono text-lg font-bold leading-none">{{ summary.kwp.toLocaleString() }}<span class="text-xs font-normal text-muted-foreground"> kWp</span></div>
    </div>
    <div class="card-soft p-3 space-y-1">
      <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">Equipos</div>
      <div class="font-mono text-lg font-bold leading-none">{{ summary.equipos }}</div>
    </div>
    <div class="card-soft p-3 space-y-1">
      <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">Señales</div>
      <div class="font-mono text-lg font-bold leading-none">{{ summary.senales }}</div>
    </div>
    <div class="card-soft p-3 space-y-1">
      <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">En línea</div>
      <div class="font-mono text-lg font-bold leading-none" :class="summary.live ? 'text-emerald-500' : ''">
        {{ summary.live }}<span class="text-xs font-normal text-muted-foreground"> / {{ summary.total }} &lt;2 min</span>
      </div>
    </div>
    <div class="card-soft p-3 space-y-1" :class="summary.alarmas ? '!border-red-500/40' : ''">
      <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">Alarmas activas</div>
      <div class="font-mono text-lg font-bold leading-none" :class="summary.alarmas ? 'text-red-400' : ''">{{ summary.alarmas }}</div>
    </div>
  </div>
</template>
