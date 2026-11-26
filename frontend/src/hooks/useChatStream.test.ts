// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const listeners = new Map<string, (e: any) => void>()
const respond = vi.fn()
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, f: (e: any) => void) => { listeners.set(name, f); return () => listeners.delete(name) },
}))
vi.mock('../../wailsjs/go/main/App', () => ({
  GetRunState: () => Promise.resolve({ run_id: '', conversation_id: '' }),
  RespondPermission: (...a: unknown[]) => respond(...a),
}))

const { useChatStream } = await import('./useChatStream')

const emit = (name: string, conversation_id: string, seq: number, data: unknown) =>
  act(() => listeners.get(`chat:${name}`)!({ conversation_id, run_id: 'r1', seq, data }))
const texts = (items: any[]) => items.map(i => i.type === 'msg' ? i.data.text : i.type === 'tool' ? i.data.body : i.text)

const setup = () => renderHook(() => useChatStream({ notify: vi.fn(), onRunEnd: vi.fn() }))

describe('chat stream', () => {
  beforeEach(() => { listeners.clear(); respond.mockClear() })
  afterEach(cleanup)

  it('ignores events of conversations that are not shown', () => {
    const { result } = setup()
    act(() => result.current.commitLoad(result.current.beginLoad('b'), [], false, 0))
    emit('started', 'a', 1, null)
    emit('chunk', 'a', 2, 'secret of a')
    expect(result.current.items).toHaveLength(0)
    expect(result.current.runConv).toBe('a')
    emit('done', 'a', 3, null)
    expect(result.current.runConv).toBeNull()
  })

  it('applies exactly the events missing from the snapshot of a running conversation', () => {
    const { result } = setup()
    let token = 0
    act(() => { token = result.current.beginLoad('a') })
    emit('chunk', 'a', 4, 'par') // already in the snapshot
    emit('chunk', 'a', 5, 'tial') // newer
    act(() => result.current.commitLoad(token, [{ type: 'msg', data: { id: -1, role: 'assistant', text: 'par', streaming: true } }], true, 4))
    expect(texts(result.current.items)).toEqual(['partial'])
    emit('assistant', 'a', 6, 'partial reply')
    emit('done', 'a', 7, null)
    expect(texts(result.current.items)).toEqual(['partial reply'])
  })

  it('drops a load that a newer one replaced', () => {
    const { result } = setup()
    let first = 0, second = 0
    act(() => { first = result.current.beginLoad('a'); second = result.current.beginLoad('b') })
    act(() => result.current.commitLoad(second, [{ type: 'status', id: -1, text: 'b' }], false, 0))
    act(() => result.current.commitLoad(first, [{ type: 'status', id: -2, text: 'a' }], false, 0))
    expect(texts(result.current.items)).toEqual(['b'])
    expect(result.current.isStale(first)).toBe(true)
  })

  it('marks stops and errors and removes a reply that was only tool calls', () => {
    const { result } = setup()
    act(() => result.current.commitLoad(result.current.beginLoad('a'), [], false, 0))
    emit('chunk', 'a', 1, 'TOOL_CALL: {}')
    emit('assistant', 'a', 2, '')
    expect(result.current.items).toHaveLength(0)
    emit('stopped', 'a', 3, null)
    emit('error', 'a', 4, 'boom')
    expect(texts(result.current.items)).toEqual(['Stopped by user', 'boom'])
  })

  it('answers the permission request it shows and unsubscribes', () => {
    const view = setup()
    act(() => listeners.get('chat:permission_request')!({ id: 'p1', tool: 'shell' }))
    act(() => listeners.get('chat:permission_closed')!('other'))
    expect(view.result.current.permReq?.id).toBe('p1')
    act(() => view.result.current.answerPermission('once'))
    expect(respond).toHaveBeenCalledWith('p1', 'once')
    expect(view.result.current.permReq).toBeNull()
    view.unmount()
    expect(listeners.size).toBe(0)
  })
})
