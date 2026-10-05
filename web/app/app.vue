<template>
  <div class="app-shell">
    <header class="topbar">
      <NuxtLink to="/" class="brand">go<span>drive</span></NuxtLink>
      <div class="topbar-actions">
        <template v-if="user?.authenticated">
          <span>{{ user.username }}</span>
          <NuxtLink v-if="user.is_admin" to="/settings">Settings</NuxtLink>
          <a href="/api/logout">Logout</a>
        </template>
        <a v-else href="/api/login">Login</a>
      </div>
    </header>
    <main class="main">
      <NuxtPage />
    </main>
  </div>
</template>

<script setup lang="ts">
const api = useApi()
const user = ref<Awaited<ReturnType<typeof api.me>> | null>(null)

onMounted(async () => {
  try {
    user.value = await api.me()
  } catch {
    user.value = { authenticated: false }
  }
})
</script>
