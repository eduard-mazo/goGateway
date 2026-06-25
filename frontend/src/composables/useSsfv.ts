// useSsfv — singleton composable holding all SSFV view state, catalogs, and
// actions (plantas/equipos/fronteras/señales/tipos/monitoreo/autodiscovery/
// bulk-approve). Extracted from SSFVView.vue so the view and its tab components
// share one instance. State is created once (memoized); lifecycle hooks register
// against the first caller (the SSFV view shell).
import { ref, computed, onMounted, onUnmounted, watch, reactive } from 'vue'
import { toast } from 'vue-sonner'
import {
  api,
  type SSFVStatus, type SSFVPlanta, type SSFVEquipo, type SSFVSenal,
  type SSFVFrontera,
  type SSFVTipoEquipo, type SSFVTipoVariable, type SSFVUnidad,
  type SSFVSenalXTipo, type IEC104Server,
} from '@/api'
import { useConfirm } from '@/composables/useConfirm'
import { Sun, Cpu, Link2, Activity, GitBranch, Gauge, Cog, Fingerprint, Radio } from 'lucide-vue-next'

function create() {
const { confirm } = useConfirm()

// ─── Tab state ────────────────────────────────────────────────────────────────
const tab = ref<'plantas' | 'catalogo' | 'tipos' | 'estado' | 'pendientes'>('plantas')

// ─── SSFV Connection status ───────────────────────────────────────────────────
const status    = ref<SSFVStatus | null>(null)
const missed    = ref<any[]>([])
const refreshing = ref(false)

async function fetchStatus() {
  try {
    const r = await api.get('/ssfv/status')
    status.value = r.data
  } catch { status.value = null }
}

async function fetchMissed() {
  try {
    const r = await api.get('/ssfv/missed')
    missed.value = r.data ?? []
  } catch { missed.value = [] }
}

async function invalidateCache() {
  try {
    await api.post('/ssfv/cache/invalidate')
    toast.success('Cache de señales invalidado')
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error invalidando cache')
  }
}

// ─── Support catalogs (shared dropdowns) ─────────────────────────────────────
const tipoEquipos  = ref<SSFVTipoEquipo[]>([])
const tipoVars     = ref<SSFVTipoVariable[]>([])
const unidades     = ref<SSFVUnidad[]>([])

async function fetchCatalogs() {
  const [te, tv, u] = await Promise.all([
    api.get('/ssfv/tipo-equipo'),
    api.get('/ssfv/tipo-variable'),
    api.get('/ssfv/unidades'),
  ])
  tipoEquipos.value = te.data ?? []
  tipoVars.value    = tv.data ?? []
  unidades.value    = u.data ?? []
}

// ─── Plantas ──────────────────────────────────────────────────────────────────
const plantas       = ref<SSFVPlanta[]>([])
const plantaOpen    = ref<number | null>(null) // expanded planta_id

async function fetchPlantas() {
  const r = await api.get('/ssfv/plantas')
  plantas.value = r.data ?? []
}

// Con miles de plantas la lista necesita búsqueda + filtro de estado + render
// incremental (lotes de PLANTA_PAGE) para no inflar el DOM.
const PLANTA_PAGE = 25
const plantaSearch = ref('')
const plantaEstadoFilter = ref<'all' | '1' | '0' | '2'>('all')
const plantaLimit = ref(PLANTA_PAGE)
watch([plantaSearch, plantaEstadoFilter], () => { plantaLimit.value = PLANTA_PAGE })

const filteredPlantas = computed(() => {
  const q = plantaSearch.value.trim().toLowerCase()
  let list = plantas.value
  if (plantaEstadoFilter.value !== 'all') list = list.filter(p => String(p.estado) === plantaEstadoFilter.value)
  if (q) {
    list = list.filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      p.broker_base.toLowerCase().includes(q) ||
      (p.ubicacion ?? '').toLowerCase().includes(q) ||
      (p.propietario ?? '').toLowerCase().includes(q))
  }
  return list
})
const visiblePlantas = computed(() => filteredPlantas.value.slice(0, plantaLimit.value))

// ─── Vista de flota: frescura de datos y totales ─────────────────────────────
// nowTick refresca cada 10 s para que «hace N s» y los puntos de frescura se
// muevan sin re-consultar la API.
const nowTick = ref(Date.now())
let nowTimer: ReturnType<typeof setInterval> | undefined
onMounted(() => { nowTimer = setInterval(() => { nowTick.value = Date.now() }, 10_000) })
onUnmounted(() => { if (nowTimer) clearInterval(nowTimer) })

function relTime(ts?: string | null): string {
  if (!ts) return 'sin datos'
  const s = Math.max(0, Math.round((nowTick.value - new Date(ts).getTime()) / 1000))
  if (s < 60) return `hace ${s} s`
  if (s < 3600) return `hace ${Math.floor(s / 60)} min`
  if (s < 86400) return `hace ${Math.floor(s / 3600)} h`
  return `hace ${Math.floor(s / 86400)} d`
}

// En línea <2 min (un par de ciclos de publicación); rezagada <15 min; luego fría.
type Freshness = 'live' | 'lag' | 'cold' | 'none'
function freshness(p: SSFVPlanta): Freshness {
  if (!p.ultima_lectura) return 'none'
  const s = (nowTick.value - new Date(p.ultima_lectura).getTime()) / 1000
  if (s < 120) return 'live'
  if (s < 900) return 'lag'
  return 'cold'
}
const FRESH_META: Record<Freshness, { dot: string; text: string; label: string }> = {
  live: { dot: 'bg-emerald-500 animate-pulse', text: 'text-emerald-500', label: 'en línea' },
  lag:  { dot: 'bg-amber-500',                 text: 'text-amber-400',  label: 'rezagada' },
  cold: { dot: 'bg-zinc-500',                  text: 'text-muted-foreground', label: 'sin flujo' },
  none: { dot: 'bg-transparent border border-dashed border-muted-foreground/60', text: 'text-muted-foreground', label: 'sin datos' },
}

const fleetSummary = computed(() => {
  const ps = plantas.value
  return {
    total:   ps.length,
    activas: ps.filter(p => p.estado === 1).length,
    kwp:     ps.reduce((a, p) => a + (p.capacidad_kWp ?? 0), 0),
    equipos: ps.reduce((a, p) => a + (p.n_equipos ?? 0), 0),
    senales: ps.reduce((a, p) => a + (p.n_senales ?? 0), 0),
    alarmas: ps.reduce((a, p) => a + (p.n_alarmas ?? 0), 0),
    live:    ps.filter(p => freshness(p) === 'live').length,
  }
})

function togglePlanta(id: number) {
  plantaOpen.value = plantaOpen.value === id ? null : id
  if (plantaOpen.value === id) {
    loadEquiposByPlanta(id)
    loadFronterasByPlanta(id)
  }
}

// IEC-104 server list, passed to the per-plant <PlantaIec104Link> control which
// owns the link/mirror lifecycle itself.
const iec104Servers = ref<IEC104Server[]>([])
async function fetchIec104Servers() {
  try {
    const r = await api.get<IEC104Server[]>('/iec104-servers')
    iec104Servers.value = r.data ?? []
  } catch { /* servers tab handles its own errors */ }
}

// Planta dialog
const plantaDialog  = ref(false)
const plantaEdit    = ref<SSFVPlanta | null>(null)
const plantaForm    = reactive({ nombre: '', broker_base: '', ubicacion: '', propietario: '', capacidad_kWp: undefined as number | undefined, estado: 1 })

function openCreatePlanta() {
  plantaEdit.value = null
  Object.assign(plantaForm, { nombre: '', broker_base: '', ubicacion: '', propietario: '', capacidad_kWp: undefined, estado: 1 })
  plantaDialog.value = true
}
function openEditPlanta(p: SSFVPlanta) {
  plantaEdit.value = p
  Object.assign(plantaForm, { nombre: p.nombre, broker_base: p.broker_base, ubicacion: p.ubicacion ?? '', propietario: p.propietario ?? '', capacidad_kWp: p.capacidad_kWp, estado: p.estado })
  plantaDialog.value = true
}
async function savePlanta() {
  try {
    if (plantaEdit.value?.planta_id) {
      await api.put(`/ssfv/plantas/${plantaEdit.value.planta_id}`, plantaForm)
      toast.success('Planta actualizada')
    } else {
      await api.post('/ssfv/plantas', plantaForm)
      toast.success('Planta creada')
    }
    plantaDialog.value = false
    await fetchPlantas()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error guardando planta') }
}
async function deletePlanta(p: SSFVPlanta) {
  const ok = await confirm({
    title: 'Eliminar planta',
    message: 'La planta y sus equipos se marcarán como dados de baja y dejarán de ingerir datos. El histórico de valores y alarmas se conserva.',
    detail: p.nombre,
    variant: 'danger',
    confirmText: 'Eliminar planta',
  })
  if (!ok) return
  try {
    await api.delete(`/ssfv/plantas/${p.planta_id}`)
    toast.success('Planta dada de baja')
    if (plantaOpen.value === p.planta_id) plantaOpen.value = null
    await fetchPlantas()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// Contract §1.1: broker_base = the Sparkplug group (one segment). Invalid if it
// contains a '/' or whitespace.
const brokerBaseInvalid = computed(() => {
  const b = plantaForm.broker_base.trim()
  return b !== '' && /[/\s]/.test(b)
})

// ─── Equipos ──────────────────────────────────────────────────────────────────
const plantaEquipos  = ref<Record<number, SSFVEquipo[]>>({})
const equipoSignals  = ref<Record<number, any[]>>({})
const equipoOpen     = ref<number | null>(null)

async function loadEquiposByPlanta(pid: number) {
  const r = await api.get(`/ssfv/plantas/${pid}/equipos`)
  plantaEquipos.value = { ...plantaEquipos.value, [pid]: r.data ?? [] }
}

function toggleEquipo(eid: number) {
  equipoOpen.value = equipoOpen.value === eid ? null : eid
  if (equipoOpen.value === eid) loadEquipoSignals(eid)
}

async function loadEquipoSignals(eid: number) {
  const r = await api.get(`/ssfv/equipos/${eid}/senales`)
  equipoSignals.value = { ...equipoSignals.value, [eid]: r.data ?? [] }
}

// Delete a single instanced signal (binding). The backend hard-deletes a binding
// with no history (204) or soft-deactivates one referenced by tbl_valores (200).
async function deleteAsignacion(s: any) {
  const inst = s.nombre_instancia && s.nombre_instancia !== 'default' ? ' — ' + s.nombre_instancia : ''
  const ok = await confirm({
    title: 'Eliminar señal instanciada',
    message: 'Se elimina esta instancia de la señal en el equipo. Si tiene histórico, se da de baja (deja de ingerir; el histórico se conserva).',
    detail: `${s.senal_nombre || s.codigo_senal}${inst}`,
    variant: 'danger',
    confirmText: 'Eliminar',
  })
  if (!ok) return
  try {
    const r = await api.delete(`/ssfv/asignaciones/${s.equisenal_id}`)
    toast.success(r.status === 200 && r.data?.deactivated
      ? 'Señal dada de baja — histórico conservado'
      : 'Señal instanciada eliminada')
    if (s.equipo_id) await loadEquipoSignals(s.equipo_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error eliminando señal') }
}

// Contract §1.1: node = the first topic segment after the group (broker_base).
function nodeOf(planta: SSFVPlanta, eq: SSFVEquipo): string {
  const base = planta.broker_base
  const rel = eq.nombre_topic.startsWith(base + '/') ? eq.nombre_topic.slice(base.length + 1) : eq.nombre_topic
  return rel.split('/')[0] || '(nodo)'
}
// Whether an equipo is the node-level entity (group/node) vs a device (group/node/device).
function isNodeEntity(planta: SSFVPlanta, eq: SSFVEquipo): boolean {
  const base = planta.broker_base
  const rel = eq.nombre_topic.startsWith(base + '/') ? eq.nombre_topic.slice(base.length + 1) : eq.nombre_topic
  return !rel.includes('/')
}
// Contract §1.1 hierarchy: Planta (group) → Nodo (group/node, carries System +
// node-level señales) → Devices (group/node/device). The node-level equipo is
// the PARENT in the tree; its devices nest beneath it. Devices discovered
// before their node was approved hang under a ghost node header.
type NodeBranch = { node: string; nodeEntity: SSFVEquipo | null; devices: SSFVEquipo[] }
function nodeTree(planta: SSFVPlanta): NodeBranch[] {
  const branches = new Map<string, NodeBranch>()
  for (const eq of plantaEquipos.value[planta.planta_id!] ?? []) {
    const n = nodeOf(planta, eq)
    let b = branches.get(n)
    if (!b) { b = { node: n, nodeEntity: null, devices: [] }; branches.set(n, b) }
    if (isNodeEntity(planta, eq)) b.nodeEntity = eq
    else b.devices.push(eq)
  }
  for (const b of branches.values()) {
    b.devices.sort((x, y) => x.nombre_topic.localeCompare(y.nombre_topic))
  }
  return [...branches.values()].sort((a, b) => a.node.localeCompare(b.node))
}
// Last topic segment — the device's own id, shown as the child's primary label.
function lastSegment(topic: string): string {
  return topic.split('/').pop() || topic
}

// Equipo dialog
const equipoDialog = ref(false)
const equipoEdit   = ref<SSFVEquipo | null>(null)
const equipoPlantaCtx = ref(0)
const equipoForm   = reactive({ planta_id: 0, tipo_id: 0, nombre_equipo: '', nombre_topic: '', nombre_topic_suffix: '', fabricante: '', modelo: '', nro_serie: '', estado: 1 })

watch(() => equipoForm.nombre_topic_suffix, (suffix) => {
  const pid = equipoForm.planta_id
  if (!pid || !suffix || equipoEdit.value) return
  const planta = plantas.value.find(p => p.planta_id === pid)
  if (planta) equipoForm.nombre_topic = planta.broker_base + '/' + suffix.replace(/^\/+/, '')
})

// The group (broker_base) of the equipo's planta, and the prefix invariant check.
const equipoPlantaBase = computed(() =>
  plantas.value.find(p => p.planta_id === equipoForm.planta_id)?.broker_base ?? '')
const equipoTopicInvalid = computed(() => {
  const base = equipoPlantaBase.value, t = equipoForm.nombre_topic.trim()
  if (!base || !t) return false
  return t !== base && !t.startsWith(base + '/')
})

function openCreateEquipo(plantaId: number) {
  equipoEdit.value = null
  equipoPlantaCtx.value = plantaId
  Object.assign(equipoForm, { planta_id: plantaId, tipo_id: 0, nombre_equipo: '', nombre_topic: '', nombre_topic_suffix: '', fabricante: '', modelo: '', nro_serie: '', estado: 1 })
  equipoDialog.value = true
}
function openEditEquipo(eq: SSFVEquipo) {
  equipoEdit.value = eq
  equipoPlantaCtx.value = eq.planta_id
  const suffix = eq.nombre_topic.split('/').pop() ?? ''
  Object.assign(equipoForm, { planta_id: eq.planta_id, tipo_id: eq.tipo_id, nombre_equipo: eq.nombre_equipo, nombre_topic: eq.nombre_topic, nombre_topic_suffix: suffix, fabricante: eq.fabricante ?? '', modelo: eq.modelo ?? '', nro_serie: eq.nro_serie ?? '', estado: eq.estado })
  equipoDialog.value = true
}
async function saveEquipo() {
  try {
    const payload = { planta_id: equipoForm.planta_id, tipo_id: equipoForm.tipo_id, nombre_equipo: equipoForm.nombre_equipo, nombre_topic: equipoForm.nombre_topic, fabricante: equipoForm.fabricante || undefined, modelo: equipoForm.modelo || undefined, nro_serie: equipoForm.nro_serie || undefined, estado: equipoForm.estado }
    if (equipoEdit.value?.equipo_id) {
      await api.put(`/ssfv/equipos/${equipoEdit.value.equipo_id}`, payload)
      toast.success('Equipo actualizado')
    } else {
      await api.post('/ssfv/equipos', payload)
      toast.success('Equipo creado — las señales se vinculan al descubrir el equipo (NBIRTH/DBIRTH)')
    }
    equipoDialog.value = false
    await loadEquiposByPlanta(equipoForm.planta_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error guardando equipo') }
}
async function deleteEquipo(eq: SSFVEquipo) {
  const ok = await confirm({
    title: 'Eliminar equipo',
    message: 'El equipo se marcará como dado de baja y dejará de ingerir datos. El histórico de valores y alarmas se conserva.',
    detail: eq.nombre_equipo,
    variant: 'danger',
    confirmText: 'Eliminar equipo',
  })
  if (!ok) return
  try {
    await api.delete(`/ssfv/equipos/${eq.equipo_id}`)
    toast.success('Equipo dado de baja')
    if (equipoOpen.value === eq.equipo_id) equipoOpen.value = null
    await loadEquiposByPlanta(eq.planta_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// ─── Fronteras ────────────────────────────────────────────────────────────────
const plantaFronteras = ref<Record<number, SSFVFrontera[]>>({})

async function loadFronterasByPlanta(pid: number) {
  const r = await api.get('/ssfv/fronteras', { params: { planta_id: pid } })
  plantaFronteras.value = { ...plantaFronteras.value, [pid]: (r.data ?? []).filter((f: SSFVFrontera) => f.planta_id === pid) }
}

const fronteraDialog = ref(false)
const fronteraEdit   = ref<SSFVFrontera | null>(null)
const fronteraForm   = reactive({ planta_id: 0, codigo_nie: '', nombre: '', tipo_conexion: '', activo: true })

function openCreateFrontera(plantaId: number) {
  fronteraEdit.value = null
  Object.assign(fronteraForm, { planta_id: plantaId, codigo_nie: '', nombre: '', tipo_conexion: '', activo: true })
  fronteraDialog.value = true
}
function openEditFrontera(f: SSFVFrontera) {
  fronteraEdit.value = f
  Object.assign(fronteraForm, { planta_id: f.planta_id, codigo_nie: f.codigo_nie, nombre: f.nombre, tipo_conexion: f.tipo_conexion ?? '', activo: f.activo })
  fronteraDialog.value = true
}
async function saveFrontera() {
  try {
    const payload = { ...fronteraForm, tipo_conexion: fronteraForm.tipo_conexion || undefined }
    if (fronteraEdit.value?.frontera_id) {
      await api.put(`/ssfv/fronteras/${fronteraEdit.value.frontera_id}`, payload)
      toast.success('Frontera actualizada')
    } else {
      await api.post('/ssfv/fronteras', payload)
      toast.success('Frontera creada')
    }
    fronteraDialog.value = false
    await loadFronterasByPlanta(fronteraForm.planta_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}
async function deleteFrontera(f: SSFVFrontera) {
  const ok = await confirm({ title: 'Eliminar frontera', message: f.nombre, variant: 'danger', confirmText: 'Eliminar' })
  if (!ok) return
  try {
    await api.delete(`/ssfv/fronteras/${f.frontera_id}`)
    toast.success('Frontera eliminada')
    await loadFronterasByPlanta(f.planta_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// ─── Señales (catálogo) ───────────────────────────────────────────────────────
const senales       = ref<SSFVSenal[]>([])
const senalSearch   = ref('')
const senalOpen     = ref<number | null>(null)
const senalXTipo    = ref<Record<number, SSFVSenalXTipo[]>>({}) // per senal_id

const filteredSenales = computed(() => {
  const q = senalSearch.value.toLowerCase()
  if (!q) return senales.value
  return senales.value.filter(s =>
    s.nombre.toLowerCase().includes(q) ||
    s.codigo_senal.toLowerCase().includes(q) ||
    (s.tipo_var_nombre ?? '').toLowerCase().includes(q),
  )
})

// Render incremental del catálogo (lotes de SENAL_PAGE) — misma razón que
// visiblePlantas: el filtro es sobre todo el set, el DOM solo pinta una página.
const SENAL_PAGE = 50
const senalLimit = ref(SENAL_PAGE)
watch(senalSearch, () => { senalLimit.value = SENAL_PAGE })
const visibleSenales = computed(() => filteredSenales.value.slice(0, senalLimit.value))

async function fetchSenales() {
  const r = await api.get('/ssfv/senales')
  senales.value = r.data ?? []
}

function toggleSenal(id: number) {
  senalOpen.value = senalOpen.value === id ? null : id
  if (senalOpen.value === id) loadSenalXTipo(id)
}

async function loadSenalXTipo(senalId: number) {
  const r = await api.get('/ssfv/senales-x-tipo', { params: { tipo_id: undefined } })
  senalXTipo.value = { ...senalXTipo.value, [senalId]: (r.data ?? []).filter((s: SSFVSenalXTipo) => s.senal_id === senalId) }
}

// Señal dialog
const senalDialog = ref(false)
const senalEdit   = ref<SSFVSenal | null>(null)
const senalForm   = reactive({ tipavar_id: 0, unidad_id: 0, nombre: '', descripcion: '', tipo_valor: 'Instantaneo' as 'Instantaneo' | 'Acumulado', codigo_senal: '', es_indexada: false, activo: true })

function openCreateSenal() {
  senalEdit.value = null
  Object.assign(senalForm, { tipavar_id: 0, unidad_id: 0, nombre: '', descripcion: '', tipo_valor: 'Instantaneo', codigo_senal: '', es_indexada: false, activo: true })
  senalDialog.value = true
}
function openEditSenal(s: SSFVSenal) {
  senalEdit.value = s
  Object.assign(senalForm, { tipavar_id: s.tipavar_id, unidad_id: s.unidad_id, nombre: s.nombre, descripcion: s.descripcion ?? '', tipo_valor: s.tipo_valor, codigo_senal: s.codigo_senal, es_indexada: s.es_indexada, activo: s.activo })
  senalDialog.value = true
}
async function saveSenal() {
  try {
    const payload = { ...senalForm, descripcion: senalForm.descripcion || undefined }
    if (senalEdit.value?.senal_id) {
      await api.put(`/ssfv/senales/${senalEdit.value.senal_id}`, payload)
      toast.success('Señal actualizada')
    } else {
      await api.post('/ssfv/senales', payload)
      toast.success('Señal creada')
    }
    senalDialog.value = false
    await fetchSenales()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}
async function deleteSenal(s: SSFVSenal) {
  const ok = await confirm({
    title: 'Eliminar señal',
    message: 'Una señal sin vínculos se elimina del catálogo. Si está vinculada a equipos, la señal y sus instancias se darán de baja: dejan de ingerir datos y el histórico se conserva.',
    detail: `${s.codigo_senal} — ${s.nombre}`,
    variant: 'danger',
    confirmText: 'Eliminar señal',
  })
  if (!ok) return
  try {
    const r = await api.delete(`/ssfv/senales/${s.senal_id}`)
    if (r.status === 200 && r.data?.deactivated) {
      toast.success(`Señal dada de baja — ${r.data.bindings} instancia(s) conservan su histórico`)
    } else {
      toast.success('Señal eliminada')
    }
    await fetchSenales()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// Señal × Tipo dialog (add signal to equipment type template)
const senalXTipoDialog = ref(false)
const senalXTipoCtxSenalId = ref(0)
const senalXTipoForm = reactive({ tipo_id: 0, num_canales: 1 })

function openAddSenalXTipo(senalId: number) {
  senalXTipoCtxSenalId.value = senalId
  Object.assign(senalXTipoForm, { tipo_id: 0, num_canales: 1 })
  senalXTipoDialog.value = true
}
async function saveSenalXTipo() {
  try {
    await api.post('/ssfv/senales-x-tipo', { senal_id: senalXTipoCtxSenalId.value, ...senalXTipoForm })
    toast.success('Señal asignada al tipo de equipo')
    senalXTipoDialog.value = false
    await loadSenalXTipo(senalXTipoCtxSenalId.value)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}
async function deleteSenalXTipo(sxt: SSFVSenalXTipo) {
  const ok = await confirm({ title: 'Quitar asignación', message: `${sxt.codigo_senal} ↔ ${sxt.tipo_nombre}`, variant: 'danger', confirmText: 'Quitar' })
  if (!ok) return
  try {
    await api.delete(`/ssfv/senales-x-tipo/${sxt.senaltipo_id}`)
    toast.success('Asignación eliminada')
    await loadSenalXTipo(sxt.senal_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// ─── Tipo Equipo ──────────────────────────────────────────────────────────────
const tipoEquipoDialog = ref(false)
const tipoEquipoEdit   = ref<SSFVTipoEquipo | null>(null)
const tipoEquipoForm   = reactive({ nombre: '', descripcion: '', activo: true })

function openCreateTipoEquipo() {
  tipoEquipoEdit.value = null
  Object.assign(tipoEquipoForm, { nombre: '', descripcion: '', activo: true })
  tipoEquipoDialog.value = true
}
function openEditTipoEquipo(te: SSFVTipoEquipo) {
  tipoEquipoEdit.value = te
  Object.assign(tipoEquipoForm, { nombre: te.nombre, descripcion: te.descripcion ?? '', activo: te.activo })
  tipoEquipoDialog.value = true
}
async function saveTipoEquipo() {
  try {
    const payload = { nombre: tipoEquipoForm.nombre, descripcion: tipoEquipoForm.descripcion || undefined, activo: tipoEquipoForm.activo }
    if (tipoEquipoEdit.value?.tipo_id) {
      await api.put(`/ssfv/tipo-equipo/${tipoEquipoEdit.value.tipo_id}`, payload)
    } else {
      await api.post('/ssfv/tipo-equipo', payload)
    }
    toast.success('Tipo de equipo guardado')
    tipoEquipoDialog.value = false
    await fetchCatalogs()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}
async function deleteTipoEquipo(te: SSFVTipoEquipo) {
  const ok = await confirm({ title: 'Eliminar tipo de equipo', message: te.nombre, variant: 'danger', confirmText: 'Eliminar' })
  if (!ok) return
  try {
    await api.delete(`/ssfv/tipo-equipo/${te.tipo_id}`)
    toast.success('Tipo de equipo eliminado')
    await fetchCatalogs()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// ─── Tipo Variable ────────────────────────────────────────────────────────────
const tipoVarDialog = ref(false)
const tipoVarEdit   = ref<SSFVTipoVariable | null>(null)
const tipoVarForm   = reactive({ nombre: '', descripcion: '', activo: true })

function openCreateTipoVar() {
  tipoVarEdit.value = null
  Object.assign(tipoVarForm, { nombre: '', descripcion: '', activo: true })
  tipoVarDialog.value = true
}
function openEditTipoVar(tv: SSFVTipoVariable) {
  tipoVarEdit.value = tv
  Object.assign(tipoVarForm, { nombre: tv.nombre, descripcion: tv.descripcion ?? '', activo: tv.activo })
  tipoVarDialog.value = true
}
async function saveTipoVar() {
  try {
    const payload = { nombre: tipoVarForm.nombre, descripcion: tipoVarForm.descripcion || undefined, activo: tipoVarForm.activo }
    if (tipoVarEdit.value?.tipovar_id) {
      await api.put(`/ssfv/tipo-variable/${tipoVarEdit.value.tipovar_id}`, payload)
    } else {
      await api.post('/ssfv/tipo-variable', payload)
    }
    toast.success('Tipo de variable guardado')
    tipoVarDialog.value = false
    await fetchCatalogs()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}
async function deleteTipoVar(tv: SSFVTipoVariable) {
  const ok = await confirm({ title: 'Eliminar tipo de variable', message: tv.nombre, variant: 'danger', confirmText: 'Eliminar' })
  if (!ok) return
  try {
    await api.delete(`/ssfv/tipo-variable/${tv.tipovar_id}`)
    toast.success('Tipo de variable eliminado')
    await fetchCatalogs()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// ─── Unidades ─────────────────────────────────────────────────────────────────
const unidadDialog = ref(false)
const unidadEdit   = ref<SSFVUnidad | null>(null)
const unidadForm   = reactive({ simbolo: '', nombre: '', magnitud: '', activo: true })

function openCreateUnidad() {
  unidadEdit.value = null
  Object.assign(unidadForm, { simbolo: '', nombre: '', magnitud: '', activo: true })
  unidadDialog.value = true
}
function openEditUnidad(u: SSFVUnidad) {
  unidadEdit.value = u
  Object.assign(unidadForm, { simbolo: u.simbolo, nombre: u.nombre, magnitud: u.magnitud, activo: u.activo })
  unidadDialog.value = true
}
async function saveUnidad() {
  try {
    const payload = { ...unidadForm }
    if (unidadEdit.value?.unidad_id) {
      await api.put(`/ssfv/unidades/${unidadEdit.value.unidad_id}`, payload)
    } else {
      await api.post('/ssfv/unidades', payload)
    }
    toast.success('Unidad guardada')
    unidadDialog.value = false
    await fetchCatalogs()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}
async function deleteUnidad(u: SSFVUnidad) {
  const ok = await confirm({ title: 'Eliminar unidad', message: `${u.simbolo} — ${u.nombre}`, variant: 'danger', confirmText: 'Eliminar' })
  if (!ok) return
  try {
    await api.delete(`/ssfv/unidades/${u.unidad_id}`)
    toast.success('Unidad eliminada')
    await fetchCatalogs()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// ─── Monitoreo de señales ─────────────────────────────────────────────────────
const ultimasLecturas = ref<any[]>([])
const alarmasActivas  = ref<any[]>([])
const monTab = ref<'missed' | 'lecturas' | 'alarmas'>('missed')

async function fetchMonitoreo() {
  refreshing.value = true
  try {
    const [ul, aa, mi] = await Promise.all([
      api.get('/ssfv/vista/ultimas-lecturas'),
      api.get('/ssfv/vista/alarmas-activas'),
      api.get('/ssfv/missed'),
    ])
    ultimasLecturas.value = ul.data ?? []
    alarmasActivas.value  = aa.data ?? []
    missed.value          = mi.data ?? []
  } catch (e: any) {
    toast.error('Error cargando datos: ' + (e.response?.data?.error ?? e.message))
  } finally { refreshing.value = false }
}

// ─── Auto-Discovery ───────────────────────────────────────────────────────────
interface MetricMetaItem {
  name: string
  engUnit?: string
  tipo_variable?: string
  tipo_valor?: string
  description?: string
  uns_name?: string       // producer-declared display name (→ tbl_senales.nombre)
  uns_code?: string       // producer-declared FIWARE attribute (→ codigo_senal)
  uns_instance?: string   // producer-declared entity instance/channel (→ nombre_instancia)
  device_topic?: string
  device_type?: string
  value?: string          // birth string value (device-identity metrics)
}

interface AutodiscEntity {
  id: number
  group_id: string
  node_id: string
  device_id: string
  metric_names: string[]
  metric_meta: MetricMetaItem[]
  node_properties: Record<string, string>
  status: 'pending' | 'approved' | 'rejected'
  first_seen: string
  last_seen: string
}

const autoEntities       = ref<AutodiscEntity[]>([])
const autoPendingCount   = computed(() => autoEntities.value.filter(e => e.status === 'pending').length)
const approveDialog      = ref(false)
const approveTarget      = ref<AutodiscEntity | null>(null)
// planta_id '__new__' = stage the parent Planta with the approval (contract v3
// §7): nombre = UI alias (pre-filled from the NBIRTH uns/planta property),
// broker_base = the entity's Sparkplug group_id.
const approveForm        = reactive({ planta_id: '', tipo_id: '', nombre_equipo: '', nombre_topic: '', planta_nombre: '' })
const PLANTA_NEW = '__new__'

// Derived: unique device types detected in the approve target's metric_meta
const approveDetectedTypes = computed(() => {
  const meta = approveTarget.value?.metric_meta ?? []
  const types = [...new Set(meta.map(m => m.device_type).filter(Boolean))]
  return types as string[]
})

// ─── Metric review model ──────────────────────────────────────────────────────
// Classifies each NBIRTH/DBIRTH metric by the Sparkplug B naming convention
// (sparkplug-contract.md §4) and merges in the typed PropertySet metadata
// (§5) so the operator can audit every reported signal before approval.
type MetricKind = 'proceso' | 'sistema' | 'identidad' | 'sesion'

const KIND_META: Record<MetricKind, { label: string; icon: any; dot: string; chip: string }> = {
  proceso:   { label: 'Proceso',   icon: Gauge,       dot: 'bg-emerald-500', chip: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30' },
  sistema:   { label: 'Sistema',   icon: Cog,         dot: 'bg-sky-500',     chip: 'bg-sky-500/15 text-sky-400 border-sky-500/30' },
  identidad: { label: 'Identidad', icon: Fingerprint, dot: 'bg-violet-500',  chip: 'bg-violet-500/15 text-violet-400 border-violet-500/30' },
  sesion:    { label: 'Sesión',    icon: Radio,       dot: 'bg-zinc-500',    chip: 'bg-zinc-500/15 text-zinc-400 border-zinc-500/30' },
}

// Host telemetry categories (matches worker.hostCategories). Metrics in these
// namespaces describe the gateway host; like any signal they persist to
// ssfv.tbl_valores once registered on the node's host station. The producer's
// "System/" prefix is optional.
const HOST_CATEGORIES = new Set(['CPU', 'Memory', 'Disk', 'Network', 'Temperature', 'Power', 'Process'])

function metricKind(name: string): MetricKind {
  if (name === 'bdSeq' || name === 'seq') return 'sesion'
  // The producer's host-telemetry prefix is matched case-insensitively
  // ("System/" default, "SYSTEM/" in the EPM deployment) — contract v3 §4.
  const n = /^system\//i.test(name) ? name.slice(name.indexOf('/') + 1) : name
  if (n.startsWith('Device/')) return 'identidad'
  if (n === 'Uptime_h' || HOST_CATEGORIES.has(n.split('/')[0])) return 'sistema'
  return 'proceso'
}

interface ReviewMetric {
  name: string
  group: string
  leaf: string
  engUnit?: string
  tipoVariable?: string
  tipoValor?: string
  description?: string
  deviceType?: string
  unsName?: string        // producer-declared display name (→ nombre)
  unsCode?: string        // producer-declared attribute (→ codigo_senal)
  unsInstance?: string    // producer-declared instance/channel (→ nombre_instancia)
  value?: string          // birth string value (device-identity metrics)
  kind: MetricKind
}

// buildReviewMetrics merges metric_names with the per-metric PropertySet meta,
// keyed by name. Names without meta still appear (NBIRTH always lists names).
function buildReviewMetrics(e: AutodiscEntity): ReviewMetric[] {
  const metaByName = new Map(e.metric_meta.map(m => [m.name, m]))
  return e.metric_names.map(name => {
    const m = metaByName.get(name)
    const seg = name.split('/')
    return {
      name,
      group: seg.length > 1 ? seg[0] : 'Nodo',
      leaf:  seg.length > 1 ? seg.slice(1).join('/') : name,
      engUnit:      m?.engUnit || undefined,
      tipoVariable: m?.tipo_variable || undefined,
      tipoValor:    m?.tipo_valor || undefined,
      description:  m?.description || undefined,
      deviceType:   m?.device_type || undefined,
      unsName:      m?.uns_name || undefined,
      unsCode:      m?.uns_code || undefined,
      unsInstance:  m?.uns_instance || undefined,
      value:        m?.value || undefined,
      kind: metricKind(name),
    }
  })
}

// groupedPreview groups an entity's metrics by source namespace for the
// expandable inline preview in the Pendientes table.
function groupedPreview(e: AutodiscEntity): { name: string; metrics: ReviewMetric[] }[] {
  const groups = new Map<string, ReviewMetric[]>()
  for (const m of buildReviewMetrics(e)) {
    if (!groups.has(m.group)) groups.set(m.group, [])
    groups.get(m.group)!.push(m)
  }
  return [...groups.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([name, metrics]) => ({ name, metrics }))
}

// Per-entity kind tally for the table summary chips.
function entityKindCounts(e: AutodiscEntity): { kind: MetricKind; n: number }[] {
  const counts = {} as Record<MetricKind, number>
  for (const n of e.metric_names) {
    const k = metricKind(n)
    counts[k] = (counts[k] ?? 0) + 1
  }
  return (Object.keys(counts) as MetricKind[])
    .sort((a, b) => counts[b] - counts[a])
    .map(kind => ({ kind, n: counts[kind] }))
}

// Inline row expansion in the Pendientes table.
const expandedAuto = ref<Set<number>>(new Set())
function toggleAutoRow(id: number) {
  const s = new Set(expandedAuto.value)
  s.has(id) ? s.delete(id) : s.add(id)
  expandedAuto.value = s
}

// ─── Metric review modal (opens on Aprobar, or read-only via Inspeccionar) ────
const metricSearch  = ref('')
const approveReadOnly = ref(false)

const reviewMetrics = computed<ReviewMetric[]>(() =>
  approveTarget.value ? buildReviewMetrics(approveTarget.value) : [])

const reviewFiltered = computed<ReviewMetric[]>(() => {
  const q = metricSearch.value.trim().toLowerCase()
  if (!q) return reviewMetrics.value
  return reviewMetrics.value.filter(m =>
    m.name.toLowerCase().includes(q) ||
    (m.description?.toLowerCase().includes(q)) ||
    (m.engUnit?.toLowerCase().includes(q)) ||
    (m.tipoVariable?.toLowerCase().includes(q)))
})

// Grouped by source namespace (first path segment) for the scrollable list.
const reviewGroups = computed(() => {
  const groups = new Map<string, ReviewMetric[]>()
  for (const m of reviewFiltered.value) {
    if (!groups.has(m.group)) groups.set(m.group, [])
    groups.get(m.group)!.push(m)
  }
  return [...groups.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([name, metrics]) => ({ name, metrics }))
})

// Summary stats shown above the metric list.
const reviewSummary = computed(() => {
  const ms = reviewMetrics.value
  const units = new Set(ms.map(m => m.engUnit).filter(Boolean))
  const vars  = new Set(ms.map(m => m.tipoVariable).filter(Boolean))
  const devs  = new Set(ms.map(m => m.deviceType).filter(Boolean))
  const kinds = entityKindCounts(approveTarget.value ?? { metric_names: [] } as any)
  return { total: ms.length, units: [...units] as string[], vars: [...vars] as string[], devs: [...devs] as string[], kinds }
})

// ─── Signal provisioning plan ─────────────────────────────────────────────────
// Reconciles each reported NBIRTH metric against the SSFV catalog
// (ssfv.tbl_senales) and the chosen tipo_equipo template
// (public.tbl_senales_x_tipo_equipo) so the operator sees exactly which
// signals already map, which exist but aren't in the template, and which are
// brand new — then chooses what to create/link on approval.

// codigo_senal is the last path segment of the metric name (matches the SSFV
// dispatch in sparkplug_dispatch.go: code = parts[len-1]).
function signalCode(name: string): string {
  const seg = name.split('/')
  return seg[seg.length - 1]
}
// nombre_instancia fallback = the folder path between entity and leaf, with the
// cosmetic System/SYSTEM host prefix stripped (mirrors parseFiwareSignal in Go).
function signalInstance(name: string): string {
  const n = /^system\//i.test(name) ? name.slice(name.indexOf('/') + 1) : name
  const seg = n.split('/').filter(Boolean)
  return seg.length > 1 ? seg.slice(0, -1).join('/') : 'default'
}
// Indexed signals: "IDC_1" → base "IDC_x" (mirrors resolveIndexedSignal in Go).
function indexedBase(code: string): string | null {
  const parts = code.split('_')
  const last = parts[parts.length - 1]
  if (parts.length >= 2 && /^\d+$/.test(last)) return parts.slice(0, -1).join('_') + '_x'
  return null
}

const catalogByCode = computed(() => {
  const m = new Map<string, SSFVSenal>()
  for (const s of senales.value) m.set(s.codigo_senal, s)
  return m
})

// True when a metric already has a catalog señal (exact or indexed base).
function metricInCatalog(name: string): boolean {
  const code = signalCode(name)
  const base = indexedBase(code)
  return catalogByCode.value.has(code) || (!!base && catalogByCode.value.has(base))
}

// Count of process metrics on this entity that have NO catalog señal yet.
function entityNewSignalCount(e: AutodiscEntity): number {
  return buildReviewMetrics(e).filter(m => m.kind === 'proceso' && !metricInCatalog(m.name)).length
}

// codigo_senal set covered by the selected tipo_equipo template.
const templateCodes = ref<Set<string>>(new Set())
async function loadTemplateCodes(tipoId: number) {
  if (!tipoId) { templateCodes.value = new Set(); return }
  try {
    const r = await api.get('/ssfv/senales-x-tipo', { params: { tipo_id: tipoId } })
    templateCodes.value = new Set((r.data ?? []).map((s: SSFVSenalXTipo) => s.codigo_senal))
  } catch { templateCodes.value = new Set() }
}

interface PlanEntry {
  name: string
  code: string
  instance: string            // nombre_instancia (FIWARE channel); 'default' = flat
  status: 'catalog' | 'new'   // 'mapped' entries are omitted (no action needed)
  selected: boolean
  senalId?: number            // catalog → existing señal to link into template
  nombre: string
  descripcion: string         // → tbl_senales.descripcion (from uns/description)
  tipovarId: string           // new → catalog FKs (string for shadcn Select)
  unidadId: string
  tipoValor: string
  codeTooLong: boolean        // codigo_senal > 20 chars → cannot create
}

const signalPlan = reactive<Record<string, PlanEntry>>({})

function matchTipoVar(name?: string): string {
  if (!name) return ''
  const hit = tipoVars.value.find(t => t.nombre === name)
  return hit?.tipovar_id ? String(hit.tipovar_id) : ''
}
function matchUnidad(simbolo?: string): string {
  const hit = simbolo ? unidades.value.find(u => u.simbolo === simbolo) : undefined
  if (hit?.unidad_id) return String(hit.unidad_id)
  const adim = unidades.value.find(u => u.simbolo === 'Adimensional')
  return adim?.unidad_id ? String(adim.unidad_id) : ''
}

// includeSystem: when on, the gateway's own System/* host telemetry (CPU,
// memory, disk, network…) is offered for registration as catalog señales too —
// each channelized metric carries its instance (e.g. Network/Rx_MB @ docker0).
// Off by default so a node's plant signals stay the focus.
const includeSystem = ref(false)

// isRegistrable decides which metric kinds become catalog señales: always plant
// 'proceso' signals; 'sistema' (host telemetry) only when the operator opts in.
function isRegistrable(kind: MetricKind): boolean {
  return kind === 'proceso' || (includeSystem.value && kind === 'sistema')
}

// Recompute the plan from the current target + catalog + template selection.
function rebuildSignalPlan() {
  for (const k of Object.keys(signalPlan)) delete signalPlan[k]
  if (!approveTarget.value) return
  for (const m of buildReviewMetrics(approveTarget.value)) {
    if (!isRegistrable(m.kind)) continue
    // Prefer the producer-declared FIWARE decomposition (uns/*); fall back to
    // deriving from the metric name for producers that don't send it.
    const code = m.unsCode || signalCode(m.name)
    const instance = m.unsInstance || signalInstance(m.name)
    const channelized = instance !== 'default'
    const base = indexedBase(code)
    // A channelized signal is bound per-equipo with its instance, so it always
    // goes through the create path (never the type-template/link path).
    const covered = !channelized && (templateCodes.value.has(code) || (!!base && templateCodes.value.has(base)))
    if (covered) continue // already mapped by the tipo template
    const hit = channelized ? undefined : (catalogByCode.value.get(code) || (base ? catalogByCode.value.get(base) : undefined))
    if (hit) {
      signalPlan[m.name] = {
        name: m.name, code, instance, status: 'catalog', selected: true, senalId: hit.senal_id,
        nombre: hit.nombre, descripcion: '', tipovarId: '', unidadId: '', tipoValor: '', codeTooLong: false,
      }
    } else {
      signalPlan[m.name] = {
        name: m.name, code, instance, status: 'new', selected: code.length <= 20,
        // Producer-declared display name (uns/name, e.g. "Valvula abierta")
        // pre-fills nombre; uns/description pre-fills descripcion.
        nombre: m.unsName || m.description || code,
        descripcion: m.description || '',
        tipovarId: matchTipoVar(m.tipoVariable),
        unidadId:  matchUnidad(m.engUnit),
        tipoValor: m.tipoValor === 'Acumulado' ? 'Acumulado' : 'Instantaneo',
        codeTooLong: code.length > 20,
      }
    }
  }
}

// How many metrics are eligible to become señales given the current toggle.
const registrableCount = computed(() =>
  approveTarget.value ? buildReviewMetrics(approveTarget.value).filter(m => isRegistrable(m.kind)).length : 0)
// How many host (sistema) metrics exist — drives the "include system" toggle.
const sistemaCount = computed(() =>
  approveTarget.value ? buildReviewMetrics(approveTarget.value).filter(m => m.kind === 'sistema').length : 0)
const planEntries = computed(() => Object.values(signalPlan))
const planSummary = computed(() => {
  const e = planEntries.value
  const cat = e.filter(x => x.status === 'catalog')
  const nw  = e.filter(x => x.status === 'new')
  return {
    mapped:     Math.max(0, registrableCount.value - e.length),
    catalog:    cat.length,
    new:        nw.length,
    willCreate: nw.filter(x => x.selected).length,
    willLink:   cat.filter(x => x.selected).length,
  }
})
// Every selected new señal must have its variable + unidad set and a valid code.
const planValid = computed(() => planEntries.value.every(e =>
  !(e.selected && e.status === 'new') || (!!e.tipovarId && !!e.unidadId && !e.codeTooLong)))

// Reload template + rebuild plan whenever the operator changes the tipo_equipo.
watch(() => approveForm.tipo_id, async (v) => {
  if (approveReadOnly.value) return
  await loadTemplateCodes(Number(v) || 0)
  rebuildSignalPlan()
})
// Re-plan when the operator toggles host-metric inclusion.
watch(includeSystem, () => { if (!approveReadOnly.value) rebuildSignalPlan() })

function openInspect(e: AutodiscEntity) {
  approveTarget.value = e
  approveReadOnly.value = true
  metricSearch.value = ''
  approveDialog.value = true
}

async function fetchAutodiscovered() {
  try {
    const r = await api.get('/ssfv/autodiscovered')
    autoEntities.value = r.data ?? []
  } catch { autoEntities.value = [] }
}

async function openApprove(e: AutodiscEntity) {
  approveTarget.value = e
  approveReadOnly.value = false
  metricSearch.value = ''
  includeSystem.value = false
  const defaultTopic = e.device_id
    ? `${e.group_id}/${e.node_id}/${e.device_id}`
    : `${e.group_id}/${e.node_id}`

  // Try to pre-fill tipo_id from node_properties.entity_type or first metric_meta device_type
  const suggestedType = e.node_properties?.entity_type
    || e.metric_meta?.[0]?.device_type
    || ''
  const matched = tipoEquipos.value.find(t => t.nombre === suggestedType)

  // Pre-select the planta whose group (broker_base) matches the entity's
  // group_id; when none exists, offer staging a new one whose alias comes from
  // the NBIRTH uns/planta property (fallback: the group_id itself).
  const plantaHit = plantas.value.find(p => p.broker_base === e.group_id)
  const plantaAlias = e.node_properties?.['uns/planta'] || e.node_properties?.planta || e.group_id

  Object.assign(approveForm, {
    planta_id: plantaHit ? String(plantaHit.planta_id) : PLANTA_NEW,
    tipo_id:   matched ? String(matched.tipo_id) : '',
    nombre_equipo: e.device_id || e.node_id,
    nombre_topic: defaultTopic,
    planta_nombre: plantaAlias,
  })
  approveDialog.value = true

  // Reconcile reported metrics against catalog + template for the signal plan.
  try {
    if (!senales.value.length) await fetchSenales()
    await loadTemplateCodes(matched?.tipo_id ?? 0)
    rebuildSignalPlan()
  } catch { /* plan stays empty; operator can still approve standard template */ }
}

async function submitApprove() {
  if (!approveTarget.value) return
  if (!planValid.value) {
    toast.error('Completa variable y unidad de cada señal nueva marcada')
    return
  }
  const createSignals = planEntries.value
    .filter(e => e.selected && e.status === 'new')
    .map(e => ({
      codigo_senal:     e.code,
      nombre_instancia: e.instance,
      nombre:           e.nombre || e.code,
      tipavar_id:       Number(e.tipovarId),
      unidad_id:        Number(e.unidadId),
      tipo_valor:       e.tipoValor || 'Instantaneo',
      descripcion:      e.descripcion || e.nombre || null,
    }))
  const linkSignals = planEntries.value
    .filter(e => e.selected && e.status === 'catalog' && e.senalId)
    .map(e => e.senalId as number)
  const stagingPlanta = approveForm.planta_id === PLANTA_NEW
  try {
    // 207 Multi-Status (partial) is a 2xx, so axios resolves it — inspect the body
    // for `rejected[]` so the operator sees exactly what didn't register.
    const res = await api.post(`/ssfv/autodiscovered/${approveTarget.value.id}/approve`, {
      planta_id:      stagingPlanta ? 0 : Number(approveForm.planta_id),
      create_planta:  stagingPlanta
        ? { nombre: approveForm.planta_nombre, broker_base: approveTarget.value.group_id }
        : undefined,
      tipo_id:        Number(approveForm.tipo_id),
      nombre_equipo:  approveForm.nombre_equipo,
      nombre_topic:   approveForm.nombre_topic,
      create_signals: createSignals,
      link_signals:   linkSignals,
    })
    const created = res.data?.created ?? createSignals.length
    const rejected: { codigo_senal: string; reason: string }[] = res.data?.rejected ?? []
    if (rejected.length) {
      toast.warning(`Aprobado parcial · ${created} creada(s), ${rejected.length} rechazada(s): ` +
        rejected.map(r => `${r.codigo_senal} (${r.reason})`).join(' · '))
    } else {
      toast.success(`Equipo creado · ${created} nueva(s), ${linkSignals.length} vinculada(s). ` +
        `Las señales aparecen en «Catálogo» y al expandir el equipo en «Plantas».`)
    }
    approveDialog.value = false
    await Promise.all([fetchAutodiscovered(), fetchSenales(), ...(stagingPlanta ? [fetchPlantas()] : [])])
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error aprobando') }
}

async function rejectEntity(e: AutodiscEntity) {
  const ok = await confirm({
    title: 'Rechazar entidad',
    message: '¿Marcar esta entidad como rechazada? No se creará ningún equipo.',
    detail: e.device_id ? `${e.node_id}/${e.device_id}` : e.node_id,
    variant: 'danger',
    confirmText: 'Rechazar',
  })
  if (!ok) return
  try {
    await api.post(`/ssfv/autodiscovered/${e.id}/reject`)
    toast.success('Entidad rechazada')
    await fetchAutodiscovered()
  } catch (err: any) { toast.error(err.response?.data?.error ?? 'Error') }
}

async function resetEntity(e: AutodiscEntity) {
  try {
    await api.post(`/ssfv/autodiscovered/${e.id}/reset`)
    toast.success('Entidad restablecida — lista para re-asignación')
    await fetchAutodiscovered()
  } catch (err: any) { toast.error(err.response?.data?.error ?? 'Error restableciendo') }
}

async function deleteAutodiscoveredEntity(e: AutodiscEntity) {
  const ok = await confirm({
    title: 'Eliminar registro rechazado',
    message: '¿Eliminar definitivamente esta entrada? No se puede deshacer.',
    detail: e.device_id ? `${e.node_id}/${e.device_id}` : e.node_id,
    variant: 'danger',
    confirmText: 'Eliminar',
  })
  if (!ok) return
  try {
    await api.delete(`/ssfv/autodiscovered/${e.id}`)
    toast.success('Registro eliminado')
    await fetchAutodiscovered()
  } catch (err: any) { toast.error(err.response?.data?.error ?? 'Error eliminando') }
}

// ─── Pendientes a escala: búsqueda, filtro, agrupación y selección ────────────
// Con miles de plantas el bus produce miles de entidades descubiertas; la vista
// agrupa por group_id (= planta Sparkplug), filtra por estado (pendientes por
// defecto) y permite buscar por group/nodo/device o por nombre de métrica.
const autoSearch       = ref('')
const autoStatusFilter = ref<'pending' | 'approved' | 'rejected' | 'all'>('pending')
const autoSelected     = ref<Set<number>>(new Set())

const filteredAuto = computed<AutodiscEntity[]>(() => {
  const q = autoSearch.value.trim().toLowerCase()
  let list = autoEntities.value
  if (autoStatusFilter.value !== 'all') list = list.filter(e => e.status === autoStatusFilter.value)
  if (q) {
    list = list.filter(e =>
      e.group_id.toLowerCase().includes(q) ||
      e.node_id.toLowerCase().includes(q) ||
      e.device_id.toLowerCase().includes(q) ||
      e.metric_names.some(n => n.toLowerCase().includes(q)))
  }
  return list
})

interface AutoGroup {
  group: string
  planta: SSFVPlanta | null     // existing planta whose broker_base = group_id
  entities: AutodiscEntity[]    // node entity first, then devices by topic
  pending: number
}

const autoGroups = computed<AutoGroup[]>(() => {
  const byGroup = new Map<string, AutodiscEntity[]>()
  for (const e of filteredAuto.value) {
    if (!byGroup.has(e.group_id)) byGroup.set(e.group_id, [])
    byGroup.get(e.group_id)!.push(e)
  }
  return [...byGroup.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([group, entities]) => ({
      group,
      planta: plantas.value.find(p => p.broker_base === group) ?? null,
      entities: [...entities].sort((a, b) =>
        a.node_id.localeCompare(b.node_id) ||
        (a.device_id === '' ? -1 : b.device_id === '' ? 1 : a.device_id.localeCompare(b.device_id))),
      pending: entities.filter(e => e.status === 'pending').length,
    }))
})

// Selection only ever holds PENDING entities (the bulk actions' domain).
const selectedAuto = computed(() =>
  autoEntities.value.filter(e => e.status === 'pending' && autoSelected.value.has(e.id)))

function toggleAutoSelect(id: number) {
  const s = new Set(autoSelected.value)
  s.has(id) ? s.delete(id) : s.add(id)
  autoSelected.value = s
}
function groupSelectable(g: AutoGroup) { return g.entities.filter(e => e.status === 'pending') }
function groupAllSelected(g: AutoGroup) {
  const sel = groupSelectable(g)
  return sel.length > 0 && sel.every(e => autoSelected.value.has(e.id))
}
function toggleGroupSelect(g: AutoGroup) {
  const s = new Set(autoSelected.value)
  const sel = groupSelectable(g)
  const all = groupAllSelected(g)
  for (const e of sel) all ? s.delete(e.id) : s.add(e.id)
  autoSelected.value = s
}
const visiblePendingAuto = computed(() => filteredAuto.value.filter(e => e.status === 'pending'))
const allVisibleSelected = computed(() =>
  visiblePendingAuto.value.length > 0 && visiblePendingAuto.value.every(e => autoSelected.value.has(e.id)))
function toggleSelectAllVisible() {
  const s = new Set(autoSelected.value)
  const all = allVisibleSelected.value
  for (const e of visiblePendingAuto.value) all ? s.delete(e.id) : s.add(e.id)
  autoSelected.value = s
}
function clearAutoSelection() { autoSelected.value = new Set() }

// ─── Aprobación en lote ───────────────────────────────────────────────────────
// El endpoint /approve es idempotente extremo a extremo (planta ON CONFLICT
// broker_base, equipo ON CONFLICT nombre_topic, señales/plantilla ON CONFLICT),
// así que el lote es un bucle secuencial del mismo flujo que la aprobación
// individual, con la planta/tipo/señales derivados igual que en openApprove.
interface BulkRow {
  entity: AutodiscEntity
  topic: string
  plantaId: number              // 0 ⇒ se crea/reusa por create_planta
  tipoId: string                // Select model; prellenado del payload
  tipoDetected: string
  nuevas: number                // señales nuevas a crear
  vinculadas: number            // señales de catálogo a vincular a la plantilla
  omitidas: { code: string; reason: string }[]
  state: 'listo' | 'corriendo' | 'ok' | 'parcial' | 'error'
  detail: string
}

const bulkDialog      = ref(false)
const bulkRows        = ref<BulkRow[]>([])
// Plantas a crear durante el lote: una entrada editable por group sin planta.
const bulkNewPlantas  = ref<{ group: string; nombre: string }[]>([])
// Variable por defecto para señales nuevas cuyo payload no trae tipo_variable.
const bulkFallbackVar = ref('')
const bulkRunning     = ref(false)
const bulkDone        = ref(false)

// Igual que en la aprobación individual: telemetría del host (System/*) solo
// se registra como señales cuando el operador lo pide explícitamente.
const bulkIncludeSystem = ref(false)

// Plan de señales por entidad (espejo de rebuildSignalPlan, en versión pura).
// No deduplica contra la plantilla del tipo: el backend lo hace con ON
// CONFLICT, así que recrear/relink es inocuo.
function bulkPlanFor(e: AutodiscEntity, fallbackVarId: string) {
  const create: any[] = []
  const link: number[] = []
  const omitidas: { code: string; reason: string }[] = []
  for (const m of buildReviewMetrics(e)) {
    if (m.kind !== 'proceso' && !(bulkIncludeSystem.value && m.kind === 'sistema')) continue
    const code = m.unsCode || signalCode(m.name)
    const instance = m.unsInstance || signalInstance(m.name)
    const channelized = instance !== 'default'
    const base = indexedBase(code)
    const hit = channelized ? undefined
      : (catalogByCode.value.get(code) || (base ? catalogByCode.value.get(base) : undefined))
    if (hit) { if (hit.senal_id) link.push(hit.senal_id); continue }
    if (code.length > 20) { omitidas.push({ code, reason: 'código >20 caracteres' }); continue }
    const tipovarId = matchTipoVar(m.tipoVariable) || fallbackVarId
    if (!tipovarId) { omitidas.push({ code, reason: 'sin tipo de variable' }); continue }
    create.push({
      codigo_senal:     code,
      nombre_instancia: instance,
      nombre:           m.unsName || m.description || code,
      tipavar_id:       Number(tipovarId),
      unidad_id:        Number(matchUnidad(m.engUnit)),
      tipo_valor:       m.tipoValor === 'Acumulado' ? 'Acumulado' : 'Instantaneo',
      descripcion:      m.description || m.unsName || null,
    })
  }
  return { create, link, omitidas }
}

function refreshBulkCounts() {
  for (const row of bulkRows.value) {
    const plan = bulkPlanFor(row.entity, bulkFallbackVar.value)
    row.nuevas = plan.create.length
    row.vinculadas = plan.link.length
    row.omitidas = plan.omitidas
  }
}
watch([bulkFallbackVar, bulkIncludeSystem], () => { if (!bulkRunning.value && !bulkDone.value) refreshBulkCounts() })

async function openBulkApprove() {
  if (!selectedAuto.value.length) return
  if (!senales.value.length) { try { await fetchSenales() } catch { /* counts may be off */ } }
  bulkDone.value = false
  bulkRunning.value = false
  bulkFallbackVar.value = ''
  bulkIncludeSystem.value = false
  // Alias de las plantas a crear: el uns/planta viene en el NBIRTH del nodo,
  // no en los DBIRTH de sus devices — busca el alias en CUALQUIER entidad del
  // group antes de caer al group_id.
  const groupsNeedingPlanta = new Map<string, string>()
  for (const e of selectedAuto.value) {
    if (plantas.value.some(p => p.broker_base === e.group_id)) continue
    const alias = e.node_properties?.['uns/planta'] || e.node_properties?.planta
    const cur = groupsNeedingPlanta.get(e.group_id)
    if (alias && (!cur || cur === e.group_id)) groupsNeedingPlanta.set(e.group_id, alias)
    else if (!groupsNeedingPlanta.has(e.group_id)) groupsNeedingPlanta.set(e.group_id, e.group_id)
  }
  bulkRows.value = selectedAuto.value.map(e => {
    const plantaHit = plantas.value.find(p => p.broker_base === e.group_id)
    const suggestedType = e.node_properties?.entity_type || e.metric_meta?.[0]?.device_type || ''
    const matched = tipoEquipos.value.find(t => t.nombre === suggestedType)
    return {
      entity: e,
      topic: e.device_id ? `${e.group_id}/${e.node_id}/${e.device_id}` : `${e.group_id}/${e.node_id}`,
      plantaId: plantaHit?.planta_id ?? 0,
      tipoId: matched?.tipo_id ? String(matched.tipo_id) : '',
      tipoDetected: suggestedType,
      nuevas: 0, vinculadas: 0, omitidas: [],
      state: 'listo', detail: '',
    } as BulkRow
  })
  bulkNewPlantas.value = [...groupsNeedingPlanta.entries()].map(([group, nombre]) => ({ group, nombre }))
  refreshBulkCounts()
  bulkDialog.value = true
}

// Aplica un tipo de equipo a todas las filas que aún no tienen uno.
const bulkFillTipo = ref('')
watch(bulkFillTipo, (v) => {
  if (!v) return
  for (const row of bulkRows.value) if (!row.tipoId) row.tipoId = v
  bulkFillTipo.value = ''
})

const bulkReady = computed(() =>
  bulkRows.value.length > 0 &&
  bulkRows.value.every(r => !!r.tipoId) &&
  bulkNewPlantas.value.every(p => p.nombre.trim() !== ''))
const bulkMissingTipo = computed(() => bulkRows.value.filter(r => !r.tipoId).length)
const bulkProgress = computed(() => {
  const done = bulkRows.value.filter(r => r.state === 'ok' || r.state === 'parcial' || r.state === 'error').length
  return bulkRows.value.length ? Math.round(100 * done / bulkRows.value.length) : 0
})
const bulkSummary = computed(() => ({
  ok:      bulkRows.value.filter(r => r.state === 'ok').length,
  parcial: bulkRows.value.filter(r => r.state === 'parcial').length,
  error:   bulkRows.value.filter(r => r.state === 'error').length,
}))

async function runBulkApprove() {
  if (!bulkReady.value || bulkRunning.value) return
  bulkRunning.value = true
  for (const row of bulkRows.value) {
    row.state = 'corriendo'
    const plan = bulkPlanFor(row.entity, bulkFallbackVar.value)
    const newPlanta = bulkNewPlantas.value.find(p => p.group === row.entity.group_id)
    try {
      const res = await api.post(`/ssfv/autodiscovered/${row.entity.id}/approve`, {
        planta_id:     row.plantaId,
        create_planta: row.plantaId === 0 && newPlanta
          ? { nombre: newPlanta.nombre.trim(), broker_base: row.entity.group_id }
          : undefined,
        tipo_id:        Number(row.tipoId),
        nombre_equipo:  row.entity.device_id || row.entity.node_id,
        nombre_topic:   row.topic,
        create_signals: plan.create,
        link_signals:   plan.link,
      })
      const rejected: any[] = res.data?.rejected ?? []
      row.state = rejected.length ? 'parcial' : 'ok'
      row.detail = `${res.data?.created ?? 0} creadas · ${res.data?.linked ?? 0} vinculadas`
        + (rejected.length ? ` · ${rejected.length} rechazadas` : '')
        + (plan.omitidas.length ? ` · ${plan.omitidas.length} omitidas` : '')
    } catch (err: any) {
      row.state = 'error'
      row.detail = err.response?.data?.error ?? err.message ?? 'error'
    }
  }
  bulkRunning.value = false
  bulkDone.value = true
  const s = bulkSummary.value
  if (s.error === 0 && s.parcial === 0) toast.success(`${s.ok} entidad(es) aprobadas en lote`)
  else toast.warning(`Lote terminado: ${s.ok} ok · ${s.parcial} parciales · ${s.error} con error`)
  clearAutoSelection()
  await Promise.all([fetchAutodiscovered(), fetchSenales(), fetchPlantas()])
}

async function bulkReject() {
  const targets = selectedAuto.value
  if (!targets.length) return
  const ok = await confirm({
    title: 'Rechazar en lote',
    message: `¿Marcar ${targets.length} entidad(es) como rechazadas? No se creará ningún equipo.`,
    variant: 'danger',
    confirmText: `Rechazar ${targets.length}`,
  })
  if (!ok) return
  let failed = 0
  for (const e of targets) {
    try { await api.post(`/ssfv/autodiscovered/${e.id}/reject`) } catch { failed++ }
  }
  failed ? toast.warning(`${targets.length - failed} rechazadas · ${failed} con error`)
         : toast.success(`${targets.length} entidad(es) rechazadas`)
  clearAutoSelection()
  await fetchAutodiscovered()
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────
onMounted(async () => {
  try {
    await fetchStatus()
    if (status.value?.connected) {
      await Promise.all([fetchCatalogs(), fetchPlantas(), fetchAutodiscovered(), fetchIec104Servers()])
    }
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Error cargando SSFV')
  }
})

watch(tab, async (t) => {
  if (!status.value?.connected) return
  try {
    if (t === 'plantas')    await Promise.all([fetchPlantas(), fetchCatalogs()])
    if (t === 'catalogo')   await Promise.all([fetchSenales(), fetchCatalogs()])
    if (t === 'tipos')      await fetchCatalogs()
    if (t === 'estado')     await Promise.all([fetchStatus(), fetchMissed()])
    if (t === 'pendientes') await Promise.all([fetchAutodiscovered(), fetchCatalogs(), fetchSenales(), fetchPlantas()])
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Error cargando datos')
  }
})

// ─── Helpers ──────────────────────────────────────────────────────────────────
function estadoLabel(e: number) { return e === 1 ? 'Activa' : e === 0 ? 'Inactiva' : 'Mantenimiento' }
function estadoClass(e: number) { return e === 1 ? 'bg-green-500/15 text-green-500' : e === 2 ? 'bg-amber-500/15 text-amber-500' : 'bg-muted text-muted-foreground' }

function tipoEquipoIcon(nombre: string) {
  if (/inversor/i.test(nombre)) return Cpu
  if (/medidor/i.test(nombre)) return Activity
  if (/estaci/i.test(nombre)) return Sun
  if (/frontera/i.test(nombre)) return Link2
  return GitBranch
}

  return {
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
    buildReviewMetrics,
    bulkDialog,
    bulkDone,
    bulkFallbackVar,
    bulkFillTipo,
    bulkIncludeSystem,
    bulkMissingTipo,
    bulkNewPlantas,
    bulkPlanFor,
    bulkProgress,
    bulkReady,
    bulkReject,
    bulkRows,
    bulkRunning,
    bulkSummary,
    catalogByCode,
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
    equipoPlantaCtx,
    equipoSignals,
    equipoTopicInvalid,
    estadoClass,
    estadoLabel,
    expandedAuto,
    fetchAutodiscovered,
    fetchCatalogs,
    fetchIec104Servers,
    fetchMissed,
    fetchMonitoreo,
    fetchPlantas,
    fetchSenales,
    fetchStatus,
    filteredAuto,
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
    HOST_CATEGORIES,
    iec104Servers,
    includeSystem,
    indexedBase,
    invalidateCache,
    isNodeEntity,
    isRegistrable,
    KIND_META,
    lastSegment,
    loadEquiposByPlanta,
    loadEquipoSignals,
    loadFronterasByPlanta,
    loadSenalXTipo,
    loadTemplateCodes,
    matchTipoVar,
    matchUnidad,
    metricInCatalog,
    metricKind,
    metricSearch,
    missed,
    monTab,
    nodeOf,
    nodeTree,
    nowTick,
    nowTimer,
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
    planEntries,
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
    rebuildSignalPlan,
    refreshBulkCounts,
    refreshing,
    registrableCount,
    rejectEntity,
    relTime,
    resetEntity,
    reviewFiltered,
    reviewGroups,
    reviewMetrics,
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
    senalXTipoCtxSenalId,
    senalXTipoDialog,
    senalXTipoForm,
    signalCode,
    signalInstance,
    signalPlan,
    sistemaCount,
    status,
    submitApprove,
    tab,
    templateCodes,
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
  }
}

let _instance: ReturnType<typeof create> | null = null
export function useSsfv() {
  return (_instance ??= create())
}
