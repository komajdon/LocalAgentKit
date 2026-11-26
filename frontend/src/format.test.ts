import { describe, expect, it, vi } from 'vitest'
import { groupConvs, shortPath, timeAgo } from './format'

const conv = (id: string, updated_at: string, pinned = false) => ({ id, title: id, work_dir: '/w', updated_at, pinned })

describe('conversation list formatting', () => {
  it('groups by age with pinned first', () => {
    vi.useFakeTimers({ now: new Date('2026-11-26T12:00:00Z') })
    const g = groupConvs([conv('a', '2026-11-26T11:00:00Z'), conv('b', '2026-11-23T11:00:00Z'), conv('c', '2026-01-01T00:00:00Z'), conv('p', '2026-01-01T00:00:00Z', true)] as any)
    expect([g.pinned, g.today, g.week, g.older].map(l => l.map(c => c.id))).toEqual([['p'], ['a'], ['b'], ['c']])
    expect(timeAgo('2026-11-26T11:59:30Z')).toBeTruthy()
    vi.useRealTimers()
  })

  it('shortens long paths', () => {
    expect(shortPath('/w')).toBe('/w')
    expect(shortPath('/home/someone/projects/a/very/deep/directory').length).toBeLessThan(40)
  })
})
