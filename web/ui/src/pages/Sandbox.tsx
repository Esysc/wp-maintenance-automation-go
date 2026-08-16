import { useState, useEffect, useRef } from 'react'
import JobStatus, { type Job } from '../components/JobStatus'
import ConfirmDialog from '../components/ConfirmDialog'
import { apiGet, apiPost, apiDelete } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useLanguage } from '../context/LanguageContext'

interface SandboxStatus {
  running: boolean
  site_id: string
  health_url: string
  site_url: string
  wp_version: string
  healthy: boolean
  broken: boolean
  message: string
  ssh_port: number
  last_action: string
  logs?: string[]
}

interface Backup {
  id: string
  timestamp: string
  snapshot_id: string
  wp_version: string
}

interface ActionNote {
  kind: 'success' | 'info' | 'error'
  text: string
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
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [drillAck, setDrillAck] = useState(false)
  const [startupLogs, setStartupLogs] = useState<string[]>([])
  const [showStartupLogs, setShowStartupLogs] = useState(false)
  const [backupActive, setBackupActive] = useState(false)
  const [restoreActive, setRestoreActive] = useState(false)
  const [backupJobKey, setBackupJobKey] = useState(0)
  const [restoreJobKey, setRestoreJobKey] = useState(0)
  const [actionNote, setActionNote] = useState<ActionNote | null>(null)
  const startLogSource = useRef<EventSource | null>(null)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => {
    return () => { if (startLogSource.current) startLogSource.current.close() }
  }, [])

  function formatTimestamp(raw: string) {
    const m = /^(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})$/.exec(raw)
    if (!m) return raw
    return `${m[1]}-${m[2]}-${m[3]} ${m[4]}:${m[5]}:${m[6]}`
  }

  function logLine(message: string) {
    return `[${new Date().toLocaleTimeString()}] ${message}`
  }

  async function refreshStatus(syncBackups = true) {
    setLoading(syncBackups)
    try {
      const res = await apiGet<SandboxStatus>('/api/v1/sandbox/status')
      if (res.success && res.data) {
        setStatus(res.data)
        if (syncBackups && res.data.site_id) loadBackups(res.data.site_id)
        else if (!res.data.site_id) { setBackups([]); setSelectedSnapshot('') }
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
    const interval = window.setInterval(() => {
      void refreshStatus(false)
    }, 10000)

    return () => window.clearInterval(interval)
  }, [])
  useEffect(() => {
    if (status?.site_id && status.running) loadBackups(status.site_id)
  }, [status?.site_id, status?.running])

  async function loadBackups(siteId: string, selectNewest = false) {
    try {
      const res = await apiGet<Backup[]>('/api/v1/backups?site_id=' + siteId)
      if (res.success && res.data) {
        const list = Array.isArray(res.data) ? res.data : []
        setBackups(list)
        if (list.length > 0) {
          if (selectNewest || !selectedSnapshot || !list.some(b => b.snapshot_id === selectedSnapshot)) {
            setSelectedSnapshot(list[0].snapshot_id)
          }
        } else {
          setSelectedSnapshot('')
        }
      } else if (res.error) {
        toast(res.error, 'error')
      }
    } catch (e: any) {
      toast(e.message || 'Failed to load backups', 'error')
    }
  }

  async function handleStart(e: React.FormEvent) {
    e.preventDefault()
    setStarting(true)
    setStartupLogs([logLine('Starting sandbox...')])
    setShowStartupLogs(true)

    if (startLogSource.current) startLogSource.current.close()
    const es = new EventSource('/api/v1/sandbox/start/logs')
    startLogSource.current = es
    es.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data)
        if (data.done) {
          es.close()
          if (startLogSource.current === es) startLogSource.current = null
          if (!data.success) {
            setStartupLogs(prev => [...prev, logLine(t('sandbox_start_failed'))])
          }
          return
        }
        if (data.line) {
          setStartupLogs(prev => [...prev, data.line])
        }
      } catch { /* ignore malformed event */ }
    }

    try {
      const res = await apiPost<SandboxStatus>('/api/v1/sandbox/start')
      if (res.success && res.data) {
        toast(t('sandbox_started'), 'success')
        setStatus(res.data)
        setStartupLogs(res.data.logs && res.data.logs.length > 0 ? res.data.logs : [logLine('Sandbox started')])
        if (res.data.site_id) loadBackups(res.data.site_id)
      } else {
        setStartupLogs(prev => [...prev, logLine(res.error || t('sandbox_start_failed'))])
        toast(res.error || t('sandbox_start_failed'), 'error')
      }
    } catch (e: any) {
      setStartupLogs(prev => [...prev, logLine(e.message || t('sandbox_start_failed'))])
      toast(e.message || t('sandbox_start_failed'), 'error')
    }
    es.close()
    if (startLogSource.current === es) startLogSource.current = null
    setStarting(false)
  }

  async function handleBackup() {
    if (!status?.site_id || backupActive) return
    setBackupActive(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/backup', { site_id: status.site_id })
      if (res.success && res.data) {
        setBackupJobKey(k => k + 1)
        toast(t('sandbox_backup_queued'), 'info')
      } else {
        setBackupActive(false)
        toast(res.error || t('sandbox_backup_failed'), 'error')
      }
    } catch (e: any) {
      setBackupActive(false)
      toast(e.message || t('sandbox_backup_failed'), 'error')
    }
  }

  function onBackupFinished(job: Job) {
    setBackupActive(false)
    if (job.status === 'completed') {
      let timestamp = ''
      try {
        const parsed = JSON.parse(job.result)
        timestamp = parsed.timestamp || ''
      } catch { /* ignore */ }
      const when = timestamp ? formatTimestamp(timestamp) : ''
      toast(when ? t('sandbox_backup_done') + ' — ' + when : t('sandbox_backup_done'), 'success')
      setActionNote({ kind: 'success', text: when ? t('sandbox_backup_note_done') + ' — ' + when : t('sandbox_backup_note_done') })
      if (status?.site_id) loadBackups(status.site_id, true)
    } else {
      toast(t('sandbox_backup_failed'), 'error')
      setActionNote({ kind: 'error', text: job.error || t('sandbox_backup_failed') })
    }
  }

  async function handleBreak() {
    if (!drillAck || backups.length === 0) return
    setBreaking(true)
    try {
      const res = await apiPost<SandboxStatus>('/api/v1/sandbox/break')
      if (res.success && res.data) {
        toast(t('sandbox_broken'), 'info')
        setStatus(res.data)
        setActionNote({ kind: 'info', text: t('sandbox_broken_note') })
      } else {
        setActionNote({ kind: 'error', text: res.error || t('sandbox_break_failed') })
        toast(res.error || t('sandbox_break_failed'), 'error')
      }
    } catch (e: any) {
      setActionNote({ kind: 'error', text: e.message || t('sandbox_break_failed') })
      toast(e.message || t('sandbox_break_failed'), 'error')
    }
    setBreaking(false)
    setConfirmBreak(false)
  }

  async function handleRestore() {
    if (!status?.site_id || !selectedSnapshot || restoreActive) return
    setRestoreActive(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/restore', {
        site_id: status.site_id,
        snapshot_id: selectedSnapshot,
        apply_db: true,
        apply_files: true,
        confirm: true,
      })
      if (res.success && res.data) {
        setRestoreJobKey(k => k + 1)
        toast(t('sandbox_restore_queued'), 'info')
      } else {
        setRestoreActive(false)
        toast(res.error || t('sandbox_restore_failed'), 'error')
      }
    } catch (e: any) {
      setRestoreActive(false)
      toast(e.message || t('sandbox_restore_failed'), 'error')
    }
  }

  function onRestoreFinished(job: Job) {
    setRestoreActive(false)
    if (job.status === 'completed') {
      toast(t('sandbox_restore_done'), 'success')
      setActionNote({ kind: 'success', text: t('sandbox_restore_note_done') })
      if (status?.site_id) refreshStatus(false)
    } else {
      toast(t('sandbox_restore_failed'), 'error')
      setActionNote({ kind: 'error', text: job.error || t('sandbox_restore_failed') })
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

  async function handleDelete() {
    setDeleting(true)
    try {
      const res = await apiDelete('/api/v1/sandbox')
      if (res.success) {
        toast(t('sandbox_deleted'), 'success')
        setStatus(prev => prev ? { ...prev, running: false, healthy: false, broken: false, site_id: '', site_url: '', health_url: '', wp_version: '', message: '' } : prev)
        setBackups([])
        setSelectedSnapshot('')
      } else {
        toast(res.error || t('sandbox_delete_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('sandbox_delete_failed'), 'error')
    }
    setDeleting(false)
    setConfirmDelete(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_sandbox')}</h1>
        <div className="header-actions">
          <button className="btn" onClick={() => void refreshStatus()} disabled={loading}>
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
            {status.running && (status.site_url || status.health_url) && (
              <p>
                <strong>{t('sandbox_site_url')}:</strong>{' '}
                <a href={status.site_url || status.health_url} target="_blank" rel="noopener noreferrer">
                  {status.site_url || status.health_url}
                </a>
                {status.site_url && (
                  <button
                    className="btn"
                    style={{ marginLeft: 8 }}
                    onClick={() => window.open(status.site_url!, 'sandbox_site', 'width=1200,height=800,resizable=yes,scrollbars=yes')}
                  >
                    {t('sandbox_open_popup')}
                  </button>
                )}
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

      {actionNote && (
        <div className={`result-box ${actionNote.kind}`} style={{ marginTop: 16 }}>
          {actionNote.text}
        </div>
      )}

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

      {showStartupLogs && startupLogs.length > 0 && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>Sandbox start logs</h3>
          <div className="result-box info" aria-live="polite">
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}>
              {startupLogs.join('\n')}
            </pre>
          </div>
        </div>
      )}

      {status?.running && (
        <>
          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t('sandbox_backup_title')}</h3>
            <p>{t('sandbox_backup_desc')}</p>
            <div className="form-group">
              <button className="btn btn-primary" onClick={handleBackup} disabled={backupActive}>
                {backupActive && <span className="spinner" />}{t('sandbox_backup_btn')}
              </button>
            </div>
            <JobStatus
              key={`backup-${backupJobKey}`}
              jobType="backup"
              siteId={status.site_id}
              onActiveChange={setBackupActive}
              onFinished={onBackupFinished}
            />
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
                disabled={!drillAck || breaking || backups.length === 0}
              >
                {breaking && <span className="spinner" />}{t('sandbox_break_btn')}
              </button>
            </div>
            {backups.length === 0 && (
              <p className="help-text" style={{ color: '#d33' }}>{t('sandbox_drill_no_backup')}</p>
            )}
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
                disabled={!selectedSnapshot || restoreActive}
              >
                {restoreActive && <span className="spinner" />}{t('sandbox_restore_btn')}
              </button>
            </div>
            <JobStatus
              key={`restore-${restoreJobKey}`}
              jobType="restore"
              siteId={status.site_id}
              onActiveChange={setRestoreActive}
              onFinished={onRestoreFinished}
            />
          </div>

          <div className="card" style={{ marginTop: 16 }}>
            <h3>{t('sandbox_stop_title')}</h3>
            <p>{t('sandbox_stop_desc')}</p>
            <div className="form-group">
              <button className="btn btn-danger" onClick={() => setConfirmStop(true)} disabled={stopping || backupActive || restoreActive}>
                {stopping && <span className="spinner" />}{t('sandbox_stop_btn')}
              </button>
            </div>
          </div>
        </>
      )}

      {status?.site_id && (
        <div className="card" style={{ marginTop: 16, borderColor: '#d33' }}>
          <h3>{t('sandbox_delete_title')}</h3>
          <p>{t('sandbox_delete_desc')}</p>
          <div className="form-group">
            <button className="btn btn-danger" onClick={() => setConfirmDelete(true)} disabled={deleting}>
              {deleting && <span className="spinner" />}{t('sandbox_delete_btn')}
            </button>
          </div>
        </div>
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

      <ConfirmDialog
        open={confirmDelete}
        title={t('sandbox_delete_confirm_title')}
        message={t('sandbox_delete_confirm_msg')}
        onConfirm={handleDelete}
        onCancel={() => setConfirmDelete(false)}
        busy={deleting}
      />
    </>
  )
}
