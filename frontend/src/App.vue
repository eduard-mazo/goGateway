<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Toaster } from '@/components/ui/sonner'
import {
  Gauge, Radio, Server, Table2, History as HistoryIcon, Cpu,
  PanelLeftClose, PanelLeftOpen, Moon, Sun,
} from 'lucide-vue-next'
import { useStatus } from '@/composables/useStatus'
import StatusPill from '@/components/StatusPill.vue'

const { status } = useStatus()

const nav = [
  { to: '/', label: 'Overview', icon: Gauge },
  { to: '/mappings', label: 'Signal Mapping', icon: Table2 },
  { to: '/devices', label: 'Devices & Topics', icon: Cpu },
  { to: '/mqtt', label: 'MQTT', icon: Radio },
  { to: '/iec104', label: 'IEC 104', icon: Server },
  { to: '/history', label: 'History', icon: HistoryIcon },
]

const collapsed = ref(false)
const dark = ref(false)

onMounted(() => {
  collapsed.value = localStorage.getItem('sb:collapsed') === '1'
  dark.value = localStorage.getItem('theme') === 'dark' ||
    (!localStorage.getItem('theme') && window.matchMedia('(prefers-color-scheme: dark)').matches)
  applyTheme()
})

function toggleSidebar() {
  collapsed.value = !collapsed.value
  localStorage.setItem('sb:collapsed', collapsed.value ? '1' : '0')
}
function toggleTheme() {
  dark.value = !dark.value
  localStorage.setItem('theme', dark.value ? 'dark' : 'light')
  applyTheme()
}
function applyTheme() {
  document.documentElement.classList.toggle('dark', dark.value)
}

const route = useRoute()
const pageTitle = computed(() => (route.meta?.title as string) || 'goGateway')

const brokerState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  if (!status.value) return 'idle'
  return status.value.mqtt.connected ? 'ok' : 'fault'
})
const brokerText = computed(() => {
  if (!status.value) return '—'
  return status.value.mqtt.broker || 'no broker configured'
})
const iecText = computed(() => {
  if (!status.value) return '—'
  const s = status.value.iec104
  const servers = s.servers ?? []
  if (!servers.length) return 'no endpoints'
  const first = servers[0]
  if (servers.length === 1) return `${first.listen}:${first.port} · ASDU ${first.asdu_addr}`
  return `${servers.length} endpoints · ASDU ${servers.map(x => x.asdu_addr).join(',')}`
})

function fmtUptime(s: number) {
  if (!s) return '—'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (h) return `${h}h ${m}m`
  if (m) return `${m}m ${sec}s`
  return `${sec}s`
}
</script>

<template>
  <div class="flex min-h-screen bg-background text-foreground">
    <!-- SIDEBAR -->
    <aside
      class="relative bg-sidebar text-sidebar-foreground border-r border-sidebar-border flex flex-col transition-[width] duration-300 ease-out"
      :class="[collapsed ? 'w-[68px] sidebar-mini' : 'w-[240px]']"
    >
      <!-- Brand -->
      <div class="flex items-center gap-3 px-5 h-[64px] border-b border-sidebar-border">
        <div class="grid place-items-center w-9 h-9 rounded-sm bg-[color:var(--epm-citrico)] text-[color:var(--epm-bosque)] font-black text-base">
          g
        </div>
        <div class="sidebar-wide-only leading-none">
          <div class="font-sans text-[18px] font-extrabold tracking-tight text-white">goGateway</div>
          <div class="text-[10px] uppercase tracking-[0.2em] text-[color:var(--epm-citrico)] mt-1 font-medium">
            MQTT → IEC 104
          </div>
        </div>
      </div>

      <!-- Nav -->
      <nav class="flex-1 py-4 px-3 space-y-0.5">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="group relative flex items-center gap-3 rounded-sm px-3 py-2.5 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
          active-class="bg-sidebar-accent text-sidebar-accent-foreground before:absolute before:left-0 before:top-1.5 before:bottom-1.5 before:w-0.5 before:bg-[color:var(--sidebar-primary)]"
          :title="item.label"
        >
          <component :is="item.icon" class="h-4 w-4 shrink-0" />
          <span class="sidebar-label truncate">{{ item.label }}</span>
        </RouterLink>
      </nav>

      <!-- Bottom controls -->
      <div class="border-t border-sidebar-border p-3 space-y-1">
        <button
          class="w-full flex items-center gap-3 rounded-sm px-3 py-2 text-sm hover:bg-sidebar-accent transition-colors"
          :title="dark ? 'Light mode' : 'Dark mode'"
          @click="toggleTheme"
        >
          <component :is="dark ? Sun : Moon" class="h-4 w-4 shrink-0" />
          <span class="sidebar-label">{{ dark ? 'Light' : 'Dark' }}</span>
        </button>
        <button
          class="w-full flex items-center gap-3 rounded-sm px-3 py-2 text-sm hover:bg-sidebar-accent transition-colors"
          :title="collapsed ? 'Expand' : 'Collapse'"
          @click="toggleSidebar"
        >
          <component :is="collapsed ? PanelLeftOpen : PanelLeftClose" class="h-4 w-4 shrink-0" />
          <span class="sidebar-label">Collapse</span>
        </button>
        <div class="px-3 pt-3 sidebar-wide-only">
          <div class="text-[10px] uppercase tracking-[0.22em] text-[color:var(--epm-citrico)] font-semibold">Uptime</div>
          <div class="font-mono text-xs mt-1 text-white/90">{{ fmtUptime(status?.uptime_seconds ?? 0) }}</div>
        </div>
      </div>
    </aside>

    <!-- MAIN -->
    <div class="flex-1 flex flex-col min-w-0 bg-grain">
      <!-- Top rail -->
      <header class="sticky top-0 z-30 flex items-center justify-between h-16 px-8 border-b border-border bg-background/70 backdrop-blur-md">
        <div class="flex items-baseline gap-4">
          <h2 class="font-heading text-2xl leading-none">{{ pageTitle }}</h2>
          <span class="font-mono text-[11px] uppercase tracking-[0.2em] text-muted-foreground">
            / {{ route.path === '/' ? 'overview' : route.path.slice(1) }}
          </span>
        </div>
        <div class="flex items-center gap-3">
          <StatusPill
            label="MQTT"
            :state="brokerState"
            :value="brokerText"
          />
          <StatusPill
            label="IEC 104"
            :state="status ? 'ok' : 'idle'"
            :value="iecText"
          />
        </div>
      </header>

      <main class="flex-1 overflow-auto">
        <RouterView v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </RouterView>
      </main>
    </div>

    <!-- Toaster: small, bottom-right -->
    <Toaster
      position="bottom-right"
      :offset="16"
      :toast-options="{
        classes: {
          toast: '!rounded-sm !border !border-border !bg-card !text-card-foreground !text-xs !py-2 !px-3 !shadow-sm',
          title: '!text-xs !font-medium',
          description: '!text-[11px] !text-muted-foreground',
        },
      }"
    />
  </div>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 120ms ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
