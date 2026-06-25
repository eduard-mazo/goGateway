<script setup lang="ts">
// PlantaIec104Link — links one SSFV plant to an IEC-104 server, mirroring the
// plant's signals as 104 points. Self-contained: owns its link state and API
// calls. SSFV is the principal — deleting the plant/signal cascade-removes the
// mirrors server-side; this control only manages the link itself.
import { ref, onMounted } from 'vue'
import { toast } from 'vue-sonner'
import { api, type IEC104Server, type PlantaIEC104Link, type IEC104MirrorResult } from '@/api'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Radio, Loader2, X } from 'lucide-vue-next'
import { useConfirm } from '@/composables/useConfirm'

const props = defineProps<{ plantaId: number; servers: IEC104Server[] }>()
const { confirm } = useConfirm()

const link = ref<PlantaIEC104Link | null>(null)
const sel  = ref('')
const busy = ref(false)

async function load() {
  try {
    const r = await api.get<PlantaIEC104Link>(`/ssfv/plantas/${props.plantaId}/iec104-link`)
    link.value = r.data
    if (r.data.linked) sel.value = String(r.data.server_id)
  } catch { /* leave unknown */ }
}

async function doLink() {
  const sid = Number(sel.value)
  if (!sid) { toast.error('Selecciona un servidor IEC-104'); return }
  busy.value = true
  try {
    const r = await api.post<IEC104MirrorResult>(`/ssfv/plantas/${props.plantaId}/iec104-link`, { server_id: sid })
    toast.success(`Planta vinculada a IEC-104 — ${r.data.created?.length ?? 0} punto(s) espejados`)
    await load()
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error vinculando a IEC-104')
  } finally { busy.value = false }
}

async function doUnlink() {
  const ok = await confirm({
    title: 'Desvincular de IEC-104',
    message: 'Se eliminarán los puntos IEC-104 espejados de esta planta. ¿Continuar?',
    confirmText: 'Desvincular', variant: 'danger',
  })
  if (!ok) return
  busy.value = true
  try {
    await api.delete(`/ssfv/plantas/${props.plantaId}/iec104-link`)
    toast.success('Planta desvinculada — espejo IEC-104 eliminado')
    await load()
  } catch (e: any) {
    toast.error(e.response?.data?.error ?? 'Error desvinculando')
  } finally { busy.value = false }
}

onMounted(load)
</script>

<template>
  <div class="px-4 py-3 border-b border-border/60">
    <div class="flex items-center gap-2 mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
      <Radio class="h-3.5 w-3.5" /> Salida IEC-104
      <span v-if="link?.linked" class="font-mono font-normal text-[color:var(--epm-citrico)]">
        ({{ link?.mirror_count ?? 0 }} puntos espejados)
      </span>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <Select v-model="sel">
        <SelectTrigger class="h-7 w-56 rounded-sm text-xs">
          <SelectValue placeholder="Servidor IEC-104…" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="s in servers" :key="s.id" :value="String(s.id)">
            {{ s.name || ('Servidor ' + s.id) }} · :{{ s.port }} (ASDU {{ s.asdu_addr }})
          </SelectItem>
        </SelectContent>
      </Select>

      <Button variant="outline" size="sm" class="h-7 text-[11px] rounded-sm" :disabled="busy" @click="doLink">
        <Loader2 v-if="busy" class="h-3 w-3 mr-1 animate-spin" />
        <Radio v-else class="h-3 w-3 mr-1" />
        {{ link?.linked ? 'Actualizar espejo' : 'Vincular' }}
      </Button>

      <Button
        v-if="link?.linked"
        variant="ghost" size="sm" class="h-7 text-[11px] rounded-sm text-destructive"
        :disabled="busy" @click="doUnlink"
      >
        <X class="h-3 w-3 mr-1" /> Desvincular
      </Button>

      <span class="text-[10px] text-muted-foreground">
        Las señales SSFV se reflejan como puntos IEC-104; al borrar la planta o una señal se eliminan.
      </span>
    </div>
  </div>
</template>
