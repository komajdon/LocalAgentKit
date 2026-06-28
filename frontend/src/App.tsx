import { useState, useEffect, useRef, useCallback } from 'react'
import { Markdown } from './Markdown'
import './style.css'
import {
  SendMessage, StopAgent, RespondPermission,
  ListConversations, NewConversation, LoadConversation,
  DeleteConversation, UpdateConversationPath, RenameConversation,
  SearchConversations, ExportConversation, SetConversationModel,
  TruncateAndResend, SetConversationPinned, SetConversationTags,
  ListModels, ListTools, GetConfig, SaveConfig, PickDirectory,
  WhisperAvailable, StartRecording, StopRecording,
  ListMCPServers, SaveMCPServers, ProbeMCPServer,
  CheckForUpdate, GetVersion, ExportBackup, ImportBackup,
  GetUsageBudget,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

// ── Types ─────────────────────────────────────────────────────────────────

interface ConvMeta {
  id: string; title: string; work_dir: string; model?: string
  pinned?: boolean; tags?: string[]
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
  notifications: boolean
  search_provider: string
  search_api_key: string
  theme: string
  budget_data_gb: number
  budget_fund: number
  fund_per_mtokens: number
}

interface ContextUsage { used: number; limit: number; total?: number; estimated?: boolean }
interface UsageBudget {
  tokens: number
  data_total_gb: number; data_remain_gb: number
  fund_total: number; fund_remain: number; fund_unit: string
}
interface UpdateInfo { available: boolean; current: string; latest: string; url: string }

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
  const pinned: ConvMeta[] = []
  const today: ConvMeta[] = [], week: ConvMeta[] = [], older: ConvMeta[] = []
  const now = Date.now()
  list.forEach(c => {
    if (c.pinned) { pinned.push(c); return }
    const age = now - new Date(c.updated_at).getTime()
    if (age < 86400_000) today.push(c)
    else if (age < 604800_000) week.push(c)
    else older.push(c)
  })
  return { pinned, today, week, older }
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
  const [thinking, setThinking] = useState(false)
  const [loadingConv, setLoadingConv] = useState(false)
  const [permReq, setPermReq] = useState<PermReq | null>(null)
  const streamIdRef = useRef<number | null>(null)

  const [showNew, setShowNew]     = useState(false)
  const [showCfg, setShowCfg]     = useState(false)
  const [savingCfg, setSavingCfg] = useState(false)
  const [newPath, setNewPath]     = useState('')
  const [cfgTab, setCfgTab]       = useState<'general' | 'permissions' | 'mcp'>('general')

  // rename state
  const [renamingId, setRenamingId]       = useState<string | null>(null)
  const [renameValue, setRenameValue]     = useState('')
  const renameInputRef                    = useRef<HTMLInputElement>(null)

  // copy feedback
  const [copiedId, setCopiedId] = useState<number | null>(null)

  // message editing (edit & resend)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editValue, setEditValue] = useState('')

  // tag filter + inline tag editor
  const [tagFilter, setTagFilter]       = useState<string | null>(null)
  const [tagEditId, setTagEditId]       = useState<string | null>(null)
  const [tagEditValue, setTagEditValue] = useState('')

  // shared usage budget (data + fund pool)
  const [budget, setBudget] = useState<UsageBudget | null>(null)

  // speech-to-text
  const [whisperReady, setWhisperReady]       = useState(false)
  const [recording, setRecording]             = useState(false)
  const [transcribing, setTranscribing]       = useState(false)
  const [whisperStatus, setWhisperStatus]     = useState('') // download progress label
  const [sttError, setSttError]               = useState('')
  const recordingRef                          = useRef(false)

  // context usage
  const [ctxUsage, setCtxUsage] = useState<ContextUsage | null>(null)
  const [fatalError, setFatalError] = useState('')
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)
  const [updateDismissed, setUpdateDismissed] = useState(false)
  const [appVersion, setAppVersion] = useState('')
  const [backupMsg, setBackupMsg] = useState('')

  // search
  const [searchQuery, setSearchQuery]   = useState('')
  const [searchResults, setSearchResults] = useState<ConvMeta[] | null>(null)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // OS notifications — refs so the once-registered event listeners read live values
  const notifyEnabledRef = useRef(true)
  const sendStartRef     = useRef(0)

  // per-conversation model selector
  const [convModel, setConvModel] = useState('')

  const [models, setModels]     = useState<string[]>([])
  const [toolList, setToolList] = useState<ToolInfo[]>([])
  const [cfg, setCfg]           = useState<Cfg>({
    provider: 'ollama', base_url: 'http://localhost:11434',
    api_key: '', model: '', work_dir: '',
    tool_permissions: {}, whisper_model: 'base',
    system_prompt: '', context_limit: 8192,
    mcp_servers: [], notifications: true,
    search_provider: 'duckduckgo', search_api_key: '',
    theme: 'dark',
    budget_data_gb: 50, budget_fund: 50, fund_per_mtokens: 1,
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

  // Keep the notification preference ref in sync with config.
  useEffect(() => { notifyEnabledRef.current = cfg.notifications !== false }, [cfg.notifications])

  // Apply the colour theme. "system" follows the OS and updates live.
  useEffect(() => {
    const root = document.documentElement
    const apply = (light: boolean) => {
      if (light) root.setAttribute('data-theme', 'light')
      else root.removeAttribute('data-theme')
    }
    if (cfg.theme === 'light') { apply(true); return }
    if (cfg.theme === 'dark' || !cfg.theme) { apply(false); return }
    // system
    const mq = window.matchMedia('(prefers-color-scheme: light)')
    apply(mq.matches)
    const onChange = (e: MediaQueryListEvent) => apply(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [cfg.theme])

  useEffect(() => {
    if ('Notification' in window && Notification.permission === 'default') {
      Notification.requestPermission().catch(() => {})
    }
    GetConfig().then(c => setCfg(c as any)).catch(() => {/* use defaults */})
    ListModels().then((r: any) => r.models && setModels(r.models)).catch(() => {})
    ListTools().then((t: any) => setToolList(t || [])).catch(() => {})
    WhisperAvailable().then(ok => setWhisperReady(ok)).catch(() => {})
    ListMCPServers().then((s: any) => setMcpServers(s || [])).catch(() => {})
    GetVersion().then(v => setAppVersion(v)).catch(() => {})
    CheckForUpdate().then((info: any) => { if (info?.available) setUpdateInfo(info) }).catch(() => {})
    GetUsageBudget().then((b: any) => setBudget(b)).catch(() => {})
    refreshList()
  }, [])

  // ── Events ──────────────────────────────────────────────

  useEffect(() => {
    const offs: (() => void)[] = []

    offs.push(EventsOn('chat:thinking', () => setThinking(true)))

    offs.push(EventsOn('chat:chunk', (chunk: string) => {
      setThinking(false)
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
      setRunning(false); setThinking(false)
      setItems(prev => prev.map(it =>
        it.type === 'msg' && it.data.id === streamIdRef.current
          ? { ...it, data: { ...it.data, streaming: false } }
          : it
      ))
      streamIdRef.current = null
      if (sendStartRef.current && Date.now() - sendStartRef.current > 10_000) {
        notify('Agent finished', 'The task completed and is ready for you.')
      }
      sendStartRef.current = 0
      refreshList()
    }))

    offs.push(EventsOn('chat:stopped', () => {
      setRunning(false); setThinking(false)
      const sid = streamIdRef.current
      streamIdRef.current = null
      setItems(prev => prev.map(it =>
        it.type === 'msg' && it.data.id === sid
          ? { ...it, data: { ...it.data, text: it.data.text + '\n\n— stopped by user', streaming: false } }
          : it
      ))
    }))

    offs.push(EventsOn('chat:error', (msg: string) => {
      setRunning(false); setThinking(false)
      streamIdRef.current = null
      sendStartRef.current = 0
      setItems(prev => [...prev, { type: 'error', id: uid(), text: msg }])
      notify('Agent error', msg)
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

    offs.push(EventsOn('chat:permission_request', (req: PermReq) => {
      setPermReq(req)
      notify('Permission needed', `The agent wants to run "${req.tool}" — your approval is required.`)
    }))

    offs.push(EventsOn('chat:context_usage', (u: ContextUsage) => setCtxUsage(u)))

    offs.push(EventsOn('chat:usage_budget', (b: UsageBudget) => setBudget(b)))

    offs.push(EventsOn('app:fatal', (msg: string) => {
      setFatalError(msg)
      notify('Fatal error', msg)
    }))

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
    setLoadingConv(true)
    let conv: SavedConv
    try {
      conv = await LoadConversation(id) as SavedConv
    } finally {
      setLoadingConv(false)
    }
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

  const togglePin = async (e: React.MouseEvent, c: ConvMeta) => {
    e.stopPropagation()
    await SetConversationPinned(c.id, !c.pinned)
    refreshList()
  }

  const startTagEdit = (e: React.MouseEvent, c: ConvMeta) => {
    e.stopPropagation()
    setTagEditId(c.id)
    setTagEditValue((c.tags || []).join(', '))
  }

  const commitTagEdit = async () => {
    if (!tagEditId) return
    const tags = tagEditValue.split(',').map(t => t.trim()).filter(Boolean)
    await SetConversationTags(tagEditId, tags)
    setTagEditId(null); setTagEditValue('')
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
    setInput(''); setRunning(true); setThinking(true); streamIdRef.current = null
    sendStartRef.current = Date.now()
    SendMessage(text)
  }, [input, running, activeId])

  const handleKey = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
  }

  const startEdit = (id: number, text: string) => {
    if (running) return
    setEditingId(id); setEditValue(text)
  }

  // Resubmit an edited user message: truncate the transcript to that point,
  // replace the message, and re-run the agent from there.
  const commitEdit = (id: number) => {
    const text = editValue.trim()
    if (!text || running) { setEditingId(null); return }
    const idx = items.findIndex(it => it.type === 'msg' && it.data.id === id)
    if (idx < 0) { setEditingId(null); return }
    // userOrdinal = how many user messages precede this one in the transcript.
    let userOrdinal = 0
    for (let i = 0; i < idx; i++) {
      const it = items[i]
      if (it.type === 'msg' && it.data.role === 'user') userOrdinal++
    }
    setItems(prev => [...prev.slice(0, idx), { type: 'msg', data: { id: uid(), role: 'user', text } }])
    setEditingId(null); setEditValue('')
    setRunning(true); setThinking(true); streamIdRef.current = null
    sendStartRef.current = Date.now()
    TruncateAndResend(userOrdinal, text)
  }

  const autoResize = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value)
    const t = e.target; t.style.height = 'auto'
    t.style.height = Math.min(t.scrollHeight, 160) + 'px'
  }

  // Fire an OS notification, but only when enabled, granted, and the window is
  // not focused — there is no point alerting the user about what they can see.
  const notify = (title: string, body: string) => {
    if (!notifyEnabledRef.current) return
    if (!('Notification' in window) || Notification.permission !== 'granted') return
    if (document.hasFocus()) return
    try {
      const n = new Notification(title, { body, tag: 'ai-agent' })
      n.onclick = () => { window.focus(); n.close() }
    } catch {
      // Some webviews reject the constructor — ignore.
    }
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
    if (recordingRef.current) return
    recordingRef.current = true
    setSttError('')
    try {
      await StartRecording()
      setRecording(true)
    } catch (err: any) {
      recordingRef.current = false
      setSttError(`Could not start recording: ${err?.message ?? err}`)
    }
  }

  const stopRecording = async () => {
    if (!recordingRef.current) return
    recordingRef.current = false
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

  const runSearch = (q: string) => {
    setSearchQuery(q)
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    if (!q.trim()) { setSearchResults(null); return }
    searchTimerRef.current = setTimeout(async () => {
      const r = await SearchConversations(q) as ConvMeta[]
      setSearchResults(r || [])
    }, 300)
  }

  const exportConv = async (format: 'markdown' | 'text' | 'json') => {
    if (!activeId) return
    const text = await ExportConversation(activeId, format) as string
    const mime = format === 'markdown' ? 'text/markdown' : format === 'json' ? 'application/json' : 'text/plain'
    const ext = format === 'markdown' ? 'md' : format === 'json' ? 'json' : 'txt'
    const blob = new Blob([text], { type: mime })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = `${activeTitle || 'conversation'}.${ext}`
    a.click(); URL.revokeObjectURL(url)
  }

  const changeConvModel = async (model: string) => {
    setConvModel(model)
    await SetConversationModel(model)
  }

  const saveCfg = async () => {
    setSavingCfg(true)
    try {
      await SaveConfig({ ...cfg, mcp_servers: mcpServers } as any)
      await SaveMCPServers(mcpServers as any)
      const r = await ListModels() as any
      if (r.models) setModels(r.models)
      setShowCfg(false)
    } finally {
      setSavingCfg(false)
    }
  }

  const setToolPerm = (tool: string, perm: PermMode) =>
    setCfg(c => ({ ...c, tool_permissions: { ...c.tool_permissions, [tool]: perm } }))

  // ── Render ───────────────────────────────────────────────

  const allTags = Array.from(new Set(convList.flatMap(c => c.tags || []))).sort()
  const visibleConvs = tagFilter ? convList.filter(c => (c.tags || []).includes(tagFilter)) : convList
  const groups = groupConvs(visibleConvs)

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
                  {c.pinned && <span className="conv-pin-mark">📌</span>}{c.title}
                </div>
              )}
              {tagEditId === c.id ? (
                <input
                  className="conv-rename-input"
                  autoFocus
                  value={tagEditValue}
                  placeholder="tag1, tag2…"
                  onChange={e => setTagEditValue(e.target.value)}
                  onBlur={commitTagEdit}
                  onKeyDown={e => {
                    if (e.key === 'Enter') commitTagEdit()
                    if (e.key === 'Escape') { setTagEditId(null); setTagEditValue('') }
                    e.stopPropagation()
                  }}
                  onClick={e => e.stopPropagation()}
                />
              ) : (c.tags && c.tags.length > 0) && (
                <div className="conv-tags">
                  {c.tags.map(t => (
                    <span key={t} className={`conv-tag ${tagFilter === t ? 'active' : ''}`}
                      onClick={e => { e.stopPropagation(); setTagFilter(tagFilter === t ? null : t) }}>{t}</span>
                  ))}
                </div>
              )}
              <div className="conv-info">
                <span>📁 {shortPath(c.work_dir)}</span>
                <span style={{ marginLeft: 'auto' }}>{timeAgo(c.updated_at)}</span>
              </div>
            </div>
            <div className="conv-row-actions">
              <button className="conv-act" onClick={e => togglePin(e, c)} title={c.pinned ? 'Unpin' : 'Pin'}>{c.pinned ? '📍' : '📌'}</button>
              <button className="conv-act" onClick={e => startTagEdit(e, c)} title="Edit tags">🏷</button>
              <button className="conv-del" onClick={e => delConv(e, c.id)} title="Delete">✕</button>
            </div>
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

      {updateInfo && !updateDismissed && (
        <div className="update-banner">
          <span>🎉 <strong>v{updateInfo.latest}</strong> is available — you're on {updateInfo.current}</span>
          <a href={updateInfo.url} target="_blank" rel="noreferrer" className="update-download-btn">Download</a>
          <button className="update-dismiss" onClick={() => setUpdateDismissed(true)}>✕</button>
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

        {allTags.length > 0 && searchResults === null && (
          <div className="tag-filter-bar">
            {allTags.map(t => (
              <span key={t} className={`conv-tag ${tagFilter === t ? 'active' : ''}`}
                onClick={() => setTagFilter(tagFilter === t ? null : t)}>{t}</span>
            ))}
            {tagFilter && <span className="tag-filter-clear" onClick={() => setTagFilter(null)}>clear ✕</span>}
          </div>
        )}

        <div className="conv-list">
          {searchResults !== null ? (
            searchResults.length === 0
              ? <div className="conv-empty">No results for "{searchQuery}"</div>
              : <ConvGroup label={`Results (${searchResults.length})`} list={searchResults} />
          ) : convList.length === 0
            ? <div className="conv-empty">No conversations yet.<br />Click "+ New conversation" to start.</div>
            : <>
                <ConvGroup label="📌 Pinned" list={groups.pinned} />
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
                value={convModel}
                onChange={e => changeConvModel(e.target.value)}
                title="Model for this conversation"
              >
                <option value="">↩ Default ({cfg.model})</option>
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
                  : null
                }
              </select>

              {/* context usage bar */}
              {ctxUsage && (() => {
                const pct = Math.min(100, Math.round(ctxUsage.used / ctxUsage.limit * 100))
                const warn = pct >= 80
                const prefix = ctxUsage.estimated ? '~' : ''
                const totalStr = ctxUsage.total ? ` · ${ctxUsage.total.toLocaleString()} tokens used this conversation` : ''
                const budgetStr = budget
                  ? `\n\nShared budget (account-wide):`
                    + `\n• Data: ${budget.data_remain_gb.toFixed(2)} / ${budget.data_total_gb} GB remaining`
                    + `\n• Fund: ${budget.fund_remain.toFixed(2)} / ${budget.fund_total} ${budget.fund_unit} remaining`
                    + `\n• ${budget.tokens.toLocaleString()} tokens used total`
                  : ''
                const title = `Context: ${prefix}${ctxUsage.used.toLocaleString()} / ${ctxUsage.limit.toLocaleString()} tokens`
                  + (ctxUsage.estimated ? ' (estimated)' : ' (actual)') + totalStr + budgetStr
                return (
                  <div className={`ctx-bar ${warn ? 'warn' : ''}`} title={title}>
                    <div className="ctx-fill" style={{ width: pct + '%' }} />
                    <span className="ctx-label">{prefix}{pct}%</span>
                  </div>
                )
              })()}

              {/* export menu */}
              <div className="export-menu">
                <button className="topbar-btn" title="Export conversation">⬇</button>
                <div className="export-dropdown">
                  <button onClick={() => exportConv('markdown')}>Export as Markdown</button>
                  <button onClick={() => exportConv('text')}>Export as Plain Text</button>
                  <button onClick={() => exportConv('json')}>Export as JSON</button>
                </div>
              </div>

              <div className="path-chip" onClick={changePath} title="Click to change working directory">
                📁 <span>{shortPath(activePath)}</span>
              </div>
            </div>

            <div className="messages">
              {loadingConv && (
                <div className="conv-loading"><span className="thinking-dots"><span /><span /><span /></span></div>
              )}
              {!loadingConv && items.length === 0 && (
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
                const editing = editingId === m.id
                return (
                  <div key={m.id} className={`msg-row ${m.role}`}>
                    <div className={`av ${m.role}`}>{m.role === 'user' ? 'U' : '🤖'}</div>
                    <div className="bubble-wrap">
                      <div className="bubble">
                        {editing ? (
                          <div className="msg-edit">
                            <textarea
                              autoFocus
                              value={editValue}
                              onChange={e => setEditValue(e.target.value)}
                              onKeyDown={e => {
                                if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); commitEdit(m.id) }
                                if (e.key === 'Escape') { setEditingId(null); setEditValue('') }
                              }}
                              rows={Math.min(8, editValue.split('\n').length + 1)}
                            />
                            <div className="msg-edit-actions">
                              <button className="btn secondary" onClick={() => { setEditingId(null); setEditValue('') }}>Cancel</button>
                              <button className="btn primary" onClick={() => commitEdit(m.id)} disabled={!editValue.trim()}>Save &amp; resend</button>
                            </div>
                          </div>
                        ) : m.role === 'assistant' && !m.streaming
                          ? <Markdown text={m.text} />
                          : <span>{m.text}{m.streaming && <span className="cursor" />}</span>
                        }
                      </div>
                      {!m.streaming && !editing && (
                        <div className="msg-actions">
                          {m.role === 'user' && (
                            <button
                              className="copy-btn"
                              onClick={() => startEdit(m.id, m.text)}
                              disabled={running}
                              title="Edit & resend"
                            >✎</button>
                          )}
                          <button
                            className={`copy-btn ${copiedId === m.id ? 'copied' : ''}`}
                            onClick={() => copyMessage(m.id, m.text)}
                            title="Copy to clipboard"
                          >
                            {copiedId === m.id ? '✓' : '⧉'}
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
              {thinking && running && (
                <div className="msg-row assistant">
                  <div className="av assistant">🤖</div>
                  <div className="bubble-wrap">
                    <div className="bubble">
                      <span className="thinking-dots"><span /><span /><span /></span>
                    </div>
                  </div>
                </div>
              )}
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
                onTouchStart={!transcribing && !recording && whisperReady ? (e: React.TouchEvent) => { e.preventDefault(); startRecording() } : undefined}
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
                          {permReq.tool === 'shell' && k === 'command'
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
              <button className="btn danger" onClick={() => { setPermReq(null); RespondPermission(false) }}>✕ Deny</button>
              <button className="btn primary" onClick={() => { setPermReq(null); RespondPermission(true) }}>✓ Allow</button>
            </div>
          </div>
        </div>
      )}

      {/* ── Settings modal ── */}
      {showCfg && (
        <div className="overlay">
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

                <div className="mcp-prereq-banner">
                  <span className="mcp-prereq-icon">⚠️</span>
                  <div>
                    <strong>Before using MCP servers</strong> — each server is a separate program that must be installed on your machine first.<br />
                    Use <strong>Test Connection</strong> on each card to verify it works before starting a conversation.
                  </div>
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
                        <div className="mcp-setup-title">🐘 PostgreSQL — setup required</div>
                        <ol className="mcp-setup-steps">
                          <li>Install Node.js if not already: <a href="https://nodejs.org" target="_blank" rel="noreferrer">nodejs.org</a></li>
                          <li>Install the MCP server once:<br /><code>npm install -g @modelcontextprotocol/server-postgres</code></li>
                          <li>Change <strong>Command</strong> to <code>mcp-server-postgres</code> and remove the <code>-y</code> / package name from Args</li>
                          <li>Replace the connection string in Args with your actual Postgres URL</li>
                          <li>Click <strong>Test Connection</strong> to verify before saving</li>
                        </ol>
                      </div>
                    )}
                    {srv.name === 'mongodb' && (
                      <div className="mcp-setup-hint">
                        <div className="mcp-setup-title">🍃 MongoDB — setup required</div>
                        <ol className="mcp-setup-steps">
                          <li>Install Node.js if not already: <a href="https://nodejs.org" target="_blank" rel="noreferrer">nodejs.org</a></li>
                          <li>Install the MCP server once:<br /><code>npm install -g mongodb-mcp-server</code></li>
                          <li>Change <strong>Command</strong> to <code>mongodb-mcp-server</code> and clear the Args (no npx flags needed)</li>
                          <li>Add your connection string to Args: <code>--connectionString mongodb://localhost:27017/mydb</code></li>
                          <li>Click <strong>Test Connection</strong> to verify before saving</li>
                        </ol>
                      </div>
                    )}
                    {srv.name === 'gdrive' && (
                      <div className="mcp-setup-hint">
                        <div className="mcp-setup-title">📂 Google Drive — setup required</div>
                        <ol className="mcp-setup-steps">
                          <li>Install Node.js if not already: <a href="https://nodejs.org" target="_blank" rel="noreferrer">nodejs.org</a></li>
                          <li>Install the MCP server once:<br /><code>npm install -g @modelcontextprotocol/server-gdrive</code></li>
                          <li>Create OAuth2 credentials in <a href="https://console.cloud.google.com" target="_blank" rel="noreferrer">Google Cloud Console</a> → APIs &amp; Services → Credentials</li>
                          <li>Set <code>GDRIVE_CREDENTIALS_FILE</code> in Env vars to the downloaded JSON path</li>
                          <li>Set <code>GDRIVE_TOKEN_FILE</code> to a writable path (e.g. <code>/home/you/.gdrive-token.json</code>)</li>
                          <li>Click <strong>Test Connection</strong> to verify before saving</li>
                        </ol>
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
                  <label className="mcp-toggle">
                    <input
                      type="checkbox"
                      checked={cfg.notifications !== false}
                      onChange={e => setCfg(c => ({ ...c, notifications: e.target.checked }))}
                    />
                    <span>Desktop notifications</span>
                  </label>
                  <div className="field-hint">Alert me when a long task finishes, a permission is needed, or an error occurs — only while the window is in the background.</div>
                </div>

                <div className="field">
                  <label>Theme</label>
                  <select
                    value={cfg.theme || 'dark'}
                    onChange={e => setCfg(c => ({ ...c, theme: e.target.value }))}
                  >
                    <option value="dark">Dark</option>
                    <option value="light">Light</option>
                    <option value="system">Follow OS</option>
                  </select>
                  <div className="field-hint">Applies immediately. "Follow OS" tracks your system's light/dark preference.</div>
                </div>

                <div className="field">
                  <label>Shared Usage Budget</label>
                  <div className="budget-grid">
                    <div>
                      <span className="budget-sub">Data (GB)</span>
                      <input type="number" min={0} step={1}
                        value={cfg.budget_data_gb ?? 50}
                        onChange={e => setCfg(c => ({ ...c, budget_data_gb: Number(e.target.value) }))} />
                    </div>
                    <div>
                      <span className="budget-sub">Fund (STRRIAL)</span>
                      <input type="number" min={0} step={1}
                        value={cfg.budget_fund ?? 50}
                        onChange={e => setCfg(c => ({ ...c, budget_fund: Number(e.target.value) }))} />
                    </div>
                    <div>
                      <span className="budget-sub">STRRIAL / 1M tokens</span>
                      <input type="number" min={0} step={0.1}
                        value={cfg.fund_per_mtokens ?? 1}
                        onChange={e => setCfg(c => ({ ...c, fund_per_mtokens: Number(e.target.value) }))} />
                    </div>
                  </div>
                  <div className="field-hint">A single account-wide pool consumed by token usage across all conversations. Remaining data/fund is shown in the context-bar tooltip.</div>
                </div>

                <div className="field">
                  <label>Web Search Provider</label>
                  <select
                    value={cfg.search_provider || 'duckduckgo'}
                    onChange={e => setCfg(c => ({ ...c, search_provider: e.target.value }))}
                  >
                    <option value="duckduckgo">DuckDuckGo — no API key required</option>
                    <option value="brave">Brave Search — requires API key</option>
                    <option value="serpapi">SerpAPI (Google) — requires API key</option>
                  </select>
                  <div className="field-hint">Powers the <code>search_web</code> tool. DuckDuckGo works out of the box; Brave/SerpAPI return higher-quality results with an API key.</div>
                </div>

                {(cfg.search_provider === 'brave' || cfg.search_provider === 'serpapi') && (
                  <div className="field">
                    <label>{cfg.search_provider === 'brave' ? 'Brave Search' : 'SerpAPI'} API Key</label>
                    <input
                      type="password"
                      value={cfg.search_api_key || ''}
                      onChange={e => setCfg(c => ({ ...c, search_api_key: e.target.value }))}
                      placeholder={cfg.search_provider === 'brave' ? 'BSA…' : 'serpapi key'}
                    />
                  </div>
                )}

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

                <div className="field">
                  <label>Data Backup</label>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button className="btn secondary" onClick={async () => {
                      setBackupMsg('')
                      const err = await ExportBackup() as any
                      setBackupMsg(err || '✓ Backup saved')
                    }}>Export backup…</button>
                    <button className="btn secondary" onClick={async () => {
                      setBackupMsg('')
                      const result = await ImportBackup() as any
                      if (result === 'ok') setBackupMsg('✓ Restored — please restart the app')
                      else if (result) setBackupMsg('✗ ' + result)
                    }}>Restore backup…</button>
                  </div>
                  {backupMsg && (
                    <div className="field-hint" style={{ color: backupMsg.startsWith('✓') ? 'var(--green)' : 'var(--red)' }}>
                      {backupMsg}
                    </div>
                  )}
                  <div className="field-hint">
                    Exports <code>agent.db</code> + <code>agent.key</code> as a <code>.tar.gz</code>.
                    Keep the backup in a safe place — it contains your encryption key.
                  </div>
                </div>

                {appVersion && (
                  <div style={{ fontSize: 11, color: 'var(--text3)', marginTop: 8 }}>
                    Version: {appVersion}
                  </div>
                )}
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
              <button className="btn secondary" onClick={() => setShowCfg(false)} disabled={savingCfg}>Cancel</button>
              <button className="btn primary" onClick={saveCfg} disabled={savingCfg}>{savingCfg ? 'Saving…' : 'Save'}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
