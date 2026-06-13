<script setup lang="ts">
// SignalTree renders an equipo's instanced signals as a collapsible folder tree
// built from each signal's nombre_instancia path (the FIWARE folder/channel
// path between the entity and the leaf attribute). The leaf node IS the signal;
// its codigo_senal is the last segment of the metric path. 'default' instances
// (flat signals) sit at the root. Each leaf carries a delete action wired to
// DELETE /ssfv/asignaciones/{equisenal_id}.
import { computed, ref } from 'vue'
import { ChevronRight, Folder, Trash2, Radio } from 'lucide-vue-next'
import Button from '@/components/ui/button/Button.vue'

type Signal = {
  equisenal_id: number
  nombre_instancia: string
  senal_nombre: string
  codigo_senal: string
  unidad?: string
  tipo_valor?: string
  es_alarma?: boolean
  activo?: boolean
}
type TreeNode = {
  name: string
  path: string
  children: TreeNode[]
  signals: Signal[]
}

const props = withDefaults(defineProps<{
  signals?: Signal[]   // top-level call: raw flat list, built into a tree here
  node?: TreeNode      // recursive call: a pre-built branch
  depth?: number
}>(), { depth: 0 })

const emit = defineEmits<{ (e: 'delete', s: Signal): void }>()

// Build a tree from the flat signal list. 'default' / empty instance ⇒ root leaf.
function buildTree(signals: Signal[]): TreeNode {
  const root: TreeNode = { name: '', path: '', children: [], signals: [] }
  const findChild = (n: TreeNode, seg: string) => n.children.find(c => c.name === seg)
  for (const s of signals) {
    const inst = (s.nombre_instancia ?? '').trim()
    const segs = (!inst || inst === 'default') ? [] : inst.split('/').map(p => p.trim()).filter(Boolean)
    let cur = root
    for (const seg of segs) {
      let child = findChild(cur, seg)
      if (!child) { child = { name: seg, path: cur.path ? cur.path + '/' + seg : seg, children: [], signals: [] }; cur.children.push(child) }
      cur = child
    }
    cur.signals.push(s)
  }
  const sortRec = (n: TreeNode) => {
    n.children.sort((a, b) => a.name.localeCompare(b.name))
    n.signals.sort((a, b) => (a.senal_nombre || a.codigo_senal).localeCompare(b.senal_nombre || b.codigo_senal))
    n.children.forEach(sortRec)
  }
  sortRec(root)
  return root
}

const root = computed<TreeNode>(() => props.node ?? buildTree(props.signals ?? []))

// Folder expand state (default expanded).
const collapsed = ref<Set<string>>(new Set())
function toggle(path: string) {
  const next = new Set(collapsed.value)
  next.has(path) ? next.delete(path) : next.add(path)
  collapsed.value = next
}
const isOpen = (path: string) => !collapsed.value.has(path)
</script>

<template>
  <div :class="depth === 0 ? 'space-y-0.5' : 'space-y-0.5'">
    <!-- Folders -->
    <div v-for="folder in root.children" :key="'f:' + folder.path">
      <button
        type="button"
        class="flex items-center gap-1.5 w-full text-left px-1.5 py-1 rounded-sm hover:bg-muted/40 transition-colors"
        @click="toggle(folder.path)"
      >
        <ChevronRight
          class="h-3 w-3 shrink-0 text-muted-foreground transition-transform"
          :class="isOpen(folder.path) ? 'rotate-90' : ''"
        />
        <Folder class="h-3 w-3 shrink-0 text-[color:var(--epm-citrico)]" />
        <span class="font-mono text-[11px] font-semibold text-foreground">{{ folder.name }}</span>
        <span class="text-[10px] text-muted-foreground">
          {{ folder.children.length + folder.signals.length }}
        </span>
      </button>
      <div v-if="isOpen(folder.path)" class="ml-3 pl-1 border-l border-border/50">
        <SignalTree :node="folder" :depth="depth + 1" @delete="emit('delete', $event)" />
      </div>
    </div>

    <!-- Leaf signals at this level -->
    <div
      v-for="s in root.signals"
      :key="'s:' + s.equisenal_id"
      class="group flex items-center gap-2 px-1.5 py-1 rounded-sm hover:bg-muted/30"
      :class="s.activo === false ? 'opacity-50' : ''"
    >
      <Radio class="h-3 w-3 shrink-0 text-muted-foreground" />
      <span class="text-xs font-medium truncate">{{ s.senal_nombre || s.codigo_senal }}</span>
      <span class="font-mono text-[10px] text-muted-foreground truncate">{{ s.codigo_senal }}</span>
      <span v-if="s.unidad" class="text-[10px] text-muted-foreground">· {{ s.unidad }}</span>
      <span v-if="s.es_alarma" class="text-[10px] px-1.5 py-0.5 rounded-sm bg-red-500/15 text-red-400 font-semibold">Alarma</span>
      <span v-else-if="s.tipo_valor" class="text-[10px] text-muted-foreground">{{ s.tipo_valor }}</span>
      <span class="ml-auto font-mono text-[10px] px-1.5 py-0.5 rounded-sm bg-muted text-muted-foreground">{{ s.equisenal_id }}</span>
      <Button
        variant="ghost" size="icon"
        class="h-6 w-6 text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
        title="Eliminar señal instanciada"
        @click="emit('delete', s)"
      >
        <Trash2 class="h-3 w-3" />
      </Button>
    </div>
  </div>
</template>
