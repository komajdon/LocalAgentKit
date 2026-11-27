import { activate } from '../../activate'
import { shortPath } from '../../format'
import { Icon } from '../Icon'
import { ProfilePicker } from '../ProfilePicker'

export type Panel = 'activity' | 'changes' | null

interface Props {
  conversationId: string
  title: string
  path: string
  model: string
  defaultModel: string
  models: string[]
  panel: Panel
  /** Changes when profiles may have been edited. */
  profilesVersion: unknown
  onModel: (model: string) => void
  onPath: () => void
  onPanel: (panel: Panel) => void
  onExport: (format: 'markdown' | 'text' | 'json') => void
}

export function ChatHeader(p: Props) {
  const free = p.models.filter(m => m.endsWith(':free'))
  const paid = p.models.filter(m => !m.endsWith(':free'))
  const options = (list: string[]) => list.map(m => <option key={m} value={m}>{m}</option>)
  const toggle = (panel: Panel) => p.onPanel(p.panel === panel ? null : panel)
  return (
    <header className="topbar">
      <div className="topbar-heading">
        <div className="topbar-title" title={p.title}>{p.title || 'Conversation'}</div>
        <div className="path-chip" {...activate(p.onPath)} title={`${p.path}\nClick to change the working folder`}>
          <Icon name="folder" size={12} />{shortPath(p.path)}
        </div>
      </div>
      <ProfilePicker conversationId={p.conversationId} reload={p.profilesVersion} />
      <select value={p.model} onChange={e => p.onModel(e.target.value)} aria-label="Model for this conversation" title="Model for this conversation">
        <option value="">{p.defaultModel ? `Default · ${p.defaultModel}` : 'Default model'}</option>
        {free.length && paid.length
          ? <><optgroup label="Free">{options(free)}</optgroup><optgroup label="Paid">{options(paid)}</optgroup></>
          : options(p.models)}
      </select>
      <span className="topbar-sep" />
      <button className="icon-btn" aria-pressed={p.panel === 'activity'} onClick={() => toggle('activity')} title="Tool timeline">
        <Icon name="activity" /><span className="btn-label">Activity</span>
      </button>
      <button className="icon-btn" aria-pressed={p.panel === 'changes'} onClick={() => toggle('changes')} title="File changes">
        <Icon name="diff" /><span className="btn-label">Changes</span>
      </button>
      <div className="export-menu">
        <button className="icon-btn" title="Export conversation" aria-label="Export conversation" aria-haspopup="true"><Icon name="download" /></button>
        <div className="export-dropdown">
          <button onClick={() => p.onExport('markdown')}>Export as Markdown</button>
          <button onClick={() => p.onExport('text')}>Export as plain text</button>
          <button onClick={() => p.onExport('json')}>Export as JSON</button>
        </div>
      </div>
    </header>
  )
}
