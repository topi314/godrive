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
    if (Number.isNaN(t.getTime())) return d
    return t.toLocaleString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    })
  } catch {
    return d
  }
}

/**
 * GitHub-style relative time for list views (“2 hours ago”, “yesterday”, “Mar 15”).
 * Use formatDate for the absolute timestamp (e.g. title).
 */
export function formatRelativeDate(d: string, now = new Date()) {
  if (!d) return '—'
  const t = new Date(d)
  if (Number.isNaN(t.getTime())) return d

  const deltaMs = t.getTime() - now.getTime()
  const absSec = Math.round(Math.abs(deltaMs) / 1000)
  if (absSec < 60) return 'just now'

  const absDay = absSec / 86400
  // After ~30 days, switch to a short absolute date.
  if (absDay >= 30) {
    const opts: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' }
    if (t.getFullYear() !== now.getFullYear()) opts.year = 'numeric'
    return t.toLocaleDateString(undefined, opts)
  }

  const divisions: [Intl.RelativeTimeFormatUnit, number][] = [
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ]
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  for (const [unit, seconds] of divisions) {
    if (absSec >= seconds || unit === 'minute') {
      const value = Math.round(deltaMs / 1000 / seconds)
      return rtf.format(value, unit)
    }
  }
  return 'just now'
}

export function normalizeClientPath(p: string) {
  p = (p || '').trim()
  if (!p || p === '/') return '/'
  if (!p.startsWith('/')) p = '/' + p
  p = p.replace(/\/+/g, '/')
  if (p.length > 1) p = p.replace(/\/+$/, '')
  return p
}
