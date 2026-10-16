export type ConfirmOptions = {
  title?: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
}

type ConfirmState = {
  title: string
  message: string
  confirmLabel: string
  cancelLabel: string
  danger: boolean
  resolve: (ok: boolean) => void
}

const state = ref<ConfirmState | null>(null)

export function useConfirm() {
  function confirm(opts: string | ConfirmOptions) {
    const message = typeof opts === 'string' ? opts : opts.message
    const title = typeof opts === 'string' ? 'Confirm' : (opts.title || 'Confirm')
    const confirmLabel = typeof opts === 'string' ? 'Confirm' : (opts.confirmLabel || 'Confirm')
    const cancelLabel = typeof opts === 'string' ? 'Cancel' : (opts.cancelLabel || 'Cancel')
    const danger = typeof opts === 'string' ? false : !!opts.danger

    if (state.value) {
      state.value.resolve(false)
      state.value = null
    }

    return new Promise<boolean>((resolve) => {
      state.value = {
        title,
        message,
        confirmLabel,
        cancelLabel,
        danger,
        resolve,
      }
    })
  }

  function answer(ok: boolean) {
    const cur = state.value
    if (!cur) return
    state.value = null
    cur.resolve(ok)
  }

  return { state, confirm, answer }
}
