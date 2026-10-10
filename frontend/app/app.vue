<template>
  <div class="app-shell">
    <header class="topbar">
      <div class="topbar-brand">
        <NuxtLink to="/" class="brand">go<span>drive</span></NuxtLink>
        <a
          class="icon-btn"
          href="https://github.com/topi314/godrive"
          target="_blank"
          rel="noopener noreferrer"
          title="GitHub"
          aria-label="GitHub"
        >
          <AppIcon name="github" />
        </a>
      </div>
      <div class="topbar-actions">
        <IconBtn
          v-if="user?.authenticated && user.auth_enabled && user.is_admin"
          name="shield"
          :label="user.sudo ? 'Sudo on — see all paths' : 'Sudo off — ACL view'"
          :variant="user.sudo ? 'primary' : ''"
          @click="toggleSudo"
        />
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
            <div class="user-menu-head">
              <strong>{{ user.username || 'Account' }}</strong>
              <span v-if="user.email" class="muted">{{ user.email }}</span>
            </div>
            <button type="button" role="menuitem" @click="openSettings">
              <AppIcon name="settings" />
              Settings
            </button>
            <button
              v-if="user.is_admin"
              type="button"
              role="menuitem"
              @click="openAdmin"
            >
              <AppIcon name="lock" />
              Admin
            </button>
            <div class="user-menu-sep" role="separator" />
            <a href="/api/logout" role="menuitem" class="danger">
              <AppIcon name="logout" />
              Logout
            </a>
          </nav>
        </div>
        <template v-else-if="user?.authenticated && !user.auth_enabled">
          <IconBtn
            v-if="user.is_admin"
            name="lock"
            label="Admin"
            @click="adminOpen = true"
          />
          <IconBtn
            name="settings"
            label="Settings"
            @click="settingsOpen = true"
          />
        </template>
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
      <NuxtPage :page-key="pageKey" />
    </main>
    <SettingsDialog
      v-if="user?.authenticated"
      v-model="settingsOpen"
      :me="user"
      @updated="applyMe"
    />
    <AdminDialog
      v-if="user?.authenticated && user.is_admin"
      v-model="adminOpen"
      :me="user"
    />
    <LoginRequiredDialog v-if="needsLogin" :href="loginHref" />
    <ConfirmDialog />
    <ToastHost />
  </div>
</template>

<script setup lang="ts">
import type { Me } from '~/composables/useApi'
import type { RouteLocationNormalizedLoaded } from 'vue-router'

const api = useApi()
const user = useState<Me | null>('me', () => null)
const meReady = ref(false)
const theme = ref<'dark' | 'light'>('dark')
const settingsOpen = useState('settingsOpen', () => false)
const adminOpen = useState('adminOpen', () => false)
const userMenuOpen = ref(false)
const userMenuEl = ref<HTMLElement | null>(null)

/** Keep BrowserView mounted across folder navigations (default key is full path → remount/flash). */
function pageKey(route: RouteLocationNormalizedLoaded) {
  if (route.path === '/s' || route.path.startsWith('/s/')) {
    return 'share:' + String(route.params.id || '')
  }
  return 'browse'
}
const loginHref = computed(() => {
  if (typeof window === 'undefined') return '/api/login?rd=/'
  return '/api/login?rd=' + encodeURIComponent(window.location.href)
})
const needsLogin = computed(() =>
  meReady.value
  && !!user.value?.auth_enabled
  && !user.value?.authenticated
  && user.value?.guests_allowed === false,
)

watch(needsLogin, (locked) => {
  if (typeof document === 'undefined') return
  document.body.style.overflow = locked ? 'hidden' : ''
}, { immediate: true })

function applyTheme(next: 'dark' | 'light') {
  theme.value = next
  const root = document.documentElement
  root.classList.remove('dark', 'light')
  root.classList.add(next)
  localStorage.setItem('godrive-theme', next)
  const icon =
    document.querySelector<HTMLLinkElement>('link[rel="icon"][type="image/png"]') ||
    document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (icon) {
    icon.href = next === 'light' ? '/favicon-light.png' : '/favicon.png'
  }
}

function toggleTheme() {
  applyTheme(theme.value === 'dark' ? 'light' : 'dark')
}

async function toggleSudo() {
  if (!user.value?.is_admin) return
  const next = !user.value.sudo
  try {
    applyMe(await api.updateMe({ sudo: next }))
  } catch {
    /* keep current */
  }
}

function applyMe(me: Me) {
  user.value = me
}

function openSettings() {
  userMenuOpen.value = false
  settingsOpen.value = true
}

function openAdmin() {
  userMenuOpen.value = false
  adminOpen.value = true
}

async function refreshMe() {
  try {
    user.value = await api.me()
  } catch {
    /* keep current */
  }
}

let sessionRefreshTimer: ReturnType<typeof setTimeout> | null = null

function clearSessionRefreshTimer() {
  if (sessionRefreshTimer) {
    clearTimeout(sessionRefreshTimer)
    sessionRefreshTimer = null
  }
}

function scheduleSessionRefresh() {
  clearSessionRefreshTimer()
  const raw = user.value?.session_expires_at
  if (!user.value?.authenticated || user.value.is_guest || !raw) return
  const at = Date.parse(raw)
  if (!Number.isFinite(at)) return
  const delay = Math.max(5_000, at - Date.now() - 60_000)
  sessionRefreshTimer = setTimeout(async () => {
    try {
      const res = await api.refreshSession()
      if (user.value && res?.session_expires_at) {
        user.value = { ...user.value, session_expires_at: res.session_expires_at }
      }
    } catch {
      /* session will be retried on the next 401 */
    }
  }, delay)
}

watch(() => user.value?.session_expires_at, scheduleSessionRefresh)

function onDocClick(e: MouseEvent) {
  if (!userMenuEl.value?.contains(e.target as Node)) {
    userMenuOpen.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (adminOpen.value) {
    adminOpen.value = false
    return
  }
  if (settingsOpen.value) {
    settingsOpen.value = false
    return
  }
  userMenuOpen.value = false
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
  } finally {
    meReady.value = true
  }
})

onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
  clearSessionRefreshTimer()
})
</script>
