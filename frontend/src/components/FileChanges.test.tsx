// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const undo = vi.fn().mockResolvedValue('conflict: the file has changed since')
vi.mock('../../wailsjs/go/main/App', () => ({
  ListFileChanges: () => Promise.resolve([{ id: 'c/1', at: '2026-11-26T10:00:00Z', path: '/w/a.txt', kind: 'modified', diff: '--- a/a\n+++ b/a\n@@ -1 +1 @@\n-<img src=x>\n+new', undoable: true, undone: false }]),
  GetGitStatus: () => Promise.resolve({ repository: true, lines: ['## main', ' M a.txt'], error: '' }),
  UndoFileChange: (id: string) => undo(id),
}))

const { FileChanges } = await import('./FileChanges')
afterEach(cleanup)

describe('file review', () => {
  it('shows diffs as text, git status, and reports an undo conflict', async () => {
    const view = render(<FileChanges conversationId="c" onClose={() => {}} />)
    await screen.findByText('/w/a.txt')
    expect(view.container.querySelector('img')).toBeNull()
    expect(view.container.querySelector('.diff-view .add')?.textContent).toBe('+new')
    expect(await screen.findByText('1 changed paths')).toBeTruthy()
    fireEvent.click(screen.getByText('Undo this change'))
    expect((await screen.findByRole('alert')).textContent).toContain('conflict')
    expect(undo).toHaveBeenCalledWith('c/1')
  })
})
