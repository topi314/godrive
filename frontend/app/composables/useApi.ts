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
  /** Admin ACL bypass; omitted for non-admins. Default off. */
  sudo?: boolean
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

/** Zip download for selected children of a directory (`?dl=1&name=…`). */
export function fileZipDownloadUrl(dir: string, names: string[]) {
  const base = publicFilePath(dir)
  const params = new URLSearchParams()
  params.set('dl', '1')
  for (const name of names) {
    if (name) params.append('name', name)
  }
  return base + '?' + params.toString()
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

let refreshInflight: Promise<{ ok?: boolean; session_expires_at?: string | null }> | null = null
let uploadConfigCache: { chunk_size: number; max_parallel: number; max_size: number } | null = null

export async function refreshSession() {
  if (!refreshInflight) {
    refreshInflight = $fetch<{ ok?: boolean; session_expires_at?: string | null }>('/api/refresh', {
      method: 'POST',
    }).finally(() => {
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

  async function updateMe(body: { home?: string; sudo?: boolean }) {
    return await apiFetch<Me>('/api/me', {
      method: 'PATCH',
      body,
    })
  }

  async function listPath(path: string) {
    return await apiFetch<{ path: string; files: FileEntry[]; permissions?: number; share_id?: string }>(path || '/', {
      headers: { Accept: 'application/json' },
    })
  }

  async function mkdir(dir: string, name: string) {
    const base = publicFilePath(dir)
    const target = base === '/' ? `/${name}` : `${base}/${name}`
    return await apiFetch<{ path: string; is_dir: boolean }>(target, { method: 'POST' })
  }

  async function rename(path: string, name: string) {
    await apiFetch(publicFilePath(path), {
      method: 'PATCH',
      body: { name },
    })
  }

  /** Move one or more paths into dest (directory). Uses PUT + Destination. */
  async function move(paths: string[], dest: string) {
    const unique = [...new Set(paths.map(publicFilePath).filter(p => p && p !== '/'))]
    const destination = publicFilePath(dest)
    if (!unique.length || !destination) return

    const byParent = new Map<string, string[]>()
    for (const full of unique) {
      const parts = full.split('/').filter(Boolean)
      const name = parts.at(-1)!
      const parent = parts.length <= 1 ? '/' : '/' + parts.slice(0, -1).join('/')
      const list = byParent.get(parent) || []
      list.push(name)
      byParent.set(parent, list)
    }

    await Promise.all([...byParent.entries()].map(([parent, names]) => {
      if (names.length === 1) {
        const target = parent === '/' ? `/${names[0]}` : `${parent}/${names[0]}`
        return apiFetch(target, {
          method: 'PUT',
          headers: { Destination: destination },
        })
      }
      return apiFetch(parent === '/' ? '/' : parent, {
        method: 'PUT',
        headers: { Destination: destination },
        body: names,
      })
    }))
  }

  async function setOwner(path: string, ownerId: string, opts?: { recursive?: boolean }) {
    await apiFetch(publicFilePath(path), {
      method: 'PATCH',
      body: {
        owner_id: ownerId,
        ...(opts?.recursive ? { owner_recursive: true } : {}),
      },
    })
  }

  async function remove(paths: string[]) {
    const unique = [...new Set(paths.map(publicFilePath).filter(p => p && p !== '/'))]
    if (!unique.length) return

    const byParent = new Map<string, string[]>()
    for (const full of unique) {
      const parts = full.split('/').filter(Boolean)
      const name = parts.at(-1)!
      const parent = parts.length <= 1 ? '/' : '/' + parts.slice(0, -1).join('/')
      const list = byParent.get(parent) || []
      list.push(name)
      byParent.set(parent, list)
    }

    await Promise.all([...byParent.entries()].map(([parent, names]) => {
      if (names.length === 1) {
        const target = parent === '/' ? `/${names[0]}` : `${parent}/${names[0]}`
        return apiFetch(target, { method: 'DELETE' })
      }
      return apiFetch(parent === '/' ? '/' : parent, {
        method: 'DELETE',
        body: names,
      })
    }))
  }

  function aclApiPath(path: string) {
    const p = publicFilePath(path)
    return p === '/' ? '/api/acl' : '/api/acl' + p
  }

  async function getPermissions(path: string) {
    return await apiFetch<{
      path: string
      owner_id?: string
      owner?: string
      is_dir?: boolean
      effective: number
      acl: ACLEntry[]
      inherited?: ACLEntry[]
      available_groups?: string[]
      available_users?: { id: string; username: string; email?: string }[]
    }>(aclApiPath(path))
  }

  async function putPermissions(path: string, acl: ACLEntry[]) {
    await apiFetch(aclApiPath(path), {
      method: 'PUT',
      body: { acl },
    })
  }

  async function patchPermissions(
    path: string,
    body: {
      upsert?: Pick<ACLEntry, 'principal_type' | 'principal_id' | 'allow' | 'deny'>[]
      remove?: Pick<ACLEntry, 'principal_type' | 'principal_id'>[]
    },
  ) {
    await apiFetch(aclApiPath(path), {
      method: 'PATCH',
      body,
    })
  }

  async function listAllPermissions() {
    return await apiFetch<ACLEntry[]>('/api/settings/acl')
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
    if (uploadConfigCache) {
      return { session_ttl: '', ...uploadConfigCache }
    }
    const cfg = await apiFetch<{ max_size: number; chunk_size: number; session_ttl: string; max_parallel: number }>('/api/upload/config')
    uploadConfigCache = {
      max_size: cfg.max_size,
      chunk_size: cfg.chunk_size,
      max_parallel: cfg.max_parallel,
    }
    return cfg
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
    return await apiFetch<{ id: string; path: string; size: number; upload_offset: number; chunk_size: number }>('/api/uploads', {
      method: 'POST',
      body,
    })
  }

  async function getUploadSession(id: string) {
    return await apiFetch<{ id: string; path: string; size: number; upload_offset: number }>('/api/uploads/' + encodeURIComponent(id))
  }

  async function uploadChunk(id: string, uploadOffset: number, blob: Blob) {
    const url = (useRuntimeConfig().public.apiBase || '') + '/api/uploads/' + encodeURIComponent(id)
    const send = () => new Promise<number>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('PATCH', url)
      xhr.withCredentials = true
      xhr.setRequestHeader('Upload-Offset', String(uploadOffset))
      xhr.setRequestHeader('Content-Type', 'application/octet-stream')
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const j = JSON.parse(xhr.responseText || '{}')
            resolve(Number(j.upload_offset) || uploadOffset + blob.size)
          } catch {
            resolve(uploadOffset + blob.size)
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
    return await apiFetch<{
      token: string
      token_prefix: string
      token_hash: string
      description?: string
      created_at: string
    }>('/api/tokens', {
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
    mkdir,
    rename,
    move,
    setOwner,
    remove,
    getPermissions,
    putPermissions,
    patchPermissions,
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
  }
}
