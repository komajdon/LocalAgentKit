import { useState, useEffect, useRef } from 'react'
import {
  DeleteConversation, RenameConversation, SearchConversations, SetConversationPinned, SetConversationTags,
} from '../../wailsjs/go/main/App'
import type { ConvMeta } from '../types'
import { groupConvs, shortPath, timeAgo } from '../format'

const CONV_PAGE = 100

interface Props {
  convList: ConvMeta[]
  activeId: string | null
  model: string
  onOpen: (id: string) => void
  onDeleted: (id: string) => void
  onRenamed: (id: string, title: string) => void
  refreshList: () => void
  onNew: () => void
  onSettings: () => void
  onUsage: () => void
  onTimeline: () => void
  onChanges: () => void
  onWorkspaces: () => void
}

export function ConversationSidebar({
  convList, activeId, model, onOpen, onDeleted, onRenamed, refreshList,
  onNew, onSettings, onUsage, onTimeline, onChanges, onWorkspaces,
}: Props) {
  const [convLimit, setConvLimit] = useState(CONV_PAGE)
  const [renamingId, setRenamingId]       = useState<string | null>(null)
  const [renameValue, setRenameValue]     = useState('')
  const [tagFilter, setTagFilter]       = useState<string | null>(null)
  const [tagEditId, setTagEditId]       = useState<string | null>(null)
  const [tagEditValue, setTagEditValue] = useState('')
  const [searchQuery, setSearchQuery]   = useState('')
  const [searchResults, setSearchResults] = useState<ConvMeta[] | null>(null)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => localStorage.getItem('sidebar-collapsed') === 'true')
  const renameInputRef                    = useRef<HTMLInputElement>(null)
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [renamingId])

  const delConv = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    await DeleteConversation(id)
    onDeleted(id)
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
    onRenamed(renamingId, renameValue.trim())
    setRenamingId(null)
    refreshList()
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

  const allTags = Array.from(new Set(convList.flatMap(c => c.tags || []))).sort()
  const matching = tagFilter ? convList.filter(c => (c.tags || []).includes(tagFilter)) : convList
  // Large histories are rendered a page at a time; pinned ones always show.
  const visibleConvs = matching.filter((c, i) => i < convLimit || c.pinned || c.id === activeId)
  const hiddenConvs = matching.length - visibleConvs.length
  const groups = groupConvs(visibleConvs)

  const renderGroup = (label: string, list: ConvMeta[]) =>
    list.length === 0 ? null : (
      <div className="conv-group">
        <div className="conv-group-label">{label}</div>
        {list.map(c => (
          <div key={c.id} className={`conv-row ${c.id === activeId ? 'active' : ''}`}
            onClick={() => c.id !== activeId && onOpen(c.id)}>
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
    <>
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
            ? <button className="new-btn collapsed" onClick={() => onNew()} aria-label="New conversation">＋</button>
            : <button className="new-btn" onClick={() => onNew()} aria-label="New conversation">＋ New conversation</button>
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
                  : renderGroup(`Results (${searchResults.length})`, searchResults)
              ) : convList.length === 0
                ? <div className="conv-empty">No conversations yet.<br />Click "+ New conversation" to start.</div>
                : <>
                    {renderGroup("📌 Pinned", groups.pinned)}
                    {renderGroup("Today", groups.today)}
                    {renderGroup("This week", groups.week)}
                    {renderGroup("Older", groups.older)}
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
            <button className="foot-btn collapsed" onClick={() => onSettings()}
              aria-label="Settings">⚙️</button>
          ) : (
            <>
              <button className="foot-btn" onClick={() => onUsage()}>📊 Usage</button>
            <button className="foot-btn" onClick={() => onTimeline()} disabled={!activeId}>🧭 Tool timeline</button>
            <button className="foot-btn" onClick={() => onChanges()} disabled={!activeId}>📝 File changes</button>
            <button className="foot-btn" onClick={() => onWorkspaces()}>🗂 Workspaces &amp; profiles</button>
              <button className="foot-btn" onClick={() => onSettings()}
                aria-label="Settings">⚙️ Settings</button>
              <div className="conn-badge">
                <div className="dot" />
                <span>{model || 'No model selected'}</span>
              </div>
            </>
          )}
        </div>
      </aside>
    </>
  )
}
