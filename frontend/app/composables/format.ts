export function formatSize(n: number) {
  if (n < 1000) return n + ' B'
  const u = ['KB', 'MB', 'GB', 'TB']
  let i = -1
  do { n /= 1000; i++ } while (n >= 1000 && i < u.length - 1)
  return n.toFixed(1) + ' ' + u[i]
}

export function formatDate(d: string) {
  if (!d) return '—'
  try {
    const t = new Date(d)
    return Number.isNaN(t.getTime()) ? d : t.toLocaleString()
  } catch {
    return d
  }
}

export function normalizeClientPath(p: string) {
  p = (p || '').trim()
  if (!p || p === '/') return '/'
  if (!p.startsWith('/')) p = '/' + p
  p = p.replace(/\/+/g, '/')
  if (p.length > 1) p = p.replace(/\/+$/, '')
  return p
}
