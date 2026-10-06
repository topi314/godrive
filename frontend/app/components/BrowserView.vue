<template>
  <div
    @dragover.prevent="drag = true"
    @dragleave.prevent="drag = false"
    @drop.prevent="onDrop"
  >
    <nav class="crumbs" aria-label="Breadcrumb">
      <NuxtLink :to="rootLink" class="crumb-root">{{ rootLabel }}</NuxtLink>
      <template v-for="(c, i) in crumbs" :key="c.path">
        <!-- Root is already "/"; don't render a second slash before the first segment. -->
        <span v-if="i > 0 || rootLabel !== '/'" class="sep" aria-hidden="true">/</span>
        <NuxtLink v-if="i < crumbs.length - 1" :to="c.path">{{ c.name }}</NuxtLink>
        <span v-else class="crumb-current">{{ c.name }}</span>
      </template>
    </nav>

    <div class="toolbar">
      <IconBtn name="download" label="Download" :disabled="!selected.length" @click="downloadSelected" />
      <IconBtn name="upload" label="Upload" :disabled="!canCreate" @click="fileInput?.click()" />
      <IconBtn name="trash" label="Delete" variant="danger" :disabled="!selected.length || !canDelete" @click="deleteSelected" />
      <IconBtn v-if="selectedOne && canShare" name="share" label="Share" @click="shareSelected" />
      <IconBtn v-if="selectedOne && canACL" name="lock" label="Permissions" @click="openACL" />
      <input ref="fileInput" type="file" multiple hidden @change="onPick" />
    </div>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="loading" class="muted">Loading…</p>

    <table class="file-table" v-if="!loading">
      <thead>
        <tr>
          <th><input type="checkbox" :checked="allSelected" @change="toggleAll" aria-label="Select all" /></th>
          <th class="col-type" aria-label="Type"></th>
          <th>
            <button type="button" class="th-sort" @click="sortBy('name')">
              Name <span class="sort-ind">{{ sortIndicator('name') }}</span>
            </button>
          </th>
          <th>
            <button type="button" class="th-sort" @click="sortBy('size')">
              Size <span class="sort-ind">{{ sortIndicator('size') }}</span>
            </button>
          </th>
          <th>
            <button type="button" class="th-sort" @click="sortBy('date')">
              Modified <span class="sort-ind">{{ sortIndicator('date') }}</span>
            </button>
          </th>
          <th>
            <button type="button" class="th-sort" @click="sortBy('description')">
              Description <span class="sort-ind">{{ sortIndicator('description') }}</span>
            </button>
          </th>
          <th>
            <button type="button" class="th-sort" @click="sortBy('owner')">
              Owner <span class="sort-ind">{{ sortIndicator('owner') }}</span>
            </button>
          </th>
          <th class="col-actions"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="parentPath != null" class="parent-row">
          <td></td>
          <td class="col-type">
            <AppIcon name="folder" class="type-icon" />
          </td>
          <td>
            <NuxtLink class="name" :to="parentPath">..</NuxtLink>
          </td>
          <td class="muted">—</td>
          <td class="muted">—</td>
          <td class="muted desc">—</td>
          <td class="muted">—</td>
          <td class="col-actions"></td>
        </tr>
        <tr
          v-for="f in sortedFiles"
          :key="f.path"
          :class="{ selected: selected.includes(f.path) }"
        >
          <td><input type="checkbox" :checked="selected.includes(f.path)" @change="toggle(f.path)" :aria-label="'Select ' + f.name" /></td>
          <td class="col-type">
            <AppIcon :name="f.is_dir ? 'folder' : 'file'" class="type-icon" />
          </td>
          <td>
            <NuxtLink v-if="f.is_dir" class="name" :to="linkFor(f)">{{ f.name }}</NuxtLink>
            <a
              v-else-if="mediaKind(f.name, f.content_type)"
              class="name"
              :href="streamUrl(f)"
              target="_blank"
              rel="noopener"
            >{{ f.name }}</a>
            <a v-else class="name" :href="downloadUrl(f)" target="_blank" rel="noopener">{{ f.name }}</a>
          </td>
          <td class="muted">{{ formatSize(f.size) }}</td>
          <td class="muted">{{ formatDate(f.date) }}</td>
          <td class="muted desc">{{ f.description || '—' }}</td>
          <td class="muted">{{ f.owner || '—' }}</td>
          <td class="col-actions">
            <div v-if="hasRowMenu(f)" class="row-menu" :class="{ open: menuPath === f.path }">
              <IconBtn
                name="more"
                :label="'Actions for ' + f.name"
                @click.stop="toggleMenu(f.path)"
              />
              <div v-if="menuPath === f.path" class="row-menu-panel" role="menu" @click.stop>
                <a
                  v-if="rowMenu(f).preview"
                  :href="streamUrl(f)"
                  target="_blank"
                  rel="noopener"
                  role="menuitem"
                  class="row-menu-link"
                  @click="menuPath = ''"
                >
                  <AppIcon name="external" /> Open
                </a>
                <button
                  v-if="rowMenu(f).download"
                  type="button"
                  role="menuitem"
                  @click="runMenu(() => downloadOne(f))"
                >
                  <AppIcon name="download" /> Download
                </button>
                <NuxtLink
                  v-if="rowMenu(f).open"
                  :to="linkFor(f)"
                  role="menuitem"
                  class="row-menu-link"
                  @click="menuPath = ''"
                >
                  <AppIcon name="external" /> Open
                </NuxtLink>
                <button
                  v-if="rowMenu(f).share"
                  type="button"
                  role="menuitem"
                  @click="runMenu(() => shareOne(f.path))"
                >
                  <AppIcon name="share" /> Share
                </button>
                <button
                  v-if="rowMenu(f).permissions"
                  type="button"
                  role="menuitem"
                  @click="runMenu(() => openACL(f.path))"
                >
                  <AppIcon name="lock" /> Permissions
                </button>
                <button
                  v-if="rowMenu(f).delete"
                  type="button"
                  role="menuitem"
                  class="danger"
                  @click="runMenu(() => deleteOne(f))"
                >
                  <AppIcon name="trash" /> Delete
                </button>
              </div>
            </div>
          </td>
        </tr>
      </tbody>
    </table>

    <div
      v-if="canCreate"
      class="dropzone"
      :class="{ active: drag }"
      role="button"
      tabindex="0"
      @click="fileInput?.click()"
      @keydown.enter.prevent="fileInput?.click()"
      @keydown.space.prevent="fileInput?.click()"
    >
      Drop files here or click to upload
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
          <IconBtn name="x" label="Cancel" @click="aclOpen = false" />
          <IconBtn name="check" label="Save" variant="primary" @click="saveACL" />
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
          <IconBtn name="copy" label="Copy" variant="primary" @click="copyShare" />
          <IconBtn name="x" label="Close" @click="shareUrl = ''" />
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import {
  fileDownloadUrl,
  fileStreamUrl,
  hasPerm,
  mediaKind,
  Perm,
  type FileEntry,
} from '~/composables/useApi'

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
const menuPath = ref('')
type SortKey = 'type' | 'name' | 'description' | 'owner' | 'size' | 'date'
const sortKey = ref<SortKey>('name')
const sortDir = ref<'asc' | 'desc'>('asc')

const rootLink = computed(() => (props.shareId ? `/s/${props.shareId}` : '/'))
const rootLabel = '/'

/** Parent directory path, or null when already at browse root (can't go up). */
const parentPath = computed(() => {
  const root = rootLink.value
  if (props.shareId) {
    const cur = (props.basePath || root).replace(/\/+$/, '') || root
    if (cur === root) return null
    const idx = cur.lastIndexOf('/')
    return idx <= 0 ? root : cur.slice(0, idx)
  }
  const cur = normalizePath(props.basePath || '/')
  if (cur === '/') return null
  const idx = cur.lastIndexOf('/')
  return idx <= 0 ? '/' : cur.slice(0, idx)
})

function compareEntries(a: FileEntry, b: FileEntry, key: SortKey, dir: number) {
  let cmp = 0
  switch (key) {
    case 'size':
      cmp = a.size - b.size
      break
    case 'date':
      cmp = Date.parse(a.date || '') - Date.parse(b.date || '')
      break
    case 'description':
      cmp = (a.description || '').localeCompare(b.description || '', undefined, { sensitivity: 'base' })
      break
    case 'owner':
      cmp = (a.owner || '').localeCompare(b.owner || '', undefined, { sensitivity: 'base' })
      break
    case 'type':
    case 'name':
    default:
      cmp = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
  }
  if (cmp === 0) cmp = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
  return cmp * dir
}

const sortedFiles = computed(() => {
  const dir = sortDir.value === 'asc' ? 1 : -1
  const key = sortKey.value
  const folders = files.value.filter(f => f.is_dir)
  const plain = files.value.filter(f => !f.is_dir)
  folders.sort((a, b) => compareEntries(a, b, key, dir))
  plain.sort((a, b) => compareEntries(a, b, key, dir))
  return [...folders, ...plain]
})

function sortBy(key: SortKey) {
  if (sortKey.value === key) sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  else {
    sortKey.value = key
    sortDir.value = key === 'date' || key === 'size' ? 'desc' : 'asc'
  }
}

function sortIndicator(key: SortKey) {
  if (sortKey.value !== key) return ''
  return sortDir.value === 'asc' ? '↑' : '↓'
}

const crumbs = computed(() => {
  let path = props.basePath || '/'
  let acc = ''
  if (props.shareId) {
    const prefix = `/s/${props.shareId}`
    path = path === prefix || path.startsWith(prefix + '/') ? path.slice(prefix.length) : path
    acc = prefix
  }
  const parts = path.split('/').filter(Boolean)
  const out: { name: string; path: string }[] = []
  for (const part of parts) {
    acc += '/' + part
    out.push({ name: part, path: acc })
  }
  return out
})

const selectedOne = computed(() => selected.value.length === 1 ? selected.value[0] : '')
const selectedEntries = computed(() => files.value.filter(f => selected.value.includes(f.path)))
const canCreate = computed(() =>
  files.value.some(f => hasPerm(f.permissions, Perm.Create)) || files.value.length === 0,
)
const canDelete = computed(() =>
  selectedEntries.value.length > 0 && selectedEntries.value.every(f => rowCan(f, Perm.Delete)),
)
const canShare = computed(() => {
  const f = files.value.find(x => x.path === selectedOne.value)
  return !!f && rowCan(f, Perm.Share)
})
const canACL = computed(() => {
  const f = files.value.find(x => x.path === selectedOne.value)
  return !!f && rowCan(f, Perm.UpdatePermissions)
})
const allSelected = computed(() => files.value.length > 0 && selected.value.length === files.value.length)
const absoluteShare = computed(() => (typeof window !== 'undefined' ? window.location.origin : '') + shareUrl.value)

function rowCan(f: FileEntry, bit: number) {
  // Share links are read-only in the UI even if listing bits look wider.
  if (props.shareId && bit !== Perm.Read) return false
  return hasPerm(f.permissions ?? 0, bit)
}

function rowMenu(f: FileEntry) {
  return {
    preview: !f.is_dir && !!mediaKind(f.name, f.content_type) && rowCan(f, Perm.Read),
    download: !f.is_dir && rowCan(f, Perm.Read),
    open: f.is_dir && rowCan(f, Perm.Read),
    share: rowCan(f, Perm.Share),
    permissions: rowCan(f, Perm.UpdatePermissions),
    delete: rowCan(f, Perm.Delete),
  }
}

function hasRowMenu(f: FileEntry) {
  const m = rowMenu(f)
  return m.preview || m.download || m.open || m.share || m.permissions || m.delete
}

function streamUrl(f: FileEntry) {
  if (props.shareId) {
    const base = (props.basePath || `/s/${props.shareId}`).replace(/\/$/, '')
    return `${base}/${encodeURIComponent(f.name).replace(/%2F/gi, '/')}`
  }
  return fileStreamUrl(f.path)
}

function downloadUrl(f: FileEntry) {
  if (props.shareId) {
    return `${streamUrl(f)}?dl=1`
  }
  return fileDownloadUrl(f.path)
}

function normalizePath(p: string) {
  if (!p || p === '/') return '/'
  return '/' + p.replace(/^\/+|\/+$/g, '')
}

async function load() {
  loading.value = true
  error.value = ''
  selected.value = []
  try {
    const res = await api.listPath(props.basePath || '/')
    files.value = res.files || []
  } catch (e: any) {
    const status = e?.statusCode || e?.status || e?.response?.status
    if (!props.shareId && status === 404 && normalizePath(props.basePath || '/') !== '/') {
      await navigateTo('/')
      return
    }
    error.value = e?.data?.message || e.message || 'Failed to load'
    files.value = []
  } finally {
    loading.value = false
  }
}

function linkFor(f: FileEntry) {
  if (props.shareId) {
    const prefix = `/s/${props.shareId}`
    if (f.path === prefix || f.path.startsWith(prefix + '/')) return f.path
    const base = (props.basePath || prefix).replace(/\/$/, '')
    return `${base}/${f.name}`
  }
  return f.path
}

function toggle(path: string) {
  if (selected.value.includes(path)) selected.value = selected.value.filter(p => p !== path)
  else selected.value = [...selected.value, path]
}

function toggleAll(e: Event) {
  const on = (e.target as HTMLInputElement).checked
  selected.value = on ? sortedFiles.value.map(f => f.path) : []
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

function toggleMenu(path: string) {
  menuPath.value = menuPath.value === path ? '' : path
}

function runMenu(fn: () => void | Promise<void>) {
  menuPath.value = ''
  return fn()
}

function onDocClick() {
  menuPath.value = ''
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') menuPath.value = ''
}

async function deleteSelected() {
  if (!confirm('Delete selected?')) return
  await api.remove(selected.value)
  await load()
}

async function deleteOne(f: FileEntry) {
  if (!confirm(`Delete ${f.name}?`)) return
  await api.remove([f.path])
  await load()
}

function downloadSelected() {
  for (const f of selectedEntries.value) {
    if (!f.is_dir) window.open(downloadUrl(f), '_blank')
  }
}

function downloadOne(f: FileEntry) {
  window.open(downloadUrl(f), '_blank')
}

async function shareSelected() {
  await shareOne(selectedOne.value)
}

async function shareOne(path: string) {
  const res = await api.createShare(path)
  shareUrl.value = res.url
}

async function copyShare() {
  await navigator.clipboard.writeText(absoluteShare.value)
}

async function openACL(path = selectedOne.value) {
  aclPath.value = path
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
  if (n < 1000) return n + ' B'
  const u = ['KB', 'MB', 'GB', 'TB']
  let i = -1
  do { n /= 1000; i++ } while (n >= 1000 && i < u.length - 1)
  return n.toFixed(1) + ' ' + u[i]
}

function formatDate(d: string) {
  try { return new Date(d).toLocaleString() } catch { return d }
}

watch(() => props.basePath, () => {
  menuPath.value = ''
  return load()
}, { immediate: true })

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
})
</script>
