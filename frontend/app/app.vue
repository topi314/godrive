<template>
  <div class="app-shell">
    <header class="topbar">
      <NuxtLink to="/" class="brand">go<span>drive</span></NuxtLink>
      <div class="topbar-actions">
        <IconBtn
          :name="theme === 'dark' ? 'sun' : 'moon'"
          :label="theme === 'dark' ? 'Light mode' : 'Dark mode'"
          @click="toggleTheme"
        />
        <div
          v-if="user?.authenticated && user.auth_enabled"
          ref="userMenuEl"
          class="user-menu"
        >
          <button
            type="button"
            class="user-menu-trigger"
            :title="user.username || user.email || 'Account'"
            :aria-expanded="userMenuOpen"
            aria-haspopup="menu"
            @click="userMenuOpen = !userMenuOpen"
          >
            <img
              v-if="user.avatar"
              class="user-avatar"
              :src="user.avatar"
              :alt="user.username || 'User'"
            >
            <span v-else class="user-label">{{ user.username || 'Account' }}</span>
          </button>
          <nav v-if="userMenuOpen" class="user-menu-dropdown" role="menu">
            <button
              v-if="user.is_admin"
              type="button"
              role="menuitem"
              @click="openSettings"
            >
              Settings
            </button>
            <a href="/api/logout" role="menuitem">Logout</a>
          </nav>
        </div>
        <IconBtn
          v-else-if="user?.authenticated && user.is_admin"
          name="settings"
          label="Settings"
          @click="settingsOpen = true"
        />
        <a
          v-else-if="user?.auth_enabled"
          :href="loginHref"
          class="icon-btn"
          title="Login"
          aria-label="Login"
        >
          <AppIcon name="login" />
        </a>
      </div>
    </header>
    <main class="main">
      <NuxtPage />
    </main>
    <SettingsDialog v-if="user?.is_admin" v-model="settingsOpen" />
  </div>
</template>

<script setup lang="ts">
const api = useApi()
const user = ref<Awaited<ReturnType<typeof api.me>> | null>(null)
const theme = ref<'dark' | 'light'>('dark')
const settingsOpen = useState('settingsOpen', () => false)
const userMenuOpen = ref(false)
const userMenuEl = ref<HTMLElement | null>(null)
const loginHref = computed(() => {
  if (typeof window === 'undefined') return '/api/login?rd=/'
  return '/api/login?rd=' + encodeURIComponent(window.location.href)
})

function applyTheme(next: 'dark' | 'light') {
  theme.value = next
  const root = document.documentElement
  root.classList.remove('dark', 'light')
  root.classList.add(next)
  localStorage.setItem('godrive-theme', next)
  const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (icon) {
    icon.href = next === 'light' ? '/favicon-light.png' : '/favicon.png'
  }
}

function toggleTheme() {
  applyTheme(theme.value === 'dark' ? 'light' : 'dark')
}

function openSettings() {
  userMenuOpen.value = false
  settingsOpen.value = true
}

function onDocClick(e: MouseEvent) {
  if (!userMenuEl.value?.contains(e.target as Node)) {
    userMenuOpen.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') userMenuOpen.value = false
}

onMounted(async () => {
  const saved = localStorage.getItem('godrive-theme')
  applyTheme(saved === 'light' ? 'light' : 'dark')
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeydown)
  try {
    user.value = await api.me()
  } catch {
    user.value = { authenticated: false }
  }
})

onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
})
</script>
