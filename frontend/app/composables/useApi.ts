export type FileEntry = {
  path: string
  name: string
  is_dir: boolean
  size: number
  content_type?: string
  description?: string
  owner_id?: string
  owner?: string
  date: string
  permissions: number
}

export type Me = {
  authenticated: boolean
  auth_enabled?: boolean
  guests_allowed?: boolean
  id?: string
  username?: string
  email?: string
  avatar?: string
  home?: string
  groups?: string[]
  available_groups?: string[]
  is_admin?: boolean
  is_access?: boolean
  is_guest?: boolean
  session_expires_at?: string | null
}

export type ACLEntry = {
  path?: string
  principal_type: string
  principal_id: string
  allow: number
  deny: number
}

export const Perm = {
  Read: 1,
  Create: 2,
  Update: 4,
  Delete: 8,
  UpdatePermissions: 16,
  Share: 32,
} as const

export function hasPerm(bits: number, bit: number) {
  return (bits & bit) === bit
}

function publicFilePath(path: string) {
  if (!path || path === '/') return '/'
  return path.startsWith('/') ? path : '/' + path
}

/** Force-download URL (`?dl=1`) on the public file path. */
export function fileDownloadUrl(path: string) {
  const base = publicFilePath(path)
  return base + (base.includes('?') ? '&' : '?') + 'dl=1'
}

/** Inline stream URL — public path, raw bytes for the browser. */
export function fileStreamUrl(path: string) {
  return publicFilePath(path)
}

export type MediaKind = 'audio' | 'video' | 'image' | 'pdf' | null

export function mediaKind(name: string, contentType = ''): MediaKind {
  const ct = (contentType || '').toLowerCase()
  if (ct.startsWith('audio/') || /\.(mp3|wav|ogg|m4a|aac|flac|opus|webm)$/i.test(name)) return 'audio'
  if (ct.startsWith('video/') || /\.(mp4|webm|ogv|mov|m4v)$/i.test(name)) return 'video'
  if (ct.startsWith('image/') || /\.(png|jpe?g|gif|webp|svg|bmp|avif)$/i.test(name)) return 'image'
  if (ct === 'application/pdf' || /\.pdf$/i.test(name)) return 'pdf'
  return null
}

function fetchStatus(err: any) {
  return err?.status || err?.response?.status || err?.statusCode || 0
}

let refreshInflight: Promise<void> | null = null

export async function refreshSession() {
  if (!refreshInflight) {
    refreshInflight = $fetch('/api/refresh', { method: 'POST' })
      .then(() => undefined)
      .finally(() => {
        refreshInflight = null
      })
  }
  return refreshInflight
}

async function apiFetch<T>(url: string, opts: Record<string, any> = {}) {
  try {
    return await $fetch<T>(url, opts as any)
  } catch (err: any) {
    if (opts._noRefresh || fetchStatus(err) !== 401 || url.includes('/api/refresh')) {
      throw err
    }
    try {
      await refreshSession()
    } catch {
      throw err
    }
    return await $fetch<T>(url, { ...opts, _noRefresh: true } as any)
  }
}

export function useApi() {
  async function me() {
    return await apiFetch<Me>('/api/me')
  }

  async function updateMe(body: { home: string }) {
    return await apiFetch<{ id: string; home: string }>('/api/me', {
      method: 'PATCH',
      body,
    })
  }

  async function listPath(path: string) {
    return await apiFetch<{ path: string; files: FileEntry[]; permissions?: number; share_id?: string }>(path || '/', {
      headers: { Accept: 'application/json' },
    })
  }

  async function upload(dir: string, file: File, description = '', onProgress?: (n: number) => void) {
    const data = new FormData()
    data.append('json', JSON.stringify({ name: file.name, description, size: file.size }))
    data.append('file', file, file.name)
    const url = publicFilePath(dir)

    const send = () => new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', url)
      xhr.withCredentials = true
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) resolve()
        else reject(Object.assign(new Error(xhr.responseText || 'upload failed'), { status: xhr.status }))
      }
      xhr.onerror = () => reject(new Error('upload failed'))
      xhr.send(data)
    })
    try {
      await send()
    } catch (err: any) {
      if (fetchStatus(err) !== 401) throw err
      await refreshSession()
      await send()
    }
  }

  async function mkdir(dir: string, name: string) {
    await apiFetch(publicFilePath(dir), {
      method: 'POST',
      body: { name, mkdir: true },
    })
  }

  async function rename(path: string, name: string) {
    await apiFetch(publicFilePath(path), {
      method: 'PATCH',
      body: { name },
    })
  }

  async function remove(paths: string[]) {
    for (const p of paths) {
      await apiFetch(publicFilePath(p), { method: 'DELETE', body: [] })
    }
  }

  async function getPermissions(path: string) {
    return await apiFetch<{
      path: string
      effective: number
      acl: ACLEntry[]
      inherited?: ACLEntry[]
      available_groups?: string[]
      available_users?: { id: string; username: string; email?: string }[]
    }>('/api/permissions', {
      query: { path },
    })
  }

  async function putPermissions(path: string, acl: ACLEntry[]) {
    await apiFetch('/api/permissions', {
      method: 'PUT',
      query: { path },
      body: { acl },
    })
  }

  async function listAllPermissions() {
    return await apiFetch<ACLEntry[]>('/api/settings/permissions')
  }

  async function createShare(path: string, opts?: { expires_in?: string; allow?: number; deny?: number }) {
    const body: { path: string; expires_in?: string; allow?: number; deny?: number } = { path }
    if (opts?.expires_in) body.expires_in = opts.expires_in
    if (opts?.allow != null) body.allow = opts.allow
    if (opts?.deny != null) body.deny = opts.deny
    return await apiFetch<{ id: string; url: string; expires_at?: string | null; allow?: number }>('/api/shares', {
      method: 'POST',
      body,
    })
  }

  async function uploadConfig() {
    return await apiFetch<{ max_size: number; chunk_size: number; session_ttl: string; max_parallel: number }>('/api/upload/config')
  }

  async function preflightUpload(body: {
    dir: string
    name: string
    size: number
    content_type?: string
    replace?: boolean
    share_id?: string
  }) {
    return await apiFetch<{ ok: boolean; path?: string; errors?: string[] }>('/api/uploads/preflight', {
      method: 'POST',
      body,
    })
  }

  async function createUploadSession(body: {
    dir: string
    name: string
    size: number
    content_type?: string
    description?: string
    replace?: boolean
    share_id?: string
  }) {
    return await apiFetch<{ id: string; path: string; size: number; offset: number; chunk_size: number }>('/api/uploads', {
      method: 'POST',
      body,
    })
  }

  async function getUploadSession(id: string) {
    return await apiFetch<{ id: string; path: string; size: number; offset: number }>('/api/uploads/' + encodeURIComponent(id))
  }

  async function uploadChunk(id: string, offset: number, blob: Blob) {
    const url = (useRuntimeConfig().public.apiBase || '') + '/api/uploads/' + encodeURIComponent(id)
    const send = () => new Promise<number>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('PATCH', url)
      xhr.withCredentials = true
      xhr.setRequestHeader('Upload-Offset', String(offset))
      xhr.setRequestHeader('Content-Type', 'application/octet-stream')
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const j = JSON.parse(xhr.responseText || '{}')
            resolve(Number(j.offset) || offset + blob.size)
          } catch {
            resolve(offset + blob.size)
          }
        } else {
          reject(Object.assign(new Error(xhr.responseText || 'chunk failed'), { status: xhr.status }))
        }
      }
      xhr.onerror = () => reject(new Error('chunk failed'))
      xhr.send(blob)
    })
    try {
      return await send()
    } catch (err: any) {
      if (fetchStatus(err) !== 401) throw err
      await refreshSession()
      return await send()
    }
  }

  async function completeUpload(id: string, body?: { description?: string; acl?: ACLEntry[] }) {
    return await apiFetch<{ path: string }>('/api/uploads/' + encodeURIComponent(id) + '/complete', {
      method: 'POST',
      body: body || {},
    })
  }

  async function abortUpload(id: string) {
    await apiFetch('/api/uploads/' + encodeURIComponent(id), { method: 'DELETE' })
  }

  async function listShares() {
    return await apiFetch<any[]>('/api/shares')
  }

  async function deleteShare(id: string) {
    await apiFetch('/api/shares/' + id, { method: 'DELETE' })
  }

  async function listTokens() {
    return await apiFetch<any[]>('/api/tokens')
  }

  async function createToken(description: string) {
    return await apiFetch<{ token: string }>('/api/tokens', {
      method: 'POST',
      body: { description },
    })
  }

  async function deleteToken(hash: string) {
    await apiFetch('/api/tokens/' + hash, { method: 'DELETE' })
  }

  async function listUsers() {
    return await apiFetch<any[]>('/api/settings/users')
  }

  async function updateUser(id: string, body: { home: string }) {
    return await apiFetch<any>('/api/settings/users/' + encodeURIComponent(id), {
      method: 'PATCH',
      body,
    })
  }

  async function deleteUser(id: string) {
    await apiFetch('/api/settings/users/' + encodeURIComponent(id), { method: 'DELETE' })
  }

  return {
    me,
    updateMe,
    listPath,
    upload,
    mkdir,
    rename,
    remove,
    getPermissions,
    putPermissions,
    listAllPermissions,
    createShare,
    listShares,
    deleteShare,
    listTokens,
    createToken,
    deleteToken,
    listUsers,
    updateUser,
    deleteUser,
    refreshSession,
    uploadConfig,
    preflightUpload,
    createUploadSession,
    getUploadSession,
    uploadChunk,
    completeUpload,
    abortUpload,
  }
}
