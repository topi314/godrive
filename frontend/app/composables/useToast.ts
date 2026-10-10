const toasts = ref<{ id: number; text: string }[]>([])
let nextId = 1

export function useToast() {
  function toast(text: string) {
    const id = nextId++
    toasts.value = [...toasts.value.slice(-2), { id, text }]
    setTimeout(() => {
      toasts.value = toasts.value.filter(t => t.id !== id)
    }, 2600)
  }
  return { toasts, toast }
}
