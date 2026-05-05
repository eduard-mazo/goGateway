import axios from 'axios'

export const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
})

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
}

export interface History {
  id: number
  mapping_id: number
  signal_key: string
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
}

export interface TSDBTestResult {
  ok: boolean
  latency_ms?: number
  message: string
}

export interface NATSConfig {
  id: number
  host: string
  port: number
  stream_name: string
  enabled: boolean
}
