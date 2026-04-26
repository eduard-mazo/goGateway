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
