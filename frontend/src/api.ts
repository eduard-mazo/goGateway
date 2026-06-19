import axios from 'axios'

export const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
})

// Inject Bearer token from localStorage on every request.
api.interceptors.request.use(config => {
  const token = localStorage.getItem('gw:access')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// On 401: attempt a silent token refresh, then retry once.
// If refresh fails, clear auth state and redirect to /login.
let _refreshing: Promise<boolean> | null = null
api.interceptors.response.use(
  r => r,
  async err => {
    const status = err.response?.status
    const isAuthRoute = err.config?.url?.includes('/auth/login') ||
                        err.config?.url?.includes('/auth/refresh')
    if (status === 401 && !isAuthRoute && !err.config?._retry) {
      err.config._retry = true
      if (!_refreshing) {
        _refreshing = (async () => {
          const sid = localStorage.getItem('gw:session')
          const rt  = localStorage.getItem('gw:refresh')
          if (!sid || !rt) return false
          try {
            const { data } = await api.post('/auth/refresh', {
              session_id: sid, refresh_token: rt,
            })
            localStorage.setItem('gw:access', data.access_token)
            return true
          } catch {
            return false
          } finally {
            _refreshing = null
          }
        })()
      }
      const ok = await _refreshing
      if (ok) {
        err.config.headers.Authorization = `Bearer ${localStorage.getItem('gw:access')}`
        return api(err.config)
      }
      // Refresh failed — wipe tokens and send user to login.
      ;['gw:access', 'gw:refresh', 'gw:session'].forEach(k => localStorage.removeItem(k))
      window.location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export interface Device {
  id: number
  server_id: number
  name: string
  description: string
  created_at: string
}

export interface Topic {
  id: number
  device_id: number
  topic: string
  qos: number
  enabled: boolean
  payload_format: string  // 'json' | 'sparkplug'
}

// GatewaySignal = normalisation layer (Menu 2). One logical measurement per topic stream.
// persist_to_db=true records values to local SQLite history; signal_mappings row
// (signal_id FK) exposes it via IEC-104 (Menu 3).
export interface GatewaySignal {
  id: number
  topic_id: number
  name: string
  json_key: string
  metric_name: string
  quality_key: string
  variable_type: string
  characteristic: string
  unit: string
  scale: number
  persist_to_db: boolean
  enabled: boolean
  created_at?: string
}

export interface MQTTConfig {
  id: number
  host: string
  port: number
  username: string
  password: string
  client_id: string
  use_tls: boolean
  sparkplug_enabled: boolean
  sp_group_id: string
  sp_host_id: string
  sp_topics: string
}

// IEC104Gateway = host-wide IEC-104 settings. Singleton.
export interface IEC104Gateway {
  id: number
  listen_ip: string
}

// IEC104Server = one passive IEC 60870-5-104 slave endpoint. Multiple rows
// expose the same point set under different Common ASDU Addresses. The bind
// IP is gateway-wide (see IEC104Gateway). scada_ips is a CSV allowlist of
// remote IPs permitted to connect; empty = block all.
export interface IEC104Server {
  id: number
  name: string
  port: number
  asdu_addr: number
  scada_ips: string
  k: number
  w: number
  t0: number
  t1: number
  t2: number
  t3: number
  enabled: boolean
}

export interface SignalMapping {
  id: number
  server_id: number
  topic_id: number
  signal_id: number | null  // links to gateway_signals row (Menu 2 → Menu 3 workflow)
  device_name: string
  variable_type: string
  characteristic: string
  json_key: string
  quality_key: string
  metric_name: string
  iec104_type: string
  ioa: number
  unit: string
  scale: number
  enabled: boolean
  business: string
  company: string
  deadband_abs: number
  deadband_pct: number
}

export interface History {
  id: number
  mapping_id: number
  signal_path: string
  value: number
  quality: number
  timestamp: string
}

// TSDB pipeline types
export interface TSDBBackendStatus {
  name: string
  type: string
  healthy: boolean
  writeRate: number
  errorRate: number
  bytesSent: number
  circuitOpen: boolean
  lastError?: string
  skippedRows?: number // unmapped signals dropped (ssfv only)
}

export interface TSDBStatus {
  running: boolean
  backends: TSDBBackendStatus[]
  walPending: number
  dlqDepth: number
  inputRate: number
  inputQueue: number
  retryQueues: Record<string, number>
  timestamp: string
}

export interface TSDBDLQEntry {
  id: number
  backend: string
  ts: string
  reason: string
  retries: number
  batch: unknown[]
}

export interface TSDBDLQList {
  count: number
  entries: TSDBDLQEntry[]
}

export interface TSDBConfig {
  id: number
  backend: 'none' | 'victoriametrics' | 'timescaledb' | 'both'
  vm_url: string
  vm_username: string
  vm_password: string
  ts_dsn: string
  ts_table: string
  wal_path: string
  dlq_path: string
  batch_size: number
  flush_ms: number
  enabled: boolean
  // Server never returns actual credentials; these flags say whether they're stored.
  has_ts_dsn?: boolean
  has_vm_password?: boolean
}

export interface TSDBTestResult {
  ok: boolean
  latency_ms?: number
  message: string
}

// ─── SSFV (Sistemas Solares Fotovoltaicos) ────────────────────────────────────

export interface SSFVStatus {
  connected: boolean
  healthy?: boolean
  write_rate?: number
  error_rate?: number
  circuit_open?: boolean
  last_error?: string
  skipped_rows?: number
}

export interface SSFVPlanta {
  planta_id?: number
  nombre: string
  ubicacion?: string
  propietario?: string
  broker_base: string
  capacidad_kWp?: number
  fecha_comisionamiento?: string
  estado: number
  // Fleet stats (read-only, computed by GET /ssfv/plantas)
  n_equipos?: number
  n_senales?: number
  n_alarmas?: number
  ultima_lectura?: string | null
}

export interface SSFVEquipo {
  equipo_id?: number
  planta_id: number
  tipo_id: number
  nombre_equipo: string
  nombre_topic: string
  fabricante?: string
  modelo?: string
  nro_serie?: string
  estado: number
  // device-reported hardware identity (ICR edges, auto-populated from Device/* metrics)
  hw_part_number?: string
  hw_product_type?: string
  hw_product_name?: string
  hw_firmware?: string
  hw_serial?: string
  hw_uuid?: string
  hw_reported_at?: string
  // joined
  tipo_nombre?: string
  planta_nombre?: string
}

export interface SSFVSenal {
  senal_id?: number
  tipavar_id: number
  unidad_id: number
  nombre: string
  descripcion?: string
  tipo_valor: 'Instantaneo' | 'Acumulado'
  codigo_senal: string
  es_indexada: boolean
  activo: boolean
  // joined
  tipo_var_nombre?: string
  unidad_simbolo?: string
}

export interface SSFVAsignacion {
  equisenal_id?: number
  senal_id: number
  equipo_id: number
  indice_canal?: number
  nombre_instancia: string
  activo: boolean
  // joined
  senal_nombre?: string
  codigo_senal?: string
  nombre_equipo?: string
  nombre_topic?: string
  signal_path?: string
}

export interface SSFVFrontera {
  frontera_id?: number
  planta_id: number
  codigo_nie: string
  nombre: string
  tipo_conexion?: string
  activo: boolean
  planta_nombre?: string
}

export interface SSFVTipoEquipo {
  tipo_id?: number
  nombre: string
  descripcion?: string
  activo: boolean
}

export interface SSFVTipoVariable {
  tipovar_id?: number
  nombre: string
  descripcion?: string
  activo: boolean
}

export interface SSFVUnidad {
  unidad_id?: number
  simbolo: string
  nombre: string
  magnitud: string
  activo: boolean
}

export interface SSFVSenalXTipo {
  senaltipo_id?: number
  senal_id: number
  tipo_id: number
  num_canales: number
  // joined
  senal_nombre?: string
  codigo_senal?: string
  tipo_nombre?: string
}

export interface SSFVMissedSignal {
  signal_path: string
  count: number
  last_seen: string
}

export interface SSFVCatalogItem {
  [key: string]: unknown
}

// ─── Auth / User Management ───────────────────────────────────────────────────

export interface User {
  id: number
  username: string
  email: string
  full_name: string
  role: string
  enabled: boolean
  created_at: string
  updated_at: string
}
