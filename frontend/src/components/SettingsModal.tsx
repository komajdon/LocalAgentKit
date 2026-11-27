import { useState, useEffect } from 'react'
import {
  ListModels, SaveConfig,
} from '../../wailsjs/go/main/App'
import type { Dispatch, SetStateAction } from 'react'
import { GeneralTab, type Section } from './settings/GeneralTab'
import { McpTab } from './settings/McpTab'
import { Icon, type IconName } from './Icon'
import type { ToolInfo, MCPServer, Cfg } from '../types'

const SECTIONS: [Section | 'mcp', string, IconName][] = [
  ['model', 'Model', 'cpu'], ['agent', 'Agent', 'spark'], ['safety', 'Safety', 'shield'],
  ['mcp', 'MCP servers', 'plug'], ['data', 'Data and privacy', 'database'], ['app', 'App', 'sun'],
]

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
  const [cfgTab, setCfgTab] = useState<Section | 'mcp'>('model')
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

  const title = SECTIONS.find(([id]) => id === cfgTab)![1]
  return (
    <div className="overlay" role="dialog" aria-modal="true" aria-label="Settings">
      <div className="modal settings">
        <div className="modal-hd">
          <div><h2>Settings</h2><p>{title}</p></div>
          <button className="icon-btn" onClick={onClose} aria-label="Close settings" title="Close (Esc)"><Icon name="x" /></button>
        </div>
        <div className="settings-main">
          <nav className="settings-nav" aria-label="Settings sections">
            {SECTIONS.map(([id, label, icon]) => (
              <button key={id} className={`tab ${cfgTab === id ? 'active' : ''}`} aria-current={cfgTab === id} onClick={() => setCfgTab(id)}>
                <Icon name={icon} /><span>{label}</span>
              </button>
            ))}
          </nav>
          {cfgTab === 'mcp'
            ? <div className="settings-page"><McpTab mcpServers={mcpServers} setMcpServers={setMcpServers} /></div>
            : <GeneralTab section={cfgTab} toolList={toolList} cfg={cfg} setCfg={setCfg} models={models} setModels={setModels}
                whisperReady={whisperReady} appVersion={appVersion} onImported={onImported} />}
        </div>
        <div className="modal-ft">
          <button className="btn" onClick={onClose} disabled={savingCfg}>Cancel</button>
          <button className="btn primary" onClick={saveCfg} disabled={savingCfg}>{savingCfg ? 'Saving…' : 'Save'}</button>
        </div>
      </div>
    </div>
  )
}
