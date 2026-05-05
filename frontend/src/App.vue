<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Toaster } from '@/components/ui/sonner'
import {
  Gauge, Radio, Server, Table2, History as HistoryIcon, Cpu,
  Database,
  PanelLeftClose, PanelLeftOpen, Moon, Sun, Menu, X,
} from 'lucide-vue-next'
import { useStatus } from '@/composables/useStatus'
import StatusPill from '@/components/StatusPill.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'

const { status } = useStatus()

const nav = [
  { to: '/', label: 'Overview', icon: Gauge },
  { to: '/mappings', label: 'Signal Mapping', icon: Table2 },
  { to: '/devices', label: 'Devices & Topics', icon: Cpu },
  { to: '/mqtt', label: 'MQTT', icon: Radio },
  { to: '/nats', label: 'NATS Fan-Out', icon: Database },
  { to: '/iec104', label: 'IEC 104', icon: Server },
  { to: '/history', label: 'History', icon: HistoryIcon },
  { to: '/tsdb', label: 'TSDB Pipeline', icon: Database },
]

const collapsed = ref(false)        // desktop mini mode
const mobileOpen = ref(false)       // mobile drawer
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
function toggleMobile() {
  mobileOpen.value = !mobileOpen.value
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

// Close mobile drawer on route change.
watch(() => route.fullPath, () => { mobileOpen.value = false })

const brokerState = computed<'ok' | 'warn' | 'fault' | 'idle'>(() => {
  if (!status.value) return 'idle'
  return status.value.mqtt.connected ? 'ok' : 'fault'
})
const brokerText = computed(() => {
  if (!status.value) return '—'
  return status.value.mqtt.broker || 'no broker configured'
})

type FleetState = 'ok' | 'warn' | 'fault' | 'idle' | 'wait'
const iecFleet = computed<{ state: FleetState; label: string; value: string }>(() => {
  const ip = status.value?.iec104.listen_ip || '0.0.0.0'
  const list = status.value?.iec104.servers ?? []
  if (!list.length) return { state: 'idle', label: 'IEC 104', value: `${ip} · no endpoints` }
  const enabled = list.filter(s => s.enabled)
  if (!enabled.length) return { state: 'idle', label: 'IEC 104', value: `${ip} · disabled` }
  const running = enabled.filter(s => s.running)
  if (!running.length) return { state: 'fault', label: 'IEC 104', value: `${ip} · bind failed` }
  if (running.length < enabled.length) {
    return { state: 'warn', label: 'IEC 104', value: `${ip} · ${running.length}/${enabled.length} bound` }
  }
  const totalActivated = list.reduce((n, s) => n + s.activated, 0)
  const totalTCP = list.reduce((n, s) => n + s.clients, 0)
  if (totalActivated > 0) {
    return { state: 'ok', label: 'IEC 104', value: `${ip} · ${totalActivated} protocol link${totalActivated === 1 ? '' : 's'}` }
  }
  if (totalTCP > 0) {
    return { state: 'warn', label: 'IEC 104', value: `${ip} · ${totalTCP} TCP, no STARTDT` }
  }
  return { state: 'wait', label: 'IEC 104', value: `${ip} · listening` }
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
  <!-- Root: full viewport, clip overflow. Only <main> scrolls. -->
  <div class="h-screen w-screen overflow-hidden flex bg-background text-foreground">
    <!-- Mobile backdrop -->
    <div
      v-show="mobileOpen"
      class="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm md:hidden"
      @click="mobileOpen = false"
    />

    <!-- SIDEBAR -->
    <aside
      class="bg-sidebar text-sidebar-foreground border-r border-sidebar-border flex flex-col h-screen overflow-hidden transition-[width,transform] duration-300 ease-out
             fixed inset-y-0 left-0 z-50 md:static md:z-auto"
      :class="[
        collapsed ? 'w-[68px] sidebar-mini' : 'w-[260px] md:w-[240px]',
        mobileOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0',
      ]"
    >
      <!-- Brand -->
      <div class="flex items-center gap-3 px-5 h-[64px] border-b border-sidebar-border shrink-0">
        <div class="grid place-items-center w-9 h-9 rounded-sm bg-[color:var(--epm-citrico)] text-[color:var(--epm-bosque)] font-black text-base shrink-0">
          g
        </div>
        <div class="sidebar-wide-only leading-none flex-1 min-w-0">
          <div class="font-sans text-[18px] font-extrabold tracking-tight text-white truncate">goGateway</div>
          <div class="text-[10px] uppercase tracking-[0.2em] text-[color:var(--epm-citrico)] mt-1 font-medium">
            MQTT → IEC 104
          </div>
        </div>
        <button
          class="md:hidden grid place-items-center w-8 h-8 rounded-sm hover:bg-sidebar-accent text-white/70"
          aria-label="Close menu"
          @click="mobileOpen = false"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <!-- Nav (scrollable only if it really overflows; sidebar itself doesn't scroll) -->
      <nav class="flex-1 min-h-0 overflow-y-auto py-4 px-3 space-y-0.5 sidebar-nav-scroll">
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
      <div class="border-t border-sidebar-border p-3 space-y-1 shrink-0">
        <button
          class="w-full flex items-center gap-3 rounded-sm px-3 py-2 text-sm hover:bg-sidebar-accent transition-colors"
          :title="dark ? 'Light mode' : 'Dark mode'"
          @click="toggleTheme"
        >
          <component :is="dark ? Sun : Moon" class="h-4 w-4 shrink-0" />
          <span class="sidebar-label">{{ dark ? 'Light' : 'Dark' }}</span>
        </button>
        <button
          class="hidden md:flex w-full items-center gap-3 rounded-sm px-3 py-2 text-sm hover:bg-sidebar-accent transition-colors"
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

    <!-- MAIN COLUMN -->
    <div class="flex-1 flex flex-col min-w-0 h-screen overflow-hidden bg-grain">
      <!-- Top rail: never scrolls, never reflows on status change -->
      <header class="shrink-0 flex items-center justify-between gap-3 sm:gap-6 h-16 px-4 sm:px-8 border-b border-border bg-background/80 backdrop-blur-md z-30">
        <div class="flex items-center gap-3 min-w-0 flex-1">
          <button
            class="md:hidden grid place-items-center w-9 h-9 rounded-sm border border-border hover:bg-muted shrink-0"
            aria-label="Open menu"
            @click="toggleMobile"
          >
            <Menu class="h-4 w-4" />
          </button>
          <div class="flex items-baseline gap-3 min-w-0">
            <h2 class="font-heading text-xl sm:text-2xl leading-none truncate">{{ pageTitle }}</h2>
            <span class="font-mono text-[11px] uppercase tracking-[0.2em] text-muted-foreground hidden lg:inline">
              / {{ route.path === '/' ? 'overview' : route.path.slice(1) }}
            </span>
          </div>
        </div>
        <!-- Status pills: desktop only. Mobile sees status on dashboard. -->
        <div class="hidden md:flex items-center gap-3 shrink-0">
          <StatusPill label="MQTT" :state="brokerState" :value="brokerText" stable />
          <StatusPill :label="iecFleet.label" :state="iecFleet.state" :value="iecFleet.value" stable />
        </div>
      </header>

      <main class="flex-1 min-h-0 overflow-auto">
        <RouterView v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </RouterView>
      </main>
    </div>

    <!-- Toaster: portaled to body, z-100 via global CSS -->
    <Toaster
      position="bottom-right"
      :offset="20"
      :toast-options="{
        classes: {
          toast: '!rounded-sm !border !border-border !bg-card !text-card-foreground !text-xs !py-2 !px-3 !shadow-md',
          title: '!text-xs !font-medium',
          description: '!text-[11px] !text-muted-foreground',
        },
      }"
    />

    <!-- Global confirm dialog — one instance, served by useConfirm composable -->
    <ConfirmDialog />
  </div>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 120ms ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

/* Subtle scrollbar inside sidebar nav when overflow happens */
.sidebar-nav-scroll {
  scrollbar-width: thin;
  scrollbar-color: rgba(255,255,255,0.18) transparent;
}
.sidebar-nav-scroll::-webkit-scrollbar { width: 6px; }
.sidebar-nav-scroll::-webkit-scrollbar-thumb {
  background: rgba(255,255,255,0.18);
  border-radius: 3px;
}
</style>
