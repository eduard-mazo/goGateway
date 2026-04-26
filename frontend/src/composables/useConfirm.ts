import { reactive } from 'vue'

export type ConfirmVariant = 'danger' | 'warn' | 'primary'

export interface ConfirmOptions {
  title: string
  message?: string
  /** Optional monospace callout (e.g., the resource name being deleted). */
  detail?: string
  /** Visual tone. Defaults to "danger". */
  variant?: ConfirmVariant
  confirmText?: string
  cancelText?: string
  /** Require typing this string to enable the confirm button. */
  challenge?: string
}

interface ConfirmState extends ConfirmOptions {
  open: boolean
  resolve: ((v: boolean) => void) | null
}

// Single global state — one ConfirmHost mounted in App.vue serves every view.
export const confirmState = reactive<ConfirmState>({
  open: false,
  title: '',
  message: '',
  detail: '',
  variant: 'danger',
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  challenge: '',
  resolve: null,
})

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    confirmState.title = opts.title
    confirmState.message = opts.message ?? ''
    confirmState.detail = opts.detail ?? ''
    confirmState.variant = opts.variant ?? 'danger'
    confirmState.confirmText = opts.confirmText ?? (opts.variant === 'danger' ? 'Delete' : 'Confirm')
    confirmState.cancelText = opts.cancelText ?? 'Cancel'
    confirmState.challenge = opts.challenge ?? ''
    confirmState.resolve = resolve
    confirmState.open = true
  })
}

export function settleConfirm(value: boolean) {
  if (confirmState.resolve) {
    confirmState.resolve(value)
    confirmState.resolve = null
  }
  confirmState.open = false
}

export function useConfirm() {
  return { confirm }
}
