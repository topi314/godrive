<template>
  <div v-if="open" class="dialog-backdrop" @click.self="close" @keydown.escape="close">
    <div class="dialog dialog-wide settings-dialog" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <div class="dialog-head">
        <h2 id="settings-title">Settings</h2>
        <IconBtn name="x" label="Close" variant="ghost" @click="close" />
      </div>

      <nav class="settings-tabs" aria-label="Settings sections">
        <button
          v-for="t in tabs"
          :key="t.id"
          type="button"
          class="settings-tab"
          :class="{ active: tab === t.id }"
          @click="tab = t.id"
        >
          {{ t.label }}
        </button>
      </nav>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="loading" class="muted">Loading…</p>

      <template v-if="!loading">
        <section v-show="tab === 'account'" class="settings-section">
          <h3>Account</h3>
          <div class="account-card">
            <img v-if="me?.avatar" class="account-avatar" :src="me.avatar" alt="">
            <div class="account-meta">
              <strong>{{ me?.username || '—' }}</strong>
              <span class="muted">{{ me?.email || '—' }}</span>
              <span v-if="me?.is_admin" class="badge">admin</span>
            </div>
          </div>
          <label>
            Home directory
            <input v-model="homeDraft" placeholder="/" @keydown.enter.prevent="saveHome">
          </label>
          <p class="muted tip">Used as your default landing path after login.</p>
          <div class="dialog-actions">
            <IconBtn name="check" label="Save home" variant="primary" :disabled="savingHome" @click="saveHome" />
          </div>
        </section>

        <section v-show="tab === 'tokens'" class="settings-section">
          <h3>API tokens</h3>
          <div class="toolbar">
            <input v-model="tokenDesc" placeholder="Description" @keydown.enter.prevent="createToken">
            <IconBtn name="plus" label="Create token" variant="primary" @click="createToken" />
          </div>
          <p v-if="newToken" class="muted token-banner">
            Copy token now — it won’t be shown again:
            <code>{{ newToken }}</code>
            <IconBtn name="copy" label="Copy token" @click="copyToken" />
          </p>
          <table class="file-table">
            <thead>
              <tr>
                <th>Prefix</th>
                <th>Description</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="t in tokens" :key="t.token_hash">
                <td>{{ t.token_prefix }}…</td>
                <td>{{ t.description || '—' }}</td>
                <td class="muted">{{ formatDate(t.created_at) }}</td>
                <td>
                  <IconBtn name="trash" label="Delete token" variant="danger" @click="delToken(t.token_hash)" />
                </td>
              </tr>
              <tr v-if="!tokens.length">
                <td colspan="4" class="muted">No tokens yet</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-show="tab === 'shares'" class="settings-section">
          <h3>Your shares</h3>
          <table class="file-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Path</th>
                <th>Created</th>
                <th>Expires</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in shares" :key="s.id">
                <td><NuxtLink :to="'/s/' + s.id" @click="close">{{ s.id }}</NuxtLink></td>
                <td>{{ s.path }}</td>
                <td class="muted">{{ formatDate(s.created_at) }}</td>
                <td class="muted">{{ formatExpiry(s.expires_at) }}</td>
                <td>
                  <IconBtn name="trash" label="Delete share" variant="danger" @click="delShare(s.id)" />
                </td>
              </tr>
              <tr v-if="!shares.length">
                <td colspan="5" class="muted">No shares yet</td>
              </tr>
            </tbody>
          </table>
        </section>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { formatDate } from '~/composables/format'
import type { Me } from '~/composables/useApi'

const open = defineModel<boolean>({ required: true })
const props = defineProps<{ me: Me | null }>()
const emit = defineEmits<{ updated: [] }>()

const api = useApi()
const { toast } = useToast()
const tab = ref<'account' | 'tokens' | 'shares'>('account')
const tokens = ref<any[]>([])
const shares = ref<any[]>([])
const tokenDesc = ref('')
const newToken = ref('')
const homeDraft = ref('/')
const error = ref('')
const loading = ref(false)
const savingHome = ref(false)

const tabs = [
  { id: 'account' as const, label: 'Account' },
  { id: 'tokens' as const, label: 'Tokens' },
  { id: 'shares' as const, label: 'Shares' },
]

function close() {
  open.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))

function formatExpiry(v: string | null | undefined) {
  if (!v) return 'Never'
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  if (d.getTime() <= Date.now()) return 'Expired'
  return d.toLocaleString()
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    ;[tokens.value, shares.value] = await Promise.all([api.listTokens(), api.listShares()])
    homeDraft.value = props.me?.home || '/'
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load settings'
  } finally {
    loading.value = false
  }
}

async function saveHome() {
  savingHome.value = true
  error.value = ''
  try {
    await api.updateMe({ home: homeDraft.value || '/' })
    emit('updated')
    toast('Settings saved')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to save home'
  } finally {
    savingHome.value = false
  }
}

async function createToken() {
  try {
    const res = await api.createToken(tokenDesc.value)
    newToken.value = res.token
    tokenDesc.value = ''
    await refresh()
    toast('Token created')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to create token'
  }
}

async function copyToken() {
  if (!newToken.value) return
  await navigator.clipboard.writeText(newToken.value)
  toast('Token copied')
}

async function delToken(hash: string) {
  try {
    await api.deleteToken(hash)
    await refresh()
    toast('Token deleted')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to delete token'
  }
}

async function delShare(id: string) {
  try {
    await api.deleteShare(id)
    shares.value = shares.value.filter(s => s.id !== id)
    toast('Share deleted')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to delete share'
  }
}

watch(open, (v) => {
  if (v) {
    newToken.value = ''
    error.value = ''
    tab.value = 'account'
    refresh()
  }
})

watch(() => props.me?.home, (h) => {
  if (h) homeDraft.value = h
})
</script>
