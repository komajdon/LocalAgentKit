// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const stored = { id: 'r1-t1', run_id: 'r1', tool: 'shell', status: 'failed', started: '2026-11-26T10:00:00Z', millis: 1500, args: 'command=ls', result: 'EXIT ERROR' }
let emit: (e: unknown) => void = () => {}
const off = vi.fn()
vi.mock('../../wailsjs/go/main/App', () => ({ GetToolTimeline: () => Promise.resolve([stored]) }))
vi.mock('../../wailsjs/runtime/runtime', () => ({ EventsOn: (_: string, f: (e: unknown) => void) => { emit = f; return off } }))

const { ToolTimeline, duration, upsert } = await import('./ToolTimeline')
afterEach(cleanup)

describe('tool timeline', () => {
  it('merges updates by id', () => {
    expect(upsert([stored], { ...stored, status: 'succeeded' })).toHaveLength(1)
    expect(upsert([stored], { ...stored, id: 'x' })).toHaveLength(2)
    expect([duration(20), duration(1500)]).toEqual(['20 ms', '1.5 s'])
  })

  it('shows only this conversation, follows live calls, filters and unsubscribes', async () => {
    const view = render(<ToolTimeline conversationId="c1" onClose={() => {}} />)
    await screen.findByText('shell', { selector: 'strong' })
    act(() => emit({ conversation_id: 'other', data: { ...stored, id: 'o', tool: 'leak' } }))
    act(() => emit({ conversation_id: 'c1', data: { ...stored, id: 'r2-t1', tool: 'read_file', status: 'running' } }))
    expect(screen.queryByText('leak')).toBeNull()
    expect(screen.getByText('read_file', { selector: 'strong' })).toBeTruthy()

    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'running' } })
    expect(screen.queryByText('shell', { selector: 'strong' })).toBeNull()
    view.unmount()
    expect(off).toHaveBeenCalled()
  })
})
