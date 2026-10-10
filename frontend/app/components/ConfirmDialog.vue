<template>
  <div
    v-if="state"
    class="dialog-backdrop dialog-backdrop-sticky"
    role="presentation"
    @click.self="answer(false)"
  >
    <div
      class="dialog confirm-dialog"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      aria-describedby="confirm-message"
    >
      <div class="dialog-head">
        <h2 id="confirm-title">{{ state.title }}</h2>
        <IconBtn name="x" label="Close" variant="ghost" @click="answer(false)" />
      </div>
      <p id="confirm-message">{{ state.message }}</p>
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" @click="answer(false)">
          {{ state.cancelLabel }}
        </button>
        <button
          ref="confirmBtn"
          type="button"
          class="btn-primary"
          :class="{ danger: state.danger }"
          @click="answer(true)"
        >
          {{ state.confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const { state, answer } = useConfirm()
const confirmBtn = ref<HTMLButtonElement | null>(null)

watch(state, async (v) => {
  if (!v) return
  await nextTick()
  confirmBtn.value?.focus()
})

function onKeydown(e: KeyboardEvent) {
  if (!state.value) return
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    answer(false)
  }
}

onMounted(() => document.addEventListener('keydown', onKeydown, true))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown, true))
</script>
