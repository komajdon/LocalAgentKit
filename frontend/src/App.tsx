import { useState, useEffect, useRef, useCallback } from 'react'
import { Markdown } from './Markdown'
import './style.css'
import { shortPath } from './format'
import { uid, useChatStream } from './hooks/useChatStream'
import { useSpeech } from './hooks/useSpeech'
import {
  SendMessage, StopAgent, ListConversations, NewConversation, LoadConversation, UpdateConversationPath, ExportConversation, SetConversationModel, TruncateAndResend, ListModels, ListTools, GetConfig, GetStartupError, PickDirectory, ListMCPServers, CheckForUpdate, GetVersion, GetContextUsage, BranchConversation,
} from '../wailsjs/go/main/App'
import { ProfilePicker } from './components/ProfilePicker'
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

// ── Types ─────────────────────────────────────────────────────────────────

// Stored transcript items keep their text under data; the view does not for
// error and status rows.
const fromStored = (it: any, id: number): Item =>
  it.type === 'error' || it.type === 'status'
    ? { type: it.type, id, text: it.data.text }
    : { ...it, data: { ...it.data, id, streaming: !!it.data.streaming } }






// ── App ───────────────────────────────────────────────────────────────────

export default function App() {
  const [convList, setConvList]       = useState<ConvMeta[]>([])
  const [activeId, setActiveId]       = useState<string | null>(null)
  const [activeTitle, setActiveTitle] = useState('')
  const [activePath, setActivePath]   = useState('')

  const chat = useChatStream({ notify: (t, b) => notify(t, b), onRunEnd: () => refreshList() })
  const { items, setItems, thinking, runConv, ctxUsage, permReq, answerPermission } = chat
  const speech = useSpeech(text => setInput(prev => (prev ? prev + ' ' : '') + text))
  const { whisperReady, recording, transcribing, whisperStatus, sttError, setSttError, startRecording, stopRecording } = speech
  const [input, setInput]     = useState('')
  // The single run may belong to a conversation other than the one shown.
  const [showUsage, setShowUsage] = useState(false)
  const [showTimeline, setShowTimeline] = useState(false)
  const [showChanges, setShowChanges] = useState(false)
  const [showWorkspaces, setShowWorkspaces] = useState(false)
  const running = runConv !== null && runConv === activeId
  const busy = runConv !== null
  // Events arriving while a conversation loads are held and replayed.
  const [loadingConv, setLoadingConv] = useState(false)

  const [showNew, setShowNew]     = useState(false)
  const [showCfg, setShowCfg]     = useState(false)

  // rename state

  // copy feedback
  const [copiedId, setCopiedId] = useState<number | null>(null)

  // message editing (edit & resend)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editValue, setEditValue] = useState('')

  // tag filter + inline tag editor


  // speech-to-text

  // context usage
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
    ListMCPServers().then((s: any) => setMcpServers(s || [])).catch(() => {})
    GetVersion().then(v => setAppVersion(v)).catch(() => {})
    CheckForUpdate().then((info: any) => { if (info?.available) setUpdateInfo(info) }).catch(() => {})
    refreshList()
  }, [])

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

  const send = useCallback(() => {
    const text = input.trim()
    if (!text || busy || !activeId) return
    setItems(prev => [...prev, { type: 'msg', data: { id: uid(), role: 'user', text } }])
    setInput(''); chat.startedRun(activeId)
    SendMessage(activeId, text).then(r => chat.runRejected(activeId, r.error))
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
    chat.startedRun(activeId)
    TruncateAndResend(activeId, userOrdinal, text).then(r => chat.runRejected(activeId, r.error))
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

      <ConversationSidebar convList={convList} activeId={activeId} model={cfg.model} onOpen={openConv} refreshList={refreshList}
        onDeleted={id => {
          if (activeId !== id) return
          chat.clear(); setActiveId(null); setActiveTitle(''); setActivePath('')
        }}
        onRenamed={(id, title) => { if (activeId === id) setActiveTitle(title) }}
        onNew={() => setShowNew(true)} onSettings={() => setShowCfg(true)} onUsage={() => setShowUsage(true)}
        onTimeline={() => setShowTimeline(true)} onChanges={() => setShowChanges(true)} onWorkspaces={() => setShowWorkspaces(true)} />

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

      {showNew && <NewConversationModal onCreate={createConv} onClose={() => setShowNew(false)} />}

      {showTimeline && activeId && <ToolTimeline conversationId={activeId} onClose={() => setShowTimeline(false)} />}

      {showChanges && activeId && <FileChanges conversationId={activeId} onClose={() => setShowChanges(false)} />}

      {showWorkspaces && <WorkspaceManager onClose={() => setShowWorkspaces(false)} />}

      {showUsage && <UsageDashboard conversations={convList} onClose={() => setShowUsage(false)} />}

      {permReq && <PermissionDialog request={permReq} onAnswer={answerPermission} />}

      {showCfg && (
        <SettingsModal cfg={cfg} setCfg={setCfg} models={models} setModels={setModels} toolList={toolList}
          mcpServers={mcpServers} setMcpServers={setMcpServers} whisperReady={whisperReady}
          appVersion={appVersion} onImported={refreshList} onClose={() => setShowCfg(false)} />
      )}
    </div>
  )
}
