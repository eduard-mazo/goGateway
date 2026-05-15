<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Activity, Trash2, Pause, Play } from 'lucide-vue-next'

interface BrokerEvent {
  id: number
  at: string
  topic: string
  size: number
  kind: 'sparkplug' | 'ssfv' | 'json' | 'state'
}

const LS_KEY = 'broker-monitor-events'
const MAX_STORED = 500
const MAX_DISPLAYED = 300

const events = ref<BrokerEvent[]>([])
const connected = ref(false)
const paused = ref(false)
const totalSeen = ref(0)

let es: EventSource | null = null
let rateWindow: number[] = []
const eventsPerMin = ref(0)

function loadFromStorage(): BrokerEvent[] {
  try {
    const raw = localStorage.getItem(LS_KEY)
    return raw ? (JSON.parse(raw) as BrokerEvent[]) : []
  } catch { return [] }
}

function saveToStorage(evs: BrokerEvent[]) {
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(evs.slice(-MAX_STORED)))
  } catch { /* storage quota exceeded */ }
}

function clearAll() {
  localStorage.removeItem(LS_KEY)
  events.value = []
  totalSeen.value = 0
  rateWindow = []
  eventsPerMin.value = 0
}

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
  es?.close()
  es = null
  connected.value = false
}

const topicsSeen = computed(() => {
  const s = new Set<string>()
  events.value.forEach(e => s.add(e.topic))
  return s.size
})

function kindBadge(kind: string) {
  switch (kind) {
    case 'sparkplug': return 'bg-blue-700 text-white'
    case 'ssfv':      return 'bg-amber-600 text-white'
    case 'state':     return 'bg-purple-700 text-white'
    default:          return 'bg-emerald-700 text-white'
  }
}

function fmtTime(at: string) {
  const d = new Date(at)
  return d.toLocaleTimeString('es-CO', {
    hour12: false, hour: '2-digit', minute: '2-digit',
    second: '2-digit', fractionalSecondDigits: 3,
  } as Intl.DateTimeFormatOptions)
}

function fmtSize(size: number) {
  if (size >= 1024) return `${(size / 1024).toFixed(1)}k`
  return `${size}B`
}

onMounted(async () => {
  await loadSnapshot()
  connect()
})
onUnmounted(disconnect)
</script>

<template>
  <div class="p-4 sm:p-6 space-y-4 max-w-[1400px] mx-auto">
    <!-- Header row -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3">
        <Activity class="h-5 w-5 text-muted-foreground shrink-0" aria-hidden="true" />
        <h1 class="font-heading text-lg leading-none">Monitor Broker MQTT</h1>
        <span class="flex items-center gap-1.5 text-xs">
          <span
            class="h-2 w-2 rounded-full shrink-0"
            :class="connected ? 'bg-green-500 animate-pulse' : 'bg-red-500'"
          />
          <span :class="connected ? 'text-green-400' : 'text-destructive'">
            {{ connected ? 'En vivo' : 'Desconectado' }}
          </span>
        </span>
      </div>

      <div class="flex items-center gap-2">
        <button
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-sm border border-border text-xs hover:bg-muted transition-colors"
          @click="paused = !paused"
          :title="paused ? 'Reanudar stream' : 'Pausar stream'"
        >
          <component :is="paused ? Play : Pause" class="h-3.5 w-3.5" />
          {{ paused ? 'Reanudar' : 'Pausar' }}
        </button>
        <button
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-sm border border-destructive/60 text-xs text-destructive hover:bg-destructive/10 transition-colors"
          @click="clearAll"
          title="Limpiar eventos y localStorage"
        >
          <Trash2 class="h-3.5 w-3.5" />
          Limpiar
        </button>
      </div>
    </div>

    <!-- Stat cards -->
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

    <!-- Leyenda de tipos -->
    <div class="flex flex-wrap gap-2 text-[10px] font-bold uppercase tracking-wide">
      <span class="rounded px-1.5 py-0.5 bg-blue-700 text-white">Sparkplug</span>
      <span class="rounded px-1.5 py-0.5 bg-amber-600 text-white">SSFV</span>
      <span class="rounded px-1.5 py-0.5 bg-emerald-700 text-white">JSON</span>
      <span class="rounded px-1.5 py-0.5 bg-purple-700 text-white">State</span>
    </div>

    <!-- Events table -->
    <div class="rounded-sm border border-border bg-card overflow-hidden">
      <div class="overflow-auto" style="max-height: calc(100vh - 22rem);">
        <table class="w-full text-xs font-mono" aria-label="Eventos del broker MQTT">
          <thead class="sticky top-0 bg-card border-b border-border z-10">
            <tr>
              <th class="px-3 py-2 text-left text-muted-foreground font-medium w-36">Hora</th>
              <th class="px-3 py-2 text-left text-muted-foreground font-medium w-24">Tipo</th>
              <th class="px-3 py-2 text-left text-muted-foreground font-medium">Tópico</th>
              <th class="px-3 py-2 text-right text-muted-foreground font-medium w-16">Payload</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="ev in [...events].reverse()"
              :key="ev.id"
              class="border-b border-border/40 hover:bg-muted/20 transition-colors"
            >
              <td class="px-3 py-1 text-muted-foreground tabular-nums whitespace-nowrap">
                {{ fmtTime(ev.at) }}
              </td>
              <td class="px-3 py-1">
                <span
                  class="inline-flex items-center rounded px-1 py-0.5 text-[9px] font-bold uppercase tracking-wide"
                  :class="kindBadge(ev.kind)"
                >{{ ev.kind }}</span>
              </td>
              <td class="px-3 py-1 max-w-0 truncate" :title="ev.topic">{{ ev.topic }}</td>
              <td class="px-3 py-1 text-right tabular-nums text-muted-foreground">{{ fmtSize(ev.size) }}</td>
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
  </div>
</template>
