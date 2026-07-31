import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import JobHistory from '../components/JobHistory'
import ConfirmDialog from '../components/ConfirmDialog'
import LoadingState from '../components/LoadingState'
import SiteHint from '../components/SiteHint'
import { apiGet, apiPost, apiDelete } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'

interface Backup {
  id: string
  timestamp: string
  host: string
  ssh_host: string
  wp_version: string
  db_name: string
  file_count: number
}

export default function Backups() {
  const { siteId, setSiteId } = useSite()
  const [backups, setBackups] = useState<Backup[]>([])
  const [loading, setLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; label: string } | null>(null)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => {
    if (siteId) loadBackups()
    else setBackups([])
  }, [siteId])

  async function loadBackups() {
    setLoading(true)
    const res = await apiGet<Backup[]>('/api/v1/backups?site_id=' + siteId)
    if (res.success && res.data) {
      setBackups(Array.isArray(res.data) ? res.data : [])
    } else if (res.error) {
      toast(res.error, 'error')
    }
    setLoading(false)
  }

  async function runBackup() {
    if (!siteId) { toast(t('error_site_id_required'), 'error'); return }
    setRunning(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/backup', { site_id: siteId })
      if (res.success) {
        toast(t('backup_queued'), 'info')
      } else {
        toast(res.error || t('backup_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('backup_failed'), 'error')
    }
    setRunning(false)
  }

  async function deleteBackup(id: string) {
    setDeleteBusy(true)
    try {
      const res = await apiDelete('/api/v1/backups/' + id + '?site_id=' + siteId)
      if (res.success) {
        toast(t('backup_deleted'), 'success')
      } else {
        toast(res.error || t('backup_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('backup_failed'), 'error')
    }
    setDeleteBusy(false)
    setDeleteTarget(null)
    loadBackups()
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_backups')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
          {siteId && (
            <button className="btn btn-primary" onClick={runBackup} disabled={running}>
              {running && <span className="spinner" />}{t('btn_run_backup_now')}
            </button>
          )}
        </div>
      </header>

      {!siteId && (
        <SiteHint />
      )}

      {siteId && (
        <div className="card">
          {loading ? (
            <LoadingState text={t('loading')} />
          ) : backups.length === 0 ? (
            <p>{t('restore_no_backups')}</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>{t('table_timestamp')}</th>
                  <th>{t('table_host')}</th>
                  <th>{t('table_wp_version')}</th>
                  <th>{t('table_database')}</th>
                  <th>{t('table_files')}</th>
                  <th>{t('table_actions')}</th>
                </tr>
              </thead>
              <tbody>
                {backups.map(b => (
                  <tr key={b.id}>
                    <td>{b.timestamp || '-'}</td>
                    <td>{b.host || b.ssh_host || '-'}</td>
                    <td>{b.wp_version || '-'}</td>
                    <td>{b.db_name || '-'}</td>
                    <td>{b.file_count || 0}</td>
                    <td>
                      <button onClick={() => setDeleteTarget({ id: b.id, label: b.timestamp || b.id })} className="btn btn-danger btn-sm">{t('btn_delete')}</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      <JobStatus jobType="backup" siteId={siteId} />
      <JobHistory jobType="backup" siteId={siteId} />

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('modal_delete_backup')}
        message={deleteTarget ? t('modal_delete_backup_body') + ' (' + deleteTarget.label + ')' : ''}
        onConfirm={() => { if (deleteTarget) deleteBackup(deleteTarget.id) }}
        onCancel={() => setDeleteTarget(null)}
        busy={deleteBusy}
      />
    </>
  )
}
