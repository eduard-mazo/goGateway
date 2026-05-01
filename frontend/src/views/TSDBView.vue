<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { toast } from 'vue-sonner'
import { api, type TSDBConfig, type TSDBTestResult } from '@/api'
import { useTSDB } from '@/composables/useTSDB'
import BackendCard from '@/components/tsdb/BackendCard.vue'
import DLQTable    from '@/components/tsdb/DLQTable.vue'
import { Input }  from '@/components/ui/input'
import { Label }  from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import {
  Database, FlaskConical, CheckCircle2, XCircle,
  ChevronDown, Save, RefreshCw, Zap, Server, Eye, EyeOff,
} from 'lucide-vue-next'

const {
  status, dlq, error,
  replayDLQ, fetchDLQ,
  totalWriteRate, anyCircuitOpen, systemAlert,
} = useTSDB(2000)

// ── Config state ──────────────────────────────────────────────────────────────
const cfg = ref<TSDBConfig>({
  id: 1, backend: 'none',
  vm_url: '', vm_username: '', vm_password: '',
  ts_dsn: '', ts_table: 'signals',
  wal_path: 'data/wal.bolt', dlq_path: 'data/dlq.bolt',
  batch_size: 2000, flush_ms: 100, enabled: false,
})
const saving = ref(false)
const advancedOpen = ref(false)
const showDsn = ref(false)

const backendOptions = [
  { value: 'none',             label: 'None' },
  { value: 'victoriametrics',  label: 'VictoriaMetrics' },
  { value: 'timescaledb',      label: 'TimescaleDB' },
  { value: 'both',             label: 'Both' },
] as const

const showVM = computed(() =>
  cfg.value.backend === 'victoriametrics' || cfg.value.backend === 'both',
)
const showTS = computed(() =>
  cfg.value.backend === 'timescaledb' || cfg.value.backend === 'both',
)

// ── Connection test state ─────────────────────────────────────────────────────
const testingVM = ref(false)
const testResultVM = ref<TSDBTestResult | null>(null)
const testingTS = ref(false)
const testResultTS = ref<TSDBTestResult | null>(null)

// ── Load / Save ───────────────────────────────────────────────────────────────
async function load() {
  try {
    cfg.value = (await api.get<TSDBConfig>('/tsdb-config')).data
  } catch (e: any) {
    toast.error('Load failed: ' + (e?.message ?? e))
  }
}

async function save() {
  saving.value = true
  try {
    cfg.value = (await api.put<TSDBConfig>('/tsdb-config', cfg.value)).data
    toast.success('TSDB config saved · pipeline reloading')
  } catch (e: any) {
    toast.error('Save failed: ' + (e?.response?.data?.error ?? e?.message ?? e))
  } finally {
    saving.value = false
  }
}

// ── Connection tests ──────────────────────────────────────────────────────────
async function testVM() {
  testingVM.value = true
  testResultVM.value = null
  try {
    const r = await api.post<TSDBTestResult>('/tsdb-config/test', {
      backend: 'victoriametrics',
      vm_url: cfg.value.vm_url,
      vm_username: cfg.value.vm_username,
      vm_password: cfg.value.vm_password,
    })
    testResultVM.value = r.data
  } catch (e: any) {
    testResultVM.value = { ok: false, message: e?.response?.data?.error ?? e?.message ?? 'request failed' }
  } finally {
    testingVM.value = false
  }
}

async function testTS() {
  testingTS.value = true
  testResultTS.value = null
  try {
    const r = await api.post<TSDBTestResult>('/tsdb-config/test', {
      backend: 'timescaledb',
      ts_dsn: cfg.value.ts_dsn,
    })
    testResultTS.value = r.data
  } catch (e: any) {
    testResultTS.value = { ok: false, message: e?.response?.data?.error ?? e?.message ?? 'request failed' }
  } finally {
    testingTS.value = false
  }
}

async function handleReplay() {
  const n = await replayDLQ()
  await fetchDLQ()
  toast.info(`Replayed ${n} DLQ batch${n === 1 ? '' : 'es'}`)
}

onMounted(load)
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8 max-w-5xl">

    <!-- ── Configuration card ──────────────────────────────────────────────── -->
    <section class="card-soft overflow-hidden">
      <div class="flex items-center justify-between px-6 py-5 border-b border-border">
        <div class="flex items-center gap-3">
          <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-bosque)] text-white">
            <Database class="h-5 w-5" />
          </div>
          <div>
            <div class="text-[10px] uppercase tracking-[0.22em] text-muted-foreground font-bold">Configuration</div>
            <div class="font-sans text-lg font-extrabold tracking-tight mt-0.5">Time-Series Pipeline</div>
          </div>
        </div>
        <p class="text-xs text-muted-foreground hidden sm:block max-w-xs text-right">
          Store every signal sample in VictoriaMetrics and/or TimescaleDB
          for long-term analytics and dashboards.
        </p>
      </div>

      <div class="p-6 space-y-6">

        <!-- Backend selector -->
        <div class="space-y-2">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Backend</Label>
          <div class="inline-flex rounded-sm border border-border overflow-hidden">
            <button
              v-for="opt in backendOptions"
              :key="opt.value"
              class="px-4 py-2 text-xs font-bold uppercase tracking-wide transition-colors focus:outline-none"
              :class="cfg.backend === opt.value
                ? 'bg-[color:var(--epm-bosque)] text-white'
                : 'hover:bg-muted text-muted-foreground'"
              @click="cfg.backend = opt.value"
            >{{ opt.label }}</button>
          </div>
        </div>

        <!-- VictoriaMetrics form -->
        <div v-if="showVM" class="rounded-lg border border-[color:color-mix(in_srgb,var(--epm-bosque)_20%,transparent)] overflow-hidden">
          <div class="flex items-center gap-2 px-4 py-2.5 bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)] border-b border-[color:color-mix(in_srgb,var(--epm-bosque)_12%,transparent)]">
            <Zap class="h-4 w-4 text-[color:var(--epm-bosque)]" />
            <span class="text-[11px] uppercase tracking-[0.18em] font-bold">VictoriaMetrics</span>
          </div>
          <div class="p-4 space-y-4">
            <div class="space-y-1.5">
              <Label for="vm_url" class="text-[11px] uppercase tracking-[0.18em] font-bold">Write URL</Label>
              <Input id="vm_url" v-model="cfg.vm_url" placeholder="http://vm-host:8428" class="rounded-sm font-mono" />
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div class="space-y-1.5">
                <Label for="vm_user" class="text-[11px] uppercase tracking-[0.18em] font-bold">Username</Label>
                <Input id="vm_user" v-model="cfg.vm_username" autocomplete="off" class="rounded-sm" />
              </div>
              <div class="space-y-1.5">
                <Label for="vm_pass" class="text-[11px] uppercase tracking-[0.18em] font-bold">Password</Label>
                <Input id="vm_pass" v-model="cfg.vm_password" type="password" autocomplete="new-password" class="rounded-sm" />
              </div>
            </div>
            <!-- Test button + result -->
            <div class="flex items-center gap-3 flex-wrap">
              <Button
                variant="outline"
                size="sm"
                :disabled="testingVM || !cfg.vm_url"
                class="rounded-sm gap-1.5"
                @click="testVM"
              >
                <FlaskConical class="h-3.5 w-3.5" />
                {{ testingVM ? 'Testing…' : 'Test Connection' }}
              </Button>
              <span v-if="testResultVM" class="flex items-center gap-1.5 text-sm font-medium"
                :class="testResultVM.ok ? 'text-emerald-500' : 'text-destructive'">
                <CheckCircle2 v-if="testResultVM.ok" class="h-4 w-4" />
                <XCircle v-else class="h-4 w-4" />
                {{ testResultVM.ok
                  ? `connected · ${testResultVM.latency_ms}ms`
                  : testResultVM.message }}
              </span>
            </div>
          </div>
        </div>

        <!-- TimescaleDB form -->
        <div v-if="showTS" class="rounded-lg border border-[color:color-mix(in_srgb,var(--epm-bosque)_20%,transparent)] overflow-hidden">
          <div class="flex items-center gap-2 px-4 py-2.5 bg-[color:color-mix(in_srgb,var(--epm-bosque)_6%,transparent)] border-b border-[color:color-mix(in_srgb,var(--epm-bosque)_12%,transparent)]">
            <Server class="h-4 w-4 text-[color:var(--epm-bosque)]" />
            <span class="text-[11px] uppercase tracking-[0.18em] font-bold">TimescaleDB</span>
          </div>
          <div class="p-4 space-y-4">
            <div class="space-y-1.5">
              <Label for="ts_dsn" class="text-[11px] uppercase tracking-[0.18em] font-bold">DSN (connection string)</Label>
              <div class="flex gap-2">
                <Input
                  id="ts_dsn"
                  v-model="cfg.ts_dsn"
                  :type="showDsn ? 'text' : 'password'"
                  placeholder="postgres://user:pass@host:5432/gateway"
                  class="rounded-sm font-mono flex-1"
                  autocomplete="new-password"
                />
                <Button variant="outline" size="icon" class="rounded-sm shrink-0" @click="showDsn = !showDsn">
                  <Eye v-if="!showDsn" class="h-4 w-4" />
                  <EyeOff v-else class="h-4 w-4" />
                </Button>
              </div>
              <p class="text-[11px] text-muted-foreground">
                Format: <span class="font-mono">postgres://user:pass@host:5432/dbname</span>
              </p>
            </div>
            <!-- Test button + result -->
            <div class="flex items-center gap-3 flex-wrap">
              <Button
                variant="outline"
                size="sm"
                :disabled="testingTS || !cfg.ts_dsn"
                class="rounded-sm gap-1.5"
                @click="testTS"
              >
                <FlaskConical class="h-3.5 w-3.5" />
                {{ testingTS ? 'Testing…' : 'Test Connection' }}
              </Button>
              <span v-if="testResultTS" class="flex items-center gap-1.5 text-sm font-medium"
                :class="testResultTS.ok ? 'text-emerald-500' : 'text-destructive'">
                <CheckCircle2 v-if="testResultTS.ok" class="h-4 w-4" />
                <XCircle v-else class="h-4 w-4" />
                {{ testResultTS.ok
                  ? `connected · ${testResultTS.latency_ms}ms`
                  : testResultTS.message }}
              </span>
            </div>
          </div>
        </div>

        <!-- Advanced section (collapsible) -->
        <div class="rounded-lg border border-border overflow-hidden">
          <button
            class="w-full flex items-center justify-between px-4 py-3 text-left hover:bg-muted/50 transition-colors"
            @click="advancedOpen = !advancedOpen"
          >
            <span class="text-[11px] uppercase tracking-[0.18em] font-bold">Advanced</span>
            <ChevronDown
              class="h-4 w-4 text-muted-foreground transition-transform duration-200"
              :class="advancedOpen ? 'rotate-180' : ''"
            />
          </button>
          <div v-if="advancedOpen" class="px-4 pb-4 pt-1 grid grid-cols-1 sm:grid-cols-2 gap-4 border-t border-border">
            <div class="space-y-1.5">
              <Label for="wal_path" class="text-[11px] uppercase tracking-[0.18em] font-bold">WAL Path</Label>
              <Input id="wal_path" v-model="cfg.wal_path" placeholder="data/wal.bolt" class="rounded-sm font-mono text-xs" />
            </div>
            <div class="space-y-1.5">
              <Label for="dlq_path" class="text-[11px] uppercase tracking-[0.18em] font-bold">DLQ Path</Label>
              <Input id="dlq_path" v-model="cfg.dlq_path" placeholder="data/dlq.bolt" class="rounded-sm font-mono text-xs" />
            </div>
            <div class="space-y-1.5">
              <Label for="ts_table" class="text-[11px] uppercase tracking-[0.18em] font-bold">TimescaleDB Table</Label>
              <Input id="ts_table" v-model="cfg.ts_table" placeholder="signals" class="rounded-sm font-mono text-xs" />
            </div>
            <div class="space-y-1.5">
              <Label for="batch_size" class="text-[11px] uppercase tracking-[0.18em] font-bold">Batch Size</Label>
              <Input id="batch_size" v-model.number="cfg.batch_size" type="number" class="rounded-sm" />
            </div>
            <div class="space-y-1.5">
              <Label for="flush_ms" class="text-[11px] uppercase tracking-[0.18em] font-bold">Flush Interval (ms)</Label>
              <Input id="flush_ms" v-model.number="cfg.flush_ms" type="number" class="rounded-sm" />
            </div>
          </div>
        </div>

        <!-- Enable toggle -->
        <div class="flex items-center gap-3 p-4 rounded-lg bg-[color:color-mix(in_srgb,var(--epm-citrico)_10%,transparent)] border border-[color:color-mix(in_srgb,var(--epm-bosque)_15%,transparent)]">
          <Switch id="enabled" v-model="cfg.enabled" />
          <div class="flex-1">
            <Label for="enabled" class="font-bold inline-flex items-center gap-1.5">
              <Zap class="h-3.5 w-3.5 text-[color:var(--epm-bosque)]" /> Enable Pipeline
            </Label>
            <div class="text-xs text-muted-foreground mt-0.5">
              When disabled, no samples are forwarded to the time-series backend.
            </div>
          </div>
        </div>

      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-border flex gap-2 bg-[color:color-mix(in_srgb,var(--epm-citrico)_5%,transparent)]">
        <Button
          :disabled="saving"
          @click="save"
          class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-6"
        >
          <Save class="h-4 w-4 mr-2" />
          {{ saving ? 'Saving…' : 'Save & apply' }}
        </Button>
        <Button variant="outline" @click="load" class="rounded-sm">
          <RefreshCw class="h-4 w-4 mr-2" /> Reload
        </Button>
      </div>
    </section>

    <!-- ── Runtime metrics card (only when pipeline is running) ─────────────── -->
    <template v-if="status?.running">

      <!-- Alert banner -->
      <div
        v-if="systemAlert"
        class="flex flex-wrap items-center gap-4 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm"
      >
        <span v-if="anyCircuitOpen" class="text-destructive font-medium">
          Circuit breaker open
        </span>
        <span v-if="status!.dlqDepth > 0" class="text-destructive font-medium flex items-center gap-2">
          DLQ: {{ status!.dlqDepth }} entries
          <button
            class="rounded px-2 py-0.5 text-xs font-bold bg-destructive text-destructive-foreground hover:bg-destructive/80"
            @click="handleReplay"
          >Replay</button>
        </span>
        <span v-if="status!.walPending > 500" class="text-orange-400 font-medium">
          WAL backlog: {{ status!.walPending }}
        </span>
      </div>

      <!-- Stats row -->
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">Write rate</p>
          <p class="mt-1 font-mono text-lg font-semibold tabular-nums">
            {{ totalWriteRate.toFixed(0) }}<span class="text-xs text-muted-foreground">/s</span>
          </p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">Input rate</p>
          <p class="mt-1 font-mono text-lg font-semibold tabular-nums">
            {{ (status!.inputRate ?? 0).toFixed(0) }}<span class="text-xs text-muted-foreground">/s</span>
          </p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">Input queue</p>
          <p
            class="mt-1 font-mono text-lg font-semibold tabular-nums"
            :class="(status!.inputQueue ?? 0) > 50000 ? 'text-orange-400' : ''"
          >{{ status!.inputQueue ?? 0 }}</p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">WAL pending</p>
          <p
            class="mt-1 font-mono text-lg font-semibold tabular-nums"
            :class="(status!.walPending ?? 0) > 100 ? 'text-orange-400' : ''"
          >{{ status!.walPending ?? 0 }}</p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">DLQ</p>
          <p
            class="mt-1 font-mono text-lg font-semibold tabular-nums"
            :class="(status!.dlqDepth ?? 0) > 0 ? 'text-destructive' : ''"
          >{{ status!.dlqDepth ?? 0 }}</p>
        </div>
      </div>

      <!-- Backend cards -->
      <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <BackendCard
          v-for="b in status!.backends"
          :key="b.name"
          :backend="b"
          :retry-depth="status!.retryQueues?.[b.name] ?? 0"
        />
      </div>

      <!-- DLQ table -->
      <DLQTable
        v-if="dlq.count > 0"
        :entries="dlq.entries"
        @replay="handleReplay"
      />

    </template>

    <!-- Error -->
    <div v-if="error" class="rounded-md border border-orange-500/40 bg-orange-500/10 px-4 py-3 text-sm text-orange-400">
      {{ error }}
    </div>

  </div>
</template>
