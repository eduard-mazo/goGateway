<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { toast } from 'vue-sonner'
import { api, type GatewaySignal, type Topic } from '@/api'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
} from '@/components/ui/dialog'
import { Plus, Pencil, Trash2, Layers, Database, Search, Radio, ArrowRight } from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const { confirm } = useConfirm()

const signals = ref<GatewaySignal[]>([])
const topics = ref<Topic[]>([])
const loading = ref(false)
const search = ref('')

const topicById = computed(() => Object.fromEntries(topics.value.map(t => [t.id, t])))

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return signals.value
  return signals.value.filter(s => {
    const topic = topicById.value[s.topic_id]
    return [s.name, s.json_key, s.metric_name, s.variable_type, s.unit, topic?.topic ?? '']
      .some(v => v.toLowerCase().includes(q))
  })
})

// Topics with no signals yet (for the onboarding quick-add row)
const topicsWithSignals = computed(() => new Set(signals.value.map(s => s.topic_id)))
const unseenTopics = computed(() => topics.value.filter(t => !topicsWithSignals.value.has(t.id)))

function empty(): GatewaySignal {
  return {
    id: 0, topic_id: 0, name: '', json_key: '', metric_name: '',
    quality_key: '', variable_type: '', characteristic: '', unit: '',
    scale: 1.0, persist_to_db: false, enabled: true,
  }
}

// ── Multi-select ─────────────────────────────────────────────────────────────
const selectedSignals = reactive(new Set<number>())
const allFilteredSelected = computed(() =>
  filtered.value.length > 0 && filtered.value.every(s => selectedSignals.has(s.id))
)
function toggleSelectAllSignals() {
  if (allFilteredSelected.value) filtered.value.forEach(s => selectedSignals.delete(s.id))
  else filtered.value.forEach(s => selectedSignals.add(s.id))
}
function toggleSelectSignal(id: number) {
  if (selectedSignals.has(id)) selectedSignals.delete(id)
  else selectedSignals.add(id)
}
async function delSelectedSignals() {
  const ids = [...selectedSignals]
  const ok = await confirm({
    title: 'Eliminar señales',
    message: `Elimina ${ids.length} señal${ids.length === 1 ? '' : 'es'}. Los mapeos IEC-104 que las referencien quedarán sin signal_id.`,
    variant: 'danger',
    confirmText: `Eliminar ${ids.length}`,
  })
  if (!ok) return
  try {
    await Promise.all(ids.map(id => api.delete(`/gateway-signals/${id}`)))
    ids.forEach(id => selectedSignals.delete(id))
    await reload()
    toast.success(`${ids.length} señal${ids.length === 1 ? '' : 'es'} eliminada${ids.length === 1 ? '' : 's'}`)
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

const dialogOpen = ref(false)
const editing = reactive<GatewaySignal>(empty())
const isEdit = computed(() => editing.id > 0)

function openCreate(presetTopicId = 0) {
  Object.assign(editing, empty())
  if (presetTopicId) editing.topic_id = presetTopicId
  else if (topics.value.length === 1) editing.topic_id = topics.value[0].id
  dialogOpen.value = true
}

function openEdit(s: GatewaySignal) {
  Object.assign(editing, { ...s })
  dialogOpen.value = true
}

function validate(): string | null {
  if (!editing.topic_id) return 'Topic es requerido'
  if (!editing.name.trim()) return 'Nombre es requerido'
  if (!editing.json_key.trim() && !editing.metric_name.trim()) return 'JSON key o Metric name es requerido'
  return null
}

async function save() {
  const err = validate()
  if (err) { toast.error(err); return }
  if (editing.scale === 0) editing.scale = 1.0
  try {
    if (isEdit.value) {
      await api.put(`/gateway-signals/${editing.id}`, editing)
      toast.success(`Señal "${editing.name}" actualizada`)
    } else {
      await api.post('/gateway-signals', editing)
      toast.success(`Señal "${editing.name}" creada`)
    }
    dialogOpen.value = false
    await reload()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Error')
  }
}

async function toggleEnabled(s: GatewaySignal) {
  try {
    await api.put(`/gateway-signals/${s.id}`, { ...s, enabled: !s.enabled })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function togglePersist(s: GatewaySignal) {
  try {
    await api.put(`/gateway-signals/${s.id}`, { ...s, persist_to_db: !s.persist_to_db })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function del(s: GatewaySignal) {
  const ok = await confirm({
    title: 'Eliminar señal',
    message: 'Los mapeos IEC-104 que la referencian quedarán sin signal_id (no se eliminan).',
    detail: `${s.name} · tópico #${s.topic_id}`,
    variant: 'danger',
    confirmText: 'Eliminar',
  })
  if (!ok) return
  try {
    await api.delete(`/gateway-signals/${s.id}`)
    await reload()
    toast.success('Señal eliminada')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function reload() {
  loading.value = true
  try {
    const [sg, tp] = await Promise.all([
      api.get<GatewaySignal[]>('/gateway-signals'),
      api.get<Topic[]>('/topics'),
    ])
    signals.value = sg.data ?? []
    topics.value = tp.data ?? []
  } catch (e: any) { toast.error('Load: ' + (e?.message ?? e)) }
  finally { loading.value = false }
}

onMounted(reload)
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6">
    <!-- Page header -->
    <div class="flex items-start justify-between border-b border-border pb-5">
      <div class="flex items-start gap-3">
        <div class="grid place-items-center w-9 h-9 rounded-sm bg-[color:var(--epm-bosque)] text-white shrink-0">
          <Layers class="h-4 w-4" />
        </div>
        <div>
          <div class="text-[10px] uppercase tracking-[0.24em] font-bold text-[color:var(--epm-bosque)]">Paso 2 de 4</div>
          <h1 class="mt-1 mb-1 text-2xl font-extrabold">Señales SSFV</h1>
          <p class="text-sm text-muted-foreground">
            Define señales lógicas desde los tópicos MQTT. Activa
            <strong>Persistir en DB</strong> para enviar muestras al pipeline TSDB (TimescaleDB / VictoriaMetrics).
          </p>
        </div>
      </div>
      <Button
        class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-4"
        @click="openCreate()"
      >
        <Plus class="h-4 w-4 mr-1" /> Nueva señal
      </Button>
    </div>

    <!-- No topics configured at all -->
    <div v-if="!topics.length" class="card-soft p-8 text-center space-y-3">
      <Radio class="h-8 w-8 text-muted-foreground mx-auto" />
      <p class="font-semibold">Sin tópicos MQTT configurados</p>
      <p class="text-sm text-muted-foreground">
        Ve al <strong>Paso 1 – Tópicos y Suscripciones</strong> y agrega al menos un tópico antes de definir señales.
      </p>
    </div>

    <template v-else>
      <!-- Onboarding prompt when no signals exist yet -->
      <div v-if="!signals.length" class="card-soft overflow-hidden">
        <div class="px-6 py-5 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]">
          <h3 class="font-bold text-sm flex items-center gap-2">
            <ArrowRight class="h-4 w-4 text-[color:var(--epm-bosque)]" />
            Empieza definiendo señales desde tus tópicos
          </h3>
          <p class="text-xs text-muted-foreground mt-1">
            Cada señal extrae un campo del payload MQTT y lo normaliza para IEC-104 e histórico.
          </p>
        </div>
        <div class="divide-y divide-border">
          <div
            v-for="t in topics"
            :key="t.id"
            class="flex items-center gap-3 px-6 py-3 hover:bg-muted/40 transition-colors"
          >
            <Radio class="h-4 w-4 text-muted-foreground shrink-0" />
            <span class="flex-1 font-mono text-sm truncate">{{ t.topic }}</span>
            <span
              class="rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider"
              :class="t.payload_format === 'sparkplug'
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'"
            >{{ t.payload_format === 'sparkplug' ? 'SpB' : 'JSON' }}</span>
            <Button
              size="sm" variant="outline"
              class="rounded-sm text-xs h-7 shrink-0"
              @click="openCreate(t.id)"
            >
              <Plus class="h-3 w-3 mr-1" /> Agregar señal
            </Button>
          </div>
        </div>
      </div>

      <!-- Signals table (shown once at least one signal exists) -->
      <template v-else>
        <!-- Toolbar -->
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative flex-1 min-w-[200px] max-w-md">
            <Search class="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              v-model="search"
              placeholder="Buscar señal, key, tópico…"
              class="pl-8 rounded-sm font-mono text-xs"
            />
          </div>
          <div class="flex items-center gap-2 ml-auto">
            <span class="text-[11px] text-muted-foreground font-mono">
              {{ filtered.length }} / {{ signals.length }} señales
            </span>
            <Button
              v-if="selectedSignals.size > 0"
              size="sm" variant="destructive"
              class="rounded-sm h-7 text-xs"
              @click="delSelectedSignals"
            >
              <Trash2 class="h-3.5 w-3.5 mr-1" /> Eliminar ({{ selectedSignals.size }})
            </Button>
          </div>
        </div>

        <!-- Quick-add strip for topics with no signal yet -->
        <div
          v-if="unseenTopics.length"
          class="rounded-sm border border-dashed border-[color:var(--epm-citrico)] bg-[color:color-mix(in_srgb,var(--epm-citrico)_5%,transparent)] px-4 py-3 flex flex-wrap gap-2 items-center"
        >
          <span class="text-[11px] font-bold uppercase tracking-[0.18em] text-[color:var(--epm-bosque)] shrink-0">
            Tópicos sin señales:
          </span>
          <button
            v-for="t in unseenTopics"
            :key="t.id"
            class="inline-flex items-center gap-1.5 font-mono text-xs rounded-sm border border-border bg-card px-2 py-0.5 hover:border-[color:var(--epm-bosque)] hover:text-[color:var(--epm-bosque)] transition-colors"
            @click="openCreate(t.id)"
          >
            <Plus class="h-3 w-3" /> {{ t.topic }}
          </button>
        </div>

        <Card class="card-soft">
          <CardHeader class="pb-3">
            <CardTitle>Señales definidas</CardTitle>
            <CardDescription>
              Cada señal normaliza un campo del payload MQTT hacia el pipeline interno.
              La columna <strong>DB</strong> indica si los valores se envían al pipeline TSDB (TimescaleDB / VictoriaMetrics).
            </CardDescription>
          </CardHeader>
          <CardContent class="p-0">
            <div class="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
                    <TableHead class="w-10 pl-4">
                      <input
                        type="checkbox"
                        class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                        :checked="allFilteredSelected"
                        :indeterminate="selectedSignals.size > 0 && !allFilteredSelected"
                        @change="toggleSelectAllSignals"
                      />
                    </TableHead>
                    <TableHead class="pl-2 text-[10px] uppercase tracking-[0.2em] font-bold">Nombre</TableHead>
                    <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Tópico</TableHead>
                    <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Señal / Key</TableHead>
                    <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Variable</TableHead>
                    <TableHead class="text-right text-[10px] uppercase tracking-[0.2em] font-bold">Escala</TableHead>
                    <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Ud.</TableHead>
                    <TableHead class="w-20 text-center text-[10px] uppercase tracking-[0.2em] font-bold">
                      <span class="inline-flex items-center gap-1"><Database class="h-3 w-3" /> DB</span>
                    </TableHead>
                    <TableHead class="w-14 text-[10px] uppercase tracking-[0.2em] font-bold">On</TableHead>
                    <TableHead class="w-20 pr-6 text-right text-[10px] uppercase tracking-[0.2em] font-bold">Acc.</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-if="!filtered.length">
                    <TableCell colspan="10" class="text-center text-muted-foreground py-8 text-sm">
                      {{ search ? 'Sin coincidencias.' : 'Sin señales.' }}
                    </TableCell>
                  </TableRow>
                  <TableRow
                    v-for="s in filtered"
                    :key="s.id"
                    class="data-row border-b border-border/60"
                    :class="selectedSignals.has(s.id) ? 'bg-[color:color-mix(in_srgb,var(--epm-citrico)_6%,transparent)]' : ''"
                  >
                    <TableCell class="pl-4">
                      <input
                        type="checkbox"
                        class="h-4 w-4 rounded-sm border border-border cursor-pointer accent-[color:var(--epm-bosque)]"
                        :checked="selectedSignals.has(s.id)"
                        @change="toggleSelectSignal(s.id)"
                      />
                    </TableCell>
                    <TableCell class="pl-2 font-semibold text-sm">{{ s.name }}</TableCell>
                    <TableCell class="font-mono text-xs text-muted-foreground truncate max-w-[180px]">
                      {{ topicById[s.topic_id]?.topic ?? `#${s.topic_id}` }}
                    </TableCell>
                    <TableCell class="font-mono text-xs">
                      <template v-if="s.metric_name">
                        <span class="text-blue-600 dark:text-blue-400">{{ s.metric_name }}</span>
                        <span class="text-[10px] text-muted-foreground ml-1">spB</span>
                      </template>
                      <template v-else>{{ s.json_key }}</template>
                    </TableCell>
                    <TableCell class="text-xs text-muted-foreground">{{ s.variable_type || '—' }}</TableCell>
                    <TableCell class="text-right font-mono text-xs tabular-nums">{{ s.scale }}</TableCell>
                    <TableCell class="text-xs text-muted-foreground">{{ s.unit || '—' }}</TableCell>
                    <TableCell class="text-center">
                      <button
                        type="button"
                        class="inline-flex items-center justify-center gap-1 rounded-sm px-2 py-1 text-[10px] font-bold uppercase tracking-wider transition-colors border w-14"
                        :class="s.persist_to_db
                          ? 'bg-[color:color-mix(in_srgb,var(--epm-bosque)_12%,transparent)] border-[color:var(--epm-bosque)] text-[color:var(--epm-bosque)]'
                          : 'border-border text-muted-foreground hover:border-[color:var(--epm-bosque)] hover:text-[color:var(--epm-bosque)]'"
                        :title="s.persist_to_db ? 'Click para desactivar pipeline TSDB' : 'Click para activar pipeline TSDB'"
                        @click="togglePersist(s)"
                      >
                        <Database class="h-3 w-3 shrink-0" />
                        {{ s.persist_to_db ? 'Sí' : 'No' }}
                      </button>
                    </TableCell>
                    <TableCell>
                      <Switch :model-value="s.enabled" @update:model-value="() => toggleEnabled(s)" />
                    </TableCell>
                    <TableCell class="pr-6 text-right whitespace-nowrap">
                      <Button variant="ghost" size="icon" @click="openEdit(s)"><Pencil class="h-4 w-4" /></Button>
                      <Button variant="ghost" size="icon" @click="del(s)" class="text-[color:var(--destructive)]"><Trash2 class="h-4 w-4" /></Button>
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </template>
    </template>

    <!-- ── Create / Edit dialog ──────────────────────────────────────────── -->
    <Dialog v-model:open="dialogOpen">
      <DialogContent class="!max-w-lg sm:!max-w-xl p-0 overflow-hidden">
        <!-- Accent header -->
        <div class="px-6 pt-6 pb-4 border-b border-border bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
          <DialogHeader class="text-left space-y-1">
            <DialogTitle class="text-lg font-extrabold tracking-tight flex items-center gap-2">
              <Layers class="h-4 w-4 text-[color:var(--epm-bosque)]" />
              {{ isEdit ? `Editar señal #${editing.id}` : 'Nueva señal SSFV' }}
            </DialogTitle>
            <DialogDescription class="text-xs">
              Define la señal lógica: tópico fuente, campo del payload y si se envía al pipeline TSDB.
            </DialogDescription>
          </DialogHeader>
        </div>

        <!-- Body -->
        <div class="px-6 py-5 grid grid-cols-6 gap-x-4 gap-y-4">
          <!-- Topic selector -->
          <div class="col-span-6 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Tópico fuente</Label>
            <Select v-model="editing.topic_id">
              <SelectTrigger class="w-full font-mono text-xs">
                <SelectValue placeholder="Selecciona un tópico…" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="t in topics" :key="t.id" :value="t.id" class="font-mono text-xs">
                  <span
                    class="mr-2 inline-block rounded-sm px-1.5 py-0.5 text-[9px] font-bold uppercase"
                    :class="t.payload_format === 'sparkplug'
                      ? 'bg-blue-100 text-blue-700'
                      : 'bg-emerald-100 text-emerald-700'"
                  >{{ t.payload_format === 'sparkplug' ? 'SpB' : 'JSON' }}</span>
                  {{ t.topic }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- Name -->
          <div class="col-span-6 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Nombre de la señal</Label>
            <Input v-model="editing.name" placeholder="Potencia activa INV-1" />
          </div>

          <!-- JSON key / Sparkplug metric -->
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">
              JSON key
              <span class="ml-1 normal-case font-normal text-muted-foreground tracking-normal">modo JSON</span>
            </Label>
            <Input v-model="editing.json_key" placeholder="P" class="font-mono" />
          </div>
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">
              Metric name
              <span class="ml-1 normal-case font-normal text-muted-foreground tracking-normal">Sparkplug B</span>
            </Label>
            <Input v-model="editing.metric_name" placeholder="outputs/power" class="font-mono" />
          </div>

          <!-- Quality key + variable type -->
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">
              Quality key
              <span class="ml-1 normal-case font-normal text-muted-foreground tracking-normal">opcional</span>
            </Label>
            <Input v-model="editing.quality_key" placeholder="quality" class="font-mono" />
          </div>
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Tipo de variable</Label>
            <Input v-model="editing.variable_type" placeholder="Potencia activa" />
          </div>

          <!-- Unit + Scale -->
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Unidad</Label>
            <Input v-model="editing.unit" placeholder="kW, A, V…" />
          </div>
          <div class="col-span-3 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Escala</Label>
            <Input v-model.number="editing.scale" type="number" step="0.001" />
          </div>

          <!-- Persist to DB — prominent -->
          <div class="col-span-6 flex items-center justify-between rounded-sm border border-border px-4 py-3 bg-muted/30">
            <div>
              <div class="font-semibold text-sm flex items-center gap-2">
                <Database class="h-4 w-4 text-[color:var(--epm-bosque)]" />
                Persistir en pipeline TSDB
              </div>
              <p class="text-xs text-muted-foreground mt-0.5">
                Envía cada muestra al pipeline TSDB (TimescaleDB / VictoriaMetrics). Desactiva si solo necesitas reenvío IEC-104.
              </p>
            </div>
            <Switch id="persist" v-model="editing.persist_to_db" />
          </div>

          <!-- Enabled -->
          <div class="col-span-6 flex items-center gap-3">
            <Switch id="en" v-model="editing.enabled" />
            <Label for="en" class="text-sm">Habilitada</Label>
          </div>
        </div>

        <!-- Footer -->
        <div class="px-6 pb-6 pt-4 border-t border-border flex justify-end gap-2">
          <Button variant="outline" class="rounded-sm" @click="dialogOpen = false">Cancelar</Button>
          <Button
            class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm"
            @click="save"
          >
            {{ isEdit ? 'Guardar cambios' : 'Crear señal' }}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  </div>
</template>
