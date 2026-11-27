import { Icon } from './Icon'
import { useEffect, useMemo, useState } from 'react'
import { GetToolTimeline } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export interface ToolRecord {
  id: string; run_id: string; tool: string; status: string; permission?: string
  started: string; millis: number; args: string; result: string
}

const STATUSES = ['running', 'succeeded', 'failed', 'denied', 'canceled', 'rejected']

/** Replaces the record with the same id, or appends it. */
export function upsert(list: ToolRecord[], record: ToolRecord) {
  const i = list.findIndex(r => r.id === record.id)
  return i < 0 ? [...list, record] : list.map((r, j) => (j === i ? record : r))
}

export const duration = (ms: number) => (ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`)

export function ToolTimeline({ conversationId, onClose }: { conversationId: string; onClose: () => void }) {
  const [records, setRecords] = useState<ToolRecord[] | null>(null)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const [tool, setTool] = useState('')

  useEffect(() => {
    let current = true
    setRecords(null); setError('')
    // Subscribe first so nothing is lost while the stored records load.
    let live: ToolRecord[] = []
    const off = EventsOn('chat:tool_event', (e: { conversation_id: string; data: ToolRecord }) => {
      if (e.conversation_id !== conversationId) return
      live = upsert(live, e.data)
      setRecords(list => (list ? upsert(list, e.data) : list))
    })
    GetToolTimeline(conversationId)
      .then(stored => { if (current) setRecords(live.reduce(upsert, (stored ?? []) as ToolRecord[])) })
      .catch(e => { if (current) setError(String(e)) })
    return () => { current = false; off() }
  }, [conversationId])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const tools = useMemo(() => [...new Set((records ?? []).map(r => r.tool))].sort(), [records])
  const shown = (records ?? []).filter(r => (!status || r.status === status) && (!tool || r.tool === tool)).slice().reverse()

  return (
    <aside className="drawer" role="dialog" aria-label="Tool timeline">
        <div className="modal-hd">
          <div><h2>Tool timeline</h2><p>Newest first. Sensitive values are redacted.</p></div>
          <button className="icon-btn" onClick={onClose} aria-label="Close panel" title="Close (Esc)"><Icon name="x" /></button>
        </div>
        <div className="modal-body">
          <div className="usage-filters">
            <label>Status
              <select value={status} onChange={e => setStatus(e.target.value)}>
                <option value="">All</option>
                {STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </label>
            <label>Tool
              <select value={tool} onChange={e => setTool(e.target.value)}>
                <option value="">All</option>
                {tools.map(t => <option key={t} value={t}>{t}</option>)}
              </select>
            </label>
          </div>
          {error && <div className="field-hint" role="alert">{error}</div>}
          {!records && !error && <div className="field-hint" role="status">Loading…</div>}
          {records && shown.length === 0 && <div className="field-hint">No tool calls to show.</div>}
          {shown.map(r => (
            <details key={r.id} className="timeline-row">
              <summary>
                <strong>{r.tool}</strong>
                <span className={`timeline-status ${r.status}`}>{r.status}</span>
                <span>{r.status === 'running' ? '…' : duration(r.millis)}</span>
                <span>{r.started.startsWith('0001') ? '' : new Date(r.started).toLocaleString()}</span>
              </summary>
              {r.permission && <div className="field-hint">Permission: {r.permission}</div>}
              <div className="field-hint">Arguments</div>
              <pre className="arg-shell-cmd">{r.args || '(none)'}</pre>
              {r.result && <><div className="field-hint">{r.status === 'succeeded' ? 'Result' : 'Error or result'}</div><pre className="arg-shell-cmd">{r.result}</pre></>}
            </details>
          ))}
        </div>
    </aside>
  )
}
