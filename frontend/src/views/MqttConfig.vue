<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { toast } from 'vue-sonner'
import { api, type MQTTConfig } from '@/api'
import { t } from '@/i18n'
import { useStatus } from '@/composables/useStatus'
import StatusPill from '@/components/StatusPill.vue'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Radio, RefreshCw, Save, Shield, Wifi, KeyRound, User, Zap } from 'lucide-vue-next'

const { status } = useStatus()

const cfg = ref<MQTTConfig>({
  id: 1, host: 'localhost', port: 1883, username: '', password: '',
  client_id: 'goGateway', use_tls: false,
  sparkplug_enabled: false, sp_group_id: 'goGateway', sp_host_id: 'goGateway-host',
  sp_topics: '',
})
const saving = ref(false)

async function load() {
  try { cfg.value = (await api.get<MQTTConfig>('/mqtt-config')).data }
  catch (e: any) { toast.error('Load failed: ' + (e?.message ?? e)) }
}

async function save() {
  saving.value = true
  try {
    cfg.value = (await api.put<MQTTConfig>('/mqtt-config', cfg.value)).data
    toast.success(t.mqtt.saved)
  } catch (e: any) {
    toast.error(t.mqtt.saveFailed + (e?.response?.data?.error ?? e?.message ?? e))
  } finally { saving.value = false }
}

onMounted(load)

const brokerState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  if (!status.value) return 'idle'
  return status.value.mqtt.connected ? 'ok' : 'fault'
})

const brokerUri = computed(() => {
  if (!cfg.value.host) return '—'
  const scheme = cfg.value.use_tls ? 'ssl' : 'tcp'
  return `${scheme}://${cfg.value.host}:${cfg.value.port || 1883}`
})

function fmtAgo(ts?: number | null) {
  if (!ts) return '—'
  const diff = (Date.now() - ts * 1000) / 1000
  if (diff < 60) return `${Math.round(diff)}s ago`
  if (diff < 3600) return `${Math.round(diff / 60)}m ago`
  return `${Math.round(diff / 3600)}h ago`
}
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8 max-w-5xl">
    <!-- Hero / live broker status -->
    <section class="card-soft overflow-hidden">
      <div class="grid grid-cols-12 gap-6 p-8">
        <div class="col-span-12 md:col-span-7">
          <div class="flex items-center gap-3 mb-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-bosque)] text-white">
              <Radio class="h-5 w-5" />
            </div>
            <span class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">
              {{ t.mqtt.hero }}
            </span>
          </div>
          <h1 class="mb-2">{{ t.mqtt.title }}</h1>
          <div class="font-mono text-lg mt-2 text-[color:var(--epm-bosque)] font-bold break-all">
            {{ status?.mqtt.broker || brokerUri }}
          </div>
          <p class="mt-3 text-sm text-muted-foreground max-w-xl">
            {{ t.mqtt.desc }}
          </p>
        </div>
        <div class="col-span-12 md:col-span-5 flex flex-col gap-3 md:items-end">
          <StatusPill :state="brokerState" :label="brokerState === 'ok' ? t.status.connected : (brokerState === 'idle' ? t.status.idle : t.status.down)" />
          <div class="chip font-mono text-xs">
            <Wifi class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" />
            {{ status?.mqtt.topics ?? 0 }} {{ t.mqtt.topics }}
          </div>
          <div class="chip font-mono text-xs">
            <span class="h-2 w-2 rounded-sm bg-[color:var(--epm-citrico)]" />
            {{ (status?.mqtt.messages ?? 0).toLocaleString() }} {{ t.mqtt.msgsIn }}
          </div>
          <div class="text-[11px] text-muted-foreground font-mono mt-1">
            {{ t.mqtt.lastMsg }} {{ fmtAgo(status?.mqtt.last_msg_at) }}
          </div>
        </div>
      </div>
    </section>

    <!-- Config form -->
    <section class="card-soft overflow-hidden">
      <div class="flex items-center justify-between px-6 py-5 border-b border-border">
        <div>
          <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.mqtt.config }}</div>
          <div class="font-sans text-lg font-extrabold tracking-tight mt-1">{{ t.mqtt.brokerConn }}</div>
        </div>
        <div class="text-xs text-muted-foreground">
          {{ t.mqtt.topicsLive }} <span class="font-bold text-[color:var(--epm-bosque)]">{{ t.nav.devices }}</span>.
        </div>
      </div>

      <div class="p-6 space-y-5">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div class="sm:col-span-2 space-y-1.5">
            <Label for="host" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.mqtt.host }}</Label>
            <Input id="host" v-model="cfg.host" placeholder="broker.local" class="rounded-sm" />
          </div>
          <div class="space-y-1.5">
            <Label for="port" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.mqtt.port }}</Label>
            <Input id="port" v-model.number="cfg.port" type="number" class="rounded-sm" />
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="space-y-1.5">
            <Label for="user" class="text-[11px] uppercase tracking-[0.18em] font-bold inline-flex items-center gap-1.5">
              <User class="h-3 w-3" /> {{ t.mqtt.username }}
            </Label>
            <Input id="user" v-model="cfg.username" autocomplete="off" class="rounded-sm" />
          </div>
          <div class="space-y-1.5">
            <Label for="pwd" class="text-[11px] uppercase tracking-[0.18em] font-bold inline-flex items-center gap-1.5">
              <KeyRound class="h-3 w-3" /> {{ t.mqtt.password }}
            </Label>
            <Input id="pwd" v-model="cfg.password" type="password" autocomplete="new-password" class="rounded-sm" />
          </div>
        </div>

        <div class="space-y-1.5">
          <Label for="cid" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.mqtt.clientId }}</Label>
          <Input id="cid" v-model="cfg.client_id" class="rounded-sm font-mono" />
        </div>

        <div class="flex items-center gap-3 pt-2 p-4 rounded-lg bg-[color:color-mix(in_srgb,var(--epm-citrico)_10%,transparent)] border border-[color:color-mix(in_srgb,var(--epm-bosque)_15%,transparent)]">
          <Switch id="tls" v-model="cfg.use_tls" />
          <div class="flex-1">
            <Label for="tls" class="font-bold inline-flex items-center gap-1.5">
              <Shield class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" /> {{ t.mqtt.tls }}
            </Label>
            <div class="text-xs text-muted-foreground mt-0.5">
              {{ t.mqtt.tlsDesc }}
            </div>
          </div>
        </div>

        <!-- Sparkplug B -->
        <div class="rounded-lg border border-[color:color-mix(in_srgb,var(--epm-bosque)_20%,transparent)] overflow-hidden">
          <div class="flex items-center gap-3 px-4 py-3 bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)]">
            <Switch id="sp" v-model="cfg.sparkplug_enabled" />
            <div class="flex-1">
              <Label for="sp" class="font-bold inline-flex items-center gap-1.5">
                <Zap class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" /> {{ t.mqtt.sparkplug }}
              </Label>
              <div class="text-xs text-muted-foreground mt-0.5">
                {{ t.mqtt.sparkplugDesc }}
              </div>
            </div>
          </div>
          <div v-if="cfg.sparkplug_enabled" class="grid grid-cols-2 gap-4 px-4 pb-4 pt-3 border-t border-[color:color-mix(in_srgb,var(--epm-bosque)_12%,transparent)]">
            <div class="space-y-1.5">
              <Label for="sp_group" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.mqtt.groupId }}</Label>
              <Input id="sp_group" v-model="cfg.sp_group_id" placeholder="goGateway" class="rounded-sm font-mono" />
              <p class="text-[11px] text-muted-foreground">
                Subscribes to <span class="font-mono">spBv1.0/{{ cfg.sp_group_id || '…' }}/#</span>
              </p>
            </div>
            <div class="space-y-1.5">
              <Label for="sp_host" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.mqtt.hostId }}</Label>
              <Input id="sp_host" v-model="cfg.sp_host_id" placeholder="goGateway-host" class="rounded-sm font-mono" />
              <p class="text-[11px] text-muted-foreground">
                STATE topic: <span class="font-mono">STATE/{{ cfg.sp_host_id || '…' }}</span>
              </p>
            </div>
          </div>
        </div>

        <!-- Additional subscriptions — applies in both Sparkplug and JSON modes -->
        <div class="space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Suscripciones adicionales</Label>
          <textarea
            v-model="cfg.sp_topics"
            rows="4"
            placeholder="spBv1.0/OtherGroup/#
plant/+/data
sensors/#"
            class="w-full rounded-sm border border-input bg-background px-3 py-2 text-sm font-mono
                   placeholder:text-muted-foreground focus-visible:outline-none
                   focus-visible:ring-1 focus-visible:ring-ring resize-y"
          />
          <p class="text-[11px] text-muted-foreground">
            Un patrón MQTT por línea (o separados por comas). Se suscriben además de la suscripción
            principal, en cualquier modo. Útil para múltiples grupos Sparkplug
            (<span class="font-mono">spBv1.0/OtherGroup/#</span>), tópicos JSON adicionales o
            cualquier wildcard de monitoreo.
          </p>
        </div>

        <div class="font-mono text-[11px] text-muted-foreground border-t border-border pt-4">
          {{ t.mqtt.uriPreview }} · <span class="text-[color:var(--epm-bosque)] font-bold">{{ brokerUri }}</span>
        </div>
      </div>

      <div class="px-6 py-4 border-t border-border flex gap-2 bg-[color:color-mix(in_srgb,var(--epm-citrico)_5%,transparent)]">
        <Button
          :disabled="saving"
          @click="save"
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-6"
        >
          <Save class="h-4 w-4 mr-2" /> {{ saving ? t.mqtt.saving : t.mqtt.save }}
        </Button>
        <Button variant="outline" @click="load" class="rounded-sm">
          <RefreshCw class="h-4 w-4 mr-2" /> {{ t.mqtt.reload }}
        </Button>
      </div>
    </section>
  </div>
</template>
