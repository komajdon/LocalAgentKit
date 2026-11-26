import { useState, useEffect } from 'react'
import {
  ListPermissionGrants, RevokePermissionGrant, RevokeAllPermissionGrants,
} from '../../../wailsjs/go/main/App'
import type { Dispatch, SetStateAction } from 'react'
import type { Grant, ToolInfo, PermMode, Cfg } from '../../types'

interface Props {
  cfg: Cfg
  setCfg: Dispatch<SetStateAction<Cfg>>
  toolList: ToolInfo[]
}

export function PermissionsTab({ cfg, setCfg, toolList }: Props) {
  // Remembered approvals, listed in Settings so they can be revoked.
  const [grants, setGrants] = useState<Grant[]>([])
  const refreshGrants = () => ListPermissionGrants().then(g => setGrants((g as any) || [])).catch(() => setGrants([]))
  useEffect(() => {
    refreshGrants()
  }, [])
  const setToolPerm = (tool: string, perm: PermMode) =>
    setCfg(c => ({ ...c, tool_permissions: { ...c.tool_permissions, [tool]: perm } }))

  return (
      <div className="modal-body">
        <div style={{ fontSize: 12, color: 'var(--text2)', lineHeight: 1.7 }}>
          Set a default permission for each tool. <strong style={{ color: 'var(--text)' }}>Always Ask</strong> prompts you each time,{' '}
          <strong style={{ color: 'var(--green)' }}>Always Allow</strong> skips the prompt,{' '}
          <strong style={{ color: 'var(--red)' }}>Always Deny</strong> blocks the tool silently.
        </div>
        <div className="perm-list">
          {toolList.map(t => {
            const perm = (cfg.tool_permissions?.[t.name] || 'ask') as PermMode
            return (
              <div key={t.name} className="perm-row">
                <div className="perm-row-info">
                  <div className="perm-tool-name">{t.name}</div>
                  <div className="perm-tool-desc">{t.description}</div>
                </div>
                <select
                  className={`perm-sel ${perm}`}
                  value={perm}
                  onChange={e => setToolPerm(t.name, e.target.value as PermMode)}
                >
                  <option value="ask">Always Ask</option>
                  <option value="allow">Always Allow</option>
                  <option value="deny">Always Deny</option>
                </select>
              </div>
            )
          })}
        </div>
        <div className="field">
          <label>Remembered approvals</label>
          {grants.length === 0 && <div className="field-hint">None.</div>}
          {grants.map(g => (
            <div key={g.id} className="perm-row">
              <div className="perm-row-info">
                <div className="perm-tool-name">{g.tool} · {g.scope === 'session' ? 'this session' : 'workspace'}</div>
                <div className="perm-tool-desc" title={`${g.workspace}: ${g.label}`}>{g.workspace}: {g.label}</div>
              </div>
              <button className="btn secondary" onClick={() => RevokePermissionGrant(g.id).then(refreshGrants)}>Revoke</button>
            </div>
          ))}
          {grants.length > 0 && (
            <button className="btn danger" onClick={() => RevokeAllPermissionGrants().then(refreshGrants)}>Revoke all</button>
          )}
        </div>
      </div>
  )
}
