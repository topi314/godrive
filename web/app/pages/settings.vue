<template>
  <div>
    <h1 class="brand" style="font-size:1.8rem;margin:0 0 1rem">Settings</h1>
    <p v-if="error" class="error">{{ error }}</p>

    <section style="margin-bottom:2rem">
      <h2>API tokens</h2>
      <div class="toolbar">
        <input v-model="tokenDesc" placeholder="Description" />
        <button class="primary" @click="createToken">Create</button>
      </div>
      <p v-if="newToken" class="muted">Copy token now: <code>{{ newToken }}</code></p>
      <table class="file-table">
        <thead><tr><th>Prefix</th><th>Description</th><th>Created</th><th></th></tr></thead>
        <tbody>
          <tr v-for="t in tokens" :key="t.token_hash">
            <td>{{ t.token_prefix }}…</td>
            <td>{{ t.description }}</td>
            <td>{{ t.created_at }}</td>
            <td><button class="danger" @click="delToken(t.token_hash)">Delete</button></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section style="margin-bottom:2rem">
      <h2>Shares</h2>
      <table class="file-table">
        <thead><tr><th>ID</th><th>Path</th><th>Created</th></tr></thead>
        <tbody>
          <tr v-for="s in shares" :key="s.id">
            <td><NuxtLink :to="'/s/' + s.id">{{ s.id }}</NuxtLink></td>
            <td>{{ s.path }}</td>
            <td>{{ s.created_at }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section>
      <h2>Users</h2>
      <table class="file-table">
        <thead><tr><th>Username</th><th>Email</th><th>Home</th></tr></thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td>{{ u.username }}</td>
            <td>{{ u.email }}</td>
            <td>{{ u.home }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
const api = useApi()
const tokens = ref<any[]>([])
const shares = ref<any[]>([])
const users = ref<any[]>([])
const tokenDesc = ref('')
const newToken = ref('')
const error = ref('')

async function refresh() {
  error.value = ''
  try {
    tokens.value = await api.listTokens()
    shares.value = await api.listShares()
    users.value = await api.listUsers()
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load settings'
  }
}

async function createToken() {
  const res = await api.createToken(tokenDesc.value)
  newToken.value = res.token
  tokenDesc.value = ''
  await refresh()
}

async function delToken(hash: string) {
  await api.deleteToken(hash)
  await refresh()
}

onMounted(refresh)
</script>
