<template>
  <div class="dialog-backdrop dialog-backdrop-sticky" role="presentation">
    <div
      class="dialog"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="login-required-title"
      aria-describedby="login-required-desc"
    >
      <div class="dialog-head">
        <h2 id="login-required-title">Sign in required</h2>
      </div>
      <p id="login-required-desc" class="muted">
        Sign in to browse this drive.
      </p>
      <div class="dialog-actions">
        <a :href="href" class="btn-primary">
          <AppIcon name="login" />
          Sign in
        </a>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ href: string }>()
</script>
