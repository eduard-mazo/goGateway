<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
  DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Sun, Plus, CheckCircle2, XCircle, Building2, Cpu, Layers,
  Link2, Activity, Ruler, GitBranch, TriangleAlert, Scan,
  Search, Tag, Boxes, Cog, Radio, ListChecks, Loader2,
} from 'lucide-vue-next'
import PlantasTab from '@/components/ssfv/PlantasTab.vue'
import CatalogoTab from '@/components/ssfv/CatalogoTab.vue'
import TiposTab from '@/components/ssfv/TiposTab.vue'
import EstadoTab from '@/components/ssfv/EstadoTab.vue'
import PendientesTab from '@/components/ssfv/PendientesTab.vue'
import { useSsfv } from '@/composables/useSsfv'

// SSFV view shell: tab nav + dialogs. State/actions live in useSsfv(); each tab
// is its own component consuming the same singleton. Destructure only what the
// shell template (nav + dialogs) references.
const {
  approveDetectedTypes,
  approveDialog,
  approveForm,
  approveReadOnly,
  approveTarget,
  autoPendingCount,
  brokerBaseInvalid,
  bulkDialog,
  bulkDone,
  bulkFallbackVar,
  bulkFillTipo,
  bulkIncludeSystem,
  bulkMissingTipo,
  bulkNewPlantas,
  bulkProgress,
  bulkReady,
  bulkRows,
  bulkRunning,
  bulkSummary,
  equipoDialog,
  equipoEdit,
  equipoForm,
  equipoPlantaBase,
  equipoTopicInvalid,
  fronteraDialog,
  fronteraEdit,
  fronteraForm,
  includeSystem,
  KIND_META,
  metricSearch,
  missed,
  planSummary,
  plantaDialog,
  plantaEdit,
  plantaForm,
  PLANTA_NEW,
  plantas,
  planValid,
  reviewGroups,
  reviewSummary,
  runBulkApprove,
  saveEquipo,
  saveFrontera,
  savePlanta,
  saveSenal,
  saveSenalXTipo,
  saveTipoEquipo,
  saveTipoVar,
  saveUnidad,
  senalDialog,
  senalEdit,
  senalForm,
  senalXTipoDialog,
  senalXTipoForm,
  signalPlan,
  sistemaCount,
  status,
  submitApprove,
  tab,
  tipoEquipoDialog,
  tipoEquipoEdit,
  tipoEquipoForm,
  tipoEquipos,
  tipoVarDialog,
  tipoVarEdit,
  tipoVarForm,
  tipoVars,
  unidadDialog,
  unidadEdit,
  unidades,
  unidadForm,
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

  <!-- ═══════════════════════════════════════════════════════════════════════
       DIALOGS
  ═══════════════════════════════════════════════════════════════════════ -->

  <!-- Aprobar entidad autodescubierta — revisión de métricas del NBIRTH -->
  <Dialog v-model:open="approveDialog">
    <DialogContent class="!max-w-3xl p-0 overflow-hidden gap-0">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-bosque)_10%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Scan class="h-4 w-4 text-[color:var(--epm-bosque)]" />
          {{ approveReadOnly ? 'Métricas reportadas' : 'Aprobar entidad descubierta' }}
        </DialogTitle>
        <DialogDescription v-if="approveTarget" class="flex flex-wrap items-center gap-2 text-xs">
          <span class="inline-flex items-center gap-1 font-mono px-1.5 py-0.5 rounded-sm bg-card border border-border">
            <component :is="approveTarget.device_id ? Cpu : Radio" class="h-3 w-3"
              :class="approveTarget.device_id ? 'text-violet-400' : 'text-sky-400'" />
            {{ approveTarget.device_id
              ? `${approveTarget.group_id}/${approveTarget.node_id}/${approveTarget.device_id}`
              : `${approveTarget.group_id}/${approveTarget.node_id}` }}
          </span>
          <span class="text-[9px] px-1.5 py-0.5 rounded-sm font-bold uppercase tracking-wide"
            :class="approveTarget.device_id ? 'bg-violet-500/15 text-violet-400' : 'bg-sky-500/15 text-sky-400'">
            {{ approveTarget.device_id ? 'DBIRTH' : 'NBIRTH' }}
          </span>
        </DialogDescription>
      </DialogHeader>

      <div class="max-h-[72vh] overflow-y-auto">
        <!-- ── Asignación (oculto en modo inspección) ───────────────────────── -->
        <div v-if="!approveReadOnly" class="px-6 py-5 space-y-4 border-b border-border">
          <div class="flex items-center gap-2 text-[11px] uppercase tracking-[0.16em] font-bold text-muted-foreground">
            <Building2 class="h-3.5 w-3.5" /> Asignación
          </div>

          <div v-if="approveDetectedTypes.length" class="rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border border-[color:color-mix(in_srgb,var(--epm-citrico)_30%,transparent)] px-3 py-2 text-[11px]">
            <span class="text-muted-foreground">Tipo(s) de equipo detectado(s) en el payload: </span>
            <span v-for="(t, i) in approveDetectedTypes" :key="t" class="font-semibold">{{ t }}{{ i < approveDetectedTypes.length - 1 ? ', ' : '' }}</span>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div class="grid gap-1.5">
              <Label>Planta * <span class="text-[10px] text-muted-foreground font-normal">(destino)</span></Label>
              <Select v-model="approveForm.planta_id">
                <SelectTrigger class="rounded-sm"><SelectValue placeholder="Seleccionar planta…" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="p in plantas" :key="p.planta_id" :value="String(p.planta_id)">{{ p.nombre }}</SelectItem>
                  <SelectItem :value="PLANTA_NEW">➕ Crear planta para el group «{{ approveTarget?.group_id }}»</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div v-if="approveForm.planta_id === PLANTA_NEW" class="grid gap-1.5">
              <Label>Alias de la planta * <span class="text-[10px] text-muted-foreground font-normal">(group = {{ approveTarget?.group_id }})</span></Label>
              <Input v-model="approveForm.planta_nombre" class="rounded-sm h-8" placeholder="p. ej. GSANRAFA" />
            </div>
            <div class="grid gap-1.5">
              <Label>Tipo de equipo *</Label>
              <Select v-model="approveForm.tipo_id">
                <SelectTrigger class="rounded-sm"><SelectValue placeholder="Seleccionar tipo…" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="t in tipoEquipos" :key="t.tipo_id" :value="String(t.tipo_id)">{{ t.nombre }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="grid gap-1.5">
              <Label>Nombre del equipo</Label>
              <Input v-model="approveForm.nombre_equipo" class="rounded-sm h-8" />
            </div>
            <div class="grid gap-1.5">
              <Label>Topic MQTT <span class="text-[10px] text-muted-foreground font-normal">(clave de caché)</span></Label>
              <Input v-model="approveForm.nombre_topic" class="rounded-sm h-8 font-mono text-xs" />
            </div>
          </div>
          <p class="text-[11px] text-muted-foreground">
            Se creará el equipo en la planta seleccionada y se auto-instanciarán las señales de la plantilla
            del tipo elegido. El nodo recibirá un <span class="font-mono">NCMD Rebirth</span> para comenzar a enviar datos.
          </p>
        </div>

        <!-- ── Métricas reportadas ──────────────────────────────────────────── -->
        <div class="px-6 py-5 space-y-3">
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2 text-[11px] uppercase tracking-[0.16em] font-bold text-muted-foreground">
              <Activity class="h-3.5 w-3.5" /> Métricas reportadas
              <span class="text-foreground">· {{ reviewSummary.total }}</span>
            </div>
            <div class="relative w-48">
              <Search class="h-3.5 w-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground" />
              <Input v-model="metricSearch" placeholder="Filtrar…" class="rounded-sm h-7 pl-7 text-xs" />
            </div>
          </div>

          <!-- include host (System/*) metrics as señales -->
          <label v-if="!approveReadOnly && sistemaCount > 0"
            class="flex items-center justify-between gap-3 rounded-sm border border-sky-500/30 bg-sky-500/5 px-3 py-2 cursor-pointer select-none">
            <span class="flex items-center gap-2 text-[11px]">
              <Cog class="h-3.5 w-3.5 text-sky-400" />
              <span class="font-semibold text-foreground">Registrar métricas de Sistema</span>
              <span class="text-muted-foreground">— telemetría del host (CPU, memoria, disco, red) como
                señales en <span class="font-mono">{{ approveForm.nombre_topic }}</span>
                · <span class="font-semibold text-sky-400">{{ sistemaCount }}</span> disponible(s)</span>
            </span>
            <Switch v-model="includeSystem" />
          </label>

          <!-- signal provisioning plan -->
          <div v-if="!approveReadOnly" class="rounded-sm border border-border bg-muted/30 px-3 py-2 text-[11px]">
            <div v-if="!approveForm.tipo_id" class="text-muted-foreground inline-flex items-center gap-1.5">
              <TriangleAlert class="h-3.5 w-3.5 text-amber-500" />
              Selecciona un tipo de equipo para reconciliar las señales contra el catálogo y su plantilla.
            </div>
            <div v-else class="flex flex-wrap items-center gap-x-4 gap-y-1">
              <span class="inline-flex items-center gap-1">
                <CheckCircle2 class="h-3.5 w-3.5 text-emerald-500" />
                <span class="font-semibold">{{ planSummary.mapped }}</span> ya mapeadas
              </span>
              <span class="inline-flex items-center gap-1">
                <Link2 class="h-3.5 w-3.5 text-blue-400" />
                <span class="font-semibold">{{ planSummary.catalog }}</span> en catálogo
                <span class="text-muted-foreground">→ {{ planSummary.willLink }} a vincular</span>
              </span>
              <span class="inline-flex items-center gap-1">
                <Plus class="h-3.5 w-3.5 text-amber-500" />
                <span class="font-semibold">{{ planSummary.new }}</span> nuevas
                <span class="text-muted-foreground">→ {{ planSummary.willCreate }} a crear</span>
              </span>
              <span v-if="!planValid" class="text-destructive font-semibold inline-flex items-center gap-1">
                <TriangleAlert class="h-3.5 w-3.5" /> Completa variable y unidad de las señales nuevas marcadas
              </span>
            </div>
          </div>

          <!-- summary chips -->
          <div class="flex flex-wrap items-center gap-1.5">
            <span v-for="kc in reviewSummary.kinds" :key="kc.kind"
              class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm border" :class="KIND_META[kc.kind].chip">
              <component :is="KIND_META[kc.kind].icon" class="h-3 w-3" />
              {{ kc.n }} {{ KIND_META[kc.kind].label }}
            </span>
            <span v-if="reviewSummary.devs.length" class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm border border-border bg-muted text-muted-foreground">
              <Boxes class="h-3 w-3" /> {{ reviewSummary.devs.length }} tipo(s): {{ reviewSummary.devs.join(', ') }}
            </span>
            <span v-if="reviewSummary.units.length" class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm border border-border bg-muted text-muted-foreground">
              <Ruler class="h-3 w-3" /> {{ reviewSummary.units.join(', ') }}
            </span>
          </div>

          <!-- grouped metric list -->
          <div class="rounded-sm border border-border divide-y divide-border/60">
            <div v-for="g in reviewGroups" :key="g.name">
              <div class="flex items-center gap-1.5 px-3 py-1.5 bg-muted/40 text-[10px] uppercase tracking-wide font-bold text-muted-foreground sticky top-0">
                <Boxes class="h-3 w-3" />{{ g.name }}
                <span class="text-muted-foreground/60 font-normal normal-case">· {{ g.metrics.length }} métrica(s)</span>
              </div>
              <div v-for="m in g.metrics" :key="m.name" class="px-3 py-2 hover:bg-muted/20">
                <div class="flex items-start gap-3">
                  <span class="h-1.5 w-1.5 rounded-full mt-1.5 shrink-0" :class="KIND_META[m.kind].dot"
                    :title="KIND_META[m.kind].label" />
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-mono text-xs font-medium text-foreground">{{ m.leaf }}</span>
                      <span v-if="m.tipoVariable" class="inline-flex items-center gap-1 text-[9px] px-1.5 py-0.5 rounded-sm bg-muted border border-border text-muted-foreground">
                        <Tag class="h-2.5 w-2.5" />{{ m.tipoVariable }}
                      </span>
                      <span v-if="m.engUnit" class="inline-flex items-center gap-1 text-[9px] px-1.5 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_15%,transparent)] text-foreground font-semibold"
                        title="Unidad de referencia (cosmética) — etiqueta para el operador; no escala ni altera el valor almacenado.">
                        <Ruler class="h-2.5 w-2.5" />{{ m.engUnit }} <span class="opacity-50 font-normal not-italic">ref.</span>
                      </span>
                      <span v-if="m.tipoValor" class="text-[9px] px-1.5 py-0.5 rounded-sm border border-border text-muted-foreground italic">{{ m.tipoValor }}</span>
                      <span v-if="m.deviceType" class="text-[9px] px-1.5 py-0.5 rounded-sm bg-violet-500/10 text-violet-400">{{ m.deviceType }}</span>
                    </div>
                    <div class="font-mono text-[10px] text-muted-foreground truncate">{{ m.name }}</div>
                    <div v-if="m.description" class="text-[11px] text-muted-foreground mt-0.5">{{ m.description }}</div>
                  </div>

                  <!-- per-signal action: a plan entry exists for every registrable
                       metric (plant 'proceso', plus 'sistema' when the toggle is on) -->
                  <div v-if="!approveReadOnly && signalPlan[m.name]" class="shrink-0 flex items-center gap-2 pt-0.5">
                    <span v-if="signalPlan[m.name].status === 'catalog'"
                      class="text-[9px] px-1.5 py-0.5 rounded-sm bg-blue-500/15 text-blue-400 font-bold uppercase tracking-wide">En catálogo</span>
                    <span v-else
                      class="text-[9px] px-1.5 py-0.5 rounded-sm bg-amber-500/15 text-amber-400 font-bold uppercase tracking-wide">Nueva</span>
                    <span v-if="signalPlan[m.name].instance !== 'default'"
                      class="text-[9px] px-1.5 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)] text-foreground font-mono"
                      :title="'Canal / instancia: ' + signalPlan[m.name].instance">⌗ {{ signalPlan[m.name].instance }}</span>
                    <label class="inline-flex items-center gap-1 text-[11px] cursor-pointer select-none"
                      :class="signalPlan[m.name].codeTooLong ? 'opacity-40 cursor-not-allowed' : ''">
                      <input type="checkbox" v-model="signalPlan[m.name].selected"
                        :disabled="signalPlan[m.name].codeTooLong"
                        class="h-3.5 w-3.5 accent-[color:var(--epm-bosque)]" />
                      {{ signalPlan[m.name].status === 'catalog' ? 'Vincular' : 'Crear' }}
                    </label>
                  </div>
                  <span v-else-if="!approveReadOnly && m.kind === 'proceso' && approveForm.tipo_id"
                    class="shrink-0 text-[9px] px-1.5 py-0.5 rounded-sm bg-emerald-500/15 text-emerald-500 font-bold uppercase tracking-wide inline-flex items-center gap-1">
                    <CheckCircle2 class="h-3 w-3" />Mapeada
                  </span>
                  <span v-else-if="!approveReadOnly && m.kind === 'sistema'"
                    class="shrink-0 text-[9px] text-sky-400 inline-flex items-center gap-1 pt-1 cursor-pointer"
                    title="Telemetría del host. Activa «Registrar métricas de Sistema» arriba para crearla como señal."
                    @click="includeSystem = true">
                    <Cog class="h-3 w-3" /> host
                  </span>
                  <!-- Device hardware identity (Device/* strings): not a signal —
                       show the reported value; captured onto the equipo on approval. -->
                  <span v-else-if="m.kind === 'identidad'"
                    class="shrink-0 flex items-center gap-1.5 pt-0.5"
                    title="Identidad de hardware del dispositivo — se guarda en el equipo al aprobar">
                    <Cpu class="h-3 w-3 text-[color:var(--epm-citrico)]" />
                    <span v-if="m.value" class="font-mono text-[11px] text-foreground">{{ m.value }}</span>
                    <span v-else class="text-[9px] text-muted-foreground italic">identidad</span>
                  </span>
                  <span v-else-if="!approveReadOnly && m.kind !== 'proceso'"
                    class="shrink-0 text-[9px] text-muted-foreground italic pt-1">no aplica</span>
                </div>

                <!-- inline editor for a new señal -->
                <div v-if="!approveReadOnly && signalPlan[m.name] && signalPlan[m.name].status === 'new' && signalPlan[m.name].selected"
                  class="mt-2 ml-5 grid grid-cols-3 gap-2 rounded-sm bg-muted/30 border border-border p-2">
                  <div class="grid gap-1">
                    <Label class="text-[10px] text-muted-foreground">Variable *</Label>
                    <Select v-model="signalPlan[m.name].tipovarId">
                      <SelectTrigger class="rounded-sm h-7 text-xs"><SelectValue placeholder="— elegir —" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem v-for="tv in tipoVars" :key="tv.tipovar_id" :value="String(tv.tipovar_id)">{{ tv.nombre }}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div class="grid gap-1">
                    <Label class="text-[10px] text-muted-foreground">Unidad *</Label>
                    <Select v-model="signalPlan[m.name].unidadId">
                      <SelectTrigger class="rounded-sm h-7 text-xs"><SelectValue placeholder="— elegir —" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem v-for="u in unidades" :key="u.unidad_id" :value="String(u.unidad_id)">{{ u.simbolo }} · {{ u.nombre }}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div class="grid gap-1">
                    <Label class="text-[10px] text-muted-foreground">Tipo valor</Label>
                    <Select v-model="signalPlan[m.name].tipoValor">
                      <SelectTrigger class="rounded-sm h-7 text-xs"><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="Instantaneo">Instantáneo</SelectItem>
                        <SelectItem value="Acumulado">Acumulado</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div class="col-span-3 text-[10px] text-muted-foreground">
                    Se creará <span class="font-mono text-foreground">{{ signalPlan[m.name].code }}</span>
                    <span v-if="signalPlan[m.name].instance !== 'default'">
                      · instancia <span class="font-mono text-[color:var(--epm-citrico)]">{{ signalPlan[m.name].instance }}</span> (se vincula a este equipo)</span>
                    <span v-else> y se añadirá a la plantilla del tipo</span>.
                  </div>
                </div>
              </div>
            </div>
            <div v-if="!reviewGroups.length" class="px-3 py-6 text-center text-xs text-muted-foreground">
              {{ metricSearch ? 'Ningún metric coincide con el filtro.' : 'Esta entidad no reportó métricas.' }}
            </div>
          </div>
        </div>
      </div>

      <DialogFooter class="px-6 py-4 border-t border-border gap-2">
        <Button variant="ghost" size="sm" @click="approveDialog = false">
          {{ approveReadOnly ? 'Cerrar' : 'Cancelar' }}
        </Button>
        <Button
          v-if="!approveReadOnly"
          size="sm"
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
          :disabled="!approveForm.planta_id || (approveForm.planta_id === PLANTA_NEW && !approveForm.planta_nombre.trim()) || !approveForm.tipo_id || !planValid"
          @click="submitApprove"
        >
          <CheckCircle2 class="h-3.5 w-3.5 mr-1.5" />
          Crear equipo<span v-if="planSummary.willCreate || planSummary.willLink" class="font-normal opacity-90">
            &nbsp;· +{{ planSummary.willCreate }} nuevas, {{ planSummary.willLink }} vinc.</span>
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Planta dialog -->
  <Dialog v-model:open="plantaDialog">
    <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Sun class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ plantaEdit ? 'Editar planta' : 'Nueva planta solar' }}
        </DialogTitle>
        <DialogDescription class="text-xs">Planta fotovoltaica en el catálogo SSFV</DialogDescription>
      </DialogHeader>
      <div class="px-6 py-5 grid grid-cols-2 gap-4">
        <div class="col-span-2 space-y-1.5">
          <Label>Nombre *</Label>
          <Input v-model="plantaForm.nombre" placeholder="Central Solar Norte" />
        </div>
        <div class="col-span-2 space-y-1.5">
          <Label>Group ID * <span class="text-[10px] text-muted-foreground">(grupo Sparkplug B — la planta = un group, un solo segmento)</span></Label>
          <Input v-model="plantaForm.broker_base" placeholder="plant-floor" class="font-mono"
            :class="brokerBaseInvalid ? 'border-destructive' : ''" />
          <p v-if="brokerBaseInvalid" class="text-[11px] text-destructive">
            El Group ID debe ser un único segmento, sin «/» ni espacios (p. ej. <span class="font-mono">plant-floor</span>).
          </p>
          <p v-else-if="plantaForm.broker_base" class="text-[11px] text-muted-foreground font-mono">
            spBv1.0/<span class="text-[color:var(--epm-citrico)]">{{ plantaForm.broker_base.trim() }}</span>/NDATA/&lt;nodo&gt;
            · …/DDATA/&lt;nodo&gt;/&lt;device&gt;
          </p>
        </div>
        <div class="space-y-1.5">
          <Label>Ubicación</Label>
          <Input v-model="plantaForm.ubicacion" placeholder="Medellín, Colombia" />
        </div>
        <div class="space-y-1.5">
          <Label>Propietario</Label>
          <Input v-model="plantaForm.propietario" placeholder="EPM" />
        </div>
        <div class="space-y-1.5">
          <Label>Capacidad kWp</Label>
          <Input v-model.number="plantaForm.capacidad_kWp" type="number" step="0.01" placeholder="2500.00" />
        </div>
        <div class="space-y-1.5">
          <Label>Estado</Label>
          <Select v-model="plantaForm.estado">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem :value="1">Activa</SelectItem>
              <SelectItem :value="0">Inactiva</SelectItem>
              <SelectItem :value="2">Mantenimiento</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="plantaDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="savePlanta" :disabled="!plantaForm.nombre || !plantaForm.broker_base || brokerBaseInvalid">
          {{ plantaEdit ? 'Guardar' : 'Crear planta' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Equipo dialog -->
  <Dialog v-model:open="equipoDialog">
    <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Cpu class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ equipoEdit ? 'Editar equipo' : 'Nuevo equipo' }}
        </DialogTitle>
        <DialogDescription class="text-xs">Al crear, las señales se instancian automáticamente según el tipo de equipo</DialogDescription>
      </DialogHeader>
      <div class="px-6 py-5 grid grid-cols-2 gap-4">
        <div class="col-span-2 space-y-1.5">
          <Label>Tipo de equipo *</Label>
          <Select v-model="equipoForm.tipo_id">
            <SelectTrigger><SelectValue placeholder="Seleccionar tipo…" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="te in tipoEquipos" :key="te.tipo_id" :value="te.tipo_id!">{{ te.nombre }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="col-span-2 space-y-1.5">
          <Label>Nombre del equipo *</Label>
          <Input v-model="equipoForm.nombre_equipo" placeholder="Inversor 1" />
        </div>
        <div class="space-y-1.5">
          <Label>Ruta bajo el group * <span class="text-[10px] text-muted-foreground font-normal">nodo[/device]</span></Label>
          <Input v-model="equipoForm.nombre_topic_suffix" placeholder="edge-1/meter-01" class="font-mono" />
        </div>
        <div class="space-y-1.5">
          <Label>Topic completo <span class="text-[10px] text-muted-foreground font-normal">(group/nodo[/device])</span></Label>
          <Input v-model="equipoForm.nombre_topic" class="font-mono bg-muted/30 text-[11px]"
            :class="equipoTopicInvalid ? 'border-destructive' : ''" />
          <p v-if="equipoTopicInvalid" class="text-[11px] text-destructive">
            Debe empezar por el Group ID de la planta (<span class="font-mono">{{ equipoPlantaBase }}</span>).
          </p>
        </div>
        <div class="space-y-1.5">
          <Label>Fabricante</Label>
          <Input v-model="equipoForm.fabricante" placeholder="SMA, ABB…" />
        </div>
        <div class="space-y-1.5">
          <Label>Modelo</Label>
          <Input v-model="equipoForm.modelo" placeholder="STP 25000TL" />
        </div>
        <div class="space-y-1.5">
          <Label>N° Serie</Label>
          <Input v-model="equipoForm.nro_serie" class="font-mono" />
        </div>
        <div class="space-y-1.5">
          <Label>Estado</Label>
          <Select v-model="equipoForm.estado">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem :value="1">Activo</SelectItem>
              <SelectItem :value="0">Inactivo</SelectItem>
              <SelectItem :value="2">Mantenimiento</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="equipoDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveEquipo" :disabled="!equipoForm.nombre_equipo || !equipoForm.nombre_topic || !equipoForm.tipo_id || equipoTopicInvalid">
          {{ equipoEdit ? 'Guardar' : 'Crear equipo' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Frontera dialog -->
  <Dialog v-model:open="fronteraDialog">
    <DialogContent class="!max-w-md p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Link2 class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ fronteraEdit ? 'Editar frontera' : 'Nueva frontera comercial' }}
        </DialogTitle>
      </DialogHeader>
      <div class="px-6 py-5 space-y-4">
        <div class="space-y-1.5">
          <Label>Código NIE *</Label>
          <Input v-model="fronteraForm.codigo_nie" placeholder="NIE-001" class="font-mono" />
        </div>
        <div class="space-y-1.5">
          <Label>Nombre *</Label>
          <Input v-model="fronteraForm.nombre" placeholder="Frontera Principal" />
        </div>
        <div class="space-y-1.5">
          <Label>Tipo de conexión</Label>
          <Input v-model="fronteraForm.tipo_conexion" placeholder="MT, BT, AT…" />
        </div>
        <div class="flex items-center gap-3">
          <Switch id="f-activo" v-model:checked="fronteraForm.activo" />
          <Label for="f-activo">Activa</Label>
        </div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="fronteraDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveFrontera" :disabled="!fronteraForm.codigo_nie || !fronteraForm.nombre">
          {{ fronteraEdit ? 'Guardar' : 'Crear frontera' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Señal dialog -->
  <Dialog v-model:open="senalDialog">
    <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Layers class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ senalEdit ? 'Editar señal' : 'Nueva señal en catálogo' }}
        </DialogTitle>
        <DialogDescription class="text-xs">Define una señal reutilizable para cualquier tipo de equipo</DialogDescription>
      </DialogHeader>
      <div class="px-6 py-5 grid grid-cols-2 gap-4">
        <div class="space-y-1.5">
          <Label>Código *</Label>
          <Input v-model="senalForm.codigo_senal" placeholder="AP" class="font-mono" />
        </div>
        <div class="space-y-1.5">
          <Label>Nombre *</Label>
          <Input v-model="senalForm.nombre" placeholder="Potencia Activa" />
        </div>
        <div class="space-y-1.5">
          <Label>Tipo de variable *</Label>
          <Select v-model="senalForm.tipavar_id">
            <SelectTrigger><SelectValue placeholder="Seleccionar…" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="tv in tipoVars" :key="tv.tipovar_id" :value="tv.tipovar_id!">{{ tv.nombre }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-1.5">
          <Label>Unidad *</Label>
          <Select v-model="senalForm.unidad_id">
            <SelectTrigger><SelectValue placeholder="Seleccionar…" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="u in unidades" :key="u.unidad_id" :value="u.unidad_id!">{{ u.simbolo }} – {{ u.nombre }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-1.5">
          <Label>Tipo de valor</Label>
          <Select v-model="senalForm.tipo_valor">
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="Instantaneo">Instantáneo</SelectItem>
              <SelectItem value="Acumulado">Acumulado</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-1.5">
          <Label>Descripción</Label>
          <Input v-model="senalForm.descripcion" placeholder="Opcional…" />
        </div>
        <div class="flex items-center gap-4 col-span-2">
          <div class="flex items-center gap-2">
            <Switch id="s-idx" v-model:checked="senalForm.es_indexada" />
            <Label for="s-idx">Señal indexada <span class="text-[10px] text-muted-foreground">(multicanal, sufijo _N)</span></Label>
          </div>
          <div class="flex items-center gap-2">
            <Switch id="s-act" v-model:checked="senalForm.activo" />
            <Label for="s-act">Activa</Label>
          </div>
        </div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="senalDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveSenal" :disabled="!senalForm.codigo_senal || !senalForm.nombre || !senalForm.tipavar_id || !senalForm.unidad_id">
          {{ senalEdit ? 'Guardar' : 'Crear señal' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Señal × Tipo dialog -->
  <Dialog v-model:open="senalXTipoDialog">
    <DialogContent class="!max-w-sm p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <GitBranch class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          Asignar a tipo de equipo
        </DialogTitle>
        <DialogDescription class="text-xs">Esta señal se auto-instanciará al crear equipos de este tipo</DialogDescription>
      </DialogHeader>
      <div class="px-6 py-5 space-y-4">
        <div class="space-y-1.5">
          <Label>Tipo de equipo *</Label>
          <Select v-model="senalXTipoForm.tipo_id">
            <SelectTrigger><SelectValue placeholder="Seleccionar tipo…" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="te in tipoEquipos" :key="te.tipo_id" :value="te.tipo_id!">{{ te.nombre }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-1.5">
          <Label>Número de canales <span class="text-[10px] text-muted-foreground">(para señales indexadas)</span></Label>
          <Input v-model.number="senalXTipoForm.num_canales" type="number" min="1" />
        </div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="senalXTipoDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveSenalXTipo" :disabled="!senalXTipoForm.tipo_id">
          Asignar
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Tipo Equipo dialog -->
  <Dialog v-model:open="tipoEquipoDialog">
    <DialogContent class="!max-w-sm p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Cpu class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ tipoEquipoEdit ? 'Editar tipo de equipo' : 'Nuevo tipo de equipo' }}
        </DialogTitle>
      </DialogHeader>
      <div class="px-6 py-5 space-y-4">
        <div class="space-y-1.5"><Label>Nombre *</Label><Input v-model="tipoEquipoForm.nombre" placeholder="Inversor" /></div>
        <div class="space-y-1.5"><Label>Descripción</Label><Input v-model="tipoEquipoForm.descripcion" /></div>
        <div class="flex items-center gap-2"><Switch id="te-act" v-model:checked="tipoEquipoForm.activo" /><Label for="te-act">Activo</Label></div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="tipoEquipoDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveTipoEquipo" :disabled="!tipoEquipoForm.nombre">
          {{ tipoEquipoEdit ? 'Guardar' : 'Crear' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Tipo Variable dialog -->
  <Dialog v-model:open="tipoVarDialog">
    <DialogContent class="!max-w-sm p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Activity class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ tipoVarEdit ? 'Editar tipo de variable' : 'Nuevo tipo de variable' }}
        </DialogTitle>
      </DialogHeader>
      <div class="px-6 py-5 space-y-4">
        <div class="space-y-1.5"><Label>Nombre *</Label><Input v-model="tipoVarForm.nombre" placeholder="Potencia" /></div>
        <div class="space-y-1.5"><Label>Descripción</Label><Input v-model="tipoVarForm.descripcion" /></div>
        <div class="flex items-center gap-2"><Switch id="tv-act" v-model:checked="tipoVarForm.activo" /><Label for="tv-act">Activo</Label></div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="tipoVarDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveTipoVar" :disabled="!tipoVarForm.nombre">
          {{ tipoVarEdit ? 'Guardar' : 'Crear' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Unidad dialog -->
  <Dialog v-model:open="unidadDialog">
    <DialogContent class="!max-w-sm p-0 overflow-hidden">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <Ruler class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          {{ unidadEdit ? 'Editar unidad' : 'Nueva unidad de medida' }}
        </DialogTitle>
      </DialogHeader>
      <div class="px-6 py-5 space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div class="space-y-1.5"><Label>Símbolo *</Label><Input v-model="unidadForm.simbolo" placeholder="kW" class="font-mono" /></div>
          <div class="space-y-1.5"><Label>Magnitud *</Label><Input v-model="unidadForm.magnitud" placeholder="Potencia" /></div>
        </div>
        <div class="space-y-1.5"><Label>Nombre</Label><Input v-model="unidadForm.nombre" placeholder="Kilovatio" /></div>
        <div class="flex items-center gap-2"><Switch id="u-act" v-model:checked="unidadForm.activo" /><Label for="u-act">Activa</Label></div>
      </div>
      <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
        <Button variant="outline" class="rounded-sm" @click="unidadDialog = false">Cancelar</Button>
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveUnidad" :disabled="!unidadForm.simbolo || !unidadForm.magnitud">
          {{ unidadEdit ? 'Guardar' : 'Crear' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Aprobación en lote -->
  <Dialog v-model:open="bulkDialog">
    <DialogContent class="!max-w-4xl p-0 overflow-hidden gap-0" @interact-outside="(e: Event) => bulkRunning && e.preventDefault()">
      <DialogHeader class="px-6 py-4 bg-[color:color-mix(in_srgb,var(--epm-bosque)_10%,transparent)] border-b border-border">
        <DialogTitle class="flex items-center gap-2 text-base">
          <ListChecks class="h-4 w-4 text-[color:var(--epm-bosque)]" />
          Aprobación en lote
        </DialogTitle>
        <DialogDescription class="text-xs">
          {{ bulkRows.length }} entidad(es) — planta, tipo y señales derivados del
          <span class="font-mono">NBIRTH/DBIRTH</span>. Las señales de catálogo se vinculan; las nuevas se crean
          con su variable y unidad detectadas.
        </DialogDescription>
      </DialogHeader>

      <div class="max-h-[68vh] overflow-y-auto">

        <!-- Plantas que se crearán -->
        <div v-if="bulkNewPlantas.length" class="px-6 py-4 border-b border-border space-y-2">
          <div class="flex items-center gap-2 text-[11px] uppercase tracking-[0.16em] font-bold text-muted-foreground">
            <Sun class="h-3.5 w-3.5 text-[color:var(--epm-citrico)]" /> Plantas a crear
            <span class="font-mono font-normal normal-case">· {{ bulkNewPlantas.length }}</span>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div v-for="np in bulkNewPlantas" :key="np.group"
              class="flex items-center gap-2 rounded-sm border border-amber-500/30 bg-amber-500/5 px-2.5 py-1.5">
              <code class="font-mono text-xs text-muted-foreground shrink-0">{{ np.group }} →</code>
              <Input v-model="np.nombre" class="rounded-sm h-7 text-xs flex-1" placeholder="Alias de la planta"
                :disabled="bulkRunning || bulkDone" />
            </div>
          </div>
        </div>

        <!-- Defaults del lote -->
        <div v-if="!bulkDone" class="px-6 py-4 border-b border-border grid grid-cols-2 gap-4">
          <div class="grid gap-1.5">
            <Label class="text-xs">
              Tipo de equipo para las {{ bulkMissingTipo }} fila(s) sin tipo detectado
            </Label>
            <Select v-model="bulkFillTipo" :disabled="bulkRunning || !bulkMissingTipo">
              <SelectTrigger class="rounded-sm h-8 text-xs">
                <SelectValue :placeholder="bulkMissingTipo ? 'Asignar a las filas vacías…' : 'Todas las filas tienen tipo ✓'" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="t in tipoEquipos" :key="t.tipo_id" :value="String(t.tipo_id)">{{ t.nombre }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5">
            <Label class="text-xs">Variable por defecto <span class="text-muted-foreground font-normal">(señales nuevas sin tipo_variable en el payload)</span></Label>
            <Select v-model="bulkFallbackVar" :disabled="bulkRunning">
              <SelectTrigger class="rounded-sm h-8 text-xs"><SelectValue placeholder="— omitir esas señales —" /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="tv in tipoVars" :key="tv.tipovar_id" :value="String(tv.tipovar_id)">{{ tv.nombre }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <label class="col-span-2 flex items-center justify-between gap-3 rounded-sm border border-sky-500/30 bg-sky-500/5 px-3 py-2 cursor-pointer select-none">
            <span class="flex items-center gap-2 text-[11px]">
              <Cog class="h-3.5 w-3.5 text-sky-400" />
              <span class="font-semibold text-foreground">Registrar telemetría del host</span>
              <span class="text-muted-foreground">— métricas System/* (CPU, memoria, red…) de los nodos como señales</span>
            </span>
            <Switch v-model="bulkIncludeSystem" :disabled="bulkRunning" />
          </label>
        </div>

        <!-- Progreso -->
        <div v-if="bulkRunning || bulkDone" class="px-6 pt-4">
          <div class="flex items-center justify-between text-[11px] text-muted-foreground mb-1.5">
            <span class="font-semibold" :class="bulkDone ? 'text-foreground' : ''">
              {{ bulkDone ? `Lote terminado — ${bulkSummary.ok} ok · ${bulkSummary.parcial} parciales · ${bulkSummary.error} con error`
                          : 'Aprobando…' }}
            </span>
            <span class="font-mono">{{ bulkProgress }}%</span>
          </div>
          <div class="h-1 rounded-full bg-muted overflow-hidden">
            <div class="h-full bg-[color:var(--epm-bosque)] transition-all duration-300" :style="{ width: bulkProgress + '%' }"></div>
          </div>
        </div>

        <!-- Filas del lote -->
        <div class="px-6 py-4 space-y-1.5">
          <div v-for="row in bulkRows" :key="row.entity.id"
            class="flex items-center gap-3 rounded-sm border border-border bg-card px-3 py-2">
            <!-- estado -->
            <span class="shrink-0 w-4 grid place-items-center">
              <Loader2 v-if="row.state === 'corriendo'" class="h-4 w-4 animate-spin text-[color:var(--epm-bosque)]" />
              <CheckCircle2 v-else-if="row.state === 'ok'" class="h-4 w-4 text-emerald-500" />
              <TriangleAlert v-else-if="row.state === 'parcial'" class="h-4 w-4 text-amber-500" />
              <XCircle v-else-if="row.state === 'error'" class="h-4 w-4 text-destructive" />
              <span v-else class="h-1.5 w-1.5 rounded-full bg-muted-foreground/40"></span>
            </span>

            <!-- entidad -->
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <component :is="row.entity.device_id ? Cpu : Radio" class="h-3 w-3 shrink-0"
                  :class="row.entity.device_id ? 'text-violet-400' : 'text-sky-400'" />
                <span class="font-mono text-xs font-medium truncate">{{ row.topic }}</span>
                <span class="text-[9px] px-1 py-0.5 rounded-sm font-bold uppercase shrink-0"
                  :class="row.entity.device_id ? 'bg-violet-500/15 text-violet-400' : 'bg-sky-500/15 text-sky-400'">
                  {{ row.entity.device_id ? 'DBIRTH' : 'NBIRTH' }}
                </span>
              </div>
              <div class="text-[10px] text-muted-foreground mt-0.5">
                <template v-if="row.state === 'listo'">
                  <span class="text-amber-400 font-semibold" v-if="row.nuevas">+{{ row.nuevas }} señal(es) nueva(s)</span>
                  <span v-if="row.nuevas && row.vinculadas"> · </span>
                  <span class="text-blue-400" v-if="row.vinculadas">{{ row.vinculadas }} a vincular</span>
                  <span v-if="(row.nuevas || row.vinculadas) && row.omitidas.length"> · </span>
                  <span class="text-destructive" v-if="row.omitidas.length"
                    :title="row.omitidas.map(o => `${o.code}: ${o.reason}`).join('\n')">
                    {{ row.omitidas.length }} omitida(s)
                  </span>
                  <span v-if="!row.nuevas && !row.vinculadas && !row.omitidas.length">solo plantilla del tipo</span>
                </template>
                <template v-else>{{ row.detail }}</template>
              </div>
            </div>

            <!-- tipo equipo -->
            <div class="shrink-0 w-44">
              <Select v-model="row.tipoId" :disabled="bulkRunning || bulkDone">
                <SelectTrigger class="rounded-sm h-7 text-xs"
                  :class="!row.tipoId ? 'border-amber-500/60' : ''">
                  <SelectValue placeholder="⚠ tipo de equipo…" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="t in tipoEquipos" :key="t.tipo_id" :value="String(t.tipo_id)">{{ t.nombre }}</SelectItem>
                </SelectContent>
              </Select>
              <div v-if="row.tipoDetected" class="text-[9px] text-muted-foreground mt-0.5 truncate text-right">
                payload: {{ row.tipoDetected }}
              </div>
            </div>
          </div>
        </div>
      </div>

      <DialogFooter class="px-6 py-4 border-t border-border gap-2">
        <Button variant="ghost" size="sm" :disabled="bulkRunning" @click="bulkDialog = false">
          {{ bulkDone ? 'Cerrar' : 'Cancelar' }}
        </Button>
        <Button
          v-if="!bulkDone"
          size="sm"
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
          :disabled="!bulkReady || bulkRunning"
          @click="runBulkApprove"
        >
          <Loader2 v-if="bulkRunning" class="h-3.5 w-3.5 mr-1.5 animate-spin" />
          <CheckCircle2 v-else class="h-3.5 w-3.5 mr-1.5" />
          {{ bulkRunning ? 'Aprobando…' : `Aprobar ${bulkRows.length} entidad(es)` }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.bulkbar-enter-active, .bulkbar-leave-active { transition: opacity .18s ease, transform .18s ease; }
.bulkbar-enter-from, .bulkbar-leave-to { opacity: 0; transform: translateY(12px); }
.bulkbar-enter-to, .bulkbar-leave-from { opacity: 1; transform: translateY(0); }
</style>
