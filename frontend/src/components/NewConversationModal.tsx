import { useEffect, useState } from 'react'
import { PickDirectory, RecentWorkDirs } from '../../wailsjs/go/main/App'

interface Props {
  onCreate: (workDir: string) => void
  onClose: () => void
}

export function NewConversationModal({ onCreate, onClose }: Props) {
  const [path, setPath] = useState('')
  const [recent, setRecent] = useState<string[]>([])
  useEffect(() => {
    RecentWorkDirs().then(d => setRecent(d || [])).catch(() => {})
  }, [])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    // Clicking the backdrop is a pointer shortcut; Escape does the same.
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions
    <div className="overlay" onClick={e => e.target === e.currentTarget && onClose()}
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
            <span className="field-label">Working Directory</span>
            <div className="dir-row">
              <input value={path} onChange={e => setPath(e.target.value)} placeholder="~ (home directory)" list="recent-dirs" />
              <datalist id="recent-dirs">{recent.map(d => <option key={d} value={d} />)}</datalist>
              <button className="pick-btn" onClick={() => PickDirectory().then(d => d && setPath(d))}>Browse</button>
            </div>
          </div>
        </div>
        <div className="modal-ft">
          <button className="btn secondary" onClick={() => onClose()}>Cancel</button>
          <button className="btn primary" onClick={() => onCreate(path)}>Start</button>
        </div>
      </div>
    </div>
  )
}
