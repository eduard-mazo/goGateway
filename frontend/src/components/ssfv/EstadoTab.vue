<script setup lang="ts">
// SSFV tab component — consumes the shared useSsfv() singleton. Behaviour and
// markup extracted verbatim from SSFVView; the parent gates mounting via v-if.
import { useSsfv } from '@/composables/useSsfv'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { RefreshCw, Zap } from 'lucide-vue-next'
const {
  alarmasActivas,
  fetchMonitoreo,
  invalidateCache,
  missed,
  monTab,
  refreshing,
  status,
  ultimasLecturas,} = useSsfv()
</script>

<template>
    <div class="space-y-4">

      <!-- Metrics grid -->
      <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
        <div v-for="metric in [
          { label: 'Conexión',      value: status?.connected ? 'Activa' : 'Inactiva',   accent: status?.connected ? 'text-green-400' : 'text-destructive' },
          { label: 'Circuit',       value: status?.circuit_open ? 'Abierto' : 'Cerrado', accent: status?.circuit_open ? 'text-destructive' : 'text-green-400' },
          { label: 'Escritura',     value: (status?.write_rate?.toFixed(1) ?? '—') + ' pt/s', accent: '' },
          { label: 'Errores',       value: String(status?.error_rate ?? '—'),           accent: (status?.error_rate ?? 0) > 0 ? 'text-destructive' : '' },
          { label: 'Sin mapeo',     value: String(status?.skipped_rows ?? 0),           accent: (status?.skipped_rows ?? 0) > 0 ? 'text-amber-400' : '' },
        ]" :key="metric.label"
          class="card-soft p-3 space-y-1"
        >
          <div class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">{{ metric.label }}</div>
          <div class="font-mono text-sm" :class="metric.accent">{{ metric.value }}</div>
        </div>
      </div>

      <div v-if="status?.last_error" class="rounded-sm border border-destructive/30 bg-destructive/10 p-3 text-xs font-mono text-destructive">
        {{ status.last_error }}
      </div>

      <div class="flex items-center gap-3">
        <Button variant="outline" size="sm" @click="invalidateCache" :disabled="!status?.connected">
          <Zap class="h-3.5 w-3.5 mr-1.5" /> Invalidar cache
        </Button>
        <Button variant="outline" size="sm" @click="fetchMonitoreo" :disabled="refreshing">
          <RefreshCw class="h-3.5 w-3.5 mr-1.5" :class="{ 'animate-spin': refreshing }" /> Actualizar
        </Button>
      </div>

      <!-- Sub-tabs -->
      <div class="flex border-b border-border gap-0.5">
        <button
          v-for="mt in [
            { id: 'missed',   label: 'Señales sin mapeo', badge: missed.length },
            { id: 'lecturas', label: 'Últimas lecturas' },
            { id: 'alarmas',  label: 'Alarmas activas', badge: alarmasActivas.length },
          ] as const"
          :key="mt.id"
          class="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium transition-colors border-b-2 -mb-px"
          :class="monTab === mt.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
          @click="monTab = mt.id"
        >
          {{ mt.label }}
          <span v-if="'badge' in mt && mt.badge" class="text-[10px] px-1.5 rounded-full font-bold"
            :class="mt.id === 'alarmas' ? 'bg-red-500/20 text-red-400' : 'bg-amber-500/20 text-amber-400'">
            {{ (mt as any).badge }}
          </span>
        </button>
      </div>

      <!-- Missed signals -->
      <div v-if="monTab === 'missed'" class="overflow-x-auto rounded-sm border border-border">
        <Table>
          <TableHeader>
            <TableRow class="bg-muted/30">
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Signal Path</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold text-right">Ocurrencias</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Última vez</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="m in missed" :key="m.signal_path" class="border-b border-border/50 last:border-0 hover:bg-muted/20">
              <TableCell class="font-mono text-xs text-amber-400">{{ m.signal_path }}</TableCell>
              <TableCell class="font-mono text-xs text-right">{{ m.count }}</TableCell>
              <TableCell class="font-mono text-[11px] text-muted-foreground">{{ m.last_seen }}</TableCell>
            </TableRow>
            <TableRow v-if="!missed.length">
              <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-6">Sin señales sin mapeo — todos los paths están catalogados.</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>

      <!-- Últimas lecturas -->
      <div v-if="monTab === 'lecturas'" class="overflow-x-auto rounded-sm border border-border">
        <Table>
          <TableHeader>
            <TableRow class="bg-muted/30">
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">equisenal_id</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Timestamp UTC</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold text-right">Valor</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Calidad</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in ultimasLecturas" :key="row.equisenal_id" class="border-b border-border/50 last:border-0 hover:bg-muted/20">
              <TableCell class="font-mono text-xs">{{ row.equisenal_id }}</TableCell>
              <TableCell class="font-mono text-xs text-muted-foreground">{{ row.timestamp_utc }}</TableCell>
              <TableCell class="font-mono text-xs text-right">{{ row.valor }}</TableCell>
              <TableCell>
                <span class="text-[10px] px-1.5 py-0.5 rounded-sm font-semibold"
                  :class="{
                    'bg-green-500/15 text-green-500': row.calidad === 'Buena',
                    'bg-amber-500/15 text-amber-400': row.calidad === 'Dudosa',
                    'bg-red-500/15 text-red-400':     row.calidad === 'Mala',
                  }">
                  {{ row.calidad }}
                </span>
              </TableCell>
            </TableRow>
            <TableRow v-if="!ultimasLecturas.length">
              <TableCell colspan="4" class="text-center text-muted-foreground text-xs py-6">Sin lecturas recientes</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>

      <!-- Alarmas activas -->
      <div v-if="monTab === 'alarmas'" class="overflow-x-auto rounded-sm border border-border">
        <Table>
          <TableHeader>
            <TableRow class="bg-muted/30">
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Severidad</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Tipo</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Planta</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Equipo</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Señal</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Inicio</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in alarmasActivas" :key="row.alarma_id" class="border-b border-border/50 last:border-0 hover:bg-muted/20">
              <TableCell>
                <span class="text-[10px] px-1.5 py-0.5 rounded-sm font-semibold" :class="{
                  'bg-red-900/40 text-red-300': row.severidad === 'Critica',
                  'bg-red-500/15 text-red-400': row.severidad === 'Alta',
                  'bg-amber-500/15 text-amber-400': row.severidad === 'Media',
                  'bg-muted text-muted-foreground': row.severidad === 'Baja',
                }">{{ row.severidad }}</span>
              </TableCell>
              <TableCell class="text-xs">{{ row.tipo_alarma }}</TableCell>
              <TableCell class="text-xs">{{ row.planta_nombre }}</TableCell>
              <TableCell class="text-xs font-medium">{{ row.nombre_equipo }}</TableCell>
              <TableCell class="font-mono text-xs text-[color:var(--epm-citrico)]">{{ row.nombre_instancia }}</TableCell>
              <TableCell class="font-mono text-[11px] text-muted-foreground">{{ row.ts_inicio }}</TableCell>
            </TableRow>
            <TableRow v-if="!alarmasActivas.length">
              <TableCell colspan="6" class="text-center text-muted-foreground text-xs py-6">Sin alarmas activas</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </div>

</template>
