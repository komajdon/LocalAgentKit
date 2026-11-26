import { useState, useEffect } from 'react'
import {
  ExportBackup, ImportBackup, GetKeyLocation, SetKeyLocation, RotateEncryptionKey, ImportConversation,
} from '../../../wailsjs/go/main/App'
import type { Dispatch, SetStateAction } from 'react'
import type { Cfg } from '../../types'

interface Props {
  cfg: Cfg
  setCfg: Dispatch<SetStateAction<Cfg>>
  onImported: () => void
}

export function DataSettings({ cfg, setCfg, onImported }: Props) {
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
  // import feedback
  const [importMsg, setImportMsg] = useState('')
  const importConversation = async () => {
    setImportMsg('')
    const result = await ImportConversation() as string
    if (result) setImportMsg(result)
    if (result && !result.startsWith('No')) onImported()
  }
  useEffect(() => {
    GetKeyLocation().then(setKeyLocation).catch(() => {})
  }, [])

  return (
    <>
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
        <span className="field-label">Import Conversations</span>
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
    </>
  )
}
