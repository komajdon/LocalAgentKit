import { useEffect } from 'react'
import type { PermReq } from '../types'
import { DiffView } from './DiffView'
import { Icon, toolIcon } from './Icon'

export type PermissionDecision = 'once' | 'session' | 'workspace' | 'deny'

interface Props {
  request: PermReq
  onAnswer: (decision: PermissionDecision) => void
}

const shown = (raw: unknown) => (typeof raw === 'string' ? raw : JSON.stringify(raw, null, 2))

/**
 * Shows exactly what is being asked before the agent may do it. The change
 * comes first; Deny has the focus and Escape denies, so nothing is allowed by
 * a stray key.
 */
export function PermissionDialog({ request, onAnswer }: Props) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onAnswer('deny') }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onAnswer])
  const args = Object.entries(request.args)
  return (
    <div className="overlay" role="dialog" aria-modal="true" aria-label="Permission request">
      <div className="modal approval">
        <div className="modal-hd">
          <div className={`modal-ico ${request.sensitive ? 'warn' : ''}`}><Icon name={toolIcon(request.tool)} size={18} /></div>
          <div>
            <h2>{request.operation}?</h2>
            <p><code>{request.tool}</code> · {request.description}{request.sensitive ? ' · sensitive' : ''}</p>
          </div>
        </div>
        <div className="modal-body">
          {request.target && <pre className="approval-target">{request.target}</pre>}
          {request.preview && <DiffView diff={request.preview} />}
          <div className="approval-impact"><Icon name="alert" /><span>{request.impact}</span></div>
          {args.length > 0 && (
            // Open unless a diff already shows what the arguments would do.
            <details open={!request.preview}>
              <summary>All arguments</summary>
              <div className="perm-card">
                {args.map(([k, raw]) => (
                  <div key={k} className="arg-line">
                    <span className="arg-k">{k}</span>
                    <pre className="arg-shell-cmd">{shown(raw)}</pre>
                  </div>
                ))}
              </div>
            </details>
          )}
          <dl className="approval-facts">
            <dt>Workspace</dt><dd>{request.workspace}</dd>
            <dt>Remembering covers</dt><dd>{request.scope}</dd>
          </dl>
        </div>
        <div className="modal-ft">
          <button className="btn danger push-left" autoFocus onClick={() => onAnswer('deny')}>Deny</button>
          <button className="btn" onClick={() => onAnswer('session')}>Allow this session</button>
          <button className="btn" onClick={() => onAnswer('workspace')}>Always in this workspace</button>
          <button className="btn primary" onClick={() => onAnswer('once')}>Allow once</button>
        </div>
      </div>
    </div>
  )
}
