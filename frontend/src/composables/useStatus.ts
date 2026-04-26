import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '@/api'

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
  listen: string     // gateway-wide bind IP (mirrored from fleet)
  port: number
  asdu_addr: number
  clients: number    // TCP-accepted connections
  activated: number  // subset where the IEC-104 link is up (post-STARTDT)
  running: boolean
  enabled: boolean
}
export interface IEC104Status {
  running: boolean    // any instance is up
  listen_ip: string   // gateway-wide bind IP
  points: number      // cached points (shared across fleet)
  clients: number     // total TCP clients across fleet
  activated: number   // total protocol-active links
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
}

// Singleton-ish poller: first caller starts it, subsequent callers share state.
const status = ref<StatusResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
let timer: ReturnType<typeof setInterval> | null = null
let refCount = 0
const INTERVAL_MS = 2000

async function fetchOnce() {
  try {
    loading.value = true
    const { data } = await api.get<StatusResponse>('/status')
    status.value = data
    error.value = null
  } catch (e: any) {
    error.value = e?.message ?? 'status failed'
  } finally {
    loading.value = false
  }
}

export function useStatus() {
  onMounted(() => {
    refCount++
    if (!timer) {
      fetchOnce()
      timer = setInterval(fetchOnce, INTERVAL_MS)
    }
  })
  onUnmounted(() => {
    refCount--
    if (refCount <= 0 && timer) {
      clearInterval(timer)
      timer = null
    }
  })
  return { status, loading, error, refresh: fetchOnce }
}
