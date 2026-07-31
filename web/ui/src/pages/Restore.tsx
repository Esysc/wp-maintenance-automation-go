import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import JobHistory from '../components/JobHistory'
import ConfirmDialog from '../components/ConfirmDialog'
import SiteHint from '../components/SiteHint'
import { apiGet, apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'

interface Backup {
  id: string
  timestamp: string
  snapshot_id: string
  wp_version: string
  host: string
}

export default function Restore() {
  const { siteId, setSiteId } = useSite()
  const [backups, setBackups] = useState<Backup[]>([])
  const [snapshotId, setSnapshotId] = useState('')
  const [applyDb, setApplyDb] = useState(true)
  const [applyFiles, setApplyFiles] = useState(true)
  const [confirm, setConfirm] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => {
    if (siteId) loadBackups()
    else { setBackups([]); setSnapshotId('') }
  }, [siteId])

  async function loadBackups() {
    setSnapshotId('')
    const res = await apiGet<Backup[]>('/api/v1/backups?site_id=' + siteId)
    if (res.success && res.data) {
      setBackups(Array.isArray(res.data) ? res.data : [])
    } else if (res.error) {
      toast(res.error, 'error')
    }
  }

  function requestRestore(e: React.FormEvent) {
    e.preventDefault()
    if (!siteId || !snapshotId) { toast(t('error_snapshot_required'), 'error'); return }
    setConfirm(true)
  }

  async function handleRestore() {
    if (!siteId || !snapshotId) return
    setRestoring(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/restore', {
        site_id: siteId,
        snapshot_id: snapshotId,
        apply_db: applyDb,
        apply_files: applyFiles,
        confirm: true,
      })
      if (res.success) {
        toast(t('restore_initiated'), 'info')
      } else {
        toast(res.error || t('restore_request_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('restore_request_failed'), 'error')
    }
    setRestoring(false)
    setConfirm(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_restore')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {!siteId && (
        <SiteHint />
      )}

      {siteId && backups.length === 0 && (
        <div className="card"><p>{t('restore_no_backups')}</p></div>
      )}

      {siteId && backups.length > 0 && (
        <div className="card">
          <form onSubmit={requestRestore}>
            <div className="form-group">
              <label>{t('restore_snapshot_label')}</label>
              <select value={snapshotId} onChange={e => setSnapshotId(e.target.value)} required>
                <option value="" disabled>{t('rehearsal_select_snapshot')}</option>
                {backups.map(b => (
                  <option key={b.id} value={b.snapshot_id || b.timestamp}>
                    {b.timestamp} &mdash; {b.wp_version || ''} ({b.host})
                  </option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <label className="checkbox-label">
                <input type="checkbox" checked={applyDb} onChange={e => setApplyDb(e.target.checked)} />
                <span>{t('apply_db_restore')}</span>
              </label>
            </div>
            <div className="form-group">
              <label className="checkbox-label">
                <input type="checkbox" checked={applyFiles} onChange={e => setApplyFiles(e.target.checked)} />
                <span>{t('apply_files_restore')}</span>
              </label>
            </div>
            <div className="form-group">
              <button type="submit" className="btn btn-warning">{t('btn_restore')}</button>
            </div>
          </form>
        </div>
      )}

      <JobStatus jobType="restore" siteId={siteId} />
      <JobHistory jobType="restore" siteId={siteId} />

      <ConfirmDialog
        open={confirm}
        title={t('modal_restore_title')}
        message={t('modal_restore_body')}
        onConfirm={handleRestore}
        onCancel={() => setConfirm(false)}
        busy={restoring}
      />
    </>
  )
}
