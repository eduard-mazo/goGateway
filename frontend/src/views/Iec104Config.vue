<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { toast } from 'vue-sonner'
import { api, type IEC104Server, type IEC104Gateway, type SignalMapping, type GatewaySignal, type Topic, type Device } from '@/api'
import { t } from '@/i18n'
import { useStatus } from '@/composables/useStatus'
import StatusPill from '@/components/StatusPill.vue'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogTrigger,
} from '@/components/ui/dialog'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { Server, Save, RefreshCw, Plus, Pencil, Trash2, Globe, ShieldCheck, ShieldAlert, Link2, PlugZap, Layers, Database, Search, X } from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

const { status } = useStatus()

// ── Multi-select: servers ────────────────────────────────────────────────────
const selectedServers = reactive(new Set<number>())
const allServersSelected = computed(() =>
  servers.value.length > 0 && servers.value.every(s => selectedServers.has(s.id))
)
function toggleSelectAllServers() {
  if (allServersSelected.value) servers.value.forEach(s => selectedServers.delete(s.id))
  else servers.value.forEach(s => selectedServers.add(s.id))
}
function toggleSelectServer(id: number) {
  if (selectedServers.has(id)) selectedServers.delete(id)
  else selectedServers.add(id)
}
async function delSelectedServers() {
  const ids = [...selectedServers]
  const ok = await confirm({
    title: 'Eliminar servidores',
    message: `Elimina ${ids.length} servidor${ids.length === 1 ? '' : 'es'} y todos sus mapeos IOA en cascada.`,
    variant: 'danger',
    confirmText: `Eliminar ${ids.length}`,
  })
  if (!ok) return
  try {
    await Promise.all(ids.map(id => api.delete(`/iec104-servers/${id}`)))
    ids.forEach(id => selectedServers.delete(id))
    await reload()
    toast.success(`${ids.length} servidor${ids.length === 1 ? '' : 'es'} eliminado${ids.length === 1 ? '' : 's'}`)
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

// ── Multi-select: mappings ───────────────────────────────────────────────────
const selectedMappings = reactive(new Set<number>())
function mappingsAllSelectedForServer(serverId: number) {
  const rows = mappingsForServer(serverId)
  return rows.length > 0 && rows.every(m => selectedMappings.has(m.id))
}
function toggleSelectAllMappingsForServer(serverId: number) {
  const rows = mappingsForServer(serverId)
  if (mappingsAllSelectedForServer(serverId)) rows.forEach(m => selectedMappings.delete(m.id))
  else rows.forEach(m => selectedMappings.add(m.id))
}
function toggleSelectMapping(id: number) {
  if (selectedMappings.has(id)) selectedMappings.delete(id)
  else selectedMappings.add(id)
}
async function delSelectedMappings() {
  const ids = [...selectedMappings]
  const ok = await confirm({
    title: 'Eliminar mapeos',
    message: `Elimina ${ids.length} mapeo${ids.length === 1 ? '' : 's'} IOA seleccionado${ids.length === 1 ? '' : 's'}.`,
    variant: 'danger',
    confirmText: `Eliminar ${ids.length}`,
  })
  if (!ok) return
  try {
    await Promise.all(ids.map(id => api.delete(`/mappings/${id}`)))
    ids.forEach(id => selectedMappings.delete(id))
    await reloadMappings()
    toast.success(`${ids.length} mapeo${ids.length === 1 ? '' : 's'} eliminado${ids.length === 1 ? '' : 's'}`)
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

const servers = ref<IEC104Server[]>([])
const loading = ref(false)

// --- Gateway-wide listen IP -----------------------------------------------

const gateway = reactive<IEC104Gateway>({ id: 1, listen_ip: '0.0.0.0' })
const gatewayDirty = ref(false)
const savingGateway = ref(false)

async function loadGateway() {
  try {
    const { data } = await api.get<IEC104Gateway>('/iec104-gateway')
    Object.assign(gateway, data)
    gatewayDirty.value = false
  } catch (e: any) {
    toast.error('Gateway: ' + (e?.message ?? e))
  }
}

function isValidIP(s: string): boolean {
  // IPv4 dotted-quad or IPv6 (loose). Hostnames not allowed — kernel needs
  // a numeric bind address.
  if (/^(\d{1,3}\.){3}\d{1,3}$/.test(s)) {
    return s.split('.').every(o => { const n = +o; return n >= 0 && n <= 255 })
  }
  return /^[0-9a-fA-F:]+$/.test(s) && s.includes(':')
}

async function saveGateway() {
  const ip = gateway.listen_ip.trim() || '0.0.0.0'
  if (!isValidIP(ip)) { toast.error('Listen IP must be a numeric IPv4/IPv6 address'); return }
  savingGateway.value = true
  try {
    const { data } = await api.put<IEC104Gateway>('/iec104-gateway', { ...gateway, listen_ip: ip })
    Object.assign(gateway, data)
    gatewayDirty.value = false
    toast.success('Gateway listen IP saved · fleet restarted')
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Save failed')
  } finally {
    savingGateway.value = false
  }
}

// --- Per-server CRUD ------------------------------------------------------

function empty(): IEC104Server {
  return {
    id: 0, name: '', port: 2404, asdu_addr: 1, scada_ips: '',
    k: 12, w: 8, t0: 30, t1: 15, t2: 10, t3: 20, enabled: true,
  }
}

const dialogOpen = ref(false)
const editing = reactive<IEC104Server>(empty())
const isEdit = computed(() => editing.id > 0)
const saving = ref(false)

async function reload() {
  loading.value = true
  try {
    const { data } = await api.get<IEC104Server[]>('/iec104-servers')
    servers.value = data ?? []
  } catch (e: any) {
    toast.error('Load: ' + (e?.message ?? e))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(editing, empty())
  const maxPort = servers.value.reduce((m, s) => Math.max(m, s.port), 2403)
  const maxAsdu = servers.value.reduce((m, s) => Math.max(m, s.asdu_addr), 0)
  editing.port = maxPort + 1
  editing.asdu_addr = maxAsdu + 1
  editing.name = `server-${servers.value.length + 1}`
  dialogOpen.value = true
}

function openEdit(s: IEC104Server) {
  Object.assign(editing, s)
  dialogOpen.value = true
}

function parseIPs(csv: string): string[] {
  return csv.split(',').map(s => s.trim()).filter(Boolean)
}

function validate(): string | null {
  if (!Number.isInteger(editing.port) || editing.port <= 0 || editing.port > 65535) return 'Port must be 1..65535'
  if (!Number.isInteger(editing.asdu_addr) || editing.asdu_addr <= 0) return 'ASDU addr must be positive'
  const dup = servers.value.find(s => s.port === editing.port && s.id !== editing.id)
  if (dup) return `Port ${editing.port} already used by #${dup.id} (${dup.name || 'unnamed'})`
  for (const ip of parseIPs(editing.scada_ips)) {
    if (!isValidIP(ip)) return `SCADA IP invalid: "${ip}"`
  }
  return null
}

async function save() {
  const err = validate()
  if (err) { toast.error(err); return }
  saving.value = true
  try {
    if (isEdit.value) {
      await api.put(`/iec104-servers/${editing.id}`, editing)
      toast.success(`Server #${editing.id} saved · fleet reloaded`)
    } else {
      await api.post('/iec104-servers', editing)
      toast.success('Server added · fleet reloaded')
    }
    dialogOpen.value = false
    await reload()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Save failed')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(s: IEC104Server) {
  try {
    await api.put(`/iec104-servers/${s.id}`, { ...s, enabled: !s.enabled })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function del(s: IEC104Server) {
  const ok = await confirm({
    title: t.iec104.deleteTitle,
    message: t.iec104.deleteMessage,
    detail: `${s.name || 'unnamed'} · port ${s.port} · ASDU ${s.asdu_addr}`,
    variant: 'danger',
    confirmText: t.iec104.deleteConfirm,
    challenge: s.name || `server-${s.id}`,
  })
  if (!ok) return
  try {
    await api.delete(`/iec104-servers/${s.id}`)
    await reload()
    toast.success(t.iec104.deleted)
  } catch (e: any) { toast.error(e?.response?.data?.error ?? t.common.error) }
}

// ── IOA Mapping section (Menu 3) ──────────────────────────────────────────────
// Provides a compact view: pick a gateway_signal → assign IOA + IEC-104 type.
// The full signal mapper (/mappings) is still available for power users.

const mappings = ref<SignalMapping[]>([])
const gatewaySignals = ref<GatewaySignal[]>([])
const topics = ref<Topic[]>([])
const devices = ref<Device[]>([])

const IEC_TYPES = [
  'M_ME_TF_1', 'M_ME_NC_1', 'M_ME_NA_1', 'M_ME_NB_1',
  'M_SP_NA_1', 'M_SP_TB_1', 'M_DP_NA_1', 'M_IT_NA_1', 'M_IT_TB_1',
]

const mapDialogOpen = ref(false)

interface MapDraft {
  id: number
  server_id: number
  topic_id: number
  signal_id: number | null
  json_key: string
  metric_name: string
  iec104_type: string
  ioa: number
  scale: number
  unit: string
  enabled: boolean
  device_name: string
  variable_type: string
  characteristic: string
  quality_key: string
  business: string
  company: string
  deadband_abs: number
  deadband_pct: number
}
const editingMap = reactive<MapDraft>({
  id: 0, server_id: 0, topic_id: 0, signal_id: null,
  json_key: '', metric_name: '', iec104_type: 'M_ME_TF_1', ioa: 0,
  scale: 1.0, unit: '', enabled: true, device_name: '',
  variable_type: '', characteristic: '', quality_key: '',
  business: '', company: '', deadband_abs: 0, deadband_pct: 0,
})
const isEditMap = computed(() => editingMap.id > 0)

// unassigned gateway_signals: those not yet mapped on the selected server
const unassignedSignals = computed(() => {
  const mappedSigIDs = new Set(
    mappings.value
      .filter(m => m.server_id === editingMap.server_id && m.signal_id)
      .map(m => m.signal_id)
  )
  return gatewaySignals.value.filter(s => !mappedSigIDs.has(s.id) || s.id === editingMap.signal_id)
})

function suggestIOA(serverId: number, currentID = 0) {
  const used = mappings.value
    .filter(m => m.server_id === serverId && m.id !== currentID)
    .map(m => m.ioa)
  return (used.reduce((m, x) => Math.max(m, x), 16384)) + 1
}

function openEditMap(m: SignalMapping) {
  sigSearch.value = ''
  Object.assign(editingMap, {
    id: m.id,
    server_id: m.server_id,
    topic_id: m.topic_id,
    signal_id: m.signal_id ?? null,
    json_key: m.json_key,
    metric_name: m.metric_name,
    iec104_type: m.iec104_type,
    ioa: m.ioa,
    scale: m.scale,
    unit: m.unit,
    enabled: m.enabled,
    device_name: m.device_name,
    variable_type: m.variable_type,
    characteristic: m.characteristic,
    quality_key: m.quality_key,
    business: m.business,
    company: m.company,
    deadband_abs: m.deadband_abs,
    deadband_pct: m.deadband_pct,
  })
  mapDialogOpen.value = true
}

function openCreateMap(serverId: number) {
  sigSearch.value = ''
  Object.assign(editingMap, {
    id: 0, server_id: serverId, topic_id: 0, signal_id: null,
    json_key: '', metric_name: '', iec104_type: 'M_ME_TF_1',
    ioa: suggestIOA(serverId), scale: 1.0, unit: '', enabled: true,
    device_name: '', variable_type: '', characteristic: '',
    quality_key: '', business: '', company: '', deadband_abs: 0, deadband_pct: 0,
  })
  mapDialogOpen.value = true
}

function selectSignalForMap(sig: GatewaySignal) {
  editingMap.signal_id = sig.id
  editingMap.topic_id = sig.topic_id
  if (!editingMap.json_key) editingMap.json_key = sig.json_key
  if (!editingMap.metric_name) editingMap.metric_name = sig.metric_name
  if (!editingMap.unit) editingMap.unit = sig.unit
  if (editingMap.scale === 1.0) editingMap.scale = sig.scale
  if (!editingMap.variable_type) editingMap.variable_type = sig.variable_type
}

async function saveMap() {
  if (!editingMap.server_id) { toast.error('Servidor IEC-104 requerido'); return }
  if (!editingMap.topic_id && !editingMap.signal_id) { toast.error('Señal o tópico requerido'); return }
  if (!editingMap.iec104_type) { toast.error('Tipo IEC-104 requerido'); return }
  if (!editingMap.ioa || editingMap.ioa <= 0) { toast.error('IOA debe ser positivo'); return }
  try {
    if (isEditMap.value) {
      await api.put(`/mappings/${editingMap.id}`, editingMap)
      toast.success(`Mapeo IOA ${editingMap.ioa} actualizado`)
    } else {
      await api.post('/mappings', editingMap)
      toast.success(`Mapeo IOA ${editingMap.ioa} creado`)
    }
    mapDialogOpen.value = false
    await reloadMappings()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Error')
  }
}

async function delMap(m: SignalMapping) {
  if (!await confirm({
    title: 'Eliminar mapeo',
    message: `Elimina el punto IOA ${m.ioa} del servidor. Los datos en caché persisten hasta reinicio.`,
    detail: `IOA ${m.ioa} · ${m.iec104_type}`,
    variant: 'danger',
    confirmText: 'Eliminar',
  })) return
  try {
    await api.delete(`/mappings/${m.id}`)
    await reloadMappings()
    toast.success('Eliminado')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

function mappingsForServer(serverId: number) {
  return mappings.value.filter(m => m.server_id === serverId).sort((a, b) => a.ioa - b.ioa)
}

async function reloadMappings() {
  const [mg, gs, tp, dv] = await Promise.all([
    api.get<SignalMapping[]>('/mappings'),
    api.get<GatewaySignal[]>('/gateway-signals'),
    api.get<Topic[]>('/topics'),
    api.get<Device[]>('/devices'),
  ])
  mappings.value = mg.data ?? []
  gatewaySignals.value = gs.data ?? []
  topics.value = tp.data ?? []
  devices.value = dv.data ?? []
}

const signalById = computed(() => Object.fromEntries(gatewaySignals.value.map(s => [s.id, s])))
const topicById  = computed(() => Object.fromEntries(topics.value.map(t => [t.id, t])))

// ── Signal combobox (replaces dropdown for scalability) ──────────────────────
const sigSearch = ref('')

const filteredSignals = computed(() => {
  const q = sigSearch.value.trim().toLowerCase()
  const list = unassignedSignals.value
  if (!q) return list.slice(0, 60)
  return list.filter(s => {
    const topic = topicById.value[s.topic_id]
    return [s.name, s.json_key, s.metric_name, s.unit, s.variable_type, topic?.topic ?? '']
      .some(v => v.toLowerCase().includes(q))
  }).slice(0, 60)
})

function clearSignalMap() {
  editingMap.signal_id = null
  editingMap.topic_id  = 0
  sigSearch.value      = ''
}

onMounted(() => { loadGateway(); reload(); reloadMappings() })

function runtimeOf(id: number) {
  return status.value?.iec104.servers.find(s => s.id === id)
}

// Per-row link state — what actually matters for an industrial console.
// Green is reserved for "protocol up" (post-STARTDT exchange). A bound
// listener with no master, or a TCP socket that hasn't completed STARTDT,
// is NOT green.
//
//   off       — row disabled
//   bind-fail — enabled but listener failed to start
//   listening — listener up, no TCP client (slate-blue indication)
//   tcp-only  — TCP open but master hasn't sent STARTDT (amber indication)
//   linked    — STARTDT done, IEC-104 link active (green)
type LinkState = 'off' | 'bind-fail' | 'listening' | 'tcp-only' | 'linked'
interface LinkInfo {
  state: LinkState
  pillState: 'idle' | 'fault' | 'wait' | 'warn' | 'ok'
  label: string
  hint: string
  clients: number
  activated: number
}
function linkInfo(s: IEC104Server): LinkInfo {
  const rt = runtimeOf(s.id)
  const clients = rt?.clients ?? 0
  const activated = rt?.activated ?? 0
  if (!s.enabled) {
    return { state: 'off', pillState: 'idle', label: t.common.disabled, hint: t.status.disabled, clients, activated }
  }
  if (!rt?.running) {
    return { state: 'bind-fail', pillState: 'fault', label: t.status.bindFailed, hint: 'Listener no activo — revisar IP/puerto', clients, activated }
  }
  if (activated > 0) {
    return {
      state: 'linked',
      pillState: 'ok',
      label: activated === 1 ? `${t.status.protocolUp} · 1` : `${t.status.protocolUp} · ${activated}`,
      hint: 'STARTDT activado — tramas fluyendo',
      clients,
      activated,
    }
  }
  if (clients > 0) {
    return {
      state: 'tcp-only',
      pillState: 'warn',
      label: t.status.tcpOnly,
      hint: 'Conectado, esperando STARTDT',
      clients,
      activated,
    }
  }
  return { state: 'listening', pillState: 'wait', label: t.status.listening, hint: 'Listener activo, sin master', clients, activated }
}

const fleetState = computed<'ok' | 'warn' | 'fault' | 'wait' | 'idle'>(() => {
  const list = servers.value
  if (!list.length) return 'idle'
  const enabled = list.filter(s => s.enabled)
  if (!enabled.length) return 'idle'
  const infos = enabled.map(linkInfo)
  if (infos.some(i => i.state === 'bind-fail')) return 'fault'
  if (infos.every(i => i.state === 'linked')) return 'ok'
  if (infos.some(i => i.state === 'linked')) return 'warn'
  if (infos.some(i => i.state === 'tcp-only')) return 'warn'
  return 'wait'
})
const fleetLabel = computed(() => {
  switch (fleetState.value) {
    case 'idle': return t.status.idle
    case 'fault': return t.status.bindFailed
    case 'wait': return t.status.listening
    case 'warn': return 'Protocolo parcial'
    case 'ok': return t.status.protocolUp
  }
  return t.status.idle
})

function chips(csv: string): string[] { return parseIPs(csv) }
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8 max-w-6xl">
    <!-- Hero -->
    <section class="card-soft overflow-hidden relative">
      <div class="relative grid grid-cols-12 gap-6 p-5 sm:p-6 lg:p-8">
        <div class="col-span-12 md:col-span-8">
          <div class="flex items-center gap-3 mb-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-bosque)] text-white">
              <Server class="h-5 w-5" />
            </div>
            <span class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">
              IEC 60870-5-104 · flota pasiva
            </span>
          </div>
          <h1 class="mb-2">{{ t.iec104.servers }}</h1>
          <div class="font-mono text-sm mt-2 text-[color:var(--epm-bosque)]">
            {{ servers.length }} configurados · {{ servers.filter(s => s.enabled).length }} habilitados
          </div>
          <p class="mt-3 text-sm text-muted-foreground max-w-xl">
            El gateway es estrictamente pasivo. Los masters SCADA se conectan; el gateway nunca marca.
            Cada fila es un listener independiente (puerto + ASDU + lista de IPs SCADA) que comparte
            el mismo conjunto de puntos, todos vinculados a la IP de escucha global del gateway.
          </p>
        </div>
        <div class="col-span-12 md:col-span-4 flex flex-col gap-3 md:items-end">
          <StatusPill :state="fleetState" :label="fleetLabel" />
          <div class="chip font-mono text-xs">
            <span class="h-2 w-2 rounded-sm bg-[color:var(--epm-citrico)]" />
            {{ status?.iec104.points ?? 0 }} points cached
          </div>
          <div class="chip font-mono text-xs">
            <PlugZap class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" />
            {{ status?.iec104.activated ?? 0 }} protocol link{{ (status?.iec104.activated ?? 0) === 1 ? '' : 's' }}
            <span class="text-muted-foreground">/ {{ status?.iec104.clients ?? 0 }} TCP</span>
          </div>
        </div>
      </div>
    </section>

    <!-- Gateway-wide listen IP -->
    <Card class="card-soft">
      <CardHeader>
        <div class="flex items-center gap-2">
          <Globe class="h-5 w-5 text-[color:var(--epm-bosque)]" />
          <CardTitle class="font-extrabold tracking-tight">{{ t.iec104.gateway }}</CardTitle>
        </div>
        <CardDescription>
          {{ t.iec104.listenDesc }}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div class="flex flex-col md:flex-row md:items-end gap-4">
          <div class="space-y-1.5 flex-1 max-w-md">
            <Label>{{ t.iec104.listenIp }}</Label>
            <Input
              v-model="gateway.listen_ip"
              class="font-mono"
              placeholder="0.0.0.0"
              @input="gatewayDirty = true"
            />
            <p class="text-xs text-muted-foreground">
              Runtime value: <code class="font-mono">{{ status?.iec104.listen_ip || '—' }}</code>
            </p>
          </div>
          <Button :disabled="!gatewayDirty || savingGateway" @click="saveGateway"
                  class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
            <Save class="h-4 w-4 mr-2" />
            {{ savingGateway ? t.iec104.saving : t.iec104.save }}
          </Button>
        </div>
      </CardContent>
    </Card>

    <!-- Per-server CRUD -->
    <Card class="card-soft">
      <CardHeader class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <CardTitle class="font-extrabold tracking-tight">{{ t.iec104.servers }}</CardTitle>
          <CardDescription>
            Un listener por fila. La columna de enlace muestra la conexión en vivo entre
            este gateway y el(los) master(s) SCADA para ese puerto + ASDU.
          </CardDescription>
        </div>
        <div class="flex items-center gap-2">
          <Button
            v-if="selectedServers.size > 0"
            size="sm" variant="destructive"
            class="rounded-sm h-8 text-xs"
            @click="delSelectedServers"
          >
            <Trash2 class="h-3.5 w-3.5 mr-1" /> Eliminar ({{ selectedServers.size }})
          </Button>
          <Button variant="outline" @click="reload" class="rounded-sm">
            <RefreshCw class="h-4 w-4 mr-2" /> {{ t.common.reload }}
          </Button>
          <Dialog v-model:open="dialogOpen">
            <DialogTrigger as-child>
              <Button @click="openCreate"
                      class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-4">
                <Plus class="h-4 w-4 mr-1" /> {{ t.iec104.addServer }}
              </Button>
            </DialogTrigger>

            <!--
              Dialog content: 12-column grid, comfortable spacing, no inner
              scrollbar unless the viewport really cannot fit the form.
            -->
            <DialogContent class="!max-w-xl sm:!max-w-2xl p-0 overflow-hidden">
              <div class="px-6 pt-6 pb-2 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
                <DialogHeader class="text-left space-y-1">
                  <DialogTitle class="text-lg font-extrabold tracking-tight flex items-center gap-2">
                    <Server class="h-4 w-4 text-[color:var(--epm-bosque)]" />
                    {{ isEdit ? `Editar servidor #${editing.id}` : 'Nuevo servidor IEC-104' }}
                  </DialogTitle>
                  <DialogDescription class="text-xs">
                    Endpoint esclavo pasivo. Los masters SCADA se conectan a
                    <code class="font-mono text-[color:var(--epm-bosque)]">{{ gateway.listen_ip || '0.0.0.0' }}:{{ editing.port || 2404 }}</code>
                    y leen este gateway bajo ASDU
                    <code class="font-mono text-[color:var(--epm-bosque)]">{{ editing.asdu_addr || 1 }}</code>.
                  </DialogDescription>
                </DialogHeader>
              </div>

              <div class="px-6 py-5 space-y-5">
                <!-- Identity row -->
                <div class="grid grid-cols-12 gap-4">
                  <div class="col-span-12 sm:col-span-6 space-y-1.5">
                    <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.iec104.name }}</Label>
                    <Input v-model="editing.name" placeholder="control-center-a" />
                  </div>
                  <div class="col-span-6 sm:col-span-3 space-y-1.5">
                    <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.iec104.port }}</Label>
                    <Input v-model.number="editing.port" type="number" class="font-mono" />
                  </div>
                  <div class="col-span-6 sm:col-span-3 space-y-1.5">
                    <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.iec104.asduAddr }}</Label>
                    <Input v-model.number="editing.asdu_addr" type="number" class="font-mono" />
                  </div>
                </div>

                <!-- SCADA allowlist -->
                <div class="space-y-1.5">
                  <Label class="text-[11px] uppercase tracking-[0.18em] font-bold flex items-center gap-1.5">
                    <ShieldCheck class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" />
                    {{ t.iec104.scadaIps }}
                  </Label>
                  <Input v-model="editing.scada_ips" class="font-mono" placeholder="10.13.13.25, 10.117.18.23" />
                  <p class="text-xs text-muted-foreground leading-relaxed">
                    {{ t.iec104.scadaIpsDesc }}
                  </p>
                </div>

                <!-- Protocol timers -->
                <div class="space-y-2">
                  <div class="flex items-center gap-2">
                    <div class="rule-brand flex-1" />
                    <span class="text-[10px] uppercase tracking-[0.22em] font-bold text-muted-foreground">
                      {{ t.iec104.timing }}
                    </span>
                    <div class="rule-brand flex-1" />
                  </div>
                  <div class="grid grid-cols-6 gap-3">
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold" :title="t.iec104.kDesc">k</Label>
                      <Input v-model.number="editing.k" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">ventana env.</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold" :title="t.iec104.wDesc">w</Label>
                      <Input v-model.number="editing.w" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">umbral ack</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold" :title="t.iec104.t0Desc">t0</Label>
                      <Input v-model.number="editing.t0" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">conexión</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold" :title="t.iec104.t1Desc">t1</Label>
                      <Input v-model.number="editing.t1" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">envío</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold" :title="t.iec104.t2Desc">t2</Label>
                      <Input v-model.number="editing.t2" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">confirmación</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold" :title="t.iec104.t3Desc">t3</Label>
                      <Input v-model.number="editing.t3" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">prueba</p>
                    </div>
                  </div>
                </div>

                <!-- Enable -->
                <div class="flex items-center gap-3 pt-1">
                  <Switch id="en" v-model="editing.enabled" />
                  <Label for="en" class="cursor-pointer">
                    {{ t.iec104.enabled }} — iniciar listener al guardar
                  </Label>
                </div>
              </div>

              <DialogFooter class="px-6 py-4 border-t border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_5%,transparent)]">
                <Button variant="outline" @click="dialogOpen = false" class="rounded-sm">{{ t.common.cancel }}</Button>
                <Button :disabled="saving" @click="save"
                        class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
                  <Save class="h-4 w-4 mr-2" />
                  {{ saving ? t.iec104.saving : (isEdit ? t.common.save : 'Crear') }}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </CardHeader>
      <CardContent>
        <div class="overflow-x-auto -mx-6 px-6">
        <Table>
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="w-10 pl-4">
                <input
                  type="checkbox"
                  class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                  :checked="allServersSelected"
                  :indeterminate="selectedServers.size > 0 && !allServersSelected"
                  @change="toggleSelectAllServers"
                />
              </TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.iec104.name }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.iec104.port }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.iec104.asduAddr }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.iec104.scadaIps }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">
                <span class="inline-flex items-center gap-1"><Link2 class="h-3 w-3" /> Enlace</span>
              </TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.iec104.enabled }}</TableHead>
              <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.2em] font-bold">Acciones</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="s in servers" :key="s.id"
              class="data-row border-b border-border/60"
              :class="selectedServers.has(s.id) ? 'bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]' : ''"
            >
              <TableCell class="pl-4">
                <input
                  type="checkbox"
                  class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                  :checked="selectedServers.has(s.id)"
                  @change="toggleSelectServer(s.id)"
                />
              </TableCell>
              <TableCell class="font-semibold">{{ s.name || '—' }}</TableCell>
              <TableCell class="font-mono text-xs">{{ s.port }}</TableCell>
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ s.asdu_addr }}</TableCell>
              <TableCell>
                <div v-if="chips(s.scada_ips).length" class="flex flex-wrap gap-1">
                  <span v-for="ip in chips(s.scada_ips)" :key="ip"
                        class="chip font-mono text-[11px]">{{ ip }}</span>
                </div>
                <span v-else class="inline-flex items-center gap-1 text-xs text-[color:var(--destructive)] font-semibold">
                  <ShieldAlert class="h-3.5 w-3.5" /> vacío — bloquea todo
                </span>
              </TableCell>
              <TableCell>
                <div class="flex flex-col items-start gap-0.5">
                  <StatusPill
                    :state="linkInfo(s).pillState"
                    :label="linkInfo(s).label"
                    :pulse="linkInfo(s).state === 'linked'"
                  />
                  <span class="text-[10px] text-muted-foreground tracking-wide">{{ linkInfo(s).hint }}</span>
                </div>
              </TableCell>
              <TableCell>
                <Switch :model-value="s.enabled" @update:model-value="() => toggleEnabled(s)" />
              </TableCell>
              <TableCell class="text-right">
                <Button variant="ghost" size="icon" @click="openEdit(s)"><Pencil class="h-4 w-4" /></Button>
                <Button variant="ghost" size="icon" @click="del(s)" class="text-[color:var(--destructive)]"><Trash2 class="h-4 w-4" /></Button>
              </TableCell>
            </TableRow>
            <TableRow v-if="!servers.length">
              <TableCell colspan="8" class="text-center text-muted-foreground py-8">
                Sin servidores. Haga clic en "{{ t.iec104.addServer }}" para agregar uno.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        </div>
      </CardContent>
    </Card>

    <!-- ── IOA Mapping table (per server) ─────────────────────────────────── -->
    <div v-if="servers.length" class="space-y-4">
      <div class="flex items-center gap-3 pb-2 border-b border-border">
        <Layers class="h-5 w-5 text-[color:var(--epm-bosque)]" />
        <div>
          <h2 class="font-bold text-lg">Mapeo IOA → Señal</h2>
          <p class="text-xs text-muted-foreground">Asigna señales SSFV a Information Object Addresses en cada servidor.</p>
        </div>
      </div>

      <div v-for="srv in servers" :key="`map:${srv.id}`" class="card-soft overflow-hidden">
        <div class="flex items-center gap-3 px-4 py-3 border-b border-border bg-muted/20">
          <Server class="h-4 w-4 text-[color:var(--epm-bosque)]" />
          <span class="font-bold text-sm flex-1">{{ srv.name || `Server #${srv.id}` }}</span>
          <span class="font-mono text-xs text-muted-foreground">:{{ srv.port }} · ASDU {{ srv.asdu_addr }}</span>
          <Button
            v-if="selectedMappings.size > 0 && mappingsForServer(srv.id).some(m => selectedMappings.has(m.id))"
            size="sm" variant="destructive"
            class="rounded-sm text-xs h-7"
            @click="delSelectedMappings"
          >
            <Trash2 class="h-3.5 w-3.5 mr-1" />
            Eliminar ({{ mappingsForServer(srv.id).filter(m => selectedMappings.has(m.id)).length }})
          </Button>
          <Button size="sm" variant="outline" class="rounded-sm text-xs h-7" @click="openCreateMap(srv.id)">
            <Plus class="h-3.5 w-3.5 mr-1" /> Agregar mapeo
          </Button>
        </div>

        <div class="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
                <TableHead class="w-10 pl-3">
                  <input
                    type="checkbox"
                    class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                    :checked="mappingsAllSelectedForServer(srv.id)"
                    :indeterminate="mappingsForServer(srv.id).some(m => selectedMappings.has(m.id)) && !mappingsAllSelectedForServer(srv.id)"
                    @change="toggleSelectAllMappingsForServer(srv.id)"
                  />
                </TableHead>
                <TableHead class="w-16 text-[10px] uppercase tracking-[0.18em] font-bold">IOA</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">Señal</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">Clave / Métrica</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">IEC-104 tipo</TableHead>
                <TableHead class="text-right text-[10px] uppercase tracking-[0.18em] font-bold">Escala</TableHead>
                <TableHead class="w-14 text-[10px] uppercase tracking-[0.18em] font-bold">On</TableHead>
                <TableHead class="w-16 text-right text-[10px] uppercase tracking-[0.18em] font-bold">Acc.</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="!mappingsForServer(srv.id).length">
                <TableCell colspan="8" class="text-center text-muted-foreground py-4 text-xs italic">
                  Sin mapeos en este servidor. Haz clic en "Agregar mapeo".
                </TableCell>
              </TableRow>
              <TableRow
                v-for="m in mappingsForServer(srv.id)"
                :key="m.id"
                class="data-row border-b border-border/40 last:border-b-0"
                :class="selectedMappings.has(m.id) ? 'bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]' : ''"
              >
                <TableCell class="pl-3">
                  <input
                    type="checkbox"
                    class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                    :checked="selectedMappings.has(m.id)"
                    @change="toggleSelectMapping(m.id)"
                  />
                </TableCell>
                <TableCell class="font-mono font-bold text-[color:var(--epm-bosque)]">{{ m.ioa }}</TableCell>
                <TableCell>
                  <div class="text-xs font-medium">
                    {{ m.signal_id && signalById[m.signal_id] ? signalById[m.signal_id].name : m.device_name || '—' }}
                  </div>
                  <div v-if="m.signal_id" class="flex items-center gap-1 mt-0.5">
                    <Database class="h-2.5 w-2.5 text-muted-foreground" />
                    <span class="text-[10px] text-muted-foreground">
                      {{ signalById[m.signal_id]?.persist_to_db ? 'persistida' : 'solo reenvío' }}
                    </span>
                  </div>
                </TableCell>
                <TableCell class="font-mono text-xs">
                  <template v-if="m.metric_name">
                    <span class="text-blue-600 dark:text-blue-400">{{ m.metric_name }}</span>
                    <span class="text-[10px] text-muted-foreground ml-1">spB</span>
                  </template>
                  <template v-else>{{ m.json_key }}</template>
                </TableCell>
                <TableCell>
                  <span class="inline-flex items-center rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)]">
                    {{ m.iec104_type }}
                  </span>
                </TableCell>
                <TableCell class="text-right font-mono text-xs tabular-nums">{{ m.scale }}</TableCell>
                <TableCell>
                  <Switch :model-value="m.enabled" @update:model-value="() => api.put(`/mappings/${m.id}`, {...m, enabled: !m.enabled}).then(reloadMappings)" />
                </TableCell>
                <TableCell class="text-right whitespace-nowrap">
                  <Button variant="ghost" size="icon" @click="openEditMap(m)">
                    <Pencil class="h-4 w-4" />
                  </Button>
                  <Button variant="ghost" size="icon" @click="delMap(m)" class="text-[color:var(--destructive)]">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </div>
    </div>

    <!-- Mapping dialog -->
    <Dialog v-model:open="mapDialogOpen">
      <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
        <div class="px-6 pt-6 pb-4 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
          <DialogHeader class="text-left space-y-1">
            <DialogTitle class="text-lg font-extrabold tracking-tight flex items-center gap-2">
              <Layers class="h-4 w-4 text-[color:var(--epm-bosque)]" />
              {{ isEditMap ? `Editar mapeo IOA ${editingMap.ioa}` : 'Nuevo mapeo IOA' }}
            </DialogTitle>
            <DialogDescription class="text-xs">
              Selecciona una señal SSFV (Paso 2) y asígnale un Information Object Address en el servidor IEC-104.
            </DialogDescription>
          </DialogHeader>
        </div>

        <div class="px-6 py-5 grid grid-cols-6 gap-x-4 gap-y-3">
          <!-- Signal combobox — search-driven, scales to any number of signals -->
          <div class="col-span-6 space-y-1.5">
            <Label>Señal SSFV <span class="text-muted-foreground font-normal text-[10px]">(del Paso 2)</span></Label>

            <!-- Selected chip -->
            <div
              v-if="editingMap.signal_id && signalById[editingMap.signal_id]"
              class="flex items-center gap-2 rounded-sm border border-[color:var(--epm-bosque)] bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)] px-3 py-2"
            >
              <Layers class="h-3.5 w-3.5 text-[color:var(--epm-bosque)] shrink-0" />
              <div class="flex-1 min-w-0">
                <span class="font-semibold text-sm">{{ signalById[editingMap.signal_id].name }}</span>
                <span class="ml-2 font-mono text-[10px] text-muted-foreground">
                  {{ signalById[editingMap.signal_id].json_key || signalById[editingMap.signal_id].metric_name }}
                </span>
              </div>
              <span v-if="signalById[editingMap.signal_id].persist_to_db"
                    class="text-[9px] font-bold uppercase tracking-wider text-[color:var(--epm-bosque)] shrink-0">
                TSDB
              </span>
              <button type="button" class="text-muted-foreground hover:text-foreground shrink-0" @click="clearSignalMap">
                <X class="h-3.5 w-3.5" />
              </button>
            </div>

            <!-- Search + list (shown when no signal selected) -->
            <div v-else class="space-y-1">
              <div class="relative">
                <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
                <Input
                  v-model="sigSearch"
                  placeholder="Buscar señal por nombre, clave, tópico…"
                  class="pl-8 font-mono text-xs rounded-sm"
                />
              </div>
              <div class="border border-border rounded-sm max-h-44 overflow-y-auto divide-y divide-border/40 bg-card">
                <div v-if="!filteredSignals.length" class="px-3 py-4 text-xs text-muted-foreground text-center italic">
                  {{ unassignedSignals.length === 0
                    ? 'Sin señales disponibles — crea señales en el Paso 2.'
                    : 'Sin coincidencias para "' + sigSearch + '".' }}
                </div>
                <button
                  v-for="sg in filteredSignals"
                  :key="sg.id"
                  type="button"
                  class="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-muted/60 transition-colors"
                  @click="selectSignalForMap(sg)"
                >
                  <div class="flex-1 min-w-0">
                    <div class="font-semibold text-xs truncate">{{ sg.name }}</div>
                    <div class="font-mono text-[10px] text-muted-foreground truncate">
                      {{ sg.json_key || sg.metric_name }}
                      <span class="ml-1 text-[color:var(--epm-bosque)]/60">
                        · {{ topicById[sg.topic_id]?.topic.split('/').slice(-2).join('/') ?? `#${sg.topic_id}` }}
                      </span>
                    </div>
                  </div>
                  <span v-if="sg.persist_to_db"
                        class="shrink-0 text-[9px] font-bold uppercase tracking-wider text-[color:var(--epm-bosque)]">
                    TSDB
                  </span>
                </button>
              </div>
              <p class="text-[10px] text-muted-foreground">
                {{ filteredSignals.length }} de {{ unassignedSignals.length }} señal{{ unassignedSignals.length === 1 ? '' : 'es' }} disponible{{ unassignedSignals.length === 1 ? '' : 's' }}
              </p>
            </div>
          </div>

          <!-- IEC-104 type + IOA -->
          <div class="col-span-4 space-y-1.5">
            <Label>Tipo IEC-104</Label>
            <Select v-model="editingMap.iec104_type">
              <SelectTrigger class="w-full font-mono text-xs"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="tp in IEC_TYPES" :key="tp" :value="tp" class="font-mono text-xs">{{ tp }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="col-span-2 space-y-1.5">
            <Label>IOA</Label>
            <Input v-model.number="editingMap.ioa" type="number" class="font-mono" />
          </div>

          <!-- Scale + unit -->
          <div class="col-span-3 space-y-1.5">
            <Label>Escala</Label>
            <Input v-model.number="editingMap.scale" type="number" step="0.001" />
          </div>
          <div class="col-span-3 space-y-1.5">
            <Label>Unidad</Label>
            <Input v-model="editingMap.unit" placeholder="kW, A…" />
          </div>

          <!-- Enabled -->
          <div class="col-span-6 flex items-center gap-3">
            <Switch id="mapenabled" v-model="editingMap.enabled" />
            <Label for="mapenabled">Habilitado</Label>
          </div>
        </div>

        <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
          <Button variant="outline" @click="mapDialogOpen = false" class="rounded-sm">Cancelar</Button>
          <Button @click="saveMap" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
            {{ isEditMap ? 'Guardar' : 'Crear mapeo' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
