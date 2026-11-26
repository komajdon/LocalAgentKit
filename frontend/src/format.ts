import type { ConvMeta } from './types'

export function timeAgo(iso: string) {
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

export function shortPath(p: string) {
  if (!p) return '~'
  const m = p.match(/^(?:\/home\/[^/]+|\/Users\/[^/]+|[A-Za-z]:\\Users\\[^\\]+)/)
  if (m) return '~' + p.slice(m[0].length) || '~'
  return p.length > 26 ? '…' + p.slice(-24) : p
}

export function groupConvs(list: ConvMeta[]) {
  const pinned: ConvMeta[] = []
  const today: ConvMeta[] = [], week: ConvMeta[] = [], older: ConvMeta[] = []
  const now = Date.now()
  list.forEach(c => {
    if (c.pinned) { pinned.push(c); return }
    const age = now - new Date(c.updated_at).getTime()
    if (age < 86400_000) today.push(c)
    else if (age < 604800_000) week.push(c)
    else older.push(c)
  })
  return { pinned, today, week, older }
}
