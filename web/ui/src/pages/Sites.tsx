import { useState, useEffect } from 'react'
import ConfirmDialog from '../components/ConfirmDialog'
import { apiGet, apiPost, apiPut, apiDelete, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

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
  wp_root: '/var/www/html',
  backup_dir: './backups',
  retention_flags: '--keep-daily 7 --keep-weekly 4',
})

export default function Sites() {
  const [sites, setSites] = useState<Site[]>([])
  const [editing, setEditing] = useState<{ id?: string; data: Partial<Site> } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const { toast } = useToast()

  useEffect(() => { load() }, [])

  async function load() {
    const res = await apiGet<Site[]>('/api/v1/sites')
    if (res.success && res.data) {
      setSites(Array.isArray(res.data) ? res.data : [])
    }
  }

  async function detectConfig() {
    if (!editing) return
    const { wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_ssh_key, wp_root } = editing.data
    if (!wp_ssh_host || !wp_ssh_user) { toast('SSH Host and User are required', 'error'); return }
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
      toast('Configuration detected', 'success')
    } else {
      toast(res.error || 'Detection failed', 'error')
    }
  }

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!editing) return
    const { id, data } = editing
    const payload = { ...data }
    if (data.restic_repository === 'local:/app/backup_artifacts/restic-repo') {
      // keep as-is
    }
    if (id) {
      const res = await apiPut(`/api/v1/sites/${id}`, payload)
      if (res.success) {
        toast('Site updated', 'success')
        setEditing(null)
        load()
      } else {
        toast(res.error || 'Save failed', 'error')
      }
    } else {
      const res = await apiPost('/api/v1/sites', payload)
      if (res.success) {
        toast('Site created', 'success')
        setEditing(null)
        load()
      } else {
        toast(res.error || 'Save failed', 'error')
      }
    }
  }

  async function deleteSite(id: string) {
    await apiDelete('/api/v1/sites/' + id)
    toast('Site deleted', 'success')
    load()
  }

  function field(key: keyof Site, label: string, opts?: { readonly?: boolean; type?: string; placeholder?: string }) {
    if (!editing) return null
    const val = editing.data[key] ?? ''
    return (
      <div className="field-floating">
        <input type={opts?.type || 'text'} id={'f_' + key} placeholder=" "
          readOnly={opts?.readonly}
          value={typeof val === 'boolean' ? (val ? '1' : '0') : String(val)}
          onChange={e => setEditing(prev => ({
            ...prev!,
            data: { ...prev!.data, [key]: e.target.type === 'number' ? parseInt(e.target.value) || 0 : e.target.value }
          }))}
          required={!opts?.readonly} />
        <label>{label}</label>
      </div>
    )
  }

  return (
    <>
      <header className="page-header">
        <h1>Sites</h1>
        <button className="btn btn-primary" onClick={() => setEditing({ data: emptyForm() })}>Add Site</button>
      </header>

      <div className="card">
        <div className="list-grid">
          {sites.length === 0 && <p>No sites configured yet.</p>}
          {sites.map(s => (
            <article key={s.id} className="entity-card">
              <div className="entity-title">{s.name || '-'}</div>
              <div className="entity-meta">Host: {s.wp_ssh_host || '-'}</div>
              <div className="entity-meta">SSH User: {s.wp_ssh_user || '-'}</div>
              <div className="entity-meta">WP Root: <code>{s.wp_root || '-'}</code></div>
              <div className="entity-meta">
                <span className={`badge ${s.staging_enabled ? 'badge-success' : 'badge-muted'}`}>
                  {s.staging_enabled ? 'Staging enabled' : 'Production only'}
                </span>
              </div>
              <div className="entity-actions">
                <button onClick={() => setEditing({ id: s.id, data: s })} className="btn btn-sm">Edit</button>
                <button onClick={() => setDeleteTarget(s.id)} className="btn btn-danger btn-sm">Delete</button>
              </div>
            </article>
          ))}
        </div>
      </div>

      {editing && (
        <div className="modal show" style={{ display: 'block' }}>
          <button className="modal-backdrop" onClick={() => setEditing(null)} />
          <div className="modal-dialog">
            <div className="modal-header">
              <h3>{editing.id ? 'Edit Site' : 'Create Site'}</h3>
              <button type="button" className="icon-btn" onClick={() => setEditing(null)}>x</button>
            </div>
            <div className="modal-body">
              <form onSubmit={handleSave}>
                {field('name', 'Site Name')}
                {field('wp_ssh_host', 'SSH Host')}
                {field('wp_ssh_port', 'SSH Port', { type: 'number' })}
                {field('wp_ssh_user', 'SSH User')}
                {field('wp_ssh_key', 'SSH Private Key (optional)')}
                {field('wp_root', 'WordPress Root')}
                <button type="button" className="btn btn-sm btn-secondary" onClick={detectConfig} style={{ marginBottom: 16 }}>
                  Detect Configuration via SSH
                </button>
                {field('db_host', 'Database Host', { readonly: true })}
                {field('db_user', 'Database User', { readonly: true })}
                {field('db_password', 'Database Password', { readonly: true, type: 'password' })}
                {field('db_name', 'Database Name', { readonly: true })}
                {field('restic_repository', 'Restic Repository')}
                <label style={{ display: 'flex', alignItems: 'center', gap: 6, margin: '4px 0 12px', fontSize: '0.9em' }}>
                  <input type="checkbox" checked={editing.data.restic_repository === 'local:/app/backup_artifacts/restic-repo'}
                    onChange={e => setEditing(prev => ({
                      ...prev!,
                      data: { ...prev!.data, restic_repository: e.target.checked ? 'local:/app/backup_artifacts/restic-repo' : '' }
                    }))} />
                  <span>Use local storage (/app/backup_artifacts/restic-repo)</span>
                </label>
                {field('backup_dir', 'Backup Directory')}
                {field('retention_flags', 'Retention Flags')}
                {field('healthcheck_url', 'Healthcheck URL')}
                <div className="field-floating">
                  <select value={editing.data.staging_enabled ? '1' : '0'}
                    onChange={e => setEditing(prev => ({ ...prev!, data: { ...prev!.data, staging_enabled: e.target.value === '1' } }))}>
                    <option value="0">No</option>
                    <option value="1">Yes</option>
                  </select>
                  <label>Staging Enabled</label>
                </div>
                <div className="modal-actions">
                  <button type="button" className="btn" onClick={() => setEditing(null)}>Cancel</button>
                  <button type="submit" className="btn btn-primary">{editing.id ? 'Save Changes' : 'Create Site'}</button>
                </div>
              </form>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Site"
        message="Delete this site?"
        onConfirm={() => { if (deleteTarget) deleteSite(deleteTarget); setDeleteTarget(null) }}
        onCancel={() => setDeleteTarget(null)}
      />
    </>
  )
}
