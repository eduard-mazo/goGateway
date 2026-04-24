<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { toast } from 'vue-sonner'
import { api, type SignalMapping, type Topic } from '@/api'
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
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogTrigger,
} from '@/components/ui/dialog'
import { Pencil, Trash2, Plus, Layers } from 'lucide-vue-next'

const IEC_TYPES = [
  { v: 'M_ME_TF_1', desc: 'short float w/ time' },
  { v: 'M_ME_NC_1', desc: 'short float, no time' },
  { v: 'M_ME_NA_1', desc: 'normalized' },
  { v: 'M_ME_NB_1', desc: 'scaled int' },
  { v: 'M_SP_NA_1', desc: 'single-point (bool)' },
  { v: 'M_SP_TB_1', desc: 'single-point w/ time' },
  { v: 'M_DP_NA_1', desc: 'double-point' },
  { v: 'M_IT_NA_1', desc: 'integrated total' },
  { v: 'M_IT_TB_1', desc: 'integrated total w/ time' },
]
const iecDesc = computed(
  () => IEC_TYPES.find(t => t.v === editing.iec104_type)?.desc ?? '',
)

const mappings = ref<SignalMapping[]>([])
const topics = ref<Topic[]>([])
const loading = ref(false)

const topicById = computed(() => Object.fromEntries(topics.value.map(t => [t.id, t])))

function empty(): SignalMapping {
  return {
    id: 0, topic_id: 0, device_name: '', variable_type: '', characteristic: '',
    json_key: '', iec104_type: 'M_ME_TF_1', ioa: 0, unit: '', scale: 1.0, enabled: true,
  }
}

const dialogOpen = ref(false)
const editing = reactive<SignalMapping>(empty())
const isEdit = computed(() => editing.id > 0)

function openCreate() {
  Object.assign(editing, empty())
  // default IOA = max+1
  const max = mappings.value.reduce((m, x) => Math.max(m, x.ioa), 16384)
  editing.ioa = max + 1
  dialogOpen.value = true
}

function openEdit(m: SignalMapping) {
  Object.assign(editing, m)
  dialogOpen.value = true
}

async function reload() {
  loading.value = true
  try {
    const [m, t] = await Promise.all([
      api.get<SignalMapping[]>('/mappings'),
      api.get<Topic[]>('/topics'),
    ])
    mappings.value = m.data ?? []
    topics.value = t.data ?? []
  } catch (e: any) { toast.error('Load: ' + (e?.message ?? e)) }
  finally { loading.value = false }
}

function validate(): string | null {
  if (!editing.topic_id) return 'Topic is required'
  if (!editing.json_key.trim()) return 'JSON key is required'
  if (!editing.iec104_type) return 'IEC 104 type required'
  if (!Number.isInteger(editing.ioa) || editing.ioa <= 0) return 'IOA must be positive int'
  const dup = mappings.value.find(x => x.ioa === editing.ioa && x.id !== editing.id)
  if (dup) return `IOA ${editing.ioa} already used by mapping #${dup.id}`
  if (!editing.scale || editing.scale === 0) editing.scale = 1.0
  return null
}

async function save() {
  const err = validate()
  if (err) { toast.error(err); return }
  try {
    if (isEdit.value) {
      await api.put(`/mappings/${editing.id}`, editing)
      toast.success(`Mapping #${editing.id} updated`)
    } else {
      await api.post('/mappings', editing)
      toast.success('Mapping created')
    }
    dialogOpen.value = false
    await reload()
  } catch (e: any) {
    toast.error(e?.response?.data?.error ?? e?.message ?? 'Save failed')
  }
}

async function toggleEnabled(m: SignalMapping) {
  try {
    await api.put(`/mappings/${m.id}`, { ...m, enabled: !m.enabled })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function del(m: SignalMapping) {
  if (!confirm(`Delete mapping IOA=${m.ioa} (${m.json_key})?`)) return
  try {
    await api.delete(`/mappings/${m.id}`)
    await reload()
    toast.success('Deleted')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

onMounted(reload)
</script>

<template>
  <div class="p-8 space-y-6">
    <!-- Header -->
    <div class="flex items-start justify-between border-b border-border pb-5">
      <div class="flex items-start gap-3">
        <div class="grid place-items-center w-9 h-9 rounded-sm bg-[color:var(--epm-bosque)] text-white">
          <Layers class="h-4 w-4" />
        </div>
        <div>
          <div class="text-[10px] uppercase tracking-[0.24em] font-bold text-[color:var(--epm-bosque)]">Configuration</div>
          <h1 class="mt-1 mb-1 text-2xl font-extrabold">Signal mapping</h1>
          <p class="text-sm text-muted-foreground">MQTT JSON key → IEC 104 point · {{ mappings.length }} active</p>
        </div>
      </div>
      <Dialog v-model:open="dialogOpen">
        <DialogTrigger as-child>
          <Button @click="openCreate" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-4">
            <Plus class="h-4 w-4 mr-1" /> New mapping
          </Button>
        </DialogTrigger>
        <DialogContent class="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{{ isEdit ? `Edit mapping #${editing.id}` : 'New mapping' }}</DialogTitle>
            <DialogDescription>Bind a JSON key from an MQTT topic to an IEC 104 point.</DialogDescription>
          </DialogHeader>

          <div class="grid grid-cols-6 gap-x-4 gap-y-3 py-2">
            <div class="col-span-6 space-y-1.5">
              <Label>Topic</Label>
              <Select v-model="editing.topic_id">
                <SelectTrigger class="w-full"><SelectValue placeholder="Select topic…" /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="t in topics" :key="t.id" :value="t.id">
                    {{ t.topic }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div class="col-span-3 space-y-1.5">
              <Label>Device (label)</Label>
              <Input v-model="editing.device_name" placeholder="Inversor 1" />
            </div>
            <div class="col-span-3 space-y-1.5">
              <Label>JSON key</Label>
              <Input v-model="editing.json_key" placeholder="IA" />
            </div>

            <div class="col-span-3 space-y-1.5">
              <Label>Variable type</Label>
              <Input v-model="editing.variable_type" placeholder="Corr. Fase A" />
            </div>
            <div class="col-span-3 space-y-1.5">
              <Label>Characteristic</Label>
              <Input v-model="editing.characteristic" placeholder="Corriente AC" />
            </div>

            <div class="col-span-4 space-y-1.5 min-w-0">
              <Label>IEC 104 type</Label>
              <Select v-model="editing.iec104_type">
                <SelectTrigger class="w-full font-mono">
                  <SelectValue placeholder="Select…" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="t in IEC_TYPES" :key="t.v" :value="t.v" class="font-mono">
                    {{ t.v }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <p v-if="iecDesc" class="text-[11px] text-muted-foreground mt-1 truncate">{{ iecDesc }}</p>
            </div>
            <div class="col-span-2 space-y-1.5">
              <Label>IOA</Label>
              <Input v-model.number="editing.ioa" type="number" />
            </div>

            <div class="col-span-3 space-y-1.5">
              <Label>Unit</Label>
              <Input v-model="editing.unit" placeholder="A, V, kW…" />
            </div>
            <div class="col-span-3 space-y-1.5">
              <Label>Scale</Label>
              <Input v-model.number="editing.scale" type="number" step="0.001" />
            </div>

            <div class="col-span-6 flex items-center gap-3 pt-2">
              <Switch id="en" v-model="editing.enabled" />
              <Label for="en">Enabled</Label>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" @click="dialogOpen = false" class="rounded-sm">Cancel</Button>
            <Button @click="save" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm">
              {{ isEdit ? 'Save changes' : 'Create' }}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>

    <Card class="card-soft">
      <CardHeader>
        <CardTitle>Mappings</CardTitle>
        <CardDescription>
          {{ mappings.length }} mapping{{ mappings.length === 1 ? '' : 's' }} across
          {{ topics.length }} topic{{ topics.length === 1 ? '' : 's' }}.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div v-if="!topics.length" class="text-sm text-muted-foreground py-4">
          No topics configured. Add devices + topics first under <strong>Devices &amp; Topics</strong>.
        </div>
        <Table v-else>
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="w-14 text-[10px] uppercase tracking-[0.2em] font-bold">IOA</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Topic</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Device</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Variable type</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">JSON key</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">IEC 104</TableHead>
              <TableHead class="text-right text-[10px] uppercase tracking-[0.2em] font-bold">Scale</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Unit</TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">On</TableHead>
              <TableHead class="w-24 text-right text-[10px] uppercase tracking-[0.2em] font-bold">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="m in mappings" :key="m.id" class="data-row border-b border-border/60">
              <TableCell class="font-mono font-bold text-[color:var(--epm-bosque)]">{{ m.ioa }}</TableCell>
              <TableCell class="font-mono text-xs truncate max-w-[260px]">
                {{ topicById[m.topic_id]?.topic ?? '—' }}
              </TableCell>
              <TableCell class="font-semibold">{{ m.device_name }}</TableCell>
              <TableCell class="text-muted-foreground text-xs">{{ m.variable_type }}</TableCell>
              <TableCell class="font-mono text-xs">{{ m.json_key }}</TableCell>
              <TableCell>
                <span class="inline-flex items-center rounded-sm px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-[color:color-mix(in_srgb,var(--epm-citrico)_22%,transparent)] text-[color:var(--epm-bosque)]">
                  {{ m.iec104_type }}
                </span>
              </TableCell>
              <TableCell class="text-right font-mono text-xs tabular">{{ m.scale }}</TableCell>
              <TableCell class="text-xs text-muted-foreground">{{ m.unit }}</TableCell>
              <TableCell>
                <Switch :model-value="m.enabled" @update:model-value="() => toggleEnabled(m)" />
              </TableCell>
              <TableCell class="text-right">
                <Button variant="ghost" size="icon" @click="openEdit(m)"><Pencil class="h-4 w-4" /></Button>
                <Button variant="ghost" size="icon" @click="del(m)" class="text-[color:var(--destructive)]"><Trash2 class="h-4 w-4" /></Button>
              </TableCell>
            </TableRow>
            <TableRow v-if="!mappings.length">
              <TableCell colspan="10" class="text-center text-muted-foreground py-6">
                No mappings yet. Click "New mapping".
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
