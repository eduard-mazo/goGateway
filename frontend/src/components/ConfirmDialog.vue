<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AlertTriangle, Trash2, ShieldQuestion } from 'lucide-vue-next'
import { confirmState, settleConfirm, type ConfirmVariant } from '@/composables/useConfirm'
import { t } from '@/i18n'

const challengeInput = ref('')
const inputRef = ref<HTMLInputElement | null>(null)

const variantTone = computed<Record<ConfirmVariant, {
  ribbon: string
  iconBg: string
  iconColor: string
  icon: typeof AlertTriangle
  cta: string
  label: string
}>>(() => ({
  danger: {
    ribbon: 'bg-[color:var(--signal-fault)]',
    iconBg: 'bg-[color:color-mix(in_srgb,var(--signal-fault)_18%,transparent)]',
    iconColor: 'text-[color:var(--signal-fault)]',
    icon: Trash2,
    cta: 'bg-[color:var(--signal-fault)] hover:bg-[color:color-mix(in_srgb,var(--signal-fault)_85%,black)] text-white',
    label: t.confirm.danger,
  },
  warn: {
    ribbon: 'bg-[color:var(--signal-warn)]',
    iconBg: 'bg-[color:color-mix(in_srgb,var(--signal-warn)_22%,transparent)]',
    iconColor: 'text-[color:var(--signal-warn)]',
    icon: AlertTriangle,
    cta: 'bg-[color:var(--signal-warn)] hover:bg-[color:color-mix(in_srgb,var(--signal-warn)_85%,black)] text-[color:var(--epm-ink)]',
    label: t.confirm.warn,
  },
  primary: {
    ribbon: 'bg-[color:var(--epm-bosque)]',
    iconBg: 'bg-[color:color-mix(in_srgb,var(--epm-citrico)_28%,transparent)]',
    iconColor: 'text-[color:var(--epm-bosque)]',
    icon: ShieldQuestion,
    cta: 'bg-[color:var(--epm-bosque)] hover:bg-[color:var(--epm-bosque-deep)] text-white',
    label: t.confirm.primary,
  },
}))

const tone = computed(() => variantTone.value[confirmState.variant ?? 'danger'])
const requiresChallenge = computed(() => !!confirmState.challenge)
const challengeMet = computed(() =>
  !requiresChallenge.value || challengeInput.value.trim() === confirmState.challenge
)

function onCancel() {
  settleConfirm(false)
}
function onConfirm() {
  if (!challengeMet.value) return
  settleConfirm(true)
}

watch(() => confirmState.open, (open) => {
  if (open) {
    challengeInput.value = ''
    nextTick(() => inputRef.value?.focus())
  }
})

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && challengeMet.value) onConfirm()
  if (e.key === 'Escape') onCancel()
}
</script>

<template>
  <Dialog :open="confirmState.open" @update:open="(v: boolean) => !v && onCancel()">
    <DialogContent
      class="!max-w-md p-0 overflow-hidden border-0 ring-0"
      @keydown="onKey"
    >
      <!-- Top warning ribbon — industrial caution stripe -->
      <div class="relative h-1.5" :class="tone.ribbon">
        <div class="absolute inset-0 opacity-40 caution-stripe" />
      </div>

      <div class="px-6 pt-6 pb-2">
        <div class="flex items-start gap-4">
          <div
            class="grid place-items-center w-12 h-12 rounded-sm shrink-0"
            :class="[tone.iconBg, tone.iconColor]"
          >
            <component :is="tone.icon" class="h-5 w-5" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="text-[10px] uppercase tracking-[0.22em] font-bold mb-1" :class="tone.iconColor">
              {{ tone.label }}
            </div>
            <h3 class="text-lg font-extrabold leading-tight tracking-tight text-foreground">
              {{ confirmState.title }}
            </h3>
            <p
              v-if="confirmState.message"
              class="mt-2 text-sm text-muted-foreground leading-relaxed"
            >{{ confirmState.message }}</p>
          </div>
        </div>

        <!-- Resource callout -->
        <div
          v-if="confirmState.detail"
          class="mt-4 px-3 py-2 rounded-sm border border-border bg-muted/40 font-mono text-xs break-all"
        >
          {{ confirmState.detail }}
        </div>

        <!-- Type-to-confirm challenge -->
        <div v-if="requiresChallenge" class="mt-4 space-y-1.5">
          <Label class="text-[10px] uppercase tracking-[0.18em] font-bold">
            {{ t.confirm.typeToConfirm }}
            <code class="font-mono px-1 py-0.5 rounded-sm bg-muted text-foreground">{{ confirmState.challenge }}</code>
            {{ t.confirm.toConfirm }}
          </Label>
          <Input
            ref="inputRef"
            v-model="challengeInput"
            class="font-mono"
            autocomplete="off"
            spellcheck="false"
          />
        </div>
      </div>

      <div class="px-6 py-4 mt-2 flex items-center justify-end gap-2 bg-[color:color-mix(in_srgb,var(--muted)_60%,transparent)] border-t border-border">
        <Button
          variant="outline"
          class="rounded-sm"
          @click="onCancel"
        >
          {{ confirmState.cancelText }}
        </Button>
        <Button
          class="rounded-sm font-bold tracking-wide"
          :class="tone.cta"
          :disabled="!challengeMet"
          @click="onConfirm"
        >
          <component :is="tone.icon" class="h-4 w-4 mr-1.5" />
          {{ confirmState.confirmText }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
.caution-stripe {
  background-image: repeating-linear-gradient(
    45deg,
    transparent 0,
    transparent 6px,
    rgba(0, 0, 0, 0.45) 6px,
    rgba(0, 0, 0, 0.45) 12px
  );
}
</style>
