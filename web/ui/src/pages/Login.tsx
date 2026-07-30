import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useToast } from '../context/ToastContext'

function passwordStrength(password: string): number {
  let score = 0
  if (password.length >= 8) score += 1
  if (/[A-Z]/.test(password)) score += 1
  if (/[a-z]/.test(password)) score += 1
  if (/[0-9]/.test(password)) score += 1
  if (/[^A-Za-z0-9]/.test(password)) score += 1
  return score
}

const strengthLabels = ['Very weak', 'Weak', 'Fair', 'Good', 'Strong', 'Excellent']

export default function Login() {
  const { setupRequired, loading, login } = useAuth()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!loading && !setupRequired) navigate('/dashboard', { replace: true })
  }, [loading, setupRequired, navigate])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')

    if (!password) { setError('Password is required'); return }

    if (setupRequired) {
      if (password !== confirm) { setError('Passwords do not match'); return }
      if (password.length < 8) { setError('Minimum 8 characters'); return }
      if (!/[A-Z]/.test(password) || !/[a-z]/.test(password) || !/[0-9]/.test(password)) {
        setError('Must include uppercase, lowercase, and digit')
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

  if (loading) return <div className="login-container"><p>Loading...</p></div>

  const score = passwordStrength(password)

  return (
    <div className="login-container">
      <div className="login-card">
        <h1>WP Maintenance Automation</h1>
        <h2>{setupRequired ? 'Create your admin password' : 'Enter your password to continue'}</h2>
        <form onSubmit={handleSubmit}>
          <div className="field-floating">
            <input type="password" id="password" value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder=" " required autoFocus />
            <label htmlFor="password">{setupRequired ? 'New Password' : 'Password'}</label>
            {setupRequired && <small className="help-text">Minimum 8 characters, must include uppercase, lowercase, and digit</small>}
          </div>
          {setupRequired && (
            <>
              <div className="strength-meter">
                <div className="strength-track">
                  <div className="strength-fill" data-strength={score} />
                </div>
                <div className="strength-text">{strengthLabels[score] || 'Enter a password'}</div>
              </div>
              <div className="field-floating">
                <input type="password" id="passwordConfirm" value={confirm}
                  onChange={e => setConfirm(e.target.value)}
                  placeholder=" " required />
                <label htmlFor="passwordConfirm">Confirm Password</label>
              </div>
            </>
          )}
          <div className="form-group">
            <button type="submit" className="btn btn-primary" disabled={submitting} style={{ width: '100%' }}>
              {submitting ? 'Processing...' : setupRequired ? 'Set Password & Login' : 'Login'}
            </button>
          </div>
        </form>
        {error && <div className="error-msg" style={{ display: 'block' }}>{error}</div>}
      </div>
    </div>
  )
}
