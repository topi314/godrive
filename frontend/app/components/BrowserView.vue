<template>
  <div
    @dragover.prevent="drag = true"
    @dragleave.prevent="drag = false"
    @drop.prevent="onDrop"
  >
    <nav class="crumbs" aria-label="Breadcrumb">
      <NuxtLink :to="rootLink" class="crumb-root" @click.prevent="goTo(rootLink)">{{ rootLabel }}</NuxtLink>
      <template v-for="(c, i) in crumbs" :key="c.path">
        <!-- Root is already "/"; don't render a second slash before the first segment. -->
        <span v-if="i > 0 || rootLabel !== '/'" class="sep" aria-hidden="true">/</span>
        <NuxtLink v-if="i < crumbs.length - 1" :to="c.path" @click.prevent="goTo(c.path)">{{ c.name }}</NuxtLink>
        <span v-else class="crumb-current">{{ c.name }}</span>
      </template>
    </nav>

    <div class="toolbar">
      <label class="toolbar-select-all" title="Select all">
        <span class="check-slot">
          <input
            type="checkbox"
            :checked="allSelected"
            :disabled="!files.length"
            aria-label="Select all"
            @change="toggleAll"
          />
        </span>
      </label>
      <NuxtLink
        v-if="canGoHome"
        :to="homePath"
        class="icon-btn"
        title="Home"
        aria-label="Home"
        @click.prevent="goTo(homePath)"
      >
        <AppIcon name="home" />
      </NuxtLink>
      <label v-if="!shareId" class="toolbar-search">
        <AppIcon name="search" class="toolbar-search-icon" />
        <input
          v-model="searchQ"
          type="search"
          placeholder="Search this folder and subfolders…"
          autocomplete="off"
          spellcheck="false"
          @keydown.escape.prevent="clearSearch"
        >
        <button
          v-if="searchQ"
          type="button"
          class="toolbar-search-clear"
          title="Clear search"
          aria-label="Clear search"
          @click="clearSearch"
        >
          <AppIcon name="x" />
        </button>
      </label>
      <IconBtn class="toolbar-sort-btn" name="sort" label="Sort" @click="sortOpen = true" />
      <IconBtn name="refresh" label="Refresh" :disabled="loading" @click="load({ force: true })" />
      <template v-if="selected.length">
        <IconBtn name="download" label="Download" @click="downloadSelected" />
        <IconBtn name="move" label="Move" :disabled="!canMove || !!shareId" @click="openMove" />
      </template>
      <template v-else>
        <IconBtn name="upload" label="Upload" :disabled="!canCreate" @click="openUpload()" />
        <IconBtn name="folder-plus" label="New folder" :disabled="!canCreate || !!shareId || searching" @click="openNewFolder" />
      </template>
      <IconBtn name="trash" label="Delete" variant="danger" :disabled="!selected.length || !canDelete || !!shareId" @click="deleteSelected" />
      <input ref="fileInput" type="file" multiple hidden @change="onPick" />
    </div>

    <div v-if="error" class="load-error" role="alert">
      <AppIcon name="alert" class="load-error-icon" />
      <h2 class="load-error-title">{{ errorTitle }}</h2>
      <p class="load-error-detail">{{ error }}</p>
      <p v-if="errorHint" class="muted load-error-hint">{{ errorHint }}</p>
      <button type="button" class="btn-primary load-error-retry" :disabled="loading" @click="load({ force: true })">
        <AppIcon name="refresh" />
        {{ loading ? 'Retrying…' : 'Try again' }}
      </button>
    </div>

    <template v-else>
    <div class="file-table-wrap">
    <table class="file-table browser-files">
      <thead>
        <tr>
          <th class="col-check">
            <label class="row-select">
              <span class="check-slot">
                <input type="checkbox" :checked="allSelected" @change="toggleAll" aria-label="Select all" />
              </span>
            </label>
          </th>
          <th class="col-type" aria-label="Type"></th>
          <th class="col-name">
            <button type="button" class="th-sort" @click="sortBy('name')">
              Name <span class="sort-ind">{{ sortIndicator('name') }}</span>
            </button>
          </th>
          <th class="col-size">
            <button type="button" class="th-sort" @click="sortBy('size')">
              Size <span class="sort-ind">{{ sortIndicator('size') }}</span>
            </button>
          </th>
          <th class="col-date">
            <button type="button" class="th-sort" @click="sortBy('date')">
              Modified <span class="sort-ind">{{ sortIndicator('date') }}</span>
            </button>
          </th>
          <th class="col-desc">
            <button type="button" class="th-sort" @click="sortBy('description')">
              Description <span class="sort-ind">{{ sortIndicator('description') }}</span>
            </button>
          </th>
          <th class="col-owner">
            <button type="button" class="th-sort" @click="sortBy('owner')">
              Owner <span class="sort-ind">{{ sortIndicator('owner') }}</span>
            </button>
          </th>
          <th class="col-actions"></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-if="parentPath != null && !searching"
          class="parent-row"
          role="link"
          tabindex="0"
          aria-label="Go to parent folder"
          @click="goTo(parentPath)"
          @keydown.enter.prevent="goTo(parentPath)"
          @keydown.space.prevent="goTo(parentPath)"
        >
          <td class="col-check">
            <span class="row-select row-select-static">
              <AppIcon name="folder" class="type-icon row-select-icon" />
            </span>
          </td>
          <td class="col-type">
            <AppIcon name="folder" class="type-icon" />
          </td>
          <td class="col-name">
            <NuxtLink class="name" :to="parentPath" @click.stop.prevent="goTo(parentPath)">..</NuxtLink>
          </td>
          <td class="muted col-size">—</td>
          <td class="muted col-date">—</td>
          <td class="muted desc col-desc">—</td>
          <td class="muted col-owner">—</td>
          <td class="col-actions"></td>
        </tr>
        <tr
          v-for="f in sortedFiles"
          :key="f.path"
          class="file-row"
          :class="{ selected: selected.includes(f.path) }"
          @click="onFileRowClick($event, f)"
        >
          <td class="col-check">
            <label class="row-select">
              <span class="check-slot">
                <input
                  type="checkbox"
                  :checked="selected.includes(f.path)"
                  @change="toggle(f.path)"
                  :aria-label="'Select ' + displayName(f)"
                />
              </span>
              <AppIcon :name="f.is_dir ? 'folder' : 'file'" class="type-icon row-select-icon" />
            </label>
          </td>
          <td class="col-type">
            <AppIcon :name="f.is_dir ? 'folder' : 'file'" class="type-icon" />
          </td>
          <td class="col-name">
            <span class="name-cell">
              <button
                v-if="searching && resultParentRel(f)"
                type="button"
                class="result-dir"
                :title="'Open ' + containingFolder(f)"
                @click="openContainingFolder(f)"
              >{{ resultParentRel(f) }}/</button>
              <NuxtLink v-if="f.is_dir" class="name" :to="linkFor(f)" @click.prevent="goTo(linkFor(f))">{{ displayName(f) }}</NuxtLink>
              <a
                v-else
                class="name"
                :href="streamUrl(f)"
                target="_blank"
                rel="noopener"
              >{{ displayName(f) }}</a>
              <span class="mobile-meta muted">
                <template v-if="!f.is_dir">{{ formatSize(f.size) }} · </template>{{ formatDate(f.date) }}<template v-if="f.owner"> · {{ f.owner }}</template>
              </span>
              <span v-if="f.description" class="mobile-meta mobile-desc muted">{{ f.description }}</span>
            </span>
          </td>
          <td class="muted col-size">{{ formatSize(f.size) }}</td>
          <td class="muted col-date">{{ formatDate(f.date) }}</td>
          <td class="muted desc col-desc">{{ f.description || '—' }}</td>
          <td class="muted col-owner">{{ f.owner || '—' }}</td>
          <td class="col-actions">
            <div class="row-actions">
              <IconBtn
                v-if="searching && containingFolder(f)"
                class="search-open-folder"
                name="folder-open"
                label="Open containing folder"
                @click="openContainingFolder(f)"
              />
            <div v-if="hasRowMenu(f)" class="row-menu" :class="{ open: menuPath === f.path }">
              <IconBtn
                name="more"
                :label="'Actions for ' + displayName(f)"
                @click.stop="toggleMenu(f.path, $event)"
              />
              <Teleport to="body">
                <div
                  v-if="menuPath === f.path"
                  class="row-menu-panel"
                  role="menu"
                  :style="menuStyle"
                  @click.stop
                >
                  <button
                    v-if="searching && containingFolder(f)"
                    type="button"
                    role="menuitem"
                    @click="runMenu(() => openContainingFolder(f))"
                  >
                    <AppIcon name="folder-open" /> Open folder
                  </button>
                  <button
                    v-if="rowMenu(f).download"
                    type="button"
                    role="menuitem"
                    @click="runMenu(() => downloadOne(f))"
                  >
                    <AppIcon name="download" /> Download
                  </button>
                  <button
                    v-if="rowMenu(f).rename"
                    type="button"
                    role="menuitem"
                    @click="runMenu(() => openRename(f))"
                  >
                    <AppIcon name="edit" /> Rename
                  </button>
                  <button
                    v-if="rowMenu(f).share"
                    type="button"
                    role="menuitem"
                    @click="runMenu(() => openShare(f.path))"
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
              </Teleport>
            </div>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
    </div>

    <div
      class="dropzone"
      :class="{ active: drag && canCreate, disabled: !canCreate }"
      role="button"
      :tabindex="canCreate ? 0 : -1"
      :aria-disabled="!canCreate"
      @click="canCreate && openUpload()"
      @keydown.enter.prevent="canCreate && openUpload()"
      @keydown.space.prevent="canCreate && openUpload()"
    >
      Drop files here or click to upload
    </div>
    </template>

    <PermissionsDialog v-model="aclOpen" v-model:path="aclPath" @saved="onACLSaved" />
    <UploadDialog
      v-model="uploadOpen"
      :base-dir="uploadBaseDir"
      :share-id="shareId"
      :initial-files="uploadInitialFiles"
      @done="onUploadDone"
    />

    <div v-if="folderOpen" class="dialog-backdrop" @click.self="folderOpen = false" @keydown.escape.prevent="folderOpen = false">
      <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="folder-title">
        <div class="dialog-head">
          <h2 id="folder-title">New folder</h2>
          <IconBtn name="x" label="Close" variant="ghost" @click="folderOpen = false" />
        </div>
        <label>
          Name
          <input
            ref="folderInput"
            v-model="folderName"
            placeholder="folder"
            @keydown.enter.prevent="createFolder"
            @keydown.escape.prevent="folderOpen = false"
          >
        </label>
        <p v-if="folderError" class="error">{{ folderError }}</p>
        <div class="dialog-actions">
          <IconBtn name="check" label="Create" variant="primary" :disabled="!folderName.trim()" @click="createFolder" />
        </div>
      </div>
    </div>

    <div v-if="renameOpen" class="dialog-backdrop" @click.self="renameOpen = false" @keydown.escape.prevent="renameOpen = false">
      <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="rename-title">
        <div class="dialog-head">
          <h2 id="rename-title">Rename</h2>
          <IconBtn name="x" label="Close" variant="ghost" @click="renameOpen = false" />
        </div>
        <label>
          Name or path
          <input
            ref="renameInput"
            v-model="renameName"
            placeholder="name or ../folder/name"
            @keydown.enter.prevent="submitRename"
            @keydown.escape.prevent="renameOpen = false"
          >
        </label>
        <p class="muted">Use <code>../</code> or a folder path to move. Missing folders are created.</p>
        <p v-if="renameError" class="error">{{ renameError }}</p>
        <div class="dialog-actions">
          <IconBtn name="check" label="Rename" variant="primary" :disabled="!renameName.trim()" @click="submitRename" />
        </div>
      </div>
    </div>

    <div v-if="sortOpen" class="dialog-backdrop" @click.self="sortOpen = false" @keydown.escape.prevent="sortOpen = false">
      <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="sort-title">
        <div class="dialog-head">
          <h2 id="sort-title">Sort</h2>
          <IconBtn name="x" label="Close" variant="ghost" @click="sortOpen = false" />
        </div>
        <p class="sort-section-label">Sort by</p>
        <div class="sort-options" role="listbox" aria-label="Sort field">
          <button
            v-for="opt in sortFieldOptions"
            :key="opt.key"
            type="button"
            class="sort-option"
            :class="{ active: sortKey === opt.key }"
            role="option"
            :aria-selected="sortKey === opt.key"
            @click="setSortKey(opt.key)"
          >{{ opt.label }}</button>
        </div>
        <p class="sort-section-label">Order</p>
        <div class="sort-options" role="listbox" aria-label="Sort order">
          <button
            type="button"
            class="sort-option"
            :class="{ active: sortDir === 'asc' }"
            role="option"
            :aria-selected="sortDir === 'asc'"
            @click="sortDir = 'asc'"
          >Ascending</button>
          <button
            type="button"
            class="sort-option"
            :class="{ active: sortDir === 'desc' }"
            role="option"
            :aria-selected="sortDir === 'desc'"
            @click="sortDir = 'desc'"
          >Descending</button>
        </div>
        <div class="dialog-actions">
          <IconBtn name="check" label="Done" variant="primary" @click="sortOpen = false" />
        </div>
      </div>
    </div>

    <div v-if="moveOpen" class="dialog-backdrop" @click.self="moveOpen = false" @keydown.escape.prevent="moveOpen = false">
      <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="move-title">
        <div class="dialog-head">
          <h2 id="move-title">Move {{ selected.length }} item{{ selected.length === 1 ? '' : 's' }}</h2>
          <IconBtn name="x" label="Close" variant="ghost" @click="moveOpen = false" />
        </div>
        <label>
          Destination folder
          <input
            ref="moveInput"
            v-model="moveDest"
            placeholder="/path/to/folder or ../folder"
            @keydown.enter.prevent="submitMove"
            @keydown.escape.prevent="moveOpen = false"
          >
        </label>
        <div v-if="moveFolderPicks.length" class="move-picks">
          <button
            v-for="f in moveFolderPicks"
            :key="f.path"
            type="button"
            class="move-pick"
            @click="moveDest = f.path"
          >{{ f.name }}</button>
        </div>
        <p class="muted">Absolute path, or relative to the current folder. Nested folders are created if needed.</p>
        <p v-if="moveError" class="error">{{ moveError }}</p>
        <div class="dialog-actions">
          <IconBtn name="check" label="Move" variant="primary" :disabled="!moveDest.trim()" @click="submitMove" />
        </div>
      </div>
    </div>

    <div v-if="shareOpen" class="dialog-backdrop" @click.self="closeShare" @keydown.escape.prevent="closeShare">
      <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="share-title">
        <div class="dialog-head">
          <h2 id="share-title">{{ shareUrl ? 'Share link' : 'Share' }}</h2>
          <IconBtn name="x" label="Close" variant="ghost" @click="closeShare" />
        </div>
        <template v-if="!shareUrl">
          <label>
            Expires
            <select v-model="shareExpiry">
              <option value="">Never</option>
              <option value="1h">1 hour</option>
              <option value="6h">6 hours</option>
              <option value="24h">1 day</option>
              <option value="168h">7 days</option>
              <option value="720h">30 days</option>
              <option value="custom">Custom</option>
            </select>
          </label>
          <div v-if="shareExpiry === 'custom'" class="expiry-custom">
            <label>
              After
              <input v-model.number="shareCustomAmount" type="number" min="1" step="1">
            </label>
            <label>
              Unit
              <select v-model="shareCustomUnit">
                <option value="h">hours</option>
                <option value="d">days</option>
              </select>
            </label>
          </div>
          <div class="share-perms">
            <span class="share-perms-label">Link permissions</span>
            <table class="file-table acl-table share-acl-table">
              <thead>
                <tr>
                  <th
                    v-for="b in PERM_FLAGS"
                    :key="'h-' + b.bit"
                    class="perm-col"
                    :class="'perm-' + b.short.toLowerCase()"
                  >{{ b.short }}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td v-for="b in PERM_FLAGS" :key="'c-' + b.bit" class="perm-cell">
                    <button
                      type="button"
                      class="perm-mark"
                      :class="markClass(shareAllow, shareDeny, b.bit)"
                      :title="b.label + ' (click to cycle allow / deny / none)'"
                      @click="cycleSharePerm(b.bit)"
                    >{{ markLabel(shareAllow, shareDeny, b.bit) }}</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="shareError" class="error">{{ shareError }}</p>
          <div class="dialog-actions">
            <IconBtn name="check" label="Create link" variant="primary" @click="createShareLink" />
          </div>
          <PermLegend class="perm-legend-footer" marks />
        </template>
        <template v-else>
          <label>
            URL
            <input :value="absoluteShare" readonly @focus="($event.target as HTMLInputElement).select()" />
          </label>
          <p v-if="shareExpiresLabel" class="muted">Expires {{ shareExpiresLabel }}</p>
          <div class="dialog-actions">
            <IconBtn name="copy" label="Copy" variant="primary" @click="copyShare" />
          </div>
        </template>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { formatDate, formatSize, normalizeClientPath } from '~/composables/format'
import { PERM_FLAGS, cycleAllowDeny, markClass, markLabel } from '~/composables/permBits'
import {
  fileDownloadUrl,
  fileStreamUrl,
  fileZipDownloadUrl,
  hasPerm,
  Perm,
  type FileEntry,
  type Me,
} from '~/composables/useApi'


const props = defineProps<{ basePath: string; shareId?: string }>()
const route = useRoute()
const api = useApi()
const { toast } = useToast()
const { confirm } = useConfirm()
const user = useState<Me | null>('me')
const files = ref<FileEntry[]>([])
const dirPerms = ref(0)
const selected = ref<string[]>([])
const error = ref('')
const errorStatus = ref(0)
const loading = ref(false)
/** Path currently shown in crumbs + list (updated together when a listing arrives). */
const listedPath = ref(props.basePath || '/')
/** Normalized path whose listing is already on screen (avoids reload after goTo → navigate). */
let viewPath = ''
const searchQ = ref('')
const searching = computed(() => !props.shareId && searchQ.value.trim().length > 0)
let loadSeq = 0
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSeq = 0
let suppressSearchWatch = false

function routeSearchQ() {
  if (props.shareId) return ''
  const q = route.query.q
  if (Array.isArray(q)) return String(q[0] || '').trim()
  return typeof q === 'string' ? q.trim() : ''
}

async function syncSearchToRoute(q: string) {
  if (props.shareId) return
  const term = q.trim()
  if (term === routeSearchQ()) return
  const query = { ...route.query } as Record<string, string | string[] | undefined>
  if (term) query.q = term
  else delete query.q
  await navigateTo({ path: route.path, query }, { replace: true })
}

const errorTitle = computed(() => {
  const s = errorStatus.value
  if (s === 502 || s === 503 || s === 504) return 'Server unavailable'
  if (s === 0 && /fetch|network|failed to fetch|ECONNREFUSED|502|Bad Gateway/i.test(error.value)) {
    return 'Server unavailable'
  }
  if (s === 403) return 'Access denied'
  if (s === 404) return 'Not found'
  if (s >= 500) return 'Something went wrong'
  return 'Couldn’t load this folder'
})

const errorHint = computed(() => {
  const s = errorStatus.value
  if (s === 502 || s === 503 || s === 504 || (s === 0 && /502|Bad Gateway|ECONNREFUSED|fetch/i.test(error.value))) {
    return 'The API isn’t reachable right now. Check that godrive is running, then try again.'
  }
  if (s === 403) return 'You don’t have permission to view this path.'
  if (s >= 500) return 'The server hit an error while loading this folder.'
  return ''
})

function loadErrorMessage(e: any, status: number): string {
  const raw = String(e?.data?.message || e?.statusMessage || e?.message || 'Failed to load')
  if (status === 502 || /502\s*Bad Gateway/i.test(raw)) {
    return 'Bad gateway — the frontend couldn’t reach the API.'
  }
  if (status === 503 || /503/i.test(raw)) return 'Service temporarily unavailable.'
  if (status === 504 || /504/i.test(raw)) return 'The API timed out.'
  // ofetch often formats as [GET] "/": 502 Bad Gateway
  const m = raw.match(/^\[[A-Z]+\]\s+"[^"]*":\s*(.+)$/)
  return m?.[1] || raw
}
const listingInflight = new Map<string, Promise<{ files: FileEntry[]; permissions: number }>>()

function listingKey(path: string) {
  return (props.shareId || '') + '\0' + path
}

function fetchListing(path: string) {
  const key = listingKey(path)
  let pending = listingInflight.get(key)
  if (!pending) {
    pending = api.listPath(path).then(res => ({
      files: res.files || [],
      permissions: res.permissions ?? 0,
    })).finally(() => {
      listingInflight.delete(key)
    })
    listingInflight.set(key, pending)
  }
  return pending
}
const drag = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const aclOpen = ref(false)
const aclPath = ref('/')
const shareOpen = ref(false)
const sharePath = ref('')
const shareUrl = ref('')
const shareError = ref('')
const shareExpiry = ref('')
const shareCustomAmount = ref(24)
const shareCustomUnit = ref<'h' | 'd'>('h')
const shareExpiresAt = ref<string | null>(null)
const shareAllow = ref(Perm.Read)
const shareDeny = ref(0)
function cycleSharePerm(bit: number) {
  const next = cycleAllowDeny(shareAllow.value, shareDeny.value, bit)
  shareAllow.value = next.allow
  shareDeny.value = next.deny
}
const uploadOpen = ref(false)
const uploadInitialFiles = ref<File[]>([])
const uploadBaseDir = computed(() => listedPath.value || props.basePath || '/')
const folderOpen = ref(false)
const folderName = ref('')
const folderError = ref('')
const folderInput = ref<HTMLInputElement | null>(null)
const renameOpen = ref(false)
const renameName = ref('')
const renameError = ref('')
const renamePath = ref('')
const renameInput = ref<HTMLInputElement | null>(null)
const moveOpen = ref(false)
const moveDest = ref('')
const moveError = ref('')
const moveInput = ref<HTMLInputElement | null>(null)
const sortOpen = ref(false)
const menuPath = ref('')
const menuStyle = ref<Record<string, string>>({})
type SortKey = 'type' | 'name' | 'description' | 'owner' | 'size' | 'date'
const sortKey = ref<SortKey>('name')
const sortDir = ref<'asc' | 'desc'>('asc')
const sortFieldOptions: { key: SortKey; label: string }[] = [
  { key: 'name', label: 'Name' },
  { key: 'size', label: 'Size' },
  { key: 'date', label: 'Modified' },
  { key: 'description', label: 'Description' },
  { key: 'owner', label: 'Owner' },
]

const rootLink = computed(() => (props.shareId ? `/s/${props.shareId}` : '/'))
const rootLabel = '/'

/** Parent directory path, or null when already at browse root (can't go up). */
const parentPath = computed(() => {
  const root = rootLink.value
  if (props.shareId) {
    const cur = (listedPath.value || root).replace(/\/+$/, '') || root
    if (cur === root) return null
    const idx = cur.lastIndexOf('/')
    return idx <= 0 ? root : cur.slice(0, idx)
  }
  const cur = normalizePath(listedPath.value || '/')
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

function setSortKey(key: SortKey) {
  sortKey.value = key
}

function sortIndicator(key: SortKey) {
  if (sortKey.value !== key) return ''
  return sortDir.value === 'asc' ? '↑' : '↓'
}

const crumbs = computed(() => {
  let path = listedPath.value || '/'
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

const selectedEntries = computed(() => files.value.filter(f => selected.value.includes(f.path)))
const canCreate = computed(() => hasPerm(dirPerms.value, Perm.Create))
const canGoHome = computed(() =>
  !props.shareId && (!!user.value?.authenticated || !!user.value?.guests_allowed),
)
const homePath = computed(() => {
  if (user.value?.is_guest || !user.value?.authenticated) return '/'
  const h = (user.value?.home || '/').trim() || '/'
  if (h === '/') return '/'
  return h.startsWith('/') ? h.replace(/\/+$/, '') : '/' + h.replace(/\/+$/, '')
})
const canDelete = computed(() =>
  selectedEntries.value.length > 0 && selectedEntries.value.every(f => rowCan(f, Perm.Delete)),
)
/** Move needs Delete on each source (Create on dest is checked by the API). */
const canMove = computed(() => canDelete.value)
const moveFolderPicks = computed(() => {
  const selectedSet = new Set(selected.value)
  return files.value.filter(f => f.is_dir && !selectedSet.has(f.path))
})
const allSelected = computed(() => files.value.length > 0 && selected.value.length === files.value.length)
const absoluteShare = computed(() => (typeof window !== 'undefined' ? window.location.origin : '') + shareUrl.value)
const shareExpiresLabel = computed(() => {
  if (!shareExpiresAt.value) return ''
  try { return new Date(shareExpiresAt.value).toLocaleString() } catch { return shareExpiresAt.value }
})

function rowCan(f: FileEntry, bit: number) {
  return hasPerm(f.permissions ?? 0, bit)
}

function rowMenu(f: FileEntry) {
  return {
    download: rowCan(f, Perm.Read),
    rename: !props.shareId && (rowCan(f, Perm.Update) || rowCan(f, Perm.Delete)),
    share: !props.shareId && rowCan(f, Perm.Share),
    permissions: !props.shareId && rowCan(f, Perm.UpdatePermissions),
    delete: !props.shareId && rowCan(f, Perm.Delete),
  }
}

function hasRowMenu(f: FileEntry) {
  const m = rowMenu(f)
  return m.download || m.rename || m.share || m.permissions || m.delete
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

const normalizePath = normalizeClientPath

function applyListing(path: string, res: { files: FileEntry[]; permissions: number }) {
  files.value = res.files
  dirPerms.value = res.permissions
  listedPath.value = path
  viewPath = normalizePath(path)
  error.value = ''
  errorStatus.value = 0
}

async function load(opts?: { force?: boolean }) {
  const path = props.basePath || '/'
  const q = routeSearchQ()
  // goTo applies the listing before navigateTo; skip the duplicate fetch.
  if (!opts?.force && !q && normalizePath(path) === viewPath) {
    suppressSearchWatch = true
    searchQ.value = ''
    suppressSearchWatch = false
    return
  }
  const seq = ++loadSeq
  error.value = ''
  errorStatus.value = 0
  loading.value = true
  selected.value = []
  closeMenu()
  suppressSearchWatch = true
  searchQ.value = q
  suppressSearchWatch = false
  if (q) {
    await runSearch(q)
    if (seq === loadSeq) {
      // Keep crumbs on the search root together with the result list.
      listedPath.value = path
      viewPath = normalizePath(path)
      loading.value = false
    }
    return
  }
  try {
    const res = await fetchListing(path)
    if (seq !== loadSeq) return
    applyListing(path, res)
  } catch (e: any) {
    if (seq !== loadSeq) return
    const status = Number(e?.statusCode || e?.status || e?.response?.status || 0)
    if (!props.shareId && status === 404 && normalizePath(path) !== '/') {
      viewPath = ''
      await navigateTo('/')
      return
    }
    if (status !== 401) {
      errorStatus.value = status
      error.value = loadErrorMessage(e, status)
    }
    files.value = []
    dirPerms.value = 0
    listedPath.value = path
    viewPath = normalizePath(path)
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

async function runSearch(q: string) {
  const term = q.trim()
  // Always scope to the folder currently being browsed (and its descendants).
  const path = props.basePath || '/'
  const seq = ++searchSeq
  if (!term) {
    const res = await fetchListing(path)
    if (seq !== searchSeq) return
    files.value = res.files
    dirPerms.value = res.permissions
    selected.value = []
    return
  }
  loading.value = true
  error.value = ''
  errorStatus.value = 0
  try {
    const res = await api.queryPath(path, term)
    if (seq !== searchSeq) return
    files.value = res.files || []
    dirPerms.value = res.permissions ?? dirPerms.value
    selected.value = []
  } catch (e: any) {
    if (seq !== searchSeq) return
    const status = Number(e?.statusCode || e?.status || e?.response?.status || 0)
    if (status !== 401) {
      errorStatus.value = status
      error.value = e?.data?.message || e.message || 'Search failed'
    }
    files.value = []
  } finally {
    if (seq === searchSeq) loading.value = false
  }
}

function clearSearch() {
  searchQ.value = ''
}

watch(searchQ, (q) => {
  if (props.shareId || suppressSearchWatch) return
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    void syncSearchToRoute(q)
    void runSearch(q)
  }, 250)
})

watch(() => route.query.q, () => {
  if (props.shareId || suppressSearchWatch) return
  const term = routeSearchQ()
  if (term === searchQ.value.trim()) return
  suppressSearchWatch = true
  searchQ.value = term
  suppressSearchWatch = false
  void runSearch(term)
})

async function goTo(path: string | null | undefined) {
  if (!path) return
  const cur = props.basePath || '/'
  const next = normalizePath(path)
  if (next === normalizePath(cur) && !routeSearchQ()) return
  const seq = ++loadSeq
  suppressSearchWatch = true
  searchQ.value = ''
  suppressSearchWatch = false
  selected.value = []
  closeMenu()
  // Apply listing first so URL, crumbs, and file list advance together.
  try {
    const res = await fetchListing(next)
    if (seq !== loadSeq) return
    applyListing(next, res)
    await navigateTo({ path: next, query: {} })
  } catch (e: any) {
    if (seq !== loadSeq) return
    const status = Number(e?.statusCode || e?.status || e?.response?.status || 0)
    if (!props.shareId && status === 404 && next !== '/') {
      viewPath = ''
      await navigateTo('/')
      return
    }
    if (status !== 401) {
      toast(loadErrorMessage(e, status))
    }
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

function containingFolder(f: FileEntry) {
  const p = normalizePath(f.path)
  if (p === '/') return ''
  const idx = p.lastIndexOf('/')
  return idx <= 0 ? '/' : p.slice(0, idx)
}

/** Parent path relative to the search root (empty if the hit is directly in that folder). */
function resultParentRel(f: FileEntry) {
  const parent = containingFolder(f)
  if (!parent) return ''
  const root = normalizePath(props.basePath || '/')
  if (parent === root) return ''
  if (root === '/') return parent.replace(/^\//, '')
  if (parent.startsWith(root + '/')) return parent.slice(root.length + 1)
  return parent.replace(/^\//, '')
}

function displayName(f: FileEntry) {
  if (!searching.value) return f.name
  return f.path.split('/').filter(Boolean).at(-1) || f.name
}

function openContainingFolder(f: FileEntry) {
  const dir = containingFolder(f)
  if (!dir) return
  void goTo(dir)
}

function openEntry(f: FileEntry) {
  if (f.is_dir) {
    void goTo(linkFor(f))
    return
  }
  // Prefer a real <a> navigation over window.open(..., 'noopener'), which can
  // leave a blank tab in Firefox when the third-arg features string is set.
  const a = document.createElement('a')
  a.href = streamUrl(f)
  a.target = '_blank'
  a.rel = 'noopener'
  a.click()
}

function onFileRowClick(e: MouseEvent, f: FileEntry) {
  const t = e.target as HTMLElement | null
  if (t?.closest('a, button, input, label, .row-actions, .row-menu')) return
  openEntry(f)
}

function toggle(path: string) {
  if (selected.value.includes(path)) selected.value = selected.value.filter(p => p !== path)
  else selected.value = [...selected.value, path]
}

function toggleAll(e: Event) {
  const on = (e.target as HTMLInputElement).checked
  selected.value = on ? sortedFiles.value.map(f => f.path) : []
}

function openUpload(files?: File[]) {
  if (!canCreate.value) return
  if (files?.length) {
    uploadInitialFiles.value = [...files]
    uploadOpen.value = true
    return
  }
  // Open the OS picker first; the dialog opens once files are chosen.
  fileInput.value?.click()
}

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files?.length) return
  openUpload([...input.files])
  input.value = ''
}

function onDrop(e: DragEvent) {
  drag.value = false
  if (!canCreate.value) return
  const list = e.dataTransfer?.files
  if (!list?.length) return
  openUpload([...list])
}

async function onUploadDone() {
  await load({ force: true })
}

watch(uploadOpen, (v) => {
  if (!v) uploadInitialFiles.value = []
})

function closeMenu() {
  menuPath.value = ''
  menuStyle.value = {}
}

function isMobileMenu() {
  return typeof window !== 'undefined' && window.matchMedia('(max-width: 720px)').matches
}

function positionMenu(anchor: HTMLElement, panelHeight = 0) {
  if (isMobileMenu()) {
    // Bottom sheet layout comes from CSS.
    menuStyle.value = {}
    return
  }
  const gap = 4
  const r = anchor.getBoundingClientRect()
  const h = panelHeight || 240
  let top = r.bottom + gap
  if (top + h > window.innerHeight - 8) {
    top = Math.max(8, r.top - gap - h)
  }
  menuStyle.value = {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    right: `${Math.round(window.innerWidth - r.right)}px`,
    left: 'auto',
    bottom: 'auto',
  }
}

async function toggleMenu(path: string, e: MouseEvent) {
  if (menuPath.value === path) {
    closeMenu()
    return
  }
  const btn = e.currentTarget as HTMLElement
  positionMenu(btn)
  menuPath.value = path
  await nextTick()
  const panel = document.querySelector('.row-menu-panel') as HTMLElement | null
  if (panel && btn.isConnected) positionMenu(btn, panel.offsetHeight)
}

function runMenu(fn: () => void | Promise<void>) {
  closeMenu()
  return fn()
}

function onDocClick() {
  closeMenu()
}

function onViewportChange() {
  if (!menuPath.value) return
  // Keep the panel from drifting / expanding scroll containers while open.
  closeMenu()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  // Nested dialogs (upload/permissions/settings/admin) handle Escape themselves.
  if (shareOpen.value) {
    closeShare()
    e.preventDefault()
    return
  }
  if (sortOpen.value) {
    sortOpen.value = false
    e.preventDefault()
    return
  }
  if (moveOpen.value) {
    moveOpen.value = false
    e.preventDefault()
    return
  }
  if (renameOpen.value) {
    renameOpen.value = false
    e.preventDefault()
    return
  }
  if (folderOpen.value) {
    folderOpen.value = false
    e.preventDefault()
    return
  }
  if (uploadOpen.value || aclOpen.value) return
  closeMenu()
}

function dropPaths(paths: string[]) {
  const drop = new Set(paths)
  files.value = files.value.filter(f => !drop.has(f.path))
  selected.value = selected.value.filter(p => !drop.has(p))
}

async function deleteSelected() {
  const n = selected.value.length
  const ok = await confirm({
    title: 'Delete',
    message: n === 1 ? 'Delete this item?' : `Delete ${n} selected items?`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  const paths = [...selected.value]
  await api.remove(paths)
  dropPaths(paths)
}

async function deleteOne(f: FileEntry) {
  const ok = await confirm({
    title: 'Delete',
    message: `Delete ${f.name}?`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  await api.remove([f.path])
  dropPaths([f.path])
}

function downloadSelected() {
  const entries = selectedEntries.value
  if (!entries.length) return
  if (entries.length === 1) {
    window.open(downloadUrl(entries[0]), '_blank')
    return
  }
  const names = entries.map(f => f.name)
  if (props.shareId) {
    const base = (listedPath.value || props.basePath || `/s/${props.shareId}`).replace(/\/$/, '') || `/s/${props.shareId}`
    const params = new URLSearchParams()
    params.set('dl', '1')
    for (const name of names) params.append('name', name)
    window.open(`${base}?${params.toString()}`, '_blank')
    return
  }
  window.open(fileZipDownloadUrl(listedPath.value || '/', names), '_blank')
}

function downloadOne(f: FileEntry) {
  window.open(downloadUrl(f), '_blank')
}

function openShare(path: string) {
  sharePath.value = path
  shareUrl.value = ''
  shareError.value = ''
  shareExpiry.value = ''
  shareCustomAmount.value = 24
  shareCustomUnit.value = 'h'
  shareExpiresAt.value = null
  shareAllow.value = Perm.Read
  shareDeny.value = 0
  shareOpen.value = true
}

function closeShare() {
  shareOpen.value = false
  shareUrl.value = ''
  shareError.value = ''
}

function shareExpiresIn() {
  if (!shareExpiry.value) return undefined
  if (shareExpiry.value !== 'custom') return shareExpiry.value
  const n = Number(shareCustomAmount.value)
  if (!Number.isFinite(n) || n < 1) return ''
  return Math.floor(n) + shareCustomUnit.value
}

async function createShareLink() {
  const expiresIn = shareExpiresIn()
  if (expiresIn === '') {
    shareError.value = 'Enter a valid duration'
    return
  }
  shareError.value = ''
  try {
    const allow = shareAllow.value
    if (!allow) {
      shareError.value = 'Select at least one permission'
      return
    }
    const res = await api.createShare(sharePath.value, {
      ...(expiresIn ? { expires_in: expiresIn } : {}),
      allow,
      deny: shareDeny.value,
    })
    shareUrl.value = res.url
    shareExpiresAt.value = res.expires_at || null
  } catch (e: any) {
    shareError.value = e?.data?.message || e.message || 'Failed to create share'
  }
}

async function copyShare() {
  await navigator.clipboard.writeText(absoluteShare.value)
}

async function openACL(path: string) {
  closeMenu()
  aclPath.value = path
  aclOpen.value = true
}

async function onACLSaved() {
  await load({ force: true })
  toast('Permissions saved')
}

async function openNewFolder() {
  if (!canCreate.value) return
  folderName.value = ''
  folderError.value = ''
  folderOpen.value = true
  await nextTick()
  folderInput.value?.focus()
}

async function createFolder() {
  const name = folderName.value.trim()
  if (!name || name === '.' || name === '..' || /[\\/]/.test(name)) {
    folderError.value = 'Invalid folder name'
    return
  }
  folderError.value = ''
  try {
    const created = await api.mkdir(listedPath.value || '/', name)
    folderOpen.value = false
    toast('Folder created')
    const path = created.path
    if (!files.value.some(f => f.path === path)) {
      files.value = [
        {
          path,
          name: path.split('/').filter(Boolean).at(-1) || name,
          is_dir: true,
          size: 0,
          date: new Date().toISOString(),
          permissions: dirPerms.value,
        },
        ...files.value,
      ]
    }
  } catch (e: any) {
    folderError.value = e?.data?.message || e.message || 'Failed to create folder'
  }
}

async function openRename(f: FileEntry) {
  if (!rowCan(f, Perm.Update)) return
  renamePath.value = f.path
  renameName.value = f.name
  renameError.value = ''
  renameOpen.value = true
  await nextTick()
  renameInput.value?.focus()
  renameInput.value?.select()
}

function resolveMoveDest(spec: string, cwd: string) {
  const raw = spec.replaceAll('\\', '/').trim()
  if (!raw) return ''
  const joined = raw.startsWith('/')
    ? raw
    : (cwd === '/' ? '' : cwd) + '/' + raw
  const parts: string[] = []
  for (const part of joined.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') {
      parts.pop()
      continue
    }
    parts.push(part)
  }
  return parts.length ? '/' + parts.join('/') : '/'
}

function invalidMoveDest(dest: string, sources: string[]) {
  if (!dest) return 'Destination required'
  for (const src of sources) {
    if (dest === src || dest.startsWith(src + '/')) {
      return 'Cannot move into a selected folder'
    }
  }
  return ''
}

async function openMove() {
  if (!canMove.value || props.shareId) return
  moveDest.value = ''
  moveError.value = ''
  moveOpen.value = true
  await nextTick()
  moveInput.value?.focus()
}

async function submitMove() {
  const paths = [...selected.value]
  if (!paths.length) return
  const dest = resolveMoveDest(moveDest.value, listedPath.value || '/')
  const bad = invalidMoveDest(dest, paths)
  if (bad) {
    moveError.value = bad
    return
  }
  const parent = listedPath.value || '/'
  if (dest === parent) {
    moveError.value = 'Already in this folder'
    return
  }
  moveError.value = ''
  try {
    await api.move(paths, dest)
    moveOpen.value = false
    toast(paths.length === 1 ? 'Moved' : `Moved ${paths.length} items`)
    dropPaths(paths)
  } catch (e: any) {
    moveError.value = e?.data?.message || e.message || 'Failed to move'
  }
}

async function submitRename() {
  const spec = renameName.value.trim()
  if (!validRenameSpec(spec)) {
    renameError.value = 'Invalid name'
    return
  }
  renameError.value = ''
  try {
    const from = renamePath.value
    await api.rename(from, spec)
    renameOpen.value = false
    toast('Renamed')
    const parent = from === '/' ? '/' : from.replace(/\/[^/]+$/, '') || '/'
    const base = spec.replaceAll('\\', '/').replace(/\/+$/, '').split('/').filter(Boolean).at(-1) || ''
    const sameDir = !spec.includes('/') && !spec.includes('\\')
    if (!sameDir || !base) {
      await load({ force: true })
      return
    }
    const to = parent === '/' ? `/${base}` : `${parent}/${base}`
    files.value = files.value.map((f) => {
      if (f.path !== from) return f
      return { ...f, path: to, name: base }
    })
    selected.value = selected.value.map(p => (p === from ? to : p))
  } catch (e: any) {
    renameError.value = e?.data?.message || e.message || 'Failed to rename'
  }
}

function validRenameSpec(spec: string) {
  const s = spec.replaceAll('\\', '/').trim()
  if (!s) return false
  const base = s.split('/').filter(Boolean).at(-1) || ''
  return base !== '' && base !== '.' && base !== '..'
}

watch(() => props.basePath, () => load(), { immediate: true })

watch(() => user.value?.sudo, () => {
  if (props.shareId) return
  void load({ force: true })
})

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('scroll', onViewportChange, true)
  window.addEventListener('resize', onViewportChange)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('scroll', onViewportChange, true)
  window.removeEventListener('resize', onViewportChange)
})
</script>
