<template>
  <div
    @dragover.prevent="drag = true"
    @dragleave.prevent="drag = false"
    @drop.prevent="onDrop"
  >
    <nav class="crumbs">
      <NuxtLink to="/">/</NuxtLink>
      <template v-for="(c, i) in crumbs" :key="c.path">
        <span class="sep">/</span>
        <NuxtLink v-if="i < crumbs.length - 1" :to="c.path">{{ c.name }}</NuxtLink>
        <span v-else class="crumb-current">{{ c.name }}</span>
      </template>
    </nav>

    <div class="toolbar">
      <button :disabled="!selected.length" @click="downloadSelected">Download</button>
      <button :disabled="!canCreate" @click="fileInput?.click()">Upload</button>
      <button :disabled="!selected.length || !canDelete" class="danger" @click="deleteSelected">Delete</button>
      <button v-if="selectedOne && canShare" @click="shareSelected">Share</button>
      <button v-if="selectedOne && canACL" @click="openACL">Permissions</button>
      <input ref="fileInput" type="file" multiple hidden @change="onPick" />
    </div>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="loading" class="muted">Loading…</p>

    <table class="file-table" v-if="!loading">
      <thead>
        <tr>
          <th style="width:2rem"><input type="checkbox" :checked="allSelected" @change="toggleAll" /></th>
          <th>Name</th>
          <th>Size</th>
          <th>Modified</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="f in files"
          :key="f.path"
          :class="{ selected: selected.includes(f.path) }"
        >
          <td><input type="checkbox" :checked="selected.includes(f.path)" @change="toggle(f.path)" /></td>
          <td>
            <NuxtLink v-if="f.is_dir" class="name" :to="linkFor(f)">{{ f.name }}/</NuxtLink>
            <a v-else class="name" :href="f.path + '?dl=1'">{{ f.name }}</a>
          </td>
          <td class="muted">{{ formatSize(f.size) }}</td>
          <td class="muted">{{ formatDate(f.date) }}</td>
        </tr>
      </tbody>
    </table>

    <div class="dropzone" :class="{ active: drag }" v-if="canCreate">
      Drop files to upload
      <div v-if="progress >= 0" class="progress"><i :style="{ width: progress + '%' }" /></div>
    </div>

    <div v-if="aclOpen" class="dialog-backdrop" @click.self="aclOpen = false">
      <div class="dialog">
        <h2>Permissions</h2>
        <p class="muted">{{ aclPath }}</p>
        <label>
          Publish (everyone can read)
          <input type="checkbox" v-model="publish" />
        </label>
        <div class="dialog-actions">
          <button @click="aclOpen = false">Cancel</button>
          <button class="primary" @click="saveACL">Save</button>
        </div>
      </div>
    </div>

    <div v-if="shareUrl" class="dialog-backdrop" @click.self="shareUrl = ''">
      <div class="dialog">
        <h2>Share link</h2>
        <label>
          URL
          <input :value="absoluteShare" readonly @focus="($event.target as HTMLInputElement).select()" />
        </label>
        <div class="dialog-actions">
          <button class="primary" @click="copyShare">Copy</button>
          <button @click="shareUrl = ''">Close</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ basePath: string; shareId?: string }>()
const api = useApi()
const files = ref<FileEntry[]>([])
const selected = ref<string[]>([])
const loading = ref(true)
const error = ref('')
const drag = ref(false)
const progress = ref(-1)
const fileInput = ref<HTMLInputElement | null>(null)
const aclOpen = ref(false)
const aclPath = ref('/')
const publish = ref(false)
const shareUrl = ref('')

const crumbs = computed(() => {
  const parts = props.basePath.split('/').filter(Boolean)
  // for share routes keep /s/id prefix out of drive crumbs when possible
  const out: { name: string; path: string }[] = []
  let acc = ''
  for (const part of parts) {
    acc += '/' + part
    out.push({ name: part, path: acc })
  }
  return out
})

const selectedOne = computed(() => selected.value.length === 1 ? selected.value[0] : '')
const selectedEntries = computed(() => files.value.filter(f => selected.value.includes(f.path)))
const canCreate = computed(() => selectedEntries.value[0] ? false : files.value.some(f => hasPerm(f.permissions, Perm.Create)) || files.value.length === 0 || true)
const folderPerms = computed(() => {
  // use max create from listing context — backend enforces
  return Perm.Create | Perm.Delete | Perm.Share | Perm.UpdatePermissions
})
const canDelete = computed(() => selectedEntries.value.every(f => hasPerm(f.permissions, Perm.Delete) || f.permissions === 0))
const canShare = computed(() => {
  const f = files.value.find(x => x.path === selectedOne.value)
  return !!f && hasPerm(f.permissions, Perm.Share)
})
const canACL = computed(() => {
  const f = files.value.find(x => x.path === selectedOne.value)
  return !!f && hasPerm(f.permissions, Perm.UpdatePermissions)
})
const allSelected = computed(() => files.value.length > 0 && selected.value.length === files.value.length)
const absoluteShare = computed(() => (typeof window !== 'undefined' ? window.location.origin : '') + shareUrl.value)

async function load() {
  loading.value = true
  error.value = ''
  selected.value = []
  try {
    const res = props.shareId
      ? await api.listPath(props.basePath)
      : await api.listFiles(props.basePath || '/')
    files.value = res.files || []
  } catch (e: any) {
    error.value = e?.data?.message || e.message || 'Failed to load'
    files.value = []
  } finally {
    loading.value = false
  }
}

function linkFor(f: FileEntry) {
  return f.path
}

function toggle(path: string) {
  if (selected.value.includes(path)) selected.value = selected.value.filter(p => p !== path)
  else selected.value = [...selected.value, path]
}

function toggleAll(e: Event) {
  const on = (e.target as HTMLInputElement).checked
  selected.value = on ? files.value.map(f => f.path) : []
}

async function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files?.length) return
  await uploadFiles([...input.files])
  input.value = ''
}

async function onDrop(e: DragEvent) {
  drag.value = false
  const list = e.dataTransfer?.files
  if (!list?.length) return
  await uploadFiles([...list])
}

async function uploadFiles(list: File[]) {
  progress.value = 0
  try {
    for (const file of list) {
      await api.upload(props.basePath || '/', file, '', (n) => { progress.value = n })
    }
    await load()
  } catch (e: any) {
    error.value = e.message || 'Upload failed'
  } finally {
    progress.value = -1
  }
}

async function deleteSelected() {
  if (!confirm('Delete selected?')) return
  await api.remove(selected.value)
  await load()
}

function downloadSelected() {
  for (const p of selected.value) {
    window.open(p + '?dl=1', '_blank')
  }
}

async function shareSelected() {
  const res = await api.createShare(selectedOne.value)
  shareUrl.value = res.url
}

async function copyShare() {
  await navigator.clipboard.writeText(absoluteShare.value)
}

async function openACL() {
  aclPath.value = selectedOne.value
  const res = await api.getPermissions(aclPath.value)
  publish.value = res.acl.some(
    (a: any) => a.principal_type === 'everyone' && (a.allow & Perm.Read) === Perm.Read && a.deny === 0,
  )
  aclOpen.value = true
}

async function saveACL() {
  const acl = publish.value
    ? [{ principal_type: 'everyone', principal_id: '*', allow: Perm.Read, deny: 0 }]
    : []
  await api.putPermissions(aclPath.value, acl)
  aclOpen.value = false
  await load()
}

function formatSize(n: number) {
  if (n < 1024) return n + ' B'
  const u = ['KiB', 'MiB', 'GiB', 'TiB']
  let i = -1
  do { n /= 1024; i++ } while (n >= 1024 && i < u.length - 1)
  return n.toFixed(1) + ' ' + u[i]
}

function formatDate(d: string) {
  try { return new Date(d).toLocaleString() } catch { return d }
}

watch(() => props.basePath, load, { immediate: true })
</script>
