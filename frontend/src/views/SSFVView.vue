<script setup lang="ts">
import {
  Sun, TriangleAlert, CheckCircle2, XCircle,
  Building2, Layers, Ruler, Activity, Scan,
} from 'lucide-vue-next'
import PlantasTab from '@/components/ssfv/PlantasTab.vue'
import CatalogoTab from '@/components/ssfv/CatalogoTab.vue'
import TiposTab from '@/components/ssfv/TiposTab.vue'
import EstadoTab from '@/components/ssfv/EstadoTab.vue'
import PendientesTab from '@/components/ssfv/PendientesTab.vue'
import SsfvDialogs from '@/components/ssfv/SsfvDialogs.vue'
import { useSsfv } from '@/composables/useSsfv'

// SSFV view shell: header/status + tab nav. Each tab and the dialog set are
// their own components consuming the useSsfv() singleton; the shell destructures
// only what its nav references.
const {
  autoPendingCount,
  missed,
  status,
  tab,
} = useSsfv()
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6">

    <!-- ── Header ─────────────────────────────────────────────────────────── -->
    <div class="flex items-start justify-between border-b border-border pb-5">
      <div class="flex items-start gap-3">
        <div class="grid place-items-center w-9 h-9 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)] text-[color:var(--epm-bosque)]">
          <Sun class="h-4 w-4" />
        </div>
        <div>
          <div class="text-[10px] uppercase tracking-[0.24em] font-bold text-[color:var(--epm-bosque)]">Pipeline TSDB</div>
          <h1 class="mt-1 mb-1 text-2xl font-extrabold">Señales SSFV</h1>
          <p class="text-sm text-muted-foreground">Catálogo de plantas, equipos y señales fotovoltaicas</p>
        </div>
      </div>

      <!-- SSFV status badge -->
      <div
        class="shrink-0 flex items-center gap-2 px-3 py-1.5 rounded-sm border text-xs font-mono"
        :class="status?.connected
          ? 'border-green-500/40 bg-green-500/10 text-green-400'
          : 'border-[color:var(--destructive)]/30 bg-[color:var(--destructive)]/10 text-[color:var(--destructive)]'"
      >
        <component :is="status?.connected ? CheckCircle2 : XCircle" class="h-3.5 w-3.5" />
        {{ status?.connected ? 'TimescaleDB conectado' : 'Sin conexión SSFV' }}
        <template v-if="status?.connected && status.write_rate !== undefined">
          · {{ status.write_rate.toFixed(0) }} pt/s
        </template>
      </div>
    </div>

    <!-- Not connected warning -->
    <div
      v-if="!status?.connected"
      class="flex items-start gap-3 rounded-sm border border-amber-500/40 bg-amber-500/8 p-4 text-sm"
    >
      <TriangleAlert class="h-4 w-4 text-amber-400 shrink-0 mt-0.5" />
      <div>
        <div class="font-semibold text-amber-300">Adaptador SSFV no conectado</div>
        <div class="text-amber-400/80 text-xs mt-1">
          Configure el pipeline TimescaleDB en <strong>Pipeline TSDB</strong> y aplique las migraciones
          <code class="font-mono bg-black/20 px-1 rounded">000007</code> y
          <code class="font-mono bg-black/20 px-1 rounded">000008</code>.
        </div>
      </div>
    </div>

    <!-- ── Tabs ────────────────────────────────────────────────────────────── -->
    <div class="flex border-b border-border gap-0.5">
      <button
        v-for="t in [
          { id: 'plantas',    label: 'Infraestructura',  icon: Building2 },
          { id: 'catalogo',   label: 'Catálogo Señales', icon: Layers },
          { id: 'tipos',      label: 'Tipos y Unidades', icon: Ruler },
          { id: 'estado',     label: 'Estado',            icon: Activity },
          { id: 'pendientes', label: 'Pendientes',        icon: Scan },
        ] as const"
        :key="t.id"
        class="inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium transition-colors border-b-2 -mb-px"
        :class="tab === t.id
          ? 'border-[color:var(--epm-bosque)] text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'"
        @click="tab = t.id"
      >
        <component :is="t.icon" class="h-3.5 w-3.5" />
        {{ t.label }}
        <span
          v-if="t.id === 'estado' && (missed.length > 0 || (status?.skipped_rows ?? 0) > 0)"
          class="ml-0.5 text-[10px] px-1.5 rounded-full bg-amber-500/20 text-amber-400 font-bold"
        >
          !
        </span>
        <span
          v-if="t.id === 'pendientes' && autoPendingCount > 0"
          class="ml-0.5 text-[10px] px-1.5 rounded-full bg-blue-500/20 text-blue-400 font-bold"
        >
          {{ autoPendingCount }}
        </span>
      </button>
    </div>


    <PlantasTab    v-if="tab === 'plantas'" />
    <CatalogoTab   v-if="tab === 'catalogo'" />
    <TiposTab      v-if="tab === 'tipos'" />
    <EstadoTab     v-if="tab === 'estado'" />
    <PendientesTab v-if="tab === 'pendientes'" />
  </div>

  <SsfvDialogs />
</template>
