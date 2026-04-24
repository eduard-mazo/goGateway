<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { toast } from 'vue-sonner'
import { api, type Device, type Topic } from '@/api'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Trash2, Cpu, Plus } from 'lucide-vue-next'

const devices = ref<Device[]>([])
const topics = ref<Topic[]>([])
const newDev = ref<{ name: string; description: string }>({ name: '', description: '' })
const newTop = ref<{ device_id: number | null; topic: string; qos: number; enabled: boolean }>({
  device_id: null, topic: '', qos: 0, enabled: true,
})

const devById = computed(() => Object.fromEntries(devices.value.map(d => [d.id, d])))

async function reload() {
  try {
    const [d, t] = await Promise.all([
      api.get<Device[]>('/devices'),
      api.get<Topic[]>('/topics'),
    ])
    devices.value = d.data ?? []
    topics.value = t.data ?? []
  } catch (e: any) { toast.error('Load: ' + (e?.message ?? e)) }
}

async function createDevice() {
  if (!newDev.value.name.trim()) return
  try {
    await api.post('/devices', newDev.value)
    newDev.value = { name: '', description: '' }
    await reload()
    toast.success('Device added')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? e?.message ?? 'Error') }
}

async function deleteDevice(id: number) {
  if (!confirm('Delete device and all its topics + mappings?')) return
  try {
    await api.delete(`/devices/${id}`)
    await reload()
    toast.success('Deleted')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function createTopic() {
  if (!newTop.value.device_id || !newTop.value.topic.trim()) return
  try {
    await api.post('/topics', newTop.value)
    newTop.value = { device_id: newTop.value.device_id, topic: '', qos: 0, enabled: true }
    await reload()
    toast.success('Topic added')
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function toggleTopic(t: Topic) {
  try {
    await api.put(`/topics/${t.id}`, { ...t, enabled: !t.enabled })
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

async function deleteTopic(id: number) {
  if (!confirm('Delete topic (mappings will cascade)?')) return
  try {
    await api.delete(`/topics/${id}`)
    await reload()
  } catch (e: any) { toast.error(e?.response?.data?.error ?? 'Error') }
}

onMounted(reload)
</script>

<template>
  <div class="p-8 space-y-6 max-w-5xl">
    <div class="card-soft overflow-hidden relative">
      <div class="relative flex items-start gap-4 p-6">
        <div class="grid place-items-center w-11 h-11 rounded-sm bg-[color:var(--epm-bosque)] text-white">
          <Cpu class="h-5 w-5" />
        </div>
        <div>
          <div class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">Infrastructure</div>
          <h1 class="mt-1 mb-1">Devices &amp; topics</h1>
          <p class="text-sm text-muted-foreground">
            {{ devices.length }} device{{ devices.length === 1 ? '' : 's' }} ·
            {{ topics.length }} topic{{ topics.length === 1 ? '' : 's' }} subscribed
          </p>
        </div>
      </div>
    </div>

    <Card class="card-soft">
      <CardHeader>
        <CardTitle class="font-extrabold tracking-tight">Devices</CardTitle>
        <CardDescription>e.g. "Inversor 1", "Meteo 1".</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex gap-2 items-end">
          <div class="flex-1 space-y-1.5">
            <Label for="dname" class="text-[11px] uppercase tracking-[0.18em] font-bold">Name</Label>
            <Input id="dname" v-model="newDev.name" placeholder="INV_1" class="rounded-sm" @keyup.enter="createDevice" />
          </div>
          <div class="flex-1 space-y-1.5">
            <Label for="ddesc" class="text-[11px] uppercase tracking-[0.18em] font-bold">Description</Label>
            <Input id="ddesc" v-model="newDev.description" placeholder="Inversor 1" class="rounded-sm" @keyup.enter="createDevice" />
          </div>
          <Button @click="createDevice" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-5">
            <Plus class="h-4 w-4 mr-1" /> Add
          </Button>
        </div>

        <Table v-if="devices.length">
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="w-12 text-[10px] uppercase tracking-[0.2em] font-bold">ID</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Name</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Description</TableHead>
              <TableHead class="w-16"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="d in devices" :key="d.id" class="data-row border-b border-border/60">
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ d.id }}</TableCell>
              <TableCell class="font-bold">{{ d.name }}</TableCell>
              <TableCell class="text-muted-foreground text-xs">{{ d.description }}</TableCell>
              <TableCell>
                <Button variant="ghost" size="icon" @click="deleteDevice(d.id)" class="text-[color:var(--destructive)]">
                  <Trash2 class="h-4 w-4" />
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p v-else class="text-sm text-muted-foreground">No devices yet.</p>
      </CardContent>
    </Card>

    <Card class="card-soft">
      <CardHeader>
        <CardTitle class="font-extrabold tracking-tight">Topics</CardTitle>
        <CardDescription>MQTT subscription strings bound to a device.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex gap-2 items-end flex-wrap">
          <div class="w-48 space-y-1.5">
            <Label class="text-[11px] uppercase tracking-[0.18em] font-bold">Device</Label>
            <Select v-model="newTop.device_id">
              <SelectTrigger class="rounded-sm"><SelectValue placeholder="Select…" /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="d in devices" :key="d.id" :value="d.id">{{ d.name }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="flex-1 min-w-[200px] space-y-1.5">
            <Label for="tstr" class="text-[11px] uppercase tracking-[0.18em] font-bold">Topic</Label>
            <Input id="tstr" v-model="newTop.topic" placeholder="EPM/SSFV/…/INV_1" class="rounded-sm font-mono" @keyup.enter="createTopic" />
          </div>
          <div class="w-20 space-y-1.5">
            <Label for="qos" class="text-[11px] uppercase tracking-[0.18em] font-bold">QoS</Label>
            <Input id="qos" v-model.number="newTop.qos" type="number" min="0" max="2" class="rounded-sm" />
          </div>
          <Button @click="createTopic" class="bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white rounded-sm px-5">
            <Plus class="h-4 w-4 mr-1" /> Add
          </Button>
        </div>

        <Table v-if="topics.length">
          <TableHeader>
            <TableRow class="bg-[color:color-mix(in_srgb,var(--epm-citrico)_8%,transparent)]">
              <TableHead class="w-12 text-[10px] uppercase tracking-[0.2em] font-bold">ID</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Device</TableHead>
              <TableHead class="text-[10px] uppercase tracking-[0.2em] font-bold">Topic</TableHead>
              <TableHead class="w-16 text-[10px] uppercase tracking-[0.2em] font-bold">QoS</TableHead>
              <TableHead class="w-20 text-[10px] uppercase tracking-[0.2em] font-bold">On</TableHead>
              <TableHead class="w-16"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="t in topics" :key="t.id" class="data-row border-b border-border/60">
              <TableCell class="font-mono text-xs font-bold text-[color:var(--epm-bosque)]">{{ t.id }}</TableCell>
              <TableCell class="font-semibold">{{ devById[t.device_id]?.name ?? '?' }}</TableCell>
              <TableCell class="font-mono text-xs">{{ t.topic }}</TableCell>
              <TableCell class="font-mono text-xs">{{ t.qos }}</TableCell>
              <TableCell>
                <Switch :model-value="t.enabled" @update:model-value="() => toggleTopic(t)" />
              </TableCell>
              <TableCell>
                <Button variant="ghost" size="icon" @click="deleteTopic(t.id)" class="text-[color:var(--destructive)]">
                  <Trash2 class="h-4 w-4" />
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p v-else class="text-sm text-muted-foreground">No topics yet.</p>
      </CardContent>
    </Card>
  </div>
</template>
