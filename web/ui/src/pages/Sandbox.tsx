import { useState, useEffect } from 'react'
import JobStatus from '../components/JobStatus'
import ConfirmDialog from '../components/ConfirmDialog'
import { apiGet, apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useLanguage } from '../context/LanguageContext'

interface SandboxStatus {
  running: boolean
  site_id: string
  health_url: string
  wp_version: string
  healthy: boolean
  broken: boolean
  message: string
  ssh_port: number
  last_action: string
}

interface Backup {
  id: string
  timestamp: string
  snapshot_id: string
  wp_version: string
}

export default function Sandbox() {
  const [status, setStatus] = useState<SandboxStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [starting, setStarting] = useState(false)
  const [breaking, setBreaking] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [backups, setBackups] = useState<Backup[]>([])
  const [selectedSnapshot, setSelectedSnapshot] = useState('')
  const [confirmBreak, setConfirmBreak] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  const [drillAck, setDrillAck] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  async function refreshStatus() {
    setLoading(true)
    try {
      const res = await apiGet<SandboxStatus>('/api/v1/sandbox/status')
      if (res.success && res.data) {
        setStatus(res.data)
        if (res.data.site_id) loadBackups(res.data.site_id)
        else { setBackups([]); setSelectedSnapshot('') }
      } else if (res.error) {
        toast(res.error, 'error')
      }
    } catch (e: any) {
      toast(e.message || 'Failed to load sandbox status', 'error')
    }
    setLoading(false)
  }

  useEffect(() => { refreshStatus() }, [])
  useEffect(() => {
    if (status?.site_id && status.running) loadBackups(status.site_id)
  }, [status?.site_id, status?.running])

  async function loadBackups(siteId: string) {
    const res = await apiGet<Backup[]>('/api/v1/backups?site_id=' + siteId)
    if (res.success && res.data) {
      const list = Array.isArray(res.data) ? res.data : []
      setBackups(list)
      if (list.length > 0 && !selectedSnapshot) {
        setSelectedSnapshot(list[0].snapshot_id)
      }
    }
  }

  async function handleStart(e: React.FormEvent) {
    e.preventDefault()
    setStarting(true)
    try {
      const res = await apiPost<SandboxStatus>('/api/v1/sandbox/start')
      if (res.success && res.data) {
        toast(t('sandbox_started'), 'success')
        setStatus(res.data)
        if (res.data.site_id) loadBackups(res.data.site_id)
      } else {
        toast(res.error || t('sandbox_start_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('sandbox_start_failed'), 'error')
    }
    setStarting(false)
  }

  async function handleBackup() {
    if (!status?.site_id) return
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/backup', { site_id: status.site_id })
      if (res.success && res.data) {
        toast(t('sandbox_backup_queued'), 'info')
      } else {
        toast(res.error || t('sandbox_backup_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('sandbox_backup_failed'), 'error')
    }
  }

  async function handleBreak() {
    if (!drillAck) return
    setBreaking(true)
    try {
      const res = await apiPost<SandboxStatus>('/api/v1/sandbox/break')
      if (res.success && res.data) {
        toast(t('sandbox_broken'), 'info')
        setStatus(res.data)
      } else {
        toast(res.error || t('sandbox_break_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('sandbox_break_failed'), 'error')
    }
    setBreaking(false)
    setConfirmBreak(false)
  }

  async function handleRestore() {
    if (!status?.site_id || !selectedSnapshot) return
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/restore', {
        site_id: status.site_id,
        snapshot_id: selectedSnapshot,
        apply_db: true,
        apply_files: true,
        confirm: true,
      })
      if (res.success && res.data) {
        toast(t('sandbox_restore_queued'), 'info')
      } else {
        toast(res.error || t('sandbox_restore_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('sandbox_restore_failed'), 'error')
    }
  }

  async function handleStop() {
    setStopping(true)
    try {
      const res = await apiPost('/api/v1/sandbox/stop')
      if (res.success) {
        toast(t('sandbox_stopped'), 'success')
        setStatus(prev => prev ? { ...prev, running: false, healthy: false, broken: false } : prev)
      } else {
        toast(res.error || t('sandbox_stop_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('sandbox_stop_failed'), 'error')
    }
    setStopping(false)
    setConfirmStop(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_sandbox')}</h1>
        <div className="header-actions">
          <button className="btn" onClick={refreshStatus} disabled={loading}>
            {loading && <span className="spinner" />}{t('sandbox_refresh')}
          </button>
        </div>
      </header>

      <div className="card">
        <h3>{t('sandbox_status_title')}</h3>
        {!status && <p>{t('sandbox_status_loading')}</p>}
        {status && (
          <div className="form-group">
            <p>
              <strong>{t('sandbox_state')}:</strong>{' '}
              {status.running
                ? <span className="badge badge-success">{t('sandbox_running')}</span>
                : <span className="badge badge-muted">{t('sandbox_stopped')}</span>}
              {' '}
              {status.running && (status.healthy
                ? <span className="badge badge-success">{t('sandbox_healthy')}</span>
                : <span className="badge badge-error">{t('sandbox_unhealthy')}</span>)}
              {' '}
              {status.running && status.broken && <span className="badge badge-error">{t('sandbox_broken_badge')}</span>}
            </p>
            {status.running && status.health_url && (
              <p>
                <strong>{t('sandbox_site_url')}:</strong>{' '}
                <a href={status.health_url} target="_blank" rel="noopener noreferrer">{status.health_url}</a>
              </p>
            )}
            {status.running && (
              <p>
                <strong>{t('sandbox_wp_version')}:</strong> {status.wp_version || '-'}{' '}
                <strong style={{ marginLeft: 16 }}>{t('sandbox_ssh_port')}:</strong> {status.ssh_port || '-'}
              </p>
            )}
            {status.message && <p><strong>{t('sandbox_message')}:</strong> {status.message}</p>}
          </div>
        )}
      </div>

      {!status?.running && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>{t('sandbox_start_title')}</h3>
          <p>{t('sandbox_start_desc')}</p>
          <form onSubmit={handleStart}>
            <div className="form-group">
              <button type="submit" className="btn btn-primary" disabled={starting}>
                {starting && <span className="spinner" />}{t('sandbox_start_btn')}
              </button>
            </div>
          </form>
        </div>
      )}

      {status?.running && (
        <>
          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t('sandbox_backup_title')}</h3>
            <p>{t('sandbox_backup_desc')}</p>
            <div className="form-group">
              <button className="btn btn-primary" onClick={handleBackup}>
                {t('sandbox_backup_btn')}
              </button>
            </div>
            <JobStatus jobType="backup" siteId={status.site_id} />
          </div>

          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t('sandbox_drill_title')}</h3>
            <p>{t('sandbox_drill_desc')}</p>
            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={drillAck}
                  onChange={e => setDrillAck(e.target.checked)}
                />
                <span>{t('sandbox_drill_ack')}</span>
              </label>
              <p className="help-text">{t('sandbox_drill_hint')}</p>
            </div>
            <div className="form-group">
              <button
                className="btn btn-danger"
                onClick={() => setConfirmBreak(true)}
                disabled={!drillAck || breaking}
              >
                {breaking && <span className="spinner" />}{t('sandbox_break_btn')}
              </button>
            </div>
            {status.broken && (
              <div className="result-box error">{t('sandbox_broken_msg')}</div>
            )}
          </div>

          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t('sandbox_restore_title')}</h3>
            <p>{t('sandbox_restore_desc')}</p>
            <div className="form-group">
              <label>{t('sandbox_snapshot_label')}</label>
              <select value={selectedSnapshot} onChange={e => setSelectedSnapshot(e.target.value)}>
                <option value="">{t('sandbox_select_snapshot')}</option>
                {backups.map(b => (
                  <option key={b.id} value={b.snapshot_id}>
                    {b.timestamp} — {(b.snapshot_id || b.id).substring(0, 12)}
                  </option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <button
                className="btn btn-primary"
                onClick={handleRestore}
                disabled={!selectedSnapshot}
              >
                {t('sandbox_restore_btn')}
              </button>
            </div>
            <JobStatus jobType="restore" siteId={status.site_id} />
          </div>

          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t('sandbox_stop_title')}</h3>
            <p>{t('sandbox_stop_desc')}</p>
            <div className="form-group">
              <button className="btn btn-danger" onClick={() => setConfirmStop(true)} disabled={stopping}>
                {stopping && <span className="spinner" />}{t('sandbox_stop_btn')}
              </button>
            </div>
          </div>
        </>
      )}

      <ConfirmDialog
        open={confirmBreak}
        title={t('sandbox_break_confirm_title')}
        message={t('sandbox_break_confirm_msg')}
        onConfirm={handleBreak}
        onCancel={() => setConfirmBreak(false)}
        busy={breaking}
      />

      <ConfirmDialog
        open={confirmStop}
        title={t('sandbox_stop_confirm_title')}
        message={t('sandbox_stop_confirm_msg')}
        onConfirm={handleStop}
        onCancel={() => setConfirmStop(false)}
        busy={stopping}
      />
    </>
  )
}
