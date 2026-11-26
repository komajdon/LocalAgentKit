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
  return (
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
            <label>Working Directory</label>
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
