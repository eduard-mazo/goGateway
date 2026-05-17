<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { LogOut, User } from 'lucide-vue-next'
import { useAuth, type Role } from '@/composables/useAuth'

const router = useRouter()
const { user, isLoggedIn, logout } = useAuth()

const roleLabel: Record<Role | string, string> = {
  superadmin: 'Super Admin',
  operator:   'Operador',
  viewer:     'Visor',
}

const roleColor = computed(() => {
  switch (user.value?.role) {
    case 'superadmin': return 'text-amber-400 border-amber-400/30 bg-amber-400/10'
    case 'operator':   return 'text-blue-400 border-blue-400/30 bg-blue-400/10'
    default:           return 'text-slate-400 border-slate-400/30 bg-slate-400/10'
  }
})

const initial = computed(() =>
  user.value?.username?.charAt(0).toUpperCase() ?? '?'
)

async function handleLogout() {
  await logout()
  router.push('/login')
}
</script>

<template>
  <!-- Logged-in state: avatar + name + role + logout -->
  <div v-if="isLoggedIn" class="px-3 pt-3 pb-1 space-y-2">
    <div class="flex items-center gap-2.5">
      <!-- Avatar -->
      <div class="grid place-items-center w-8 h-8 rounded-sm bg-[color:var(--epm-citrico)]
                  text-[color:var(--epm-bosque)] font-bold text-sm shrink-0 select-none">
        {{ initial }}
      </div>
      <!-- Name + role chip -->
      <div class="flex-1 min-w-0 sidebar-wide-only">
        <div class="text-xs font-semibold text-white/90 truncate leading-none">
          {{ user?.username }}
        </div>
        <span
          class="inline-block mt-1 px-1.5 py-0.5 rounded border text-[10px] font-medium leading-none
                 uppercase tracking-[0.1em]"
          :class="roleColor"
        >
          {{ roleLabel[user?.role ?? ''] ?? user?.role }}
        </span>
      </div>
      <!-- Logout button -->
      <button
        class="sidebar-wide-only grid place-items-center w-7 h-7 rounded-sm
               text-white/40 hover:text-white/80 hover:bg-sidebar-accent transition-colors shrink-0"
        title="Cerrar sesión"
        aria-label="Cerrar sesión"
        @click="handleLogout"
      >
        <LogOut class="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </div>
  </div>

  <!-- Logged-out state: compact login link -->
  <div v-else class="px-3 pt-2 pb-1">
    <RouterLink
      to="/login"
      class="flex items-center gap-2.5 rounded-sm px-2 py-2 text-sm
             text-white/50 hover:text-white/80 hover:bg-sidebar-accent transition-colors"
      title="Iniciar sesión"
    >
      <User class="h-4 w-4 shrink-0" aria-hidden="true" />
      <span class="sidebar-wide-only text-xs">Iniciar sesión</span>
    </RouterLink>
  </div>
</template>
