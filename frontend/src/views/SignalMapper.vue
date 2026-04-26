<script setup lang="ts">
import { ref, onMounted, computed, reactive, watch } from 'vue'
import { toast } from 'vue-sonner'
import { api, type SignalMapping, type Topic, type Device, type IEC104Server } from '@/api'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogTrigger,
} from '@/components/ui/dialog'
import {
  Pencil, Trash2, Plus, Layers, Folder, FolderOpen, ChevronRight,
  Activity, ToggleLeft, Search, ListTree, Table as TableIcon, Server as ServerIcon,
  ChevronsDownUp, ChevronsUpDown,
} from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

const IEC_TYPES = [
  { v: 'M_ME_TF_1', desc: 'short float w/ time' },
  { v: 'M_ME_NC_1', desc: 'short float, no time' },
  { v: 'M_ME_NA_1', desc: 'normalized' },
  { v: 'M_ME_NB_1', desc: 'scaled int' },
  { v: 'M_SP_NA_1', desc: 'single-point (bool)' },
  { v: 'M_SP_TB_1', desc: 'single-point w/ time' },
  { v: 'M_DP_NA_1', desc: 'double-point' },
  { v: 'M_IT_NA_1', desc: 'integrated total' },
  { v: 'M_IT_TB_1', desc: 'integrated total w/ time' },
]
const iecDesc = computed(
  () => IEC_TYPES.find(t => t.v === editing.iec104_type)?.desc ?? '',
)

const mappings = ref<SignalMapping[]>([])
const topics = ref<Topic[]>([])
const devices = ref<Device[]>([])
const servers = ref<IEC104Server[]>([])
const loading = ref(false)

const topicById = computed(() => Object.fromEntries(topics.value.map(t => [t.id, t])))
const deviceById = computed(() => Object.fromEntries(devices.value.map(d => [d.id, d])))
const serverById = computed(() => Object.fromEntries(servers.value.map(s => [s.id, s])))

const devicesByServer = computed(() => {
  const m = new Map<number, Device[]>()
  for (const d of devices.value) {
    const arr = m.get(d.server_id) ?? []
    arr.push(d)
    m.set(d.server_id, arr)
  }
  return m
})

type Kind = 'analog' | 'digital'

function classify(iec: string): Kind {
  // M_SP_*, M_DP_* → digital. M_ME_*, M_IT_* → analog.
  if (iec.startsWith('M_SP') || iec.startsWith('M_DP')) return 'digital'
  return 'analog'
}

const search = ref('')
const view = ref<'tree' | 'flat'>('tree')

// expanded tracks which nodes are open. Default is CLOSED for all levels;
// toggling a device auto-opens its kind buckets to avoid an extra click.
const expanded = ref<Record<string, boolean>>({})

function isOpen(key: string) {
  return expanded.value[key] === true
}

function toggle(key: string) {
  expanded.value[key] = !isOpen(key)
}

function toggleDevice(sid: number, did: number, dn: DeviceNode) {
  const key = nodeKey(sid, did)
  const opening = !isOpen(key)
  expanded.value[key] = opening
  if (opening) {
    for (const b of dn.buckets) expanded.value[nodeKey(sid, did, b.kind)] = true
  }
}

function expandAll() {
  const next: Record<string, boolean> = {}
  for (const sn of tree.value) {
    const sid = sn.server?.id ?? -999
    next[`s:${sid}`] = true
    for (const dn of sn.devices) {
      const did = dn.device?.id ?? -1
      next[`s:${sid}:d:${did}`] = true
      for (const b of dn.buckets) next[`s:${sid}:d:${did}:${b.kind}`] = true
    }
  }
  expanded.value = next
}
function collapseAll() { expanded.value = {} }

const filteredMappings = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return mappings.value
  return mappings.value.filter(m => {
    const t = topicById.value[m.topic_id]
    const dev = t ? deviceById.value[t.device_id] : null
    const srv = serverById.value[m.server_id]
    return [
      m.device_name, m.variable_type, m.characteristic, m.json_key,
      m.iec104_type, String(m.ioa), m.unit,
      t?.topic ?? '', dev?.name ?? '', dev?.description ?? '',
      srv?.name ?? '', srv ? String(srv.port) : '',
    ].some(s => (s ?? '').toLowerCase().includes(q))
  })
})

interface KindBucket { kind: Kind; mappings: SignalMapping[] }
interface DeviceNode {
  device: Device | null         // null = orphan bucket (mapping's topic was deleted)
  buckets: KindBucket[]
  total: number
}
interface ServerNode {
  server: IEC104Server | null   // null = mapping references a deleted server
  devices: DeviceNode[]
  total: number
}

const tree = computed<ServerNode[]>(() => {
  // server_id → device_id (or -1 for orphan-topic) → mapping[]
  const byServer = new Map<number, Map<number, SignalMapping[]>>()
  const ensureDev = (sid: number, did: number) => {
    let dm = byServer.get(sid)
    if (!dm) { dm = new Map(); byServer.set(sid, dm) }
    let arr = dm.get(did)
    if (!arr) { arr = []; dm.set(did, arr) }
    return arr
  }
  for (const m of filteredMappings.value) {
    const t = topicById.value[m.topic_id]
    const did = t ? t.device_id : -1
    ensureDev(m.server_id, did).push(m)
  }

  const orderedServers: (IEC104Server | null)[] = [...servers.value]
  // Trail an "Unbound" server node if any mapping references a deleted server.
  for (const sid of byServer.keys()) {
    if (!serverById.value[sid]) orderedServers.push(null)
  }

  const nodes: ServerNode[] = orderedServers.map(s => {
    const sid = s?.id ?? -999
    const devMap = byServer.get(s ? s.id : sid) ?? new Map()
    const ownDevices = s ? (devicesByServer.value.get(s.id) ?? []) : []
    const devNodes: DeviceNode[] = ownDevices
      .map(d => buildDeviceNode(d, devMap.get(d.id) ?? []))
      .filter(n => n.total > 0 || !search.value)
    const orphan = devMap.get(-1) ?? []
    if (orphan.length) devNodes.push(buildDeviceNode(null, orphan))
    const total = devNodes.reduce((n, d) => n + d.total, 0)
    return { server: s, devices: devNodes, total }
  })
  // When searching, hide servers with no matches; otherwise show every server.
  return nodes.filter(n => n.total > 0 || !search.value)
})

function buildDeviceNode(dev: Device | null, list: SignalMapping[]): DeviceNode {
  const sorted = [...list].sort((a, b) => a.ioa - b.ioa)
  const analog = sorted.filter(m => classify(m.iec104_type) === 'analog')
  const digital = sorted.filter(m => classify(m.iec104_type) === 'digital')
  const buckets: KindBucket[] = []
  if (digital.length) buckets.push({ kind: 'digital', mappings: digital })
  if (analog.length) buckets.push({ kind: 'analog', mappings: analog })
  return { device: dev, buckets, total: sorted.length }
}

function nodeKey(sid: number, did: number, kind?: Kind) {
  return kind ? `s:${sid}:d:${did}:${kind}` : did === undefined ? `s:${sid}` : `s:${sid}:d:${did}`
}

watch(search, q => {
  if (!q.trim()) return
  for (const sn of tree.value) {
    const sid = sn.server?.id ?? -999
    expanded.value[`s:${sid}`] = true
    for (const dn of sn.devices) {
      const did = dn.device?.id ?? -1
      expanded.value[`s:${sid}:d:${did}`] = true
      for (const b of dn.buckets) expanded.value[`s:${sid}:d:${did}:${b.kind}`] = true
    }
  }
})

function empty(): SignalMapping {
  return {
    id: 0, server_id: 0, topic_id: 0, device_name: '', variable_type: '', characteristic: '',
    json_key: '', iec104_type: 'M_ME_TF_1', ioa: 0, unit: '', scale: 1.0, enabled: true,
  }
}

const dialogOpen = ref(false)
const editing = reactive<SignalMapping>(empty())
const isEdit = computed(() => editing.id > 0)

// IOA suggestion = next free slot within the chosen server's address space.
function suggestIOA(serverID: number, currentID = 0) {
  const used = mappings.value
    .filter(m => m.server_id === serverID && m.id !== currentID)
    .map(m => m.ioa)
  const max = used.reduce((m, x) => Math.max(m, x), 16384)
  return max + 1
}

function openCreate() {
  Object.assign(editing, empty())
  if (servers.value.length === 1) editing.server_id = servers.value[0].id
  editing.ioa = editing.server_id ? suggestIOA(editing.server_id) : 16385
  dialogOpen.value = true
}

function openEdit(m: SignalMapping) {
  Object.assign(editing, m)
  dialogOpen.value = true
}

async function reload() {
  loading.value = true
  try {
    const [m, t, d, s] = await Promise.all([
      api.get<SignalMapping[]>('/mappings'),
      api.get<Topic[]>('/topics'),
      api.get<Device[]>('/devices'),
      api.get<IEC104Server[]>('/iec104-servers'),
    ])
    mappings.value = m.data ?? []
    topics.value = t.data ?? []
    devices.value = d.data ?? []
    servers.value = s.data ?? []
  } catch (e: any) { toast.error('Load: ' + (e?.message ?? e)) }
  finally { loading.value = false }
}

function openCreateUnder(serverId: number, deviceId: number | null, kind: Kind) {
  Object.assign(editing, empty())
  editing.server_id = serverId
  editing.ioa = suggestIOA(serverId)
  if (deviceId != null) {
    const t = topics.value.find(x => x.device_id === deviceId)
    if (t) editing.topic_id = t.id
    const dev = devices.value.find(d => d.id === deviceId)
    if (dev) editing.device_name = dev.name
  }
  editing.iec104_type = kind === 'digital' ? 'M_SP_NA_1' : 'M_ME_TF_1'
  dialogOpen.value = true
}

// Devices are scoped per server, so only show topics whose device belongs to
// the picked server. Otherwise the backend would silently override the user's
// server choice (it derives server_id from topic → device).
const topicsForDialog = computed(() => {
  if (!editing.server_id) return [] as Topic[]
  return topics.value.filter(t => {
    const dev = deviceById.value[t.device_id]
    return dev && dev.server_id === editing.server_id
  })
})

// Reset topic when server changes if the current topic no longer belongs.
watch(() => editing.server_id, sid => {
  if (!sid) { editing.topic_id = 0; return }
  const t = topics.value.find(x => x.id === editing.topic_id)
  const dev = t ? deviceById.value[t.device_id] : null
  if (!dev || dev.server_id !== sid) editing.topic_id = 0
  editing.ioa = suggestIOA(sid, editing.id)
})

function validate(): string | null {
  if (!editing.server_id) return 'IEC-104 server is required'
  if (!editing.topic_id) return 'Topic is required'
  if (!editing.json_key.trim()) return 'JSON key is required'
  if (!editing.iec104_type) return 'IEC 104 type required'
  if (!Number.isInteger(editing.ioa) || editing.ioa <= 0) return 'IOA must be positive int'
  const dup = mappings.value.find(
    x => x.server_id === editing.server_id && x.ioa === editing.ioa && x.id !== editing.id,
  )
  if (dup) {
    const srvName = serverById.value[editing.server_id]?.name ?? `#${editing.server_id}`
    return `IOA ${editing.ioa} already used on server "${srvName}" by mapping #${dup.id}`
  }
  if (!editing.scale || editing.scale === 0) editing.scale = 1.0
  return null
}

async function save() {
  const err = validate()
  if (err) { toast.error(err); return }
  try {
    if (isEdit.value) {
      await api.put(`/mappings/${editing.id}`, editing)
      toast.success(`Mapping #${editing.id} updated`)
    } else {
      await api.post('/mappings', editing)
      toast.success('Mapping created')
    }
    dialogOpen.value = false
    await reload()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Save failed')
  }
}

async function toggleEnabled(m: SignalMapping) {
  try {
    await api.put(`/mappings/${m.id}`, { ...m, enabled: !m.enabled })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function del(m: SignalMapping) {
  const ok = await confirm({
    title: 'Delete mapping',
    message: 'Removes this MQTT → IEC-104 binding. Cached point stays until restart; new samples for this key will be ignored.',
    detail: `IOA ${m.ioa} · ${m.iec104_type} · key "${m.json_key}"`,
    variant: 'danger',
    confirmText: 'Delete mapping',
  })
  if (!ok) return
  try {
    await api.delete(`/mappings/${m.id}`)
    await reload()
    toast.success('Deleted')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

onMounted(reload)
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6">
    <!-- Header -->
    <div class="flex items-start justify-between border-b border-border pb-5">
      <div class="flex items-start gap-3">
        <div class="grid place-items-center w-9 h-9 rounded-sm bg-[color:var(--epm-bosque)] text-white">
          <Layers class="h-4 w-4" />
        </div>
        <div>
          <div class="text-[10px] uppercase tracking-[0.24em] font-bold text-[color:var(--epm-bosque)]">Configuration</div>
          <h1 class="mt-1 mb-1 text-2xl font-extrabold">Signal mapping</h1>
          <p class="text-sm text-muted-foreground">MQTT JSON key → IEC 104 point · {{ mappings.length }} active</p>
        </div>
      </div>
      <Dialog v-model:open="dialogOpen">
        <DialogTrigger as-child>
          <Button @click="openCreate" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-4">
            <Plus class="h-4 w-4 mr-1" /> New mapping
          </Button>
        </DialogTrigger>
        <DialogContent class="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{{ isEdit ? `Edit mapping #${editing.id}` : 'New mapping' }}</DialogTitle>
            <DialogDescription>Bind a JSON key from an MQTT topic to an IEC 104 point.</DialogDescription>
          </DialogHeader>

          <div class="grid grid-cols-6 gap-x-4 gap-y-3 py-2">
            <div class="col-span-6 space-y-1.5">
              <Label>IEC-104 server</Label>
              <Select v-model="editing.server_id">
                <SelectTrigger class="w-full"><SelectValue placeholder="Select slave endpoint…" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="s in servers" :key="s.id" :value="s.id">
                    {{ s.name }} · :{{ s.port }} · ASDU {{ s.asdu_addr }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <p class="text-[11px] text-muted-foreground">
                Routes the value to this slave only. SCADA on this endpoint will see it under IOA + ASDU.
              </p>
            </div>
            <div class="col-span-6 space-y-1.5">
              <Label>Topic</Label>
              <Select v-model="editing.topic_id">
                <SelectTrigger class="w-full"><SelectValue placeholder="Select topic…" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="t in topicsForDialog" :key="t.id" :value="t.id">
                    {{ t.topic }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div class="col-span-3 space-y-1.5">
              <Label>Device (label)</Label>
              <Input v-model="editing.device_name" placeholder="Inversor 1" />
            </div>
            <div class="col-span-3 space-y-1.5">
              <Label>JSON key</Label>
              <Input v-model="editing.json_key" placeholder="IA" />
            </div>

            <div class="col-span-3 space-y-1.5">
              <Label>Variable type</Label>
              <Input v-model="editing.variable_type" placeholder="Corr. Fase A" />
            </div>
            <div class="col-span-3 space-y-1.5">
              <Label>Characteristic</Label>
              <Input v-model="editing.characteristic" placeholder="Corriente AC" />
            </div>

            <div class="col-span-4 space-y-1.5 min-w-0">
              <Label>IEC 104 type</Label>
              <Select v-model="editing.iec104_type">
                <SelectTrigger class="w-full font-mono">
                  <SelectValue placeholder="Select…" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="t in IEC_TYPES" :key="t.v" :value="t.v" class="font-mono">
                    {{ t.v }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <p v-if="iecDesc" class="text-[11px] text-muted-foreground mt-1 truncate">{{ iecDesc }}</p>
            </div>
            <div class="col-span-2 space-y-1.5">
              <Label>IOA</Label>
              <Input v-model.number="editing.ioa" type="number" />
            </div>

            <div class="col-span-3 space-y-1.5">
              <Label>Unit</Label>
              <Input v-model="editing.unit" placeholder="A, V, kW…" />
            </div>
            <div class="col-span-3 space-y-1.5">
              <Label>Scale</Label>
              <Input v-model.number="editing.scale" type="number" step="0.001" />
            </div>

            <div class="col-span-6 flex items-center gap-3 pt-2">
              <Switch id="en" v-model="editing.enabled" />
              <Label for="en">Enabled</Label>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" @click="dialogOpen = false" class="rounded-sm">Cancel</Button>
            <Button @click="save" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
              {{ isEdit ? 'Save changes' : 'Create' }}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>

    <!-- Toolbar: search + view toggle + expand/collapse -->
    <div class="flex flex-wrap items-center gap-3">
      <div class="relative flex-1 min-w-[200px] max-w-md">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
        <Input
          v-model="search"
          placeholder="Search IOA, key, topic, device…"
          class="pl-8 rounded-sm font-mono text-xs"
        />
      </div>
      <div class="inline-flex rounded-sm border border-border bg-card overflow-hidden text-[11px]">
        <button
          class="px-3 py-1.5 inline-flex items-center gap-1.5 transition-colors"
          :class="view === 'tree' ? 'bg-[color:var(--epm-bosque)] text-white' : 'hover:bg-muted'"
          @click="view = 'tree'"
        ><ListTree class="h-3.5 w-3.5" /> Tree</button>
        <button
          class="px-3 py-1.5 inline-flex items-center gap-1.5 transition-colors border-l border-border"
          :class="view === 'flat' ? 'bg-[color:var(--epm-bosque)] text-white' : 'hover:bg-muted'"
          @click="view = 'flat'"
        ><TableIcon class="h-3.5 w-3.5" /> Flat</button>
      </div>
      <div v-if="view === 'tree'" class="inline-flex rounded-sm border border-border overflow-hidden text-[11px]">
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
      <div class="text-[11px] text-muted-foreground ml-auto font-mono">
        {{ filteredMappings.length }} / {{ mappings.length }}
      </div>
    </div>

    <!-- TREE VIEW -->
    <div v-if="view === 'tree'" class="space-y-3">
      <div v-if="!topics.length" class="card-soft p-6 text-sm text-muted-foreground">
        No topics configured. Add devices + topics first under <strong>Devices &amp; Topics</strong>.
      </div>
      <div v-else-if="!tree.length" class="card-soft p-6 text-sm text-muted-foreground">
        {{ search ? 'No mappings match the current filter.' : 'No mappings yet. Click "New mapping".' }}
      </div>

      <div
        v-for="sn in tree"
        :key="`s:${sn.server?.id ?? -999}`"
        class="card-soft overflow-hidden"
      >
        <!-- Server header -->
        <button
          type="button"
          class="w-full flex items-center gap-3 px-4 py-3 hover:bg-muted/40 transition-colors text-left"
          @click="toggle(`s:${sn.server?.id ?? -999}`)"
        >
          <ChevronRight
            class="h-4 w-4 shrink-0 text-muted-foreground transition-transform"
            :class="isOpen(`s:${sn.server?.id ?? -999}`) ? 'rotate-90' : ''"
          />
          <ServerIcon class="h-4 w-4 shrink-0 text-[color:var(--epm-bosque)]" />
          <div class="flex-1 min-w-0">
            <div class="font-bold text-sm truncate">
              {{ sn.server?.name ?? 'Orphan mappings' }}
            </div>
            <div class="text-[11px] text-muted-foreground truncate font-mono">
              <template v-if="sn.server">
                :{{ sn.server.port }} · ASDU {{ sn.server.asdu_addr }}
                <span v-if="!sn.server.enabled" class="ml-2 uppercase tracking-wider text-[10px] not-italic font-bold text-[color:var(--destructive,theme(colors.red.500))]">disabled</span>
              </template>
              <template v-else>mappings whose server row was deleted</template>
            </div>
          </div>
          <span
            v-if="sn.server"
            class="shrink-0 inline-flex items-center justify-center w-6 h-6 rounded-sm hover:bg-[color:var(--epm-bosque)] hover:text-white text-muted-foreground"
            title="Add mapping on this server"
            @click.stop="openCreateUnder(sn.server.id, null, 'analog')"
          >
            <Plus class="h-3.5 w-3.5" />
          </span>
          <span class="shrink-0 font-mono text-[10px] uppercase tracking-wider px-2 py-0.5 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)] font-bold">
            {{ sn.total }} pt{{ sn.total === 1 ? '' : 's' }}
          </span>
        </button>

        <!-- Devices under this server -->
        <div v-show="isOpen(`s:${sn.server?.id ?? -999}`)" class="border-t border-border bg-muted/20">
          <div v-if="!sn.devices.length" class="px-10 py-3 text-xs text-muted-foreground italic">
            No mappings on this server yet. Click + to add one.
          </div>
          <div
            v-for="dn in sn.devices"
            :key="nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1)"
            class="border-b last:border-b-0 border-border/60"
          >
            <!-- Device sub-header -->
            <button
              type="button"
              class="w-full flex items-center gap-3 pl-10 pr-4 py-2 hover:bg-muted/40 transition-colors text-left"
              @click="toggleDevice(sn.server?.id ?? -999, dn.device?.id ?? -1, dn)"
            >
              <ChevronRight
                class="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform"
                :class="isOpen(nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1)) ? 'rotate-90' : ''"
              />
              <component
                :is="isOpen(nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1)) ? FolderOpen : Folder"
                class="h-3.5 w-3.5 shrink-0 text-[color:var(--epm-citrico)]"
              />
              <div class="flex-1 min-w-0">
                <div class="font-semibold text-xs truncate">
                  {{ dn.device?.name ?? 'Unbound topics' }}
                </div>
                <div class="text-[10px] text-muted-foreground truncate">
                  {{ dn.device?.description || (dn.device ? '—' : 'mappings whose topic was deleted') }}
                </div>
              </div>
              <span class="font-mono text-[10px] text-muted-foreground">
                {{ dn.total }}
              </span>
            </button>

            <!-- Kind buckets -->
            <div v-show="isOpen(nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1))">
              <div
                v-for="b in dn.buckets"
                :key="nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1, b.kind)"
                class="border-t border-border/40"
              >
                <button
                  type="button"
                  class="w-full flex items-center gap-3 pl-16 pr-4 py-1.5 hover:bg-muted/40 transition-colors text-left"
                  @click="toggle(nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1, b.kind))"
                >
                  <ChevronRight
                    class="h-3 w-3 shrink-0 text-muted-foreground transition-transform"
                    :class="isOpen(nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1, b.kind)) ? 'rotate-90' : ''"
                  />
                  <component
                    :is="b.kind === 'analog' ? Activity : ToggleLeft"
                    class="h-3 w-3 shrink-0"
                    :class="b.kind === 'analog' ? 'text-[color:var(--signal-warn,theme(colors.amber.500))]' : 'text-[color:var(--epm-bosque)]'"
                  />
                  <span class="flex-1 text-[11px] uppercase tracking-[0.18em] font-bold">
                    {{ b.kind === 'analog' ? 'Analogs' : 'Digitals' }}
                  </span>
                  <span class="font-mono text-[10px] text-muted-foreground">
                    {{ b.mappings.length }}
                  </span>
                  <span
                    v-if="sn.server && dn.device"
                    class="shrink-0 inline-flex items-center justify-center w-5 h-5 rounded-sm hover:bg-[color:var(--epm-bosque)] hover:text-white text-muted-foreground"
                    :title="`Add ${b.kind} mapping`"
                    @click.stop="openCreateUnder(sn.server.id, dn.device.id, b.kind)"
                  >
                    <Plus class="h-3 w-3" />
                  </span>
                </button>

                <div v-show="isOpen(nodeKey(sn.server?.id ?? -999, dn.device?.id ?? -1, b.kind))" class="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
                        <TableHead class="w-14 text-[10px] uppercase tracking-[0.18em] font-bold pl-20">IOA</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">Variable</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">JSON key</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">IEC 104</TableHead>
                        <TableHead class="text-right text-[10px] uppercase tracking-[0.18em] font-bold">Scale</TableHead>
                        <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">Unit</TableHead>
                        <TableHead class="w-16 text-[10px] uppercase tracking-[0.18em] font-bold">On</TableHead>
                        <TableHead class="w-20 text-right text-[10px] uppercase tracking-[0.18em] font-bold">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      <TableRow v-for="m in b.mappings" :key="m.id" class="data-row border-b border-border/40 last:border-b-0">
                        <TableCell class="font-mono font-bold text-[color:var(--epm-bosque)] pl-20">{{ m.ioa }}</TableCell>
                        <TableCell>
                          <div class="font-medium text-xs">{{ m.variable_type || m.device_name || '—' }}</div>
                          <div v-if="m.characteristic" class="text-[10px] text-muted-foreground">{{ m.characteristic }}</div>
                        </TableCell>
                        <TableCell class="font-mono text-xs">{{ m.json_key }}</TableCell>
                        <TableCell>
                          <span class="inline-flex items-center rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)]">
                            {{ m.iec104_type }}
                          </span>
                        </TableCell>
                        <TableCell class="text-right font-mono text-xs tabular">{{ m.scale }}</TableCell>
                        <TableCell class="text-xs text-muted-foreground">{{ m.unit }}</TableCell>
                        <TableCell>
                          <Switch :model-value="m.enabled" @update:model-value="() => toggleEnabled(m)" />
                        </TableCell>
                        <TableCell class="text-right whitespace-nowrap">
                          <Button variant="ghost" size="icon" @click="openEdit(m)"><Pencil class="h-4 w-4" /></Button>
                          <Button variant="ghost" size="icon" @click="del(m)" class="text-[color:var(--destructive)]"><Trash2 class="h-4 w-4" /></Button>
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

    <!-- FLAT VIEW -->
    <Card v-else class="card-soft">
      <CardHeader>
        <CardTitle>Mappings</CardTitle>
        <CardDescription>
          {{ filteredMappings.length }} of {{ mappings.length }} mapping{{ mappings.length === 1 ? '' : 's' }} across
          {{ topics.length }} topic{{ topics.length === 1 ? '' : 's' }}.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div v-if="!topics.length" class="text-sm text-muted-foreground py-4">
          No topics configured. Add devices + topics first under <strong>Devices &amp; Topics</strong>.
        </div>
        <div v-else class="overflow-x-auto -mx-6 px-6">
          <Table>
            <TableHeader>
              <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
                <TableHead class="w-14 text-[10px] uppercase tracking-[0.2em] font-bold">IOA</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Topic</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Device</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Variable type</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">JSON key</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">IEC 104</TableHead>
                <TableHead class="text-right text-[10px] uppercase tracking-[0.2em] font-bold">Scale</TableHead>
                <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Unit</TableHead>
                <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">On</TableHead>
                <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.2em] font-bold">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="m in filteredMappings" :key="m.id" class="data-row border-b border-border/60">
                <TableCell class="font-mono font-bold text-[color:var(--epm-bosque)]">{{ m.ioa }}</TableCell>
                <TableCell class="font-mono text-xs truncate max-w-[260px]">
                  {{ topicById[m.topic_id]?.topic ?? '—' }}
                </TableCell>
                <TableCell class="font-semibold">{{ m.device_name }}</TableCell>
                <TableCell class="text-muted-foreground text-xs">{{ m.variable_type }}</TableCell>
                <TableCell class="font-mono text-xs">{{ m.json_key }}</TableCell>
                <TableCell>
                  <span class="inline-flex items-center rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)]">
                    {{ m.iec104_type }}
                  </span>
                </TableCell>
                <TableCell class="text-right font-mono text-xs tabular">{{ m.scale }}</TableCell>
                <TableCell class="text-xs text-muted-foreground">{{ m.unit }}</TableCell>
                <TableCell>
                  <Switch :model-value="m.enabled" @update:model-value="() => toggleEnabled(m)" />
                </TableCell>
                <TableCell class="text-right">
                  <Button variant="ghost" size="icon" @click="openEdit(m)"><Pencil class="h-4 w-4" /></Button>
                  <Button variant="ghost" size="icon" @click="del(m)" class="text-[color:var(--destructive)]"><Trash2 class="h-4 w-4" /></Button>
                </TableCell>
              </TableRow>
              <TableRow v-if="!filteredMappings.length">
                <TableCell colspan="10" class="text-center text-muted-foreground py-6">
                  {{ search ? 'No mappings match the current filter.' : 'No mappings yet. Click "New mapping".' }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
