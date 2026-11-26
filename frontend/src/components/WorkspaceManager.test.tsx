// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const saveWorkspace = vi.fn().mockResolvedValue('')
const builtIn = { id: 'readonly', name: 'Read-Only Assistant', built_in: true, instructions: '', model: '', tools: ['read_file'], permissions: null, context_limit: 0, context_strategy: '', max_rounds: 0 }
vi.mock('../../wailsjs/go/main/App', () => ({
  ListProfiles: () => Promise.resolve([builtIn]),
  ListWorkspaces: () => Promise.resolve([]),
  ListTools: () => Promise.resolve([{ name: 'shell' }]),
  SaveWorkspace: (w: unknown) => saveWorkspace(w),
  SaveProfile: () => Promise.resolve(''),
  DeleteProfile: vi.fn(), DeleteWorkspace: vi.fn(), PickDirectory: vi.fn(),
}))

const { WorkspaceManager, parsePermissions } = await import('./WorkspaceManager')
afterEach(cleanup)

describe('workspaces and profiles', () => {
  it('parses permission lines', () => {
    expect(parsePermissions('shell = deny\n\nwrite_file')).toEqual({ shell: 'deny', write_file: 'ask' })
  })

  it('offers copying, not editing, of built-ins and saves a workspace', async () => {
    render(<WorkspaceManager onClose={() => {}} />)
    await screen.findByText('Read-Only Assistant · built in')
    expect(screen.queryByText('Delete')).toBeNull()
    fireEvent.click(screen.getByText('New workspace'))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Site' } })
    fireEvent.change(screen.getByLabelText('Directory'), { target: { value: '/w' } })
    fireEvent.change(screen.getByLabelText(/Ignored paths/), { target: { value: '.env\n*.pem\n' } })
    fireEvent.click(screen.getByText('Save workspace'))
    await waitFor(() => expect(saveWorkspace).toHaveBeenCalledWith(expect.objectContaining({ name: 'Site', path: '/w', ignore: ['.env', '*.pem'] })))
  })
})
