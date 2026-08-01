import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useLanguage, LANGUAGE_OPTIONS } from '../context/LanguageContext'
import LoadingState from '../components/LoadingState'

function passwordStrength(password: string): number {
  let score = 0
  if (password.length >= 8) score += 1
  if (/[A-Z]/.test(password)) score += 1
  if (/[a-z]/.test(password)) score += 1
  if (/[0-9]/.test(password)) score += 1
  if (/[^A-Za-z0-9]/.test(password)) score += 1
  return score
}

export default function Login() {
  const { authenticated, setupRequired, loading, login } = useAuth()
  const { lang, setLang, t } = useLanguage()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!loading && authenticated) navigate('/dashboard', { replace: true })
  }, [loading, authenticated, navigate])

  useEffect(() => {
    document.title = t('page_title_login') + ' — WP Maintenance Automation'
  }, [lang, t])

  const strengthLabels = [
    t('strength_very_weak'), t('strength_weak'), t('strength_fair'),
    t('strength_good'), t('strength_strong'), t('strength_excellent'),
  ]

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')

    if (!password) { setError(t('auth_password_required')); return }

    if (setupRequired) {
      if (password !== confirm) { setError(t('auth_passwords_match')); return }
      if (password.length < 8) { setError(t('auth_password_min_length')); return }
      if (!/[A-Z]/.test(password) || !/[a-z]/.test(password) || !/[0-9]/.test(password)) {
        setError(t('auth_password_requirements'))
        return
      }
    }

    setSubmitting(true)
    const err = await login(password, setupRequired ? confirm : undefined)
    setSubmitting(false)

    if (err) {
      setError(err)
    } else {
      navigate('/dashboard', { replace: true })
    }
  }

  if (loading) return <div className="login-container"><LoadingState text={t('loading')} /></div>

  const score = passwordStrength(password)

  return (
    <div className="login-container">
      <div className="login-card">
        <div className="login-head">
          <h1>WP Maintenance Automation</h1>
          <select className="lang-select" value={lang} onChange={e => setLang(e.target.value as typeof lang)} aria-label={t('form_language')}>
            {LANGUAGE_OPTIONS.map(opt => (
              <option key={opt} value={opt}>{opt.toUpperCase()}</option>
            ))}
          </select>
        </div>
        <h2>{setupRequired ? t('login_title_setup') : t('login_title_normal')}</h2>
        <form onSubmit={handleSubmit}>
          <div className="field-floating">
            <input type="password" id="password" value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder=" " required autoFocus />
            <label htmlFor="password">{setupRequired ? t('form_new_password') : t('form_password')}</label>
            {setupRequired && <small className="help-text">{t('help_password_requirements')}</small>}
          </div>
          {setupRequired && (
            <>
              <div className="strength-meter">
                <div className="strength-track">
                  <div className="strength-fill" data-strength={score} />
                </div>
                <div className="strength-text">{password ? strengthLabels[score] : t('strength_enter_password')}</div>
              </div>
              <div className="field-floating">
                <input type="password" id="passwordConfirm" value={confirm}
                  onChange={e => setConfirm(e.target.value)}
                  placeholder=" " required />
                <label htmlFor="passwordConfirm">{t('form_confirm_password')}</label>
              </div>
            </>
          )}
          <div className="form-group">
            <button type="submit" className="btn btn-primary" disabled={submitting} style={{ width: '100%' }}>
              {submitting && <span className="spinner" />}
              {submitting ? t('btn_processing') : setupRequired ? t('btn_set_password_login') : t('btn_login')}
            </button>
          </div>
        </form>
        {error && <div className="error-msg" style={{ display: 'block' }} role="alert">{error}</div>}
      </div>
    </div>
  )
}
