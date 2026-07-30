import { useState, useEffect } from 'react'
import ConfirmDialog from '../components/ConfirmDialog'
import { apiGet, apiPost, apiDelete, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

interface Token {
  id: string
  name: string
  owner_display_name: string
  owner_name: string
  token?: string
  created_at: string
  expires_at: string
  revoked: boolean
  current_session: boolean
}

export default function Tokens() {
  const [tokens, setTokens] = useState<Token[]>([])
  const [showCreate, setShowCreate] = useState(false)
  const [tokenName, setTokenName] = useState('')
  const [tokenDuration, setTokenDuration] = useState(720)
  const [createdToken, setCreatedToken] = useState('')
  const [revokeTarget, setRevokeTarget] = useState<string | null>(null)
  const [currentSessionId, setCurrentSessionId] = useState('')
  const { toast } = useToast()

  useEffect(() => { load() }, [])

  async function load() {
    const res = await apiGet<Token[]>('/api/v1/tokens')
    if (res.success && res.data) {
      const list = Array.isArray(res.data) ? res.data : []
      setTokens(list)
      const cur = list.find(t => t.current_session)
      if (cur) setCurrentSessionId(cur.id || '')
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    const res = await apiPost<{ token: string }>('/api/v1/tokens', {
      name: tokenName,
      duration: tokenDuration,
    })
    if (res.success && res.data) {
      setCreatedToken(res.data.token || '')
      setShowCreate(false)
      load()
    } else {
      toast(res.error || 'Failed to create token', 'error')
    }
  }

  async function revokeToken(id: string) {
    const res = await apiDelete('/api/v1/tokens/' + id)
    if (res.success) {
      toast('Token revoked', 'success')
      if (currentSessionId && id === currentSessionId) {
        document.cookie = 'token=; Max-Age=0; path=/'
        window.location.href = '/login'
        return
      }
      load()
    } else {
      toast(res.error || 'Failed to revoke', 'error')
    }
  }

  async function copyId(id: string) {
    try {
      await navigator.clipboard.writeText(id)
      toast('Token ID copied', 'success')
    } catch { toast('Unable to copy', 'error') }
  }

  async function copyCreatedTokenValue() {
    try {
      await navigator.clipboard.writeText(createdToken)
      toast('Token copied to clipboard', 'success')
    } catch { toast('Unable to copy', 'error') }
  }

  return (
    <>
      <header className="page-header">
        <h1>API Tokens</h1>
        <button className="btn btn-primary" onClick={() => setShowCreate(true)}>Create Token</button>
      </header>

      <div className="card">
        <div className="list-grid">
          {tokens.length === 0 && (
            <article className="entity-card">
              <div className="entity-title">No active tokens</div>
              <div className="entity-meta">Create a token to enable external API access.</div>
            </article>
          )}
          {tokens.filter(t => !t.revoked).map(t => {
            const expires = t.expires_at ? new Date(t.expires_at).toLocaleString() : 'No expiry'
            return (
              <article key={t.id} className="entity-card">
                <div className="entity-title">{t.name || 'Unnamed Token'}</div>
                <div className="entity-meta">Owner: {t.owner_display_name || t.owner_name || 'internal'}</div>
                <div className="entity-meta">Created: {t.created_at ? new Date(t.created_at).toLocaleString() : '-'}</div>
                <div className="entity-meta">Expires: {expires} {t.expires_at ? <span className="badge badge-muted">Expiring</span> : <span className="badge badge-success">No expiry</span>}</div>
                <div className="entity-actions">
                  <button onClick={() => copyId(t.id)} className="btn btn-sm">Copy ID</button>
                  {!t.current_session && (
                    <button onClick={() => setRevokeTarget(t.id)} className="btn btn-danger btn-sm">Revoke</button>
                  )}
                </div>
              </article>
            )
          })}
        </div>
      </div>

      {showCreate && (
        <div className="modal show" style={{ display: 'block' }}>
          <button className="modal-backdrop" onClick={() => setShowCreate(false)} />
          <div className="modal-dialog modal-sm">
            <div className="modal-header">
              <h3>Create Token</h3>
              <button type="button" className="icon-btn" onClick={() => setShowCreate(false)}>x</button>
            </div>
            <div className="modal-body">
              <form onSubmit={handleCreate}>
                <div className="field-floating">
                  <input type="text" value={tokenName} onChange={e => setTokenName(e.target.value)} placeholder=" " required />
                  <label>Token Name</label>
                </div>
                <div className="field-floating">
                  <input type="number" value={tokenDuration} onChange={e => setTokenDuration(parseInt(e.target.value) || 0)} placeholder=" " />
                  <label>Duration (hours, 0 = no expiry)</label>
                </div>
                <div className="modal-actions">
                  <button type="button" className="btn" onClick={() => setShowCreate(false)}>Cancel</button>
                  <button type="submit" className="btn btn-primary">Create Token</button>
                </div>
              </form>
            </div>
          </div>
        </div>
      )}

      {createdToken && (
        <div className="modal show" style={{ display: 'block' }}>
          <button className="modal-backdrop" onClick={() => setCreatedToken('')} />
          <div className="modal-dialog modal-sm">
            <div className="modal-header">
              <h3>Token Created</h3>
              <button type="button" className="icon-btn" onClick={() => setCreatedToken('')}>x</button>
            </div>
            <div className="modal-body">
              <p>Copy the token now. It will not be shown again.</p>
              <div className="field-floating">
                <input type="text" value={createdToken} readOnly placeholder=" " />
                <label>Token Value</label>
              </div>
              <div className="modal-actions">
                <button type="button" className="btn" onClick={copyCreatedTokenValue}>Copy Token</button>
                <button type="button" className="btn btn-primary" onClick={() => setCreatedToken('')}>Close</button>
              </div>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={revokeTarget !== null}
        title="Revoke Token"
        message="Revoke this token?"
        onConfirm={() => { if (revokeTarget) revokeToken(revokeTarget); setRevokeTarget(null) }}
        onCancel={() => setRevokeTarget(null)}
      />
    </>
  )
}
