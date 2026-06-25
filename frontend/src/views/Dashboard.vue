<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { api, type History, type SignalMapping } from '@/api'
import { useStatus, type IEC104ServerStatus, type SystemHealth } from '@/composables/useStatus'
import { t } from '@/i18n'
import StatCard from '@/components/StatCard.vue'
import StatusPill from '@/components/StatusPill.vue'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { RouterLink } from 'vue-router'
import {
  ArrowRight, Radio, Server, Activity, Clock, Layers, Gauge, Database,
  AlertTriangle, WifiOff,
} from 'lucide-vue-next'

const { status, systemHealth } = useStatus()

// ── History feed ──────────────────────────────────────────────────────────────
const recent = ref<History[]>([])
const mappings = ref<SignalMapping[]>([])
const mapById = computed(() => Object.fromEntries(mappings.value.map(m => [m.id, m])))

// ── Msg rate tracking + sparkline ─────────────────────────────────────────────
const prevMsgCount = ref(0)
const msgRate = ref(0)
const lastPoll = ref(Date.now())
const MAX_HISTORY = 30
const rateHistory = ref<number[]>([])

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

  rateHistory.value.push(msgRate.value)
  if (rateHistory.value.length > MAX_HISTORY) rateHistory.value.shift()
}

// SVG sparkline path (line) from rate history
const sparklinePath = computed(() => {
  const h = rateHistory.value
  if (h.length < 2) return ''
  const max = Math.max(...h, 0.001)
  const W = 88, H = 22
  return h.map((v, i) => {
    const x = (i / (h.length - 1)) * W
    const y = H - (v / max) * H * 0.88 + H * 0.06
    return `${i === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`
  }).join(' ')
})

// SVG fill area below the sparkline
const sparkliFill = computed(() => {
  const h = rateHistory.value
  if (h.length < 2) return ''
  const max = Math.max(...h, 0.001)
  const W = 88, H = 22
  const pts = h.map((v, i) => {
    const x = (i / (h.length - 1)) * W
    const y = H - (v / max) * H * 0.88 + H * 0.06
    return `${x.toFixed(1)} ${y.toFixed(1)}`
  })
  return `M ${pts[0]} ${pts.slice(1).map(p => `L ${p}`).join(' ')} L ${W} ${H} L 0 ${H} Z`
})

// ── Previous-value tracking for KPI trend arrows ──────────────────────────────
const prevHistoryCount = ref<number>(0)

watch(() => status.value?.history_count, (_n, o) => {
  if (o !== undefined && o !== null) prevHistoryCount.value = o
})

const historyTrend = computed(() => {
  const curr = status.value?.history_count ?? 0
  const prev = prevHistoryCount.value
  if (!prev) return ''
  return curr > prev ? ' ↑' : curr < prev ? ' ↓' : ''
})

// ── Polling ───────────────────────────────────────────────────────────────────
let pollTimer: ReturnType<typeof setInterval> | null = null

async function loadRecent() {
  try {
    const { data } = await api.get<History[]>('/history', { params: { limit: 14 } })
    recent.value = data ?? []
  } catch { /* ignore — stale feed is acceptable */ }
}
async function loadMappings() {
  try {
    const { data } = await api.get<SignalMapping[]>('/mappings')
    mappings.value = data ?? []
  } catch { /* ignore */ }
}

onMounted(() => {
  loadMappings()
  loadRecent()
  pollTimer = setInterval(() => { loadRecent(); tickRate() }, 2500)
})
onUnmounted(() => { if (pollTimer) clearInterval(pollTimer) })

// ── IEC-104 derived state ─────────────────────────────────────────────────────
const iecServers = computed(() => status.value?.iec104.servers ?? [])

const brokerState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  if (!status.value) return 'idle'
  return status.value.mqtt.connected ? 'ok' : 'fault'
})

const iecState = computed<'ok' | 'warn' | 'fault' | 'idle' | 'wait'>(() => {
  if (!status.value) return 'idle'
  const list = iecServers.value
  if (!list.length) return 'idle'
  const enabled = list.filter(s => s.enabled)
  if (!enabled.length) return 'idle'
  const running = enabled.filter(s => s.running)
  if (!running.length) return 'fault'
  if (running.length < enabled.length) return 'warn'
  const activated = list.reduce((n, s) => n + s.activated, 0)
  const tcp = list.reduce((n, s) => n + s.clients, 0)
  if (activated > 0) return 'ok'
  if (tcp > 0) return 'warn'
  return 'wait'
})

const iecStateLabel = computed(() => {
  switch (iecState.value) {
    case 'ok':   return t.status.protocolUp
    case 'warn': return t.status.tcpOnly
    case 'fault': return t.status.bindFailed
    case 'wait': return t.status.listening
    default:     return t.status.idle
  }
})

const iecSummary = computed(() => {
  const list = iecServers.value
  const ip = status.value?.iec104.listen_ip || '0.0.0.0'
  if (!list.length) return ip
  if (list.length === 1) return `${ip}:${list[0].port}`
  const active = list.reduce((n, s) => n + (s.activated > 0 ? 1 : 0), 0)
  return `${ip} · ${active}/${list.filter(s => s.enabled).length} protocolo activo`
})

// ── Per-server chip classes ───────────────────────────────────────────────────
function serverPillState(s: IEC104ServerStatus): 'ok' | 'warn' | 'fault' | 'wait' | 'idle' {
  if (!s.enabled) return 'idle'
  if (!s.running) return 'fault'
  if (s.activated > 0) return 'ok'
  if (s.clients > 0) return 'warn'
  return 'wait'
}

const serverChipColors: Record<string, string> = {
  ok:    'border-[color:color-mix(in_srgb,var(--signal-ok)_35%,transparent)] bg-[color:color-mix(in_srgb,var(--signal-ok)_10%,transparent)]',
  warn:  'border-[color:color-mix(in_srgb,var(--signal-warn)_35%,transparent)] bg-[color:color-mix(in_srgb,var(--signal-warn)_10%,transparent)]',
  fault: 'border-[color:color-mix(in_srgb,var(--signal-fault)_35%,transparent)] bg-[color:color-mix(in_srgb,var(--signal-fault)_10%,transparent)]',
  wait:  'border-[color:color-mix(in_srgb,var(--signal-wait)_38%,transparent)] bg-[color:color-mix(in_srgb,var(--signal-wait)_10%,transparent)]',
  idle:  'border-border bg-card',
}
const serverDotColors: Record<string, string> = {
  ok:    'bg-[color:var(--signal-ok)]',
  warn:  'bg-[color:var(--signal-warn)]',
  fault: 'bg-[color:var(--signal-fault)]',
  wait:  'bg-[color:var(--signal-wait)]',
  idle:  'bg-muted-foreground',
}

// ── Health banner ─────────────────────────────────────────────────────────────
// Shown only when systemHealth is not 'ok'. Provides a concise operator alert
// at the top of the dashboard with a specific, actionable message.
interface HealthBanner {
  severity: 'fault' | 'warn'
  message: string
}
const healthBanner = computed<HealthBanner | null>(() => {
  if (!status.value) return null
  const h = systemHealth.value as SystemHealth
  if (h === 'ok' || h === 'unknown') return null
  if (h === 'fault') {
    if (!status.value.mqtt.connected) {
      return { severity: 'fault', message: 'Broker MQTT desconectado — la telemetría MQTT/Sparkplug B no fluye hacia IEC-104.' }
    }
    return { severity: 'fault', message: 'Uno o más servidores IEC-104 habilitados no pudieron hacer bind — masters SCADA sin conexión.' }
  }
  return { severity: 'warn', message: 'Flota IEC-104 parcialmente activa — revisar listeners en la sección de configuración.' }
})

// ── Formatting helpers ────────────────────────────────────────────────────────
function fmt(ts: string) {
  try { return new Date(ts).toLocaleTimeString() } catch { return ts }
}
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
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8">

    <!-- ── SYSTEM HEALTH BANNER ────────────────────────────────────────────── -->
    <!-- Only shown when a subsystem is degraded. Stays compact to not dominate
         the dashboard — the operator's first instinct is to scan data, not read. -->
    <Transition name="fade">
      <div
        v-if="healthBanner"
        class="flex items-start gap-3 rounded-sm border px-4 py-3"
        :class="healthBanner.severity === 'fault'
          ? 'border-[color:color-mix(in_srgb,var(--signal-fault)_40%,transparent)] bg-[color:color-mix(in_srgb,var(--signal-fault)_8%,transparent)]'
          : 'border-[color:color-mix(in_srgb,var(--signal-warn)_40%,transparent)] bg-[color:color-mix(in_srgb,var(--signal-warn)_8%,transparent)]'"
        role="alert"
        :aria-live="healthBanner.severity === 'fault' ? 'assertive' : 'polite'"
      >
        <WifiOff
          v-if="healthBanner.severity === 'fault'"
          class="h-4 w-4 mt-0.5 shrink-0 text-[color:var(--signal-fault)]"
          aria-hidden="true"
        />
        <AlertTriangle
          v-else
          class="h-4 w-4 mt-0.5 shrink-0 text-[color:var(--signal-warn)]"
          aria-hidden="true"
        />
        <p
          class="text-sm font-semibold"
          :class="healthBanner.severity === 'fault' ? 'text-[color:var(--signal-fault)]' : 'text-[color:var(--signal-warn)]'"
        >
          {{ healthBanner.message }}
        </p>
      </div>
    </Transition>

    <!-- ── HERO ────────────────────────────────────────────────────────────── -->
    <section class="card-soft" aria-label="Resumen del gateway">
      <div class="grid grid-cols-12 gap-6 p-5 sm:p-6 lg:p-8">
        <div class="col-span-12 md:col-span-8">
          <div class="flex items-center gap-3 mb-3">
            <div class="h-1 w-10 bg-[color:var(--epm-bosque)]" aria-hidden="true" />
            <span class="text-[11px] uppercase tracking-[0.26em] text-[color:var(--epm-bosque)] font-bold">
              {{ t.dashboard.subtitle }}
            </span>
          </div>
          <h1>
            {{ t.dashboard.heroTitle }}
            <span class="text-[color:var(--epm-bosque)]">MQTT</span>
            {{ t.dashboard.heroMid }}
            <span style="color: var(--epm-citrico-deep)">IEC&nbsp;60870-5-104</span>.
          </h1>
          <p class="mt-4 max-w-xl text-sm text-muted-foreground leading-relaxed">
            {{ t.dashboard.heroDesc }}
          </p>
          <div class="mt-6 flex flex-wrap items-center gap-2">
            <RouterLink to="/mappings">
              <Button class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-5">
                <Layers class="h-4 w-4 mr-2" aria-hidden="true" />
                {{ t.dashboard.openMappings }}
              </Button>
            </RouterLink>
            <RouterLink to="/mqtt">
              <Button variant="outline" class="rounded-sm border-[color:var(--epm-bosque)] text-[color:var(--epm-bosque)]">
                {{ t.dashboard.brokerSettings }}
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

    <!-- ── KPI CARDS ───────────────────────────────────────────────────────── -->
    <section class="grid grid-cols-2 lg:grid-cols-4 gap-4" aria-label="Indicadores clave">
      <StatCard
        :label="t.dashboard.mappings"
        :value="fmtNumber(status?.mappings)"
        :hint="`${status?.topics ?? 0} tópicos · ${status?.devices ?? 0} dispositivos`"
        :icon="Layers"
        accent
      />
      <StatCard
        :label="t.dashboard.messagesIn"
        :value="fmtNumber(status?.mqtt.messages)"
        :hint="`${msgRate.toFixed(2)} msg/s en vivo`"
        :icon="Radio"
      />
      <StatCard
        :label="t.dashboard.iec104Points"
        :value="fmtNumber(status?.iec104.points)"
        :hint="t.dashboard.cachedValues"
        :icon="Gauge"
      />
      <StatCard
        :label="t.dashboard.historyRows"
        :value="fmtNumber(status?.history_count)"
        :hint="`${t.dashboard.lastSample} ${fmtAgo(status?.last_sample_at)}${historyTrend}`"
        :icon="Database"
      />
    </section>

    <!-- ── PROTOCOL CARDS ──────────────────────────────────────────────────── -->
    <section class="grid grid-cols-1 lg:grid-cols-2 gap-5" aria-label="Estado de protocolos">

      <!-- MQTT card -->
      <div class="card-soft overflow-hidden">
        <div class="flex items-center justify-between px-6 py-5 border-b border-border">
          <div class="flex items-center gap-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)]">
              <Radio class="h-4 w-4 text-[color:var(--epm-bosque)]" aria-hidden="true" />
            </div>
            <div>
              <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.dashboard.mqttBroker }}</div>
              <div class="font-mono text-sm mt-1 truncate max-w-[260px]">{{ status?.mqtt.broker || t.common.notConfigured }}</div>
            </div>
          </div>
          <StatusPill :state="brokerState" :label="brokerState === 'ok' ? t.status.connected : t.status.down" />
        </div>

        <dl class="grid grid-cols-3 text-sm">
          <div class="px-6 py-5">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Suscritos</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2 text-[color:var(--epm-bosque)]">{{ status?.mqtt.topics ?? 0 }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Msgs recibidos</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">{{ fmtNumber(status?.mqtt.messages) }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Velocidad</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">
              {{ msgRate.toFixed(2) }}<span class="text-xs text-muted-foreground ml-1 font-normal">/s</span>
            </dd>
          </div>
        </dl>

        <!-- Sparkline: msg rate over last ~75s (30 samples × 2.5s) -->
        <div
          v-if="sparklinePath"
          class="px-6 pb-3 flex items-center gap-3"
          aria-hidden="true"
        >
          <span class="text-[10px] uppercase tracking-[0.18em] text-muted-foreground font-bold shrink-0">Live</span>
          <svg
            width="88" height="22"
            class="shrink-0 opacity-70"
            aria-label="Gráfico de velocidad de mensajes"
          >
            <path :d="sparkliFill" fill="var(--epm-bosque)" fill-opacity="0.18" />
            <path
              :d="sparklinePath"
              fill="none"
              stroke="var(--epm-bosque)"
              stroke-width="1.5"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </div>

        <div class="px-6 py-3 border-t border-border text-right bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
          <RouterLink to="/mqtt" class="inline-flex items-center gap-1 text-xs font-bold text-[color:var(--epm-bosque)] hover:underline">
            {{ t.dashboard.brokerConfig }} <ArrowRight class="h-3 w-3" aria-hidden="true" />
          </RouterLink>
        </div>
      </div>

      <!-- IEC-104 card -->
      <div class="card-soft overflow-hidden">
        <div class="flex items-center justify-between px-6 py-5 border-b border-border">
          <div class="flex items-center gap-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-bosque)_18%,transparent)]">
              <Server class="h-4 w-4 text-[color:var(--epm-bosque)]" aria-hidden="true" />
            </div>
            <div>
              <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">IEC&nbsp;60870-5-104</div>
              <div class="font-mono text-sm mt-1">{{ iecSummary }}</div>
            </div>
          </div>
          <StatusPill :state="iecState" :label="iecStateLabel" />
        </div>

        <dl class="grid grid-cols-3 text-sm">
          <div class="px-6 py-5">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.dashboard.endpoints }}</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">{{ iecServers.length }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.dashboard.points }}</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2 text-[color:var(--epm-bosque)]">{{ status?.iec104.points ?? 0 }}</dd>
          </div>
          <div class="px-6 py-5 border-l border-border">
            <dt class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.dashboard.mappings }}</dt>
            <dd class="font-mono text-2xl font-bold tabular mt-2">{{ status?.mappings ?? 0 }}</dd>
          </div>
        </dl>

        <!-- Per-server fleet chips: one chip per configured server showing
             real-time link state. Green = STARTDT active, amber = TCP only,
             blue = listening, red = bind failed, muted = disabled. -->
        <div
          v-if="iecServers.length > 0"
          class="px-6 pb-4 pt-1 flex flex-wrap gap-2"
          aria-label="Estado de servidores IEC-104"
        >
          <div
            v-for="srv in iecServers"
            :key="srv.id"
            class="inline-flex items-center gap-1.5 rounded-sm border px-2.5 py-1 font-mono text-[11px] transition-colors"
            :class="serverChipColors[serverPillState(srv)]"
            :title="`${srv.name || `#${srv.id}`} · puerto ${srv.port} · ASDU ${srv.asdu_addr} · ${srv.activated} activados / ${srv.clients} TCP`"
          >
            <span
              class="h-1.5 w-1.5 rounded-full shrink-0"
              :class="serverDotColors[serverPillState(srv)]"
              aria-hidden="true"
            />
            {{ srv.name || `#${srv.id}` }}
            <span class="text-muted-foreground">:{{ srv.port }}</span>
          </div>
        </div>

        <div class="px-6 py-3 border-t border-border text-right bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)]">
          <RouterLink to="/iec104" class="inline-flex items-center gap-1 text-xs font-bold text-[color:var(--epm-bosque)] hover:underline">
            {{ t.dashboard.serverConfig }} <ArrowRight class="h-3 w-3" aria-hidden="true" />
          </RouterLink>
        </div>
      </div>
    </section>

    <!-- ── LIVE FEED ────────────────────────────────────────────────────────── -->
    <section class="card-soft overflow-hidden" aria-label="Feed en vivo de muestras">
      <div class="flex items-center justify-between px-6 py-5 border-b border-border">
        <div class="flex items-center gap-3">
          <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:color-mix(in_srgb,var(--epm-citrico)_28%,transparent)]">
            <Activity class="h-4 w-4 text-[color:var(--epm-bosque)]" aria-hidden="true" />
          </div>
          <div>
            <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.dashboard.liveFeed }}</div>
            <div class="font-sans font-extrabold text-lg leading-none mt-1 tracking-tight">{{ t.dashboard.recentSamples }}</div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <span class="font-mono text-[11px] text-muted-foreground inline-flex items-center gap-1">
            <Clock class="h-3 w-3" aria-hidden="true" /> {{ t.dashboard.every25s }}
          </span>
          <RouterLink to="/history">
            <Button variant="outline" size="sm" class="rounded-sm">{{ t.dashboard.openHistory }}</Button>
          </RouterLink>
        </div>
      </div>
      <div class="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow class="border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_7%,transparent)]">
              <TableHead class="w-28 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.dashboard.cols.time }}</TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.dashboard.cols.ioa }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.dashboard.cols.signal }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.dashboard.cols.type }}</TableHead>
              <TableHead class="text-right text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.dashboard.cols.value }}</TableHead>
              <TableHead class="w-16 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.dashboard.cols.unit }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="r in recent" :key="r.id" class="data-row border-b border-border/60">
              <TableCell class="font-mono text-xs text-muted-foreground">{{ fmt(r.timestamp) }}</TableCell>
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ mapById[r.mapping_id]?.ioa ?? '—' }}</TableCell>
              <TableCell class="font-mono text-xs">{{ r.signal_path }}</TableCell>
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
                {{ t.dashboard.noSamples }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </section>

  </div>
</template>
