<script setup lang="ts">
import { useTSDB } from '@/composables/useTSDB'
import BackendCard from '@/components/tsdb/BackendCard.vue'
import DLQTable    from '@/components/tsdb/DLQTable.vue'
import { Database } from 'lucide-vue-next'

const {
  status, dlq, error, enabled,
  replayDLQ, fetchDLQ,
  totalWriteRate, anyCircuitOpen, systemAlert,
} = useTSDB(2000)

async function handleReplay() {
  const n = await replayDLQ()
  await fetchDLQ()
  console.log(`replayed ${n} DLQ batches`)
}
</script>

<template>
  <div class="p-6 space-y-6">

    <!-- Not configured -->
    <div
      v-if="!enabled"
      class="flex flex-col items-center justify-center gap-4 rounded-lg border border-dashed border-border py-20 text-center"
    >
      <Database class="h-10 w-10 text-muted-foreground/40" />
      <div>
        <p class="text-sm font-medium">TSDB not configured</p>
        <p class="text-xs text-muted-foreground mt-1">
          Set <code class="rounded bg-muted px-1">GW_VM_URL</code> or
          <code class="rounded bg-muted px-1">GW_TIMESCALE_DSN</code> to enable
          the time-series pipeline.
        </p>
      </div>
    </div>

    <template v-else-if="status">

      <!-- Alert banner -->
      <div
        v-if="systemAlert"
        class="flex flex-wrap items-center gap-4 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm"
      >
        <span v-if="anyCircuitOpen" class="text-destructive font-medium">
          Circuit breaker open
        </span>
        <span v-if="status.dlqDepth > 0" class="text-destructive font-medium flex items-center gap-2">
          DLQ: {{ status.dlqDepth }} entries
          <button
            class="rounded px-2 py-0.5 text-xs font-bold bg-destructive text-destructive-foreground hover:bg-destructive/80"
            @click="handleReplay"
          >Replay</button>
        </span>
        <span v-if="status.walPending > 500" class="text-orange-400 font-medium">
          WAL backlog: {{ status.walPending }}
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
            {{ (status.inputRate ?? 0).toFixed(0) }}<span class="text-xs text-muted-foreground">/s</span>
          </p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">Input queue</p>
          <p
            class="mt-1 font-mono text-lg font-semibold tabular-nums"
            :class="(status.inputQueue ?? 0) > 50000 ? 'text-orange-400' : ''"
          >{{ status.inputQueue ?? 0 }}</p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">WAL pending</p>
          <p
            class="mt-1 font-mono text-lg font-semibold tabular-nums"
            :class="(status.walPending ?? 0) > 100 ? 'text-orange-400' : ''"
          >{{ status.walPending ?? 0 }}</p>
        </div>
        <div class="rounded-md border border-border bg-card px-4 py-3">
          <p class="text-[11px] uppercase tracking-wide text-muted-foreground">DLQ</p>
          <p
            class="mt-1 font-mono text-lg font-semibold tabular-nums"
            :class="(status.dlqDepth ?? 0) > 0 ? 'text-destructive' : ''"
          >{{ status.dlqDepth ?? 0 }}</p>
        </div>
      </div>

      <!-- Backend cards -->
      <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <BackendCard
          v-for="b in status.backends"
          :key="b.name"
          :backend="b"
          :retry-depth="status.retryQueues?.[b.name] ?? 0"
        />
      </div>

      <!-- DLQ table -->
      <DLQTable
        v-if="dlq.count > 0"
        :entries="dlq.entries"
        @replay="handleReplay"
      />

    </template>

    <!-- Loading -->
    <div v-else-if="!error" class="flex justify-center py-20">
      <span class="text-sm text-muted-foreground animate-pulse">Loading…</span>
    </div>

    <!-- Error -->
    <div v-if="error" class="rounded-md border border-orange-500/40 bg-orange-500/10 px-4 py-3 text-sm text-orange-400">
      {{ error }}
    </div>

  </div>
</template>
