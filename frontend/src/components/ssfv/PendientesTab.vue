<script setup lang="ts">
// SSFV tab component — consumes the shared useSsfv() singleton. Behaviour and
// markup extracted verbatim from SSFVView; the parent gates mounting via v-if.
import { useSsfv } from '@/composables/useSsfv'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Sun, Plus, Trash2, RefreshCw, CheckCircle2, XCircle, ChevronRight, ChevronDown, Cpu, Scan, Eye, Search, Hash, Boxes, Radio, ListChecks, X } from 'lucide-vue-next'
const {
  allVisibleSelected,
  autoEntities,
  autoGroups,
  autoPendingCount,
  autoSearch,
  autoSelected,
  autoStatusFilter,
  bulkReject,
  clearAutoSelection,
  deleteAutodiscoveredEntity,
  entityKindCounts,
  entityNewSignalCount,
  expandedAuto,
  fetchAutodiscovered,
  groupAllSelected,
  groupedPreview,
  groupSelectable,
  KIND_META,
  metricInCatalog,
  openApprove,
  openBulkApprove,
  openInspect,
  rejectEntity,
  resetEntity,
  selectedAuto,
  toggleAutoRow,
  toggleAutoSelect,
  toggleGroupSelect,
  toggleSelectAllVisible,
  visiblePendingAuto,} = useSsfv()
</script>

<template>
    <div class="space-y-4">
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

</template>
