// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Item } from '../../types'
import { Transcript } from './Transcript'

afterEach(cleanup)

const items: Item[] = [
  { type: 'msg', data: { id: 1, role: 'user', text: 'question' } },
  { type: 'tool', data: { id: 2, kind: 'call', tool: 'shell', body: 'command=ls' } },
  { type: 'tool', data: { id: 3, kind: 'result', tool: 'shell', body: 'ERROR: nope <img src=x>' } },
  { type: 'msg', data: { id: 4, role: 'assistant', text: 'answer' } },
]

const show = (over: Partial<Parameters<typeof Transcript>[0]> = {}) => {
  const props = { items, loading: false, running: false, thinking: false, canAct: true, onSuggest: vi.fn(), onResend: vi.fn(), onBranch: vi.fn(), ...over }
  return { props, view: render(<Transcript {...props} />) }
}

describe('transcript', () => {
  it('folds a tool call and its result into one row, as text', () => {
    const { view } = show()
    expect(view.container.querySelectorAll('.tool-step')).toHaveLength(1)
    expect(screen.getByRole('img', { name: 'Failed' })).toBeTruthy()
    expect(view.container.querySelector('img[src="x"]')).toBeNull()
  })

  it('branches by item position and resends the edited text', () => {
    const { props } = show()
    fireEvent.click(screen.getAllByLabelText('Branch conversation from this message')[1])
    expect(props.onBranch).toHaveBeenCalledWith(3)
    fireEvent.click(screen.getByLabelText('Edit and resend'))
    fireEvent.change(screen.getByLabelText('Edit message'), { target: { value: ' better question ' } })
    fireEvent.click(screen.getByLabelText('Save and resend'))
    expect(props.onResend).toHaveBeenCalledWith(1, 'better question')
  })

  it('disables history actions while a run is active', () => {
    show({ canAct: false })
    expect((screen.getByLabelText('Edit and resend') as HTMLButtonElement).disabled).toBe(true)
  })

  it('offers starting points in an empty conversation without sending them', () => {
    const { props } = show({ items: [] })
    fireEvent.click(screen.getByText('Find TODOs in the code and summarise them'))
    expect(props.onSuggest).toHaveBeenCalledWith('Find TODOs in the code and summarise them')
    expect(props.onResend).not.toHaveBeenCalled()
  })
})
