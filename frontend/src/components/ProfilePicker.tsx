import { useEffect, useState } from 'react'
import { ListProfiles, LoadConversation, SetConversationProfile } from '../../wailsjs/go/main/App'
import type { Profile } from './WorkspaceManager'

/** Chooses the profile of one conversation. `reload` changes when profiles may have. */
export function ProfilePicker({ conversationId, reload }: { conversationId: string; reload: unknown }) {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [selected, setSelected] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    let current = true
    setError('')
    ListProfiles().then(p => { if (current) setProfiles(p as any) }).catch(() => {})
    LoadConversation(conversationId)
      .then((c: any) => { if (current) setSelected(c.profile_id ?? '') })
      .catch(() => {})
    return () => { current = false }
  }, [conversationId, reload])

  const choose = (id: string) => {
    setSelected(id); setError('')
    SetConversationProfile(conversationId, id).catch(e => setError(String(e)))
  }

  return (
    <select className="perm-sel" aria-label="Agent profile" title={error || 'Agent profile, from the next message'}
      value={selected} onChange={e => choose(e.target.value)}>
      <option value="">Workspace default</option>
      {profiles.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
    </select>
  )
}
