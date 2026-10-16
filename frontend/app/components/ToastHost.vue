<template>
  <Teleport to="body">
    <div class="toast-stack" aria-live="polite">
      <div v-for="t in toasts" :key="t.id" class="toast" role="status">
        <span class="toast-icon" aria-hidden="true">
          <AppIcon name="check" />
        </span>
        <span>{{ t.text }}</span>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
const { toasts } = useToast()
</script>
