<script setup lang="ts">
import type { TSDBDLQEntry } from '@/api'

defineProps<{ entries: TSDBDLQEntry[] }>()
const emit = defineEmits<{ replay: [] }>()

function fmtAge(ts: string) {
  const s = Math.round((Date.now() - new Date(ts).getTime()) / 1000)
  if (s > 3600) return `${Math.floor(s / 3600)}h ago`
  if (s > 60)   return `${Math.floor(s / 60)}m ago`
  return `${s}s ago`
}
</script>

<template>
  <div class="mt-6 rounded-md border border-destructive/50">
    <div class="flex items-center justify-between px-4 py-3 border-b border-destructive/30">
      <span class="text-sm font-semibold text-destructive">
        Dead Letter Queue — {{ entries.length }} entries
      </span>
      <button
        class="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium
               bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors"
        @click="emit('replay')"
      >
        Replay All
      </button>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full text-xs">
        <thead>
          <tr class="border-b border-border text-muted-foreground">
            <th class="px-4 py-2 text-left font-medium">ID</th>
            <th class="px-4 py-2 text-left font-medium">Backend</th>
            <th class="px-4 py-2 text-left font-medium">Age</th>
            <th class="px-4 py-2 text-left font-medium">Points</th>
            <th class="px-4 py-2 text-left font-medium">Retries</th>
            <th class="px-4 py-2 text-left font-medium">Reason</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="e in entries"
            :key="e.id"
            class="border-b border-border/50 last:border-0 hover:bg-muted/30"
          >
            <td class="px-4 py-2 font-mono">{{ e.id }}</td>
            <td class="px-4 py-2">
              <span
                class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-bold uppercase"
                :class="e.backend === 'victoriametrics'
                  ? 'bg-blue-700/20 text-blue-400'
                  : 'bg-emerald-700/20 text-emerald-400'"
              >{{ e.backend }}</span>
            </td>
            <td class="px-4 py-2 text-muted-foreground">{{ fmtAge(e.ts) }}</td>
            <td class="px-4 py-2 font-mono">{{ e.batch?.length ?? 0 }}</td>
            <td class="px-4 py-2 font-mono">{{ e.retries }}</td>
            <td class="px-4 py-2 max-w-xs truncate text-orange-400 font-mono" :title="e.reason">
              {{ e.reason }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
