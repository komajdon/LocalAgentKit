// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { activate } from './activate'

afterEach(cleanup)

describe('activate', () => {
  it('works by click, Enter and Space but not through nested controls', () => {
    const action = vi.fn()
    render(<div {...activate(action, 'open')}><input aria-label="inner" /></div>)
    const el = screen.getByRole('button', { name: 'open' })
    expect(el.tabIndex).toBe(0)
    fireEvent.click(el)
    fireEvent.keyDown(el, { key: 'Enter' })
    fireEvent.keyDown(el, { key: ' ' })
    fireEvent.keyDown(el, { key: 'a' })
    fireEvent.keyDown(screen.getByLabelText('inner'), { key: 'Enter' })
    expect(action).toHaveBeenCalledTimes(3)
  })
})
