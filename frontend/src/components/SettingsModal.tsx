import { useState, useEffect } from 'react'
import {
  ListModels, SaveConfig,
} from '../../wailsjs/go/main/App'
import type { Dispatch, SetStateAction } from 'react'
import { GeneralTab } from './settings/GeneralTab'
import { McpTab } from './settings/McpTab'
import { PermissionsTab } from './settings/PermissionsTab'
import type { ToolInfo, MCPServer, Cfg } from '../types'

interface Props {
  cfg: Cfg
  setCfg: Dispatch<SetStateAction<Cfg>>
  models: string[]
  setModels: (models: string[]) => void
  toolList: ToolInfo[]
  mcpServers: MCPServer[]
  setMcpServers: Dispatch<SetStateAction<MCPServer[]>>
  whisperReady: boolean
  appVersion: string
  onImported: () => void
  onClose: () => void
}

export function SettingsModal({ cfg, setCfg, models, setModels, toolList, mcpServers, setMcpServers, whisperReady, appVersion, onImported, onClose }: Props) {
  const [cfgTab, setCfgTab] = useState<'general' | 'permissions' | 'mcp'>('general')
  const [savingCfg, setSavingCfg] = useState(false)

  const saveCfg = async () => {
    setSavingCfg(true)
    try {
      await SaveConfig({ ...cfg, mcp_servers: mcpServers } as any)
      const r = await ListModels() as any
      if (r.models) setModels(r.models)
      onClose()
    } finally {
      setSavingCfg(false)
    }
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="overlay" role="dialog" aria-modal="true" aria-label="Settings">
      <div className="modal">
        <div className="modal-hd">
          <div className="modal-ico ico-cfg">⚙️</div>
          <div><h2>Settings</h2></div>
        </div>

        <div style={{ padding: '0 22px 14px', flexShrink: 0 }}>
          <div className="tabs">
            <button className={`tab ${cfgTab === 'general' ? 'active' : ''}`} onClick={() => setCfgTab('general')}>General</button>
            <button className={`tab ${cfgTab === 'permissions' ? 'active' : ''}`} onClick={() => setCfgTab('permissions')}>Permissions</button>
            <button className={`tab ${cfgTab === 'mcp' ? 'active' : ''}`} onClick={() => setCfgTab('mcp')}>MCP Servers</button>
          </div>
        </div>

        {cfgTab === 'mcp' ? (
          <McpTab mcpServers={mcpServers} setMcpServers={setMcpServers} />
        ) : cfgTab === 'general' ? (
          <GeneralTab cfg={cfg} setCfg={setCfg} models={models} setModels={setModels}
            whisperReady={whisperReady} appVersion={appVersion} onImported={onImported} />
        ) : (
          <PermissionsTab cfg={cfg} setCfg={setCfg} toolList={toolList} />
        )}

            <div className="modal-ft">
              <button className="btn secondary" onClick={onClose} disabled={savingCfg}>Cancel</button>
              <button className="btn primary" onClick={saveCfg} disabled={savingCfg}>{savingCfg ? 'Saving…' : 'Save'}</button>
            </div>
          </div>
        </div>
  )
}
