import { useState, useEffect, useRef } from 'react'
import {
  DeleteConversation, RenameConversation, SearchConversations, SetConversationPinned, SetConversationTags,
} from '../../wailsjs/go/main/App'
import { activate } from '../activate'
import { Icon } from './Icon'
import type { ConvMeta } from '../types'
import { groupConvs, shortPath, timeAgo } from '../format'

const CONV_PAGE = 100

interface Props {
  convList: ConvMeta[]
  activeId: string | null
  model: string
  /** Whether the model provider answered. */
  online: boolean
  collapsed: boolean
  onToggle: () => void
  onOpen: (id: string) => void
  onDeleted: (id: string) => void
  onRenamed: (id: string, title: string) => void
  refreshList: () => void
  onNew: () => void
  onSettings: () => void
  onUsage: () => void
  onWorkspaces: () => void
}

export function ConversationSidebar({
  convList, activeId, model, online, collapsed, onToggle, onOpen, onDeleted, onRenamed, refreshList,
  onNew, onSettings, onUsage, onWorkspaces,
}: Props) {
  const [convLimit, setConvLimit] = useState(CONV_PAGE)
  const [renamingId, setRenamingId]       = useState<string | null>(null)
  const [renameValue, setRenameValue]     = useState('')
  const [tagFilter, setTagFilter]       = useState<string | null>(null)
  const [tagEditId, setTagEditId]       = useState<string | null>(null)
  const [tagEditValue, setTagEditValue] = useState('')
  const [searchQuery, setSearchQuery]   = useState('')
  const [searchResults, setSearchResults] = useState<ConvMeta[] | null>(null)
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
            aria-current={c.id === activeId} {...activate(() => { if (c.id !== activeId) onOpen(c.id) }, `Open ${c.title}`)}>
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
                  {c.pinned && <span className="conv-pin-mark"><Icon name="pin" size={12} /></span>}{c.title}
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
                      {...activate(() => setTagFilter(tagFilter === t ? null : t))}>{t}</span>
                  ))}
                </div>
              )}
              <div className="conv-info">
                <span>{shortPath(c.work_dir)}</span>
                <span className="conv-time">{timeAgo(c.updated_at)}</span>
              </div>
            </div>
            <div className="conv-row-actions">
              <button className="icon-btn" onClick={e => togglePin(e, c)} title={c.pinned ? 'Unpin' : 'Pin'} aria-label={c.pinned ? 'Unpin' : 'Pin'}><Icon name="pin" size={13} /></button>
              <button className="icon-btn" onClick={e => startRename(e, c)} title="Rename" aria-label="Rename"><Icon name="edit" size={13} /></button>
              <button className="icon-btn" onClick={e => startTagEdit(e, c)} title="Edit tags" aria-label="Edit tags"><Icon name="tag" size={13} /></button>
              <button className="icon-btn danger" onClick={e => delConv(e, c.id)} title="Delete" aria-label="Delete"><Icon name="trash" size={13} /></button>
            </div>
          </div>
        ))}
      </div>
    )

  const search = (
    <div className="sidebar-search">
      <Icon name="search" size={14} />
      <input className="search-input" placeholder="Search" value={searchQuery}
        onChange={e => runSearch(e.target.value)} aria-label="Search conversations" />
      {searchQuery
        ? <button className="icon-btn search-clear" onClick={() => runSearch('')} aria-label="Clear search"><Icon name="x" size={13} /></button>
        : <kbd>Ctrl K</kbd>}
    </div>
  )

  if (collapsed) {
    return (
      <aside className="sidebar collapsed" aria-label="Conversations">
        <div className="sidebar-head">
          <button className="icon-btn" onClick={onToggle} aria-label="Expand sidebar" title="Expand sidebar"><Icon name="panel" /></button>
          <button className="icon-btn" onClick={onNew} aria-label="New conversation" title="New conversation (Ctrl N)"><Icon name="plus" /></button>
        </div>
        <div className="sidebar-foot">
          <button className="icon-btn" onClick={onUsage} aria-label="Usage" title="Usage"><Icon name="chart" /></button>
          <button className="icon-btn" onClick={onWorkspaces} aria-label="Workspaces and profiles" title="Workspaces and profiles"><Icon name="layers" /></button>
          <button className="icon-btn" onClick={onSettings} aria-label="Settings" title="Settings (Ctrl ,)"><Icon name="settings" /></button>
        </div>
      </aside>
    )
  }

  return (
    <aside className="sidebar" aria-label="Conversations">
      <div className="sidebar-head">
        <div className="brand">
          <div className="brand-icon"><Icon name="spark" /></div>
          <div className="brand-text">
            <div className="brand-name">AI Agent</div>
            <div className="brand-sub">Powered by Komajdon</div>
          </div>
          <button className="icon-btn" onClick={onToggle} aria-label="Collapse sidebar" title="Collapse sidebar"><Icon name="panel" /></button>
        </div>
        <button className="new-btn" onClick={onNew}><Icon name="plus" />New conversation<kbd>Ctrl N</kbd></button>
        {search}
      </div>

      {allTags.length > 0 && searchResults === null && (
        <div className="tag-filter-bar">
          {allTags.map(t => (
            <span key={t} className={`conv-tag ${tagFilter === t ? 'active' : ''}`}
              {...activate(() => setTagFilter(tagFilter === t ? null : t))}>{t}</span>
          ))}
          {tagFilter && <span className="tag-filter-clear" {...activate(() => setTagFilter(null))}>clear</span>}
        </div>
      )}

      <div className="conv-list" role="list" aria-label="Conversation list">
        {searchResults !== null ? (
          searchResults.length === 0
            ? <div className="conv-empty">No results for "{searchQuery}"</div>
            : renderGroup(`Results (${searchResults.length})`, searchResults)
        ) : convList.length === 0
          ? <div className="conv-empty">Your conversations will appear here.</div>
          : <>
              {renderGroup('Pinned', groups.pinned)}
              {renderGroup('Today', groups.today)}
              {renderGroup('This week', groups.week)}
              {renderGroup('Older', groups.older)}
              {hiddenConvs > 0 && (
                <button className="btn ghost" onClick={() => setConvLimit(n => n + CONV_PAGE)}>Show more ({hiddenConvs})</button>
              )}
            </>}
      </div>

      <nav className="sidebar-foot" aria-label="App">
        <button className="foot-btn" onClick={onUsage}><Icon name="chart" />Usage</button>
        <button className="foot-btn" onClick={onWorkspaces}><Icon name="layers" />Workspaces &amp; profiles</button>
        <button className="foot-btn" onClick={onSettings}><Icon name="settings" />Settings</button>
        <div className="conn-badge" title={online ? 'Default model' : 'The model provider is not responding'}>
          <span className={`dot ${online && model ? '' : 'off'}`} />
          <span>{model || 'No model selected'}</span>
        </div>
      </nav>
    </aside>
  )
}
