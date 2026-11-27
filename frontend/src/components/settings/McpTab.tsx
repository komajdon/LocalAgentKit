import { useState, useEffect, useCallback } from 'react'
import {
  GetMCPStatus, RestartMCPServer, ProbeMCPServer,
} from '../../../wailsjs/go/main/App'
import { Icon } from '../Icon'
import type { Dispatch, SetStateAction } from 'react'
import type { MCPServer } from '../../types'

interface Props {
  mcpServers: MCPServer[]
  setMcpServers: Dispatch<SetStateAction<MCPServer[]>>
}

export function McpTab({ mcpServers, setMcpServers }: Props) {
  // MCP UI state
  const [mcpProbeResult, setMcpProbeResult] = useState<Record<number, string>>({})
  const [mcpProbing, setMcpProbing]         = useState<Record<number, boolean>>({})
  const emptyMCP = (): MCPServer => ({ name: '', command: '', args: [], env: {}, enabled: true })

  const MCP_PRESETS: MCPServer[] = [
    {
      name: 'postgres',
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-postgres', 'postgresql://user:password@localhost:5432/dbname'],
      env: {},
      enabled: true,
    },
    {
      name: 'mongodb',
      command: 'npx',
      args: ['-y', 'mongodb-mcp-server', '--connectionString', 'mongodb://localhost:27017/mydb'],
      env: {},
      enabled: true,
    },
    {
      name: 'gdrive',
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-gdrive'],
      env: { GDRIVE_CREDENTIALS_FILE: '/path/to/credentials.json', GDRIVE_TOKEN_FILE: '/path/to/token.json' },
      enabled: true,
    },
  ]

  // Live state of the saved servers, polled while the MCP tab is open.
  const [mcpStatus, setMcpStatus] = useState<{ name: string; state: string; tools: string[]; last_error: string }[]>([])
  const [mcpRestarting, setMcpRestarting] = useState('')
  const [mcpRestartError, setMcpRestartError] = useState('')
  const refreshMcpStatus = useCallback(() => GetMCPStatus().then(s => setMcpStatus((s as any) || [])).catch(() => {}), [])
  useEffect(() => {
        setMcpRestartError('')
    refreshMcpStatus()
    const timer = window.setInterval(refreshMcpStatus, 3000)
    return () => window.clearInterval(timer)
  }, [refreshMcpStatus])
  const restartMcp = async (name: string) => {
    setMcpRestarting(name); setMcpRestartError('')
    const err = await RestartMCPServer(name)
    setMcpRestarting('')
    if (err) setMcpRestartError(`${name}: ${err}`)
    refreshMcpStatus()
  }

  return (
    <>
      {mcpStatus.length > 0 && (
        <div style={{ padding: '8px 20px 0' }} aria-label="MCP server status">
          {mcpStatus.map(s => (
            <div key={s.name} className="perm-row">
              <div className="perm-row-info">
                <div className="perm-tool-name">{s.name} · {mcpRestarting === s.name ? 'starting…' : s.state}</div>
                <div className="perm-tool-desc" title={s.last_error || s.tools.join(', ')}>
                  {s.last_error || (s.state === 'connected' ? `${s.tools.length} tools: ${s.tools.join(', ')}`
                    : s.state === 'disconnected' ? 'Starts when the agent next runs' : '')}
                </div>
              </div>
              <button className="btn secondary" disabled={s.state === 'disabled' || mcpRestarting !== ''}
                onClick={() => restartMcp(s.name)}>Restart</button>
            </div>
          ))}
          {mcpRestartError && <div role="alert" className="field-hint" style={{ color: 'var(--red)' }}>{mcpRestartError}</div>}
          <div className="field-hint">Status of the saved configuration. Save after editing a server to apply it.</div>
        </div>
      )}

      <div className="modal-body">
        <div style={{ fontSize: 12, color: 'var(--text2)', lineHeight: 1.7, marginBottom: 12 }}>
          Add MCP servers to give the agent access to external services (databases, email, APIs, etc.).<br />
          Each server runs as a subprocess and exposes tools via the <strong>Model Context Protocol</strong>.
        </div>

        <div className="mcp-prereq-banner">
          <Icon name="alert" />
          <div>
            <strong>Before using MCP servers</strong> — each server is a separate program that must be installed on your machine first.<br />
            Use <strong>Test connection</strong> on each card to verify it works before starting a conversation.
          </div>
        </div>

        <div className="mcp-presets">
          <div className="mcp-presets-label">Quick add</div>
          <div className="mcp-presets-row">
            {[
              { key: 'postgres', icon: 'database' as const, label: 'PostgreSQL', desc: 'Query & modify Postgres databases' },
              { key: 'mongodb',  icon: 'database' as const, label: 'MongoDB',    desc: 'Read & write MongoDB collections' },
              { key: 'gdrive',   icon: 'folder' as const, label: 'Google Drive', desc: 'List, read & upload Drive files' },
            ].map(p => {
              const already = mcpServers.some(s => s.name === p.key)
              return (
                <button key={p.key}
                  className={`mcp-preset-btn ${already ? 'added' : ''}`}
                  disabled={already}
                  onClick={() => {
                    const preset = MCP_PRESETS.find(x => x.name === p.key)
                    if (preset) setMcpServers(s => [...s, { ...preset }])
                  }}>
                  <Icon name={p.icon} size={18} />
                  <span className="mcp-preset-name">{p.label}</span>
                  <span className="mcp-preset-desc">{already ? '✓ Added' : p.desc}</span>
                </button>
              )
            })}
          </div>
        </div>

        {mcpServers.map((srv, i) => (
          <div key={i} className="mcp-server-card">
            <div className="mcp-server-header">
              <label className="mcp-toggle">
                <input type="checkbox" checked={srv.enabled}
                  onChange={e => setMcpServers(s => s.map((x, j) => j === i ? { ...x, enabled: e.target.checked } : x))} />
                <span>Enabled</span>
              </label>
              <button className="icon-btn danger" title="Remove server"
                aria-label="Remove server" onClick={() => setMcpServers(s => s.filter((_, j) => j !== i))}><Icon name="trash" size={14} /></button>
            </div>

            {srv.name === 'postgres' && (
              <div className="mcp-setup-hint">
                <div className="mcp-setup-title">PostgreSQL — setup required</div>
                <ol className="mcp-setup-steps">
                  <li>Install Node.js if not already: <a href="https://nodejs.org" target="_blank" rel="noreferrer">nodejs.org</a></li>
                  <li>Install the MCP server once:<br /><code>npm install -g @modelcontextprotocol/server-postgres</code></li>
                  <li>Change <strong>Command</strong> to <code>mcp-server-postgres</code> and remove the <code>-y</code> / package name from Args</li>
                  <li>Replace the connection string in Args with your actual Postgres URL</li>
                  <li>Click <strong>Test connection</strong> to verify before saving</li>
                </ol>
              </div>
            )}
            {srv.name === 'mongodb' && (
              <div className="mcp-setup-hint">
                <div className="mcp-setup-title">MongoDB — setup required</div>
                <ol className="mcp-setup-steps">
                  <li>Install Node.js if not already: <a href="https://nodejs.org" target="_blank" rel="noreferrer">nodejs.org</a></li>
                  <li>Install the MCP server once:<br /><code>npm install -g mongodb-mcp-server</code></li>
                  <li>Change <strong>Command</strong> to <code>mongodb-mcp-server</code> and clear the Args (no npx flags needed)</li>
                  <li>Add your connection string to Args: <code>--connectionString mongodb://localhost:27017/mydb</code></li>
                  <li>Click <strong>Test connection</strong> to verify before saving</li>
                </ol>
              </div>
            )}
            {srv.name === 'gdrive' && (
              <div className="mcp-setup-hint">
                <div className="mcp-setup-title">Google Drive — setup required</div>
                <ol className="mcp-setup-steps">
                  <li>Install Node.js if not already: <a href="https://nodejs.org" target="_blank" rel="noreferrer">nodejs.org</a></li>
                  <li>Install the MCP server once:<br /><code>npm install -g @modelcontextprotocol/server-gdrive</code></li>
                  <li>Create OAuth2 credentials in <a href="https://console.cloud.google.com" target="_blank" rel="noreferrer">Google Cloud Console</a> → APIs &amp; Services → Credentials</li>
                  <li>Set <code>GDRIVE_CREDENTIALS_FILE</code> in Env vars to the downloaded JSON path</li>
                  <li>Set <code>GDRIVE_TOKEN_FILE</code> to a writable path (e.g. <code>/home/you/.gdrive-token.json</code>)</li>
                  <li>Click <strong>Test connection</strong> to verify before saving</li>
                </ol>
              </div>
            )}

            <div className="field">
              <label htmlFor="name">Name</label>
              <input id="name" value={srv.name} placeholder="e.g. postgres"
                onChange={e => setMcpServers(s => s.map((x, j) => j === i ? { ...x, name: e.target.value } : x))} />
            </div>

            <div className="field">
              <label htmlFor="command">Command</label>
              <input id="command" value={srv.command} placeholder="e.g. npx or uvx or /usr/bin/my-server"
                onChange={e => setMcpServers(s => s.map((x, j) => j === i ? { ...x, command: e.target.value } : x))} />
            </div>

            <div className="field">
              <label htmlFor="arguments-one-per-line">Arguments (one per line)</label>
              <textarea id="arguments-one-per-line" rows={3}
                value={srv.args.join('\n')}
                onChange={e => setMcpServers(s => s.map((x, j) =>
                  j === i ? { ...x, args: e.target.value.split('\n').filter(Boolean) } : x))}
                placeholder={"@modelcontextprotocol/server-postgres\npostgresql://localhost/mydb"}
                style={{ resize: 'vertical', fontFamily: 'monospace', fontSize: 12 }}
              />
            </div>

            <div className="field">
              <label htmlFor="environment-variables-key-valu">Environment variables (KEY=VALUE, one per line)</label>
              <textarea id="environment-variables-key-valu" rows={3}
                value={Object.entries(srv.env || {}).map(([k, v]) => `${k}=${v}`).join('\n')}
                onChange={e => {
                  const env: Record<string, string> = {}
                  e.target.value.split('\n').filter(Boolean).forEach(line => {
                    const eq = line.indexOf('=')
                    if (eq > 0) env[line.slice(0, eq)] = line.slice(eq + 1)
                  })
                  setMcpServers(s => s.map((x, j) => j === i ? { ...x, env } : x))
                }}
                placeholder={"GMAIL_CREDENTIALS_FILE=/home/user/.gmail.json"}
                style={{ resize: 'vertical', fontFamily: 'monospace', fontSize: 12 }}
              />
            </div>

            <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 4 }}>
              <button className="btn secondary"
                disabled={mcpProbing[i]}
                onClick={async () => {
                  setMcpProbing(p => ({ ...p, [i]: true }))
                  setMcpProbeResult(r => ({ ...r, [i]: '' }))
                  try {
                    const tools = await ProbeMCPServer(srv as any) as any
                    setMcpProbeResult(r => ({ ...r, [i]: `✓ Connected — ${tools?.length || 0} tools: ${(tools || []).join(', ')}` }))
                  } catch (err: any) {
                    const msg: string = err?.message || String(err)
                    let hint = ''
                    if (msg.includes('ERR_MODULE_NOT_FOUND') || msg.includes('bson') || msg.includes('process exited')) {
                      hint = ` — package not installed or broken cache. Run: npm install -g ${srv.args[1] || srv.command} then set Command to the package binary name.`
                    } else if (msg.includes('ENOENT') || msg.includes('not found')) {
                      hint = ` — command "${srv.command}" not found. Is Node.js installed and is the package installed globally?`
                    } else if (msg.includes('ECONNREFUSED') || msg.includes('connect')) {
                      hint = ` — could not connect. Is the service (e.g. database) running?`
                    }
                    setMcpProbeResult(r => ({ ...r, [i]: `✗ ${msg}${hint}` }))
                  } finally {
                    setMcpProbing(p => ({ ...p, [i]: false }))
                  }
                }}>
                {mcpProbing[i] ? 'Testing…' : 'Test connection'}
              </button>
              {mcpProbeResult[i] && (
                <span style={{ fontSize: 12, color: mcpProbeResult[i].startsWith('✓') ? 'var(--green)' : 'var(--red)' }}>
                  {mcpProbeResult[i]}
                </span>
              )}
            </div>
          </div>
        ))}

        <button className="btn secondary" style={{ marginTop: 12 }}
          onClick={() => setMcpServers(s => [...s, emptyMCP()])}>
          + Add MCP server
        </button>
      </div>
    </>
  )
}
