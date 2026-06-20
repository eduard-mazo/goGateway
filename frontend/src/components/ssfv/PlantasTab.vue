<script setup lang="ts">
// SSFV tab component — consumes the shared useSsfv() singleton. Behaviour and
// markup extracted verbatim from SSFVView; the parent gates mounting via v-if.
import { useSsfv } from '@/composables/useSsfv'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import SignalTree from '@/components/SignalTree.vue'
import HwIdentity from '@/components/HwIdentity.vue'
import FleetSummaryBar from '@/components/ssfv/FleetSummaryBar.vue'
import PlantaIec104Link from '@/components/ssfv/PlantaIec104Link.vue'
import { Sun, Plus, Pencil, Trash2, ChevronRight, Cpu, Layers, Link2, GitBranch, TriangleAlert, Search, Cog } from 'lucide-vue-next'
const {
  deleteAsignacion,
  deleteEquipo,
  deleteFrontera,
  deletePlanta,
  equipoOpen,
  equipoSignals,
  estadoClass,
  estadoLabel,
  filteredPlantas,
  fleetSummary,
  FRESH_META,
  freshness,
  iec104Servers,
  lastSegment,
  nodeTree,
  openCreateEquipo,
  openCreateFrontera,
  openCreatePlanta,
  openEditEquipo,
  openEditFrontera,
  openEditPlanta,
  plantaEquipos,
  plantaEstadoFilter,
  plantaFronteras,
  plantaLimit,
  plantaOpen,
  PLANTA_PAGE,
  plantas,
  plantaSearch,
  relTime,
  tipoEquipoIcon,
  toggleEquipo,
  togglePlanta,
  visiblePlantas,} = useSsfv()
</script>

<template>
    <div class="space-y-4">
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

</template>
