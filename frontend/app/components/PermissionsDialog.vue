<template>
  <div v-if="open" class="dialog-backdrop" @click.self="close" @keydown.escape="close">
    <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="acl-title">
      <div class="dialog-head">
        <h2 id="acl-title">Permissions</h2>
        <IconBtn name="x" label="Close" variant="ghost" @click="close" />
      </div>

      <label class="acl-path-field">
        Path
        <input
          ref="pathInput"
          v-model="pathDraft"
          spellcheck="false"
          autocomplete="off"
          @keydown.enter.prevent="commitPath"
          @blur="commitPath"
        >
      </label>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-else-if="loading" class="muted">Loading…</p>

      <template v-else>
        <p class="muted effective-line">
          Your effective access:
          <span
            v-for="p in permFlags"
            :key="p.bit"
            class="perm-chip"
            :class="[{ on: hasPerm(effective, p.bit) }, 'perm-' + p.short.toLowerCase()]"
          >
            {{ p.label }}
          </span>
        </p>

        <div class="acl-table-wrap">
          <table class="file-table acl-table">
            <thead>
              <tr>
                <th>Source</th>
                <th>Type</th>
                <th>Principal</th>
                <th
                  v-for="p in permFlags"
                  :key="'h-' + p.bit"
                  class="perm-col"
                  :class="'perm-' + p.short.toLowerCase()"
                >{{ p.short }}</th>
                <th class="acl-actions">
                  <IconBtn name="plus" label="Add rule" variant="primary" @click="addRow" />
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, idx) in inherited" :key="'i-' + idx" class="inherited">
                <td class="muted"><code>{{ row.path }}</code></td>
                <td class="muted">{{ row.principal_type }}</td>
                <td class="muted">{{ row.principal_id }}</td>
                <td v-for="p in permFlags" :key="'ic-' + p.bit" class="perm-cell">
                  <span
                    class="perm-mark"
                    :class="markClass(row, p.bit)"
                    :title="p.label"
                  >{{ markLabel(row, p.bit) }}</span>
                </td>
                <td class="muted inherit-tag">inherited</td>
              </tr>
              <tr v-for="(row, idx) in rows" :key="'l-' + idx">
                <td class="muted">this path</td>
                <td>
                  <select v-model="row.principal_type" @change="onTypeChange(row)">
                    <option value="everyone">everyone</option>
                    <option value="guest">guest</option>
                    <option value="group">group</option>
                    <option value="user">user</option>
                    <option value="share">share</option>
                  </select>
                </td>
                <td>
                  <select
                    v-if="row.principal_type === 'group'"
                    v-model="row.principal_id"
                  >
                    <option value="" disabled>Select group</option>
                    <option v-for="g in groupOptions(row.principal_id)" :key="g" :value="g">{{ g }}</option>
                  </select>
                  <select
                    v-else-if="row.principal_type === 'user'"
                    v-model="row.principal_id"
                  >
                    <option value="" disabled>Select user</option>
                    <option v-for="u in userOptions(row.principal_id)" :key="u.id" :value="u.id">
                      {{ userLabel(u) }}
                    </option>
                  </select>
                  <input
                    v-else-if="row.principal_type === 'share'"
                    v-model="row.principal_id"
                    placeholder="share id"
                    spellcheck="false"
                  >
                  <input
                    v-else
                    v-model="row.principal_id"
                    disabled
                    placeholder="*"
                  >
                </td>
                <td v-for="p in permFlags" :key="'c-' + p.bit" class="perm-cell">
                  <button
                    type="button"
                    class="perm-mark"
                    :class="markClass(row, p.bit)"
                    :title="p.label + ' (click to cycle allow / deny / none)'"
                    @click="cyclePerm(row, p.bit)"
                  >{{ markLabel(row, p.bit) }}</button>
                </td>
                <td class="acl-actions">
                  <IconBtn name="trash" label="Remove rule" variant="danger" @click="rows.splice(idx, 1)" />
                </td>
              </tr>
              <tr v-if="!rows.length && !inherited.length">
                <td colspan="10" class="muted">No ACL rules on this path or its parents.</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="toolbar acl-toolbar">
          <label class="quick-publish">
            <input type="checkbox" :checked="guestPublished" @change="toggleGuestPublish(($event.target as HTMLInputElement).checked)">
            Guests can read
          </label>
          <IconBtn name="check" label="Save" variant="primary" :disabled="saving" @click="save" />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { hasPerm, Perm } from '~/composables/useApi'

type ACLRow = {
  path?: string
  principal_type: string
  principal_id: string
  allow: number
  deny: number
}

const open = defineModel<boolean>({ required: true })
const path = defineModel<string>('path', { required: true })
const props = defineProps<{ selectPath?: boolean }>()
const emit = defineEmits<{ saved: [] }>()

const api = useApi()
const pathInput = ref<HTMLInputElement | null>(null)
const pathDraft = ref('/')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const effective = ref(0)
const rows = ref<ACLRow[]>([])
const inherited = ref<ACLRow[]>([])
const availableGroups = ref<string[]>([])
const availableUsers = ref<{ id: string; username: string; email?: string }[]>([])

const permFlags = [
  { bit: Perm.Read, label: 'Read', short: 'R' },
  { bit: Perm.Create, label: 'Create', short: 'C' },
  { bit: Perm.Update, label: 'Update', short: 'U' },
  { bit: Perm.Delete, label: 'Delete', short: 'D' },
  { bit: Perm.UpdatePermissions, label: 'ACL', short: 'A' },
  { bit: Perm.Share, label: 'Share', short: 'S' },
]

const guestPublished = computed(() => {
  const local = rows.value.find(r => r.principal_type === 'guest')
  if (local) {
    return hasPerm(local.allow, Perm.Read) && !hasPerm(local.deny, Perm.Read)
  }
  let allow = 0
  let deny = 0
  for (const r of inherited.value) {
    if (r.principal_type !== 'guest') continue
    allow |= r.allow
    deny |= r.deny
  }
  return hasPerm(allow, Perm.Read) && !hasPerm(deny, Perm.Read)
})

function userLabel(u: { id: string; username: string; email?: string }) {
  if (u.username) return u.username
  if (u.email) return u.email
  return u.id
}

function groupOptions(current: string) {
  const list = [...availableGroups.value]
  if (current && !list.includes(current)) list.unshift(current)
  return list
}

function userOptions(current: string) {
  const list = [...availableUsers.value]
  if (current && !list.some(u => u.id === current)) {
    list.unshift({ id: current, username: current })
  }
  return list
}

function onTypeChange(row: ACLRow) {
  if (row.principal_type === 'everyone' || row.principal_type === 'guest') {
    row.principal_id = '*'
  } else if (row.principal_type === 'group') {
    if (!availableGroups.value.includes(row.principal_id)) {
      row.principal_id = availableGroups.value[0] || ''
    }
  } else if (row.principal_type === 'user') {
    if (!availableUsers.value.some(u => u.id === row.principal_id)) {
      row.principal_id = availableUsers.value[0]?.id || ''
    }
  } else if (row.principal_type === 'share') {
    // Keep existing share id when switching back to share; otherwise clear for typing.
    if (!row.principal_id || row.principal_id === '*') row.principal_id = ''
  } else {
    row.principal_id = ''
  }
}

function addRow() {
  rows.value.push({
    principal_type: 'group',
    principal_id: availableGroups.value[0] || '',
    allow: Perm.Read,
    deny: 0,
  })
}

function seedDefaultRow() {
  if (rows.value.length || !availableGroups.value.length) return
  addRow()
}

function cyclePerm(row: ACLRow, bit: number) {
  const allowed = hasPerm(row.allow, bit)
  const denied = hasPerm(row.deny, bit)
  if (!allowed && !denied) {
    row.allow |= bit
  } else if (allowed) {
    row.allow &= ~bit
    row.deny |= bit
  } else {
    row.deny &= ~bit
  }
}

function markClass(row: ACLRow, bit: number) {
  if (hasPerm(row.deny, bit)) return 'deny'
  if (hasPerm(row.allow, bit)) return 'allow'
  return 'none'
}

function markLabel(row: ACLRow, bit: number) {
  if (hasPerm(row.deny, bit)) return '−'
  if (hasPerm(row.allow, bit)) return '+'
  return '·'
}

function toggleGuestPublish(on: boolean) {
  const idx = rows.value.findIndex(r => r.principal_type === 'guest')
  if (on) {
    if (idx >= 0) {
      rows.value[idx].allow |= Perm.Read
      rows.value[idx].deny &= ~Perm.Read
    } else {
      rows.value.push({
        principal_type: 'guest',
        principal_id: '*',
        allow: Perm.Read,
        deny: 0,
      })
    }
  } else if (idx >= 0) {
    const row = rows.value[idx]
    row.allow &= ~Perm.Read
    row.deny |= Perm.Read
  } else {
    rows.value.push({
      principal_type: 'guest',
      principal_id: '*',
      allow: 0,
      deny: Perm.Read,
    })
  }
}

function normalizeClientPath(p: string) {
  p = (p || '').trim()
  if (!p) return '/'
  if (!p.startsWith('/')) p = '/' + p
  p = p.replace(/\/+/g, '/')
  if (p.length > 1) p = p.replace(/\/+$/, '')
  return p
}

function commitPath() {
  const next = normalizeClientPath(pathDraft.value)
  pathDraft.value = next
  if (path.value !== next) {
    path.value = next
  }
}

function close() {
  open.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getPermissions(path.value)
    effective.value = res.effective || 0
    rows.value = (res.acl || []).map((a: any) => ({
      principal_type: a.principal_type,
      principal_id: a.principal_id,
      allow: Number(a.allow) || 0,
      deny: Number(a.deny) || 0,
    }))
    inherited.value = (res.inherited || []).map((a: any) => ({
      path: a.path,
      principal_type: a.principal_type,
      principal_id: a.principal_id,
      allow: Number(a.allow) || 0,
      deny: Number(a.deny) || 0,
    }))
    availableGroups.value = res.available_groups || []
    availableUsers.value = res.available_users || []
    seedDefaultRow()
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load permissions'
  } finally {
    loading.value = false
  }
}

async function save() {
  commitPath()
  saving.value = true
  error.value = ''
  try {
    const acl = rows.value.map(r => ({
      principal_type: r.principal_type,
      principal_id: (r.principal_type === 'everyone' || r.principal_type === 'guest')
        ? '*'
        : r.principal_id.trim(),
      allow: r.allow,
      deny: r.deny,
    }))
    for (const row of acl) {
      if ((row.principal_type === 'group' || row.principal_type === 'user') && !row.principal_id) {
        throw new Error('Group/user rules need a principal id')
      }
    }
    await api.putPermissions(path.value, acl)
    open.value = false
    emit('saved')
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to save permissions'
  } finally {
    saving.value = false
  }
}

watch(open, async (isOpen) => {
  if (!isOpen) return
  pathDraft.value = normalizeClientPath(path.value)
  await refresh()
  if (props.selectPath) {
    await nextTick()
    pathInput.value?.focus()
    pathInput.value?.select()
  }
})

watch(path, () => {
  if (!open.value) return
  pathDraft.value = normalizeClientPath(path.value)
  refresh()
})
</script>
