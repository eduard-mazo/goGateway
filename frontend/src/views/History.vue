<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { toast } from 'vue-sonner'
import { api, type History, type SignalMapping } from '@/api'
import { t } from '@/i18n'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { RefreshCw, Download, History as HistoryIcon } from 'lucide-vue-next'

const rows = ref<History[]>([])
const mappings = ref<SignalMapping[]>([])
const mappingId = ref<number | 'all'>('all')
const limit = ref(200)
const autoRefresh = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

const mapByID = computed(() => Object.fromEntries(mappings.value.map(m => [m.id, m])))

async function load() {
  try {
    const params: Record<string, string> = { limit: String(limit.value) }
    if (mappingId.value !== 'all') params.mapping_id = String(mappingId.value)
    const { data } = await api.get<History[]>('/history', { params })
    rows.value = data ?? []
  } catch (e: any) { toast.error('Load: ' + (e?.message ?? e)) }
}

async function loadMappings() {
  try {
    const { data } = await api.get<SignalMapping[]>('/mappings')
    mappings.value = data ?? []
  } catch { /* ignore */ }
}

function toggleAuto(v: boolean) {
  autoRefresh.value = v
  if (timer) { clearInterval(timer); timer = null }
  if (v) timer = setInterval(load, 2000)
}

function exportCSV() {
  const lines = ['timestamp,mapping_id,signal_path,value,quality']
  for (const r of rows.value) {
    lines.push([r.timestamp, r.mapping_id, JSON.stringify(r.signal_path), r.value, r.quality].join(','))
  }
  const blob = new Blob([lines.join('\n')], { type: 'text/csv' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `history-${new Date().toISOString()}.csv`
  a.click()
  URL.revokeObjectURL(a.href)
}

function fmt(ts: string) {
  try { return new Date(ts).toLocaleString() } catch { return ts }
}

onMounted(async () => { await loadMappings(); await load() })
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 max-w-6xl">
    <!-- Hero -->
    <div class="card-soft overflow-hidden relative">
      <div class="relative flex items-start gap-4 p-6">
        <div class="grid place-items-center w-11 h-11 rounded-sm bg-[color:var(--epm-bosque)] text-white">
          <HistoryIcon class="h-5 w-5" />
        </div>
        <div class="flex-1">
          <div class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">{{ t.history.subtitle }}</div>
          <h1 class="mt-1 mb-1">{{ t.history.title }}</h1>
          <p class="text-sm text-muted-foreground">
            {{ rows.length }} {{ rows.length === 1 ? t.history.sample : t.history.samples_pl }} {{ t.history.desc }}
          </p>
        </div>
      </div>
    </div>

    <!-- Filter -->
    <Card class="card-soft">
      <CardHeader>
        <CardTitle class="font-extrabold tracking-tight">{{ t.history.filter }}</CardTitle>
        <CardDescription>{{ t.history.filterDesc }}</CardDescription>
      </CardHeader>
      <CardContent class="flex items-end gap-3 flex-wrap">
        <div class="w-72 space-y-1.5">
          <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.history.mapping }}</Label>
          <Select v-model="mappingId">
            <SelectTrigger class="rounded-sm"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{{ t.history.allMappings }}</SelectItem>
              <SelectItem v-for="m in mappings" :key="m.id" :value="m.id">
                #{{ m.id }} · IOA {{ m.ioa }} · {{ m.json_key }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="w-28 space-y-1.5">
          <Label for="lim" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.history.limit }}</Label>
          <Input id="lim" v-model.number="limit" type="number" min="10" max="10000" class="rounded-sm" />
        </div>
        <Button @click="load"
                class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-5">
          <RefreshCw class="h-4 w-4 mr-1" /> {{ t.history.refresh }}
        </Button>
        <Button variant="outline" @click="exportCSV" :disabled="!rows.length" class="rounded-sm">
          <Download class="h-4 w-4 mr-1" /> {{ t.history.exportCsv }}
        </Button>
        <div class="flex items-center gap-2 ml-auto rounded-sm px-3 py-1.5 border border-border/60
                    bg-[color:color-mix(in_srgb,var(--epm-citrico)_10%,transparent)]">
          <Switch id="auto" :model-value="autoRefresh" @update:model-value="toggleAuto" />
          <Label for="auto" class="text-[11px] uppercase tracking-[0.18em] font-bold">{{ t.history.autoRefresh }}</Label>
        </div>
      </CardContent>
    </Card>

    <!-- Samples -->
    <Card class="card-soft">
      <CardHeader>
        <CardTitle class="font-extrabold tracking-tight">{{ t.history.samples }}</CardTitle>
        <CardDescription>{{ rows.length }} {{ rows.length === 1 ? t.history.row : t.history.rows }}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="w-52 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.history.timestamp }}</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.history.signal }}</TableHead>
              <TableHead class="w-24 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.history.ioa }}</TableHead>
              <TableHead class="text-right text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.history.value }}</TableHead>
              <TableHead class="w-24 text-[10px] uppercase tracking-[0.2em] font-bold">{{ t.history.quality }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="r in rows" :key="r.id" class="data-row border-b border-border/60">
              <TableCell class="font-mono text-xs text-muted-foreground">{{ fmt(r.timestamp) }}</TableCell>
              <TableCell class="font-mono text-xs">{{ r.signal_path }}</TableCell>
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">
                {{ mapByID[r.mapping_id]?.ioa ?? '—' }}
              </TableCell>
              <TableCell class="text-right font-mono font-semibold">{{ r.value }}</TableCell>
              <TableCell>
                <span v-if="r.quality === 0"
                      class="inline-flex items-center gap-1.5 rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]
                             bg-[color:color-mix(in_srgb,var(--epm-citrico)_18%,transparent)]
                             text-[color:var(--epm-bosque-deep)]
                             border border-[color:color-mix(in_srgb,var(--epm-bosque)_35%,transparent)]">
                  <span class="h-1.5 w-1.5 rounded-sm bg-[color:var(--epm-bosque)]" />
                  {{ t.history.good }}
                </span>
                <span v-else
                      class="inline-flex items-center gap-1.5 rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em]
                             bg-amber-500/15 text-amber-700 dark:text-amber-300
                             border border-amber-500/40 font-mono">
                  <span class="h-1.5 w-1.5 rounded-sm bg-amber-500" />
                  0x{{ r.quality.toString(16) }}
                </span>
              </TableCell>
            </TableRow>
            <TableRow v-if="!rows.length">
              <TableCell colspan="5" class="text-center text-muted-foreground py-8">
                {{ t.history.noSamples }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
