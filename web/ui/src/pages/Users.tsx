import { useState, useEffect, useRef } from 'react'
import { apiGet, apiPost, apiPut } from '../api/client'
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

function passwordStrength(password: string): number {
  let score = 0
  if (password.length >= 8) score += 1
  if (/[A-Z]/.test(password)) score += 1
  if (/[a-z]/.test(password)) score += 1
  if (/[0-9]/.test(password)) score += 1
  if (/[^A-Za-z0-9]/.test(password)) score += 1
  return score
}

export default function Users() {
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [displayName, setDisplayName] = useState('')
  const [icon, setIcon] = useState('user')
  const [saving, setSaving] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [changing, setChanging] = useState(false)
  const currentPasswordRef = useRef<HTMLInputElement>(null)
  const newPasswordRef = useRef<HTMLInputElement>(null)
  const confirmPasswordRef = useRef<HTMLInputElement>(null)
  const { toast } = useToast()
  const { t } = useLanguage()

  const strengthLabels = [
    t('strength_very_weak'), t('strength_weak'), t('strength_fair'),
    t('strength_good'), t('strength_strong'), t('strength_excellent'),
  ]

  useEffect(() => { loadProfile() }, [])

  function mapPasswordChangeError(error: string): string {
    if (error === 'invalid current password') return t('auth_current_password_invalid')
    if (error === 'password must be at least 8 characters with uppercase, lowercase, and digit') {
      return t('auth_password_requirements')
    }
    return error
  }

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

  async function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!currentPassword) {
      toast(t('auth_current_password_required'), 'error')
      currentPasswordRef.current?.focus()
      return
    }
    if (!newPassword) {
      toast(t('auth_password_required'), 'error')
      newPasswordRef.current?.focus()
      return
    }
    if (newPassword.length < 8) {
      toast(t('auth_password_min_length'), 'error')
      newPasswordRef.current?.focus()
      return
    }
    if (!/[A-Z]/.test(newPassword) || !/[a-z]/.test(newPassword) || !/[0-9]/.test(newPassword)) {
      toast(t('auth_password_requirements'), 'error')
      newPasswordRef.current?.focus()
      return
    }
    if (newPassword !== confirmPassword) {
      toast(t('auth_passwords_match'), 'error')
      confirmPasswordRef.current?.focus()
      return
    }
    if (!profile) return
    setChanging(true)
    try {
      const res = await apiPost<{ message?: string }>('/api/v1/auth/change-password', {
        user_id: profile.id,
        current_password: currentPassword,
        new_password: newPassword,
      })
      if (res.success) {
        toast(t('password_changed'), 'success')
        setCurrentPassword('')
        setNewPassword('')
        setConfirmPassword('')
      } else {
        toast(mapPasswordChangeError(res.error || t('password_failed_change')), 'error')
      }
    } catch (e: any) {
      toast(mapPasswordChangeError(e.message || t('password_failed_change')), 'error')
    }
    setChanging(false)
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
      <div className="card">
        <h3>{t('change_password_title')}</h3>
        <form onSubmit={handlePasswordSubmit}>
          <div className="field-floating">
            <input type="password" id="currentPassword" value={currentPassword}
              ref={currentPasswordRef}
              onChange={e => setCurrentPassword(e.target.value)} placeholder=" " required />
            <label htmlFor="currentPassword">{t('form_current_password')}</label>
          </div>
          <div className="field-floating">
            <input type="password" id="newPassword" value={newPassword}
              ref={newPasswordRef}
              onChange={e => setNewPassword(e.target.value)} placeholder=" " required />
            <label htmlFor="newPassword">{t('form_new_password')}</label>
          </div>
          <div className="strength-meter">
            <div className="strength-track">
              <div className="strength-fill" data-strength={passwordStrength(newPassword)} />
            </div>
            <div className="strength-text">
              {newPassword ? strengthLabels[passwordStrength(newPassword)] : t('strength_enter_password')}
            </div>
          </div>
          <div className="field-floating">
            <input type="password" id="confirmPassword" value={confirmPassword}
              ref={confirmPasswordRef}
              onChange={e => setConfirmPassword(e.target.value)} placeholder=" " required />
            <label htmlFor="confirmPassword">{t('form_confirm_password')}</label>
          </div>
          <div className="form-group"><small className="help-text">{t('help_password_requirements')}</small></div>
          <div className="modal-actions">
            <button type="submit" className="btn btn-primary" disabled={changing}>
              {changing && <span className="spinner" />}{t('btn_change_password')}
            </button>
          </div>
        </form>
      </div>
    </>
  )
}
