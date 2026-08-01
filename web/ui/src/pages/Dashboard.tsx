import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiGet, apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'
import SiteSelector from '../components/SiteSelector'
import ConfirmDialog from '../components/ConfirmDialog'

interface StatusData {
  backups: number
  snapshots: number
  sites: number
  server: string
  api: string
}

export default function Dashboard() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [confirmBackup, setConfirmBackup] = useState(false)
  const [backupBusy, setBackupBusy] = useState(false)
  const { toast } = useToast()
  const navigate = useNavigate()
  const { siteId, setSiteId } = useSite()
  const { t } = useLanguage()

  useEffect(() => {
    loadStatus()
    const iv = setInterval(loadStatus, 30000)
    return () => clearInterval(iv)
  }, [])

  async function loadStatus() {
    const res = await apiGet<StatusData>('/api/v1/status')
    if (res.success && res.data) setStatus(res.data)
  }

  function requestBackup() {
    if (!siteId) { toast(t('error_site_id_required'), 'error'); return }
    setConfirmBackup(true)
  }

  async function doBackup() {
    setBackupBusy(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/backup', { site_id: siteId })
      if (res.success) {
        toast(t('backup_queued'), 'success')
        setTimeout(loadStatus, 2000)
      } else {
        toast(res.error || t('backup_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('backup_failed'), 'error')
    }
    setBackupBusy(false)
    setConfirmBackup(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_dashboard')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} label={t('label_select_site')} />
          {siteId && <button className="btn btn-primary" onClick={requestBackup}>{t('action_run_backup')}</button>}
          <button className="btn btn-secondary" onClick={() => navigate('/sites')}>{t('btn_manage_sites')}</button>
        </div>
      </header>

      <div className="stats-grid">
        <div className="stat-card">
          <h3>{t('status_backups')}</h3>
          <p className="stat-value">{status?.backups ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>{t('status_snapshots')}</h3>
          <p className="stat-value">{status?.snapshots ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>{t('status_server')}</h3>
          <p className="stat-value">{status?.server ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>{t('status_uptime')}</h3>
          <p className="stat-value">{status?.api ?? t('status_online')}</p>
        </div>
      </div>

      <div className="action-grid">
        {siteId && (
          <button type="button" className="action-card" onClick={requestBackup}>
            <h3>{t('action_run_backup')}</h3>
            <p>{t('action_create_backup_now')}</p>
          </button>
        )}
        <button type="button" className="action-card" onClick={() => navigate('/upgrade')}>
          <h3>{t('action_run_upgrade')}</h3>
          <p>{t('action_upgrade_rollback')}</p>
        </button>
        <button type="button" className="action-card" onClick={() => navigate('/snapshots')}>
          <h3>{t('action_view_snapshots')}</h3>
          <p>{t('action_view_restic_snapshots')}</p>
        </button>
      </div>

      <ConfirmDialog
        open={confirmBackup}
        title={t('modal_backup_run_title')}
        message={t('modal_backup_run_body')}
        onConfirm={doBackup}
        onCancel={() => setConfirmBackup(false)}
        busy={backupBusy}
      />
    </>
  )
}
