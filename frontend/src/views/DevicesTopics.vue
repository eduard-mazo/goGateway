<script setup lang="ts">
import { ref, onMounted, computed, reactive, watch } from 'vue'
import { toast } from 'vue-sonner'
import { api, type Device, type Topic, type IEC104Server, type MQTTConfig } from '@/api'
import { t as i18n } from '@/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  Trash2, Plus, Folder, FolderOpen, ChevronRight, Search, Pencil, Radio,
  Server as ServerIcon, ChevronsDownUp, ChevronsUpDown, Save, Wifi, Zap,
} from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

// ── Multi-select: topics ──────────────────────────────────────────────────────
const selectedTopics = reactive(new Set<number>())
function topicsAllSelectedForDevice(deviceId: number) {
  const rows = topicsByDevice.value.get(deviceId) ?? []
  return rows.length > 0 && rows.every(t => selectedTopics.has(t.id))
}
function toggleSelectAllTopicsForDevice(deviceId: number) {
  const rows = topicsByDevice.value.get(deviceId) ?? []
  if (topicsAllSelectedForDevice(deviceId)) rows.forEach(t => selectedTopics.delete(t.id))
  else rows.forEach(t => selectedTopics.add(t.id))
}
function toggleSelectTopic(id: number) {
  if (selectedTopics.has(id)) selectedTopics.delete(id)
  else selectedTopics.add(id)
}
async function delSelectedTopics() {
  const ids = [...selectedTopics]
  const ok = await confirm({
    title: 'Eliminar tópicos',
    message: `Elimina ${ids.length} tópico${ids.length === 1 ? '' : 's'} y todos los mapeos de señales bajo ellos.`,
    variant: 'danger',
    confirmText: `Eliminar ${ids.length}`,
  })
  if (!ok) return
  try {
    await Promise.all(ids.map(id => api.delete(`/topics/${id}`)))
    ids.forEach(id => selectedTopics.delete(id))
    await reload()
    toast.success(`${ids.length} tópico${ids.length === 1 ? '' : 's'} eliminado${ids.length === 1 ? '' : 's'}`)
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

const devices = ref<Device[]>([])
const topics = ref<Topic[]>([])
const servers = ref<IEC104Server[]>([])
const loading = ref(false)

// ── MQTT broker config (Menu 1: broker section) ───────────────────────────────
const mqttCfg = ref<MQTTConfig>({
  id: 1, host: 'localhost', port: 1883, username: '', password: '',
  client_id: 'goGateway', use_tls: false,
  sparkplug_enabled: false, sp_group_id: 'goGateway', sp_host_id: 'goGateway-host',
  sp_topics: '',
})
const savingBroker = ref(false)
const brokerOpen = ref(true)

// Sparkplug B group IDs — managed as a list; serialised as comma-separated
// string in sp_group_id before saving.
const spGroups = ref<string[]>([])
const spGroupNew = ref('')
const spGroupSearch = ref('')
const spGroupSelected = reactive(new Set<number>())

const filteredSpGroups = computed(() => {
  const q = spGroupSearch.value.trim().toLowerCase()
  if (!q) return spGroups.value.map((g, i) => ({ g, i }))
  return spGroups.value.map((g, i) => ({ g, i })).filter(({ g }) => g.toLowerCase().includes(q))
})

function syncGroupsFromCfg() {
  spGroups.value = (mqttCfg.value.sp_group_id ?? '')
    .split(/[\n,]+/).map((s: string) => s.trim()).filter(Boolean)
  spGroupSelected.clear()
}

function addSpGroupFromInput() {
  const entries = spGroupNew.value
    .split(/[\n,]+/)
    .map((s: string) => s.trim())
    .filter((s: string) => s && !spGroups.value.includes(s))
  if (entries.length) spGroups.value.push(...entries)
  spGroupNew.value = ''
}

function removeSpGroup(originalIdx: number) {
  spGroups.value.splice(originalIdx, 1)
  const updated = new Set<number>()
  spGroupSelected.forEach((i: number) => {
    if (i < originalIdx) updated.add(i)
    else if (i > originalIdx) updated.add(i - 1)
  })
  spGroupSelected.clear()
  updated.forEach((i: number) => spGroupSelected.add(i))
}

function deleteSelectedSpGroups() {
  ;[...spGroupSelected].sort((a, b) => b - a).forEach(i => spGroups.value.splice(i, 1))
  spGroupSelected.clear()
}

function toggleSelectSpGroup(i: number) {
  if (spGroupSelected.has(i)) spGroupSelected.delete(i)
  else spGroupSelected.add(i)
}

function toggleSelectAllSpGroups() {
  const visible = filteredSpGroups.value.map(({ i }) => i)
  const allSelected = visible.length > 0 && visible.every(i => spGroupSelected.has(i))
  if (allSelected) visible.forEach(i => spGroupSelected.delete(i))
  else visible.forEach(i => spGroupSelected.add(i))
}

function onSpGroupNewKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') { e.preventDefault(); addSpGroupFromInput() }
}

async function loadBroker() {
  try {
    const { data } = await api.get<MQTTConfig>('/mqtt-config')
    Object.assign(mqttCfg.value, data)
    syncGroupsFromCfg()
  } catch {}
}

async function saveBroker() {
  savingBroker.value = true
  // Flush chip array back into the config string before saving.
  mqttCfg.value.sp_group_id = spGroups.value.join(',')
  try {
    const { data } = await api.put<MQTTConfig>('/mqtt-config', mqttCfg.value)
    Object.assign(mqttCfg.value, data)
    syncGroupsFromCfg()
    toast.success('Broker guardado · reconectando')
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Error')
  } finally {
    savingBroker.value = false
  }
}
const search = ref('')

const serverById = computed(() => Object.fromEntries(servers.value.map(s => [s.id, s])))
const devById = computed(() => Object.fromEntries(devices.value.map(d => [d.id, d])))
const topicsByDevice = computed(() => {
  const m = new Map<number, Topic[]>()
  for (const t of topics.value) {
    const arr = m.get(t.device_id) ?? []
    arr.push(t)
    m.set(t.device_id, arr)
  }
  for (const arr of m.values()) arr.sort((a, b) => a.topic.localeCompare(b.topic))
  return m
})
const devicesByServer = computed(() => {
  const m = new Map<number, Device[]>()
  for (const d of devices.value) {
    const arr = m.get(d.server_id) ?? []
    arr.push(d)
    m.set(d.server_id, arr)
  }
  for (const arr of m.values()) arr.sort((a, b) => a.name.localeCompare(b.name))
  return m
})

interface DeviceNode {
  device: Device
  topics: Topic[]
}
interface ServerNode {
  server: IEC104Server | null
  devices: DeviceNode[]
  topicCount: number
}

const tree = computed<ServerNode[]>(() => {
  const q = search.value.trim().toLowerCase()
  const matchDev = (d: Device) =>
    [d.name, d.description].some(s => (s ?? '').toLowerCase().includes(q))
  const matchTopic = (t: Topic) => t.topic.toLowerCase().includes(q)

  const buildSrv = (s: IEC104Server | null, devs: Device[]): ServerNode => {
    const srvHit = !q || (s && [s.name, String(s.port)].some(x => (x ?? '').toLowerCase().includes(q)))
    const dnodes: DeviceNode[] = []
    let topicCount = 0
    for (const d of devs) {
      const tlist = topicsByDevice.value.get(d.id) ?? []
      let tFiltered: Topic[]
      if (!q || srvHit || matchDev(d)) {
        tFiltered = tlist
      } else {
        tFiltered = tlist.filter(matchTopic)
        if (!tFiltered.length) continue
      }
      dnodes.push({ device: d, topics: tFiltered })
      topicCount += tFiltered.length
    }
    return { server: s, devices: dnodes, topicCount }
  }

  const out: ServerNode[] = servers.value.map(s =>
    buildSrv(s, devicesByServer.value.get(s.id) ?? []),
  )
  // Orphan bucket: devices whose server row was deleted (cascade should have
  // removed them, but show a node defensively if it ever happens).
  const orphanIDs = new Set<number>()
  for (const d of devices.value) {
    if (!serverById.value[d.server_id]) orphanIDs.add(d.server_id)
  }
  for (const sid of orphanIDs) {
    out.push(buildSrv(null, devicesByServer.value.get(sid) ?? []))
  }
  return q ? out.filter(n => n.devices.length > 0) : out
})

const expanded = ref<Record<string, boolean>>({})
function isOpen(key: string) {
  return expanded.value[key] === true
}
function toggle(key: string) {
  expanded.value[key] = !isOpen(key)
}

function expandAll() {
  const next: Record<string, boolean> = {}
  for (const sn of tree.value) {
    const sid = sn.server?.id ?? -999
    next[`s:${sid}`] = true
    for (const dn of sn.devices) next[`s:${sid}:d:${dn.device.id}`] = true
  }
  expanded.value = next
}
function collapseAll() { expanded.value = {} }

watch(search, q => {
  if (!q.trim()) return
  for (const sn of tree.value) {
    const sid = sn.server?.id ?? -999
    expanded.value[`s:${sid}`] = true
    for (const dn of sn.devices) expanded.value[`s:${sid}:d:${dn.device.id}`] = true
  }
})

// --- Device dialog ---------------------------------------------------------

interface DeviceDraft { id: number; server_id: number; name: string; description: string }
const devDialog = ref(false)
const editingDev = reactive<DeviceDraft>({ id: 0, server_id: 0, name: '', description: '' })
const isEditDev = computed(() => editingDev.id > 0)

function openCreateDevice(serverId: number) {
  Object.assign(editingDev, { id: 0, server_id: serverId, name: '', description: '' })
  devDialog.value = true
}
function openEditDevice(d: Device) {
  Object.assign(editingDev, { id: d.id, server_id: d.server_id, name: d.name, description: d.description })
  devDialog.value = true
}

async function saveDevice() {
  if (!editingDev.server_id) { toast.error('Server is required'); return }
  const name = editingDev.name.trim()
  if (!name) { toast.error('Name is required'); return }
  try {
    if (isEditDev.value) {
      await api.put(`/devices/${editingDev.id}`, editingDev)
      toast.success(`Device "${name}" updated`)
    } else {
      await api.post('/devices', editingDev)
      toast.success(`Device "${name}" added`)
    }
    devDialog.value = false
    await reload()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Save failed')
  }
}

async function deleteDevice(d: Device) {
  const topicCount = (topicsByDevice.value.get(d.id) ?? []).length
  const ok = await confirm({
    title: 'Eliminar dispositivo',
    message: `Cascada a ${topicCount} tópico${topicCount === 1 ? '' : 's'} y todos los mapeos bajo ellos. No se puede deshacer.`,
    detail: `${d.name}${d.description ? ' — ' + d.description : ''}`,
    variant: 'danger',
    confirmText: 'Eliminar dispositivo',
    challenge: topicCount > 0 ? d.name : undefined,
  })
  if (!ok) return
  try {
    await api.delete(`/devices/${d.id}`)
    toast.success('Deleted')
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

// --- Topic dialog ----------------------------------------------------------

interface TopicDraft { id: number; device_id: number; topic: string; qos: number; enabled: boolean; payload_format: string }
const topicDialog = ref(false)
const editingTopic = reactive<TopicDraft>({ id: 0, device_id: 0, topic: '', qos: 0, enabled: true, payload_format: 'json' })
const isEditTopic = computed(() => editingTopic.id > 0)

function openCreateTopic(deviceId: number) {
  Object.assign(editingTopic, { id: 0, device_id: deviceId, topic: '', qos: 0, enabled: true, payload_format: 'json' })
  topicDialog.value = true
}
function openEditTopic(t: Topic) {
  Object.assign(editingTopic, { ...t, payload_format: t.payload_format || 'json' })
  topicDialog.value = true
}

async function saveTopic() {
  const topic = editingTopic.topic.trim()
  if (!editingTopic.device_id) { toast.error('Device is required'); return }
  if (!topic) { toast.error('Topic string is required'); return }
  if (!Number.isInteger(editingTopic.qos) || editingTopic.qos < 0 || editingTopic.qos > 2) {
    toast.error('QoS must be 0, 1 or 2'); return
  }
  try {
    if (isEditTopic.value) {
      await api.put(`/topics/${editingTopic.id}`, editingTopic)
      toast.success(`Topic "${topic}" updated`)
    } else {
      await api.post('/topics', editingTopic)
      toast.success(`Topic "${topic}" added`)
    }
    topicDialog.value = false
    await reload()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Save failed')
  }
}

async function toggleTopic(t: Topic) {
  try {
    await api.put(`/topics/${t.id}`, { ...t, enabled: !t.enabled })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function deleteTopic(t: Topic) {
  const ok = await confirm({
    title: 'Eliminar tópico',
    message: 'Todos los mapeos de señales bajo este tópico se eliminan. No se puede deshacer.',
    detail: `${t.topic} · QoS ${t.qos}`,
    variant: 'danger',
    confirmText: 'Eliminar tópico',
  })
  if (!ok) return
  try {
    await api.delete(`/topics/${t.id}`)
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

// --- Lifecycle -------------------------------------------------------------

async function reload() {
  loading.value = true
  try {
    const [d, t, s] = await Promise.all([
      api.get<Device[]>('/devices'),
      api.get<Topic[]>('/topics'),
      api.get<IEC104Server[]>('/iec104-servers'),
    ])
    devices.value = d.data ?? []
    topics.value = t.data ?? []
    servers.value = s.data ?? []
  } catch (e: any) { toast.error('Load: ' + (e?.message ?? e)) }
  finally { loading.value = false }
}

onMounted(async () => {
  await Promise.all([reload(), loadBroker()])
})
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 max-w-5xl">
    <!-- Hero -->
    <div class="card-soft overflow-hidden relative">
      <div class="relative flex items-start gap-4 p-5 sm:p-6 lg:p-8">
        <div class="grid place-items-center w-11 h-11 rounded-sm bg-[color:var(--epm-bosque)] text-white shrink-0">
          <Radio class="h-5 w-5" />
        </div>
        <div class="min-w-0">
          <div class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">Paso 1 de 4</div>
          <h1 class="mt-1 mb-1">Tópicos y Suscripciones</h1>
          <p class="text-sm text-muted-foreground">
            Configura el broker MQTT, suscribe tópicos y elige el formato de payload (JSON / Sparkplug B).
          </p>
        </div>
      </div>
    </div>

    <!-- ── Broker Config section ─────────────────────────────────────────────── -->
    <div class="card-soft overflow-hidden">
      <button
        type="button"
        class="w-full flex items-center gap-3 px-5 py-3 hover:bg-muted/40 transition-colors text-left"
        @click="brokerOpen = !brokerOpen"
      >
        <ChevronRight class="h-4 w-4 shrink-0 text-muted-foreground transition-transform" :class="brokerOpen ? 'rotate-90' : ''" />
        <Wifi class="h-4 w-4 text-[color:var(--epm-bosque)]" />
        <span class="font-bold text-sm flex-1">Broker MQTT</span>
        <span class="font-mono text-xs text-muted-foreground">{{ mqttCfg.host }}:{{ mqttCfg.port }}</span>
      </button>
      <div v-show="brokerOpen" class="border-t border-border bg-muted/20 p-5 grid grid-cols-6 gap-x-4 gap-y-3">
        <div class="col-span-4 space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Host</Label>
          <Input v-model="mqttCfg.host" placeholder="localhost" class="font-mono rounded-sm" />
        </div>
        <div class="col-span-2 space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Puerto</Label>
          <Input v-model.number="mqttCfg.port" type="number" class="font-mono rounded-sm" />
        </div>
        <div class="col-span-3 space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Usuario</Label>
          <Input v-model="mqttCfg.username" placeholder="(vacío = anónimo)" class="rounded-sm" />
        </div>
        <div class="col-span-3 space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Contraseña</Label>
          <Input v-model="mqttCfg.password" type="password" placeholder="••••••" class="rounded-sm" />
        </div>
        <div class="col-span-4 space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Client ID</Label>
          <Input v-model="mqttCfg.client_id" class="font-mono rounded-sm" />
        </div>
        <div class="col-span-2 flex items-end gap-3 pb-0.5">
          <Switch id="tls" v-model="mqttCfg.use_tls" />
          <Label for="tls" class="text-sm">TLS</Label>
        </div>

        <!-- ── Sparkplug B ────────────────────────────────────────────────── -->
        <div class="col-span-6 border-t border-border/60 pt-3 mt-1 space-y-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <Zap class="h-4 w-4 text-[color:var(--epm-bosque)]" />
              <span class="font-bold text-sm">Sparkplug B</span>
              <span class="text-[10px] px-1.5 py-0.5 rounded-sm bg-blue-500/15 text-blue-400 font-semibold">spBv1.0</span>
            </div>
            <div class="flex items-center gap-2">
              <Label for="sp-en" class="text-xs text-muted-foreground">Habilitado</Label>
              <Switch id="sp-en" v-model="mqttCfg.sparkplug_enabled" />
            </div>
          </div>

          <template v-if="mqttCfg.sparkplug_enabled">
            <div class="col-span-6 space-y-1.5">
              <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Host ID</Label>
              <Input v-model="mqttCfg.sp_host_id" placeholder="goGateway-host" class="font-mono rounded-sm" />
              <p class="text-[11px] text-muted-foreground">
                Identificador del Primary Application. Se publica en <code class="font-mono bg-muted px-1 rounded">STATE/{host_id}</code>.
              </p>
            </div>

            <!-- Group IDs table -->
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">
                  Grupos Sparkplug B
                  <span class="ml-1.5 normal-case font-normal tracking-normal text-muted-foreground text-[11px]">
                    {{ spGroups.length }} grupo{{ spGroups.length === 1 ? '' : 's' }} · suscribe <code class="font-mono bg-muted px-1 rounded">spBv1.0/{grupo}/#</code>
                  </span>
                </Label>
                <button
                  v-if="spGroupSelected.size > 0"
                  type="button"
                  class="inline-flex items-center gap-1 text-xs text-destructive hover:text-destructive/80 transition-colors"
                  @click="deleteSelectedSpGroups"
                >
                  <Trash2 class="h-3.5 w-3.5" /> Eliminar ({{ spGroupSelected.size }})
                </button>
              </div>

              <!-- Add + search bar -->
              <div class="flex gap-2">
                <div class="relative flex-1">
                  <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground pointer-events-none" />
                  <input
                    v-model="spGroupSearch"
                    type="text"
                    placeholder="Filtrar grupos…"
                    class="w-full h-8 pl-7 pr-3 rounded-sm border border-border bg-background text-xs font-mono outline-none focus:ring-1 focus:ring-[color:var(--epm-bosque)] placeholder:text-muted-foreground/60"
                  />
                </div>
                <div class="relative flex-[2]">
                  <input
                    v-model="spGroupNew"
                    type="text"
                    placeholder="Nuevo grupo (Enter o pega múltiples separados por coma)"
                    class="w-full h-8 px-3 rounded-sm border border-border bg-background text-xs font-mono outline-none focus:ring-1 focus:ring-[color:var(--epm-bosque)] placeholder:text-muted-foreground/60"
                    @keydown="onSpGroupNewKeydown"
                    @blur="addSpGroupFromInput"
                  />
                </div>
                <button
                  type="button"
                  class="h-8 px-3 rounded-sm border border-[color:var(--epm-bosque)] text-[color:var(--epm-bosque)] hover:bg-[color:color-mix(in_srgb,var(--epm-bosque)_10%,transparent)] text-xs font-medium transition-colors inline-flex items-center gap-1.5 shrink-0"
                  @click="addSpGroupFromInput"
                >
                  <Plus class="h-3.5 w-3.5" /> Agregar
                </button>
              </div>

              <!-- Table -->
              <div class="rounded-sm border border-border overflow-hidden">
                <div class="overflow-y-auto" style="max-height:280px">
                  <table class="w-full text-xs">
                    <thead class="sticky top-0 z-10 bg-muted/90 backdrop-blur-sm border-b border-border">
                      <tr>
                        <th class="w-8 px-2 py-2 text-left">
                          <input
                            type="checkbox"
                            class="rounded-sm accent-[color:var(--epm-bosque)] cursor-pointer"
                            :checked="filteredSpGroups.length > 0 && filteredSpGroups.every(({i}) => spGroupSelected.has(i))"
                            :indeterminate="filteredSpGroups.some(({i}) => spGroupSelected.has(i)) && !filteredSpGroups.every(({i}) => spGroupSelected.has(i))"
                            @change="toggleSelectAllSpGroups"
                          />
                        </th>
                        <th class="w-10 px-3 py-2 text-left font-semibold text-muted-foreground uppercase tracking-[0.14em] text-[10px]">#</th>
                        <th class="px-3 py-2 text-left font-semibold text-muted-foreground uppercase tracking-[0.14em] text-[10px]">Group ID</th>
                        <th class="px-3 py-2 text-left font-semibold text-muted-foreground uppercase tracking-[0.14em] text-[10px]">Tópico MQTT</th>
                        <th class="w-8 px-2 py-2"></th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="filteredSpGroups.length === 0">
                        <td colspan="5" class="px-3 py-5 text-center text-muted-foreground text-xs">
                          {{ spGroups.length === 0 ? 'Sin grupos configurados — agrega el primero arriba' : 'Sin resultados para la búsqueda' }}
                        </td>
                      </tr>
                      <tr
                        v-for="({g, i}) in filteredSpGroups"
                        :key="i"
                        class="border-t border-border/50 hover:bg-muted/40 transition-colors"
                        :class="{ 'bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)]': spGroupSelected.has(i) }"
                      >
                        <td class="px-2 py-1.5 text-center">
                          <input
                            type="checkbox"
                            class="rounded-sm accent-[color:var(--epm-bosque)] cursor-pointer"
                            :checked="spGroupSelected.has(i)"
                            @change="toggleSelectSpGroup(i)"
                          />
                        </td>
                        <td class="px-3 py-1.5 text-muted-foreground tabular-nums select-none">{{ i + 1 }}</td>
                        <td class="px-3 py-1.5 font-mono font-semibold text-[color:var(--epm-bosque)]">{{ g }}</td>
                        <td class="px-3 py-1.5 font-mono text-muted-foreground/70">spBv1.0/{{ g }}/#</td>
                        <td class="px-2 py-1.5 text-right">
                          <button
                            type="button"
                            class="text-muted-foreground hover:text-destructive transition-colors"
                            @click="removeSpGroup(i)"
                          >
                            <Trash2 class="h-3.5 w-3.5" />
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </template>
        </div>

        <div class="col-span-6 flex justify-end pt-1">
          <Button
            class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
            :disabled="savingBroker"
            @click="saveBroker"
          >
            <Save class="h-4 w-4 mr-1.5" />
            {{ savingBroker ? 'Guardando…' : 'Guardar broker' }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Toolbar -->
    <div class="flex flex-wrap items-center gap-3">
      <Button
        v-if="selectedTopics.size > 0"
        size="sm" variant="destructive"
        class="rounded-sm h-8 text-xs"
        @click="delSelectedTopics"
      >
        <Trash2 class="h-3.5 w-3.5 mr-1" /> Eliminar tópicos ({{ selectedTopics.size }})
      </Button>
    </div>
    <div class="flex flex-wrap items-center gap-3">
      <div class="relative flex-1 min-w-[200px] max-w-md">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
        <Input
          v-model="search"
          placeholder="Search server, device or topic…"
          class="pl-8 rounded-sm font-mono text-xs"
        />
      </div>
      <div class="inline-flex rounded-sm border border-border overflow-hidden text-[11px]">
        <button
          class="px-3 py-1.5 inline-flex items-center gap-1.5 hover:bg-muted transition-colors"
          title="Expand all"
          @click="expandAll"
        ><ChevronsUpDown class="h-3.5 w-3.5" /> Expand</button>
        <button
          class="px-3 py-1.5 inline-flex items-center gap-1.5 hover:bg-muted transition-colors border-l border-border"
          title="Collapse all"
          @click="collapseAll"
        ><ChevronsDownUp class="h-3.5 w-3.5" /> Collapse</button>
      </div>
    </div>

    <!-- Tree -->
    <div v-if="!servers.length" class="card-soft p-6 text-sm text-muted-foreground">
      Sin servidores IEC-104 configurados. Agregue al menos uno en <strong>IEC 104</strong> primero.
    </div>
    <div v-else-if="!tree.length" class="card-soft p-6 text-sm text-muted-foreground">
      Nada coincide con el filtro actual.
    </div>

    <div
      v-for="sn in tree"
      :key="`s:${sn.server?.id ?? -999}`"
      class="card-soft overflow-hidden"
    >
      <!-- Server header -->
      <div class="flex items-center gap-3 px-4 py-3 hover:bg-muted/40 transition-colors">
        <button
          type="button"
          class="flex items-center gap-3 min-w-0 flex-1 text-left"
          @click="toggle(`s:${sn.server?.id ?? -999}`)"
        >
          <ChevronRight
            class="h-4 w-4 shrink-0 text-muted-foreground transition-transform"
            :class="isOpen(`s:${sn.server?.id ?? -999}`) ? 'rotate-90' : ''"
          />
          <ServerIcon class="h-4 w-4 shrink-0 text-[color:var(--epm-bosque)]" />
          <div class="min-w-0">
            <div class="font-bold text-sm truncate">
              {{ sn.server?.name ?? 'Orphan rows' }}
            </div>
            <div class="text-[11px] text-muted-foreground truncate font-mono">
              <template v-if="sn.server">
                :{{ sn.server.port }} · ASDU {{ sn.server.asdu_addr }}
                <span v-if="!sn.server.enabled" class="ml-2 uppercase tracking-wider text-[10px] not-italic font-bold text-[color:var(--destructive,theme(colors.red.500))]">disabled</span>
              </template>
              <template v-else>devices whose server row was deleted</template>
            </div>
          </div>
        </button>
        <span class="shrink-0 font-mono text-[10px] uppercase tracking-wider px-2 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)] font-bold">
          {{ sn.devices.length }} dev · {{ sn.topicCount }} top
        </span>
        <Button
          v-if="sn.server"
          variant="ghost" size="icon"
          title="Agregar dispositivo"
          class="text-muted-foreground hover:text-[color:var(--epm-bosque)]"
          @click="openCreateDevice(sn.server.id)"
        >
          <Plus class="h-4 w-4" />
        </Button>
      </div>

      <!-- Devices -->
      <div v-show="isOpen(`s:${sn.server?.id ?? -999}`)" class="border-t border-border bg-muted/20">
        <div v-if="!sn.devices.length" class="px-10 py-3 text-xs text-muted-foreground italic">
          Sin dispositivos en este servidor. Haga clic en + para agregar uno.
        </div>
        <div
          v-for="dn in sn.devices"
          :key="`s:${sn.server?.id ?? -999}:d:${dn.device.id}`"
          class="border-b last:border-b-0 border-border/60"
        >
          <!-- Device sub-header -->
          <div class="flex items-center gap-3 pl-10 pr-4 py-2 hover:bg-muted/40 transition-colors">
            <button
              type="button"
              class="flex items-center gap-3 min-w-0 flex-1 text-left"
              @click="toggle(`s:${sn.server?.id ?? -999}:d:${dn.device.id}`)"
            >
              <ChevronRight
                class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform"
                :class="isOpen(`s:${sn.server?.id ?? -999}:d:${dn.device.id}`) ? 'rotate-90' : ''"
              />
              <component
                :is="isOpen(`s:${sn.server?.id ?? -999}:d:${dn.device.id}`) ? FolderOpen : Folder"
                class="h-3.5 w-3.5 shrink-0 text-[color:var(--epm-citrico)]"
              />
              <div class="min-w-0">
                <div class="font-semibold text-xs truncate">{{ dn.device.name }}</div>
                <div class="text-[10px] text-muted-foreground truncate">
                  {{ dn.device.description || '—' }}
                </div>
              </div>
            </button>
            <span class="font-mono text-[10px] text-muted-foreground shrink-0">
              {{ dn.topics.length }} topic{{ dn.topics.length === 1 ? '' : 's' }}
            </span>
            <Button variant="ghost" size="icon" title="Agregar tópico" class="text-muted-foreground hover:text-[color:var(--epm-bosque)]" @click="openCreateTopic(dn.device.id)">
              <Plus class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" title="Edit device" @click="openEditDevice(dn.device)">
              <Pencil class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" title="Delete device" class="text-[color:var(--destructive)]" @click="deleteDevice(dn.device)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>

          <!-- Topics under this device -->
          <div v-show="isOpen(`s:${sn.server?.id ?? -999}:d:${dn.device.id}`)">
            <div v-if="!dn.topics.length" class="pl-20 py-2 text-xs text-muted-foreground italic">
              Sin tópicos aún. Haga clic en + en la fila del dispositivo para suscribirse.
            </div>
            <div v-else class="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
                    <TableHead class="w-10 pl-20">
                      <input
                        type="checkbox"
                        class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                        :checked="topicsAllSelectedForDevice(dn.device.id)"
                        :indeterminate="(topicsByDevice.get(dn.device.id) ?? []).some(t => selectedTopics.has(t.id)) && !topicsAllSelectedForDevice(dn.device.id)"
                        @change="toggleSelectAllTopicsForDevice(dn.device.id)"
                      />
                    </TableHead>
                    <TableHead class="w-12 text-[10px] uppercase tracking-[0.18em] font-bold">ID</TableHead>
                    <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">Topic</TableHead>
                    <TableHead class="w-24 text-[10px] uppercase tracking-[0.18em] font-bold">Formato</TableHead>
                    <TableHead class="w-16 text-[10px] uppercase tracking-[0.18em] font-bold">QoS</TableHead>
                    <TableHead class="w-16 text-[10px] uppercase tracking-[0.18em] font-bold">On</TableHead>
                    <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.18em] font-bold">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow
                    v-for="t in dn.topics" :key="t.id"
                    class="data-row border-b border-border/40 last:border-b-0"
                    :class="selectedTopics.has(t.id) ? 'bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]' : ''"
                  >
                    <TableCell class="pl-20">
                      <input
                        type="checkbox"
                        class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                        :checked="selectedTopics.has(t.id)"
                        @change="toggleSelectTopic(t.id)"
                      />
                    </TableCell>
                    <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ t.id }}</TableCell>
                    <TableCell class="font-mono text-xs">
                      <span class="inline-flex items-center gap-2">
                        <Radio class="h-3 w-3 text-muted-foreground shrink-0" />
                        <span class="truncate">{{ t.topic }}</span>
                      </span>
                    </TableCell>
                    <TableCell>
                      <span
                        class="inline-flex items-center rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider"
                        :class="t.payload_format === 'sparkplug'
                          ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                          : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'"
                      >
                        {{ t.payload_format === 'sparkplug' ? 'SpB' : 'JSON' }}
                      </span>
                    </TableCell>
                    <TableCell class="font-mono text-xs">{{ t.qos }}</TableCell>
                    <TableCell>
                      <Switch :model-value="t.enabled" @update:model-value="() => toggleTopic(t)" />
                    </TableCell>
                    <TableCell class="text-right whitespace-nowrap">
                      <Button variant="ghost" size="icon" @click="openEditTopic(t)" title="Edit topic">
                        <Pencil class="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="icon" @click="deleteTopic(t)" class="text-[color:var(--destructive)]" title="Delete topic">
                        <Trash2 class="h-4 w-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Device dialog -->
    <Dialog v-model:open="devDialog">
      <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
        <div class="px-6 pt-6 pb-4 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
          <DialogHeader class="text-left space-y-1">
            <DialogTitle class="text-lg font-extrabold tracking-tight flex items-center gap-2">
              <Folder class="h-4 w-4 text-[color:var(--epm-bosque)]" />
              {{ isEditDev ? `Editar dispositivo #${editingDev.id}` : 'Nuevo dispositivo' }}
            </DialogTitle>
            <DialogDescription class="text-xs">
              Un dispositivo agrupa tópicos MQTT bajo un activo lógico (ej. inversor, estación meteorológica).
              Pertenece a un esclavo IEC-104: el mismo activo físico en dos esclavos son dos filas.
            </DialogDescription>
          </DialogHeader>
        </div>

        <div class="px-6 py-5 grid grid-cols-1 gap-3">
          <div class="space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Servidor IEC-104</Label>
            <Select v-model="editingDev.server_id">
              <SelectTrigger class="w-full"><SelectValue placeholder="Select slave endpoint…" /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="s in servers" :key="s.id" :value="s.id">
                  {{ s.name }} · :{{ s.port }} · ASDU {{ s.asdu_addr }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="space-y-1.5">
            <Label for="dname" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ i18n.devices.device }}</Label>
            <Input id="dname" v-model="editingDev.name" placeholder="INV_1" class="rounded-sm font-mono" @keyup.enter="saveDevice" />
          </div>
          <div class="space-y-1.5">
            <Label for="ddesc" class="text-[11px] uppercase tracking-[0.18em] font-bold">Descripción</Label>
            <Input id="ddesc" v-model="editingDev.description" placeholder="Inversor 1" class="rounded-sm" @keyup.enter="saveDevice" />
          </div>
        </div>

        <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
          <Button variant="outline" @click="devDialog = false" class="rounded-sm">{{ i18n.common.cancel }}</Button>
          <Button @click="saveDevice" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
            {{ isEditDev ? i18n.common.save : 'Crear' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Topic dialog -->
    <Dialog v-model:open="topicDialog">
      <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
        <div class="px-6 pt-6 pb-4 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
          <DialogHeader class="text-left space-y-1">
            <DialogTitle class="text-lg font-extrabold tracking-tight flex items-center gap-2">
              <Radio class="h-4 w-4 text-[color:var(--epm-bosque)]" />
              {{ isEditTopic ? `Editar tópico #${editingTopic.id}` : 'Nuevo tópico' }}
            </DialogTitle>
            <DialogDescription class="text-xs">
              Cadena de suscripción MQTT. Se aceptan comodines (<code>+</code>, <code>#</code>).
            </DialogDescription>
          </DialogHeader>
        </div>

        <div class="px-6 py-5 grid grid-cols-3 gap-3">
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ i18n.devices.device }}</Label>
            <div class="rounded-sm border border-border bg-muted/30 px-3 py-2 text-sm font-bold">
              {{ devById[editingTopic.device_id]?.name ?? '?' }}
              <span class="ml-2 text-muted-foreground font-normal">
                {{ devById[editingTopic.device_id]?.description }}
              </span>
            </div>
          </div>
          <div class="col-span-2 space-y-1.5">
            <Label for="tstr" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ i18n.devices.topic }}</Label>
            <Input id="tstr" v-model="editingTopic.topic" placeholder="EPM/SSFV/.../INV_1" class="rounded-sm font-mono" @keyup.enter="saveTopic" />
          </div>
          <div class="col-span-1 space-y-1.5">
            <Label for="tqos" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ i18n.devices.qos }}</Label>
            <Input id="tqos" v-model.number="editingTopic.qos" type="number" min="0" max="2" class="rounded-sm" />
          </div>
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Formato payload</Label>
            <div class="flex gap-2">
              <button
                type="button"
                class="flex-1 py-2 rounded-sm border text-xs font-bold uppercase tracking-wider transition-colors"
                :class="editingTopic.payload_format === 'json'
                  ? 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                  : 'border-border hover:bg-muted'"
                @click="editingTopic.payload_format = 'json'"
              >JSON</button>
              <button
                type="button"
                class="flex-1 py-2 rounded-sm border text-xs font-bold uppercase tracking-wider transition-colors"
                :class="editingTopic.payload_format === 'sparkplug'
                  ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'border-border hover:bg-muted'"
                @click="editingTopic.payload_format = 'sparkplug'"
              >Sparkplug B</button>
            </div>
          </div>
          <div class="col-span-3 flex items-center gap-3 pt-1">
            <Switch id="ten" v-model="editingTopic.enabled" />
            <Label for="ten">{{ i18n.devices.enabled }}</Label>
          </div>
        </div>

        <DialogFooter class="px-6 pb-6 pt-4 border-t border-border">
          <Button variant="outline" @click="topicDialog = false" class="rounded-sm">{{ i18n.common.cancel }}</Button>
          <Button @click="saveTopic" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
            {{ isEditTopic ? i18n.common.save : 'Crear' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
