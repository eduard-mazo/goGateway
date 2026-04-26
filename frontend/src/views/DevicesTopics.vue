<script setup lang="ts">
import { ref, onMounted, computed, reactive, watch } from 'vue'
import { toast } from 'vue-sonner'
import { api, type Device, type Topic, type IEC104Server } from '@/api'
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
  Trash2, Cpu, Plus, Folder, FolderOpen, ChevronRight, Search, Pencil, Radio,
  Server as ServerIcon, ChevronsDownUp, ChevronsUpDown,
} from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

const devices = ref<Device[]>([])
const topics = ref<Topic[]>([])
const servers = ref<IEC104Server[]>([])
const loading = ref(false)
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
    title: 'Delete device',
    message: `Cascades to ${topicCount} topic${topicCount === 1 ? '' : 's'} and every signal mapping under them. Cannot be undone.`,
    detail: `${d.name}${d.description ? ' — ' + d.description : ''}`,
    variant: 'danger',
    confirmText: 'Delete device',
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

interface TopicDraft { id: number; device_id: number; topic: string; qos: number; enabled: boolean }
const topicDialog = ref(false)
const editingTopic = reactive<TopicDraft>({ id: 0, device_id: 0, topic: '', qos: 0, enabled: true })
const isEditTopic = computed(() => editingTopic.id > 0)

function openCreateTopic(deviceId: number) {
  Object.assign(editingTopic, { id: 0, device_id: deviceId, topic: '', qos: 0, enabled: true })
  topicDialog.value = true
}
function openEditTopic(t: Topic) {
  Object.assign(editingTopic, t)
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
    title: 'Delete topic',
    message: 'All signal mappings under this topic cascade. Cannot be undone.',
    detail: `${t.topic} · QoS ${t.qos}`,
    variant: 'danger',
    confirmText: 'Delete topic',
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

onMounted(reload)
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 max-w-5xl">
    <!-- Hero -->
    <div class="card-soft overflow-hidden relative">
      <div class="relative flex items-start gap-4 p-5 sm:p-6 lg:p-8">
        <div class="grid place-items-center w-11 h-11 rounded-sm bg-[color:var(--epm-bosque)] text-white shrink-0">
          <Cpu class="h-5 w-5" />
        </div>
        <div class="min-w-0">
          <div class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">Infrastructure</div>
          <h1 class="mt-1 mb-1">Devices &amp; topics</h1>
          <p class="text-sm text-muted-foreground">
            {{ servers.length }} server{{ servers.length === 1 ? '' : 's' }} ·
            {{ devices.length }} device{{ devices.length === 1 ? '' : 's' }} ·
            {{ topics.length }} topic{{ topics.length === 1 ? '' : 's' }}
          </p>
        </div>
      </div>
    </div>

    <!-- Toolbar -->
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
      No IEC-104 servers configured. Add at least one under <strong>IEC 104</strong> first.
    </div>
    <div v-else-if="!tree.length" class="card-soft p-6 text-sm text-muted-foreground">
      Nothing matches the current filter.
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
          title="Add device under this server"
          class="text-muted-foreground hover:text-[color:var(--epm-bosque)]"
          @click="openCreateDevice(sn.server.id)"
        >
          <Plus class="h-4 w-4" />
        </Button>
      </div>

      <!-- Devices -->
      <div v-show="isOpen(`s:${sn.server?.id ?? -999}`)" class="border-t border-border bg-muted/20">
        <div v-if="!sn.devices.length" class="px-10 py-3 text-xs text-muted-foreground italic">
          No devices on this server yet. Click + to add one.
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
            <Button variant="ghost" size="icon" title="Add topic" class="text-muted-foreground hover:text-[color:var(--epm-bosque)]" @click="openCreateTopic(dn.device.id)">
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
              No topics yet. Click + on the device row to subscribe.
            </div>
            <div v-else class="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
                    <TableHead class="w-12 text-[10px] uppercase tracking-[0.18em] font-bold pl-20">ID</TableHead>
                    <TableHead class="text-[10px] uppercase tracking-[0.18em] font-bold">Topic</TableHead>
                    <TableHead class="w-16 text-[10px] uppercase tracking-[0.18em] font-bold">QoS</TableHead>
                    <TableHead class="w-16 text-[10px] uppercase tracking-[0.18em] font-bold">On</TableHead>
                    <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.18em] font-bold">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-for="t in dn.topics" :key="t.id" class="data-row border-b border-border/40 last:border-b-0">
                    <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)] pl-20">{{ t.id }}</TableCell>
                    <TableCell class="font-mono text-xs">
                      <span class="inline-flex items-center gap-2">
                        <Radio class="h-3 w-3 text-muted-foreground shrink-0" />
                        <span class="truncate">{{ t.topic }}</span>
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
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ isEditDev ? `Edit device #${editingDev.id}` : 'New device' }}</DialogTitle>
          <DialogDescription>
            A device groups MQTT topics under one logical asset (e.g. inverter, weather station).
            It is owned by one IEC-104 slave: same physical asset on two slaves is two device rows.
          </DialogDescription>
        </DialogHeader>

        <div class="grid grid-cols-1 gap-3 py-2">
          <div class="space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">IEC-104 server</Label>
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
            <Label for="dname" class="text-[11px] uppercase tracking-[0.18em] font-bold">Name</Label>
            <Input id="dname" v-model="editingDev.name" placeholder="INV_1" class="rounded-sm font-mono" @keyup.enter="saveDevice" />
          </div>
          <div class="space-y-1.5">
            <Label for="ddesc" class="text-[11px] uppercase tracking-[0.18em] font-bold">Description</Label>
            <Input id="ddesc" v-model="editingDev.description" placeholder="Inversor 1" class="rounded-sm" @keyup.enter="saveDevice" />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" @click="devDialog = false" class="rounded-sm">Cancel</Button>
          <Button @click="saveDevice" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
            {{ isEditDev ? 'Save changes' : 'Create' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Topic dialog -->
    <Dialog v-model:open="topicDialog">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ isEditTopic ? `Edit topic #${editingTopic.id}` : 'New topic' }}</DialogTitle>
          <DialogDescription>
            MQTT subscription string. Wildcards (<code>+</code>, <code>#</code>) are accepted.
          </DialogDescription>
        </DialogHeader>

        <div class="grid grid-cols-3 gap-3 py-2">
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Device</Label>
            <div class="rounded-sm border border-border bg-muted/30 px-3 py-2 text-sm font-bold">
              {{ devById[editingTopic.device_id]?.name ?? '?' }}
              <span class="ml-2 text-muted-foreground font-normal">
                {{ devById[editingTopic.device_id]?.description }}
              </span>
            </div>
          </div>
          <div class="col-span-2 space-y-1.5">
            <Label for="tstr" class="text-[11px] uppercase tracking-[0.18em] font-bold">Topic</Label>
            <Input id="tstr" v-model="editingTopic.topic" placeholder="EPM/SSFV/.../INV_1" class="rounded-sm font-mono" @keyup.enter="saveTopic" />
          </div>
          <div class="col-span-1 space-y-1.5">
            <Label for="tqos" class="text-[11px] uppercase tracking-[0.18em] font-bold">QoS</Label>
            <Input id="tqos" v-model.number="editingTopic.qos" type="number" min="0" max="2" class="rounded-sm" />
          </div>
          <div class="col-span-3 flex items-center gap-3 pt-1">
            <Switch id="ten" v-model="editingTopic.enabled" />
            <Label for="ten">Enabled</Label>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" @click="topicDialog = false" class="rounded-sm">Cancel</Button>
          <Button @click="saveTopic" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
            {{ isEditTopic ? 'Save changes' : 'Create' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
