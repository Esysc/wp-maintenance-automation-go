import { useState, useEffect } from 'react'
import { apiGet, apiPut } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useLanguage } from '../context/LanguageContext'
import { ICONS, PROFILE_ICONS } from '../components/icons'

interface UserProfile {
  id: string
  username: string
  display_name: string
  icon: string
  created_at: string
  updated_at: string
}

export default function Users() {
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [displayName, setDisplayName] = useState('')
  const [icon, setIcon] = useState('user')
  const [saving, setSaving] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => { loadProfile() }, [])

  async function loadProfile() {
    const res = await apiGet<UserProfile>('/api/v1/users')
    if (res.success && res.data) {
      setProfile(res.data)
      setDisplayName(res.data.display_name || '')
      setIcon(res.data.icon || 'user')
    } else if (res.error) {
      toast(res.error, 'error')
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      const res = await apiPut<UserProfile>('/api/v1/users', { display_name: displayName, icon })
      if (res.success) {
        toast(t('profile_updated'), 'success')
        if (res.data) {
          setProfile(res.data)
          setDisplayName(res.data.display_name || '')
          setIcon(res.data.icon || 'user')
        }
      } else {
        toast(res.error || t('profile_failed_update'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('profile_failed_update'), 'error')
    }
    setSaving(false)
  }

  return (
    <>
      <header className="page-header"><h1>{t('page_title_users')}</h1></header>
      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="field-floating">
            <input type="text" id="profileUsername" value={profile?.username || 'internal'} placeholder=" " readOnly />
            <label htmlFor="profileUsername">{t('profile_internal_username')}</label>
          </div>
          <div className="field-floating">
            <input type="text" id="profileDisplayName" value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder=" " required />
            <label htmlFor="profileDisplayName">{t('form_profile_display_name')}</label>
          </div>
          <div className="field-floating">
            <select id="profileIcon" value={icon} onChange={e => setIcon(e.target.value)}>
              {PROFILE_ICONS.map(k => (
                <option key={k} value={k}>{t('profile_icon_' + k)}</option>
              ))}
            </select>
            <label htmlFor="profileIcon">{t('form_profile_icon')}</label>
          </div>
          <div className="entity-card" style={{ marginBottom: 16 }}>
            <div className="entity-title" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <span className="nav-icon"><svg viewBox="0 0 24 24" aria-hidden="true" dangerouslySetInnerHTML={{ __html: ICONS[icon] || ICONS.user }} /></span>
              <span>{displayName || profile?.username || t('nav_users')}</span>
            </div>
            <div className="entity-meta">{t('profile_visible_name_help')}</div>
          </div>
          {profile && (
            <>
              <div className="entity-meta">{t('label_created')}: {profile.created_at ? new Date(profile.created_at).toLocaleString() : '-'}</div>
              <div className="entity-meta">{t('label_updated')}: {profile.updated_at ? new Date(profile.updated_at).toLocaleString() : '-'}</div>
            </>
          )}
          <div className="entity-meta">{t('profile_internal_account_help')}</div>
          <div className="modal-actions">
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving && <span className="spinner" />}{t('btn_save_profile')}
            </button>
          </div>
        </form>
      </div>
    </>
  )
}
