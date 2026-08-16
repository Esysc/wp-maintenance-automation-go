import { useState, useEffect } from 'react'
import ConfirmDialog from '../components/ConfirmDialog'
import Modal from '../components/Modal'
import { apiGet, apiPost, apiPut, apiDelete } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'

interface Site {
  id: string
  name: string
  wp_ssh_host: string
  wp_ssh_port: number
  wp_ssh_user: string
  wp_ssh_key: string
  wp_root: string
  db_host: string
  db_user: string
  db_password: string
  db_name: string
  restic_repository: string
  restic_password_file: string
  backup_dir: string
  retention_flags: string
  healthcheck_url: string
  staging_enabled: boolean
}

const emptyForm = (): Partial<Site> => ({
  wp_ssh_port: 22,
  wp_root: '',
  backup_dir: './backups',
  retention_flags: '--keep-daily 7 --keep-weekly 4',
})

export default function Sites() {
  const [sites, setSites] = useState<Site[]>([])
  const [editing, setEditing] = useState<{ id?: string; data: Partial<Site> } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [saving, setSaving] = useState(false)
  const [detecting, setDetecting] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const { toast } = useToast()
  const { reload: reloadSites } = useSite()
  const { t } = useLanguage()

  useEffect(() => { load() }, [])

  function detectErrorMessage(resp: any): string {
    const fallback = t('config_detection_failed')
    if (!resp || typeof resp !== 'object') return fallback
    if (typeof resp.error === 'string' && resp.error.trim()) return resp.error.trim()
    if (typeof resp.message === 'string' && resp.message.trim()) return resp.message.trim()
    if (resp.data && typeof resp.data === 'object') {
      const data = resp.data as Record<string, unknown>
      if (typeof data.error === 'string' && data.error.trim()) return data.error.trim()
      if (typeof data.message === 'string' && data.message.trim()) return data.message.trim()
    }
    return fallback
  }

  function detectThrownErrorMessage(e: unknown): string {
    const fallback = t('config_detection_failed')
    if (!e) return fallback
    if (typeof e === 'string' && e.trim()) return e.trim()
    if (typeof e === 'object') {
      const err = e as Record<string, unknown>
      if (typeof err.message === 'string' && err.message.trim()) return err.message.trim()
      if (typeof err.error === 'string' && err.error.trim()) return err.error.trim()
    }
    return fallback
  }

  async function load() {
    const res = await apiGet<Site[]>('/api/v1/sites')
    if (res.success && res.data) {
      setSites(Array.isArray(res.data) ? res.data : [])
    } else if (res.error) {
      toast(res.error, 'error')
    }
  }

  async function detectConfig() {
    if (!editing) return
    const { wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_ssh_key, wp_root } = editing.data
    if (!wp_ssh_host || !wp_ssh_user) { toast(t('error_ssh_host_user_required'), 'error'); return }
    setDetecting(true)
    try {
      const res = await apiPost<{ wp_root: string; db_host: string; db_user: string; db_password: string; db_name: string }>(
        '/api/v1/sites/detect-config',
        { ssh_host: wp_ssh_host, ssh_port: wp_ssh_port || 22, ssh_user: wp_ssh_user, ssh_key: wp_ssh_key || '', wp_root },
      )
      if (res.success && res.data) {
        setEditing(prev => ({
          ...prev!,
          data: {
            ...prev!.data,
            ...res.data!,
          }
        }))
        toast(t('config_detected_success'), 'success')
      } else {
        toast(detectErrorMessage(res), 'error')
      }
    } catch (e: unknown) {
      toast(detectThrownErrorMessage(e), 'error')
    }
    setDetecting(false)
  }

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!editing) return
    const { id, data } = editing
    const payload = { ...data }
    setSaving(true)
    try {
      if (id) {
        const res = await apiPut(`/api/v1/sites/${id}`, payload)
        if (res.success) {
          toast(t('site_updated_successfully'), 'success')
          setEditing(null)
          load()
          reloadSites()
        } else {
          toast(res.error || t('site_failed_save'), 'error')
        }
      } else {
        const res = await apiPost('/api/v1/sites', payload)
        if (res.success) {
          toast(t('site_created_successfully'), 'success')
          setEditing(null)
          load()
          reloadSites()
        } else {
          toast(res.error || t('site_failed_save'), 'error')
        }
      }
    } catch (e: any) {
      toast(e.message || t('site_failed_save'), 'error')
    }
    setSaving(false)
  }

  async function deleteSite(id: string) {
    setDeleteBusy(true)
    try {
      const res = await apiDelete('/api/v1/sites/' + id)
      if (res.success) {
        toast(t('site_deleted'), 'success')
      } else {
        toast(res.error || t('site_failed_delete'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('site_failed_delete'), 'error')
    }
    setDeleteBusy(false)
    setDeleteTarget(null)
    load()
    reloadSites()
  }

  function field(key: keyof Site, label: string, opts?: { readonly?: boolean; type?: string; multiline?: boolean; rows?: number }) {
    if (!editing) return null
    const val = editing.data[key] ?? ''
    const id = 'f_' + key
    const inputValue = typeof val === 'boolean' ? (val ? '1' : '0') : String(val)
    const onValueChange = (value: string, type?: string) => {
      setEditing(prev => ({
        ...prev!,
        data: { ...prev!.data, [key]: type === 'number' ? parseInt(value) || 0 : value }
      }))
    }

    if (opts?.multiline) {
      return (
        <div className="field-floating">
          <textarea
            id={id}
            placeholder=" "
            readOnly={opts?.readonly}
            rows={opts?.rows || 6}
            value={inputValue}
            onChange={e => onValueChange(e.target.value)}
            required={!opts?.readonly}
          />
          <label htmlFor={id}>{label}</label>
        </div>
      )
    }

    return (
      <div className="field-floating">
        <input id={id} type={opts?.type || 'text'} placeholder=" "
          readOnly={opts?.readonly}
          value={inputValue}
          onChange={e => onValueChange(e.target.value, e.target.type)}
          required={!opts?.readonly} />
        <label htmlFor={id}>{label}</label>
      </div>
    )
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_sites')}</h1>
        <button className="btn btn-primary" onClick={() => setEditing({ data: emptyForm() })}>{t('btn_add_site')}</button>
      </header>

      <div className="card">
        <div className="list-grid">
          {sites.length === 0 && <p>{t('no_sites_configured')}</p>}
          {sites.map(s => (
            <article key={s.id} className="entity-card">
              <div className="entity-title">{s.name || '-'}</div>
              <div className="entity-meta">{t('site_label_host')}: {s.wp_ssh_host || '-'}</div>
              <div className="entity-meta">{t('site_label_ssh_user')}: {s.wp_ssh_user || '-'}</div>
              <div className="entity-meta">{t('site_label_wp_root')}: <code>{s.wp_root || '-'}</code></div>
              <div className="entity-meta">
                <span className={`badge ${s.staging_enabled ? 'badge-success' : 'badge-muted'}`}>
                  {s.staging_enabled ? t('site_staging_enabled_badge') : t('site_production_only_badge')}
                </span>
              </div>
              <div className="entity-actions">
                <button onClick={() => setEditing({ id: s.id, data: s })} className="btn btn-sm">{t('btn_edit')}</button>
                <button onClick={() => setDeleteTarget({ id: s.id, name: s.name || s.id })} className="btn btn-danger btn-sm">{t('btn_delete')}</button>
              </div>
            </article>
          ))}
        </div>
      </div>

      {editing && (
        <Modal
          open
          title={editing.id ? t('modal_edit_site') : t('modal_create_site')}
          onClose={() => setEditing(null)}
        >
          <form onSubmit={handleSave}>
            <div className="modal-body">
              {field('name', t('form_site_name'))}
              {field('wp_ssh_host', t('form_ssh_host'))}
              {field('wp_ssh_port', t('form_ssh_port'), { type: 'number' })}
              {field('wp_ssh_user', t('form_ssh_user'))}
              {field('wp_ssh_key', t('form_ssh_key'), { multiline: true, rows: 8 })}
              {field('wp_root', t('form_wp_root'))}
              <div className="form-group">
                <button type="button" className="btn btn-sm btn-secondary" onClick={detectConfig} disabled={detecting}>
                  {detecting && <span className="spinner" />}
                  {t('btn_detect_config')}
                </button>
              </div>
              {field('db_host', t('form_db_host'), { readonly: true })}
              {field('db_user', t('form_db_user'), { readonly: true })}
              {field('db_password', t('form_db_password'), { readonly: true, type: 'password' })}
              {field('db_name', t('form_db_name'), { readonly: true })}
              {field('restic_repository', t('form_restic_repo'))}
              <label style={{ display: 'flex', alignItems: 'center', gap: 6, margin: '4px 0 12px', fontSize: '0.9em' }}>
                <input type="checkbox" checked={editing.data.restic_repository === 'local:/app/backup_artifacts/restic-repo'}
                  onChange={e => setEditing(prev => ({
                    ...prev!,
                    data: { ...prev!.data, restic_repository: e.target.checked ? 'local:/app/backup_artifacts/restic-repo' : '' }
                  }))} />
                <span>{t('form_local_restic')}</span>
              </label>
              {field('backup_dir', t('form_backup_dir'))}
              {field('retention_flags', t('form_retention_flags'))}
              {field('healthcheck_url', t('form_healthcheck_url'))}
              <div className="field-floating">
                <select id="f_staging_enabled" value={editing.data.staging_enabled ? '1' : '0'}
                  onChange={e => setEditing(prev => ({ ...prev!, data: { ...prev!.data, staging_enabled: e.target.value === '1' } }))}>
                  <option value="0">{t('option_no')}</option>
                  <option value="1">{t('option_yes')}</option>
                </select>
                <label htmlFor="f_staging_enabled">{t('form_staging_enabled')}</label>
              </div>
            </div>
            <div className="modal-actions">
              <button type="button" className="btn" onClick={() => setEditing(null)} disabled={saving}>{t('btn_cancel')}</button>
              <button type="submit" className="btn btn-primary" disabled={saving}>
                {saving && <span className="spinner" />}{editing.id ? t('btn_save_changes') : t('btn_create_site')}
              </button>
            </div>
          </form>
        </Modal>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('modal_delete_site')}
        message={deleteTarget ? t('modal_delete_confirmation') + ' (' + deleteTarget.name + ')' : ''}
        onConfirm={() => { if (deleteTarget) deleteSite(deleteTarget.id) }}
        onCancel={() => setDeleteTarget(null)}
        busy={deleteBusy}
      />
    </>
  )
}
