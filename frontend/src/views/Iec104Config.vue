<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { toast } from 'vue-sonner'
import { api, type IEC104Server, type IEC104Gateway } from '@/api'
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
import { Server, Save, RefreshCw, Plus, Pencil, Trash2, Globe, ShieldCheck, ShieldAlert, Link2, PlugZap } from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

const { status } = useStatus()

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
    title: 'Delete IEC-104 server',
    message: 'The listener stops immediately. Connected SCADA masters will be dropped. This cannot be undone.',
    detail: `${s.name || 'unnamed'} · port ${s.port} · ASDU ${s.asdu_addr}`,
    variant: 'danger',
    confirmText: 'Delete server',
    challenge: s.name || `server-${s.id}`,
  })
  if (!ok) return
  try {
    await api.delete(`/iec104-servers/${s.id}`)
    await reload()
    toast.success('Deleted')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

onMounted(() => { loadGateway(); reload() })

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
    return { state: 'off', pillState: 'idle', label: 'Off', hint: 'Disabled', clients, activated }
  }
  if (!rt?.running) {
    return { state: 'bind-fail', pillState: 'fault', label: 'Bind failed', hint: 'Listener not running — check IP/port', clients, activated }
  }
  if (activated > 0) {
    return {
      state: 'linked',
      pillState: 'ok',
      label: activated === 1 ? 'Protocol up · 1' : `Protocol up · ${activated}`,
      hint: 'STARTDT activated — frames flowing',
      clients,
      activated,
    }
  }
  if (clients > 0) {
    return {
      state: 'tcp-only',
      pillState: 'warn',
      label: 'TCP only',
      hint: 'Connected, awaiting STARTDT',
      clients,
      activated,
    }
  }
  return { state: 'listening', pillState: 'wait', label: 'Listening', hint: 'Listener up, no master yet', clients, activated }
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
    case 'idle': return 'Idle'
    case 'fault': return 'Bind failed'
    case 'wait': return 'Listening'
    case 'warn': return 'Partial protocol'
    case 'ok': return 'Protocol up'
  }
  return 'Idle'
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
              IEC 60870-5-104 · passive fleet
            </span>
          </div>
          <h1 class="mb-2">Slave endpoints</h1>
          <div class="font-mono text-sm mt-2 text-[color:var(--epm-bosque)]">
            {{ servers.length }} configured · {{ servers.filter(s => s.enabled).length }} enabled
          </div>
          <p class="mt-3 text-sm text-muted-foreground max-w-xl">
            The gateway is strictly passive. SCADA masters dial in; the gateway never dials out.
            Each row below is a separate listener (port + ASDU + SCADA-IP allowlist) sharing the
            same point set, all bound on the gateway-wide listen IP defined in the next card.
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
          <CardTitle class="font-extrabold tracking-tight">Gateway listen IP</CardTitle>
        </div>
        <CardDescription>
          Local NIC IP this gateway binds for every slave endpoint. Use
          <code class="font-mono">0.0.0.0</code> to bind every interface.
          The IP must already exist on this host — the kernel will reject otherwise.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div class="flex flex-col md:flex-row md:items-end gap-4">
          <div class="space-y-1.5 flex-1 max-w-md">
            <Label>Listen IP</Label>
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
            {{ savingGateway ? 'Saving…' : 'Save listen IP' }}
          </Button>
        </div>
      </CardContent>
    </Card>

    <!-- Per-server CRUD -->
    <Card class="card-soft">
      <CardHeader class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <CardTitle class="font-extrabold tracking-tight">Servers</CardTitle>
          <CardDescription>
            One listener per row. The link column shows the live connection between
            this gateway and the SCADA master(s) for that port + ASDU.
          </CardDescription>
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

            <!--
              Dialog content: 12-column grid, comfortable spacing, no inner
              scrollbar unless the viewport really cannot fit the form.
            -->
            <DialogContent class="!max-w-xl sm:!max-w-2xl p-0 overflow-hidden">
              <div class="px-6 pt-6 pb-2 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
                <DialogHeader class="text-left space-y-1">
                  <DialogTitle class="text-lg font-extrabold tracking-tight flex items-center gap-2">
                    <Server class="h-4 w-4 text-[color:var(--epm-bosque)]" />
                    {{ isEdit ? `Edit server #${editing.id}` : 'New IEC-104 server' }}
                  </DialogTitle>
                  <DialogDescription class="text-xs">
                    Passive slave endpoint. SCADA masters connect to
                    <code class="font-mono text-[color:var(--epm-bosque)]">{{ gateway.listen_ip || '0.0.0.0' }}:{{ editing.port || 2404 }}</code>
                    and read this gateway under ASDU
                    <code class="font-mono text-[color:var(--epm-bosque)]">{{ editing.asdu_addr || 1 }}</code>.
                  </DialogDescription>
                </DialogHeader>
              </div>

              <div class="px-6 py-5 space-y-5">
                <!-- Identity row -->
                <div class="grid grid-cols-12 gap-4">
                  <div class="col-span-12 sm:col-span-6 space-y-1.5">
                    <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Name</Label>
                    <Input v-model="editing.name" placeholder="control-center-a" />
                  </div>
                  <div class="col-span-6 sm:col-span-3 space-y-1.5">
                    <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Port</Label>
                    <Input v-model.number="editing.port" type="number" class="font-mono" />
                  </div>
                  <div class="col-span-6 sm:col-span-3 space-y-1.5">
                    <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">ASDU</Label>
                    <Input v-model.number="editing.asdu_addr" type="number" class="font-mono" />
                  </div>
                </div>

                <!-- SCADA allowlist -->
                <div class="space-y-1.5">
                  <Label class="text-[11px] uppercase tracking-[0.18em] font-bold flex items-center gap-1.5">
                    <ShieldCheck class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" />
                    SCADA IP allowlist
                  </Label>
                  <Input v-model="editing.scada_ips" class="font-mono" placeholder="10.13.13.25, 10.117.18.23" />
                  <p class="text-xs text-muted-foreground leading-relaxed">
                    Comma-separated. Only these remote IPs may complete the TCP handshake.
                    Empty = block all (fail closed).
                  </p>
                </div>

                <!-- Protocol timers -->
                <div class="space-y-2">
                  <div class="flex items-center gap-2">
                    <div class="rule-brand flex-1" />
                    <span class="text-[10px] uppercase tracking-[0.22em] font-bold text-muted-foreground">
                      Protocol timers · IEC 60870-5-104 §5
                    </span>
                    <div class="rule-brand flex-1" />
                  </div>
                  <div class="grid grid-cols-6 gap-3">
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold">k</Label>
                      <Input v-model.number="editing.k" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">unacked I</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold">w</Label>
                      <Input v-model.number="editing.w" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">ack window</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold">t0</Label>
                      <Input v-model.number="editing.t0" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">connect</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold">t1</Label>
                      <Input v-model.number="editing.t1" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">send</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold">t2</Label>
                      <Input v-model.number="editing.t2" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">ack</p>
                    </div>
                    <div class="col-span-2 sm:col-span-1 space-y-1">
                      <Label class="text-[10px] uppercase tracking-[0.16em] font-bold">t3</Label>
                      <Input v-model.number="editing.t3" type="number" class="font-mono text-center" />
                      <p class="text-[10px] text-muted-foreground leading-tight text-center">test</p>
                    </div>
                  </div>
                </div>

                <!-- Enable -->
                <div class="flex items-center gap-3 pt-1">
                  <Switch id="en" v-model="editing.enabled" />
                  <Label for="en" class="cursor-pointer">
                    Enabled — start listener immediately on save
                  </Label>
                </div>
              </div>

              <DialogFooter class="px-6 py-4 border-t border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_5%,transparent)]">
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
        <div class="overflow-x-auto -mx-6 px-6">
        <Table>
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Name</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Port</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">ASDU</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">SCADA allowlist</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">
                <span class="inline-flex items-center gap-1"><Link2 class="h-3 w-3" /> Link</span>
              </TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">On</TableHead>
              <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.2em] font-bold">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="s in servers" :key="s.id" class="data-row border-b border-border/60">
              <TableCell class="font-semibold">{{ s.name || '—' }}</TableCell>
              <TableCell class="font-mono text-xs">{{ s.port }}</TableCell>
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ s.asdu_addr }}</TableCell>
              <TableCell>
                <div v-if="chips(s.scada_ips).length" class="flex flex-wrap gap-1">
                  <span v-for="ip in chips(s.scada_ips)" :key="ip"
                        class="chip font-mono text-[11px]">{{ ip }}</span>
                </div>
                <span v-else class="inline-flex items-center gap-1 text-xs text-[color:var(--destructive)] font-semibold">
                  <ShieldAlert class="h-3.5 w-3.5" /> empty — blocks all
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
              <TableCell colspan="7" class="text-center text-muted-foreground py-8">
                No servers. Click "New server" to add one.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
