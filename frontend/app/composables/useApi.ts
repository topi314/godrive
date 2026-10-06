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
  id?: string
  username?: string
  email?: string
  avatar?: string
  home?: string
  is_admin?: boolean
  is_guest?: boolean
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

export function useApi() {
  async function me() {
    return await $fetch<Me>('/api/me')
  }

  async function listPath(path: string) {
    return await $fetch<{ path: string; files: FileEntry[]; share_id?: string }>(path || '/', {
      headers: { Accept: 'application/json' },
    })
  }

  async function upload(dir: string, file: File, description = '', onProgress?: (n: number) => void) {
    const data = new FormData()
    data.append('json', JSON.stringify({ name: file.name, description, size: file.size }))
    data.append('file', file, file.name)
    const url = publicFilePath(dir)

    await new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', url)
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) resolve()
        else reject(new Error(xhr.responseText || 'upload failed'))
      }
      xhr.onerror = () => reject(new Error('upload failed'))
      xhr.send(data)
    })
  }

  async function remove(paths: string[]) {
    for (const p of paths) {
      await $fetch(publicFilePath(p), { method: 'DELETE', body: [] })
    }
  }

  async function getPermissions(path: string) {
    return await $fetch<{ path: string; effective: number; acl: any[] }>('/api/permissions', {
      query: { path },
    })
  }

  async function putPermissions(path: string, acl: any[]) {
    await $fetch('/api/permissions', {
      method: 'PUT',
      query: { path },
      body: { acl },
    })
  }

  async function createShare(path: string) {
    return await $fetch<{ id: string; url: string }>('/api/shares', {
      method: 'POST',
      body: { path },
    })
  }

  async function listShares() {
    return await $fetch<any[]>('/api/shares')
  }

  async function listTokens() {
    return await $fetch<any[]>('/api/tokens')
  }

  async function createToken(description: string) {
    return await $fetch<{ token: string }>('/api/tokens', {
      method: 'POST',
      body: { description },
    })
  }

  async function deleteToken(hash: string) {
    await $fetch('/api/tokens/' + hash, { method: 'DELETE' })
  }

  async function listUsers() {
    return await $fetch<any[]>('/api/settings/users')
  }

  return {
    me,
    listPath,
    upload,
    remove,
    getPermissions,
    putPermissions,
    createShare,
    listShares,
    listTokens,
    createToken,
    deleteToken,
    listUsers,
  }
}
