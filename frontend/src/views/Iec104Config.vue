<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { toast } from 'vue-sonner'
import { api, type IEC104Server } from '@/api'
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
import { Server, Save, RefreshCw, Plus, Pencil, Trash2 } from 'lucide-vue-next'

const { status } = useStatus()

const servers = ref<IEC104Server[]>([])
const loading = ref(false)

function empty(): IEC104Server {
  return {
    id: 0, name: '', listen_addr: '0.0.0.0', port: 2404, asdu_addr: 1,
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
  // Bump port + ASDU to avoid collision with existing rows.
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

function validate(): string | null {
  if (!editing.listen_addr.trim()) return 'Listen address required'
  if (!Number.isInteger(editing.port) || editing.port <= 0 || editing.port > 65535) return 'Port must be 1..65535'
  if (!Number.isInteger(editing.asdu_addr) || editing.asdu_addr <= 0) return 'ASDU addr must be positive'
  const dup = servers.value.find(
    s => s.listen_addr === editing.listen_addr && s.port === editing.port && s.id !== editing.id,
  )
  if (dup) return `${editing.listen_addr}:${editing.port} already used by #${dup.id}`
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
  if (!confirm(`Delete IEC-104 server "${s.name}" (${s.listen_addr}:${s.port})?`)) return
  try {
    await api.delete(`/iec104-servers/${s.id}`)
    await reload()
    toast.success('Deleted')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

onMounted(reload)

const fleetState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  const list = status.value?.iec104.servers ?? []
  if (!list.length) return 'idle'
  const enabled = list.filter(s => s.enabled)
  const running = list.filter(s => s.running)
  if (!enabled.length) return 'idle'
  if (!running.length) return 'fault'
  if (running.length < enabled.length) return 'warn'
  return 'ok'
})

function runtimeOf(id: number) {
  return status.value?.iec104.servers.find(s => s.id === id)
}
</script>

<template>
  <div class="p-8 space-y-8 max-w-6xl">
    <!-- Hero -->
    <section class="card-soft overflow-hidden relative">
      <div class="relative grid grid-cols-12 gap-6 p-8">
        <div class="col-span-12 md:col-span-8">
          <div class="flex items-center gap-3 mb-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-bosque)] text-white">
              <Server class="h-5 w-5" />
            </div>
            <span class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">
              IEC 60870-5-104 · passive fleet
            </span>
          </div>
          <h1 class="mb-2">Slave endpoints</h1>
          <div class="font-mono text-sm mt-2 text-[color:var(--epm-bosque)]">
            {{ servers.length }} configured · {{ servers.filter(s => s.enabled).length }} enabled
          </div>
          <p class="mt-3 text-sm text-muted-foreground max-w-xl">
            Each row is a passive listener. SCADA masters connect; the gateway
            never dials out. Every server exposes the same point set under its
            own Common ASDU Address, so multiple masters can coexist without
            collision.
          </p>
        </div>
        <div class="col-span-12 md:col-span-4 flex flex-col gap-3 md:items-end">
          <StatusPill :state="fleetState"
                      :label="fleetState === 'ok' ? 'All running' : fleetState === 'warn' ? 'Partial' : fleetState === 'fault' ? 'Stopped' : 'Idle'" />
          <div class="chip font-mono text-xs">
            <span class="h-2 w-2 rounded-sm bg-[color:var(--epm-citrico)]" />
            {{ status?.iec104.points ?? 0 }} points cached
          </div>
          <div class="chip font-mono text-xs">
            <span class="h-2 w-2 rounded-sm bg-[color:var(--epm-bosque)]" />
            {{ status?.iec104.clients ?? 0 }} clients connected
          </div>
        </div>
      </div>
    </section>

    <Card class="card-soft">
      <CardHeader class="flex flex-row items-center justify-between">
        <div>
          <CardTitle class="font-extrabold tracking-tight">Servers</CardTitle>
          <CardDescription>One listener per row. Each row has its own Common ASDU.</CardDescription>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="outline" @click="reload" class="rounded-sm">
            <RefreshCw class="h-4 w-4 mr-2" /> Reload
          </Button>
          <Dialog v-model:open="dialogOpen">
            <DialogTrigger as-child>
              <Button @click="openCreate"
                      class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-4">
                <Plus class="h-4 w-4 mr-1" /> New server
              </Button>
            </DialogTrigger>
            <DialogContent class="max-w-3xl">
              <DialogHeader>
                <DialogTitle>{{ isEdit ? `Edit server #${editing.id}` : 'New IEC-104 server' }}</DialogTitle>
                <DialogDescription>
                  Passive slave endpoint. SCADA masters connect to {{ editing.listen_addr || '0.0.0.0' }}:{{ editing.port || 2404 }}
                  and read the gateway under ASDU {{ editing.asdu_addr || 1 }}.
                </DialogDescription>
              </DialogHeader>

              <div class="grid grid-cols-6 gap-x-4 gap-y-3 py-2">
                <div class="col-span-6 space-y-1.5">
                  <Label>Name</Label>
                  <Input v-model="editing.name" placeholder="control-center-a" />
                </div>

                <div class="col-span-3 space-y-1.5">
                  <Label>Listen address</Label>
                  <Input v-model="editing.listen_addr" class="font-mono" placeholder="0.0.0.0" />
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>Port</Label>
                  <Input v-model.number="editing.port" type="number" />
                </div>
                <div class="col-span-2 space-y-1.5">
                  <Label>Common ASDU addr</Label>
                  <Input v-model.number="editing.asdu_addr" type="number" />
                </div>

                <div class="col-span-6 text-[10px] uppercase tracking-[0.18em] font-bold text-muted-foreground mt-2">
                  Protocol timers (IEC 60870-5-104 §5)
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>k · unacked I</Label>
                  <Input v-model.number="editing.k" type="number" />
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>w · ack win</Label>
                  <Input v-model.number="editing.w" type="number" />
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>t0 · connect</Label>
                  <Input v-model.number="editing.t0" type="number" />
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>t1 · send</Label>
                  <Input v-model.number="editing.t1" type="number" />
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>t2 · ack</Label>
                  <Input v-model.number="editing.t2" type="number" />
                </div>
                <div class="col-span-1 space-y-1.5">
                  <Label>t3 · test</Label>
                  <Input v-model.number="editing.t3" type="number" />
                </div>

                <div class="col-span-6 flex items-center gap-3 pt-2">
                  <Switch id="en" v-model="editing.enabled" />
                  <Label for="en">Enabled</Label>
                </div>
              </div>

              <DialogFooter>
                <Button variant="outline" @click="dialogOpen = false" class="rounded-sm">Cancel</Button>
                <Button :disabled="saving" @click="save"
                        class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
                  <Save class="h-4 w-4 mr-2" />
                  {{ saving ? 'Saving…' : (isEdit ? 'Save changes' : 'Create') }}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Name</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Endpoint</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">ASDU</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Timers (k / w / t1 / t2 / t3)</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Clients</TableHead>
              <TableHead class="w-24 text-[10px] uppercase tracking-[0.2em] font-bold">Status</TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">On</TableHead>
              <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.2em] font-bold">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="s in servers" :key="s.id" class="data-row border-b border-border/60">
              <TableCell class="font-semibold">{{ s.name || '—' }}</TableCell>
              <TableCell class="font-mono text-xs">{{ s.listen_addr }}:{{ s.port }}</TableCell>
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ s.asdu_addr }}</TableCell>
              <TableCell class="font-mono text-xs text-muted-foreground">
                {{ s.k }} / {{ s.w }} / {{ s.t1 }} / {{ s.t2 }} / {{ s.t3 }}
              </TableCell>
              <TableCell class="font-mono text-xs">{{ runtimeOf(s.id)?.clients ?? 0 }}</TableCell>
              <TableCell>
                <StatusPill
                  :state="runtimeOf(s.id)?.running ? 'ok' : (s.enabled ? 'fault' : 'idle')"
                  :label="runtimeOf(s.id)?.running ? 'Running' : (s.enabled ? 'Stopped' : 'Off')"
                />
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
                No servers. Click "New server" to add one.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
