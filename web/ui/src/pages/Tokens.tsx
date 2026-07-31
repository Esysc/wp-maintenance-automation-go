import { useState, useEffect } from 'react'
import ConfirmDialog from '../components/ConfirmDialog'
import Modal from '../components/Modal'
import { apiGet, apiPost, apiDelete } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useLanguage } from '../context/LanguageContext'

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
  const [creating, setCreating] = useState(false)
  const [revoking, setRevoking] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => { load() }, [])

  async function load() {
    const res = await apiGet<Token[]>('/api/v1/tokens')
    if (res.success && res.data) {
      const list = Array.isArray(res.data) ? res.data : []
      setTokens(list)
      const cur = list.find(t => t.current_session)
      if (cur) setCurrentSessionId(cur.id || '')
    } else if (res.error) {
      toast(res.error, 'error')
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    setCreating(true)
    try {
      const res = await apiPost<{ token: string }>('/api/v1/tokens', {
        name: tokenName,
        duration: tokenDuration,
      })
      if (res.success && res.data) {
        setCreatedToken(res.data.token || '')
        setShowCreate(false)
        load()
      } else {
        toast(res.error || t('error_create_token_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('error_create_token_failed'), 'error')
    }
    setCreating(false)
  }

  async function revokeToken(id: string) {
    setRevoking(true)
    try {
      const res = await apiDelete('/api/v1/tokens/' + id)
      if (res.success) {
        toast(t('token_revoked'), 'success')
        if (currentSessionId && id === currentSessionId) {
          document.cookie = 'token=; Max-Age=0; path=/'
          window.dispatchEvent(new Event('auth:unauthorized'))
          return
        }
        load()
      } else {
        toast(res.error || t('token_failed_revoke'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('token_failed_revoke'), 'error')
    }
    setRevoking(false)
    setRevokeTarget(null)
  }

  async function copyId(id: string) {
    try {
      await navigator.clipboard.writeText(id)
      toast(t('token_id_copied'), 'success')
    } catch { toast(t('error_copy'), 'error') }
  }

  async function copyCreatedTokenValue() {
    try {
      await navigator.clipboard.writeText(createdToken)
      toast(t('token_copied'), 'success')
    } catch { toast(t('token_copy_failed'), 'error') }
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_tokens')}</h1>
        <button className="btn btn-primary" onClick={() => setShowCreate(true)}>{t('btn_create_token')}</button>
      </header>

      <div className="card">
        <div className="list-grid">
          {tokens.filter(tk => !tk.revoked).length === 0 && (
            <article className="entity-card">
              <div className="entity-title">{t('token_no_active')}</div>
              <div className="entity-meta">{t('token_create_external_access')}</div>
            </article>
          )}
          {tokens.filter(tk => !tk.revoked).map(tk => {
            const expires = tk.expires_at ? new Date(tk.expires_at).toLocaleString() : t('token_no_expiry')
            return (
              <article key={tk.id} className="entity-card">
                <div className="entity-title">{tk.name || t('token_unnamed')}</div>
                <div className="entity-meta">{t('token_owner')}: {tk.owner_display_name || tk.owner_name || 'internal'}</div>
                <div className="entity-meta">{t('label_created')}: {tk.created_at ? new Date(tk.created_at).toLocaleString() : '-'}</div>
                <div className="entity-meta">
                  {t('token_expires')}: {expires}
                  {tk.expires_at ? <span className="badge badge-muted">{t('token_expiring')}</span> : <span className="badge badge-success">{t('token_no_expiry')}</span>}
                </div>
                <div className="entity-actions">
                  <button onClick={() => copyId(tk.id)} className="btn btn-sm">{t('btn_copy_id')}</button>
                  {!tk.current_session && (
                    <button onClick={() => setRevokeTarget(tk.id)} className="btn btn-danger btn-sm">{t('btn_revoke')}</button>
                  )}
                </div>
              </article>
            )
          })}
        </div>
      </div>

      {showCreate && (
        <Modal
          open
          title={t('modal_create_token')}
          onClose={() => setShowCreate(false)}
          size="sm"
        >
          <form onSubmit={handleCreate}>
            <div className="modal-body">
              <div className="field-floating">
                <input type="text" id="tokenName" value={tokenName} onChange={e => setTokenName(e.target.value)} placeholder=" " required />
                <label htmlFor="tokenName">{t('form_token_name')}</label>
              </div>
              <div className="field-floating">
                <input type="number" id="tokenDuration" value={tokenDuration} onChange={e => setTokenDuration(parseInt(e.target.value) || 0)} placeholder=" " />
                <label htmlFor="tokenDuration">{t('form_token_duration')}</label>
              </div>
            </div>
            <div className="modal-actions">
              <button type="button" className="btn" onClick={() => setShowCreate(false)} disabled={creating}>{t('btn_cancel')}</button>
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating && <span className="spinner" />}{t('btn_create_token')}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {createdToken && (
        <Modal
          open
          title={t('token_created_title')}
          onClose={() => setCreatedToken('')}
          size="sm"
        >
          <div className="modal-body">
            <p>{t('token_created')}</p>
            <div className="field-floating">
              <input type="text" id="createdTokenValue" value={createdToken} readOnly placeholder=" " />
              <label htmlFor="createdTokenValue">{t('token_value')}</label>
            </div>
          </div>
          <div className="modal-actions">
            <button type="button" className="btn" onClick={copyCreatedTokenValue}>{t('btn_copy_token')}</button>
            <button type="button" className="btn btn-primary" onClick={() => setCreatedToken('')}>{t('btn_close')}</button>
          </div>
        </Modal>
      )}

      <ConfirmDialog
        open={revokeTarget !== null}
        title={t('modal_revoke_token')}
        message={t('modal_revoke_confirmation')}
        onConfirm={() => { if (revokeTarget) revokeToken(revokeTarget) }}
        onCancel={() => setRevokeTarget(null)}
        busy={revoking}
      />
    </>
  )
}
