import { useState, useEffect, useRef, useCallback } from 'react'
import { Markdown } from './Markdown'
import './style.css'
import {
  SendMessage, StopAgent, RespondPermission,
  ListPermissionGrants, RevokePermissionGrant, RevokeAllPermissionGrants,
  ListConversations, NewConversation, LoadConversation,
  DeleteConversation, UpdateConversationPath, RenameConversation,
  SearchConversations, ExportConversation, SetConversationModel,
  TruncateAndResend, SetConversationPinned, SetConversationTags,
  ListModels, ListTools, GetConfig, GetSandboxStatus, GetStartupError, RecentWorkDirs, ExportDiagnostics, GetRunState, GetMCPStatus, RestartMCPServer, SaveConfig, PickDirectory,
  WhisperAvailable, StartRecording, StopRecording,
  ListMCPServers, ProbeMCPServer,
  CheckForUpdate, GetVersion, ExportBackup, ImportBackup, GetKeyLocation, SetKeyLocation, RotateEncryptionKey,
  GetContextUsage, BranchConversation, ImportConversation,
} from '../wailsjs/go/main/App'
import { ProfilePicker } from './components/ProfilePicker'
import { WorkspaceManager } from './components/WorkspaceManager'
import { DiffView } from './components/DiffView'
import { FileChanges } from './components/FileChanges'
import { ToolTimeline } from './components/ToolTimeline'
import { ModelSettingsPanel } from './components/ModelSettingsPanel'
import { UsageDashboard } from './components/UsageDashboard'
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
interface PermReq {
  id: string; tool: string; description: string; args: Record<string, unknown>
  workspace: string; operation: string; target: string; impact: string
  sensitive: boolean; scope: string; preview?: string
}
interface Grant { id: string; workspace: string; tool: string; label: string; scope: string }
interface ToolInfo { name: string; description: string }

type MsgRole = 'user' | 'assistant'
interface ChatMsg { id: number; role: MsgRole; text: string; streaming?: boolean }
interface ToolEvt { id: number; kind: 'call' | 'result'; tool: string; body: string }
type Item =
  | { type: 'msg';   data: ChatMsg }
  | { type: 'tool';  data: ToolEvt }
  | { type: 'error'; id: number; text: string }
  | { type: 'status'; id: number; text: string }

interface ChatEvent { conversation_id: string; run_id: string; seq: number; data: any }
const CHAT_EVENTS = ['started', 'thinking', 'chunk', 'assistant', 'tool_call', 'tool_result', 'context_usage', 'warning', 'done', 'stopped', 'error']
const FINAL_EVENTS = ['done', 'stopped', 'error']
const CONV_PAGE = 100

// Stored transcript items keep their text under data; the view does not for
// error and status rows.
const fromStored = (it: any, id: number): Item =>
  it.type === 'error' || it.type === 'status'
    ? { type: it.type, id, text: it.data.text }
    : { ...it, data: { ...it.data, id, streaming: !!it.data.streaming } }

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
  sandbox_mode: string
  tool_calling: string
  backup_retention: number
  context_strategy: string
  max_rounds: number
  max_parallel_tools: number
  tool_timeout_seconds: number
  sandbox_network: boolean
  search_api_key: string
  theme: string
}

interface ContextUsage { used: number; limit: number; total?: number; estimated?: boolean; omitted?: number; summarised?: number }
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
  const m = p.match(/^(?:\/home\/[^/]+|\/Users\/[^/]+|[A-Za-z]:\\Users\\[^\\]+)/)
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
  // The single run may belong to a conversation other than the one shown.
  const [showUsage, setShowUsage] = useState(false)
  const [showTimeline, setShowTimeline] = useState(false)
  const [showChanges, setShowChanges] = useState(false)
  const [showWorkspaces, setShowWorkspaces] = useState(false)
  const [recentDirs, setRecentDirs] = useState<string[]>([])
  const [convLimit, setConvLimit] = useState(CONV_PAGE)
  const [runConv, setRunConv] = useState<string | null>(null)
  const running = runConv !== null && runConv === activeId
  const busy = runConv !== null
  const activeIdRef = useRef<string | null>(null)
  // Events arriving while a conversation loads are held and replayed.
  const loadRef = useRef({ token: 0, buffering: false, buffer: [] as [string, ChatEvent][] })
  const applyRef = useRef<(name: string, data: any) => void>(() => {})
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
  const [passphrase, setPassphrase] = useState('')
  const [keyLocation, setKeyLocation] = useState('file')
  const [keyMsg, setKeyMsg] = useState('')
  // Risky actions need a second click. window.confirm is not used because not
  // every embedded web view implements it.
  const [confirming, setConfirming] = useState<'' | 'restore' | 'keychain' | 'rotate'>('')
  const moveKey = async (next: string) => {
    setConfirming(''); setKeyMsg('')
    const err = await SetKeyLocation(next)
    if (err) setKeyMsg(err)
    GetKeyLocation().then(setKeyLocation)
  }

  // search
  const [searchQuery, setSearchQuery]   = useState('')
  const [searchResults, setSearchResults] = useState<ConvMeta[] | null>(null)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // sidebar collapse — persisted in localStorage
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem('sidebar-collapsed') === 'true')

  // import feedback
  const [importMsg, setImportMsg] = useState('')

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
    sandbox_mode: 'auto', sandbox_network: false, tool_calling: 'auto', backup_retention: 7, context_strategy: 'sliding', max_rounds: 0, max_parallel_tools: 0, tool_timeout_seconds: 0,
  })

  // Isolation actually applied to shell commands under the saved settings.
  const [sandboxStatus, setSandboxStatus] = useState<{ backend: string; isolated: boolean; network_blocked: boolean; detail: string } | null>(null)
  useEffect(() => {
    if (showCfg) GetSandboxStatus().then(setSandboxStatus).catch(() => setSandboxStatus(null))
  }, [showCfg])

  // Remembered approvals, listed in Settings so they can be revoked.
  const [grants, setGrants] = useState<Grant[]>([])
  const refreshGrants = () => ListPermissionGrants().then(g => setGrants((g as any) || [])).catch(() => setGrants([]))
  useEffect(() => {
    if (showCfg && cfgTab === 'permissions') refreshGrants()
  }, [showCfg, cfgTab])
  const answerPermission = (decision: 'once' | 'session' | 'workspace' | 'deny') => {
    if (!permReq) return
    RespondPermission(permReq.id, decision)
    setPermReq(null)
  }

  useEffect(() => {
    if (showCfg) {
      setPassphrase(''); setKeyMsg(''); setConfirming('')
      GetKeyLocation().then(setKeyLocation).catch(() => {})
    }
  }, [showCfg])

  // Live state of the saved servers, polled while the MCP tab is open.
  const [mcpStatus, setMcpStatus] = useState<{ name: string; state: string; tools: string[]; last_error: string }[]>([])
  const [mcpRestarting, setMcpRestarting] = useState('')
  const [mcpRestartError, setMcpRestartError] = useState('')
  const refreshMcpStatus = useCallback(() => GetMCPStatus().then(s => setMcpStatus((s as any) || [])).catch(() => {}), [])
  useEffect(() => {
    if (!showCfg || cfgTab !== 'mcp') return
    setMcpRestartError('')
    refreshMcpStatus()
    const timer = window.setInterval(refreshMcpStatus, 3000)
    return () => window.clearInterval(timer)
  }, [showCfg, cfgTab, refreshMcpStatus])
  const restartMcp = async (name: string) => {
    setMcpRestarting(name); setMcpRestartError('')
    const err = await RestartMCPServer(name)
    setMcpRestarting('')
    if (err) setMcpRestartError(`${name}: ${err}`)
    refreshMcpStatus()
  }

  useEffect(() => {
    if (showNew) RecentWorkDirs().then(d => setRecentDirs(d || [])).catch(() => {})
  }, [showNew])

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

  // Virtual scroller refs & state
  const msgContainerRef = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const isNearBottom = useRef(true)

  const VIRTUAL_OVERSCAN = 6

  const estItemHeight = (it: Item): number => {
    if (it.type === 'error' || it.type === 'status') return 48
    if (it.type === 'tool') return 48
    return Math.max(80, Math.min(500, it.data.text.length * 0.3 + 50))
  }

  const computeVirtualRange = () => {
    if (items.length === 0) return { start: 0, end: 0, offsets: [], total: 0 }
    const heights = items.map(estItemHeight)
    const offsets: number[] = []
    let total = 0
    for (const h of heights) { offsets.push(total); total += h }
    const ch = msgContainerRef.current?.clientHeight ?? 600
    const viewTop = scrollTop
    const viewBot = scrollTop + ch
    let start = offsets.findIndex((o, i) => o + heights[i] > viewTop - 80 * VIRTUAL_OVERSCAN)
    if (start < 0) start = Math.max(0, items.length - 1)
    start = Math.max(0, start - 1)
    let end = start
    while (end < items.length && offsets[end] < viewBot + 80 * VIRTUAL_OVERSCAN) end++
    return { start, end, offsets, total, heights }
  }

  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (isNearBottom.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [items])

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
    refreshList()
  }, [])

  // ── Events ──────────────────────────────────────────────

  useEffect(() => {
    const offs: (() => void)[] = []

    const handlers: Record<string, (d: any) => void> = {}

    handlers['started'] = () => {}
    handlers['thinking'] = () => setThinking(true)

    handlers['chunk'] = ((chunk: string) => {
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
    })

    handlers['done'] = (() => {
      setThinking(false)
      setItems(prev => prev.map(it =>
        it.type === 'msg' && it.data.id === streamIdRef.current
          ? { ...it, data: { ...it.data, streaming: false } }
          : it
      ))
      streamIdRef.current = null
    })

    handlers['stopped'] = (() => {
      setThinking(false)
      const sid = streamIdRef.current
      streamIdRef.current = null
      setItems(prev => [...prev.map(it =>
        it.type === 'msg' && it.data.id === sid
          ? { ...it, data: { ...it.data, streaming: false } }
          : it
      ), { type: 'status', id: uid(), text: 'Stopped by user' }])
    })

    handlers['error'] = ((msg: string) => {
      setThinking(false)
      streamIdRef.current = null
      setItems(prev => [...prev, { type: 'error', id: uid(), text: msg }])
    })

    // The finished reply replaces the streamed text, which may hold tool calls.
    handlers['assistant'] = (text: string) => {
      const sid = streamIdRef.current
      streamIdRef.current = null
      setItems(prev => {
        if (sid === null) return text ? [...prev, { type: 'msg', data: { id: uid(), role: 'assistant', text, streaming: false } }] : prev
        return text
          ? prev.map(it => it.type === 'msg' && it.data.id === sid ? { ...it, data: { ...it.data, text, streaming: false } } : it)
          : prev.filter(it => !(it.type === 'msg' && it.data.id === sid))
      })
    }

    handlers['tool_call'] = (d: { tool: string; args: string }) => {
      const toolId = uid() // allocate outside setState — never call uid() inside an updater
      setItems(prev => [...prev, { type: 'tool', data: { id: toolId, kind: 'call', tool: d.tool, body: d.args } }])
    }

    handlers['tool_result'] = ((d: { tool: string; result: string }) => {
      const resultId = uid()
      setItems(prev => [...prev, { type: 'tool', data: { id: resultId, kind: 'result', tool: d.tool, body: d.result } }])
    })

    offs.push(EventsOn('chat:permission_request', (req: PermReq) => {
      setPermReq(req)
      notify('Permission needed', `The agent wants to run "${req.tool}" — your approval is required.`)
    }))

    offs.push(EventsOn('chat:permission_closed', (id: string) => {
      setPermReq(cur => (cur?.id === id ? null : cur))
    }))

    handlers['warning'] = (text: string) =>
      setItems(prev => [...prev, { type: 'status', id: uid(), text: `Warning: ${text}` }])

    handlers['context_usage'] = (u: ContextUsage) => setCtxUsage(u)

    applyRef.current = (name, data) => handlers[name]?.(data)

    for (const name of CHAT_EVENTS) {
      offs.push(EventsOn(`chat:${name}`, (e: ChatEvent) => {
        // Run bookkeeping applies whichever conversation is on screen.
        if (name === 'started') setRunConv(e.conversation_id)
        if (FINAL_EVENTS.includes(name)) {
          setRunConv(cur => (cur === e.conversation_id ? null : cur))
          if (name === 'error') notify('Agent error', e.data)
          else if (name === 'done' && sendStartRef.current && Date.now() - sendStartRef.current > 10_000) {
            notify('Agent finished', 'The task completed and is ready for you.')
          }
          sendStartRef.current = 0
          refreshList()
        }
        if (e.conversation_id !== activeIdRef.current) return
        if (loadRef.current.buffering) loadRef.current.buffer.push([name, e])
        else applyRef.current(name, e.data)
      }))
    }

    GetRunState().then(r => { if (r.run_id) setRunConv(r.conversation_id) }).catch(() => {})


    offs.push(EventsOn('app:fatal', (msg: string) => {
      setFatalError(msg)
      notify('Fatal error', msg)
    }))

    GetStartupError().then(msg => { if (msg) setFatalError(msg) }).catch(() => {})

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
    const load = loadRef.current
    const token = ++load.token
    load.buffering = true
    load.buffer = []
    activeIdRef.current = id
    setLoadingConv(true)
    let conv: SavedConv & { running?: boolean; seq?: number }
    try {
      conv = await LoadConversation(id) as any
    } catch (err) {
      if (token === load.token) load.buffering = false
      throw err
    } finally {
      if (token === load.token) setLoadingConv(false)
    }
    if (token !== load.token) return // a newer load replaced this one

    setActiveId(conv.id); setActiveTitle(conv.title); setActivePath(conv.work_dir)
    setConvModel((conv as any).model || '')
    setCtxUsage(null)
    setThinking(false); streamIdRef.current = null
    GetContextUsage().then((u: any) => setCtxUsage(u)).catch(() => {})

    const loaded: Item[] = conv.display_items?.length
      ? conv.display_items.map((it: any) => fromStored(it, uid()))
      : conv.messages
        .filter(m => (m.role === 'user' || m.role === 'assistant') && !m.content.startsWith('TOOL_RESULT'))
        .map(m => ({ type: 'msg' as const, data: { id: uid(), role: m.role as MsgRole, text: m.content, streaming: false } }))
    const last = loaded[loaded.length - 1]
    if (conv.running && last?.type === 'msg' && last.data.streaming) streamIdRef.current = last.data.id
    setItems(loaded)

    load.buffering = false
    for (const [name, e] of load.buffer) {
      if (e.seq > (conv.seq || 0)) applyRef.current(name, e.data)
    }
    load.buffer = []
  }

  const createConv = async () => {
    const conv = await NewConversation(newPath) as SavedConv
    setNewPath(''); setShowNew(false)
    await refreshList(); await openConv(conv.id)
  }

  const delConv = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    await DeleteConversation(id)
    if (activeId === id) { activeIdRef.current = null; setActiveId(null); setItems([]); setActiveTitle(''); setActivePath('') }
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

  // Optimistic state for a run that is about to start.
  const startedRun = (id: string) => {
    setRunConv(id); setThinking(true); streamIdRef.current = null
    sendStartRef.current = Date.now()
  }
  const runRejected = (id: string, error?: string) => {
    if (!error) return
    setRunConv(cur => (cur === id ? null : cur))
    setThinking(false)
    if (activeIdRef.current === id) setItems(prev => [...prev, { type: 'error', id: uid(), text: error }])
    GetRunState().then(r => setRunConv(r.run_id ? r.conversation_id : null)).catch(() => {})
  }

  const send = useCallback(() => {
    const text = input.trim()
    if (!text || busy || !activeId) return
    setItems(prev => [...prev, { type: 'msg', data: { id: uid(), role: 'user', text } }])
    setInput(''); startedRun(activeId)
    SendMessage(activeId, text).then(r => runRejected(activeId, r.error))
  }, [input, busy, activeId])

  const handleKey = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
  }

  const startEdit = (id: number, text: string) => {
    if (busy) return
    setEditingId(id); setEditValue(text)
  }

  // Resubmit an edited user message: truncate the transcript to that point,
  // replace the message, and re-run the agent from there.
  const commitEdit = (id: number) => {
    const text = editValue.trim()
    if (!text || busy || !activeId) { setEditingId(null); return }
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
    startedRun(activeId)
    TruncateAndResend(activeId, userOrdinal, text).then(r => runRejected(activeId, r.error))
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

  const toggleSidebar = () => {
    setSidebarCollapsed(prev => {
      const next = !prev
      localStorage.setItem('sidebar-collapsed', String(next))
      return next
    })
  }

  const branchFromHere = async (itemIdx: number) => {
    if (!activeId || busy) return
    try {
      const conv = await BranchConversation(activeId, itemIdx) as any
      await refreshList()
      await openConv(conv.id)
    } catch (err: any) {
      setItems(prev => [...prev, { type: 'error', id: uid(), text: `Could not branch: ${err?.message ?? err}` }])
    }
  }

  const importConversation = async () => {
    setImportMsg('')
    const result = await ImportConversation() as string
    if (result) setImportMsg(result)
    if (result && !result.startsWith('No')) refreshList()
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
  const matching = tagFilter ? convList.filter(c => (c.tags || []).includes(tagFilter)) : convList
  // Large histories are rendered a page at a time; pinned ones always show.
  const visibleConvs = matching.filter((c, i) => i < convLimit || c.pinned || c.id === activeId)
  const hiddenConvs = matching.length - visibleConvs.length
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
      <aside className={`sidebar${sidebarCollapsed ? ' collapsed' : ''}`}
        role="navigation" aria-label="Conversations">
        <div className="sidebar-head">
          <div className="brand">
            <div className="brand-icon">🤖</div>
            {!sidebarCollapsed && (
              <div>
                <div className="brand-name">AI Agent</div>
                <div className="brand-sub">Powered by Komajdon</div>
              </div>
            )}
          </div>
          {sidebarCollapsed
            ? <button className="new-btn collapsed" onClick={() => setShowNew(true)} aria-label="New conversation">＋</button>
            : <button className="new-btn" onClick={() => setShowNew(true)} aria-label="New conversation">＋ New conversation</button>
          }
        </div>

        <button className="sidebar-collapse-btn" onClick={toggleSidebar}
          aria-label={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}>
          {sidebarCollapsed ? '▶' : '◀'}
        </button>

        {!sidebarCollapsed && (
          <>
            <div className="sidebar-search">
              <input
                className="search-input"
                placeholder="Search conversations…"
                value={searchQuery}
                onChange={e => runSearch(e.target.value)}
                aria-label="Search conversations"
              />
              {searchQuery && <button className="search-clear" onClick={() => runSearch('')} aria-label="Clear search">✕</button>}
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

            <div className="conv-list" role="list" aria-label="Conversation list">
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
                    {hiddenConvs > 0 && searchResults === null && (
                      <button className="btn secondary" onClick={() => setConvLimit(n => n + CONV_PAGE)}>
                        Show more ({hiddenConvs})
                      </button>
                    )}
                  </>
              }
            </div>
          </>
        )}

        <div className="sidebar-foot">
          {sidebarCollapsed ? (
            <button className="foot-btn collapsed" onClick={() => { setShowCfg(true); setCfgTab('general') }}
              aria-label="Settings">⚙️</button>
          ) : (
            <>
              <button className="foot-btn" onClick={() => setShowUsage(true)}>📊 Usage</button>
            <button className="foot-btn" onClick={() => setShowTimeline(true)} disabled={!activeId}>🧭 Tool timeline</button>
            <button className="foot-btn" onClick={() => setShowChanges(true)} disabled={!activeId}>📝 File changes</button>
            <button className="foot-btn" onClick={() => setShowWorkspaces(true)}>🗂 Workspaces &amp; profiles</button>
              <button className="foot-btn" onClick={() => { setShowCfg(true); setCfgTab('general') }}
                aria-label="Settings">⚙️ Settings</button>
              <div className="conn-badge">
                <div className="dot" />
                <span>{cfg.model || 'No model selected'}</span>
              </div>
            </>
          )}
        </div>
      </aside>

      {/* ── Main ── */}
      <div className="main" role="main" aria-label="Chat area">
        {activeId ? (
          <>
            <div className="topbar">
              <div className="topbar-title">{activeTitle || 'Conversation'}</div>
              {activeId && <ProfilePicker conversationId={activeId} reload={showWorkspaces} />}

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
                const title = `Context: ${prefix}${ctxUsage.used.toLocaleString()} / ${ctxUsage.limit.toLocaleString()} tokens`
                  + (ctxUsage.estimated ? ' (estimated)' : ' (actual)') + totalStr
                  + (ctxUsage.omitted ? `\n${ctxUsage.omitted} older messages are not sent`
                    + (ctxUsage.summarised ? `; ${ctxUsage.summarised} are summarised` : '') + ' (history is kept)' : '')
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

            <div className="messages" ref={msgContainerRef}
              onScroll={e => {
                const el = e.currentTarget
                setScrollTop(el.scrollTop)
                isNearBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 200
              }}
              role="log" aria-live="polite" aria-label="Conversation messages"
            >
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

              {items.length > 0 && (() => {
                const { start, end, offsets, total } = computeVirtualRange()
                const topPad = offsets[start] || 0
                const lastIdx = Math.min(end, items.length) - 1
                const bottomPad = lastIdx >= start ? total - (offsets[lastIdx] + estItemHeight(items[lastIdx])) : 0
                return (
                  <>
                    {topPad > 0 && <div style={{ height: topPad }} />}
                    {items.slice(start, end).map((item, i) => {
                      const globalIdx = start + i
                      if (item.type === 'error')
                        return <div key={item.id} className="error-row" role="alert">⚠ {item.text}</div>
                      if (item.type === 'status')
                        return <div key={item.id} className="field-hint" role="status" style={{ textAlign: 'center' }}>— {item.text} —</div>
                      if (item.type === 'tool') {
                        const t = item.data
                        return (
                          <div key={t.id} className={`tool-evt ${t.kind === 'result' ? 'result' : ''}`}
                            role="status" aria-label={`Tool ${t.kind}: ${t.tool}`}>
                            <span className="tool-lbl">{t.kind === 'call' ? `🔧 ${t.tool}` : `↩ ${t.tool}`}</span>
                            <span className="tool-body">{t.body}</span>
                          </div>
                        )
                      }
                      const m = item.data
                      const editing = editingId === m.id
                      return (
                        <div key={m.id} className={`msg-row ${m.role}`}
                          role="article" aria-label={`${m.role} message`}>
                          <div className={`av ${m.role}`} aria-hidden="true">{m.role === 'user' ? 'U' : '🤖'}</div>
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
                                    aria-label="Edit message"
                                  />
                                  <div className="msg-edit-actions">
                                    <button className="btn secondary" onClick={() => { setEditingId(null); setEditValue('') }}
                                      aria-label="Cancel edit">Cancel</button>
                                    <button className="btn primary" onClick={() => commitEdit(m.id)} disabled={!editValue.trim()}
                                      aria-label="Save and resend">Save &amp; resend</button>
                                  </div>
                                </div>
                              ) : m.role === 'assistant' && !m.streaming
                                ? <Markdown text={m.text} />
                                : <span>{m.text}{m.streaming && <span className="cursor" role="status" aria-label="Streaming" />}</span>
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
                                    aria-label="Edit and resend"
                                  >✎</button>
                                )}
                                <button
                                  className={`copy-btn ${copiedId === m.id ? 'copied' : ''}`}
                                  onClick={() => copyMessage(m.id, m.text)}
                                  title="Copy to clipboard"
                                  aria-label="Copy message text"
                                >
                                  {copiedId === m.id ? '✓' : '⧉'}
                                </button>
                                <button
                                  className="copy-btn"
                                  onClick={() => branchFromHere(globalIdx)}
                                  disabled={running || !activeId}
                                  title="Branch from here"
                                  aria-label="Branch conversation from this message"
                                >⑂</button>
                              </div>
                            )}
                          </div>
                        </div>
                      )
                    })}
                    {bottomPad > 0 && <div style={{ height: bottomPad }} />}
                  </>
                )
              })()}
              {thinking && running && (
                <div className="msg-row assistant" role="status" aria-label="Agent is thinking">
                  <div className="av assistant" aria-hidden="true">🤖</div>
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
                : <button className="send-btn" onClick={send} disabled={!input.trim() || transcribing || busy}>➤</button>
              }
            </div>
            {busy && !running && (
              <div className="field-hint" role="status" style={{ textAlign: 'center' }}>
                The agent is working in another conversation.{' '}
                <button className="btn secondary" onClick={() => openConv(runConv!)}>Show it</button>{' '}
                <button className="btn danger" onClick={() => StopAgent()}>Stop it</button>
              </div>
            )}
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
        <div className="overlay" onClick={e => e.target === e.currentTarget && setShowNew(false)}
          role="dialog" aria-modal="true" aria-label="New conversation">
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
                  <input value={newPath} onChange={e => setNewPath(e.target.value)} placeholder="~ (home directory)" list="recent-dirs" />
                  <datalist id="recent-dirs">{recentDirs.map(d => <option key={d} value={d} />)}</datalist>
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
      {showTimeline && activeId && <ToolTimeline conversationId={activeId} onClose={() => setShowTimeline(false)} />}

      {showChanges && activeId && <FileChanges conversationId={activeId} onClose={() => setShowChanges(false)} />}

      {showWorkspaces && <WorkspaceManager onClose={() => setShowWorkspaces(false)} />}

      {showUsage && <UsageDashboard conversations={convList} onClose={() => setShowUsage(false)} />}

      {permReq && (
        <div className="overlay" role="dialog" aria-modal="true" aria-label="Permission request">
          <div className="modal">
            <div className="modal-hd">
              <div className="modal-ico ico-perm">🔐</div>
              <div>
                <h2>Permission Request</h2>
                <p>{permReq.operation}{permReq.sensitive ? ' — sensitive action' : ''}</p>
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
                {permReq.target && (
                  <div className="perm-kv">
                    <div className="perm-key">Target</div>
                    <pre className="arg-shell-cmd">{permReq.target}</pre>
                  </div>
                )}
                {permReq.preview && (
                  <div className="perm-kv">
                    <div className="perm-key">Change to be made</div>
                    <DiffView diff={permReq.preview} />
                  </div>
                )}
                <div className="perm-kv">
                  <div className="perm-key">Potential impact</div>
                  <div className="perm-val">{permReq.impact}</div>
                </div>
                <div className="perm-kv">
                  <div className="perm-key">Workspace</div>
                  <div className="perm-val">{permReq.workspace}</div>
                </div>
                <div className="perm-kv">
                  <div className="perm-key">Remembering an approval covers</div>
                  <div className="perm-val">{permReq.scope}</div>
                </div>
                {Object.keys(permReq.args).length > 0 && (
                  <div className="perm-kv">
                    <div className="perm-key">Arguments</div>
                    <div>
                      {Object.entries(permReq.args).map(([k, raw]) => [k, typeof raw === 'string' ? raw : JSON.stringify(raw, null, 2)]).map(([k, v]) => (
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
              <button className="btn danger" onClick={() => answerPermission('deny')}>✕ Deny</button>
              <button className="btn secondary" onClick={() => answerPermission('session')}>Allow this session</button>
              <button className="btn secondary" onClick={() => answerPermission('workspace')}>Always in this workspace</button>
              <button className="btn primary" onClick={() => answerPermission('once')}>✓ Allow once</button>
            </div>
          </div>
        </div>
      )}

      {/* ── Settings modal ── */}
      {showCfg && (
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

            {cfgTab === 'mcp' && mcpStatus.length > 0 && (
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
                  <div className="field-hint">Approximate size of what is sent to the model (default: 8192). History is never deleted.</div>
                </div>

                <div className="field">
                  <label>Execution limits per run (0 = default)</label>
                  <div className="usage-grid">
                    {([['max_rounds', 'Model rounds (50)'], ['max_parallel_tools', 'Tools at once (4)'], ['tool_timeout_seconds', 'Seconds per tool (360)']] as const).map(([k, label]) => (
                      <label key={k}>{label}
                        <input type="number" min={0} value={cfg[k] || 0}
                          onChange={e => setCfg(c => ({ ...c, [k]: Math.max(0, Number(e.target.value)) }))} />
                      </label>
                    ))}
                  </div>
                </div>

                <div className="field">
                  <label>Diagnostics</label>
                  <button className="btn secondary" onClick={() => ExportDiagnostics().then(r => setBackupMsg(r === 'ok' ? '✓ Diagnostics saved' : r ? '✗ ' + r : ''))}>Export diagnostics…</button>
                  <div className="field-hint">Versions, settings without keys or addresses, MCP state and the app log. No conversation content. Nothing is sent anywhere.</div>
                </div>

                <div className="field">
                  <label htmlFor="context-strategy">Older turns</label>
                  <select id="context-strategy" value={cfg.context_strategy || 'sliding'}
                    onChange={e => setCfg(c => ({ ...c, context_strategy: e.target.value }))}>
                    <option value="sliding">Sliding window — leave them out</option>
                    <option value="hybrid">Hybrid — keep what fits, summarise the rest</option>
                    <option value="summary">Summary — keep two turns, summarise the rest</option>
                  </select>
                  <div className="field-hint">Summaries are written by the model, cost extra requests and can be wrong.</div>
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

                <ModelSettingsPanel provider={cfg.provider} model={cfg.model} onModels={setModels} />

                <div className="field">
                  <label htmlFor="tool-calling">Tool calling</label>
                  <select id="tool-calling" value={cfg.tool_calling || 'auto'}
                    onChange={e => setCfg(c => ({ ...c, tool_calling: e.target.value }))}>
                    <option value="auto">Automatic — native when the model supports it</option>
                    <option value="text">Text protocol</option>
                  </select>
                  <div className="field-hint">Choose the text protocol if a model claims to support tools but uses them badly.</div>
                </div>

                <div className="field">
                  <label htmlFor="sandbox-mode">Shell sandbox</label>
                  <select id="sandbox-mode" value={cfg.sandbox_mode || 'auto'}
                    onChange={e => setCfg(c => ({ ...c, sandbox_mode: e.target.value }))}>
                    <option value="auto">Automatic — isolate when the system supports it</option>
                    <option value="strict">Strict — refuse to run without isolation</option>
                    <option value="off">Off — commands can reach the whole computer</option>
                  </select>
                  <label className="mcp-toggle">
                    <input type="checkbox" checked={!!cfg.sandbox_network}
                      onChange={e => setCfg(c => ({ ...c, sandbox_network: e.target.checked }))} />
                    <span>Allow network access inside the sandbox</span>
                  </label>
                  <div className="field-hint" role="status">
                    {sandboxStatus === null ? 'Checking isolation…'
                      : sandboxStatus.isolated
                        ? `Saved settings: isolated with ${sandboxStatus.backend}, network ${sandboxStatus.network_blocked ? 'blocked' : 'allowed'}.`
                        : `Saved settings: NOT isolated (${sandboxStatus.detail}).`}
                  </div>
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
                  <label htmlFor="backup-passphrase">Data Backup</label>
                  <input id="backup-passphrase" type="password" autoComplete="new-password"
                    placeholder="Backup passphrase (8+ characters)"
                    value={passphrase} onChange={e => setPassphrase(e.target.value)} />
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button className="btn secondary" onClick={async () => {
                      setBackupMsg('')
                      const result = await ExportBackup(passphrase)
                      if (result === 'ok') setBackupMsg('✓ Encrypted backup saved')
                      else if (result) setBackupMsg('✗ ' + result)
                    }}>Export backup…</button>
                    <button className={`btn ${confirming === 'restore' ? 'danger' : 'secondary'}`} onClick={async () => {
                      if (confirming !== 'restore') { setConfirming('restore'); return }
                      setConfirming('')
                      setBackupMsg('')
                      const result = await ImportBackup(passphrase)
                      if (result === 'ok') setBackupMsg('✓ Backup checked — quit and reopen the app to apply it')
                      else if (result) setBackupMsg('✗ ' + result)
                    }}>{confirming === 'restore' ? 'Click again to choose a backup' : 'Restore backup…'}</button>
                  </div>
                  {confirming === 'restore' && (
                    <div role="alert" className="field-hint">
                      Restoring replaces all conversations and settings at the next start. A copy of the current data is kept in the data folder.
                    </div>
                  )}
                  {backupMsg && (
                    <div role="status" className="field-hint" style={{ color: backupMsg.startsWith('✓') ? 'var(--green)' : 'var(--red)' }}>
                      {backupMsg}
                    </div>
                  )}
                  <div className="field-hint">
                    Exports are encrypted with the passphrase, which cannot be recovered. Automatic backups
                    in <code>data/backups/</code> contain no key, so they only restore while this
                    installation still has its key.
                  </div>
                  <label htmlFor="backup-retention">Automatic backups to keep</label>
                  <input id="backup-retention" type="number" min={1} max={100} value={cfg.backup_retention || 7}
                    onChange={e => setCfg(c => ({ ...c, backup_retention: Number(e.target.value) }))} />
                </div>

                <div className="field">
                  <label htmlFor="key-location">Encryption key</label>
                  <select id="key-location" value={confirming === 'keychain' ? 'keychain' : keyLocation} onChange={e => {
                    if (e.target.value === 'keychain') { setConfirming('keychain'); return }
                    moveKey('file')
                  }}>
                    <option value="file">File beside the database (portable)</option>
                    <option value="keychain">System keychain</option>
                  </select>
                  {confirming === 'keychain' && keyLocation !== 'keychain' && (
                    <div role="alert" style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                      <span className="field-hint">The data folder will then only open under this user account. Export a passphrase backup first.</span>
                      <button className="btn danger" onClick={() => moveKey('keychain')}>Move key</button>
                      <button className="btn secondary" onClick={() => setConfirming('')}>Cancel</button>
                    </div>
                  )}
                  {keyLocation === 'file' && (
                    <button className={`btn ${confirming === 'rotate' ? 'danger' : 'secondary'}`} onClick={async () => {
                      if (confirming !== 'rotate') { setConfirming('rotate'); return }
                      setConfirming('')
                      const err = await RotateEncryptionKey()
                      setKeyMsg(err || 'Key rotated. Export a new backup: older automatic backups can no longer be restored.')
                    }}>{confirming === 'rotate' ? 'Click again to rotate' : 'Rotate key'}</button>
                  )}
                  {keyMsg && <div role="alert" className="field-hint" style={{ color: 'var(--red)' }}>{keyMsg}</div>}
                  <div className="field-hint">
                    With the key in a file, anyone who can read the data folder can read your conversations.
                    Backups made by earlier versions also contain the key; delete them if that matters.
                  </div>
                </div>

                <div className="field">
                  <label>Import Conversations</label>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button className="btn secondary" onClick={importConversation}
                      aria-label="Import conversations from file"
                    >Import conversations…</button>
                    <button className="btn secondary" onClick={() => setImportMsg('')}
                      aria-label="Dismiss import message"
                      style={{ display: importMsg ? 'inline-block' : 'none' }}>✕</button>
                  </div>
                  {importMsg && (
                    <div className="field-hint" style={{ color: importMsg.startsWith('✓') ? 'var(--green)' : 'var(--text2)' }}>
                      {importMsg}
                    </div>
                  )}
                  <div className="field-hint">
                    Accepts <code>.json</code> from this app, ChatGPT <code>conversations.json</code>, or Claude exports.
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
