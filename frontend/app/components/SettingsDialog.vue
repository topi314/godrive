<template>
  <div v-if="open" class="dialog-backdrop" @click.self="close" @keydown.escape="close">
    <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <div class="dialog-head">
        <h2 id="settings-title">Settings</h2>
        <IconBtn name="x" label="Close" @click="close" />
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-else-if="loading" class="muted">Loading…</p>

      <template v-else>
        <section class="settings-section">
          <h3>API tokens</h3>
          <div class="toolbar">
            <input v-model="tokenDesc" placeholder="Description" @keydown.enter.prevent="createToken" />
            <IconBtn name="plus" label="Create token" variant="primary" @click="createToken" />
          </div>
          <p v-if="newToken" class="muted">
            Copy token now: <code>{{ newToken }}</code>
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
                <td>{{ t.description }}</td>
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

        <section class="settings-section">
          <h3>Shares</h3>
          <table class="file-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Path</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in shares" :key="s.id">
                <td><NuxtLink :to="'/s/' + s.id" @click="close">{{ s.id }}</NuxtLink></td>
                <td>{{ s.path }}</td>
                <td class="muted">{{ formatDate(s.created_at) }}</td>
              </tr>
              <tr v-if="!shares.length">
                <td colspan="3" class="muted">No shares yet</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="settings-section">
          <h3>Users</h3>
          <table class="file-table">
            <thead>
              <tr>
                <th>Username</th>
                <th>Email</th>
                <th>Home</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="u in users" :key="u.id">
                <td>{{ u.username }}</td>
                <td>{{ u.email }}</td>
                <td>{{ u.home }}</td>
              </tr>
              <tr v-if="!users.length">
                <td colspan="3" class="muted">No users</td>
              </tr>
            </tbody>
          </table>
        </section>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
const open = defineModel<boolean>({ required: true })

const api = useApi()
const tokens = ref<any[]>([])
const shares = ref<any[]>([])
const users = ref<any[]>([])
const tokenDesc = ref('')
const newToken = ref('')
const error = ref('')
const loading = ref(false)

function close() {
  open.value = false
}

function formatDate(v: string) {
  if (!v) return '—'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString()
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    ;[tokens.value, shares.value, users.value] = await Promise.all([
      api.listTokens(),
      api.listShares(),
      api.listUsers(),
    ])
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load settings'
  } finally {
    loading.value = false
  }
}

async function createToken() {
  try {
    const res = await api.createToken(tokenDesc.value)
    newToken.value = res.token
    tokenDesc.value = ''
    await refresh()
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to create token'
  }
}

async function delToken(hash: string) {
  try {
    await api.deleteToken(hash)
    await refresh()
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to delete token'
  }
}

watch(open, (v) => {
  if (v) {
    newToken.value = ''
    refresh()
  }
})
</script>
