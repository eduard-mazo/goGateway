<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuth } from '@/composables/useAuth'
import { Input }  from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Label }  from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'

const router = useRouter()
const route  = useRoute()
const { login } = useAuth()

const username = ref('')
const password = ref('')
const loading  = ref(false)
const error    = ref('')

async function submit() {
  if (!username.value || !password.value) return
  loading.value = true
  error.value   = ''
  try {
    await login(username.value, password.value)
    // Redirect to the page the user was trying to reach, or the dashboard.
    const next = (route.query.redirect as string) || '/'
    router.push(next)
  } catch (e: any) {
    const msg = e?.response?.data?.error
    error.value = msg === 'invalid credentials'
      ? 'Usuario o contraseña incorrectos'
      : msg === 'account disabled'
      ? 'Cuenta deshabilitada'
      : 'Error al iniciar sesión'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-background p-4">
    <!-- Subtle grid pattern overlay -->
    <div class="pointer-events-none fixed inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))]
                from-[color:color-mix(in_srgb,var(--epm-bosque)_60%,transparent)] via-transparent to-transparent opacity-60" />

    <div class="relative w-full max-w-sm">
      <!-- Logo row -->
      <div class="flex items-center gap-3 mb-8">
        <div class="grid place-items-center w-10 h-10 rounded-sm bg-[color:var(--epm-citrico)]
                    text-[color:var(--epm-bosque)] font-black text-lg shrink-0">
          g
        </div>
        <div class="leading-none">
          <div class="font-sans text-xl font-extrabold tracking-tight">goGateway</div>
          <div class="text-[10px] uppercase tracking-[0.2em] text-muted-foreground mt-0.5">
            MQTT → IEC 104
          </div>
        </div>
      </div>

      <Card class="shadow-xl border-border">
        <CardHeader class="pb-4">
          <CardTitle class="text-lg">Iniciar sesión</CardTitle>
          <CardDescription>Ingresa tus credenciales de operador</CardDescription>
        </CardHeader>

        <CardContent>
          <form class="space-y-4" @submit.prevent="submit">
            <div class="space-y-1.5">
              <Label for="username">Usuario</Label>
              <Input
                id="username"
                v-model="username"
                placeholder="admin"
                autocomplete="username"
                :disabled="loading"
              />
            </div>

            <div class="space-y-1.5">
              <Label for="password">Contraseña</Label>
              <Input
                id="password"
                v-model="password"
                type="password"
                placeholder="••••••••"
                autocomplete="current-password"
                :disabled="loading"
              />
            </div>

            <!-- Error message -->
            <Transition name="err">
              <p v-if="error" class="text-xs text-destructive bg-destructive/10 border border-destructive/20
                                     rounded-sm px-3 py-2">
                {{ error }}
              </p>
            </Transition>

            <Button type="submit" class="w-full" :disabled="loading || !username || !password">
              <span v-if="!loading">Entrar</span>
              <span v-else class="flex items-center gap-2">
                <svg class="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                  <path class="opacity-75" fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
                </svg>
                Verificando…
              </span>
            </Button>
          </form>
        </CardContent>
      </Card>

      <p class="text-center text-[11px] text-muted-foreground mt-5">
        El acceso está restringido a operadores autorizados.
      </p>
    </div>
  </div>
</template>

<style scoped>
.err-enter-active, .err-leave-active { transition: opacity 150ms, transform 150ms; }
.err-enter-from, .err-leave-to { opacity: 0; transform: translateY(-4px); }
</style>
