<script setup lang="ts">
// SSFV tab component — consumes the shared useSsfv() singleton. Behaviour and
// markup extracted verbatim from SSFVView; the parent gates mounting via v-if.
import { useSsfv } from '@/composables/useSsfv'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Plus, Pencil, Trash2, ChevronRight, Layers } from 'lucide-vue-next'
const {
  deleteSenal,
  deleteSenalXTipo,
  filteredSenales,
  openAddSenalXTipo,
  openCreateSenal,
  openEditSenal,
  senales,
  senalLimit,
  senalOpen,
  SENAL_PAGE,
  senalSearch,
  senalXTipo,
  tipoEquipoIcon,
  toggleSenal,
  visibleSenales,} = useSsfv()
</script>

<template>
    <div class="space-y-4">
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

</template>
