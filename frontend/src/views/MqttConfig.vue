<script setup lang="ts">
import { ref, onMounted, computed, nextTick } from 'vue'
import { toast } from 'vue-sonner'
import { api, type MQTTConfig } from '@/api'
import { t } from '@/i18n'
import { useStatus } from '@/composables/useStatus'
import StatusPill from '@/components/StatusPill.vue'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import {
  Radio, RefreshCw, Save, Shield, Wifi, KeyRound, User, Zap,
  Plus, X, Hash,
} from 'lucide-vue-next'

const { status } = useStatus()

const cfg = ref<MQTTConfig>({
  id: 1, host: 'localhost', port: 1883, username: '', password: '',
  client_id: 'goGateway', use_tls: false,
  sparkplug_enabled: false, sp_group_id: 'goGateway', sp_host_id: 'goGateway-host',
  sp_topics: '',
})
const saving = ref(false)

// ── Topic list state ──────────────────────────────────────────────────────────
const topicList   = ref<string[]>([])
const topicInputs = ref<HTMLInputElement[]>([])

function parseTopics(raw: string): string[] {
  return raw.split(/[\n,]/).map(s => s.trim()).filter(Boolean)
}

function syncTopics() {
  cfg.value.sp_topics = topicList.value.filter(s => s.trim()).join('\n')
}

function addTopic() {
  topicList.value.push('')
  nextTick(() => {
    const last = topicInputs.value[topicList.value.length - 1]
    last?.focus()
  })
}

function removeTopic(i: number) {
  topicList.value.splice(i, 1)
  syncTopics()
}

function updateTopic(i: number, val: string) {
  topicList.value[i] = val
  syncTopics()
}

// Handle Enter key → add next row; Backspace on empty row → remove it
function onTopicKey(e: KeyboardEvent, i: number) {
  if (e.key === 'Enter') {
    e.preventDefault()
    topicList.value.splice(i + 1, 0, '')
    nextTick(() => topicInputs.value[i + 1]?.focus())
  } else if (e.key === 'Backspace' && topicList.value[i] === '' && topicList.value.length > 1) {
    e.preventDefault()
    topicList.value.splice(i, 1)
    syncTopics()
    nextTick(() => topicInputs.value[Math.max(0, i - 1)]?.focus())
  }
}

// ── Load / save ───────────────────────────────────────────────────────────────
async function load() {
  try {
    cfg.value = (await api.get<MQTTConfig>('/mqtt-config')).data
    topicList.value = parseTopics(cfg.value.sp_topics || '')
  } catch (e: any) {
    toast.error('Load failed: ' + (e?.message ?? e))
  }
}

async function save() {
  syncTopics()
  saving.value = true
  try {
    cfg.value = (await api.put<MQTTConfig>('/mqtt-config', cfg.value)).data
    topicList.value = parseTopics(cfg.value.sp_topics || '')
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

        <!-- Host + Port -->
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

        <!-- Credentials -->
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

        <!-- Client ID -->
        <div class="space-y-1.5">
          <Label for="cid" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.mqtt.clientId }}</Label>
          <Input id="cid" v-model="cfg.client_id" class="rounded-sm font-mono" />
        </div>

        <!-- TLS -->
        <div class="flex items-center gap-3 pt-2 p-4 rounded-lg bg-[color:color-mix(in_srgb,var(--epm-citrico)_10%,transparent)] border border-[color:color-mix(in_srgb,var(--epm-bosque)_15%,transparent)]">
          <Switch id="tls" v-model="cfg.use_tls" />
          <div class="flex-1">
            <Label for="tls" class="font-bold inline-flex items-center gap-1.5">
              <Shield class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" /> {{ t.mqtt.tls }}
            </Label>
            <div class="text-xs text-muted-foreground mt-0.5">{{ t.mqtt.tlsDesc }}</div>
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
              <div class="text-xs text-muted-foreground mt-0.5">{{ t.mqtt.sparkplugDesc }}</div>
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

        <!-- ── Additional subscriptions ──────────────────────────────────────── -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold inline-flex items-center gap-1.5">
              <Hash class="h-3 w-3 text-[color:var(--epm-citrico)]" />
              Suscripciones adicionales
            </Label>
            <span class="text-[10px] font-mono text-muted-foreground">
              {{ topicList.length }} patrón{{ topicList.length !== 1 ? 'es' : '' }}
            </span>
          </div>

          <!-- Topic list -->
          <div class="rounded-sm border border-border overflow-hidden bg-card">

            <!-- Column header -->
            <div class="flex items-center gap-0 px-0 py-1.5 border-b border-border bg-muted/40">
              <span class="w-8 shrink-0" />
              <span class="flex-1 text-[9px] uppercase tracking-[0.22em] font-bold text-muted-foreground px-2">
                Patrón MQTT
              </span>
              <span class="w-8 shrink-0" />
            </div>

            <!-- Empty state -->
            <div v-if="topicList.length === 0"
              class="flex items-center justify-center gap-2 py-5 text-xs text-muted-foreground select-none">
              <Plus class="h-3 w-3 opacity-40" />
              Sin suscripciones adicionales — usa el botón de abajo para agregar
            </div>

            <!-- Rows -->
            <div
              v-for="(topic, i) in topicList"
              :key="i"
              class="topic-row group flex items-center border-b border-border/50 last:border-b-0
                     focus-within:bg-[color:color-mix(in_srgb,var(--epm-bosque)_4%,transparent)]
                     hover:bg-muted/20 transition-colors"
            >
              <!-- Row index -->
              <span class="w-8 shrink-0 text-center text-[10px] font-mono tabular-nums
                           text-[color:var(--epm-citrico)] opacity-60 select-none">
                {{ i + 1 }}
              </span>

              <!-- Editable pattern -->
              <input
                :ref="(el) => { if (el) topicInputs[i] = el as HTMLInputElement }"
                :value="topic"
                spellcheck="false"
                autocomplete="off"
                placeholder="spBv1.0/Group/#  ·  plant/+/data  ·  sensors/#"
                class="flex-1 min-w-0 bg-transparent py-2.5 pr-2 text-xs font-mono
                       text-foreground placeholder:text-muted-foreground/35
                       focus:outline-none"
                @input="updateTopic(i, ($event.target as HTMLInputElement).value)"
                @keydown="onTopicKey($event, i)"
              />

              <!-- Delete button -->
              <button
                class="w-8 shrink-0 flex items-center justify-center py-2.5
                       opacity-0 group-hover:opacity-100 focus-visible:opacity-100
                       text-muted-foreground/50 hover:text-destructive transition-all"
                :title="`Eliminar suscripción ${i + 1}`"
                @click="removeTopic(i)"
              >
                <X class="h-3 w-3" />
              </button>
            </div>
          </div>

          <!-- Add button -->
          <button
            class="add-btn flex items-center gap-1.5 text-xs font-medium
                   text-[color:var(--epm-bosque)] hover:text-[color:var(--epm-bosque-deep)]
                   transition-colors mt-1 px-0.5"
            @click="addTopic"
          >
            <span class="grid place-items-center w-4 h-4 rounded-sm
                         bg-[color:color-mix(in_srgb,var(--epm-bosque)_15%,transparent)]
                         border border-[color:color-mix(in_srgb,var(--epm-bosque)_30%,transparent)]">
              <Plus class="h-2.5 w-2.5" />
            </span>
            Agregar suscripción
          </button>

          <p class="text-[11px] text-muted-foreground leading-relaxed">
            Suscripciones MQTT adicionales activas en cualquier modo, además de la principal.
            Sparkplug extra: <span class="font-mono text-[color:var(--epm-bosque)]">spBv1.0/OtherGroup/#</span> ·
            JSON wildcard: <span class="font-mono text-[color:var(--epm-bosque)]">plant/+/data</span>.
            Los tópicos aquí configurados pueden usarse en el mapeo de señales.
            <span class="text-[color:var(--epm-citrico)] font-mono text-[10px]">Enter</span> = nueva línea ·
            <span class="text-[color:var(--epm-citrico)] font-mono text-[10px]">Backspace</span> = elimina si vacío.
          </p>
        </div>

        <!-- URI preview -->
        <div class="font-mono text-[11px] text-muted-foreground border-t border-border pt-4">
          {{ t.mqtt.uriPreview }} · <span class="text-[color:var(--epm-bosque)] font-bold">{{ brokerUri }}</span>
        </div>
      </div>

      <!-- Footer actions -->
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

<style scoped>
/* Left accent bar on focused row */
.topic-row:focus-within {
  box-shadow: inset 2px 0 0 0 var(--epm-bosque);
}

/* Add button hover: icon background pulses to citric */
.add-btn:hover span:first-child {
  background-color: color-mix(in srgb, var(--epm-citrico) 20%, transparent);
  border-color: color-mix(in srgb, var(--epm-citrico) 40%, transparent);
}
</style>
