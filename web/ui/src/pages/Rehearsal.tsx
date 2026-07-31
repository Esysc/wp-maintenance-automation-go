import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import ConfirmDialog from '../components/ConfirmDialog'
import SiteHint from '../components/SiteHint'
import { apiGet, apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'

interface RehearsalEnv {
  health_url: string
  wp_version: string
  db_name: string
  snapshot_id: string
}

interface RehearsalJob {
  id: string
  status: string
  result: string
}

interface RehearsalActive {
  job_id: string
  status: string
  active: boolean
  env: RehearsalEnv | null
}

interface ResticSnapshot {
  short_id: string
  id: string
  time: string
  tags: string[]
}

export default function Rehearsal() {
  const { siteId, setSiteId } = useSite()
  const [snapshots, setSnapshots] = useState<ResticSnapshot[]>([])
  const [selectedSnapshot, setSelectedSnapshot] = useState('')
  const [env, setEnv] = useState<RehearsalEnv | null>(null)
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => {
    if (siteId) { loadSnapshots(); checkRunningRehearsal() }
    else { setSnapshots([]); setSelectedSnapshot(''); setEnv(null) }
  }, [siteId])

  async function loadSnapshots() {
    const res = await apiGet<ResticSnapshot[]>('/api/v1/snapshots')
    if (res.success && res.data) {
      const all = Array.isArray(res.data) ? res.data : []
      setSnapshots(all.filter(s => s.tags?.includes(siteId)))
    } else if (res.error) {
      toast(res.error, 'error')
    }
  }

  async function checkRunningRehearsal() {
    const res = await apiGet<RehearsalActive>('/api/v1/rehearsal/active?site_id=' + siteId)
    if (res.success && res.data) {
      const d = res.data
      if (d.status === 'running' || d.status === 'queued') {
        setEnv(null)
        setActiveJobId(d.job_id || null)
      } else if (d.active && d.env) {
        setEnv(d.env)
        setActiveJobId(d.job_id)
      } else {
        setEnv(null)
        setActiveJobId(null)
      }
    }
  }

  async function handleStart(e: React.FormEvent) {
    e.preventDefault()
    if (!siteId || !selectedSnapshot) { toast(t('error_snapshot_required'), 'error'); return }
    setStarting(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/rehearsal', {
        site_id: siteId,
        snapshot_id: selectedSnapshot,
      })
      if (res.success && res.data) {
        toast(t('rehearsal_queued'), 'info')
        setActiveJobId(res.data.job_id)
      } else {
        toast(res.error || t('rehearsal_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('rehearsal_failed'), 'error')
    }
    setStarting(false)
  }

  async function handleStop() {
    if (!activeJobId) return
    setStopping(true)
    try {
      const res = await apiPost('/api/v1/rehearsal/' + activeJobId + '/stop')
      if (res.success) {
        toast(t('rehearsal_stopped'), 'success')
        setEnv(null)
        setActiveJobId(null)
      } else {
        toast(res.error || t('rehearsal_stop_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('rehearsal_stop_failed'), 'error')
    }
    setStopping(false)
    setConfirmStop(false)
  }

  function onRehearsalReady(jobId: string) {
    (async () => {
      const res = await apiGet<RehearsalJob>('/api/v1/rehearsal/' + jobId)
      if (res.success && res.data && res.data.status === 'completed' && res.data.result) {
        try {
          setEnv(JSON.parse(res.data.result))
          setActiveJobId(jobId)
        } catch { /* ignore */ }
      }
    })()
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_rehearsal')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {!siteId && (
        <SiteHint />
      )}

      {siteId && (
        <div className="card">
          <h3>{t('rehearsal_start_title')}</h3>
          <p>{t('rehearsal_start_desc')}</p>
          <form onSubmit={handleStart}>
            <div className="form-group">
              <label>{t('rehearsal_snapshot_label')}</label>
              <select value={selectedSnapshot} onChange={e => setSelectedSnapshot(e.target.value)} required>
                <option value="" disabled>{t('rehearsal_select_snapshot')}</option>
                {snapshots.map(s => (
                  <option key={s.short_id || s.id} value={s.short_id || s.id}>
                    {(s.short_id || s.id).substring(0, 16)} &mdash; {s.time || ''}
                  </option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <button type="submit" className="btn btn-primary" disabled={starting}>
                {starting && <span className="spinner" />}{t('btn_start_rehearsal')}
              </button>
            </div>
          </form>
        </div>
      )}

      <JobStatus jobType="rehearsal" siteId={siteId} onReady={onRehearsalReady} />

      {env && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>{t('rehearsal_running_title')}</h3>
          <p><strong>{t('rehearsal_health_url')}:</strong> <a href={env.health_url} target="_blank" rel="noopener">{env.health_url}</a></p>
          <p><strong>{t('rehearsal_wp_version')}:</strong> {env.wp_version || '-'}</p>
          <p><strong>{t('rehearsal_db_name')}:</strong> {env.db_name || '-'}</p>
          <p><strong>{t('rehearsal_snapshot_label')}:</strong> {env.snapshot_id || '-'}</p>
          {stopping && <div className="result-box info"><span className="spinner" /> {t('rehearsal_stopping')}</div>}
          <div className="form-group" style={{ marginTop: 16 }}>
            <button onClick={() => setConfirmStop(true)} className="btn btn-danger" disabled={stopping}>
              {stopping && <span className="spinner" />}{t('btn_stop_rehearsal')}
            </button>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirmStop}
        title={t('rehearsal_stop_title')}
        message={t('rehearsal_stop_confirm')}
        onConfirm={handleStop}
        onCancel={() => setConfirmStop(false)}
        busy={stopping}
      />
    </>
  )
}
