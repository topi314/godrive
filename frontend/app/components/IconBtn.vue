<template>
  <button
    type="button"
    class="icon-btn"
    :class="variant"
    :disabled="disabled"
    :title="label"
    :aria-label="label"
    @click="$emit('click', $event)"
  >
    <AppIcon :name="name" />
  </button>
</template>

<script setup lang="ts">
defineProps<{
  name: string
  label: string
  disabled?: boolean
  variant?: 'primary' | 'danger' | 'ghost' | ''
}>()

defineEmits<{ click: [MouseEvent] }>()
</script>
