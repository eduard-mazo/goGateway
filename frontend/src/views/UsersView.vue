<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { toast } from 'vue-sonner'
import { api, type User } from '@/api'
import { useConfirm } from '@/composables/useConfirm'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Users, UserPlus, Pencil, Trash2,
  ShieldCheck, Wrench, Eye, CheckCircle2, XCircle,
} from 'lucide-vue-next'

const { confirm } = useConfirm()

const users = ref<User[]>([])
const loading = ref(false)

const showDialog = ref(false)
const editingUser = ref<User | null>(null)
const saving = ref(false)

const form = ref({
  username: '',
  password: '',
  email: '',
  full_name: '',
  role: 'viewer' as string,
  enabled: true,
})

async function loadUsers() {
  loading.value = true
  try {
    users.value = (await api.get<User[]>('/auth/users')).data
  } catch (e: any) {
    toast.error('Error cargando usuarios: ' + (e?.response?.data?.error ?? e?.message ?? e))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingUser.value = null
  form.value = { username: '', password: '', email: '', full_name: '', role: 'viewer', enabled: true }
  showDialog.value = true
}

function openEdit(u: User) {
  editingUser.value = u
  form.value = {
    username: u.username,
    password: '',
    email: u.email ?? '',
    full_name: u.full_name ?? '',
    role: u.role,
    enabled: u.enabled,
  }
  showDialog.value = true
}

async function saveUser() {
  if (saving.value) return
  saving.value = true
  try {
    if (editingUser.value) {
      const payload: Record<string, unknown> = {
        email:     form.value.email,
        full_name: form.value.full_name,
        role:      form.value.role,
        enabled:   form.value.enabled,
      }
      if (form.value.password) payload.password = form.value.password
      await api.put(`/auth/users/${editingUser.value.id}`, payload)
      toast.success('Usuario actualizado')
    } else {
      await api.post('/auth/users', {
        username:  form.value.username,
        password:  form.value.password,
        email:     form.value.email,
        full_name: form.value.full_name,
        role:      form.value.role,
      })
      toast.success('Usuario creado')
    }
    showDialog.value = false
    await loadUsers()
  } catch (e: any) {
    toast.error('Error: ' + (e?.response?.data?.error ?? e?.message ?? e))
  } finally {
    saving.value = false
  }
}

async function removeUser(u: User) {
  const ok = await confirm({
    title: 'Eliminar usuario',
    message: `¿Eliminar la cuenta de "${u.username}"? Esta acción no se puede deshacer.`,
    detail: u.username,
    variant: 'danger',
    confirmText: 'Eliminar',
  })
  if (!ok) return
  try {
    await api.delete(`/auth/users/${u.id}`)
    toast.success(`Usuario "${u.username}" eliminado`)
    await loadUsers()
  } catch (e: any) {
    toast.error('Error eliminando usuario: ' + (e?.response?.data?.error ?? e?.message ?? e))
  }
}

onMounted(loadUsers)

const roleChip: Record<string, string> = {
  superadmin: 'text-amber-600 border-amber-300 bg-amber-50 dark:text-amber-400 dark:border-amber-400/30 dark:bg-amber-400/10',
  operator:   'text-blue-600 border-blue-300 bg-blue-50 dark:text-blue-400 dark:border-blue-400/30 dark:bg-blue-400/10',
  viewer:     'text-slate-600 border-slate-300 bg-slate-50 dark:text-slate-400 dark:border-slate-400/30 dark:bg-slate-400/10',
}
const roleLabel: Record<string, string> = {
  superadmin: 'Super Admin',
  operator:   'Operador',
  viewer:     'Visor',
}
</script>

<template>
  <div class="p-4 sm:p-6 lg:p-8 space-y-6 max-w-5xl">

    <!-- Header -->
    <section class="card-soft overflow-hidden">
      <div class="grid grid-cols-12 gap-6 p-8">
        <div class="col-span-12 md:col-span-8">
          <div class="flex items-center gap-3 mb-3">
            <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-bosque)] text-white">
              <Users class="h-5 w-5" />
            </div>
            <span class="text-[11px] uppercase tracking-[0.26em] font-bold text-[color:var(--epm-bosque)]">
              Administración
            </span>
          </div>
          <h1 class="mb-2">Gestión de Usuarios</h1>
          <p class="mt-3 text-sm text-muted-foreground max-w-xl">
            Crea, edita y desactiva cuentas de operadores. Solo los superadmins tienen acceso a esta sección.
          </p>
        </div>
        <div class="col-span-12 md:col-span-4 flex items-center justify-end">
          <Button @click="openCreate" class="gap-2">
            <UserPlus class="h-4 w-4" />
            Nuevo usuario
          </Button>
        </div>
      </div>
    </section>

    <!-- Users table -->
    <section class="card-soft overflow-hidden">
      <div v-if="loading" class="p-8 text-center text-sm text-muted-foreground">
        Cargando…
      </div>
      <div v-else-if="!users.length" class="p-8 text-center text-sm text-muted-foreground">
        No hay usuarios registrados.
      </div>
      <table v-else class="w-full text-sm">
        <thead>
          <tr class="border-b border-border bg-muted/30 text-[11px] uppercase tracking-[0.15em] text-muted-foreground">
            <th class="px-4 py-3 text-left font-medium">Usuario</th>
            <th class="px-4 py-3 text-left font-medium">Nombre</th>
            <th class="px-4 py-3 text-left font-medium hidden md:table-cell">Email</th>
            <th class="px-4 py-3 text-left font-medium">Rol</th>
            <th class="px-4 py-3 text-center font-medium">Estado</th>
            <th class="px-4 py-3 text-right font-medium">Acciones</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="u in users"
            :key="u.id"
            class="border-b border-border last:border-0 hover:bg-muted/20 transition-colors"
          >
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <div
                  class="grid place-items-center w-7 h-7 rounded-sm bg-[color:var(--epm-citrico)]
                         text-[color:var(--epm-bosque)] font-bold text-xs shrink-0 select-none"
                >
                  {{ u.username.charAt(0).toUpperCase() }}
                </div>
                <span class="font-medium">{{ u.username }}</span>
              </div>
            </td>
            <td class="px-4 py-3 text-muted-foreground">{{ u.full_name || '—' }}</td>
            <td class="px-4 py-3 text-muted-foreground hidden md:table-cell font-mono text-xs">
              {{ u.email || '—' }}
            </td>
            <td class="px-4 py-3">
              <span
                class="inline-flex items-center gap-1 px-2 py-0.5 rounded border text-[10px] font-semibold
                       uppercase tracking-[0.1em]"
                :class="roleChip[u.role] ?? roleChip.viewer"
              >
                <ShieldCheck v-if="u.role === 'superadmin'" class="h-3 w-3" />
                <Wrench v-else-if="u.role === 'operator'" class="h-3 w-3" />
                <Eye v-else class="h-3 w-3" />
                {{ roleLabel[u.role] ?? u.role }}
              </span>
            </td>
            <td class="px-4 py-3 text-center">
              <span v-if="u.enabled" class="inline-flex items-center gap-1 text-[color:var(--signal-ok)] text-xs">
                <CheckCircle2 class="h-3.5 w-3.5" /> Activo
              </span>
              <span v-else class="inline-flex items-center gap-1 text-muted-foreground text-xs">
                <XCircle class="h-3.5 w-3.5" /> Inactivo
              </span>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center justify-end gap-1">
                <Button variant="ghost" size="icon" class="h-7 w-7" @click="openEdit(u)" title="Editar">
                  <Pencil class="h-3.5 w-3.5" />
                </Button>
                <Button variant="ghost" size="icon" class="h-7 w-7 text-destructive hover:text-destructive" @click="removeUser(u)" title="Eliminar">
                  <Trash2 class="h-3.5 w-3.5" />
                </Button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

  </div>

  <!-- Create / Edit dialog -->
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="showDialog"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
        @click.self="showDialog = false"
      >
        <div class="absolute inset-0 bg-black/50 backdrop-blur-sm" @click="showDialog = false" />
        <div class="relative z-10 w-full max-w-md rounded-sm border border-border bg-card shadow-xl">
          <!-- Dialog header -->
          <div class="flex items-center justify-between px-6 py-4 border-b border-border">
            <h3 class="font-heading text-base font-semibold">
              {{ editingUser ? 'Editar usuario' : 'Nuevo usuario' }}
            </h3>
            <button
              class="grid place-items-center w-7 h-7 rounded-sm text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              @click="showDialog = false"
            >
              ✕
            </button>
          </div>

          <!-- Form -->
          <form class="px-6 py-5 space-y-4" @submit.prevent="saveUser">

            <!-- Username (create only) -->
            <div v-if="!editingUser" class="space-y-1.5">
              <Label for="dlg-username">Usuario</Label>
              <Input
                id="dlg-username"
                v-model="form.username"
                placeholder="jdoe"
                autocomplete="off"
                required
              />
            </div>

            <!-- Full name + email -->
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <Label for="dlg-fullname">Nombre completo</Label>
                <Input id="dlg-fullname" v-model="form.full_name" placeholder="Juan Doe" />
              </div>
              <div class="space-y-1.5">
                <Label for="dlg-email">Email</Label>
                <Input id="dlg-email" v-model="form.email" type="email" placeholder="juan@empresa.com" />
              </div>
            </div>

            <!-- Role -->
            <div class="space-y-1.5">
              <Label for="dlg-role">Rol</Label>
              <select
                id="dlg-role"
                v-model="form.role"
                class="flex h-9 w-full rounded-sm border border-input bg-background px-3 py-1 text-sm
                       shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              >
                <option value="viewer">Visor</option>
                <option value="operator">Operador</option>
                <option value="superadmin">Super Admin</option>
              </select>
            </div>

            <!-- Password -->
            <div class="space-y-1.5">
              <Label for="dlg-password">
                {{ editingUser ? 'Nueva contraseña (dejar vacío para no cambiar)' : 'Contraseña' }}
              </Label>
              <Input
                id="dlg-password"
                v-model="form.password"
                type="password"
                placeholder="••••••••"
                :required="!editingUser"
                autocomplete="new-password"
              />
            </div>

            <!-- Enabled toggle (edit only) -->
            <div v-if="editingUser" class="flex items-center justify-between py-1">
              <Label for="dlg-enabled" class="cursor-pointer">Cuenta activa</Label>
              <Switch id="dlg-enabled" v-model:checked="form.enabled" />
            </div>

            <!-- Actions -->
            <div class="flex items-center justify-end gap-2 pt-2">
              <Button type="button" variant="ghost" @click="showDialog = false">Cancelar</Button>
              <Button type="submit" :disabled="saving">
                {{ saving ? 'Guardando…' : editingUser ? 'Guardar cambios' : 'Crear usuario' }}
              </Button>
            </div>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active, .modal-leave-active { transition: opacity 150ms ease; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-active > div:last-child, .modal-leave-active > div:last-child {
  transition: transform 150ms ease;
}
.modal-enter-from > div:last-child, .modal-leave-to > div:last-child {
  transform: translateY(8px);
}
</style>
