<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { toast } from 'vue-sonner'
import { api, type SSFVStatus, type SSFVPlanta, type SSFVEquipo, type SSFVSenal, type SSFVAsignacion } from '@/api'
import { Button }  from '@/components/ui/button'
import { Input }   from '@/components/ui/input'
import { Label }   from '@/components/ui/label'
import { Switch }  from '@/components/ui/switch'
import {
  Sun, Plus, Pencil, Trash2, RefreshCw, CheckCircle2, XCircle,
  ChevronDown, ChevronRight, Zap,
} from 'lucide-vue-next'

// ─── Tab state ────────────────────────────────────────────────────────────────
const tab = ref<'estado' | 'plantas' | 'senales' | 'monitoreo'>('estado')

// ─── Status ──────────────────────────────────────────────────────────────────
const status = ref<SSFVStatus | null>(null)
async function fetchStatus() {
  try {
    const r = await api.get('/ssfv/status')
    status.value = r.data
  } catch { status.value = null }
}

// ─── Catalogs (shared dropdowns) ──────────────────────────────────────────────
const tipoEquipos  = ref<any[]>([])
const tipoVars     = ref<any[]>([])
const unidades     = ref<any[]>([])

async function fetchCatalogs() {
  const [te, tv, u] = await Promise.all([
    api.get('/ssfv/tipo-equipo'),
    api.get('/ssfv/tipo-variable'),
    api.get('/ssfv/unidades'),
  ])
  tipoEquipos.value = te.data
  tipoVars.value    = tv.data
  unidades.value    = u.data
}

// ─── Plantas ─────────────────────────────────────────────────────────────────
const plantas        = ref<SSFVPlanta[]>([])
const plantaForm     = ref<SSFVPlanta>(emptyPlanta())
const plantaEditing  = ref<number | null>(null)
const plantaExpanded = ref<number | null>(null)

// Sparkplug B auto-derivation from broker_base
const spGroupId   = computed(() => plantaForm.value.broker_base.split('/')[1] ?? '')
const spNodeId    = computed(() => plantaForm.value.broker_base.split('/')[3] ?? '')

// Equipos nested per planta
const plantaEquipos     = ref<Record<number, SSFVEquipo[]>>({})
const equipoForm        = ref<SSFVEquipo & { nombre_topic_suffix: string }>(emptyEquipo())
const equipoEditing     = ref<number | null>(null)
const showEquipoForm    = ref<number | null>(null) // planta_id whose form is open
// Signals panel per equipo
const equipoSenales     = ref<Record<number, any[]>>({})
const senalPanelEquipo  = ref<number | null>(null)

function emptyPlanta(): SSFVPlanta {
  return { nombre: '', broker_base: '', estado: 1 }
}
function emptyEquipo(plantaId = 0): SSFVEquipo & { nombre_topic_suffix: string } {
  return { planta_id: plantaId, tipo_id: 0, nombre_equipo: '', nombre_topic: '', estado: 1, nombre_topic_suffix: '' }
}

// Auto-build nombre_topic from broker_base + suffix
watch(() => equipoForm.value.nombre_topic_suffix, (suffix) => {
  if (!plantaExpanded.value) return
  const planta = plantas.value.find(p => p.planta_id === plantaExpanded.value)
  if (planta && suffix && !equipoEditing.value) {
    equipoForm.value.nombre_topic = planta.broker_base + '/' + suffix
  }
})

async function fetchPlantas() {
  const r = await api.get('/ssfv/plantas')
  plantas.value = r.data
}

async function loadEquiposForPlanta(plantaId: number) {
  const r = await api.get(`/ssfv/plantas/${plantaId}/equipos`)
  plantaEquipos.value = { ...plantaEquipos.value, [plantaId]: r.data }
}

async function loadSenalesForEquipo(equipoId: number) {
  const r = await api.get(`/ssfv/equipos/${equipoId}/senales`)
  equipoSenales.value = { ...equipoSenales.value, [equipoId]: r.data }
}

function toggleSenalPanel(equipoId: number) {
  if (senalPanelEquipo.value === equipoId) {
    senalPanelEquipo.value = null
  } else {
    senalPanelEquipo.value = equipoId
    loadSenalesForEquipo(equipoId)
  }
}

function togglePlanta(id: number) {
  if (plantaExpanded.value === id) {
    plantaExpanded.value = null
  } else {
    plantaExpanded.value = id
    loadEquiposForPlanta(id)
  }
}

function editPlanta(p: SSFVPlanta) {
  plantaEditing.value = p.planta_id!
  plantaForm.value = { ...p }
}

function cancelPlanta() {
  plantaEditing.value = null
  plantaForm.value = emptyPlanta()
}

async function savePlanta() {
  try {
    if (plantaEditing.value) {
      await api.put(`/ssfv/plantas/${plantaEditing.value}`, plantaForm.value)
      toast.success('Planta actualizada')
    } else {
      await api.post('/ssfv/plantas', plantaForm.value)
      toast.success('Planta creada')
    }
    cancelPlanta()
    await fetchPlantas()
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error guardando planta')
  }
}

async function deletePlanta(id: number) {
  if (!confirm('¿Eliminar planta? Se eliminan sus equipos y asignaciones.')) return
  try {
    await api.delete(`/ssfv/plantas/${id}`)
    toast.success('Planta eliminada')
    await fetchPlantas()
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error eliminando planta')
  }
}

function openEquipoForm(plantaId: number) {
  showEquipoForm.value = plantaId
  equipoEditing.value  = null
  equipoForm.value     = emptyEquipo(plantaId)
}

function editEquipo(eq: SSFVEquipo) {
  showEquipoForm.value = eq.planta_id
  equipoEditing.value  = eq.equipo_id!
  equipoForm.value     = { ...eq, nombre_topic_suffix: eq.nombre_topic.split('/').pop() ?? '' }
}

function cancelEquipo() {
  showEquipoForm.value = null
  equipoEditing.value  = null
  equipoForm.value     = emptyEquipo()
}

async function saveEquipo() {
  try {
    if (equipoEditing.value) {
      await api.put(`/ssfv/equipos/${equipoEditing.value}`, equipoForm.value)
      toast.success('Equipo actualizado')
    } else {
      await api.post('/ssfv/equipos', equipoForm.value)
      toast.success('Equipo creado')
    }
    const pid = equipoForm.value.planta_id
    cancelEquipo()
    await loadEquiposForPlanta(pid)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error guardando equipo')
  }
}

async function deleteEquipo(eq: SSFVEquipo) {
  if (!confirm('¿Eliminar equipo?')) return
  try {
    await api.delete(`/ssfv/equipos/${eq.equipo_id}`)
    toast.success('Equipo eliminado')
    await loadEquiposForPlanta(eq.planta_id)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error eliminando equipo')
  }
}

// ─── Señales ──────────────────────────────────────────────────────────────────
const senales       = ref<SSFVSenal[]>([])
const senalForm     = ref<SSFVSenal>(emptySenal())
const senalEditing  = ref<number | null>(null)
const senalSearch   = ref('')

// Asignaciones per signal
const senalAsign     = ref<Record<number, SSFVAsignacion[]>>({})
const asignExpanded  = ref<number | null>(null)
const asignForm      = ref<SSFVAsignacion>(emptyAsign())
const asignEditing   = ref<number | null>(null)
const showAsignForm  = ref<number | null>(null)

// All equipos flat (for asignacion dropdown)
const allEquipos = ref<SSFVEquipo[]>([])

function emptySenal(): SSFVSenal {
  return { tipavar_id: 0, unidad_id: 0, nombre: '', tipo_valor: 'Instantaneo', codigo_senal: '', es_indexada: false, activo: true }
}
function emptyAsign(senalId = 0): SSFVAsignacion {
  return { senal_id: senalId, equipo_id: 0, nombre_instancia: '', activo: true }
}

const filteredSenales = computed(() => {
  const q = senalSearch.value.toLowerCase()
  if (!q) return senales.value
  return senales.value.filter(s =>
    s.nombre.toLowerCase().includes(q) ||
    s.codigo_senal.toLowerCase().includes(q),
  )
})

async function fetchSenales() {
  const r = await api.get('/ssfv/senales')
  senales.value = r.data
}

async function fetchAllEquipos() {
  const r = await api.get('/ssfv/equipos')
  allEquipos.value = r.data
}

function editSenal(s: SSFVSenal) {
  senalEditing.value = s.senal_id!
  senalForm.value    = { ...s }
}

function cancelSenal() {
  senalEditing.value = null
  senalForm.value    = emptySenal()
}

async function saveSenal() {
  try {
    if (senalEditing.value) {
      await api.put(`/ssfv/senales/${senalEditing.value}`, senalForm.value)
      toast.success('Señal actualizada')
    } else {
      await api.post('/ssfv/senales', senalForm.value)
      toast.success('Señal creada')
    }
    cancelSenal()
    await fetchSenales()
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error guardando señal')
  }
}

async function deleteSenal(id: number) {
  if (!confirm('¿Eliminar señal?')) return
  try {
    await api.delete(`/ssfv/senales/${id}`)
    toast.success('Señal eliminada')
    await fetchSenales()
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error eliminando señal')
  }
}

function toggleSenal(id: number) {
  if (asignExpanded.value === id) {
    asignExpanded.value = null
  } else {
    asignExpanded.value = id
    loadAsign(id)
  }
}

async function loadAsign(senalId: number) {
  const r = await api.get('/ssfv/asignaciones', { params: { equipo_id: undefined } })
  senalAsign.value = { ...senalAsign.value, [senalId]: (r.data as SSFVAsignacion[]).filter(a => a.senal_id === senalId) }
}

function openAsignForm(senalId: number) {
  showAsignForm.value = senalId
  asignEditing.value  = null
  asignForm.value     = emptyAsign(senalId)
}

function editAsign(a: SSFVAsignacion) {
  showAsignForm.value = a.senal_id
  asignEditing.value  = a.equisenal_id!
  asignForm.value     = { ...a }
}

function cancelAsign() {
  showAsignForm.value = null
  asignEditing.value  = null
  asignForm.value     = emptyAsign()
}

async function saveAsign() {
  try {
    if (asignEditing.value) {
      await api.put(`/ssfv/asignaciones/${asignEditing.value}`, asignForm.value)
      toast.success('Asignación actualizada')
    } else {
      await api.post('/ssfv/asignaciones', asignForm.value)
      toast.success('Asignación creada')
    }
    const sid = asignForm.value.senal_id
    cancelAsign()
    await loadAsign(sid)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error guardando asignación')
  }
}

async function deleteAsign(a: SSFVAsignacion) {
  if (!confirm('¿Eliminar asignación?')) return
  try {
    await api.delete(`/ssfv/asignaciones/${a.equisenal_id}`)
    toast.success('Asignación eliminada')
    await loadAsign(a.senal_id)
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error eliminando asignación')
  }
}

// ─── Monitoreo ────────────────────────────────────────────────────────────────
const ultimasLecturas = ref<any[]>([])
const alarmasActivas  = ref<any[]>([])
const rawRows         = ref<any[]>([])
const monTab = ref<'lecturas' | 'alarmas' | 'raw'>('lecturas')
const refreshing = ref(false)

async function fetchMonitoreo() {
  refreshing.value = true
  try {
    const [ul, aa, rr] = await Promise.all([
      api.get('/ssfv/vista/ultimas-lecturas'),
      api.get('/ssfv/vista/alarmas-activas'),
      api.get('/ssfv/vista/raw', { params: { limit: 50 } }),
    ])
    ultimasLecturas.value = ul.data
    alarmasActivas.value  = aa.data
    rawRows.value         = rr.data
  } catch (e: any) {
    toast.error('Error cargando monitoreo: ' + (e.response?.data?.error ?? e.message))
  } finally {
    refreshing.value = false
  }
}

async function invalidateCache() {
  try {
    await api.post('/ssfv/cache/invalidate')
    toast.success('Cache de señales invalidado')
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error invalidando cache')
  }
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────
onMounted(async () => {
  await fetchStatus()
  if (status.value?.connected) {
    await Promise.all([fetchCatalogs(), fetchPlantas()])
  }
})

watch(tab, async (t) => {
  if (!status.value?.connected) return
  if (t === 'plantas') { await Promise.all([fetchPlantas(), fetchCatalogs()]) }
  if (t === 'senales') { await Promise.all([fetchSenales(), fetchAllEquipos(), fetchCatalogs()]) }
  if (t === 'monitoreo') { await fetchMonitoreo() }
})
</script>

<template>
  <div class="p-6 space-y-6 max-w-7xl mx-auto">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div class="grid place-items-center w-10 h-10 rounded-sm bg-amber-500/20 text-amber-400">
          <Sun class="h-5 w-5" />
        </div>
        <div>
          <h1 class="font-heading text-2xl">Plantas Solares (SSFV)</h1>
          <p class="text-xs text-muted-foreground">Gestión del catálogo fotovoltaico</p>
        </div>
      </div>
      <!-- Status badge -->
      <div v-if="status" class="flex items-center gap-2 px-3 py-1.5 rounded-sm border text-xs font-mono"
        :class="status.connected
          ? 'border-green-500/40 bg-green-500/10 text-green-400'
          : 'border-red-500/40 bg-red-500/10 text-red-400'">
        <component :is="status.connected ? CheckCircle2 : XCircle" class="h-3.5 w-3.5" />
        {{ status.connected ? 'Conectado' : 'Sin conexión' }}
        <span v-if="status.connected && status.write_rate !== undefined" class="text-muted-foreground">
          · {{ status.write_rate?.toFixed(0) }} pt/s
        </span>
      </div>
    </div>

    <!-- Warning: adapter not connected -->
    <div v-if="!status?.connected"
      class="rounded-sm border border-amber-500/40 bg-amber-500/10 p-4 text-sm text-amber-300">
      El adaptador SSFV no está conectado. Configure TimescaleDB en la sección TSDB Pipeline y aplique
      la migración <code class="font-mono bg-black/30 px-1 rounded">000007_ssfv_schema.up.sql</code> primero.
    </div>

    <!-- Tabs -->
    <div class="flex border-b border-border gap-1">
      <button v-for="t in ([
        { id: 'estado',   label: 'Estado' },
        { id: 'plantas',  label: 'Plantas & Equipos' },
        { id: 'senales',  label: 'Catálogo Señales' },
        { id: 'monitoreo',label: 'Monitoreo' },
      ] as const)"
        :key="t.id"
        class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px"
        :class="tab === t.id
          ? 'border-primary text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'"
        @click="tab = t.id">
        {{ t.label }}
      </button>
    </div>

    <!-- ── TAB: Estado ──────────────────────────────────────────────────────── -->
    <div v-if="tab === 'estado'" class="space-y-4">
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div class="rounded-sm border border-border bg-card p-4 space-y-1">
          <div class="text-xs text-muted-foreground uppercase tracking-wider">Conexión</div>
          <div class="font-mono text-sm" :class="status?.connected ? 'text-green-400' : 'text-red-400'">
            {{ status?.connected ? 'Activa' : 'Inactiva' }}
          </div>
        </div>
        <div class="rounded-sm border border-border bg-card p-4 space-y-1">
          <div class="text-xs text-muted-foreground uppercase tracking-wider">Circuit</div>
          <div class="font-mono text-sm" :class="status?.circuit_open ? 'text-red-400' : 'text-green-400'">
            {{ status?.circuit_open ? 'Abierto' : 'Cerrado' }}
          </div>
        </div>
        <div class="rounded-sm border border-border bg-card p-4 space-y-1">
          <div class="text-xs text-muted-foreground uppercase tracking-wider">Escritura</div>
          <div class="font-mono text-sm">{{ status?.write_rate?.toFixed(1) ?? '—' }} pt/s</div>
        </div>
        <div class="rounded-sm border border-border bg-card p-4 space-y-1">
          <div class="text-xs text-muted-foreground uppercase tracking-wider">Errores</div>
          <div class="font-mono text-sm" :class="(status?.error_rate ?? 0) > 0 ? 'text-red-400' : ''">
            {{ status?.error_rate ?? '—' }}
          </div>
        </div>
      </div>
      <div v-if="status?.last_error" class="rounded-sm border border-red-500/30 bg-red-500/10 p-3 text-xs font-mono text-red-300">
        {{ status.last_error }}
      </div>
      <Button variant="outline" size="sm" @click="invalidateCache" :disabled="!status?.connected">
        <Zap class="h-3.5 w-3.5 mr-1.5" /> Invalidar Cache
      </Button>
    </div>

    <!-- ── TAB: Plantas & Equipos ───────────────────────────────────────────── -->
    <div v-if="tab === 'plantas'" class="space-y-4">
      <!-- Planta form -->
      <div class="rounded-sm border border-border bg-card p-4 space-y-3">
        <h3 class="text-sm font-semibold">{{ plantaEditing ? 'Editar Planta' : 'Nueva Planta' }}</h3>
        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3">
          <div class="space-y-1">
            <Label class="text-xs">Nombre *</Label>
            <Input v-model="plantaForm.nombre" placeholder="Sede30" class="h-8 text-xs" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Broker Base *</Label>
            <Input v-model="plantaForm.broker_base" placeholder="EPM/SSFV/EPM/Sede30" class="h-8 text-xs font-mono" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Ubicación</Label>
            <Input v-model="plantaForm.ubicacion" placeholder="Medellín" class="h-8 text-xs" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Propietario</Label>
            <Input v-model="plantaForm.propietario" placeholder="EPM" class="h-8 text-xs" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Capacidad kWp</Label>
            <Input v-model.number="plantaForm.capacidad_kWp" type="number" step="0.01" class="h-8 text-xs" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Estado</Label>
            <select v-model.number="plantaForm.estado" class="w-full h-8 rounded-sm border border-input bg-background px-2 text-xs">
              <option :value="1">Activa</option>
              <option :value="0">Inactiva</option>
              <option :value="2">Mantenimiento</option>
            </select>
          </div>
        </div>
        <!-- Sparkplug B derivation from broker_base -->
        <div v-if="plantaForm.broker_base" class="rounded-sm border border-border bg-muted/20 px-3 py-2 text-xs space-y-1">
          <div class="text-muted-foreground uppercase tracking-wider text-[10px]">Sparkplug B (derivado de broker_base)</div>
          <div class="font-mono flex gap-4">
            <span><span class="text-muted-foreground">group_id: </span><span class="text-amber-400">{{ spGroupId || '—' }}</span></span>
            <span><span class="text-muted-foreground">node_id: </span><span class="text-amber-400">{{ spNodeId || '—' }}</span></span>
          </div>
          <div class="text-muted-foreground font-mono truncate">
            spBv1.0/{{ spGroupId }}/DDATA/{{ spNodeId }}/&lt;device_id&gt;
          </div>
        </div>
        <div class="flex gap-2 pt-1">
          <Button size="sm" @click="savePlanta" :disabled="!plantaForm.nombre || !plantaForm.broker_base">
            <Plus class="h-3.5 w-3.5 mr-1" /> {{ plantaEditing ? 'Guardar' : 'Crear' }}
          </Button>
          <Button v-if="plantaEditing" variant="ghost" size="sm" @click="cancelPlanta">Cancelar</Button>
        </div>
      </div>

      <!-- Planta list -->
      <div v-for="p in plantas" :key="p.planta_id" class="rounded-sm border border-border bg-card">
        <!-- Planta header -->
        <div class="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-muted/30"
          @click="togglePlanta(p.planta_id!)">
          <component :is="plantaExpanded === p.planta_id ? ChevronDown : ChevronRight" class="h-4 w-4 text-muted-foreground shrink-0" />
          <div class="flex-1 min-w-0">
            <div class="font-medium text-sm">{{ p.nombre }}</div>
            <div class="text-xs text-muted-foreground font-mono">{{ p.broker_base }}</div>
          </div>
          <div class="flex items-center gap-1 shrink-0" @click.stop>
            <span class="text-xs px-2 py-0.5 rounded-sm font-mono"
              :class="p.estado === 1 ? 'bg-green-500/20 text-green-400' : 'bg-muted text-muted-foreground'">
              {{ p.estado === 1 ? 'Activa' : p.estado === 0 ? 'Inactiva' : 'Mtto' }}
            </span>
            <Button variant="ghost" size="icon" class="h-7 w-7" @click="editPlanta(p)">
              <Pencil class="h-3.5 w-3.5" />
            </Button>
            <Button variant="ghost" size="icon" class="h-7 w-7 text-destructive" @click="deletePlanta(p.planta_id!)">
              <Trash2 class="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>

        <!-- Equipos expanded -->
        <div v-if="plantaExpanded === p.planta_id" class="border-t border-border px-4 py-3 space-y-3 bg-muted/10">
          <!-- Equipo list -->
          <div v-for="eq in (plantaEquipos[p.planta_id!] ?? [])" :key="eq.equipo_id"
            class="rounded-sm border border-border bg-card">
            <div class="flex items-center gap-3 px-3 py-2">
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium">{{ eq.nombre_equipo }}
                  <span class="text-xs text-muted-foreground ml-2">{{ eq.tipo_nombre }}</span>
                </div>
                <div class="text-xs font-mono text-muted-foreground truncate">{{ eq.nombre_topic }}</div>
              </div>
              <div class="flex items-center gap-1 shrink-0">
                <Button variant="ghost" size="sm" class="h-7 text-xs text-muted-foreground"
                  @click="toggleSenalPanel(eq.equipo_id!)">
                  <component :is="senalPanelEquipo === eq.equipo_id ? ChevronDown : ChevronRight" class="h-3 w-3 mr-1" />
                  Señales
                </Button>
                <Button variant="ghost" size="icon" class="h-7 w-7" @click="editEquipo(eq)">
                  <Pencil class="h-3.5 w-3.5" />
                </Button>
                <Button variant="ghost" size="icon" class="h-7 w-7 text-destructive" @click="deleteEquipo(eq)">
                  <Trash2 class="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
            <!-- Signal panel per equipo -->
            <div v-if="senalPanelEquipo === eq.equipo_id"
              class="border-t border-border bg-muted/10 px-3 py-2">
              <div class="text-[10px] text-muted-foreground uppercase tracking-wider mb-2">
                Señales instanciadas — {{ eq.nombre_equipo }}
              </div>
              <div v-if="!equipoSenales[eq.equipo_id!]?.length" class="text-xs text-muted-foreground py-1">
                Sin señales instanciadas
              </div>
              <table v-else class="w-full text-xs">
                <thead>
                  <tr class="text-[10px] text-muted-foreground">
                    <th class="text-left pr-3 py-0.5 font-medium">Código</th>
                    <th class="text-left pr-3 py-0.5 font-medium">Nombre</th>
                    <th class="text-left pr-3 py-0.5 font-medium">Unidad</th>
                    <th class="text-left pr-3 py-0.5 font-medium">Tipo</th>
                    <th class="text-right py-0.5 font-medium font-mono text-amber-400/70">equisenal_id</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="s in equipoSenales[eq.equipo_id!]" :key="s.equisenal_id"
                    class="border-t border-border/50 hover:bg-muted/20">
                    <td class="pr-3 py-0.5 font-mono text-amber-400">{{ s.nombre_instancia }}</td>
                    <td class="pr-3 py-0.5 truncate max-w-[180px]">{{ s.senal_nombre }}</td>
                    <td class="pr-3 py-0.5 text-muted-foreground">{{ s.unidad }}</td>
                    <td class="pr-3 py-0.5">
                      <span v-if="s.es_alarma" class="px-1 rounded-sm bg-red-500/20 text-red-400 text-[10px]">Alarma</span>
                      <span v-else class="text-muted-foreground text-[10px]">{{ s.tipo_valor }}</span>
                    </td>
                    <td class="py-0.5 text-right font-mono text-[10px]">
                      <span class="px-1.5 py-0.5 rounded-sm bg-muted text-muted-foreground">{{ s.equisenal_id }}</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Equipo form -->
          <div v-if="showEquipoForm === p.planta_id || equipoEditing && equipoForm.planta_id === p.planta_id"
            class="rounded-sm border border-border p-3 space-y-2 bg-card">
            <h4 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              {{ equipoEditing ? 'Editar equipo' : 'Nuevo equipo' }}
            </h4>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <div class="space-y-1">
                <Label class="text-xs">Tipo *</Label>
                <select v-model.number="equipoForm.tipo_id" class="w-full h-8 rounded-sm border border-input bg-background px-2 text-xs">
                  <option :value="0" disabled>Seleccionar…</option>
                  <option v-for="te in tipoEquipos" :key="te.Tipo_Id" :value="te.Tipo_Id">{{ te.Nombre }}</option>
                </select>
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Nombre *</Label>
                <Input v-model="equipoForm.nombre_equipo" placeholder="Inversor 1" class="h-8 text-xs" />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Sufijo topic (device_id) *</Label>
                <Input v-model="equipoForm.nombre_topic_suffix" placeholder="INV_1" class="h-8 text-xs font-mono" />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Nombre Topic (calculado)</Label>
                <Input v-model="equipoForm.nombre_topic" class="h-8 text-xs font-mono bg-muted/30" />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Fabricante</Label>
                <Input v-model="equipoForm.fabricante" class="h-8 text-xs" />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Modelo</Label>
                <Input v-model="equipoForm.modelo" class="h-8 text-xs" />
              </div>
            </div>
            <div class="flex gap-2 pt-1">
              <Button size="sm" @click="saveEquipo"
                :disabled="!equipoForm.nombre_equipo || !equipoForm.nombre_topic || !equipoForm.tipo_id">
                {{ equipoEditing ? 'Guardar' : 'Crear' }}
              </Button>
              <Button variant="ghost" size="sm" @click="cancelEquipo">Cancelar</Button>
            </div>
          </div>

          <Button v-if="showEquipoForm !== p.planta_id" variant="outline" size="sm"
            class="w-full border-dashed" @click="openEquipoForm(p.planta_id!)">
            <Plus class="h-3.5 w-3.5 mr-1" /> Agregar equipo
          </Button>
        </div>
      </div>

      <p v-if="!plantas.length" class="text-sm text-muted-foreground text-center py-6">
        No hay plantas configuradas
      </p>
    </div>

    <!-- ── TAB: Catálogo Señales ─────────────────────────────────────────────── -->
    <div v-if="tab === 'senales'" class="space-y-4">
      <!-- Senal form -->
      <div class="rounded-sm border border-border bg-card p-4 space-y-3">
        <h3 class="text-sm font-semibold">{{ senalEditing ? 'Editar Señal' : 'Nueva Señal' }}</h3>
        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3">
          <div class="space-y-1">
            <Label class="text-xs">Código *</Label>
            <Input v-model="senalForm.codigo_senal" placeholder="AP" class="h-8 text-xs font-mono" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Nombre *</Label>
            <Input v-model="senalForm.nombre" placeholder="Potencia Activa" class="h-8 text-xs" />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Tipo Variable *</Label>
            <select v-model.number="senalForm.tipavar_id" class="w-full h-8 rounded-sm border border-input bg-background px-2 text-xs">
              <option :value="0" disabled>Seleccionar…</option>
              <option v-for="tv in tipoVars" :key="tv.TipoVar_Id" :value="tv.TipoVar_Id">{{ tv.Nombre }}</option>
            </select>
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Unidad *</Label>
            <select v-model.number="senalForm.unidad_id" class="w-full h-8 rounded-sm border border-input bg-background px-2 text-xs">
              <option :value="0" disabled>Seleccionar…</option>
              <option v-for="u in unidades" :key="u.Unidad_Id" :value="u.Unidad_Id">{{ u.Simbolo }} – {{ u.Nombre }}</option>
            </select>
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Tipo Valor</Label>
            <select v-model="senalForm.tipo_valor" class="w-full h-8 rounded-sm border border-input bg-background px-2 text-xs">
              <option value="Instantaneo">Instantáneo</option>
              <option value="Acumulado">Acumulado</option>
            </select>
          </div>
          <div class="space-y-1">
            <Label class="text-xs">Descripción</Label>
            <Input v-model="senalForm.descripcion" class="h-8 text-xs" />
          </div>
          <div class="flex items-center gap-4 pt-4">
            <label class="flex items-center gap-2 text-xs cursor-pointer">
              <Switch v-model:checked="senalForm.es_indexada" /> Indexada
            </label>
            <label class="flex items-center gap-2 text-xs cursor-pointer">
              <Switch v-model:checked="senalForm.activo" /> Activa
            </label>
          </div>
        </div>
        <div class="flex gap-2 pt-1">
          <Button size="sm" @click="saveSenal"
            :disabled="!senalForm.codigo_senal || !senalForm.nombre || !senalForm.tipavar_id || !senalForm.unidad_id">
            <Plus class="h-3.5 w-3.5 mr-1" /> {{ senalEditing ? 'Guardar' : 'Crear' }}
          </Button>
          <Button v-if="senalEditing" variant="ghost" size="sm" @click="cancelSenal">Cancelar</Button>
        </div>
      </div>

      <!-- Search -->
      <Input v-model="senalSearch" placeholder="Filtrar señales…" class="h-8 text-xs max-w-sm" />

      <!-- Senal list -->
      <div v-for="s in filteredSenales" :key="s.senal_id" class="rounded-sm border border-border bg-card">
        <div class="flex items-center gap-3 px-4 py-2.5 cursor-pointer hover:bg-muted/30"
          @click="toggleSenal(s.senal_id!)">
          <component :is="asignExpanded === s.senal_id ? ChevronDown : ChevronRight"
            class="h-4 w-4 text-muted-foreground shrink-0" />
          <code class="text-xs font-mono text-amber-400 w-20 shrink-0">{{ s.codigo_senal }}</code>
          <div class="flex-1 min-w-0">
            <div class="text-sm">{{ s.nombre }}</div>
            <div class="text-xs text-muted-foreground">{{ s.tipo_var_nombre }} · {{ s.unidad_simbolo }} · {{ s.tipo_valor }}</div>
          </div>
          <div class="flex items-center gap-1 shrink-0" @click.stop>
            <span class="text-xs px-1.5 py-0.5 rounded-sm"
              :class="s.activo ? 'bg-green-500/20 text-green-400' : 'bg-muted text-muted-foreground'">
              {{ s.activo ? 'Activa' : 'Inactiva' }}
            </span>
            <span v-if="s.es_indexada" class="text-xs px-1.5 py-0.5 rounded-sm bg-blue-500/20 text-blue-400">idx</span>
            <Button variant="ghost" size="icon" class="h-7 w-7" @click="editSenal(s)">
              <Pencil class="h-3.5 w-3.5" />
            </Button>
            <Button variant="ghost" size="icon" class="h-7 w-7 text-destructive" @click="deleteSenal(s.senal_id!)">
              <Trash2 class="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>

        <!-- Asignaciones expanded -->
        <div v-if="asignExpanded === s.senal_id" class="border-t border-border px-4 py-3 space-y-2 bg-muted/10">
          <div v-for="a in (senalAsign[s.senal_id!] ?? [])" :key="a.equisenal_id"
            class="flex items-center gap-3 rounded-sm border border-border bg-card px-3 py-2 text-xs">
            <code class="font-mono text-muted-foreground flex-1 truncate">{{ a.signal_path }}</code>
            <span :class="a.activo ? 'text-green-400' : 'text-muted-foreground'">{{ a.activo ? '●' : '○' }}</span>
            <Button variant="ghost" size="icon" class="h-6 w-6" @click="editAsign(a)">
              <Pencil class="h-3 w-3" />
            </Button>
            <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteAsign(a)">
              <Trash2 class="h-3 w-3" />
            </Button>
          </div>

          <!-- Asign form -->
          <div v-if="showAsignForm === s.senal_id || (asignEditing && asignForm.senal_id === s.senal_id)"
            class="rounded-sm border border-border p-3 space-y-2 bg-card">
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <div class="space-y-1">
                <Label class="text-xs">Equipo *</Label>
                <select v-model.number="asignForm.equipo_id" class="w-full h-8 rounded-sm border border-input bg-background px-2 text-xs">
                  <option :value="0" disabled>Seleccionar…</option>
                  <option v-for="eq in allEquipos" :key="eq.equipo_id" :value="eq.equipo_id">
                    {{ eq.nombre_equipo }} ({{ eq.planta_nombre }})
                  </option>
                </select>
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Nombre Instancia *</Label>
                <Input v-model="asignForm.nombre_instancia" placeholder="AP" class="h-8 text-xs font-mono" />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">Índice Canal</Label>
                <Input v-model.number="asignForm.indice_canal" type="number" min="1" class="h-8 text-xs" />
              </div>
              <div class="flex items-center gap-2 pt-4">
                <Switch v-model:checked="asignForm.activo" />
                <span class="text-xs">Activa</span>
              </div>
            </div>
            <div class="flex gap-2 pt-1">
              <Button size="sm" @click="saveAsign"
                :disabled="!asignForm.equipo_id || !asignForm.nombre_instancia">
                {{ asignEditing ? 'Guardar' : 'Crear' }}
              </Button>
              <Button variant="ghost" size="sm" @click="cancelAsign">Cancelar</Button>
            </div>
          </div>

          <Button v-if="showAsignForm !== s.senal_id" variant="outline" size="sm"
            class="w-full border-dashed text-xs" @click="openAsignForm(s.senal_id!)">
            <Plus class="h-3 w-3 mr-1" /> Asignar a equipo
          </Button>
        </div>
      </div>

      <p v-if="!filteredSenales.length" class="text-sm text-muted-foreground text-center py-6">
        No hay señales en el catálogo
      </p>
    </div>

    <!-- ── TAB: Monitoreo ────────────────────────────────────────────────────── -->
    <div v-if="tab === 'monitoreo'" class="space-y-4">
      <div class="flex items-center gap-2">
        <div class="flex border-b border-border gap-1 flex-1">
          <button v-for="mt in ([
            { id: 'lecturas', label: 'Últimas Lecturas' },
            { id: 'alarmas',  label: 'Alarmas Activas' },
            { id: 'raw',      label: 'Raw (sin mapear)' },
          ] as const)"
            :key="mt.id"
            class="px-3 py-1.5 text-xs font-medium transition-colors border-b-2 -mb-px"
            :class="monTab === mt.id
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'"
            @click="monTab = mt.id">
            {{ mt.label }}
            <span v-if="mt.id === 'alarmas' && alarmasActivas.length"
              class="ml-1.5 text-[10px] px-1.5 rounded-full bg-red-500/20 text-red-400">
              {{ alarmasActivas.length }}
            </span>
          </button>
        </div>
        <Button variant="outline" size="sm" @click="fetchMonitoreo" :disabled="refreshing" class="shrink-0">
          <RefreshCw class="h-3.5 w-3.5 mr-1.5" :class="{ 'animate-spin': refreshing }" /> Actualizar
        </Button>
      </div>

      <!-- Últimas lecturas -->
      <div v-if="monTab === 'lecturas'" class="overflow-x-auto rounded-sm border border-border">
        <table class="w-full text-xs">
          <thead class="bg-muted/50">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">EquiSenal_Id</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Timestamp UTC</th>
              <th class="px-3 py-2 text-right font-medium text-muted-foreground">Valor</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Calidad</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <tr v-for="row in ultimasLecturas" :key="row.EquiSenal_Id" class="hover:bg-muted/20">
              <td class="px-3 py-1.5 font-mono">{{ row.EquiSenal_Id }}</td>
              <td class="px-3 py-1.5 font-mono text-muted-foreground">{{ row.Timestamp_UTC }}</td>
              <td class="px-3 py-1.5 font-mono text-right">{{ row.Valor }}</td>
              <td class="px-3 py-1.5">
                <span class="px-1.5 py-0.5 rounded-sm text-[10px]"
                  :class="{
                    'bg-green-500/20 text-green-400': row.Calidad === 'Buena',
                    'bg-amber-500/20 text-amber-400': row.Calidad === 'Dudosa',
                    'bg-red-500/20 text-red-400': row.Calidad === 'Mala',
                  }">
                  {{ row.Calidad }}
                </span>
              </td>
            </tr>
            <tr v-if="!ultimasLecturas.length">
              <td colspan="4" class="px-3 py-6 text-center text-muted-foreground">Sin lecturas</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Alarmas activas -->
      <div v-if="monTab === 'alarmas'" class="overflow-x-auto rounded-sm border border-border">
        <table class="w-full text-xs">
          <thead class="bg-muted/50">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Severidad</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Tipo</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Planta</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Equipo</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Señal</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">Inicio</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <tr v-for="row in alarmasActivas" :key="row.Alarma_Id" class="hover:bg-muted/20">
              <td class="px-3 py-1.5">
                <span class="px-1.5 py-0.5 rounded-sm text-[10px]"
                  :class="{
                    'bg-red-700/40 text-red-300': row.Severidad === 'Critica',
                    'bg-red-500/20 text-red-400': row.Severidad === 'Alta',
                    'bg-amber-500/20 text-amber-400': row.Severidad === 'Media',
                    'bg-muted text-muted-foreground': row.Severidad === 'Baja',
                  }">
                  {{ row.Severidad }}
                </span>
              </td>
              <td class="px-3 py-1.5">{{ row.Tipo_Alarma }}</td>
              <td class="px-3 py-1.5">{{ row.Planta_Nombre }}</td>
              <td class="px-3 py-1.5">{{ row.Nombre_Equipo }}</td>
              <td class="px-3 py-1.5 font-mono">{{ row.Nombre_Instancia }}</td>
              <td class="px-3 py-1.5 font-mono text-muted-foreground">{{ row.Ts_Inicio }}</td>
            </tr>
            <tr v-if="!alarmasActivas.length">
              <td colspan="6" class="px-3 py-6 text-center text-muted-foreground">Sin alarmas activas</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Raw rows -->
      <div v-if="monTab === 'raw'" class="overflow-x-auto rounded-sm border border-border">
        <table class="w-full text-xs">
          <thead class="bg-muted/50">
            <tr>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">ts</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">signal_path</th>
              <th class="px-3 py-2 text-right font-medium text-muted-foreground">value</th>
              <th class="px-3 py-2 text-left font-medium text-muted-foreground">quality</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <tr v-for="(row, i) in rawRows" :key="i" class="hover:bg-muted/20">
              <td class="px-3 py-1.5 font-mono text-muted-foreground">{{ row.ts }}</td>
              <td class="px-3 py-1.5 font-mono text-xs max-w-xs truncate">{{ row.signal_path }}</td>
              <td class="px-3 py-1.5 font-mono text-right">{{ row.value }}</td>
              <td class="px-3 py-1.5">{{ row.quality }}</td>
            </tr>
            <tr v-if="!rawRows.length">
              <td colspan="4" class="px-3 py-6 text-center text-muted-foreground">Sin datos raw</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
