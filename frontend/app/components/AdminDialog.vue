<template>
  <div v-if="open" class="dialog-backdrop" @click.self="close" @keydown.escape="close">
    <div class="dialog dialog-wide settings-dialog" role="dialog" aria-modal="true" aria-labelledby="admin-title">
      <div class="dialog-head">
        <h2 id="admin-title">Admin</h2>
        <IconBtn name="x" label="Close" variant="ghost" @click="close" />
      </div>

      <nav class="settings-tabs" aria-label="Admin sections">
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

      <section v-show="tab === 'users'" class="settings-section">
          <p v-if="loadingUsers" class="muted">Loading…</p>
          <template v-else>
          <p class="muted user-list-count">{{ users.length }} {{ users.length === 1 ? 'user' : 'users' }}</p>
          <ul class="user-list">
            <li v-for="u in users" :key="u.id">
              <details class="user-fold">
                <summary>
                  <img v-if="u.avatar" class="user-fold-avatar" :src="u.avatar" alt="">
                  <span class="user-fold-meta">
                    <strong>{{ u.username }}</strong>
                    <span class="muted">{{ u.email || u.id }}</span>
                  </span>
                  <span v-if="u.is_admin" class="badge">admin</span>
                  <span class="muted user-fold-count">{{ groupCountLabel(u) }}</span>
                </summary>
                <div class="user-fold-body">
                  <div class="group-chips">
                    <span v-for="g in userGroups(u)" :key="g" class="group-chip">{{ g }}</span>
                    <span v-if="!userGroups(u).length" class="muted">No mapped groups</span>
                  </div>
                  <label class="user-home-label">
                    Home
                    <input
                      class="home-input"
                      :value="userHomeDraft[u.id] ?? u.home"
                      @input="userHomeDraft[u.id] = ($event.target as HTMLInputElement).value"
                      @keydown.enter.prevent="saveUserHome(u.id)"
                    >
                  </label>
                  <div class="dialog-actions">
                    <IconBtn
                      name="trash"
                      label="Delete user"
                      variant="danger"
                      :disabled="u.id === me?.id"
                      @click="delUser(u.id)"
                    />
                    <IconBtn name="check" label="Save home" variant="primary" @click="saveUserHome(u.id)" />
                  </div>
                </div>
              </details>
            </li>
            <li v-if="!users.length" class="muted user-list-empty">No users</li>
          </ul>
          </template>
        </section>

        <section v-show="tab === 'access'" class="settings-section">
          <p v-if="loadingACL" class="muted">Loading…</p>
          <template v-else>
          <p class="muted user-list-count">{{ aclGroups.length }} {{ aclGroups.length === 1 ? 'path' : 'paths' }}</p>
          <table class="file-table">
            <thead>
              <tr>
                <th>Path</th>
                <th>Rules</th>
                <th class="acl-actions">
                  <IconBtn name="plus" label="Add permissions" variant="primary" @click="editACL('/', true)" />
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="g in aclGroups" :key="g.path">
                <td>
                  <button type="button" class="path-link" @click="editACL(g.path)">
                    <code>{{ g.path }}</code>
                  </button>
                </td>
                <td class="muted">
                  <span v-for="(r, i) in g.rules" :key="i" class="acl-pill">
                    {{ r.principal_type }}{{ r.principal_id && r.principal_id !== '*' ? ':' + r.principal_id : '' }}
                    <span class="perm-mini">
                      <template v-if="permSummary(r.allow, r.deny).length">
                        <span
                          v-for="p in permSummary(r.allow, r.deny)"
                          :key="p.key"
                          class="perm-letter"
                          :class="[p.tone, 'perm-' + p.key]"
                        >{{ p.sign }}{{ p.label }}</span>
                      </template>
                      <span v-else class="muted">·</span>
                    </span>
                  </span>
                </td>
                <td class="acl-actions">
                  <button type="button" class="row-text-btn" @click="editACL(g.path)">Edit</button>
                </td>
              </tr>
              <tr v-if="!aclGroups.length">
                <td colspan="3" class="muted">No ACL rules yet</td>
              </tr>
            </tbody>
          </table>
          </template>
        </section>
    </div>
  </div>

  <PermissionsDialog
    v-model="aclEditorOpen"
    v-model:path="aclEditorPath"
    :select-path="aclSelectPath"
    @saved="onACLSaved"
  />
</template>

<script setup lang="ts">
import { PERM_FLAGS } from '~/composables/permBits'
import { hasPerm, type Me } from '~/composables/useApi'

const open = defineModel<boolean>({ required: true })
const props = defineProps<{ me: Me | null }>()

const api = useApi()
const { toast } = useToast()
const tab = ref<'users' | 'access'>('users')
const users = ref<any[]>([])
const allACL = ref<any[]>([])
const userHomeDraft = reactive<Record<string, string>>({})
const aclEditorOpen = ref(false)
const aclEditorPath = ref('/')
const aclSelectPath = ref(false)
const error = ref('')
const loadingUsers = ref(false)
const loadingACL = ref(false)
const usersLoaded = ref(false)
const aclLoaded = ref(false)

const tabs = [
  { id: 'users' as const, label: 'Users' },
  { id: 'access' as const, label: 'Access' },
]

const aclGroups = computed(() => {
  const map = new Map<string, any[]>()
  for (const row of allACL.value) {
    const list = map.get(row.path) || []
    list.push(row)
    map.set(row.path, list)
  }
  return [...map.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([path, rules]) => ({ path, rules }))
})

function close() {
  open.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))

function userGroups(u: { groups?: string[] | string }) {
  const raw = u.groups
  let list: string[] = []
  if (Array.isArray(raw)) list = raw
  else if (typeof raw === 'string' && raw.trim()) {
    try {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed)) list = parsed
    } catch {
      list = raw.split(',').map(s => s.trim()).filter(Boolean)
    }
  }
  return [...new Set(list.filter(Boolean))]
}

function groupCountLabel(u: { groups?: string[] | string }) {
  const n = userGroups(u).length
  return n === 1 ? '1 group' : `${n} groups`
}

function permSummary(allow: number, deny: number) {
  return PERM_FLAGS.flatMap(({ bit, short }) => {
    const key = short.toLowerCase()
    if (hasPerm(deny, bit)) return [{ key, label: short, sign: '-', tone: 'deny' as const }]
    if (hasPerm(allow, bit)) return [{ key, label: short, sign: '+', tone: 'allow' as const }]
    return []
  })
}

function editACL(path: string, selectPath = false) {
  aclEditorPath.value = path || '/'
  aclSelectPath.value = selectPath
  aclEditorOpen.value = true
}

async function onACLSaved() {
  await loadACL(true)
  toast('Permissions saved')
}

async function loadUsers(force = false) {
  if (usersLoaded.value && !force) return
  loadingUsers.value = true
  error.value = ''
  try {
    users.value = await api.listUsers()
    usersLoaded.value = true
    for (const u of users.value) {
      if (userHomeDraft[u.id] == null) userHomeDraft[u.id] = u.home
    }
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load users'
  } finally {
    loadingUsers.value = false
  }
}

async function loadACL(force = false) {
  if (aclLoaded.value && !force) return
  loadingACL.value = true
  error.value = ''
  try {
    allACL.value = await api.listAllPermissions()
    aclLoaded.value = true
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load ACL'
  } finally {
    loadingACL.value = false
  }
}

async function saveUserHome(id: string) {
  error.value = ''
  try {
    const home = userHomeDraft[id] || '/'
    const updated = await api.updateUser(id, { home })
    const idx = users.value.findIndex(u => u.id === id)
    if (idx >= 0) users.value[idx] = updated
    toast('User saved')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to update user'
  }
}

async function delUser(id: string) {
  if (!confirm('Delete this user and their sessions/tokens?')) return
  error.value = ''
  try {
    await api.deleteUser(id)
    users.value = users.value.filter(u => u.id !== id)
    toast('User deleted')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to delete user'
  }
}

watch(open, (v) => {
  if (v) {
    tab.value = 'users'
    error.value = ''
    usersLoaded.value = false
    aclLoaded.value = false
    users.value = []
    allACL.value = []
    void loadUsers()
  }
})

watch(tab, (t) => {
  if (!open.value) return
  if (t === 'users') void loadUsers()
  if (t === 'access') void loadACL()
})
</script>
