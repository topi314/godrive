<template>
  <div v-if="open" class="dialog-backdrop" @click.self="close">
    <div class="dialog dialog-wide" role="dialog" aria-modal="true" aria-labelledby="upload-title">
      <div class="dialog-head">
        <h2 id="upload-title">Upload</h2>
        <IconBtn name="x" label="Close" variant="ghost" :disabled="busy" @click="close" />
      </div>

      <p class="muted">
        Paths are relative to <code>{{ baseDir }}</code>, or absolute (start with <code>/</code>).
      </p>

      <div class="upload-options">
        <label class="quick-publish">
          <input v-model="replace" type="checkbox">
          Replace existing
        </label>
        <label v-if="!shareId" class="quick-publish">
          <input v-model="guestRead" type="checkbox">
          Guests can read
        </label>
      </div>

      <label>
        Description
        <input v-model="description" placeholder="optional">
      </label>

      <div class="upload-queue">
        <div v-for="item in items" :key="item.id" class="upload-row">
          <label class="upload-name">
            Path
            <input
              v-model="item.name"
              :disabled="item.status === 'uploading' || item.status === 'done'"
              @change="recheck(item)"
            >
          </label>
          <p class="muted upload-resolved">→ {{ resolvePreview(item.name) }}</p>
          <div class="progress"><i :style="{ width: item.progress + '%' }" /></div>
          <div class="upload-row-meta">
            <span class="muted">{{ statusLabel(item) }}</span>
            <span v-if="item.error" class="error">{{ item.error }}</span>
            <div class="upload-row-actions">
              <IconBtn
                v-if="item.status === 'failed' || item.status === 'declined'"
                name="check"
                label="Retry"
                variant="primary"
                @click="retryItem(item)"
              />
              <IconBtn
                v-if="item.status !== 'uploading' && item.status !== 'done'"
                name="trash"
                label="Remove"
                variant="danger"
                @click="removeItem(item.id)"
              />
            </div>
          </div>
        </div>
        <p v-if="!items.length" class="muted">No files yet — add files to upload.</p>
      </div>

      <div class="dialog-actions">
        <IconBtn name="upload" label="Add files" :disabled="busy" @click="picker?.click()" />
        <IconBtn
          name="check"
          label="Start upload"
          variant="primary"
          :disabled="busy || !items.some(i => i.status === 'queued' || i.status === 'pending')"
          @click="startAll"
        />
      </div>
      <input ref="picker" type="file" multiple hidden @change="onPick">
    </div>
  </div>
</template>

<script setup lang="ts">
import { Perm, type ACLEntry } from '~/composables/useApi'

type ItemStatus = 'pending' | 'checking' | 'declined' | 'queued' | 'uploading' | 'done' | 'failed'

type QueueItem = {
  id: string
  file: File
  name: string
  status: ItemStatus
  progress: number
  error: string
  sessionId?: string
  uploadOffset: number
}

const open = defineModel<boolean>({ required: true })
const props = defineProps<{
  baseDir: string
  shareId?: string
  initialFiles?: File[]
}>()
const emit = defineEmits<{ done: [] }>()

const api = useApi()
const items = ref<QueueItem[]>([])
const replace = ref(false)
const guestRead = ref(false)
const description = ref('')
const busy = ref(false)
const picker = ref<HTMLInputElement | null>(null)
const chunkSize = ref(16_000_000)
const maxParallel = ref(6)
const maxSize = ref(50_000_000_000)

function uid() {
  return Math.random().toString(36).slice(2) + Date.now().toString(36)
}

function resolvePreview(name: string) {
  const n = (name || '').trim().replace(/\\/g, '/')
  if (!n) return '—'
  if (n.startsWith('/')) return n.replace(/\/+/g, '/').replace(/\/$/, '') || '/'
  const base = (props.baseDir || '/').replace(/\/+$/, '') || ''
  return (base + '/' + n).replace(/\/+/g, '/')
}

function statusLabel(item: QueueItem) {
  switch (item.status) {
    case 'pending': return 'Pending'
    case 'checking': return 'Checking…'
    case 'declined': return 'Declined'
    case 'queued': return 'Ready'
    case 'uploading': return `Uploading ${item.progress}%`
    case 'done': return 'Done'
    case 'failed': return 'Failed'
  }
}

function addFiles(files: File[]) {
  for (const file of files) {
    const item: QueueItem = {
      id: uid(),
      file,
      name: file.name,
      status: 'pending',
      progress: 0,
      error: '',
      uploadOffset: 0,
    }
    try {
      const sid = sessionStorage.getItem('godrive-upload:' + item.id)
      if (sid) item.sessionId = sid
    } catch { /* ignore */ }
    items.value.push(item)
  }
}

function removeItem(id: string) {
  items.value = items.value.filter(i => i.id !== id)
}

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) {
    addFiles([...input.files])
    for (const item of items.value) {
      if (item.status === 'pending') void recheck(item)
    }
  }
  input.value = ''
}

async function loadConfig() {
  try {
    const cfg = await api.uploadConfig()
    chunkSize.value = cfg.chunk_size || chunkSize.value
    maxParallel.value = cfg.max_parallel || maxParallel.value
    maxSize.value = cfg.max_size || maxSize.value
  } catch { /* defaults */ }
}

async function recheck(item: QueueItem) {
  if (item.status === 'uploading' || item.status === 'done') return
  item.status = 'checking'
  item.error = ''
  if (item.file.size > maxSize.value) {
    item.status = 'declined'
    item.error = 'file too large'
    return
  }
  try {
    const res = await api.preflightUpload({
      dir: props.baseDir || '/',
      name: item.name,
      size: item.file.size,
      content_type: item.file.type,
      replace: replace.value,
      share_id: props.shareId,
    })
    if (!res.ok) {
      item.status = 'declined'
      item.error = (res.errors || ['rejected']).join(', ')
      return
    }
    item.status = 'queued'
  } catch (e: any) {
    item.status = 'declined'
    item.error = e?.data?.message || e.message || 'preflight failed'
  }
}

async function uploadOne(item: QueueItem) {
  item.error = ''
  item.progress = Math.round((item.uploadOffset / (item.file.size || 1)) * 100)
  try {
    // Already validated via preflight when queued; only recheck pending/failed.
    if (item.status !== 'queued') {
      await recheck(item)
      if (item.status === 'declined') return
    }
    item.status = 'uploading'

    let sessionId = item.sessionId
    let uploadOffset = item.uploadOffset
    if (sessionId) {
      try {
        const st = await api.getUploadSession(sessionId)
        uploadOffset = st.upload_offset
        item.uploadOffset = uploadOffset
      } catch {
        sessionId = undefined
        uploadOffset = 0
        item.uploadOffset = 0
      }
    }
    if (!sessionId) {
      const sess = await api.createUploadSession({
        dir: props.baseDir || '/',
        name: item.name,
        size: item.file.size,
        content_type: item.file.type,
        description: description.value,
        replace: replace.value,
        share_id: props.shareId,
      })
      sessionId = sess.id
      uploadOffset = sess.upload_offset
      item.sessionId = sessionId
      try {
        sessionStorage.setItem('godrive-upload:' + item.id, sessionId)
      } catch { /* ignore */ }
    }

    const size = item.file.size
    while (uploadOffset < size) {
      const end = Math.min(uploadOffset + chunkSize.value, size)
      const blob = item.file.slice(uploadOffset, end)
      uploadOffset = await api.uploadChunk(sessionId, uploadOffset, blob)
      item.uploadOffset = uploadOffset
      item.progress = size ? Math.round((uploadOffset / size) * 100) : 100
    }

    const acl: ACLEntry[] = []
    if (!props.shareId && guestRead.value) {
      acl.push({ principal_type: 'guest', principal_id: '*', allow: Perm.Read, deny: 0 })
    }
    await api.completeUpload(sessionId, {
      description: description.value,
      acl: acl.length ? acl : undefined,
    })
    item.progress = 100
    item.status = 'done'
    item.sessionId = undefined
    try { sessionStorage.removeItem('godrive-upload:' + item.id) } catch { /* ignore */ }
  } catch (e: any) {
    item.status = 'failed'
    item.error = e?.data?.message || e.message || 'upload failed'
  }
}

async function startAll() {
  busy.value = true
  const queue = items.value.filter(i => i.status === 'queued' || i.status === 'pending' || i.status === 'failed')
  for (const item of queue) {
    if (item.status === 'pending' || item.status === 'failed') await recheck(item)
  }
  const ready = items.value.filter(i => i.status === 'queued')
  let i = 0
  const workers = Array.from({ length: Math.min(maxParallel.value, ready.length) }, async () => {
    while (i < ready.length) {
      const item = ready[i++]
      await uploadOne(item)
    }
  })
  await Promise.all(workers)
  busy.value = false
  if (items.value.some(x => x.status === 'done')) emit('done')
}

async function retryItem(item: QueueItem) {
  await uploadOne(item)
  if (item.status === 'done') emit('done')
}

function close() {
  if (busy.value) return
  open.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))

watch(open, async (v) => {
  if (!v) {
    if (!busy.value) {
      items.value = []
      replace.value = false
      guestRead.value = false
      description.value = ''
    }
    return
  }
  await loadConfig()
  items.value = []
  if (props.initialFiles?.length) addFiles(props.initialFiles)
  for (const item of items.value) {
    if (item.status === 'pending') await recheck(item)
  }
})

watch(replace, () => {
  for (const item of items.value) {
    if (item.status === 'declined' || item.status === 'queued' || item.status === 'failed') {
      void recheck(item)
    }
  }
})
</script>
