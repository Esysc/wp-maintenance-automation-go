import { useState, useEffect } from 'react'
import { apiGet, apiPut, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

interface UserProfile {
  id: string
  username: string
  display_name: string
  icon: string
  created_at: string
  updated_at: string
}

const ICONS: Record<string, string> = {
  user: '<path d="M12 12a4 4 0 100-8 4 4 0 000 8zm0 2c-4.4 0-8 2-8 4.5V21h16v-2.5C20 16 16.4 14 12 14z"></path>',
  shield: '<path d="M12 2l7 3v6c0 5-3.4 9.7-7 11-3.6-1.3-7-6-7-11V5l7-3z"></path>',
  bolt: '<path d="M13 2L5 14h5l-1 8 8-12h-5l1-8z"></path>',
  globe: '<path d="M12 2a10 10 0 100 20 10 10 0 000-20zm6.9 9h-3.1a15.7 15.7 0 00-1.1-5A8.1 8.1 0 0118.9 11zM12 4.1c1 1.2 1.9 3.5 2.2 6.9H9.8C10.1 7.6 11 5.3 12 4.1zM4.1 13h3.1a15.7 15.7 0 001.1 5A8.1 8.1 0 014.1 13zm3.1-2H4.1a8.1 8.1 0 014.2-5 15.7 15.7 0 00-1.1 5zm4.8 8.9c-1-1.2-1.9-3.5-2.2-6.9h4.4c-.3 3.4-1.2 5.7-2.2 6.9zM9.8 11c.3-3.4 1.2-5.7 2.2-6.9 1 1.2 1.9 3.5 2.2 6.9H9.8zm4.9 7a15.7 15.7 0 001.1-5h3.1a8.1 8.1 0 01-4.2 5z"></path>',
  gear: '<path d="M19.4 13a7.7 7.7 0 000-2l2.1-1.6-2-3.5-2.5 1a7.2 7.2 0 00-1.7-1l-.4-2.7H9.1L8.7 6a7.2 7.2 0 00-1.7 1l-2.5-1-2 3.5L4.6 11a7.7 7.7 0 000 2l-2.1 1.6 2 3.5 2.5-1a7.2 7.2 0 001.7 1l.4 2.7h5.8l.4-2.7a7.2 7.2 0 001.7-1l2.5 1 2-3.5-2.1-1.6zM12 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z"></path>',
}

export default function Users() {
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [displayName, setDisplayName] = useState('')
  const [icon, setIcon] = useState('user')
  const { toast } = useToast()

  useEffect(() => { loadProfile() }, [])

  async function loadProfile() {
    const res = await apiGet<UserProfile>('/api/v1/users')
    if (res.success && res.data) {
      setProfile(res.data)
      setDisplayName(res.data.display_name || '')
      setIcon(res.data.icon || 'user')
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const res = await apiPut<UserProfile>('/api/v1/users', { display_name: displayName, icon })
    if (res.success) {
      toast('Profile updated', 'success')
      if (res.data) {
        setProfile(res.data)
        setDisplayName(res.data.display_name || '')
        setIcon(res.data.icon || 'user')
      }
    } else {
      toast(res.error || 'Failed to update', 'error')
    }
  }

  return (
    <>
      <header className="page-header"><h1>Profile</h1></header>
      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="field-floating">
            <input type="text" id="profileUsername" value={profile?.username || 'internal'} placeholder=" " readOnly />
            <label>Internal Username</label>
          </div>
          <div className="field-floating">
            <input type="text" value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder=" " required />
            <label>Display Name</label>
          </div>
          <div className="field-floating">
            <select value={icon} onChange={e => setIcon(e.target.value)}>
              {Object.keys(ICONS).map(k => (
                <option key={k} value={k}>{k.charAt(0).toUpperCase() + k.slice(1)}</option>
              ))}
            </select>
            <label>Profile Icon</label>
          </div>
          <div className="entity-card" style={{ marginBottom: 16 }}>
            <div className="entity-title" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <span className="nav-icon"><svg viewBox="0 0 24 24" aria-hidden="true" dangerouslySetInnerHTML={{ __html: ICONS[icon] || ICONS.user }} /></span>
              <span>{displayName || profile?.username || 'User'}</span>
            </div>
            <div className="entity-meta">Visible name used across the application UI.</div>
          </div>
          {profile && (
            <>
              <div className="entity-meta">Created: {profile.created_at ? new Date(profile.created_at).toLocaleString() : '-'}</div>
              <div className="entity-meta">Updated: {profile.updated_at ? new Date(profile.updated_at).toLocaleString() : '-'}</div>
            </>
          )}
          <div className="entity-meta">The internal account powers the web session and all API tokens. It cannot be deleted.</div>
          <div className="modal-actions">
            <button type="submit" className="btn btn-primary">Save Profile</button>
          </div>
        </form>
      </div>
    </>
  )
}
