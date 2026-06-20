<script setup lang="ts">
import { Button }  from '@/components/ui/button'
import { Input }   from '@/components/ui/input'
import { Label }   from '@/components/ui/label'
import { Switch }  from '@/components/ui/switch'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
  DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import {
  Sun, Plus, Pencil, Trash2, RefreshCw, CheckCircle2, XCircle,
  ChevronRight, ChevronDown, Zap, Building2, Cpu, Layers,
  Link2, Activity, Ruler, GitBranch, TriangleAlert, Scan,
  Eye, Search, Hash, Tag, Boxes, Cog, Radio,
  ListChecks, Loader2, X,
} from 'lucide-vue-next'
import SignalTree from '@/components/SignalTree.vue'
import HwIdentity from '@/components/HwIdentity.vue'
import FleetSummaryBar from '@/components/ssfv/FleetSummaryBar.vue'
import PlantaIec104Link from '@/components/ssfv/PlantaIec104Link.vue'
import { useSsfv } from '@/composables/useSsfv'

// All state + actions live in the useSsfv() composable (singleton); this view is
// the shell that renders the tabs/dialogs. Destructure only what the template uses.
const {
  alarmasActivas,
  allVisibleSelected,
  approveDetectedTypes,
  approveDialog,
  approveForm,
  approveReadOnly,
  approveTarget,
  autoEntities,
  autoGroups,
  autoPendingCount,
  autoSearch,
  autoSelected,
  autoStatusFilter,
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
  bulkReject,
  bulkRows,
  bulkRunning,
  bulkSummary,
  clearAutoSelection,
  deleteAsignacion,
  deleteAutodiscoveredEntity,
  deleteEquipo,
  deleteFrontera,
  deletePlanta,
  deleteSenal,
  deleteSenalXTipo,
  deleteTipoEquipo,
  deleteTipoVar,
  deleteUnidad,
  entityKindCounts,
  entityNewSignalCount,
  equipoDialog,
  equipoEdit,
  equipoForm,
  equipoOpen,
  equipoPlantaBase,
  equipoSignals,
  equipoTopicInvalid,
  estadoClass,
  estadoLabel,
  expandedAuto,
  fetchAutodiscovered,
  fetchMonitoreo,
  filteredPlantas,
  filteredSenales,
  fleetSummary,
  FRESH_META,
  freshness,
  fronteraDialog,
  fronteraEdit,
  fronteraForm,
  groupAllSelected,
  groupedPreview,
  groupSelectable,
  iec104Servers,
  includeSystem,
  invalidateCache,
  KIND_META,
  lastSegment,
  metricInCatalog,
  metricSearch,
  missed,
  monTab,
  nodeTree,
  openAddSenalXTipo,
  openApprove,
  openBulkApprove,
  openCreateEquipo,
  openCreateFrontera,
  openCreatePlanta,
  openCreateSenal,
  openCreateTipoEquipo,
  openCreateTipoVar,
  openCreateUnidad,
  openEditEquipo,
  openEditFrontera,
  openEditPlanta,
  openEditSenal,
  openEditTipoEquipo,
  openEditTipoVar,
  openEditUnidad,
  openInspect,
  planSummary,
  plantaDialog,
  plantaEdit,
  plantaEquipos,
  plantaEstadoFilter,
  plantaForm,
  plantaFronteras,
  plantaLimit,
  PLANTA_NEW,
  plantaOpen,
  PLANTA_PAGE,
  plantas,
  plantaSearch,
  planValid,
  refreshing,
  rejectEntity,
  relTime,
  resetEntity,
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
  selectedAuto,
  senalDialog,
  senalEdit,
  senales,
  senalForm,
  senalLimit,
  senalOpen,
  SENAL_PAGE,
  senalSearch,
  senalXTipo,
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
  tipoEquipoIcon,
  tipoEquipos,
  tipoVarDialog,
  tipoVarEdit,
  tipoVarForm,
  tipoVars,
  toggleAutoRow,
  toggleAutoSelect,
  toggleEquipo,
  toggleGroupSelect,
  togglePlanta,
  toggleSelectAllVisible,
  toggleSenal,
  ultimasLecturas,
  unidadDialog,
  unidadEdit,
  unidades,
  unidadForm,
  visiblePendingAuto,
  visiblePlantas,
  visibleSenales,
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

    <!-- ═══════════════════════════════════════════════════════════════════════
         TAB: Infraestructura (Plantas → Equipos → Señales + Fronteras)
    ═══════════════════════════════════════════════════════════════════════ -->
    <div v-if="tab === 'plantas'" class="space-y-4">
      <!-- Resumen de flota -->
      <FleetSummaryBar :summary="fleetSummary" />

      <!-- Toolbar: búsqueda + filtro de estado, pensado para miles de plantas -->
      <div class="flex flex-wrap items-center gap-3">
        <div class="relative flex-1 min-w-56 max-w-sm">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input v-model="plantaSearch" placeholder="Buscar planta, group, ubicación…" class="pl-8 rounded-sm text-xs" />
        </div>
        <div class="flex rounded-sm border border-border overflow-hidden">
          <button
            v-for="f in [
              { id: 'all', label: 'Todas' },
              { id: '1',   label: 'Activas' },
              { id: '2',   label: 'Mantto.' },
              { id: '0',   label: 'Inactivas' },
            ] as const"
            :key="f.id"
            class="px-2.5 py-1.5 text-[11px] font-medium transition-colors border-r border-border last:border-r-0"
            :class="plantaEstadoFilter === f.id
              ? 'bg-[color:var(--epm-bosque)] text-white'
              : 'bg-card text-muted-foreground hover:text-foreground'"
            @click="plantaEstadoFilter = f.id"
          >
            {{ f.label }}
          </button>
        </div>
        <span class="text-xs text-muted-foreground font-mono">{{ filteredPlantas.length }} / {{ plantas.length }}</span>
        <Button
          class="ml-auto bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
          @click="openCreatePlanta"
        >
          <Plus class="h-4 w-4 mr-1.5" /> Nueva planta
        </Button>
      </div>

      <div v-if="!plantas.length" class="card-soft p-8 text-center text-sm text-muted-foreground">
        No hay plantas configuradas. Crea la primera planta solar para comenzar.
      </div>
      <div v-else-if="!filteredPlantas.length" class="card-soft p-8 text-center text-sm text-muted-foreground">
        Ninguna planta coincide con «{{ plantaSearch }}».
      </div>

      <!-- Plant cards -->
      <div v-for="p in visiblePlantas" :key="p.planta_id" class="card-soft overflow-hidden">

        <!-- Plant header -->
        <button
          type="button"
          class="w-full flex items-center gap-3 px-4 py-3.5 hover:bg-muted/30 transition-colors text-left"
          @click="togglePlanta(p.planta_id!)"
        >
          <ChevronRight
            class="h-4 w-4 shrink-0 text-muted-foreground transition-transform"
            :class="plantaOpen === p.planta_id ? 'rotate-90' : ''"
          />
          <div class="grid place-items-center w-8 h-8 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_14%,transparent)] shrink-0">
            <Sun class="h-4 w-4 text-[color:var(--epm-citrico)]" />
          </div>
          <div class="flex-1 min-w-0">
            <div class="font-bold text-sm flex items-center gap-2">
              {{ p.nombre }}
              <span v-if="p.ubicacion" class="font-normal text-[11px] text-muted-foreground truncate">· {{ p.ubicacion }}</span>
            </div>
            <div class="text-[11px] text-muted-foreground font-mono truncate">{{ p.broker_base }}</div>
          </div>

          <!-- Fleet stats: equipos / señales / alarmas / frescura del dato -->
          <div class="hidden md:flex items-center gap-2 shrink-0 mr-1">
            <span class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm bg-muted text-muted-foreground font-mono"
              title="Equipos activos">
              <Cpu class="h-3 w-3" />{{ p.n_equipos ?? 0 }}
            </span>
            <span class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm bg-muted text-muted-foreground font-mono"
              title="Señales instanciadas activas">
              <Layers class="h-3 w-3" />{{ p.n_senales ?? 0 }}
            </span>
            <span v-if="p.n_alarmas"
              class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm bg-red-500/15 text-red-400 font-bold font-mono"
              title="Alarmas activas">
              <TriangleAlert class="h-3 w-3" />{{ p.n_alarmas }}
            </span>
            <span class="inline-flex items-center gap-1.5 text-[10px] w-28 justify-end"
              :class="FRESH_META[freshness(p)].text"
              :title="p.ultima_lectura ? 'Última lectura: ' + new Date(p.ultima_lectura).toLocaleString() : 'Esta planta nunca ha escrito valores'">
              <span class="h-2 w-2 rounded-full shrink-0" :class="FRESH_META[freshness(p)].dot"></span>
              <span class="font-mono truncate">{{ relTime(p.ultima_lectura) }}</span>
            </span>
          </div>

          <div class="flex items-center gap-2 shrink-0" @click.stop>
            <span class="text-[10px] px-2 py-0.5 rounded-sm font-semibold uppercase tracking-wide" :class="estadoClass(p.estado)">
              {{ estadoLabel(p.estado) }}
            </span>
            <template v-if="p.capacidad_kWp">
              <span class="text-[10px] font-mono text-muted-foreground">{{ p.capacidad_kWp }} kWp</span>
            </template>
            <Button variant="ghost" size="icon" class="h-7 w-7" @click="openEditPlanta(p)">
              <Pencil class="h-3.5 w-3.5" />
            </Button>
            <Button variant="ghost" size="icon" class="h-7 w-7 text-destructive" @click="deletePlanta(p)">
              <Trash2 class="h-3.5 w-3.5" />
            </Button>
          </div>
        </button>

        <!-- Expanded: Fronteras + Equipos -->
        <div v-show="plantaOpen === p.planta_id" class="border-t border-border bg-muted/10">

          <!-- Fronteras section -->
          <div class="px-4 py-3 border-b border-border/60">
            <div class="flex items-center justify-between mb-2">
              <div class="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
                <Link2 class="h-3.5 w-3.5" /> Fronteras Comerciales
                <span class="font-mono font-normal">({{ (plantaFronteras[p.planta_id!] ?? []).length }})</span>
              </div>
              <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateFrontera(p.planta_id!)">
                <Plus class="h-3 w-3 mr-1" /> Agregar
              </Button>
            </div>
            <div v-if="!(plantaFronteras[p.planta_id!] ?? []).length" class="text-xs text-muted-foreground italic py-1 pl-1">
              Sin fronteras comerciales
            </div>
            <div v-else class="space-y-1">
              <div
                v-for="f in plantaFronteras[p.planta_id!]"
                :key="f.frontera_id"
                class="flex items-center gap-3 rounded-sm border border-border/60 bg-background px-3 py-1.5 text-xs"
              >
                <code class="font-mono text-[color:var(--epm-citrico)] w-24 shrink-0">{{ f.codigo_nie }}</code>
                <span class="flex-1 font-medium truncate">{{ f.nombre }}</span>
                <span v-if="f.tipo_conexion" class="text-muted-foreground">{{ f.tipo_conexion }}</span>
                <span :class="f.activo ? 'text-green-500' : 'text-muted-foreground'" class="text-[10px]">
                  {{ f.activo ? '● Activa' : '○ Inactiva' }}
                </span>
                <Button variant="ghost" size="icon" class="h-5 w-5" @click="openEditFrontera(f)"><Pencil class="h-3 w-3" /></Button>
                <Button variant="ghost" size="icon" class="h-5 w-5 text-destructive" @click="deleteFrontera(f)"><Trash2 class="h-3 w-3" /></Button>
              </div>
            </div>
          </div>

          <!-- IEC-104 link: mirror this plant's signals to a 104 server (lazy:
               mounts/loads only while the plant is expanded) -->
          <PlantaIec104Link
            v-if="plantaOpen === p.planta_id"
            :planta-id="p.planta_id!"
            :servers="iec104Servers"
          />

          <!-- Equipos section -->
          <div class="px-4 py-3">
            <div class="flex items-center justify-between mb-2">
              <div class="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
                <Cpu class="h-3.5 w-3.5" /> Equipos
                <span class="font-mono font-normal">({{ (plantaEquipos[p.planta_id!] ?? []).length }})</span>
              </div>
              <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateEquipo(p.planta_id!)">
                <Plus class="h-3 w-3 mr-1" /> Agregar equipo
              </Button>
            </div>
            <div v-if="!(plantaEquipos[p.planta_id!] ?? []).length" class="text-xs text-muted-foreground italic py-1 pl-1">
              Sin equipos. Agrega un equipo para auto-instanciar sus señales.
            </div>

            <!-- Hierarchy (contract §1.1): Nodo (group/node) is the PARENT;
                 its devices (group/node/device) nest beneath with tree rails -->
            <div v-for="br in nodeTree(p)" :key="br.node" class="mb-3">
              <div class="rounded-sm border border-border bg-card overflow-hidden border-l-[3px] !border-l-[color:var(--epm-bosque)]">

                <!-- Node row (parent entity, carries System + node señales) -->
                <template v-if="br.nodeEntity">
                  <div class="flex items-center gap-3 px-3 py-2.5 bg-[color:color-mix(in_srgb,var(--epm-bosque)_5%,transparent)]">
                    <button type="button" class="flex items-center gap-2.5 flex-1 min-w-0 text-left" @click="toggleEquipo(br.nodeEntity.equipo_id!)">
                      <ChevronRight
                        class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform"
                        :class="equipoOpen === br.nodeEntity.equipo_id ? 'rotate-90' : ''"
                      />
                      <div class="grid place-items-center w-7 h-7 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-bosque)_14%,transparent)] shrink-0">
                        <GitBranch class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" />
                      </div>
                      <div class="min-w-0">
                        <div class="text-sm font-bold truncate">
                          {{ br.node }}
                          <span v-if="br.nodeEntity.nombre_equipo !== br.node" class="font-normal text-muted-foreground">· {{ br.nodeEntity.nombre_equipo }}</span>
                        </div>
                        <div class="text-[10px] text-muted-foreground font-mono truncate">{{ br.nodeEntity.nombre_topic }}</div>
                      </div>
                    </button>
                    <div class="flex items-center gap-2 shrink-0">
                      <span
                        class="text-[9px] px-1.5 py-0.5 rounded-sm bg-sky-500/15 text-sky-400 font-bold uppercase tracking-wide inline-flex items-center gap-1"
                        title="Entidad de nivel nodo (group/node) — porta System + señales de proceso del nodo">
                        <Cog class="h-2.5 w-2.5" />Nodo
                      </span>
                      <span class="text-[10px] px-1.5 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)] text-[color:var(--epm-bosque)] font-semibold">
                        {{ br.nodeEntity.tipo_nombre }}
                      </span>
                      <span class="text-[10px] font-mono text-muted-foreground">{{ br.devices.length }} device(s)</span>
                      <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditEquipo(br.nodeEntity)"><Pencil class="h-3 w-3" /></Button>
                      <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteEquipo(br.nodeEntity)"><Trash2 class="h-3 w-3" /></Button>
                    </div>
                  </div>

                  <!-- Node signal instances -->
                  <div v-if="equipoOpen === br.nodeEntity.equipo_id" class="border-t border-border/60 bg-muted/10 px-4 py-2">
                    <HwIdentity :equipo="br.nodeEntity" class="mb-3" />
                    <div class="text-[10px] uppercase tracking-[0.16em] text-muted-foreground mb-2 font-semibold">
                      Señales instanciadas — <span class="font-mono font-normal">{{ (equipoSignals[br.nodeEntity.equipo_id!] ?? []).length }} puntos</span>
                    </div>
                    <div v-if="!(equipoSignals[br.nodeEntity.equipo_id!] ?? []).length" class="text-xs text-muted-foreground italic">Sin señales instanciadas</div>
                    <SignalTree v-else :signals="equipoSignals[br.nodeEntity.equipo_id!]" @delete="deleteAsignacion" />
                  </div>
                </template>

                <!-- Ghost node: devices exist but the node entity isn't registered -->
                <div v-else class="flex items-center gap-2.5 px-3 py-2.5 text-xs text-muted-foreground">
                  <div class="grid place-items-center w-7 h-7 rounded-sm border border-dashed border-border shrink-0">
                    <GitBranch class="h-3.5 w-3.5" />
                  </div>
                  <span>
                    Nodo <code class="font-mono text-foreground">{{ br.node }}</code>
                    <span class="italic"> — sin registrar (aprueba su NBIRTH en Pendientes para capturar System)</span>
                  </span>
                </div>

                <!-- Devices nested under the node -->
                <div v-if="br.devices.length" class="pl-[26px] pr-2 pb-2">
                  <div v-for="(dev, di) in br.devices" :key="dev.equipo_id" class="relative pl-5 pt-2">
                    <!-- tree rails: vertical guide + elbow into the row -->
                    <div class="absolute left-0 top-0 w-px bg-border" :class="di === br.devices.length - 1 ? 'h-[30px]' : 'h-full'"></div>
                    <div class="absolute left-0 top-[30px] h-px w-4 bg-border"></div>

                    <div class="rounded-sm border border-border bg-background overflow-hidden border-l-2 !border-l-violet-500/50">
                      <!-- Device row -->
                      <div class="flex items-center gap-3 px-3 py-2">
                        <button type="button" class="flex items-center gap-2 flex-1 min-w-0 text-left" @click="toggleEquipo(dev.equipo_id!)">
                          <ChevronRight
                            class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform"
                            :class="equipoOpen === dev.equipo_id ? 'rotate-90' : ''"
                          />
                          <div class="grid place-items-center w-6 h-6 rounded-sm bg-violet-500/10 shrink-0">
                            <component :is="tipoEquipoIcon(dev.tipo_nombre ?? '')" class="h-3.5 w-3.5 text-violet-400" />
                          </div>
                          <div class="min-w-0">
                            <div class="text-sm font-semibold truncate">
                              {{ lastSegment(dev.nombre_topic) }}
                              <span v-if="dev.nombre_equipo !== lastSegment(dev.nombre_topic)" class="font-normal text-muted-foreground">· {{ dev.nombre_equipo }}</span>
                            </div>
                            <div class="text-[10px] text-muted-foreground font-mono truncate">{{ dev.nombre_topic }}</div>
                          </div>
                        </button>
                        <div class="flex items-center gap-2 shrink-0">
                          <span
                            class="text-[9px] px-1.5 py-0.5 rounded-sm bg-violet-500/15 text-violet-400 font-bold uppercase tracking-wide inline-flex items-center gap-1"
                            title="Device (group/node/device)">
                            <Cpu class="h-2.5 w-2.5" />Device
                          </span>
                          <span class="text-[10px] px-1.5 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)] text-[color:var(--epm-bosque)] font-semibold">
                            {{ dev.tipo_nombre }}
                          </span>
                          <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditEquipo(dev)"><Pencil class="h-3 w-3" /></Button>
                          <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteEquipo(dev)"><Trash2 class="h-3 w-3" /></Button>
                        </div>
                      </div>

                      <!-- Device signal instances -->
                      <div v-if="equipoOpen === dev.equipo_id" class="border-t border-border/60 bg-muted/10 px-4 py-2">
                        <HwIdentity :equipo="dev" class="mb-3" />
                        <div class="text-[10px] uppercase tracking-[0.16em] text-muted-foreground mb-2 font-semibold">
                          Señales instanciadas — <span class="font-mono font-normal">{{ (equipoSignals[dev.equipo_id!] ?? []).length }} puntos</span>
                        </div>
                        <div v-if="!(equipoSignals[dev.equipo_id!] ?? []).length" class="text-xs text-muted-foreground italic">Sin señales instanciadas</div>
                        <SignalTree v-else :signals="equipoSignals[dev.equipo_id!]" @delete="deleteAsignacion" />
                      </div>
                    </div>
                  </div>
                </div>
                <div v-else-if="br.nodeEntity" class="pl-[26px] pb-2 text-[11px] italic text-muted-foreground">
                  Sin devices bajo este nodo
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Render incremental: el filtro corre sobre todo el set, el DOM pinta por páginas -->
      <div v-if="filteredPlantas.length > plantaLimit" class="flex justify-center pt-1">
        <Button variant="outline" size="sm" class="rounded-sm text-xs" @click="plantaLimit += PLANTA_PAGE">
          Mostrar {{ Math.min(PLANTA_PAGE, filteredPlantas.length - plantaLimit) }} más
          <span class="text-muted-foreground font-mono ml-1.5">({{ filteredPlantas.length - plantaLimit }} restantes)</span>
        </Button>
      </div>
    </div>

    <!-- ═══════════════════════════════════════════════════════════════════════
         TAB: Catálogo Señales (tbl_senales + senales_x_tipo_equipo)
    ═══════════════════════════════════════════════════════════════════════ -->
    <div v-if="tab === 'catalogo'" class="space-y-4">
      <div class="flex items-center gap-3">
        <div class="relative flex-1 max-w-sm">
          <Layers class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input v-model="senalSearch" placeholder="Filtrar señales…" class="pl-8 rounded-sm font-mono text-xs" />
        </div>
        <div class="ml-auto flex items-center gap-2">
          <span class="text-xs text-muted-foreground font-mono">{{ filteredSenales.length }} / {{ senales.length }}</span>
          <Button
            class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
            @click="openCreateSenal"
          >
            <Plus class="h-4 w-4 mr-1.5" /> Nueva señal
          </Button>
        </div>
      </div>

      <div v-if="!senales.length" class="card-soft p-8 text-center text-sm text-muted-foreground">
        Sin señales en el catálogo.
      </div>

      <div v-for="s in visibleSenales" :key="s.senal_id" class="card-soft overflow-hidden">
        <!-- Signal row -->
        <button
          type="button"
          class="w-full flex items-center gap-3 px-4 py-3 hover:bg-muted/30 transition-colors text-left"
          @click="toggleSenal(s.senal_id!)"
        >
          <ChevronRight class="h-4 w-4 shrink-0 text-muted-foreground transition-transform" :class="senalOpen === s.senal_id ? 'rotate-90' : ''" />
          <code class="font-mono text-[color:var(--epm-citrico)] text-sm w-20 shrink-0">{{ s.codigo_senal }}</code>
          <div class="flex-1 min-w-0">
            <div class="font-semibold text-sm truncate">{{ s.nombre }}</div>
            <div class="text-[11px] text-muted-foreground">
              {{ s.tipo_var_nombre }} · {{ s.unidad_simbolo }} · {{ s.tipo_valor }}
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0" @click.stop>
            <span v-if="s.es_indexada" class="text-[10px] px-1.5 py-0.5 rounded-sm bg-blue-500/15 text-blue-400 font-semibold">Indexada</span>
            <span class="text-[10px] px-1.5 py-0.5 rounded-sm font-semibold" :class="s.activo ? 'bg-green-500/15 text-green-500' : 'bg-muted text-muted-foreground'">
              {{ s.activo ? 'Activa' : 'Inactiva' }}
            </span>
            <Button variant="ghost" size="icon" class="h-7 w-7" @click="openEditSenal(s)"><Pencil class="h-3.5 w-3.5" /></Button>
            <Button variant="ghost" size="icon" class="h-7 w-7 text-destructive" @click="deleteSenal(s)"><Trash2 class="h-3.5 w-3.5" /></Button>
          </div>
        </button>

        <!-- Expanded: tipo_equipo assignments -->
        <div v-if="senalOpen === s.senal_id" class="border-t border-border bg-muted/10 px-4 py-3">
          <div class="flex items-center justify-between mb-2">
            <div class="text-[10px] uppercase tracking-[0.16em] font-semibold text-muted-foreground">
              Tipos de equipo que usan esta señal
            </div>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openAddSenalXTipo(s.senal_id!)">
              <Plus class="h-3 w-3 mr-1" /> Asignar tipo
            </Button>
          </div>
          <div v-if="!(senalXTipo[s.senal_id!] ?? []).length" class="text-xs text-muted-foreground italic">
            No asignada a ningún tipo de equipo — esta señal no se auto-instanciará al crear equipos.
          </div>
          <div v-else class="flex flex-wrap gap-2">
            <div
              v-for="sxt in senalXTipo[s.senal_id!]"
              :key="sxt.senaltipo_id"
              class="flex items-center gap-1.5 rounded-sm border border-border bg-card px-2.5 py-1 text-xs"
            >
              <component :is="tipoEquipoIcon(sxt.tipo_nombre ?? '')" class="h-3 w-3 text-muted-foreground shrink-0" />
              <span class="font-medium">{{ sxt.tipo_nombre }}</span>
              <span v-if="sxt.num_canales > 1" class="font-mono text-[10px] text-[color:var(--epm-citrico)]">×{{ sxt.num_canales }}</span>
              <button
                class="ml-1 text-muted-foreground hover:text-destructive transition-colors"
                @click="deleteSenalXTipo(sxt)"
              >
                <Trash2 class="h-3 w-3" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <div v-if="filteredSenales.length > senalLimit" class="flex justify-center pt-1">
        <Button variant="outline" size="sm" class="rounded-sm text-xs" @click="senalLimit += SENAL_PAGE">
          Mostrar {{ Math.min(SENAL_PAGE, filteredSenales.length - senalLimit) }} más
          <span class="text-muted-foreground font-mono ml-1.5">({{ filteredSenales.length - senalLimit }} restantes)</span>
        </Button>
      </div>
    </div>

    <!-- ═══════════════════════════════════════════════════════════════════════
         TAB: Tipos & Unidades
    ═══════════════════════════════════════════════════════════════════════ -->
    <div v-if="tab === 'tipos'" class="grid grid-cols-1 lg:grid-cols-3 gap-4">

      <!-- Tipos de Equipo -->
      <Card class="card-soft">
        <CardHeader class="px-4 py-3 border-b border-border">
          <div class="flex items-center justify-between">
            <CardTitle class="text-sm font-semibold flex items-center gap-2">
              <Cpu class="h-4 w-4 text-muted-foreground" /> Tipos de Equipo
            </CardTitle>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateTipoEquipo">
              <Plus class="h-3 w-3 mr-1" /> Nuevo
            </Button>
          </div>
        </CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/30">
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold px-4">Nombre</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Estado</TableHead>
                <TableHead class="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="te in tipoEquipos" :key="te.tipo_id" class="border-b border-border/50 last:border-0">
                <TableCell class="px-4 text-sm font-medium">{{ te.nombre }}</TableCell>
                <TableCell>
                  <span class="text-[10px]" :class="te.activo ? 'text-green-500' : 'text-muted-foreground'">
                    {{ te.activo ? '●' : '○' }}
                  </span>
                </TableCell>
                <TableCell class="text-right pr-2">
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditTipoEquipo(te)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteTipoEquipo(te)"><Trash2 class="h-3 w-3" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!tipoEquipos.length">
                <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-4">Sin tipos de equipo</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <!-- Tipos de Variable -->
      <Card class="card-soft">
        <CardHeader class="px-4 py-3 border-b border-border">
          <div class="flex items-center justify-between">
            <CardTitle class="text-sm font-semibold flex items-center gap-2">
              <Activity class="h-4 w-4 text-muted-foreground" /> Tipos de Variable
            </CardTitle>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateTipoVar">
              <Plus class="h-3 w-3 mr-1" /> Nuevo
            </Button>
          </div>
        </CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/30">
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold px-4">Nombre</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Estado</TableHead>
                <TableHead class="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="tv in tipoVars" :key="tv.tipovar_id" class="border-b border-border/50 last:border-0">
                <TableCell class="px-4 text-sm font-medium">{{ tv.nombre }}</TableCell>
                <TableCell>
                  <span class="text-[10px]" :class="tv.activo ? 'text-green-500' : 'text-muted-foreground'">{{ tv.activo ? '●' : '○' }}</span>
                </TableCell>
                <TableCell class="text-right pr-2">
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditTipoVar(tv)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteTipoVar(tv)"><Trash2 class="h-3 w-3" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!tipoVars.length">
                <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-4">Sin tipos de variable</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <!-- Unidades -->
      <Card class="card-soft">
        <CardHeader class="px-4 py-3 border-b border-border">
          <div class="flex items-center justify-between">
            <CardTitle class="text-sm font-semibold flex items-center gap-2">
              <Ruler class="h-4 w-4 text-muted-foreground" /> Unidades de Medida
            </CardTitle>
            <Button variant="outline" size="sm" class="h-6 text-[11px] rounded-sm" @click="openCreateUnidad">
              <Plus class="h-3 w-3 mr-1" /> Nueva
            </Button>
          </div>
        </CardHeader>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/30">
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold px-4">Símbolo</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Magnitud</TableHead>
                <TableHead class="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="u in unidades" :key="u.unidad_id" class="border-b border-border/50 last:border-0">
                <TableCell class="px-4 font-mono text-sm font-bold text-[color:var(--epm-citrico)]">{{ u.simbolo }}</TableCell>
                <TableCell class="text-xs text-muted-foreground">{{ u.magnitud }}</TableCell>
                <TableCell class="text-right pr-2">
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditUnidad(u)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteUnidad(u)"><Trash2 class="h-3 w-3" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!unidades.length">
                <TableCell colspan="3" class="text-center text-muted-foreground text-xs py-4">Sin unidades</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>

    <!-- ═══════════════════════════════════════════════════════════════════════
         TAB: Estado
    ═══════════════════════════════════════════════════════════════════════ -->
    <div v-if="tab === 'estado'" class="space-y-4">

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

    <!-- ═══════════════════════════════════════════════════════════════════════
         TAB: Pendientes (Sparkplug B auto-discovery)
    ═══════════════════════════════════════════════════════════════════════ -->
    <div v-if="tab === 'pendientes'" class="space-y-4">
      <p class="text-sm text-muted-foreground max-w-3xl">
        Nodos y dispositivos <span class="font-semibold text-foreground">Sparkplug B</span> detectados en el bus,
        agrupados por planta (<span class="font-mono text-[11px]">group_id</span>). Marca varias entidades y
        apruébalas en lote — la planta, el tipo y las señales se derivan del
        <span class="font-mono text-[11px]">NBIRTH/DBIRTH</span> — o entra fila por fila para ajustar el detalle.
      </p>

      <!-- Toolbar: búsqueda + filtro por estado + selección global -->
      <div class="flex flex-wrap items-center gap-3">
        <div class="relative flex-1 min-w-64 max-w-md">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input v-model="autoSearch" placeholder="Buscar group, nodo, device o métrica…" class="pl-8 rounded-sm text-xs" />
        </div>
        <div class="flex rounded-sm border border-border overflow-hidden">
          <button
            v-for="f in [
              { id: 'pending',  label: 'Pendientes', n: autoPendingCount },
              { id: 'approved', label: 'Aprobadas',  n: autoEntities.filter(e => e.status === 'approved').length },
              { id: 'rejected', label: 'Rechazadas', n: autoEntities.filter(e => e.status === 'rejected').length },
              { id: 'all',      label: 'Todas',      n: autoEntities.length },
            ] as const"
            :key="f.id"
            class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] font-medium transition-colors border-r border-border last:border-r-0"
            :class="autoStatusFilter === f.id
              ? 'bg-[color:var(--epm-bosque)] text-white'
              : 'bg-card text-muted-foreground hover:text-foreground'"
            @click="autoStatusFilter = f.id"
          >
            {{ f.label }}
            <span class="font-mono font-bold" :class="autoStatusFilter === f.id ? 'opacity-80' : 'opacity-60'">{{ f.n }}</span>
          </button>
        </div>
        <label
          v-if="visiblePendingAuto.length"
          class="inline-flex items-center gap-2 text-[11px] text-muted-foreground cursor-pointer select-none"
        >
          <input type="checkbox" :checked="allVisibleSelected" class="h-3.5 w-3.5 accent-[color:var(--epm-bosque)]"
            @change="toggleSelectAllVisible" />
          Seleccionar {{ visiblePendingAuto.length }} pendiente(s) visibles
        </label>
        <Button variant="outline" size="sm" class="ml-auto" @click="fetchAutodiscovered">
          <RefreshCw class="h-3.5 w-3.5 mr-1.5" /> Actualizar
        </Button>
      </div>

      <div v-if="!autoGroups.length" class="card-soft p-10 text-center text-xs text-muted-foreground">
        <Scan class="h-6 w-6 mx-auto mb-2 opacity-40" />
        <template v-if="autoSearch || autoStatusFilter !== 'all'">
          Nada coincide con el filtro actual.
          <button class="underline ml-1" @click="autoSearch = ''; autoStatusFilter = 'all'">Ver todas</button>
        </template>
        <template v-else>
          Sin entidades descubiertas. Cuando llegue un NBIRTH/DBIRTH de un nodo no catalogado aparecerá aquí.
        </template>
      </div>

      <!-- Un card por group (= planta Sparkplug): a escala, el operador trabaja
           planta por planta y puede aprobar el nodo + sus devices de una vez -->
      <div v-for="g in autoGroups" :key="g.group"
        class="card-soft overflow-hidden border-l-[3px] !border-l-[color:var(--epm-bosque)]">

        <!-- Group header -->
        <div class="flex items-center gap-3 px-4 py-2.5 bg-[color:color-mix(in_srgb,var(--epm-bosque)_5%,transparent)] border-b border-border">
          <input
            v-if="groupSelectable(g).length"
            type="checkbox" :checked="groupAllSelected(g)"
            class="h-3.5 w-3.5 accent-[color:var(--epm-bosque)] cursor-pointer"
            :title="`Seleccionar las ${groupSelectable(g).length} pendientes de ${g.group}`"
            @change="toggleGroupSelect(g)"
          />
          <span v-else class="w-3.5"></span>
          <Sun class="h-3.5 w-3.5 text-[color:var(--epm-citrico)] shrink-0" />
          <span class="font-mono text-sm font-bold">{{ g.group }}</span>
          <span v-if="g.planta"
            class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm bg-green-500/15 text-green-500 font-semibold"
            :title="'Las aprobaciones caen en la planta existente «' + g.planta.nombre + '»'">
            <CheckCircle2 class="h-3 w-3" /> {{ g.planta.nombre }}
          </span>
          <span v-else
            class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm bg-amber-500/15 text-amber-400 font-semibold"
            title="No existe planta con este group — se creará al aprobar">
            <Plus class="h-3 w-3" /> planta nueva
          </span>
          <span class="ml-auto text-[11px] font-mono text-muted-foreground">
            {{ g.entities.length }} entidad(es)<template v-if="g.pending"> · <span class="text-blue-400 font-semibold">{{ g.pending }} pendiente(s)</span></template>
          </span>
        </div>

        <div class="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow class="bg-muted/30">
              <TableHead class="w-8" />
              <TableHead class="w-8" />
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Entidad Sparkplug</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Métricas reportadas</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Estado</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Visto</TableHead>
              <TableHead class="text-right" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <template v-for="e in g.entities" :key="e.id">
              <TableRow class="border-b border-border/50 hover:bg-muted/20"
                :class="autoSelected.has(e.id) ? 'bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)]' : ''">
                <!-- selection -->
                <TableCell class="align-top pt-3.5">
                  <input
                    v-if="e.status === 'pending'"
                    type="checkbox" :checked="autoSelected.has(e.id)"
                    class="h-3.5 w-3.5 accent-[color:var(--epm-bosque)] cursor-pointer"
                    @change="toggleAutoSelect(e.id)"
                  />
                </TableCell>
                <!-- expand toggle -->
                <TableCell class="align-top pt-3">
                  <button
                    class="h-6 w-6 inline-flex items-center justify-center rounded-sm text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                    :title="expandedAuto.has(e.id) ? 'Contraer' : 'Inspeccionar métricas'"
                    @click="toggleAutoRow(e.id)"
                  >
                    <ChevronDown v-if="expandedAuto.has(e.id)" class="h-4 w-4" />
                    <ChevronRight v-else class="h-4 w-4" />
                  </button>
                </TableCell>

                <!-- entity identity -->
                <TableCell class="align-top">
                  <div class="flex items-center gap-2">
                    <component :is="e.device_id ? Cpu : Radio" class="h-3.5 w-3.5 shrink-0"
                      :class="e.device_id ? 'text-violet-400' : 'text-sky-400'" />
                    <span class="font-mono text-xs font-medium">{{ e.device_id || e.node_id }}</span>
                    <span class="text-[9px] px-1.5 py-0.5 rounded-sm font-bold uppercase tracking-wide"
                      :class="e.device_id ? 'bg-violet-500/15 text-violet-400' : 'bg-sky-500/15 text-sky-400'">
                      {{ e.device_id ? 'DBIRTH' : 'NBIRTH' }}
                    </span>
                  </div>
                  <div class="font-mono text-[10px] text-muted-foreground mt-0.5">
                    {{ e.group_id }} / {{ e.node_id }}{{ e.device_id ? ' / ' + e.device_id : '' }}
                  </div>
                </TableCell>

                <!-- metrics summary -->
                <TableCell class="align-top">
                  <div class="flex items-center gap-2">
                    <span class="inline-flex items-center gap-1 font-mono text-xs font-semibold">
                      <Hash class="h-3 w-3 text-muted-foreground" />{{ e.metric_names.length }}
                    </span>
                    <div class="flex flex-wrap gap-1">
                      <span
                        v-for="kc in entityKindCounts(e)" :key="kc.kind"
                        class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded-sm border"
                        :class="KIND_META[kc.kind].chip"
                      >
                        <span class="h-1.5 w-1.5 rounded-full" :class="KIND_META[kc.kind].dot"></span>
                        {{ kc.n }} {{ KIND_META[kc.kind].label.toLowerCase() }}
                      </span>
                    </div>
                  </div>
                  <div v-if="[...new Set(e.metric_meta.map(m => m.device_type).filter(Boolean))].length"
                    class="text-[10px] text-muted-foreground mt-1">
                    Tipo(s): <span class="font-medium text-foreground">{{ [...new Set(e.metric_meta.map(m => m.device_type).filter(Boolean))].join(', ') }}</span>
                  </div>
                </TableCell>

                <TableCell class="align-top">
                  <span class="text-[10px] px-1.5 py-0.5 rounded-sm font-semibold" :class="{
                    'bg-blue-500/15 text-blue-400':    e.status === 'pending',
                    'bg-green-500/15 text-green-500':  e.status === 'approved',
                    'bg-muted text-muted-foreground':  e.status === 'rejected',
                  }">
                    {{ e.status === 'pending' ? 'Pendiente' : e.status === 'approved' ? 'Aprobado' : 'Rechazado' }}
                  </span>
                </TableCell>

                <TableCell class="align-top font-mono text-[11px] text-muted-foreground whitespace-nowrap">
                  {{ new Date(e.last_seen).toLocaleString() }}
                </TableCell>

                <TableCell class="text-right align-top">
                  <div class="flex justify-end gap-1.5">
                    <Button
                      size="sm" variant="ghost"
                      class="h-7 px-2 text-xs rounded-sm text-muted-foreground hover:text-foreground"
                      title="Ver todas las métricas reportadas"
                      @click="openInspect(e)"
                    >
                      <Eye class="h-3.5 w-3.5" />
                    </Button>
                    <template v-if="e.status === 'pending'">
                      <Button
                        size="sm"
                        class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white h-7 px-2 text-xs rounded-sm"
                        @click="openApprove(e)"
                      >
                        <CheckCircle2 class="h-3.5 w-3.5 mr-1" /> Aprobar
                      </Button>
                      <Button
                        size="sm" variant="ghost"
                        class="text-destructive hover:text-destructive h-7 px-2 text-xs rounded-sm"
                        @click="rejectEntity(e)"
                      >
                        <XCircle class="h-3.5 w-3.5 mr-1" /> Rechazar
                      </Button>
                    </template>
                    <template v-else>
                      <Button
                        size="sm" variant="ghost"
                        class="h-7 px-2 text-xs rounded-sm text-muted-foreground hover:text-foreground"
                        title="Restablecer a pendiente para re-asignar planta o tipo"
                        @click="resetEntity(e)"
                      >
                        <RefreshCw class="h-3 w-3 mr-1" /> Re-asignar
                      </Button>
                      <Button
                        v-if="e.status === 'rejected'"
                        size="sm" variant="ghost"
                        class="h-7 px-2 text-xs rounded-sm text-destructive hover:text-destructive"
                        title="Eliminar definitivamente este registro rechazado"
                        @click="deleteAutodiscoveredEntity(e)"
                      >
                        <Trash2 class="h-3 w-3" />
                      </Button>
                    </template>
                  </div>
                </TableCell>
              </TableRow>

              <!-- expanded inline metric preview -->
              <TableRow v-if="expandedAuto.has(e.id)" class="bg-muted/10 border-b border-border/50">
                <TableCell />
                <TableCell />
                <TableCell colspan="5" class="py-3">
                  <div class="space-y-3">
                    <!-- node properties -->
                    <div v-if="Object.keys(e.node_properties || {}).length" class="flex flex-wrap items-center gap-1.5">
                      <span class="text-[10px] uppercase tracking-wide font-bold text-muted-foreground mr-1">Propiedades del nodo</span>
                      <span v-for="(v, k) in e.node_properties" :key="k"
                        class="inline-flex items-center gap-1 text-[10px] font-mono px-1.5 py-0.5 rounded-sm bg-muted border border-border">
                        <span class="text-muted-foreground">{{ k }}</span><span class="text-foreground font-semibold">{{ v }}</span>
                      </span>
                    </div>

                    <!-- catalog-coverage legend -->
                    <div class="flex flex-wrap items-center gap-3 text-[10px] text-muted-foreground">
                      <span class="inline-flex items-center gap-1">
                        <CheckCircle2 class="h-3 w-3 text-emerald-500" /> en catálogo
                      </span>
                      <span class="inline-flex items-center gap-1">
                        <Plus class="h-3 w-3 text-amber-500" /> señal nueva
                      </span>
                      <span v-if="entityNewSignalCount(e)" class="font-semibold text-amber-500">
                        {{ entityNewSignalCount(e) }} señal(es) sin catalogar — se pueden crear al aprobar
                      </span>
                    </div>

                    <!-- grouped metric chips -->
                    <div v-for="g in groupedPreview(e)" :key="g.name" class="space-y-1">
                      <div class="flex items-center gap-1.5 text-[10px] uppercase tracking-wide font-bold text-muted-foreground">
                        <Boxes class="h-3 w-3" />{{ g.name }}
                        <span class="text-muted-foreground/60 font-normal normal-case">· {{ g.metrics.length }}</span>
                      </div>
                      <div class="flex flex-wrap gap-1.5">
                        <span v-for="m in g.metrics" :key="m.name"
                          class="inline-flex items-center gap-1.5 text-[11px] px-2 py-1 rounded-sm bg-card border"
                          :class="m.kind === 'proceso' && !metricInCatalog(m.name) ? 'border-amber-500/40' : 'border-border'"
                          :title="m.description || m.name">
                          <CheckCircle2 v-if="m.kind === 'proceso' && metricInCatalog(m.name)" class="h-3 w-3 text-emerald-500 shrink-0" />
                          <Plus v-else-if="m.kind === 'proceso'" class="h-3 w-3 text-amber-500 shrink-0" />
                          <span v-else class="h-1.5 w-1.5 rounded-full shrink-0" :class="KIND_META[m.kind].dot"></span>
                          <span class="font-mono">{{ m.leaf }}</span>
                          <span v-if="m.engUnit" class="text-[9px] px-1 rounded bg-muted text-muted-foreground font-semibold">{{ m.engUnit }}</span>
                          <span v-if="m.tipoValor" class="text-[9px] text-muted-foreground italic">{{ m.tipoValor }}</span>
                        </span>
                      </div>
                    </div>

                    <div class="flex items-center gap-2 pt-1">
                      <Button size="sm" variant="outline" class="h-7 px-2.5 text-xs rounded-sm" @click="openInspect(e)">
                        <Eye class="h-3.5 w-3.5 mr-1.5" /> Ver detalle completo
                      </Button>
                      <Button
                        v-if="e.status === 'pending'"
                        size="sm"
                        class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white h-7 px-2.5 text-xs rounded-sm"
                        @click="openApprove(e)"
                      >
                        <CheckCircle2 class="h-3.5 w-3.5 mr-1.5" /> Aprobar y crear equipo
                      </Button>
                    </div>
                  </div>
                </TableCell>
              </TableRow>
            </template>
          </TableBody>
        </Table>
        </div>
      </div>

      <!-- Barra flotante de acciones en lote -->
      <div class="fixed bottom-6 inset-x-0 z-40 flex justify-center pointer-events-none">
      <Transition name="bulkbar">
        <div
          v-if="selectedAuto.length"
          class="pointer-events-auto flex items-center gap-3 rounded-sm border border-[color:var(--epm-bosque-deep)] bg-[color:var(--epm-bosque)] text-white shadow-2xl pl-4 pr-2 py-2"
        >
          <ListChecks class="h-4 w-4 opacity-90" />
          <span class="text-sm font-semibold whitespace-nowrap">
            {{ selectedAuto.length }} seleccionada{{ selectedAuto.length === 1 ? '' : 's' }}
          </span>
          <span class="h-5 w-px bg-white/25"></span>
          <Button
            size="sm"
            class="h-7 px-3 text-xs rounded-sm bg-white text-[color:var(--epm-bosque)] hover:bg-white/90 font-bold"
            @click="openBulkApprove"
          >
            <CheckCircle2 class="h-3.5 w-3.5 mr-1.5" /> Aprobar en lote
          </Button>
          <Button
            size="sm" variant="ghost"
            class="h-7 px-3 text-xs rounded-sm text-white hover:bg-white/15 hover:text-white"
            @click="bulkReject"
          >
            <XCircle class="h-3.5 w-3.5 mr-1.5" /> Rechazar
          </Button>
          <button
            class="h-7 w-7 inline-flex items-center justify-center rounded-sm hover:bg-white/15 transition-colors"
            title="Limpiar selección"
            @click="clearAutoSelection"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>
      </Transition>
      </div>
    </div>

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
