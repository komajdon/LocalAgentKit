import type { PermReq } from '../types'
import { DiffView } from './DiffView'

export type PermissionDecision = 'once' | 'session' | 'workspace' | 'deny'

interface Props {
  request: PermReq
  onAnswer: (decision: PermissionDecision) => void
}

/** Shows exactly what is being asked before the agent may do it. */
export function PermissionDialog({ request, onAnswer }: Props) {
  return (
    <div className="overlay" role="dialog" aria-modal="true" aria-label="Permission request">
      <div className="modal">
        <div className="modal-hd">
          <div className="modal-ico ico-perm">🔐</div>
          <div>
            <h2>Permission Request</h2>
            <p>{request.operation}{request.sensitive ? ' — sensitive action' : ''}</p>
          </div>
        </div>
        <div className="modal-body">
          <div className="perm-card">
            <div className="perm-kv">
              <div className="perm-key">Tool</div>
              <div className="perm-val"><strong style={{ fontFamily: 'monospace' }}>{request.tool}</strong></div>
            </div>
            <div className="perm-kv">
              <div className="perm-key">What it does</div>
              <div className="perm-val">{request.description}</div>
            </div>
            {request.target && (
              <div className="perm-kv">
                <div className="perm-key">Target</div>
                <pre className="arg-shell-cmd">{request.target}</pre>
              </div>
            )}
            {request.preview && (
              <div className="perm-kv">
                <div className="perm-key">Change to be made</div>
                <DiffView diff={request.preview} />
              </div>
            )}
            <div className="perm-kv">
              <div className="perm-key">Potential impact</div>
              <div className="perm-val">{request.impact}</div>
            </div>
            <div className="perm-kv">
              <div className="perm-key">Workspace</div>
              <div className="perm-val">{request.workspace}</div>
            </div>
            <div className="perm-kv">
              <div className="perm-key">Remembering an approval covers</div>
              <div className="perm-val">{request.scope}</div>
            </div>
            {Object.keys(request.args).length > 0 && (
              <div className="perm-kv">
                <div className="perm-key">Arguments</div>
                <div>
                  {Object.entries(request.args).map(([k, raw]) => [k, typeof raw === 'string' ? raw : JSON.stringify(raw, null, 2)]).map(([k, v]) => (
                    <div key={k} className="arg-line">
                      <span className="arg-k">{k}</span>
                      <span className="arg-sep">: </span>
                      {request.tool === 'shell' && k === 'command'
                        ? <pre className="arg-shell-cmd">{v}</pre>
                        : <span>{v}</span>}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
          <div style={{ fontSize: 12, color: 'var(--text3)', lineHeight: 1.6 }}>
            Tip: go to <strong style={{ color: 'var(--text2)' }}>Settings → Permissions</strong> to always allow or deny specific tools without being asked.
          </div>
        </div>
        <div className="modal-ft">
          <button className="btn danger" onClick={() => onAnswer('deny')}>✕ Deny</button>
          <button className="btn secondary" onClick={() => onAnswer('session')}>Allow this session</button>
          <button className="btn secondary" onClick={() => onAnswer('workspace')}>Always in this workspace</button>
          <button className="btn primary" onClick={() => onAnswer('once')}>✓ Allow once</button>
        </div>
      </div>
    </div>
  )
}
