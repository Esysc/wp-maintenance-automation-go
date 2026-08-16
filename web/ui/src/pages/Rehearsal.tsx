import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus, { type Job } from '../components/JobStatus'
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
  upgrade_steps?: RehearsalStep[]
  rollback_steps?: RehearsalStep[]
  inventory_before?: RehearsalInventory
  inventory_after?: RehearsalInventory
  inventory_rollback?: RehearsalInventory
}

interface RehearsalStep {
  name: string
  command: string
  output: string
  success: boolean
}

interface RehearsalComponent {
  name: string
  version: string
  status?: string
  update?: string
  update_version?: string
  auto_update?: string
}

interface RehearsalInventory {
  core_version: string
  plugins: RehearsalComponent[]
  themes: RehearsalComponent[]
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
  const [envActive, setEnvActive] = useState(false)
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const [stopJobId, setStopJobId] = useState<string | null>(null)
  const [jobRefreshToken, setJobRefreshToken] = useState(0)
  const [lastFailure, setLastFailure] = useState('')
  const [starting, setStarting] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  const [forceCleanup, setForceCleanup] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()
  const rehearsalInProgress = starting || !!activeJobId || envActive

  function renderInventoryBlock(title: string, inv?: RehearsalInventory) {
    if (!inv) return null
    const plugins = Array.isArray(inv.plugins) ? inv.plugins : []
    const themes = Array.isArray(inv.themes) ? inv.themes : []
    return (
      <div className="card" style={{ marginTop: 12 }}>
        <h4 style={{ marginTop: 0 }}>{title}</h4>
        <p><strong>Core:</strong> {inv.core_version || '-'}</p>
        <p><strong>Plugins:</strong> {plugins.length}</p>
        {plugins.length > 0 && (
          <details>
            <summary>Show plugin inventory</summary>
            <div style={{ marginTop: 8 }}>
              {plugins.map((p, i) => (
                <div key={`p-${i}`} style={{ fontFamily: 'monospace', fontSize: 12 }}>
                  {p.name || '-'}
                  {p.version ? ` (v${p.version})` : ''}
                  {p.status ? ` - ${p.status}` : ''}
                  {p.update && p.update !== 'none' ? ` - update available: ${p.update_version || p.update}` : ' - up to date'}
                  {p.auto_update ? ` - auto-update: ${p.auto_update}` : ''}
                </div>
              ))}
            </div>
          </details>
        )}
        <p><strong>Themes:</strong> {themes.length}</p>
        {themes.length > 0 && (
          <details>
            <summary>Show theme inventory</summary>
            <div style={{ marginTop: 8 }}>
              {themes.map((th, i) => (
                <div key={`t-${i}`} style={{ fontFamily: 'monospace', fontSize: 12 }}>
                  {th.name || '-'}
                  {th.version ? ` (v${th.version})` : ''}
                  {th.status ? ` - ${th.status}` : ''}
                  {th.update && th.update !== 'none' ? ` - update available: ${th.update_version || th.update}` : ' - up to date'}
                  {th.auto_update ? ` - auto-update: ${th.auto_update}` : ''}
                </div>
              ))}
            </div>
          </details>
        )}
      </div>
    )
  }

  function renderStepBlock(title: string, steps?: RehearsalStep[]) {
    if (!steps || steps.length === 0) return null
    return (
      <div className="card" style={{ marginTop: 12 }}>
        <h4 style={{ marginTop: 0 }}>{title}</h4>
        {steps.map((s, i) => (
          <div key={`${title}-${i}`} style={{ marginBottom: 10 }}>
            <div><strong>{s.name}</strong> {s.success ? 'ok' : 'failed'}</div>
            <div style={{ fontFamily: 'monospace', fontSize: 12, opacity: 0.8 }}>{s.command}</div>
            {s.output && <pre style={{ whiteSpace: 'pre-wrap', marginTop: 4 }}>{s.output}</pre>}
          </div>
        ))}
      </div>
    )
  }

  useEffect(() => {
    if (siteId) { loadSnapshots(); checkRunningRehearsal() }
    else { setSnapshots([]); setSelectedSnapshot(''); setEnv(null); setEnvActive(false) }
  }, [siteId])

  useEffect(() => {
    if (!siteId) return
    const iv = window.setInterval(() => { void checkRunningRehearsal() }, 4000)
    return () => window.clearInterval(iv)
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
        // Keep existing env visibility during transient queued/running states.
        setActiveJobId(d.job_id || null)
        setStopJobId(d.job_id || null)
      } else if (d.env) {
        setEnv(d.env)
        setEnvActive(!!d.active)
        setActiveJobId(d.active ? d.job_id : null)
        setStopJobId(d.job_id || null)
      } else {
        setEnv(null)
        setEnvActive(false)
        setActiveJobId(null)
        setStopJobId(null)
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
        setEnv(null)
        setLastFailure('')
        setActiveJobId(res.data.job_id)
        setStopJobId(res.data.job_id)
        setJobRefreshToken(v => v + 1)
      } else {
        toast(res.error || t('rehearsal_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('rehearsal_failed'), 'error')
    }
    setStarting(false)
  }

  async function handleStop() {
    if (!stopJobId) return
    setStopping(true)
    try {
      const res = await apiPost('/api/v1/rehearsal/' + stopJobId + '/stop' + (forceCleanup ? '?force=1' : ''))
      if (res.success) {
        toast(t('rehearsal_stopped'), 'success')
        setEnv(null)
        setEnvActive(false)
        setActiveJobId(null)
        setStopJobId(null)
        setForceCleanup(false)
      } else {
        toast(res.error || t('rehearsal_stop_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('rehearsal_stop_failed'), 'error')
    }
    setStopping(false)
    setConfirmStop(false)
  }

  function extractJSONArray(raw: string): string {
    const s = (raw || '').trim()
    if (!s) return ''
    const i = s.indexOf('[')
    const j = s.lastIndexOf(']')
    if (i < 0 || j < i) return ''
    return s.slice(i, j + 1)
  }

  async function wpcli(jobId: string, args: string): Promise<string> {
    const res = await apiPost<{ success: boolean; output: string; error?: string }>(`/api/v1/rehearsal/${jobId}/wpcli`, { args })
    if (!res.success || !res.data) {
      throw new Error(res.error || 'wp-cli request failed')
    }
    if (!res.data.success) {
      throw new Error(res.data.error || res.data.output || 'wp-cli command failed')
    }
    return res.data.output || ''
  }

  async function refreshInventoryFromWPCLI(jobId: string) {
    const [coreRaw, pluginsRaw, themesRaw] = await Promise.all([
      wpcli(jobId, 'core version'),
      wpcli(jobId, 'plugin list --format=json --fields=name,status,version,update,update_version,auto_update'),
      wpcli(jobId, 'theme list --format=json --fields=name,status,version,update,update_version,auto_update'),
    ])

    let plugins: RehearsalComponent[] = []
    let themes: RehearsalComponent[] = []
    const pluginsJSON = extractJSONArray(pluginsRaw)
    const themesJSON = extractJSONArray(themesRaw)
    if (pluginsJSON) {
      try { plugins = JSON.parse(pluginsJSON) as RehearsalComponent[] } catch { /* ignore */ }
    }
    if (themesJSON) {
      try { themes = JSON.parse(themesJSON) as RehearsalComponent[] } catch { /* ignore */ }
    }

    setEnv(prev => {
      if (!prev) return prev
      return {
        ...prev,
        inventory_after: {
          core_version: (coreRaw || '').trim(),
          plugins,
          themes,
        },
      }
    })
  }

  async function handleRunUpdates() {
    if (!stopJobId || !envActive) return
    setUpdating(true)
    try {
      const commands = [
        'core update',
        'plugin update --all',
        'theme update --all',
        'language core update',
        'language plugin update --all',
        'language theme update --all',
        'core update-db',
      ]

      for (const cmd of commands) {
        try {
          await wpcli(stopJobId, cmd)
        } catch {
          // Continue with remaining update commands, similar to rehearsal stage behavior.
        }
      }

      await refreshInventoryFromWPCLI(stopJobId)
      toast('Rehearsal update completed', 'success')
    } catch (e: any) {
      toast(e?.message || 'Rehearsal update failed', 'error')
    }
    setUpdating(false)
  }

  function onRehearsalReady(jobId: string) {
    (async () => {
      const res = await apiGet<RehearsalJob>('/api/v1/rehearsal/' + jobId)
      if (res.success && res.data && res.data.status === 'completed' && res.data.result) {
        try {
          setEnv(JSON.parse(res.data.result))
          setEnvActive(true)
          setActiveJobId(jobId)
          setStopJobId(jobId)
        } catch { /* ignore */ }
      }
    })()
  }

  function onJobFinished(job: Job) {
    if (job.status === 'failed' || (job.status === 'cancelled' && job.error)) {
      setLastFailure(job.error || `Rehearsal ${job.status}`)
    }
    void checkRunningRehearsal()
    void loadSnapshots()
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_rehearsal')}</h1>
        <div className="header-actions">
          {activeJobId && <span className="badge badge-success">Running</span>}
          {!activeJobId && envActive && <span className="badge badge-success">Active</span>}
          {!activeJobId && env && !envActive && <span className="badge badge-muted">Inactive</span>}
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {!siteId && (
        <SiteHint />
      )}

      {siteId && !rehearsalInProgress && !env && (
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

      <JobStatus
        jobType="rehearsal"
        siteId={siteId}
        refreshToken={jobRefreshToken}
        onReady={onRehearsalReady}
        onFinished={onJobFinished}
      />

      {lastFailure && (
        <div className="result-box error" style={{ marginTop: 12 }}>
          <strong>Rehearsal failed:</strong>
          <div style={{ marginTop: 6, whiteSpace: 'pre-wrap' }}>{lastFailure}</div>
        </div>
      )}

      {env && (
        <>
          <div className="card" style={{ marginTop: 16 }}>
            <h3>{envActive ? t('rehearsal_running_title') : 'Last Rehearsal Snapshot'}</h3>
            <p><strong>{t('rehearsal_health_url')}:</strong> <a href={env.health_url} target="_blank" rel="noopener">{env.health_url}</a></p>
            <p><strong>{t('rehearsal_wp_version')}:</strong> {env.wp_version || '-'}</p>
            <p><strong>{t('rehearsal_db_name')}:</strong> {env.db_name || '-'}</p>
            <p><strong>{t('rehearsal_snapshot_label')}:</strong> {env.snapshot_id || '-'}</p>
            {!envActive && <div className="result-box info">Rehearsal metadata is available, but staging containers are not currently running.</div>}
            {stopping && <div className="result-box info"><span className="spinner" /> {t('rehearsal_stopping')}</div>}
            {envActive && stopJobId && (
              <div className="form-group" style={{ marginTop: 8 }}>
                <button onClick={handleRunUpdates} className="btn btn-primary" disabled={updating || stopping}>
                  {updating && <span className="spinner" />}Run Updates
                </button>
              </div>
            )}
            {stopJobId && (
              <div className="form-group" style={{ marginTop: 16 }}>
                <button onClick={() => setConfirmStop(true)} className="btn btn-danger" disabled={stopping}>
                  {stopping && <span className="spinner" />}{envActive ? t('btn_stop_rehearsal') : 'Cleanup Rehearsal'}
                </button>
              </div>
            )}
          </div>

          {renderStepBlock('Upgrade Stage Steps', env.upgrade_steps)}
          {renderStepBlock('Rollback Stage Steps', env.rollback_steps)}
          {renderInventoryBlock('Inventory Before Upgrade', env.inventory_before)}
          {renderInventoryBlock('Inventory After Upgrade', env.inventory_after)}
          {renderInventoryBlock('Inventory After Rollback', env.inventory_rollback)}
        </>
      )}

      <ConfirmDialog
        open={confirmStop}
        title={t('rehearsal_stop_title')}
        message={t('rehearsal_stop_confirm')}
        onConfirm={handleStop}
        onCancel={() => setConfirmStop(false)}
        busy={stopping}
        >
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 12 }}>
            <input
              type="checkbox"
              checked={forceCleanup}
              onChange={e => setForceCleanup(e.target.checked)}
              disabled={stopping}
            />
            Force cleanup if containers are still running
          </label>
        </ConfirmDialog>
    </>
  )
}
