import type { KeyboardEvent, MouseEvent } from 'react'

/**
 * Props that make a non-button element behave as a button: focusable, and
 * activated by click, Enter or Space. Keys from nested controls are ignored.
 */
export function activate(action: () => void, label?: string) {
  return {
    role: 'button' as const,
    tabIndex: 0,
    'aria-label': label,
    onClick: (e: MouseEvent) => { e.stopPropagation(); action() },
    onKeyDown: (e: KeyboardEvent) => {
      if (e.target !== e.currentTarget || (e.key !== 'Enter' && e.key !== ' ')) return
      e.preventDefault()
      action()
    },
  }
}
