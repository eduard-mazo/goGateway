import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '@/api'
import type { TSDBStatus, TSDBDLQList } from '@/api'

export function useTSDB(pollMs = 2000) {
  const status  = ref<TSDBStatus | null>(null)
  const dlq     = ref<TSDBDLQList>({ count: 0, entries: [] })
  const error   = ref<string | null>(null)
  let timer: ReturnType<typeof setInterval> | null = null

  async function fetchStatus() {
    try {
      const r = await api.get<TSDBStatus>('/tsdb/status')
      status.value = r.data
      error.value  = null
    } catch (e: any) {
      error.value = e?.message ?? 'fetch failed'
    }
  }

  async function fetchDLQ() {
    try {
      const r = await api.get<TSDBDLQList>('/tsdb/dlq')
      dlq.value = r.data
    } catch { /* ignore */ }
  }

  async function replayDLQ(): Promise<number> {
    const r = await api.post<{ replayed: number }>('/tsdb/dlq/replay')
    await fetchDLQ()
    return r.data.replayed
  }

  function start() {
    fetchStatus()
    fetchDLQ()
    timer = setInterval(() => {
      fetchStatus()
      if (dlq.value.count > 0) fetchDLQ()
    }, pollMs)
  }

  function stop() {
    if (timer) clearInterval(timer)
  }

  onMounted(start)
  onUnmounted(stop)

  const totalWriteRate = computed(() =>
    status.value?.backends.reduce((s, b) => s + b.writeRate, 0) ?? 0,
  )
  const anyCircuitOpen = computed(() =>
    status.value?.backends.some(b => b.circuitOpen) ?? false,
  )
  const systemAlert = computed(() =>
    anyCircuitOpen.value ||
    (status.value?.dlqDepth ?? 0) > 0 ||
    (status.value?.walPending ?? 0) > 500,
  )

  return {
    status, dlq, error,
    fetchStatus, fetchDLQ, replayDLQ,
    totalWriteRate, anyCircuitOpen, systemAlert,
  }
}
