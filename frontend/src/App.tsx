import { useState, useEffect, useRef } from 'react'
import './style.css'
import { uid, useChatStream } from './hooks/useChatStream'
import { useSpeech } from './hooks/useSpeech'
import {
  SendMessage, StopAgent, ListConversations, NewConversation, LoadConversation, UpdateConversationPath, ExportConversation, SetConversationModel, TruncateAndResend, ListModels, ListTools, GetConfig, GetStartupError, PickDirectory, ListMCPServers, CheckForUpdate, GetVersion, GetContextUsage, BranchConversation,
} from '../wailsjs/go/main/App'
import { ChatHeader, type Panel } from './components/chat/ChatHeader'
import { Composer } from './components/chat/Composer'
import { Transcript } from './components/chat/Transcript'
import { Icon } from './components/Icon'
import { WorkspaceManager } from './components/WorkspaceManager'
import { ConversationSidebar } from './components/ConversationSidebar'
import { NewConversationModal } from './components/NewConversationModal'
import { PermissionDialog } from './components/PermissionDialog'
import { FileChanges } from './components/FileChanges'
import { ToolTimeline } from './components/ToolTimeline'
import { UsageDashboard } from './components/UsageDashboard'
import { SettingsModal } from './components/SettingsModal'
import type { ConvMeta, SavedConv, ToolInfo, MsgRole, Item, MCPServer, Cfg, UpdateInfo } from './types'
import { EventsOn } from '../wailsjs/runtime/runtime'

// Stored transcript items keep their text under data; the view does not for
// error and status rows.
const fromStored = (it: any, id: number): Item =>
  it.type === 'error' || it.type === 'status'
    ? { type: it.type, id, text: it.data.text }
    : { ...it, data: { ...it.data, id, streaming: !!it.data.streaming } }

export default function App() {
  const [convList, setConvList]       = useState<ConvMeta[]>([])
  const [activeId, setActiveId]       = useState<string | null>(null)
  const [activeTitle, setActiveTitle] = useState('')
  const [activePath, setActivePath]   = useState('')

  const chat = useChatStream({ notify: (t, b) => notify(t, b), onRunEnd: () => refreshList() })
  const { items, setItems, thinking, runConv, ctxUsage, permReq, answerPermission } = chat
  const speech = useSpeech(text => setInput(prev => (prev ? prev + ' ' : '') + text))
  const { whisperReady } = speech
  const [input, setInput]     = useState('')
  // The single run may belong to a conversation other than the one shown.
  const [showUsage, setShowUsage] = useState(false)
  const [panel, setPanel] = useState<Panel>(null)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem('sidebar-collapsed') === 'true')
  const [modelError, setModelError] = useState('')
  const [showWorkspaces, setShowWorkspaces] = useState(false)
  const running = runConv !== null && runConv === activeId
  const busy = runConv !== null
  // Events arriving while a conversation loads are held and replayed.
  const [loadingConv, setLoadingConv] = useState(false)

  const [showNew, setShowNew]     = useState(false)
  const [showCfg, setShowCfg]     = useState(false)

  const [fatalError, setFatalError] = useState('')
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)
  const [updateDismissed, setUpdateDismissed] = useState(false)
  const [appVersion, setAppVersion] = useState('')
  // OS notifications — a ref so listeners registered once read the live setting
  const notifyEnabledRef = useRef(true)

  // per-conversation model selector
  const [convModel, setConvModel] = useState('')

  const [mcpServers, setMcpServers] = useState<MCPServer[]>([])
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
    checkModels()
    ListTools().then((t: any) => setToolList(t || [])).catch(() => {})
    ListMCPServers().then((s: any) => setMcpServers(s || [])).catch(() => {})
    GetVersion().then(v => setAppVersion(v)).catch(() => {})
    CheckForUpdate().then((info: any) => { if (info?.available) setUpdateInfo(info) }).catch(() => {})
    refreshList()
  }, [])

  // Keyboard shortcuts: Ctrl or Cmd with N, K or comma.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.shiftKey || e.altKey) return
      const action = {
        n: () => setShowNew(true),
        ',': () => setShowCfg(true),
        k: () => { setSidebarCollapsed(false); setTimeout(() => document.querySelector<HTMLInputElement>('.search-input')?.focus()) },
      }[e.key.toLowerCase()]
      if (!action) return
      e.preventDefault()
      action()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => localStorage.setItem('sidebar-collapsed', String(sidebarCollapsed)), [sidebarCollapsed])

  // ── Events ──────────────────────────────────────────────

  useEffect(() => {
    const off = EventsOn('app:fatal', (msg: string) => {
      setFatalError(msg)
      notify('Fatal error', msg)
    })
    GetStartupError().then(msg => { if (msg) setFatalError(msg) }).catch(() => {})
    return off
  }, [])

  // ── Helpers ─────────────────────────────────────────────

  const checkModels = () => ListModels().then((r: any) => {
    setModels(r.models || [])
    setModelError(r.error || '')
  }).catch(() => {})

  const refreshList = () => ListConversations().then((l: any) => setConvList(l || []))

  const openConv = async (id: string) => {
    const token = chat.beginLoad(id)
    setLoadingConv(true)
    let conv: SavedConv & { running?: boolean; seq?: number }
    try {
      conv = await LoadConversation(id) as any
    } catch (err) {
      chat.abortLoad(token)
      throw err
    } finally {
      if (!chat.isStale(token)) setLoadingConv(false)
    }
    if (chat.isStale(token)) return // a newer load replaced this one

    setActiveId(conv.id); setActiveTitle(conv.title); setActivePath(conv.work_dir)
    setConvModel((conv as any).model || '')
    const loaded: Item[] = conv.display_items?.length
      ? conv.display_items.map((it: any) => fromStored(it, uid()))
      : conv.messages
        .filter(m => (m.role === 'user' || m.role === 'assistant') && !m.content.startsWith('TOOL_RESULT'))
        .map(m => ({ type: 'msg' as const, data: { id: uid(), role: m.role as MsgRole, text: m.content, streaming: false } }))
    chat.commitLoad(token, loaded, !!conv.running, conv.seq || 0)
    GetContextUsage().then((u: any) => chat.setCtxUsage(u)).catch(() => {})
  }

  const createConv = async (workDir: string) => {
    const conv = await NewConversation(workDir) as SavedConv
    setShowNew(false)
    await refreshList(); await openConv(conv.id)
  }







  const changePath = async () => {
    const dir = await PickDirectory()
    if (!dir) return
    await UpdateConversationPath(dir)
    setActivePath(dir)
    setConvList(prev => prev.map(c => c.id === activeId ? { ...c, work_dir: dir } : c))
  }

  const send = () => {
    const text = input.trim()
    if (!text || busy || !activeId) return
    setItems(prev => [...prev, { type: 'msg', data: { id: uid(), role: 'user', text } }])
    setInput(''); chat.startedRun(activeId)
    SendMessage(activeId, text).then(r => chat.runRejected(activeId, r.error))
  }

  // Resubmit an edited user message: truncate the transcript to that point,
  // replace the message, and re-run the agent from there.
  const resend = (id: number, text: string) => {
    if (busy || !activeId) return
    const idx = items.findIndex(it => it.type === 'msg' && it.data.id === id)
    if (idx < 0) return
    // userOrdinal = how many user messages precede this one in the transcript.
    const userOrdinal = items.slice(0, idx).filter(it => it.type === 'msg' && it.data.role === 'user').length
    setItems(prev => [...prev.slice(0, idx), { type: 'msg', data: { id: uid(), role: 'user', text } }])
    chat.startedRun(activeId)
    TruncateAndResend(activeId, userOrdinal, text).then(r => chat.runRejected(activeId, r.error))
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

  const model = convModel || cfg.model
  const problem = !model ? 'No model is selected, so the agent cannot reply yet.'
    : modelError ? `The model provider is not responding: ${modelError}` : ''
  const closeSettings = () => { setShowCfg(false); checkModels() }

  return (
    <div className="layout">
      {fatalError && (
        <div className="banner fatal-banner" role="alert">
          <Icon name="alert" />
          <span><strong>Startup error:</strong> {fatalError}</span>
          <button className="icon-btn" onClick={() => setFatalError('')} aria-label="Dismiss"><Icon name="x" /></button>
        </div>
      )}
      {updateInfo && !updateDismissed && !fatalError && (
        <div className="banner" role="status">
          <span><strong>v{updateInfo.latest}</strong> is available; you have {updateInfo.current}.</span>
          <a href={updateInfo.url} target="_blank" rel="noreferrer">Download</a>
          <button className="icon-btn" onClick={() => setUpdateDismissed(true)} aria-label="Dismiss"><Icon name="x" /></button>
        </div>
      )}

      <ConversationSidebar convList={convList} activeId={activeId} model={cfg.model} online={!modelError}
        collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed(c => !c)}
        onOpen={openConv} refreshList={refreshList}
        onDeleted={id => {
          if (activeId !== id) return
          chat.clear(); setActiveId(null); setActiveTitle(''); setActivePath(''); setPanel(null)
        }}
        onRenamed={(id, title) => { if (activeId === id) setActiveTitle(title) }}
        onNew={() => setShowNew(true)} onSettings={() => setShowCfg(true)} onUsage={() => setShowUsage(true)}
        onWorkspaces={() => setShowWorkspaces(true)} />

      <main className="main" aria-label="Chat area">
        {activeId ? (
          <>
            <ChatHeader conversationId={activeId} title={activeTitle} path={activePath} model={convModel}
              defaultModel={cfg.model} models={models} panel={panel} profilesVersion={showWorkspaces}
              onModel={changeConvModel} onPath={changePath} onPanel={setPanel} onExport={exportConv} />
            <div className="workspace">
              <div className="chat">
                <Transcript key={activeId} items={items} loading={loadingConv} running={running} thinking={thinking}
                  canAct={!busy} onSuggest={setInput} onResend={resend} onBranch={branchFromHere} />
                <div className="composer">
                  <div className="composer-inner">
                    {busy && !running && (
                      <div className="notice" role="status">
                        <Icon name="clock" />
                        <span className="notice-text">The agent is working in another conversation.</span>
                        <button className="btn" onClick={() => openConv(runConv!)}>Show it</button>
                        <button className="btn danger" onClick={() => StopAgent()}>Stop it</button>
                      </div>
                    )}
                    {problem && (
                      <div className="notice" role="status">
                        <Icon name="alert" />
                        <span className="notice-text">{problem}</span>
                        <button className="btn" onClick={() => setShowCfg(true)}>Open settings</button>
                      </div>
                    )}
                    <Composer value={input} onChange={setInput} onSend={send} running={running} busy={busy}
                      usage={ctxUsage} speech={speech} />
                  </div>
                </div>
              </div>
              {panel === 'activity' && <ToolTimeline conversationId={activeId} onClose={() => setPanel(null)} />}
              {panel === 'changes' && <FileChanges conversationId={activeId} onClose={() => setPanel(null)} />}
            </div>
          </>
        ) : (
          <div className="no-conv">
            <div className="empty-icon"><Icon name="spark" size={22} /></div>
            <h2>{convList.length ? 'Pick up where you left off' : 'Welcome to your local agent'}</h2>
            <p>{convList.length
              ? 'Choose a conversation on the left, or start a new one.'
              : 'Start a conversation in any folder. Your history stays on this computer.'}</p>
            <button className="btn primary" onClick={() => setShowNew(true)}><Icon name="plus" />New conversation</button>
            {modelError && (
              <div className="notice" role="status">
                <Icon name="alert" />
                <span className="notice-text">The model provider is not responding.</span>
                <button className="btn" onClick={() => setShowCfg(true)}>Open settings</button>
              </div>
            )}
          </div>
        )}
      </main>

      {showNew && <NewConversationModal onCreate={createConv} onClose={() => setShowNew(false)} />}
      {showWorkspaces && <WorkspaceManager onClose={() => setShowWorkspaces(false)} />}
      {showUsage && <UsageDashboard conversations={convList} onClose={() => setShowUsage(false)} />}
      {permReq && <PermissionDialog request={permReq} onAnswer={answerPermission} />}
      {showCfg && (
        <SettingsModal cfg={cfg} setCfg={setCfg} models={models} setModels={setModels} toolList={toolList}
          mcpServers={mcpServers} setMcpServers={setMcpServers} whisperReady={whisperReady}
          appVersion={appVersion} onImported={refreshList} onClose={closeSettings} />
      )}
    </div>
  )
}
