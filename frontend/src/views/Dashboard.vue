<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { api, type History, type SignalMapping } from '@/api'
import { useStatus } from '@/composables/useStatus'
import StatCard from '@/components/StatCard.vue'
import StatusPill from '@/components/StatusPill.vue'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { RouterLink } from 'vue-router'
import { ArrowRight, Radio, Server, Activity, Clock, Layers, Gauge, Database } from 'lucide-vue-next'

const { status } = useStatus()

const recent = ref<History[]>([])
const mappings = ref<SignalMapping[]>([])
const mapById = computed(() => Object.fromEntries(mappings.value.map(m => [m.id, m])))

const prevMsgCount = ref<number>(0)
const msgRate = ref<number>(0)
const lastPoll = ref<number>(Date.now())

let pollTimer: ReturnType<typeof setInterval> | null = null

async function loadRecent() {
  try {
    const { data } = await api.get<History[]>('/history', { params: { limit: 14 } })
    recent.value = data ?? []
  } catch { /* ignore */ }
}
async function loadMappings() {
  try {
    const { data } = await api.get<SignalMapping[]>('/mappings')
    mappings.value = data ?? []
  } catch { /* ignore */ }
}

function tickRate() {
  if (!status.value) return
  const now = Date.now()
  const dt = (now - lastPoll.value) / 1000
  const delta = status.value.mqtt.messages - prevMsgCount.value
  if (dt > 0 && prevMsgCount.value > 0) {
    msgRate.value = Math.max(0, delta / dt)
  }
  prevMsgCount.value = status.value.mqtt.messages
  lastPoll.value = now
}

onMounted(() => {
  loadMappings()
  loadRecent()
  pollTimer = setInterval(() => { loadRecent(); tickRate() }, 2500)
})
onUnmounted(() => { if (pollTimer) clearInterval(pollTimer) })

const brokerState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  if (!status.value) return 'idle'
  return status.value.mqtt.connected ? 'ok' : 'fault'
})
const iecServers = computed(() => status.value?.iec104.servers ?? [])
const iecState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  if (!status.value) return 'idle'
  const list = iecServers.value
  if (!list.length) return 'idle'
  const running = list.filter(s => s.running).length
  if (running === 0) return 'fault'
  if (running < list.filter(s => s.enabled).length) return 'warn'
  return 'ok'
})
const iecSummary = computed(() => {
  const list = iecServers.value
  const ip = status.value?.iec104.listen_ip || '0.0.0.0'
  if (!list.length) return ip
  if (list.length === 1) return `${ip}:${list[0].port}`
  const running = list.filter(s => s.running).length
  return `${ip} · ${running}/${list.length} running`
})
function fmt(ts: string) { try { return new Date(ts).toLocaleTimeString() } catch { return ts } }
function fmtAgo(ts?: string | null) {
  if (!ts) return '—'
  const diff = (Date.now() - new Date(ts).getTime()) / 1000
  if (diff < 60) return `${Math.round(diff)}s ago`
  if (diff < 3600) return `${Math.round(diff / 60)}m ago`
  return `${Math.round(diff / 3600)}h ago`
}
function fmtNumber(n?: number | null) {
  if (n === null || n === undefined) return '—'
  return n.toLocaleString()
}
</script>

<template>
  <div class="p-8 space-y-8">
    <!-- HERO -->
    <section class="card-soft">
      <div class="grid grid-cols-12 gap-6 p-8">
        <div class="col-span-12 md:col-span-8">
          <div class="flex items-center gap-3 mb-3">
            <div class="h-1 w-10 bg-[color:var(--epm-bosque)]" />
            <span class="text-[11px] uppercase tracking-[0.26em] text-[color:var(--epm-bosque)] font-bold">
              Operator console · live
            </span>
          </div>
          <h1 class="max-w-2xl">
            Telemetry flowing from
            <span class="text-[color:var(--epm-bosque)]">MQTT</span>
            into
            <span class="text-[color:var(--epm-citrico-deep)]" style="color: var(--epm-citrico-deep)">IEC&nbsp;60870-5-104</span>.
          </h1>
          <p class="mt-4 max-w-xl text-sm text-muted-foreground leading-relaxed">
            Broker topics map to IOAs, values stream through the gateway and land in the
            time-series log — every sample timestamped, every point cached.
          </p>
          <div class="mt-6 flex flex-wrap items-center gap-2">
            <RouterLink to="/mappings">
              <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-5">
                <Layers class="h-4 w-4 mr-2" /> Open mappings
              </Button>
            </RouterLink>
            <RouterLink to="/mqtt">
              <Button variant="outline" class="rounded-sm border-[color:var(--epm-bosque)] text-[color:var(--epm-bosque)]">
                Broker settings
              </Button>
            </RouterLink>
          </div>
        </div>

        <div class="col-span-12 md:col-span-4 flex md:justify-end md:items-end">
          <div class="flex flex-col gap-2 items-start md:items-end">
            <StatusPill label="MQTT" :state="brokerState" :value="status?.mqtt.broker || '—'" />
            <StatusPill label="IEC 104" :state="iecState" :value="iecSummary" />
            <div class="font-mono text-[11px] text-muted-foreground mt-2">
              {{ msgRate.toFixed(2) }} msg/s · up {{ status?.uptime_seconds ?? 0 }}s
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- KPIs -->
    <section class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <StatCard
        label="Mappings"
        :value="fmtNumber(status?.mappings)"
        :hint="`${status?.topics ?? 0} topics · ${status?.devices ?? 0} devices`"
        :icon="Layers"
        accent
      />
      <StatCard
        label="Messages in"
        :value="fmtNumber(status?.mqtt.messages)"
        :hint="`${msgRate.toFixed(2)} msg/s`"
        :icon="Radio"
      />
      <StatCard
        label="IEC 104 points"
        :value="fmtNumber(status?.iec104.points)"
        hint="cached live values"
        :icon="Gauge"
      />
      <StatCard
        label="History rows"
        :value="fmtNumber(status?.history_count)"
        :hint="`last sample ${fmtAgo(status?.last_sample_at)}`"
        :icon="Database"
      />
    </section>

    <!-- Broker + IEC 104 -->
    <section class="grid grid-cols-1 lg:grid-cols-2 gap-5">
      <div class="card-soft overflow-hidden">
        <div class="flex items-center justify-between px-6 py-5 border-b border-border">
          <div class="flex items-center gap-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)]">
              <Radio class="h-4 w-4 text-[color:var(--epm-bosque)]" />
            </div>
            <div>
              <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">MQTT broker</div>
              <div class="font-mono text-sm mt-1 truncate max-w-[260px]">{{ status?.mqtt.broker || 'not configured' }}</div>
            </div>
          </div>
          <StatusPill :state="brokerState" :label="brokerState === 'ok' ? 'Connected' : 'Down'" />
        </div>
        <dl class="grid grid-cols-3 text-sm">
          <div class="px-6 py-5">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Subscribed</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2 text-[color:var(--epm-bosque)]">{{ status?.mqtt.topics ?? 0 }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Msg in</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">{{ fmtNumber(status?.mqtt.messages) }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Rate</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">
              {{ msgRate.toFixed(2) }}<span class="text-xs text-muted-foreground ml-1 font-normal">/s</span>
            </dd>
          </div>
        </dl>
        <div class="px-6 py-3 border-t border-border text-right bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
          <RouterLink to="/mqtt" class="inline-flex items-center gap-1 text-xs font-bold text-[color:var(--epm-bosque)] hover:underline">
            Broker config <ArrowRight class="h-3 w-3" />
          </RouterLink>
        </div>
      </div>

      <div class="card-soft overflow-hidden">
        <div class="flex items-center justify-between px-6 py-5 border-b border-border">
          <div class="flex items-center gap-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-bosque)_18%,transparent)]">
              <Server class="h-4 w-4 text-[color:var(--epm-bosque)]" />
            </div>
            <div>
              <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">IEC 60870-5-104</div>
              <div class="font-mono text-sm mt-1">
                {{ iecSummary }}
              </div>
            </div>
          </div>
          <StatusPill :state="iecState" :label="iecState === 'ok' ? 'Running' : iecState === 'warn' ? 'Partial' : iecState === 'fault' ? 'Stopped' : 'Idle'" />
        </div>
        <dl class="grid grid-cols-3 text-sm">
          <div class="px-6 py-5">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Endpoints</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">{{ iecServers.length }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Points</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2 text-[color:var(--epm-bosque)]">{{ status?.iec104.points ?? 0 }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Mappings</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">{{ status?.mappings ?? 0 }}</dd>
          </div>
        </dl>
        <div class="px-6 py-3 border-t border-border text-right bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)]">
          <RouterLink to="/iec104" class="inline-flex items-center gap-1 text-xs font-bold text-[color:var(--epm-bosque)] hover:underline">
            Server config <ArrowRight class="h-3 w-3" />
          </RouterLink>
        </div>
      </div>
    </section>

    <!-- Live feed -->
    <section class="card-soft overflow-hidden">
      <div class="flex items-center justify-between px-6 py-5 border-b border-border">
        <div class="flex items-center gap-3">
          <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_28%,transparent)]">
            <Activity class="h-4 w-4 text-[color:var(--epm-bosque)]" />
          </div>
          <div>
            <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Live feed</div>
            <div class="font-sans font-extrabold text-lg leading-none mt-1 tracking-tight">Recent samples</div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <span class="font-mono text-[11px] text-muted-foreground inline-flex items-center gap-1">
            <Clock class="h-3 w-3" /> every 2.5s
          </span>
          <RouterLink to="/history">
            <Button variant="outline" size="sm" class="rounded-sm">Open history</Button>
          </RouterLink>
        </div>
      </div>
      <div class="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow class="border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_7%,transparent)]">
              <TableHead class="w-28 text-[10px] uppercase tracking-[0.2em] font-bold">Time</TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">IOA</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Signal</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Type</TableHead>
              <TableHead class="text-right text-[10px] uppercase tracking-[0.2em] font-bold">Value</TableHead>
              <TableHead class="w-16 text-[10px] uppercase tracking-[0.2em] font-bold">Unit</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="r in recent" :key="r.id" class="data-row border-b border-border/60">
              <TableCell class="font-mono text-xs text-muted-foreground">{{ fmt(r.timestamp) }}</TableCell>
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ mapById[r.mapping_id]?.ioa ?? '—' }}</TableCell>
              <TableCell class="font-mono text-xs">{{ r.signal_key }}</TableCell>
              <TableCell>
                <span class="inline-flex items-center rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)]">
                  {{ mapById[r.mapping_id]?.iec104_type ?? '—' }}
                </span>
              </TableCell>
              <TableCell class="text-right font-mono font-bold tabular">{{ r.value }}</TableCell>
              <TableCell class="text-muted-foreground text-xs">{{ mapById[r.mapping_id]?.unit }}</TableCell>
            </TableRow>
            <TableRow v-if="!recent.length">
              <TableCell colspan="6" class="text-center text-muted-foreground py-12">
                No samples yet. Publish to a subscribed topic to see data here.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </section>
  </div>
</template>
