import { useCallback, useEffect, useState } from 'react'
import {
  DeleteProfile, DeleteWorkspace, ListProfiles, ListTools, ListWorkspaces,
  PickDirectory, SaveProfile, SaveWorkspace,
} from '../../wailsjs/go/main/App'

export interface Profile {
  id: string; name: string; built_in: boolean; instructions: string; model: string
  tools: string[] | null; permissions: Record<string, string> | null
  context_limit: number; context_strategy: string; max_rounds: number
}
export interface Workspace {
  id: string; name: string; path: string; profile_id: string
  permissions: Record<string, string> | null; ignore: string[] | null
}

const NEW_PROFILE: Profile = { id: '', name: '', built_in: false, instructions: '', model: '', tools: [], permissions: {}, context_limit: 0, context_strategy: '', max_rounds: 0 }
const NEW_WORKSPACE: Workspace = { id: '', name: '', path: '', profile_id: '', permissions: {}, ignore: [] }

/** "shell=deny" lines to a permission map; other values are rejected by the backend. */
export function parsePermissions(text: string) {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const [tool, value] = line.split('=').map(s => s.trim())
    if (tool) out[tool] = value || 'ask'
  }
  return out
}
const formatPermissions = (m: Record<string, string> | null) => Object.entries(m ?? {}).map(([k, v]) => `${k}=${v}`).join('\n')
const lines = (text: string) => text.split('\n').map(s => s.trim()).filter(Boolean)

function ProfileForm({ initial, tools, onDone }: { initial: Profile; tools: string[]; onDone: (error: string) => void }) {
  const [p, setP] = useState(initial)
  const [perms, setPerms] = useState(formatPermissions(initial.permissions))
  const patch = (c: Partial<Profile>) => setP(old => ({ ...old, ...c }))
  const enabled = p.tools ?? []
  return (
    <div className="field" aria-label="Profile editor">
      <label>Name<input value={p.name} onChange={e => patch({ name: e.target.value })} /></label>
      <label>Instructions<textarea rows={4} value={p.instructions} onChange={e => patch({ instructions: e.target.value })} /></label>
      <label>Model (empty keeps the configured one)<input value={p.model} onChange={e => patch({ model: e.target.value })} /></label>
      <fieldset>
        <legend>Enabled tools (none ticked means all, including MCP)</legend>
        {tools.map(t => (
          <label key={t} className="mcp-toggle">
            <input type="checkbox" checked={enabled.includes(t)}
              onChange={e => patch({ tools: e.target.checked ? [...enabled, t] : enabled.filter(x => x !== t) })} />
            <span>{t}</span>
          </label>
        ))}
      </fieldset>
      <label>Stricter permissions, one per line (tool=ask or tool=deny)
        <textarea rows={3} value={perms} onChange={e => setPerms(e.target.value)} />
      </label>
      <div className="usage-grid">
        <label>Context limit (0 = default)<input type="number" min={0} value={p.context_limit} onChange={e => patch({ context_limit: Number(e.target.value) })} /></label>
        <label>Model rounds (0 = default)<input type="number" min={0} value={p.max_rounds} onChange={e => patch({ max_rounds: Number(e.target.value) })} /></label>
        <label>Older turns
          <select value={p.context_strategy} onChange={e => patch({ context_strategy: e.target.value })}>
            <option value="">Default</option><option value="sliding">Sliding window</option>
            <option value="hybrid">Hybrid</option><option value="summary">Summary</option>
          </select>
        </label>
      </div>
      <div className="field-hint">Limits can only lower the global ones. The provider and key are always the global ones.</div>
      <button className="btn primary" onClick={() => SaveProfile({ ...p, permissions: parsePermissions(perms) } as any).then(onDone)}>Save profile</button>
    </div>
  )
}

function WorkspaceForm({ initial, profiles, onDone }: { initial: Workspace; profiles: Profile[]; onDone: (error: string) => void }) {
  const [w, setW] = useState(initial)
  const [perms, setPerms] = useState(formatPermissions(initial.permissions))
  const [ignore, setIgnore] = useState((initial.ignore ?? []).join('\n'))
  const patch = (c: Partial<Workspace>) => setW(old => ({ ...old, ...c }))
  return (
    <div className="field" aria-label="Workspace editor">
      <label>Name<input value={w.name} onChange={e => patch({ name: e.target.value })} /></label>
      <label>Directory<input value={w.path} onChange={e => patch({ path: e.target.value })} /></label>
      <button className="btn secondary" onClick={() => PickDirectory().then(d => d && patch({ path: d }))}>Browse…</button>
      <label>Default profile
        <select value={w.profile_id} onChange={e => patch({ profile_id: e.target.value })}>
          <option value="">General Assistant</option>
          {profiles.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </label>
      <label>Stricter permissions, one per line (tool=ask or tool=deny)
        <textarea rows={3} value={perms} onChange={e => setPerms(e.target.value)} />
      </label>
      <label>Ignored paths, one pattern per line (for example .env or *.pem)
        <textarea rows={3} value={ignore} onChange={e => setIgnore(e.target.value)} />
      </label>
      <div className="field-hint">File and document tools cannot open ignored paths. Shell commands are not bound by these patterns.</div>
      <button className="btn primary" onClick={() => SaveWorkspace({ ...w, permissions: parsePermissions(perms), ignore: lines(ignore) } as any).then(onDone)}>Save workspace</button>
    </div>
  )
}

export function WorkspaceManager({ onClose }: { onClose: () => void }) {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [tools, setTools] = useState<string[]>([])
  const [profile, setProfile] = useState<Profile | null>(null)
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [error, setError] = useState('')

  const refresh = useCallback(() => {
    ListProfiles().then(p => setProfiles(p as any)).catch(e => setError(String(e)))
    ListWorkspaces().then(w => setWorkspaces(w as any)).catch(e => setError(String(e)))
  }, [])
  useEffect(() => {
    refresh()
    ListTools().then(t => setTools((t as any[]).map(x => x.name).sort())).catch(() => {})
  }, [refresh])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const done = (close: () => void) => (message: string) => {
    setError(message)
    if (!message) { close(); refresh() }
  }

  return (
    <div className="overlay" role="dialog" aria-modal="true" aria-label="Workspaces and profiles">
      <div className="modal wide">
        <div className="modal-hd">
          <div><h2>Workspaces and profiles</h2><p>Both can only tighten the global permissions and limits.</p></div>
        </div>
        <div className="modal-body">
          {error && <div className="field-hint" role="alert" style={{ color: 'var(--red)' }}>{error}</div>}
          <h3>Profiles</h3>
          {profiles.map(p => (
            <div key={p.id} className="perm-row">
              <div className="perm-row-info">
                <div className="perm-tool-name">{p.name}{p.built_in ? ' · built in' : ''}</div>
                <div className="perm-tool-desc">{p.tools?.length ? p.tools.join(', ') : 'all tools'}</div>
              </div>
              <button className="btn secondary" onClick={() => setProfile(p.built_in ? { ...p, id: '', built_in: false, name: `${p.name} copy` } : p)}>
                {p.built_in ? 'Copy' : 'Edit'}
              </button>
              {!p.built_in && <button className="btn danger" onClick={() => DeleteProfile(p.id).then(done(() => setProfile(null)))}>Delete</button>}
            </div>
          ))}
          <button className="btn secondary" onClick={() => setProfile(NEW_PROFILE)}>New profile</button>
          {profile && <ProfileForm key={profile.id || profile.name} initial={profile} tools={tools} onDone={done(() => setProfile(null))} />}

          <h3>Workspaces</h3>
          {workspaces.length === 0 && <div className="field-hint">None yet. A workspace applies its rules to a directory and everything in it.</div>}
          {workspaces.map(w => (
            <div key={w.id} className="perm-row">
              <div className="perm-row-info">
                <div className="perm-tool-name">{w.name}</div>
                <div className="perm-tool-desc" title={w.path}>{w.path}</div>
              </div>
              <button className="btn secondary" onClick={() => setWorkspace(w)}>Edit</button>
              <button className="btn danger" onClick={() => DeleteWorkspace(w.id).then(done(() => setWorkspace(null)))}>Delete</button>
            </div>
          ))}
          <button className="btn secondary" onClick={() => setWorkspace(NEW_WORKSPACE)}>New workspace</button>
          {workspace && <WorkspaceForm key={workspace.id} initial={workspace} profiles={profiles} onDone={done(() => setWorkspace(null))} />}
        </div>
        <div className="modal-ft"><button className="btn secondary" onClick={onClose}>Close</button></div>
      </div>
    </div>
  )
}
