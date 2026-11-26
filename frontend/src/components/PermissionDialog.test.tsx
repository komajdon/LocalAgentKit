// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PermissionDialog } from './PermissionDialog'

const request = {
  id: 'p1', tool: 'shell', description: 'Run a shell command', args: { command: 'rm -rf <b>x</b>', retries: 2, opts: { a: 1 } },
  workspace: '/w', operation: 'Run a shell command', target: 'rm -rf <b>x</b>',
  impact: 'Can delete files.', sensitive: true, scope: 'only this exact request in this workspace',
}
afterEach(cleanup)

describe('permission dialog', () => {
  it('shows the tool, target, impact, workspace, scope and every argument as text', () => {
    const view = render(<PermissionDialog request={request as any} onAnswer={() => {}} />)
    const text = screen.getByRole('dialog').textContent ?? ''
    for (const part of ['shell', 'rm -rf <b>x</b>', 'Can delete files.', '/w', 'only this exact request', 'sensitive', '"a": 1', 'retries']) {
      expect(text).toContain(part)
    }
    expect(view.container.querySelector('b')).toBeNull()
  })

  it.each([
    ['Deny', 'deny'], ['Allow once', 'once'], ['Allow this session', 'session'], ['Always in this workspace', 'workspace'],
  ])('%s answers %s and nothing is chosen without a click', (label, decision) => {
    const onAnswer = vi.fn()
    render(<PermissionDialog request={request as any} onAnswer={onAnswer} />)
    expect(onAnswer).not.toHaveBeenCalled()
    fireEvent.click(screen.getByText(new RegExp(label)))
    expect(onAnswer).toHaveBeenCalledExactlyOnceWith(decision)
  })
})
