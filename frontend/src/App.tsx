import { useState, useEffect, useRef, useCallback } from 'react'
import { Markdown } from './Markdown'
import './style.css'
import {
  SendMessage, StopAgent, RespondPermission,
  ListConversations, NewConversation, LoadConversation,
  DeleteConversation, UpdateConversationPath, RenameConversation,
  SearchConversations, ExportConversation, SetConversationModel,
  ListModels, ListTools, GetConfig, SaveConfig, PickDirectory,
  WhisperAvailable, StartRecording, StopRecording,
  ListMCPServers, SaveMCPServers, ProbeMCPServer,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

// ── Types ─────────────────────────────────────────────────────────────────

interface ConvMeta {
  id: string; title: string; work_dir: string; model?: string
  created_at: string; updated_at: string
}
interface SavedConv extends ConvMeta {
  messages: { role: string; content: string }[]
  display_items?: any[]
}
interface PermReq { tool: string; description: string; args: Record<string, string> }
interface ToolInfo { name: string; description: string }

type MsgRole = 'user' | 'assistant'
interface ChatMsg { id: number; role: MsgRole; text: string; streaming?: boolean }
interface ToolEvt { id: number; kind: 'call' | 'result'; tool: string; body: string }
type Item =
  | { type: 'msg';   data: ChatMsg }
  | { type: 'tool';  data: ToolEvt }
  | { type: 'error'; id: number; text: string }

type PermMode = 'ask' | 'allow' | 'deny'

interface MCPServer {
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  enabled: boolean
}

interface Cfg {
  provider: string; base_url: string; api_key: string
  model: string; work_dir: string
  tool_permissions: Record<string, PermMode>
  whisper_model: string
  system_prompt: string
  context_limit: number
  mcp_servers: MCPServer[]
}

interface ContextUsage { used: number; limit: number }

let seq = 0
const uid = () => ++seq

function timeAgo(iso: string) {
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

function shortPath(p: string) {
  if (!p) return '~'
  const m = p.match(/^\/home\/[^/]+/)
  if (m) return '~' + p.slice(m[0].length) || '~'
  return p.length > 26 ? '…' + p.slice(-24) : p
}

function groupConvs(list: ConvMeta[]) {
  const today: ConvMeta[] = [], week: ConvMeta[] = [], older: ConvMeta[] = []
  const now = Date.now()
  list.forEach(c => {
    const age = now - new Date(c.updated_at).getTime()
    if (age < 86400_000) today.push(c)
    else if (age < 604800_000) week.push(c)
    else older.push(c)
  })
  return { today, week, older }
}


// ── App ───────────────────────────────────────────────────────────────────

export default function App() {
  const [convList, setConvList]       = useState<ConvMeta[]>([])
  const [activeId, setActiveId]       = useState<string | null>(null)
  const [activeTitle, setActiveTitle] = useState('')
  const [activePath, setActivePath]   = useState('')

  const [items, setItems]     = useState<Item[]>([])
  const [input, setInput]     = useState('')
  const [running, setRunning] = useState(false)
  const [permReq, setPermReq] = useState<PermReq | null>(null)
  const streamIdRef = useRef<number | null>(null)

  const [showNew, setShowNew]     = useState(false)
  const [showCfg, setShowCfg]     = useState(false)
  const [newPath, setNewPath]     = useState('')
  const [cfgTab, setCfgTab]       = useState<'general' | 'permissions' | 'mcp'>('general')

  // rename state
  const [renamingId, setRenamingId]       = useState<string | null>(null)
  const [renameValue, setRenameValue]     = useState('')
  const renameInputRef                    = useRef<HTMLInputElement>(null)

  // copy feedback
  const [copiedId, setCopiedId] = useState<number | null>(null)

  // speech-to-text
  const [whisperReady, setWhisperReady]       = useState(false)
  const [recording, setRecording]             = useState(false)
  const [transcribing, setTranscribing]       = useState(false)
  const [whisperStatus, setWhisperStatus]     = useState('') // download progress label
  const [sttError, setSttError]               = useState('')

  // context usage
  const [ctxUsage, setCtxUsage] = useState<ContextUsage | null>(null)
  const [fatalError, setFatalError] = useState('')

  // search
  const [searchQuery, setSearchQuery]   = useState('')
  const [searchResults, setSearchResults] = useState<ConvMeta[] | null>(null)

  // per-conversation model selector
  const [convModel, setConvModel] = useState('')

  const [models, setModels]     = useState<string[]>([])
  const [toolList, setToolList] = useState<ToolInfo[]>([])
  const [cfg, setCfg]           = useState<Cfg>({
    provider: 'ollama', base_url: 'http://localhost:11434',
    api_key: '', model: '', work_dir: '',
    tool_permissions: {}, whisper_model: 'base',
    system_prompt: '', context_limit: 8192,
    mcp_servers: [],
  })

  // MCP UI state
  const [mcpServers, setMcpServers] = useState<MCPServer[]>([])
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

  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [items])

  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [renamingId])

  // ── Boot ────────────────────────────────────────────────

  useEffect(() => {
    GetConfig().then(c => setCfg(c as any)).catch(() => {/* use defaults */})
    ListModels().then((r: any) => r.models && setModels(r.models)).catch(() => {})
    ListTools().then((t: any) => setToolList(t || [])).catch(() => {})
    WhisperAvailable().then(ok => setWhisperReady(ok)).catch(() => {})
    ListMCPServers().then((s: any) => setMcpServers(s || [])).catch(() => {})
    refreshList()
  }, [])

  // ── Events ──────────────────────────────────────────────

  useEffect(() => {
    const offs: (() => void)[] = []

    offs.push(EventsOn('chat:chunk', (chunk: string) => {
      if (streamIdRef.current !== null) {
        // Append to the existing streaming message — no side-effects needed.
        const sid = streamIdRef.current
        setItems(prev => prev.map(it =>
          it.type === 'msg' && it.data.id === sid
            ? { ...it, data: { ...it.data, text: it.data.text + chunk } }
            : it
        ))
      } else {
        // First chunk of a new assistant turn — allocate the ID *before* setState
        // so the ref and the array item always agree, even if React re-runs the updater.
        const id = uid()
        streamIdRef.current = id
        setItems(prev => [...prev, { type: 'msg', data: { id, role: 'assistant', text: chunk, streaming: true } }])
      }
    }))

    offs.push(EventsOn('chat:done', () => {
      setRunning(false)
      setItems(prev => prev.map(it =>
        it.type === 'msg' && it.data.id === streamIdRef.current
          ? { ...it, data: { ...it.data, streaming: false } }
          : it
      ))
      streamIdRef.current = null
      refreshList()
    }))

    offs.push(EventsOn('chat:stopped', () => {
      setRunning(false)
      const sid = streamIdRef.current
      streamIdRef.current = null
      setItems(prev => prev.map(it =>
        it.type === 'msg' && it.data.id === sid
          ? { ...it, data: { ...it.data, text: it.data.text + '\n\n— stopped by user', streaming: false } }
          : it
      ))
    }))

    offs.push(EventsOn('chat:error', (msg: string) => {
      setRunning(false)
      streamIdRef.current = null
      setItems(prev => [...prev, { type: 'error', id: uid(), text: msg }])
    }))

    offs.push(EventsOn('chat:tool_call', (d: { tool: string; args: string }) => {
      const sid = streamIdRef.current
      streamIdRef.current = null
      const toolId = uid() // allocate outside setState — never call uid() inside an updater
      setItems(prev => {
        let next = prev
        if (sid !== null) {
          next = prev.map(it => {
            if (it.type !== 'msg' || it.data.id !== sid) return it
            const cleaned = it.data.text
              .split('\n')
              .filter(l => !l.trimStart().startsWith('TOOL_CALL:'))
              .join('\n')
              .trim()
            if (!cleaned) return null as any
            return { ...it, data: { ...it.data, text: cleaned, streaming: false } }
          }).filter(Boolean)
        }
        return [...next, { type: 'tool', data: { id: toolId, kind: 'call', tool: d.tool, body: d.args } }]
      })
    }))

    offs.push(EventsOn('chat:tool_result', (d: { tool: string; result: string }) => {
      const resultId = uid()
      setItems(prev => [...prev, { type: 'tool', data: { id: resultId, kind: 'result', tool: d.tool, body: d.result } }])
    }))

    offs.push(EventsOn('chat:permission_request', (req: PermReq) => setPermReq(req)))

    offs.push(EventsOn('chat:context_usage', (u: ContextUsage) => setCtxUsage(u)))

    offs.push(EventsOn('app:fatal', (msg: string) => setFatalError(msg)))

    offs.push(EventsOn('whisper:downloading', (model: string) => {
      setWhisperStatus(`Downloading whisper ${model} model…`)
    }))
    offs.push(EventsOn('whisper:ready', () => {
      setWhisperStatus('')
      setWhisperReady(true)
    }))
    offs.push(EventsOn('whisper:download_error', (msg: string) => {
      setWhisperStatus('')
      setSttError(`Model download failed: ${msg}`)
    }))

    return () => offs.forEach(o => o())
  }, [])

  // ── Helpers ─────────────────────────────────────────────

  const refreshList = () => ListConversations().then((l: any) => setConvList(l || []))

  const openConv = async (id: string) => {
    const conv = await LoadConversation(id) as SavedConv
    setActiveId(conv.id); setActiveTitle(conv.title); setActivePath(conv.work_dir)
    setConvModel((conv as any).model || '')
    setCtxUsage(null)
    setRunning(false); streamIdRef.current = null

    if (conv.display_items?.length) {
      setItems(conv.display_items.map((it: any) => ({
        ...it,
        data: { ...it.data, id: uid(), streaming: false },
      })))
    } else {
      setItems(conv.messages
        .filter(m => (m.role === 'user' || m.role === 'assistant') && !m.content.startsWith('TOOL_RESULT'))
        .map(m => ({ type: 'msg' as const, data: { id: uid(), role: m.role as MsgRole, text: m.content, streaming: false } }))
      )
    }
  }

  const createConv = async () => {
    const conv = await NewConversation(newPath) as SavedConv
    setNewPath(''); setShowNew(false)
    await refreshList(); await openConv(conv.id)
  }

  const delConv = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    await DeleteConversation(id)
    if (activeId === id) { setActiveId(null); setItems([]); setActiveTitle(''); setActivePath('') }
    refreshList()
  }

  const startRename = (e: React.MouseEvent, c: ConvMeta) => {
    e.stopPropagation()
    setRenamingId(c.id)
    setRenameValue(c.title)
  }

  const commitRename = async () => {
    if (!renamingId || !renameValue.trim()) { setRenamingId(null); return }
    await RenameConversation(renamingId, renameValue.trim())
    if (activeId === renamingId) setActiveTitle(renameValue.trim())
    setRenamingId(null)
    refreshList()
  }

  const changePath = async () => {
    const dir = await PickDirectory()
    if (!dir) return
    await UpdateConversationPath(dir)
    setActivePath(dir)
    setConvList(prev => prev.map(c => c.id === activeId ? { ...c, work_dir: dir } : c))
  }

  const send = useCallback(() => {
    const text = input.trim()
    if (!text || running || !activeId) return
    setItems(prev => [...prev, { type: 'msg', data: { id: uid(), role: 'user', text } }])
    setInput(''); setRunning(true); streamIdRef.current = null
    SendMessage(text)
  }, [input, running, activeId])

  const handleKey = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
  }

  const autoResize = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value)
    const t = e.target; t.style.height = 'auto'
    t.style.height = Math.min(t.scrollHeight, 160) + 'px'
  }

  const copyMessage = (id: number, text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 2000)
    }).catch(() => {
      // Clipboard access denied — silently ignore, copy button stays in default state.
    })
  }

  const startRecording = async () => {
    setSttError('')
    try {
      await StartRecording()
      setRecording(true)
    } catch (err: any) {
      setSttError(`Could not start recording: ${err?.message ?? err}`)
    }
  }

  const stopRecording = async () => {
    setRecording(false)
    setTranscribing(true)
    try {
      const text: string = await StopRecording()
      if (text) setInput(prev => (prev ? prev + ' ' : '') + text)
    } catch (err: any) {
      setSttError(`${err?.message ?? err}`)
    } finally {
      setTranscribing(false)
    }
  }

  const runSearch = async (q: string) => {
    setSearchQuery(q)
    if (!q.trim()) { setSearchResults(null); return }
    const r = await SearchConversations(q) as ConvMeta[]
    setSearchResults(r || [])
  }

  const exportConv = async (format: 'markdown' | 'text') => {
    if (!activeId) return
    const text = await ExportConversation(activeId, format) as string
    const blob = new Blob([text], { type: format === 'markdown' ? 'text/markdown' : 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = `${activeTitle || 'conversation'}.${format === 'markdown' ? 'md' : 'txt'}`
    a.click(); URL.revokeObjectURL(url)
  }

  const changeConvModel = async (model: string) => {
    setConvModel(model)
    await SetConversationModel(model)
  }

  const saveCfg = async () => {
    await SaveConfig({ ...cfg, mcp_servers: mcpServers } as any)
    await SaveMCPServers(mcpServers as any)
    const r = await ListModels() as any
    if (r.models) setModels(r.models)
    setShowCfg(false)
  }

  const setToolPerm = (tool: string, perm: PermMode) =>
    setCfg(c => ({ ...c, tool_permissions: { ...c.tool_permissions, [tool]: perm } }))

  // ── Render ───────────────────────────────────────────────

  const groups = groupConvs(convList)

  const ConvGroup = ({ label, list }: { label: string; list: ConvMeta[] }) =>
    list.length === 0 ? null : (
      <div className="conv-group">
        <div className="conv-group-label">{label}</div>
        {list.map(c => (
          <div key={c.id} className={`conv-row ${c.id === activeId ? 'active' : ''}`}
            onClick={() => c.id !== activeId && openConv(c.id)}>
            <div className="conv-row-body">
              {renamingId === c.id ? (
                <input
                  ref={renameInputRef}
                  className="conv-rename-input"
                  value={renameValue}
                  onChange={e => setRenameValue(e.target.value)}
                  onBlur={commitRename}
                  onKeyDown={e => {
                    if (e.key === 'Enter') commitRename()
                    if (e.key === 'Escape') setRenamingId(null)
                    e.stopPropagation()
                  }}
                  onClick={e => e.stopPropagation()}
                />
              ) : (
                <div className="conv-name" onDoubleClick={e => startRename(e, c)} title="Double-click to rename">
                  {c.title}
                </div>
              )}
              <div className="conv-info">
                <span>📁 {shortPath(c.work_dir)}</span>
                <span style={{ marginLeft: 'auto' }}>{timeAgo(c.updated_at)}</span>
              </div>
            </div>
            <button className="conv-del" onClick={e => delConv(e, c.id)} title="Delete">✕</button>
          </div>
        ))}
      </div>
    )

  return (
    <div className="layout">

      {fatalError && (
        <div className="fatal-banner">
          <strong>Startup error:</strong> {fatalError}
          <button onClick={() => setFatalError('')}>✕</button>
        </div>
      )}

      {/* ── Sidebar ── */}
      <aside className="sidebar">
        <div className="sidebar-head">
          <div className="brand">
            <div className="brand-icon">🤖</div>
            <div>
              <div className="brand-name">AI Agent</div>
              <div className="brand-sub">Powered by Komajdon</div>
            </div>
          </div>
          <button className="new-btn" onClick={() => setShowNew(true)}>＋ New conversation</button>
        </div>

        <div className="sidebar-search">
          <input
            className="search-input"
            placeholder="Search conversations…"
            value={searchQuery}
            onChange={e => runSearch(e.target.value)}
          />
          {searchQuery && <button className="search-clear" onClick={() => runSearch('')}>✕</button>}
        </div>

        <div className="conv-list">
          {searchResults !== null ? (
            searchResults.length === 0
              ? <div className="conv-empty">No results for "{searchQuery}"</div>
              : <ConvGroup label={`Results (${searchResults.length})`} list={searchResults} />
          ) : convList.length === 0
            ? <div className="conv-empty">No conversations yet.<br />Click "+ New conversation" to start.</div>
            : <>
                <ConvGroup label="Today" list={groups.today} />
                <ConvGroup label="This week" list={groups.week} />
                <ConvGroup label="Older" list={groups.older} />
              </>
          }
        </div>

        <div className="sidebar-foot">
          <button className="foot-btn" onClick={() => { setShowCfg(true); setCfgTab('general') }}>
            ⚙️ Settings
          </button>
          <div className="conn-badge">
            <div className="dot" />
            <span>{cfg.model || 'No model selected'}</span>
          </div>
        </div>
      </aside>

      {/* ── Main ── */}
      <div className="main">
        {activeId ? (
          <>
            <div className="topbar">
              <div className="topbar-title">{activeTitle || 'Conversation'}</div>

              {/* per-conversation model selector */}
              <select
                className="topbar-model-sel"
                value={convModel || cfg.model}
                onChange={e => changeConvModel(e.target.value)}
                title="Model for this conversation"
              >
                {models.length > 0
                  ? (() => {
                      const freeModels = models.filter(m => m.endsWith(':free'))
                      const paidModels = models.filter(m => !m.endsWith(':free'))
                      if (freeModels.length > 0 && paidModels.length > 0) {
                        return (
                          <>
                            <optgroup label="── Free ──">
                              {freeModels.map(m => <option key={m} value={m}>{m}</option>)}
                            </optgroup>
                            <optgroup label="── Paid ──">
                              {paidModels.map(m => <option key={m} value={m}>{m}</option>)}
                            </optgroup>
                          </>
                        )
                      }
                      return models.map(m => <option key={m} value={m}>{m}</option>)
                    })()
                  : <option value={convModel || cfg.model}>{convModel || cfg.model}</option>
                }
              </select>

              {/* context usage bar */}
              {ctxUsage && (() => {
                const pct = Math.min(100, Math.round(ctxUsage.used / ctxUsage.limit * 100))
                const warn = pct >= 80
                return (
                  <div className={`ctx-bar ${warn ? 'warn' : ''}`} title={`Context: ~${ctxUsage.used} / ${ctxUsage.limit} tokens`}>
                    <div className="ctx-fill" style={{ width: pct + '%' }} />
                    <span className="ctx-label">{pct}%</span>
                  </div>
                )
              })()}

              {/* export menu */}
              <div className="export-menu">
                <button className="topbar-btn" title="Export conversation">⬇</button>
                <div className="export-dropdown">
                  <button onClick={() => exportConv('markdown')}>Export as Markdown</button>
                  <button onClick={() => exportConv('text')}>Export as Plain Text</button>
                </div>
              </div>

              <div className="path-chip" onClick={changePath} title="Click to change working directory">
                📁 <span>{shortPath(activePath)}</span>
              </div>
            </div>

            <div className="messages">
              {items.length === 0 && (
                <div className="empty-state">
                  <div className="empty-icon">🤖</div>
                  <h2>Ready to help</h2>
                  <p>Ask me anything. I can read/write files, run shell commands, and call external APIs — with your permission.</p>
                </div>
              )}

              {items.map(item => {
                if (item.type === 'error')
                  return <div key={item.id} className="error-row">⚠ {item.text}</div>
                if (item.type === 'tool') {
                  const t = item.data
                  return (
                    <div key={t.id} className={`tool-evt ${t.kind === 'result' ? 'result' : ''}`}>
                      <span className="tool-lbl">{t.kind === 'call' ? `🔧 ${t.tool}` : `↩ ${t.tool}`}</span>
                      <span className="tool-body">{t.body}</span>
                    </div>
                  )
                }
                const m = item.data
                return (
                  <div key={m.id} className={`msg-row ${m.role}`}>
                    <div className={`av ${m.role}`}>{m.role === 'user' ? 'U' : '🤖'}</div>
                    <div className="bubble-wrap">
                      <div className="bubble">
                        {m.role === 'assistant' && !m.streaming
                          ? <Markdown text={m.text} />
                          : <span>{m.text}{m.streaming && <span className="cursor" />}</span>
                        }
                      </div>
                      {m.role === 'assistant' && !m.streaming && (
                        <button
                          className={`copy-btn ${copiedId === m.id ? 'copied' : ''}`}
                          onClick={() => copyMessage(m.id, m.text)}
                          title="Copy to clipboard"
                        >
                          {copiedId === m.id ? '✓' : '⧉'}
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
              <div ref={bottomRef} />
            </div>

            <div className="input-bar">
              <div className="input-shell">
                <textarea
                  placeholder="Message the agent… (Enter to send, Shift+Enter for newline)"
                  value={input} onChange={autoResize} onKeyDown={handleKey}
                  disabled={(running && !permReq) || transcribing} rows={1}
                />
              </div>

              {whisperStatus && (
                <div className="whisper-status">{whisperStatus}</div>
              )}

              <button
                className={`mic-btn ${recording ? 'recording' : ''} ${transcribing ? 'transcribing' : ''} ${!whisperReady ? 'unavailable' : ''}`}
                onMouseDown={!transcribing && !recording && whisperReady ? startRecording : undefined}
                onMouseUp={recording ? () => stopRecording() : undefined}
                onMouseLeave={recording ? () => stopRecording() : undefined}
                onTouchStart={!transcribing && !recording && whisperReady ? startRecording : undefined}
                onTouchEnd={recording ? () => stopRecording() : undefined}
                onClick={!whisperReady ? () => setSttError('Whisper not found. Install with: pip install openai-whisper  (requires ffmpeg too)') : undefined}
                disabled={running || transcribing}
                title={
                  !whisperReady ? 'Whisper not installed — click for instructions'
                  : recording ? 'Release to transcribe'
                  : transcribing ? 'Transcribing…'
                  : 'Hold to record'
                }
              >
                {transcribing ? '⌛' : recording ? '⏺' : '🎤'}
              </button>

              {running
                ? <button className="send-btn stop" onClick={() => StopAgent()} title="Stop">⏹</button>
                : <button className="send-btn" onClick={send} disabled={!input.trim() || transcribing}>➤</button>
              }
            </div>
            {sttError && (
              <div className="stt-error" onClick={() => setSttError('')}>⚠ {sttError}</div>
            )}
          </>
        ) : (
          <div className="no-conv">
            <div style={{ fontSize: 48 }}>💬</div>
            <p>Select a conversation or create a new one.</p>
            <button className="btn primary" style={{ marginTop: 10 }} onClick={() => setShowNew(true)}>
              ＋ New conversation
            </button>
          </div>
        )}
      </div>

      {/* ── New conversation modal ── */}
      {showNew && (
        <div className="overlay" onClick={e => e.target === e.currentTarget && setShowNew(false)}>
          <div className="modal">
            <div className="modal-hd">
              <div className="modal-ico ico-new">💬</div>
              <div>
                <h2>New Conversation</h2>
                <p>Choose a working directory for file access.</p>
              </div>
            </div>
            <div className="modal-body">
              <div className="field">
                <label>Working Directory</label>
                <div className="dir-row">
                  <input value={newPath} onChange={e => setNewPath(e.target.value)} placeholder="~ (home directory)" />
                  <button className="pick-btn" onClick={() => PickDirectory().then(d => d && setNewPath(d))}>Browse</button>
                </div>
              </div>
            </div>
            <div className="modal-ft">
              <button className="btn secondary" onClick={() => setShowNew(false)}>Cancel</button>
              <button className="btn primary" onClick={createConv}>Start</button>
            </div>
          </div>
        </div>
      )}

      {/* ── Permission request modal ── */}
      {permReq && (
        <div className="overlay">
          <div className="modal">
            <div className="modal-hd">
              <div className="modal-ico ico-perm">🔐</div>
              <div>
                <h2>Permission Request</h2>
                <p>The agent wants to execute a tool. Allow or deny?</p>
              </div>
            </div>
            <div className="modal-body">
              <div className="perm-card">
                <div className="perm-kv">
                  <div className="perm-key">Tool</div>
                  <div className="perm-val"><strong style={{ fontFamily: 'monospace' }}>{permReq.tool}</strong></div>
                </div>
                <div className="perm-kv">
                  <div className="perm-key">What it does</div>
                  <div className="perm-val">{permReq.description}</div>
                </div>
                {Object.keys(permReq.args).length > 0 && (
                  <div className="perm-kv">
                    <div className="perm-key">Arguments</div>
                    <div>
                      {Object.entries(permReq.args).map(([k, v]) => (
                        <div key={k} className="arg-line">
                          <span className="arg-k">{k}</span>
                          <span className="arg-sep">: </span>
                          <span>{v}</span>
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
              <button className="btn danger" onClick={() => { setPermReq(null); RespondPermission(false) }}>✕ Deny</button>
              <button className="btn primary" onClick={() => { setPermReq(null); RespondPermission(true) }}>✓ Allow</button>
            </div>
          </div>
        </div>
      )}

      {/* ── Settings modal ── */}
      {showCfg && (
        <div className="overlay" onClick={e => e.target === e.currentTarget && setShowCfg(false)}>
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
              <div className="modal-body">
                <div style={{ fontSize: 12, color: 'var(--text2)', lineHeight: 1.7, marginBottom: 12 }}>
                  Add MCP servers to give the agent access to external services (databases, email, APIs, etc.).<br />
                  Each server runs as a subprocess and exposes tools via the <strong>Model Context Protocol</strong>.
                </div>

                <div className="mcp-presets">
                  <div className="mcp-presets-label">Quick Add</div>
                  <div className="mcp-presets-row">
                    {[
                      { key: 'postgres', icon: '🐘', label: 'PostgreSQL', desc: 'Query & modify Postgres databases' },
                      { key: 'mongodb',  icon: '🍃', label: 'MongoDB',    desc: 'Read & write MongoDB collections' },
                      { key: 'gdrive',   icon: '📂', label: 'Google Drive', desc: 'List, read & upload Drive files' },
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
                          <span className="mcp-preset-icon">{p.icon}</span>
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
                      <button className="btn-icon danger" title="Remove server"
                        onClick={() => setMcpServers(s => s.filter((_, j) => j !== i))}>✕</button>
                    </div>

                    {srv.name === 'postgres' && (
                      <div className="mcp-setup-hint">
                        <strong>🐘 PostgreSQL setup:</strong> Replace the connection string in Args with your actual database URL.
                        Requires Node.js. Install once: <code>npm i -g @modelcontextprotocol/server-postgres</code>
                      </div>
                    )}
                    {srv.name === 'mongodb' && (
                      <div className="mcp-setup-hint">
                        <strong>🍃 MongoDB setup:</strong> Replace the <code>--connectionString</code> value in Args with your MongoDB URI.
                        Requires Node.js. Install once: <code>npm i -g mongodb-mcp-server</code>
                      </div>
                    )}
                    {srv.name === 'gdrive' && (
                      <div className="mcp-setup-hint">
                        <strong>📂 Google Drive setup:</strong> Set <code>GDRIVE_CREDENTIALS_FILE</code> to your OAuth2 credentials JSON downloaded from Google Cloud Console, and <code>GDRIVE_TOKEN_FILE</code> to a writable path for the token cache.
                        Requires Node.js. Install once: <code>npm i -g @modelcontextprotocol/server-gdrive</code>
                      </div>
                    )}

                    <div className="field">
                      <label>Name</label>
                      <input value={srv.name} placeholder="e.g. postgres"
                        onChange={e => setMcpServers(s => s.map((x, j) => j === i ? { ...x, name: e.target.value } : x))} />
                    </div>

                    <div className="field">
                      <label>Command</label>
                      <input value={srv.command} placeholder="e.g. npx or uvx or /usr/bin/my-server"
                        onChange={e => setMcpServers(s => s.map((x, j) => j === i ? { ...x, command: e.target.value } : x))} />
                    </div>

                    <div className="field">
                      <label>Arguments (one per line)</label>
                      <textarea rows={3}
                        value={srv.args.join('\n')}
                        onChange={e => setMcpServers(s => s.map((x, j) =>
                          j === i ? { ...x, args: e.target.value.split('\n').filter(Boolean) } : x))}
                        placeholder={"@modelcontextprotocol/server-postgres\npostgresql://localhost/mydb"}
                        style={{ resize: 'vertical', fontFamily: 'monospace', fontSize: 12 }}
                      />
                    </div>

                    <div className="field">
                      <label>Environment variables (KEY=VALUE, one per line)</label>
                      <textarea rows={3}
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
                            setMcpProbeResult(r => ({ ...r, [i]: `✗ ${err?.message || String(err)}` }))
                          } finally {
                            setMcpProbing(p => ({ ...p, [i]: false }))
                          }
                        }}>
                        {mcpProbing[i] ? 'Testing…' : 'Test Connection'}
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
                  + Add MCP Server
                </button>
              </div>
            ) : cfgTab === 'general' ? (
              <div className="modal-body">
                <div className="field">
                  <label>Provider</label>
                  <select value={cfg.provider} onChange={e => setCfg(c => ({ ...c, provider: e.target.value }))}>
                    <optgroup label="── Free · No API Key ──">
                      <option value="ollama">Ollama (local)</option>
                      <option value="llm7">LLM7.io — DeepSeek, GPT-4o, Gemini</option>
                      <option value="zai">Z AI (Zhipu) — GLM-4.7-Flash</option>
                    </optgroup>
                    <optgroup label="── Free Tier · API Token Required ──">
                      <option value="github">GitHub Models — GPT-5, Llama 4, Grok 3</option>
                      <option value="groq">Groq — Llama 4, Qwen3, GPT-OSS</option>
                      <option value="gemini">Google Gemini (AI Studio)</option>
                      <option value="cerebras">Cerebras — GPT-OSS-120B (very fast)</option>
                      <option value="openrouter">OpenRouter — 10+ free models</option>
                      <option value="mistral">Mistral AI</option>
                      <option value="cohere">Cohere</option>
                      <option value="together">Together AI</option>
                      <option value="huggingface">Hugging Face</option>
                      <option value="sambanova">SambaNova Cloud</option>
                      <option value="nvidia">NVIDIA NIM</option>
                    </optgroup>
                    <optgroup label="── Paid / Custom ──">
                      <option value="openai">OpenAI-compatible</option>
                    </optgroup>
                  </select>
                </div>

                {(cfg.provider === 'ollama' || cfg.provider === 'openai') && (
                  <div className="field">
                    <label>Base URL</label>
                    <input value={cfg.base_url} onChange={e => setCfg(c => ({ ...c, base_url: e.target.value }))}
                      placeholder="http://localhost:11434" />
                  </div>
                )}

                {!['ollama', 'llm7', 'zai'].includes(cfg.provider) && (
                  <div className="field">
                    <label>API Key{cfg.provider === 'github' ? ' (GitHub PAT with models:read)' : ''}</label>
                    <input type="password" value={cfg.api_key}
                      onChange={e => setCfg(c => ({ ...c, api_key: e.target.value }))}
                      placeholder={cfg.provider === 'github' ? 'github_pat_…' : 'sk-…'} />
                  </div>
                )}

                <div className="field">
                  <label>Default Model</label>
                  {models.length > 0
                    ? <select value={cfg.model} onChange={e => setCfg(c => ({ ...c, model: e.target.value }))}>
                        {(() => {
                          const freeModels = models.filter(m => m.endsWith(':free'))
                          const paidModels = models.filter(m => !m.endsWith(':free'))
                          if (freeModels.length > 0 && paidModels.length > 0) {
                            return (
                              <>
                                <optgroup label="── Free ──">
                                  {freeModels.map(m => <option key={m} value={m}>{m}</option>)}
                                </optgroup>
                                <optgroup label="── Paid ──">
                                  {paidModels.map(m => <option key={m} value={m}>{m}</option>)}
                                </optgroup>
                              </>
                            )
                          }
                          return models.map(m => <option key={m} value={m}>{m}</option>)
                        })()}
                      </select>
                    : <input value={cfg.model} onChange={e => setCfg(c => ({ ...c, model: e.target.value }))} placeholder="gemma2:27b" />
                  }
                </div>

                <div className="field">
                  <label>Context Limit (tokens)</label>
                  <input
                    type="number" min={1024} max={131072} step={1024}
                    value={cfg.context_limit || 8192}
                    onChange={e => setCfg(c => ({ ...c, context_limit: Number(e.target.value) }))}
                  />
                  <div className="field-hint">Approximate token limit before old messages are trimmed (default: 8192)</div>
                </div>

                <div className="field">
                  <label>System Prompt (optional prefix)</label>
                  <textarea
                    rows={4}
                    value={cfg.system_prompt || ''}
                    onChange={e => setCfg(c => ({ ...c, system_prompt: e.target.value }))}
                    placeholder="Add custom instructions that will be prepended to the agent's system prompt…"
                    style={{ resize: 'vertical', fontFamily: 'monospace', fontSize: 12 }}
                  />
                </div>

                <div className="field">
                  <label>Whisper Model (speech-to-text)</label>
                  <select value={cfg.whisper_model || 'base'} onChange={e => setCfg(c => ({ ...c, whisper_model: e.target.value }))}>
                    <option value="tiny">tiny — fastest, lower accuracy (~39 MB)</option>
                    <option value="base">base — fast, decent accuracy (~74 MB)</option>
                    <option value="small">small — balanced (~244 MB)</option>
                    <option value="medium">medium — high accuracy (~769 MB)</option>
                    <option value="large">large — best accuracy (~1.5 GB)</option>
                  </select>
                  {!whisperReady && (
                    <div className="field-hint">
                      whisper not found — install with: <code>pip install openai-whisper</code>
                    </div>
                  )}
                </div>
              </div>
            ) : (
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
              </div>
            )}

            <div className="modal-ft">
              <button className="btn secondary" onClick={() => setShowCfg(false)}>Cancel</button>
              <button className="btn primary" onClick={saveCfg}>Save</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
