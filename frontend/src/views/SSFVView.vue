<script setup lang="ts">
import { ref, computed, onMounted, watch, reactive } from 'vue'
import { toast } from 'vue-sonner'
import {
  api,
  type SSFVStatus, type SSFVPlanta, type SSFVEquipo, type SSFVSenal,
  type SSFVFrontera,
  type SSFVTipoEquipo, type SSFVTipoVariable, type SSFVUnidad,
  type SSFVSenalXTipo,
} from '@/api'
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
  ChevronRight, Zap, Building2, Cpu, Layers,
  Link2, Activity, Ruler, GitBranch, TriangleAlert, Scan,
} from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

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

function togglePlanta(id: number) {
  plantaOpen.value = plantaOpen.value === id ? null : id
  if (plantaOpen.value === id) {
    loadEquiposByPlanta(id)
    loadFronterasByPlanta(id)
  }
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
    message: 'Se eliminarán todos los equipos, señales instanciadas y fronteras asociadas.',
    detail: p.nombre,
    variant: 'danger',
    confirmText: 'Eliminar planta',
  })
  if (!ok) return
  try {
    await api.delete(`/ssfv/plantas/${p.planta_id}`)
    toast.success('Planta eliminada')
    if (plantaOpen.value === p.planta_id) plantaOpen.value = null
    await fetchPlantas()
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error') }
}

// Sparkplug B derivation
const spGroupId = computed(() => plantaForm.broker_base.split('/')[1] ?? '')
const spNodeId  = computed(() => plantaForm.broker_base.split('/')[3] ?? '')

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

// Equipo dialog
const equipoDialog = ref(false)
const equipoEdit   = ref<SSFVEquipo | null>(null)
const equipoPlantaCtx = ref(0)
const equipoForm   = reactive({ planta_id: 0, tipo_id: 0, nombre_equipo: '', nombre_topic: '', nombre_topic_suffix: '', fabricante: '', modelo: '', nro_serie: '', estado: 1 })

watch(() => equipoForm.nombre_topic_suffix, (suffix) => {
  const pid = equipoForm.planta_id
  if (!pid || !suffix || equipoEdit.value) return
  const planta = plantas.value.find(p => p.planta_id === pid)
  if (planta) equipoForm.nombre_topic = planta.broker_base + '/' + suffix
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
      toast.success('Equipo creado — señales auto-instanciadas según tipo')
    }
    equipoDialog.value = false
    await loadEquiposByPlanta(equipoForm.planta_id)
  } catch (e: any) { toast.error(e.response?.data?.error ?? 'Error guardando equipo') }
}
async function deleteEquipo(eq: SSFVEquipo) {
  const ok = await confirm({
    title: 'Eliminar equipo',
    message: 'Se eliminan todas las señales instanciadas de este equipo.',
    detail: eq.nombre_equipo,
    variant: 'danger',
    confirmText: 'Eliminar equipo',
  })
  if (!ok) return
  try {
    await api.delete(`/ssfv/equipos/${eq.equipo_id}`)
    toast.success('Equipo eliminado')
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
  const ok = await confirm({ title: 'Eliminar señal', message: `${s.codigo_senal} — ${s.nombre}`, variant: 'danger', confirmText: 'Eliminar señal' })
  if (!ok) return
  try {
    await api.delete(`/ssfv/senales/${s.senal_id}`)
    toast.success('Señal eliminada')
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
interface AutodiscEntity {
  id: number
  group_id: string
  node_id: string
  device_id: string
  metric_names: string[]
  status: 'pending' | 'approved' | 'rejected'
  first_seen: string
  last_seen: string
}

const autoEntities       = ref<AutodiscEntity[]>([])
const autoPendingCount   = computed(() => autoEntities.value.filter(e => e.status === 'pending').length)
const approveDialog      = ref(false)
const approveTarget      = ref<AutodiscEntity | null>(null)
const approveForm        = reactive({ planta_id: '', tipo_id: '', nombre_equipo: '', nombre_topic: '' })

async function fetchAutodiscovered() {
  try {
    const r = await api.get('/ssfv/autodiscovered')
    autoEntities.value = r.data ?? []
  } catch { autoEntities.value = [] }
}

function openApprove(e: AutodiscEntity) {
  approveTarget.value = e
  const defaultTopic = e.device_id
    ? `${e.group_id}/${e.node_id}/${e.device_id}`
    : `${e.group_id}/${e.node_id}`
  Object.assign(approveForm, {
    planta_id: '',
    tipo_id: '',
    nombre_equipo: e.device_id || e.node_id,
    nombre_topic: defaultTopic,
  })
  approveDialog.value = true
}

async function submitApprove() {
  if (!approveTarget.value) return
  try {
    await api.post(`/ssfv/autodiscovered/${approveTarget.value.id}/approve`, {
      planta_id:     Number(approveForm.planta_id),
      tipo_id:       Number(approveForm.tipo_id),
      nombre_equipo: approveForm.nombre_equipo,
      nombre_topic:  approveForm.nombre_topic,
    })
    toast.success('Equipo creado y señales instanciadas')
    approveDialog.value = false
    await fetchAutodiscovered()
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

// ─── Lifecycle ────────────────────────────────────────────────────────────────
onMounted(async () => {
  await fetchStatus()
  if (status.value?.connected) {
    await Promise.all([fetchCatalogs(), fetchPlantas(), fetchAutodiscovered()])
  }
})

watch(tab, async (t) => {
  if (!status.value?.connected) return
  if (t === 'plantas')    await Promise.all([fetchPlantas(), fetchCatalogs()])
  if (t === 'catalogo')   await Promise.all([fetchSenales(), fetchCatalogs()])
  if (t === 'tipos')      await fetchCatalogs()
  if (t === 'estado')     await Promise.all([fetchStatus(), fetchMissed()])
  if (t === 'pendientes') await Promise.all([fetchAutodiscovered(), fetchCatalogs()])
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
      <div class="flex justify-end">
        <Button
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
          @click="openCreatePlanta"
        >
          <Plus class="h-4 w-4 mr-1.5" /> Nueva planta
        </Button>
      </div>

      <div v-if="!plantas.length" class="card-soft p-8 text-center text-sm text-muted-foreground">
        No hay plantas configuradas. Crea la primera planta solar para comenzar.
      </div>

      <!-- Plant cards -->
      <div v-for="p in plantas" :key="p.planta_id" class="card-soft overflow-hidden">

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
            <div class="font-bold text-sm">{{ p.nombre }}</div>
            <div class="text-[11px] text-muted-foreground font-mono truncate">{{ p.broker_base }}</div>
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

            <div v-for="eq in plantaEquipos[p.planta_id!] ?? []" :key="eq.equipo_id" class="rounded-sm border border-border bg-card mb-2 overflow-hidden">
              <!-- Equipo row -->
              <div class="flex items-center gap-3 px-3 py-2">
                <button type="button" class="flex items-center gap-2 flex-1 min-w-0 text-left" @click="toggleEquipo(eq.equipo_id!)">
                  <ChevronRight
                    class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform"
                    :class="equipoOpen === eq.equipo_id ? 'rotate-90' : ''"
                  />
                  <div class="grid place-items-center w-6 h-6 rounded-sm bg-muted shrink-0">
                    <component :is="tipoEquipoIcon(eq.tipo_nombre ?? '')" class="h-3.5 w-3.5 text-muted-foreground" />
                  </div>
                  <div class="min-w-0">
                    <div class="text-sm font-semibold truncate">{{ eq.nombre_equipo }}</div>
                    <div class="text-[10px] text-muted-foreground font-mono truncate">{{ eq.nombre_topic }}</div>
                  </div>
                </button>
                <div class="flex items-center gap-2 shrink-0">
                  <span class="text-[10px] px-1.5 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)] text-[color:var(--epm-bosque)] font-semibold">
                    {{ eq.tipo_nombre }}
                  </span>
                  <Button variant="ghost" size="icon" class="h-6 w-6" @click="openEditEquipo(eq)"><Pencil class="h-3 w-3" /></Button>
                  <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" @click="deleteEquipo(eq)"><Trash2 class="h-3 w-3" /></Button>
                </div>
              </div>

              <!-- Signal instances panel -->
              <div v-if="equipoOpen === eq.equipo_id" class="border-t border-border/60 bg-muted/10 px-4 py-2">
                <div class="text-[10px] uppercase tracking-[0.16em] text-muted-foreground mb-2 font-semibold">
                  Señales instanciadas — <span class="font-mono font-normal">{{ (equipoSignals[eq.equipo_id!] ?? []).length }} puntos</span>
                </div>
                <div v-if="!(equipoSignals[eq.equipo_id!] ?? []).length" class="text-xs text-muted-foreground italic">Sin señales instanciadas</div>
                <div v-else class="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
                        <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold w-32">Instancia</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Señal</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Unidad</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Tipo</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold text-right">equisenal_id</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      <TableRow v-for="s in equipoSignals[eq.equipo_id!]" :key="s.equisenal_id" class="border-b border-border/40 last:border-0">
                        <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-citrico)]">{{ s.nombre_instancia }}</TableCell>
                        <TableCell class="text-xs">{{ s.senal_nombre }}</TableCell>
                        <TableCell class="text-xs text-muted-foreground">{{ s.unidad }}</TableCell>
                        <TableCell>
                          <span v-if="s.es_alarma" class="text-[10px] px-1.5 py-0.5 rounded-sm bg-red-500/15 text-red-400 font-semibold">Alarma</span>
                          <span v-else class="text-[10px] text-muted-foreground">{{ s.tipo_valor }}</span>
                        </TableCell>
                        <TableCell class="text-right font-mono text-[10px]">
                          <span class="px-1.5 py-0.5 rounded-sm bg-muted text-muted-foreground">{{ s.equisenal_id }}</span>
                        </TableCell>
                      </TableRow>
                    </TableBody>
                  </Table>
                </div>
              </div>
            </div>
          </div>
        </div>
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

      <div v-for="s in filteredSenales" :key="s.senal_id" class="card-soft overflow-hidden">
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
      <div class="flex items-center justify-between">
        <p class="text-sm text-muted-foreground">
          Nodos y dispositivos Sparkplug B detectados en el bus sin entrada en el catálogo SSFV.
          Aprueba para crear el equipo y auto-instanciar señales, o rechaza para ignorar.
        </p>
        <Button variant="outline" size="sm" @click="fetchAutodiscovered">
          <RefreshCw class="h-3.5 w-3.5 mr-1.5" /> Actualizar
        </Button>
      </div>

      <div class="overflow-x-auto rounded-sm border border-border">
        <Table>
          <TableHeader>
            <TableRow class="bg-muted/30">
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Grupo</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Nodo</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Dispositivo</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Métricas</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Estado</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.16em] font-bold">Última vez</TableHead>
              <TableHead class="text-right" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="e in autoEntities"
              :key="e.id"
              class="border-b border-border/50 last:border-0 hover:bg-muted/20"
            >
              <TableCell class="font-mono text-xs">{{ e.group_id }}</TableCell>
              <TableCell class="font-mono text-xs font-medium">{{ e.node_id }}</TableCell>
              <TableCell class="font-mono text-xs text-muted-foreground">{{ e.device_id || '—' }}</TableCell>
              <TableCell class="text-xs text-muted-foreground">
                <span class="font-mono">{{ e.metric_names.length }}</span>
                <span class="text-[10px] ml-1 truncate max-w-[180px] inline-block align-bottom">
                  {{ e.metric_names.slice(0, 3).join(', ') }}{{ e.metric_names.length > 3 ? '…' : '' }}
                </span>
              </TableCell>
              <TableCell>
                <span class="text-[10px] px-1.5 py-0.5 rounded-sm font-semibold" :class="{
                  'bg-blue-500/15 text-blue-400':    e.status === 'pending',
                  'bg-green-500/15 text-green-500':  e.status === 'approved',
                  'bg-muted text-muted-foreground':  e.status === 'rejected',
                }">
                  {{ e.status === 'pending' ? 'Pendiente' : e.status === 'approved' ? 'Aprobado' : 'Rechazado' }}
                </span>
              </TableCell>
              <TableCell class="font-mono text-[11px] text-muted-foreground">
                {{ new Date(e.last_seen).toLocaleString() }}
              </TableCell>
              <TableCell class="text-right">
                <div v-if="e.status === 'pending'" class="flex justify-end gap-1.5">
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
                </div>
              </TableCell>
            </TableRow>
            <TableRow v-if="!autoEntities.length">
              <TableCell colspan="7" class="text-center text-muted-foreground text-xs py-6">
                Sin entidades descubiertas. Cuando llegue un NBIRTH/DBIRTH de un nodo no catalogado aparecerá aquí.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </div>

  </div>

  <!-- ═══════════════════════════════════════════════════════════════════════
       DIALOGS
  ═══════════════════════════════════════════════════════════════════════ -->

  <!-- Aprobar entidad autodescubierta -->
  <Dialog v-model:open="approveDialog">
    <DialogContent class="!max-w-md p-0 overflow-hidden">
      <DialogHeader class="px-6 pt-6 pb-4 border-b border-border">
        <DialogTitle class="flex items-center gap-2">
          <Scan class="h-4 w-4" /> Aprobar entidad descubierta
        </DialogTitle>
        <DialogDescription v-if="approveTarget">
          {{ approveTarget.device_id
            ? `${approveTarget.group_id}/${approveTarget.node_id}/${approveTarget.device_id}`
            : `${approveTarget.group_id}/${approveTarget.node_id}` }}
          — {{ approveTarget.metric_names.length }} métricas detectadas
        </DialogDescription>
      </DialogHeader>
      <div class="px-6 py-4 space-y-4">
        <div class="grid gap-1.5">
          <Label>Planta *</Label>
          <Select v-model="approveForm.planta_id">
            <SelectTrigger class="rounded-sm"><SelectValue placeholder="Seleccionar planta…" /></SelectTrigger>
            <SelectContent>
              <SelectItem v-for="p in plantas" :key="p.planta_id" :value="String(p.planta_id)">{{ p.nombre }}</SelectItem>
            </SelectContent>
          </Select>
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
          <Label>Nombre topic MQTT</Label>
          <Input v-model="approveForm.nombre_topic" class="rounded-sm h-8 font-mono text-xs" />
        </div>
        <p class="text-[11px] text-muted-foreground">
          Se creará el equipo y se auto-instanciarán las señales del tipo seleccionado.
        </p>
      </div>
      <DialogFooter class="px-6 pb-5 gap-2">
        <Button variant="ghost" size="sm" @click="approveDialog = false">Cancelar</Button>
        <Button
          size="sm"
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
          :disabled="!approveForm.planta_id || !approveForm.tipo_id"
          @click="submitApprove"
        >
          <CheckCircle2 class="h-3.5 w-3.5 mr-1.5" /> Crear equipo
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
          <Label>Broker Base * <span class="text-[10px] text-muted-foreground">(prefijo Sparkplug B: grupo/tipo/nodo/planta)</span></Label>
          <Input v-model="plantaForm.broker_base" placeholder="EPM/SSFV/EPM/Sede30" class="font-mono" />
          <p v-if="plantaForm.broker_base" class="text-[11px] text-muted-foreground font-mono">
            spBv1.0/<span class="text-[color:var(--epm-citrico)]">{{ spGroupId }}</span>/DDATA/<span class="text-[color:var(--epm-citrico)]">{{ spNodeId }}</span>/&lt;device_id&gt;
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
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="savePlanta" :disabled="!plantaForm.nombre || !plantaForm.broker_base">
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
          <Label>Sufijo topic (device_id) *</Label>
          <Input v-model="equipoForm.nombre_topic_suffix" placeholder="INV_1" class="font-mono" />
        </div>
        <div class="space-y-1.5">
          <Label>Topic completo</Label>
          <Input v-model="equipoForm.nombre_topic" class="font-mono bg-muted/30 text-[11px]" />
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
        <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm" @click="saveEquipo" :disabled="!equipoForm.nombre_equipo || !equipoForm.nombre_topic || !equipoForm.tipo_id">
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
</template>
