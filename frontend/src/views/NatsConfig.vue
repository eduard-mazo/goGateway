<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { toast } from 'vue-sonner'
import { api, type NATSConfig } from '@/api'
import { useStatus } from '@/composables/useStatus'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Database, RefreshCw, Save, Zap, Server, AlertTriangle, CheckCircle2, WifiOff } from 'lucide-vue-next'
import { t } from '@/i18n'

const { status } = useStatus()

const cfg = ref<NATSConfig>({
  id: 1, host: 'localhost', port: 4222, stream_name: 'GOGATEWAY', enabled: false
})
const saving = ref(false)

async function load() {
  try { cfg.value = (await api.get<NATSConfig>('/nats-config')).data }
  catch (e: any) { toast.error(t.nats.loadFailed + (e?.message ?? e)) }
}

async function save() {
  saving.value = true
  try {
    await api.put('/nats-config', cfg.value)
    toast.success(t.nats.saved)
  } catch (e: any) {
    toast.error(t.nats.saveFailed + (e?.response?.data?.error ?? e?.message ?? e))
  } finally { saving.value = false }
}

onMounted(load)

const natsState = computed<'unavailable' | 'disabled' | 'active'>(() => {
  if (!cfg.value.enabled) return 'disabled'
  // If status has a nats field, check it; otherwise assume unavailable when enabled
  const s = status.value as any
  if (s?.nats?.available === false) return 'unavailable'
  if (s?.nats?.connected === false) return 'unavailable'
  return 'active'
})

const natsUri = computed(() => {
  return `nats://${cfg.value.host}:${cfg.value.port}`
})
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8 max-w-5xl">
    <!-- Hero / Concept -->
    <section class="card-soft overflow-hidden">
      <div class="grid grid-cols-12 gap-6 p-8">
        <div class="col-span-12 md:col-span-7">
          <div class="flex items-center gap-3 mb-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-bosque)] text-white">
              <Database class="h-5 w-5" />
            </div>
            <span class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">
              {{ t.nats.hero }}
            </span>
          </div>
          <h1 class="mb-2">{{ t.nats.title }}</h1>
          <p class="mt-3 text-sm text-muted-foreground max-w-xl">
            {{ t.nats.desc }}
          </p>
        </div>
        <div class="col-span-12 md:col-span-5 flex flex-col gap-3 md:items-end justify-center">
            <div class="chip font-mono text-xs">
              <Server class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" />
              {{ natsUri }}
            </div>
            <div class="chip font-mono text-xs">
              <Zap class="h-3.5 w-3.5 text-[color:var(--epm-citrico)]" />
              Stream: {{ cfg.stream_name }}
            </div>
        </div>
      </div>
    </section>

    <!-- NATS status banner -->
    <div
      v-if="natsState === 'unavailable'"
      class="flex items-start gap-3 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3"
    >
      <WifiOff class="h-4 w-4 text-destructive mt-0.5 shrink-0" />
      <div>
        <p class="text-sm font-semibold text-destructive">{{ t.nats.statusBanner.unavailable }}</p>
        <p class="text-xs text-muted-foreground mt-1">{{ t.nats.statusBanner.unavailableDesc }}</p>
      </div>
    </div>
    <div
      v-else-if="natsState === 'disabled'"
      class="flex items-start gap-3 rounded-md border border-border bg-muted/40 px-4 py-3"
    >
      <AlertTriangle class="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
      <div>
        <p class="text-sm font-semibold">{{ t.nats.statusBanner.disabled }}</p>
        <p class="text-xs text-muted-foreground mt-1">{{ t.nats.statusBanner.disabledDesc }}</p>
      </div>
    </div>
    <div
      v-else
      class="flex items-start gap-3 rounded-md border border-[color:color-mix(in_srgb,var(--epm-bosque)_40%,transparent)] bg-[color:color-mix(in_srgb,var(--epm-bosque)_8%,transparent)] px-4 py-3"
    >
      <CheckCircle2 class="h-4 w-4 text-[color:var(--epm-bosque)] mt-0.5 shrink-0" />
      <div>
        <p class="text-sm font-semibold text-[color:var(--epm-bosque)]">{{ t.nats.statusBanner.active }}</p>
        <p class="text-xs text-muted-foreground mt-1">{{ t.nats.statusBanner.activeDesc }}</p>
      </div>
    </div>

    <!-- Config form -->
    <section class="card-soft overflow-hidden">
      <div class="flex items-center justify-between px-6 py-5 border-b border-border">
        <div>
          <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">{{ t.nats.config }}</div>
          <div class="font-sans text-lg font-extrabold tracking-tight mt-1">{{ t.nats.integration }}</div>
        </div>
      </div>

      <div class="p-6 space-y-5">
        <!-- Enable Toggle -->
        <div class="flex items-center gap-3 p-4 rounded-lg bg-[color:color-mix(in_srgb,var(--epm-citrico)_10%,transparent)] border border-[color:color-mix(in_srgb,var(--epm-bosque)_15%,transparent)]">
          <Switch id="nats-enabled" v-model="cfg.enabled" />
          <div class="flex-1">
            <Label for="nats-enabled" class="font-bold inline-flex items-center gap-1.5">
              {{ t.nats.enable }}
            </Label>
            <div class="text-xs text-muted-foreground mt-0.5">
              {{ t.nats.enableDesc }}
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div class="sm:col-span-2 space-y-1.5">
            <Label for="host" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.nats.host }}</Label>
            <Input id="host" v-model="cfg.host" placeholder="localhost" class="rounded-sm" />
          </div>
          <div class="space-y-1.5">
            <Label for="port" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.nats.port }}</Label>
            <Input id="port" v-model.number="cfg.port" type="number" class="rounded-sm" />
          </div>
        </div>

        <div class="space-y-1.5">
          <Label for="stream" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.nats.streamName }}</Label>
          <Input id="stream" v-model="cfg.stream_name" class="rounded-sm font-mono" />
          <p class="text-[11px] text-muted-foreground">
            {{ t.nats.streamHint }} <span class="font-mono">{{ cfg.stream_name }}.metrics.></span>
          </p>
        </div>

        <div class="font-mono text-[11px] text-muted-foreground border-t border-border pt-4">
          NATS URI · <span class="text-[color:var(--epm-bosque)] font-bold">{{ natsUri }}</span>
        </div>
      </div>

      <div class="px-6 py-4 border-t border-border flex gap-2 bg-[color:color-mix(in_srgb,var(--epm-citrico)_5%,transparent)]">
        <Button
          :disabled="saving"
          @click="save"
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-6"
        >
          <Save class="h-4 w-4 mr-2" /> {{ saving ? t.nats.saving : t.nats.save }}
        </Button>
        <Button variant="outline" @click="load" class="rounded-sm">
          <RefreshCw class="h-4 w-4 mr-2" /> {{ t.nats.reload }}
        </Button>
      </div>
    </section>
  </div>
</template>
