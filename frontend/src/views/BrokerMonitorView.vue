<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Activity, Trash2, Pause, Play, X, Info } from 'lucide-vue-next'

// ── Types ────────────────────────────────────────────────────────────────────

interface BrokerEvent {
  id: number
  at: string
  topic: string
  size: number
  kind: 'sparkplug' | 'ssfv' | 'json' | 'state'
  payload?: string
}

interface MissedSignal {
  at: string
  signal_path: string
  equipo: string
}

// ── Constants ─────────────────────────────────────────────────────────────────

const LS_KEY = 'broker-monitor-events'
const MAX_STORED = 500
const MAX_DISPLAYED = 300

// Chip kind descriptions shown in the legend
const KIND_INFO: Record<string, { label: string; color: string; desc: string }> = {
  sparkplug: {
    label: 'Sparkplug',
    color: 'bg-blue-700 text-white',
    desc: 'Trama binaria Sparkplug B (protobuf). Contiene métricas de inversores y nodos de la red. El payload es binario — no se muestra el contenido crudo.',
  },
  ssfv: {
    label: 'SSFV',
    color: 'bg-amber-600 text-white',
    desc: 'Mensaje JSON de un equipo solar reconocido en el catálogo SSFV (tbl_equipo). Sus métricas se persisten en tbl_valores.',
  },
  json: {
    label: 'JSON',
    color: 'bg-emerald-700 text-white',
    desc: 'Mensaje JSON genérico (no SSFV). Se despacha por el mapeo señal → IOA IEC-104 y se registra en el histórico.',
  },
  state: {
    label: 'State',
    color: 'bg-purple-700 text-white',
    desc: 'Mensaje STATE del broker Sparkplug B. Indica el estado de un host o aplicación (ONLINE / OFFLINE). No contiene métricas.',
  },
}

// ── State ─────────────────────────────────────────────────────────────────────

const tab = ref<'live' | 'missed'>('live')
const events = ref<BrokerEvent[]>([])
const connected = ref(false)
const paused = ref(false)
const totalSeen = ref(0)
const showLegend = ref(false)
const selectedEvent = ref<BrokerEvent | null>(null)
const missedSignals = ref<MissedSignal[]>([])
const missedLoading = ref(false)

let es: EventSource | null = null
let rateWindow: number[] = []
const eventsPerMin = ref(0)

// ── Storage ───────────────────────────────────────────────────────────────────

function loadFromStorage(): BrokerEvent[] {
  try {
    const raw = localStorage.getItem(LS_KEY)
    return raw ? (JSON.parse(raw) as BrokerEvent[]) : []
  } catch { return [] }
}

function saveToStorage(evs: BrokerEvent[]) {
  try {
    // Strip large payloads from storage to save space
    const stripped = evs.slice(-MAX_STORED).map(e => ({ ...e, payload: undefined }))
    localStorage.setItem(LS_KEY, JSON.stringify(stripped))
  } catch { /* quota exceeded */ }
}

function clearAll() {
  localStorage.removeItem(LS_KEY)
  events.value = []
  totalSeen.value = 0
  rateWindow = []
  eventsPerMin.value = 0
  selectedEvent.value = null
}

// ── Data loading ──────────────────────────────────────────────────────────────

async function loadSnapshot() {
  try {
    const r = await fetch('/api/broker/events')
    if (!r.ok) return
    const snap: BrokerEvent[] = await r.json()
    const stored = loadFromStorage()
    const seen = new Set<number>()
    const merged: BrokerEvent[] = []
    for (const ev of [...stored, ...snap]) {
      if (!seen.has(ev.id)) { seen.add(ev.id); merged.push(ev) }
    }
    merged.sort((a, b) => a.id - b.id)
    events.value = merged.slice(-MAX_DISPLAYED)
    saveToStorage(merged)
  } catch { /* server unreachable */ }
}

async function loadMissed() {
  missedLoading.value = true
  try {
    const r = await fetch('/api/ssfv/missed')
    if (!r.ok) return
    missedSignals.value = (await r.json() as MissedSignal[]).reverse()
  } catch { /* adapter not connected */ } finally {
    missedLoading.value = false
  }
}

// ── SSE connection ────────────────────────────────────────────────────────────

function connect() {
  if (es) { es.close(); es = null }
  es = new EventSource('/api/broker/stream')
  es.onopen = () => { connected.value = true }
  es.onerror = () => { connected.value = false }
  es.onmessage = (e: MessageEvent) => {
    if (paused.value) return
    try {
      const ev: BrokerEvent = JSON.parse(e.data as string)
      totalSeen.value++
      const now = Date.now()
      rateWindow.push(now)
      rateWindow = rateWindow.filter(t => now - t < 60_000)
      eventsPerMin.value = rateWindow.length
      events.value.push(ev)
      if (events.value.length > MAX_DISPLAYED) {
        events.value.splice(0, events.value.length - MAX_DISPLAYED)
      }
      saveToStorage(events.value)
    } catch { /* malformed */ }
  }
}

function disconnect() {
  es?.close(); es = null; connected.value = false
}

// ── Computed ──────────────────────────────────────────────────────────────────

const topicsSeen = computed(() => {
  const s = new Set<string>()
  events.value.forEach(e => s.add(e.topic))
  return s.size
})

// ── Topic decomposition for Sparkplug ─────────────────────────────────────────
// spBv1.0/{GroupID}/{MessageType}/{EdgeNodeID}[/{DeviceID}]
function parseSparkplugTopic(topic: string) {
  const parts = topic.split('/')
  if (parts.length < 4) return null
  return {
    namespace: parts[0],
    group: parts[1],
    type: parts[2],
    node: parts[3],
    device: parts[4] ?? null,
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtTime(at: string) {
  return new Date(at).toLocaleTimeString('es-CO', {
    hour12: false, hour: '2-digit', minute: '2-digit',
    second: '2-digit', fractionalSecondDigits: 3,
  } as Intl.DateTimeFormatOptions)
}

function fmtSize(size: number) {
  if (size >= 1024) return `${(size / 1024).toFixed(1)} kB`
  return `${size} B`
}

function prettyJson(raw?: string) {
  if (!raw) return ''
  try { return JSON.stringify(JSON.parse(raw), null, 2) }
  catch { return raw }
}

function selectEvent(ev: BrokerEvent) {
  selectedEvent.value = ev
}

function onTabChange(t: 'live' | 'missed') {
  tab.value = t
  if (t === 'missed') loadMissed()
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

onMounted(async () => {
  await loadSnapshot()
  connect()
})
onUnmounted(disconnect)
</script>

<template>
  <div class="flex h-full min-h-0">
    <!-- ── Main panel ──────────────────────────────────────────────────────── -->
    <div class="flex-1 min-w-0 flex flex-col p-4 sm:p-6 gap-4 overflow-auto">

      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <Activity class="h-5 w-5 text-muted-foreground shrink-0" />
          <h1 class="font-heading text-lg leading-none">Monitor Broker MQTT</h1>
          <span class="flex items-center gap-1.5 text-xs">
            <span class="h-2 w-2 rounded-full shrink-0"
              :class="connected ? 'bg-green-500 animate-pulse' : 'bg-red-500'" />
            <span :class="connected ? 'text-green-400' : 'text-destructive'">
              {{ connected ? 'En vivo' : 'Desconectado' }}
            </span>
          </span>
        </div>
        <div class="flex items-center gap-2">
          <button
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-sm border border-border text-xs hover:bg-muted transition-colors"
            @click="showLegend = !showLegend"
            title="Explicación de indicadores"
          >
            <Info class="h-3.5 w-3.5" />
            Indicadores
          </button>
          <button
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-sm border border-border text-xs hover:bg-muted transition-colors"
            @click="paused = !paused"
          >
            <component :is="paused ? Play : Pause" class="h-3.5 w-3.5" />
            {{ paused ? 'Reanudar' : 'Pausar' }}
          </button>
          <button
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-sm border border-destructive/60 text-xs text-destructive hover:bg-destructive/10 transition-colors"
            @click="clearAll"
          >
            <Trash2 class="h-3.5 w-3.5" />
            Limpiar
          </button>
        </div>
      </div>

      <!-- Legend (collapsible) -->
      <Transition name="fade">
        <div v-if="showLegend" class="rounded-sm border border-border bg-card p-4 space-y-2">
          <p class="text-[11px] uppercase tracking-wide font-semibold text-muted-foreground mb-3">Tipos de mensaje</p>
          <div v-for="(info, key) in KIND_INFO" :key="key" class="flex items-start gap-3">
            <span class="shrink-0 inline-flex rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wide mt-0.5"
              :class="info.color">{{ info.label }}</span>
            <p class="text-xs text-muted-foreground leading-relaxed">{{ info.desc }}</p>
          </div>
          <div class="pt-2 border-t border-border text-xs text-muted-foreground">
            <strong class="text-foreground">Click en una fila</strong> para inspeccionar la trama completa.
            La pestaña <strong class="text-foreground">Descartados</strong> muestra señales SSFV que llegan
            del broker pero no están configuradas en el catálogo.
          </div>
        </div>
      </Transition>

      <!-- Stats -->
      <dl class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div class="rounded-sm border border-border bg-card p-3">
          <dt class="text-[10px] uppercase tracking-wide text-muted-foreground">Msgs/min</dt>
          <dd class="font-mono text-2xl mt-1 tabular-nums">{{ eventsPerMin }}</dd>
        </div>
        <div class="rounded-sm border border-border bg-card p-3">
          <dt class="text-[10px] uppercase tracking-wide text-muted-foreground">Tópicos únicos</dt>
          <dd class="font-mono text-2xl mt-1 tabular-nums">{{ topicsSeen }}</dd>
        </div>
        <div class="rounded-sm border border-border bg-card p-3">
          <dt class="text-[10px] uppercase tracking-wide text-muted-foreground">Esta sesión</dt>
          <dd class="font-mono text-2xl mt-1 tabular-nums">{{ totalSeen }}</dd>
        </div>
        <div class="rounded-sm border border-border bg-card p-3">
          <dt class="text-[10px] uppercase tracking-wide text-muted-foreground">En buffer</dt>
          <dd class="font-mono text-2xl mt-1 tabular-nums">{{ events.length }}</dd>
        </div>
      </dl>

      <!-- Tabs -->
      <div class="flex border-b border-border gap-0">
        <button
          class="px-4 py-2 text-sm font-medium transition-colors -mb-px border-b-2"
          :class="tab === 'live'
            ? 'border-primary text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground'"
          @click="onTabChange('live')"
        >Mensajes en vivo</button>
        <button
          class="px-4 py-2 text-sm font-medium transition-colors -mb-px border-b-2 flex items-center gap-2"
          :class="tab === 'missed'
            ? 'border-destructive text-destructive'
            : 'border-transparent text-muted-foreground hover:text-foreground'"
          @click="onTabChange('missed')"
        >
          Descartados SSFV
          <span v-if="missedSignals.length > 0"
            class="rounded-full bg-destructive/20 text-destructive text-[10px] font-bold px-1.5 py-0.5">
            {{ missedSignals.length }}
          </span>
        </button>
      </div>

      <!-- ── Tab: Live messages ──────────────────────────────────────────────── -->
      <div v-if="tab === 'live'" class="rounded-sm border border-border bg-card overflow-hidden">
        <div class="overflow-auto" style="max-height: calc(100vh - 24rem)">
          <table class="w-full text-xs font-mono" aria-label="Eventos del broker MQTT">
            <thead class="sticky top-0 bg-card border-b border-border z-10">
              <tr>
                <th class="px-3 py-2 text-left text-muted-foreground font-medium w-36">Hora</th>
                <th class="px-3 py-2 text-left text-muted-foreground font-medium w-24">Tipo</th>
                <th class="px-3 py-2 text-left text-muted-foreground font-medium">Tópico</th>
                <th class="px-3 py-2 text-right text-muted-foreground font-medium w-20">Payload</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="ev in [...events].reverse()"
                :key="ev.id"
                class="border-b border-border/40 hover:bg-muted/20 transition-colors cursor-pointer"
                :class="selectedEvent?.id === ev.id ? 'bg-muted/40' : ''"
                @click="selectEvent(ev)"
              >
                <td class="px-3 py-1.5 text-muted-foreground tabular-nums whitespace-nowrap">
                  {{ fmtTime(ev.at) }}
                </td>
                <td class="px-3 py-1.5">
                  <span class="inline-flex rounded px-1 py-0.5 text-[9px] font-bold uppercase tracking-wide"
                    :class="KIND_INFO[ev.kind]?.color ?? 'bg-muted text-foreground'">
                    {{ ev.kind }}
                  </span>
                </td>
                <td class="px-3 py-1.5 max-w-0 truncate" :title="ev.topic">{{ ev.topic }}</td>
                <td class="px-3 py-1.5 text-right tabular-nums text-muted-foreground">{{ fmtSize(ev.size) }}</td>
              </tr>
              <tr v-if="events.length === 0">
                <td colspan="4" class="px-3 py-10 text-center text-muted-foreground text-sm font-sans">
                  Sin mensajes aún — esperando actividad del broker…
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- ── Tab: Missed signals ─────────────────────────────────────────────── -->
      <div v-if="tab === 'missed'" class="space-y-3">
        <div class="flex items-center justify-between">
          <p class="text-xs text-muted-foreground">
            Señales que llegan del broker pero no tienen entrada activa en
            <code class="font-mono bg-muted px-1 rounded">tbl_senales_x_equipo</code>.
            Para persistirlas, configúralas en la sección
            <strong>Plantas Solares → Señales x Equipo</strong>.
          </p>
          <button
            class="shrink-0 text-xs px-2 py-1 rounded-sm border border-border hover:bg-muted transition-colors"
            @click="loadMissed"
          >Actualizar</button>
        </div>

        <div class="rounded-sm border border-border bg-card overflow-hidden">
          <div v-if="missedLoading" class="py-10 text-center text-muted-foreground text-sm">
            Cargando…
          </div>
          <table v-else class="w-full text-xs font-mono">
            <thead class="sticky top-0 bg-card border-b border-border">
              <tr>
                <th class="px-3 py-2 text-left text-muted-foreground font-medium w-36">Último visto</th>
                <th class="px-3 py-2 text-left text-muted-foreground font-medium w-48">Equipo (topic)</th>
                <th class="px-3 py-2 text-left text-muted-foreground font-medium">Signal path</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="m in missedSignals"
                :key="m.signal_path"
                class="border-b border-border/40 hover:bg-muted/20 transition-colors"
              >
                <td class="px-3 py-1.5 text-muted-foreground tabular-nums whitespace-nowrap">
                  {{ fmtTime(m.at) }}
                </td>
                <td class="px-3 py-1.5 truncate max-w-0" :title="m.equipo">{{ m.equipo }}</td>
                <td class="px-3 py-1.5 truncate max-w-0 text-orange-400" :title="m.signal_path">
                  {{ m.signal_path }}
                </td>
              </tr>
              <tr v-if="missedSignals.length === 0 && !missedLoading">
                <td colspan="3" class="px-3 py-10 text-center text-muted-foreground text-sm font-sans">
                  Sin señales descartadas — todos los signal_paths SSFV están configurados en el catálogo.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- ── Detail panel (right side) ──────────────────────────────────────── -->
    <Transition name="slide">
      <aside
        v-if="selectedEvent"
        class="w-[380px] shrink-0 border-l border-border bg-card flex flex-col overflow-hidden"
      >
        <!-- Detail header -->
        <div class="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <span class="inline-flex rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wide shrink-0"
              :class="KIND_INFO[selectedEvent.kind]?.color ?? 'bg-muted text-foreground'">
              {{ selectedEvent.kind }}
            </span>
            <span class="font-mono text-xs truncate text-muted-foreground">
              #{{ selectedEvent.id }}
            </span>
          </div>
          <button
            class="grid place-items-center w-7 h-7 rounded-sm hover:bg-muted transition-colors shrink-0"
            @click="selectedEvent = null"
            title="Cerrar"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <!-- Detail body -->
        <div class="flex-1 min-h-0 overflow-auto p-4 space-y-4">

          <!-- Metadata -->
          <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
            <div>
              <dt class="text-muted-foreground">Hora</dt>
              <dd class="font-mono mt-0.5 tabular-nums">{{ fmtTime(selectedEvent.at) }}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Tamaño</dt>
              <dd class="font-mono mt-0.5">{{ fmtSize(selectedEvent.size) }}</dd>
            </div>
            <div class="col-span-2">
              <dt class="text-muted-foreground">Tópico MQTT</dt>
              <dd class="font-mono mt-0.5 break-all text-[11px]">{{ selectedEvent.topic }}</dd>
            </div>
          </dl>

          <!-- Sparkplug topic decomposition -->
          <template v-if="selectedEvent.kind === 'sparkplug'">
            <div class="rounded-sm border border-blue-700/40 bg-blue-950/20 p-3 space-y-2">
              <p class="text-[10px] uppercase tracking-wide font-semibold text-blue-400">Desglose Sparkplug B</p>
              <dl
                v-if="parseSparkplugTopic(selectedEvent.topic) as Record<string,string|null>"
                class="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11px] font-mono"
              >
                <template v-for="(v, k) in parseSparkplugTopic(selectedEvent.topic)" :key="k">
                  <div v-if="v">
                    <dt class="text-muted-foreground capitalize">{{ k }}</dt>
                    <dd class="text-blue-300 mt-0.5">{{ v }}</dd>
                  </div>
                </template>
              </dl>
              <p class="text-[11px] text-muted-foreground mt-2">
                El payload es binario protobuf (spBv1.0) — no se muestra contenido crudo.
                Los valores se decodifican y persisten vía SparkplugHandler.
              </p>
            </div>
          </template>

          <!-- State payload -->
          <template v-else-if="selectedEvent.kind === 'state'">
            <div class="rounded-sm border border-purple-700/40 bg-purple-950/20 p-3 space-y-2">
              <p class="text-[10px] uppercase tracking-wide font-semibold text-purple-400">Payload STATE</p>
              <p class="font-mono text-sm text-purple-200">
                {{ selectedEvent.payload || '(vacío)' }}
              </p>
              <p class="text-[11px] text-muted-foreground">
                Mensaje de presencia del host Sparkplug B. Valor típico: ONLINE / OFFLINE.
              </p>
            </div>
          </template>

          <!-- JSON payload (ssfv / json) -->
          <template v-else>
            <div class="space-y-1.5">
              <p class="text-[10px] uppercase tracking-wide font-semibold text-muted-foreground">
                Payload JSON
                <span v-if="selectedEvent.kind === 'ssfv'" class="text-amber-400">(SSFV)</span>
                <span v-else class="text-emerald-400">(JSON genérico)</span>
              </p>
              <pre
                v-if="selectedEvent.payload"
                class="text-[11px] font-mono bg-muted/50 rounded-sm p-3 overflow-auto max-h-[400px] whitespace-pre-wrap break-all leading-relaxed"
              >{{ prettyJson(selectedEvent.payload) }}</pre>
              <p v-else class="text-xs text-muted-foreground italic">
                Payload no disponible (snapshot del servidor no incluye cuerpo).
              </p>
            </div>

            <!-- SSFV note -->
            <div v-if="selectedEvent.kind === 'ssfv'"
              class="rounded-sm border border-amber-600/30 bg-amber-950/20 p-3 text-[11px] text-muted-foreground">
              <p class="font-semibold text-amber-400 mb-1">Ruta SSFV</p>
              <p class="font-mono break-all">{{ selectedEvent.topic }}</p>
              <p class="mt-1.5">
                Este tópico está en el catálogo
                <code class="bg-muted px-1 rounded">tbl_equipo.nombre_topic</code>.
                Cada clave del JSON debe estar en
                <code class="bg-muted px-1 rounded">tbl_senales_x_equipo.nombre_instancia</code>
                para persistirse.
              </p>
            </div>
          </template>

        </div>
      </aside>
    </Transition>
  </div>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 120ms ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

.slide-enter-active, .slide-leave-active { transition: width 200ms ease, opacity 200ms ease; }
.slide-enter-from, .slide-leave-to { width: 0; opacity: 0; }
</style>
