import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '@/api'

// ── Domain types ──────────────────────────────────────────────────────────────
export interface MQTTStatus {
  connected: boolean
  broker: string
  topics: number
  messages: number
  last_msg_at: number
}
export interface IEC104ServerStatus {
  id: number
  name: string
  listen: string
  port: number
  asdu_addr: number
  clients: number
  activated: number
  running: boolean
  enabled: boolean
}
export interface IEC104Status {
  running: boolean
  listen_ip: string
  points: number
  clients: number
  activated: number
  servers: IEC104ServerStatus[]
}
export interface StatusResponse {
  mqtt: MQTTStatus
  iec104: IEC104Status
  devices: number
  topics: number
  mappings: number
  history_count: number
  last_sample_at?: string
  uptime_seconds: number
  started_at: string
  build_time: string
  git_commit: string
}
export type SystemHealth = 'ok' | 'warn' | 'fault' | 'unknown'

// ── Module-level singleton ────────────────────────────────────────────────────
const status = ref<StatusResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const lastFetchedAt = ref<Date | null>(null)
const errorCount = ref(0)

let timer: ReturnType<typeof setInterval> | null = null
let refCount = 0
let inFlight = false
let visibilityListening = false

const BASE_MS = 2_000
const MAX_BACKOFF_MS = 30_000

function backoffMs(): number {
  if (errorCount.value === 0) return BASE_MS
  // 2s → 4s → 8s → 16s → 30s
  return Math.min(BASE_MS * (2 ** Math.min(errorCount.value, 4)), MAX_BACKOFF_MS)
}

function reschedule(): void {
  if (timer === null) return
  clearInterval(timer)
  timer = setInterval(fetchOnce, backoffMs())
}

async function fetchOnce(): Promise<void> {
  if (inFlight) return
  inFlight = true
  loading.value = true
  try {
    const { data } = await api.get<StatusResponse>('/status')
    status.value = data
    error.value = null
    lastFetchedAt.value = new Date()
    if (errorCount.value > 0) {
      errorCount.value = 0
      reschedule() // back to base interval after recovery
    }
  } catch (e: any) {
    error.value = e?.message ?? 'status fetch failed'
    errorCount.value++
    reschedule() // extend to backoff interval
  } finally {
    loading.value = false
    inFlight = false
  }
}

function onVisibilityChange(): void {
  if (document.hidden) {
    // Tab hidden — stop polling to save bandwidth
    if (timer !== null) { clearInterval(timer); timer = null }
  } else {
    // Tab re-focused — fetch immediately and restart schedule
    timer = setInterval(fetchOnce, backoffMs())
    fetchOnce()
  }
}

// ── Derived computed state (module-level, always up-to-date) ──────────────────
// systemHealth: aggregate across MQTT + IEC-104 fleet
// ok      → all enabled subsystems nominal
// warn    → at least one IEC server bound but no protocol link, or partial fleet
// fault   → MQTT disconnected, or all enabled IEC servers failed to bind
// unknown → no data yet
const systemHealth = computed<SystemHealth>(() => {
  if (!status.value) return 'unknown'
  if (!status.value.mqtt.connected) return 'fault'
  const enabled = status.value.iec104.servers.filter(s => s.enabled)
  if (enabled.length > 0) {
    if (enabled.every(s => !s.running)) return 'fault'
    if (enabled.some(s => !s.running)) return 'warn'
  }
  return 'ok'
})

const isStale = computed<boolean>(() => {
  if (!lastFetchedAt.value) return true
  return Date.now() - lastFetchedAt.value.getTime() > 10_000
})

// ── Composable ────────────────────────────────────────────────────────────────
export function useStatus() {
  onMounted(() => {
    if (refCount === 0) {
      fetchOnce()
      timer = setInterval(fetchOnce, BASE_MS)
      if (!visibilityListening) {
        document.addEventListener('visibilitychange', onVisibilityChange)
        visibilityListening = true
      }
    }
    refCount++
  })

  onUnmounted(() => {
    refCount--
    if (refCount === 0) {
      if (timer !== null) { clearInterval(timer); timer = null }
      document.removeEventListener('visibilitychange', onVisibilityChange)
      visibilityListening = false
      errorCount.value = 0
    }
  })

  return {
    status,
    loading,
    error,
    lastFetchedAt,
    errorCount,
    systemHealth,
    isStale,
    refresh: fetchOnce,
  }
}
