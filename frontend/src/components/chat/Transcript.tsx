import { useEffect, useRef, useState } from 'react'
import { Markdown } from '../../Markdown'
import type { Item } from '../../types'
import { Icon, type IconName } from '../Icon'
import { toBlocks, type Block } from './blocks'
import { ToolActivity } from './ToolActivity'

const OVERSCAN_PX = 480

// Rough heights, used to skip drawing blocks that are far from view.
const estimate = (b: Block) => {
  if (b.kind === 'tools') return 36 * b.steps.length + 16
  if (b.kind !== 'msg') return 44
  return Math.max(60, Math.min(500, b.msg.text.length * 0.3 + 44))
}

const SUGGESTIONS: [IconName, string][] = [
  ['folder', 'Give me a tour of this folder'],
  ['search', 'Find TODOs in the code and summarise them'],
  ['file', 'Summarise the documents in this folder'],
  ['terminal', 'Run the tests and explain any failures'],
]

interface Props {
  items: Item[]
  loading: boolean
  running: boolean
  thinking: boolean
  canAct: boolean
  onSuggest: (text: string) => void
  onResend: (messageId: number, text: string) => void
  onBranch: (itemIndex: number) => void
}

export function Transcript({ items, loading, running, thinking, canAct, onSuggest, onResend, onBranch }: Props) {
  const container = useRef<HTMLDivElement>(null)
  const bottom = useRef<HTMLDivElement>(null)
  const nearBottom = useRef(true)
  const [scrollTop, setScrollTop] = useState(0)
  const [editing, setEditing] = useState<number | null>(null)
  const [draft, setDraft] = useState('')
  const [copied, setCopied] = useState<number | null>(null)

  useEffect(() => {
    if (nearBottom.current) bottom.current?.scrollIntoView?.({ behavior: 'smooth' })
  }, [items, thinking])

  const blocks = toBlocks(items)
  const heights = blocks.map(estimate)
  const viewBottom = scrollTop + (container.current?.clientHeight || 800) + OVERSCAN_PX
  let top = 0, start = 0
  while (start < blocks.length - 1 && top + heights[start] < scrollTop - OVERSCAN_PX) top += heights[start++]
  let end = start, y = top
  while (end < blocks.length && y < viewBottom) y += heights[end++]
  const below = heights.slice(end).reduce((a, b) => a + b, 0)

  const copy = (id: number, text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(id)
      setTimeout(() => setCopied(null), 1600)
    }).catch(() => {})
  }
  const resend = (id: number) => {
    const text = draft.trim()
    setEditing(null)
    if (text) onResend(id, text)
  }

  const render = (block: Block) => {
    if (block.kind === 'tools') return <ToolActivity key={block.id} steps={block.steps} running={running} />
    if (block.kind === 'error') {
      return <div key={block.id} className="error-row" role="alert"><Icon name="alert" /><span>{block.text}</span></div>
    }
    if (block.kind === 'status') return <div key={block.id} className="status-row" role="status">{block.text}</div>

    const m = block.msg
    const isEditing = editing === m.id
    return (
      <div key={m.id} className={`msg-row ${m.role}`} role="article" aria-label={`${m.role} message`}>
        <div className="bubble-wrap">
          <div className="bubble">
            {isEditing ? (
              <div className="msg-edit">
                <textarea autoFocus value={draft} aria-label="Edit message"
                  rows={Math.min(8, draft.split('\n').length + 1)}
                  onChange={e => setDraft(e.target.value)}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); resend(m.id) }
                    if (e.key === 'Escape') setEditing(null)
                  }} />
                <div className="msg-edit-actions">
                  <button className="btn" onClick={() => setEditing(null)} aria-label="Cancel edit">Cancel</button>
                  <button className="btn primary" onClick={() => resend(m.id)} disabled={!draft.trim()} aria-label="Save and resend">Save &amp; resend</button>
                </div>
              </div>
            ) : m.role === 'assistant' && !m.streaming
              ? <Markdown text={m.text} />
              : <span>{m.text}{m.streaming && <span className="cursor" role="status" aria-label="Streaming" />}</span>}
          </div>
          {!m.streaming && !isEditing && (
            <div className="msg-actions">
              <button className={`copy-btn ${copied === m.id ? 'copied' : ''}`} onClick={() => copy(m.id, m.text)}
                title="Copy" aria-label="Copy message text">
                <Icon name={copied === m.id ? 'check' : 'copy'} size={14} />
              </button>
              {m.role === 'user' && (
                <button className="copy-btn" disabled={!canAct} title="Edit and resend" aria-label="Edit and resend"
                  onClick={() => { setEditing(m.id); setDraft(m.text) }}><Icon name="edit" size={14} /></button>
              )}
              <button className="copy-btn" disabled={!canAct} onClick={() => onBranch(block.index)}
                title="Branch from here" aria-label="Branch conversation from this message"><Icon name="branch" size={14} /></button>
            </div>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="messages" ref={container} role="log" aria-live="polite" aria-label="Conversation messages"
      onScroll={e => {
        const el = e.currentTarget
        setScrollTop(el.scrollTop)
        nearBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 200
      }}>
      {loading && <div className="conv-loading"><span className="thinking-dots"><span /><span /><span /></span></div>}
      {!loading && items.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon"><Icon name="spark" size={22} /></div>
          <h2>What should we work on?</h2>
          <p>The agent can read and change files in this folder, run commands and search the web. It asks before doing anything sensitive.</p>
          <div className="suggestions">
            {SUGGESTIONS.map(([icon, text]) => (
              <button key={text} className="suggestion" onClick={() => onSuggest(text)}><Icon name={icon} />{text}</button>
            ))}
          </div>
        </div>
      )}
      <div className="messages-inner">
        {top > 0 && <div style={{ height: top }} />}
        {blocks.slice(start, end).map(render)}
        {below > 0 && <div style={{ height: below }} />}
        {thinking && running && (
          <div className="msg-row assistant" role="status" aria-label="Agent is thinking">
            <span className="thinking-dots"><span /><span /><span /></span>
          </div>
        )}
        <div ref={bottom} />
      </div>
    </div>
  )
}
