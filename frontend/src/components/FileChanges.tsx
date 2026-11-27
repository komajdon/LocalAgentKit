import { Icon } from './Icon'
import { useCallback, useEffect, useState } from 'react'
import { GetGitStatus, ListFileChanges, UndoFileChange } from '../../wailsjs/go/main/App'
import { DiffView } from './DiffView'

export interface FileChange {
  id: string; at: string; path: string; kind: string; diff: string; undoable: boolean; undone: boolean
}
interface GitStatus { repository: boolean; lines: string[]; error: string }

export function FileChanges({ conversationId, onClose }: { conversationId: string; onClose: () => void }) {
  const [changes, setChanges] = useState<FileChange[] | null>(null)
  const [git, setGit] = useState<GitStatus | null>(null)
  const [error, setError] = useState('')

  const refresh = useCallback(() => {
    ListFileChanges(conversationId).then(c => setChanges((c ?? []) as any)).catch(e => setError(String(e)))
    GetGitStatus(conversationId).then(s => setGit(s as any)).catch(() => setGit(null))
  }, [conversationId])
  useEffect(refresh, [refresh])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const undo = async (id: string) => {
    setError(await UndoFileChange(id))
    refresh()
  }

  return (
    <aside className="drawer" role="dialog" aria-label="File changes">
        <div className="modal-hd">
          <div><h2>File changes</h2><p>By the write and delete tools. Shell commands are not tracked.</p></div>
          <button className="icon-btn" onClick={onClose} aria-label="Close panel" title="Close (Esc)"><Icon name="x" /></button>
        </div>
        <div className="modal-body">
          {git?.repository && (
            <details className="timeline-row">
              <summary><strong>Git status</strong><span>{Math.max(0, git.lines.length - 1)} changed paths</span></summary>
              <pre className="arg-shell-cmd">{git.lines.join('\n')}</pre>
            </details>
          )}
          {git?.error && <div className="field-hint">Git status unavailable: {git.error}</div>}
          {error && <div className="field-hint" role="alert" style={{ color: 'var(--red)' }}>{error}</div>}
          {!changes && <div className="field-hint" role="status">Loading…</div>}
          {changes?.length === 0 && <div className="field-hint">No file changes in this conversation.</div>}
          {changes?.map(c => (
            <details key={c.id} className="timeline-row">
              <summary>
                <span className={`timeline-status ${c.kind === 'deleted' ? 'failed' : 'succeeded'}`}>{c.kind}</span>
                <strong>{c.path}</strong>
                <span>{new Date(c.at).toLocaleString()}</span>
                {c.undone && <span>undone</span>}
              </summary>
              <DiffView diff={c.diff} />
              {c.undoable && <button className="btn secondary" onClick={() => undo(c.id)}>Undo this change</button>}
            </details>
          ))}
        </div>
    </aside>
  )
}
