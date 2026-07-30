import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import { apiGet, apiPost, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

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

interface ResticSnapshot {
  short_id: string
  id: string
  time: string
  tags: string[]
}

export default function Rehearsal() {
  const [siteId, setSiteId] = useState('')
  const [snapshots, setSnapshots] = useState<ResticSnapshot[]>([])
  const [selectedSnapshot, setSelectedSnapshot] = useState('')
  const [env, setEnv] = useState<RehearsalEnv | null>(null)
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const [stopping, setStopping] = useState(false)
  const { toast } = useToast()

  useEffect(() => {
    if (siteId) { loadSnapshots(); checkRunningRehearsal() }
    else { setSnapshots([]); setSelectedSnapshot(''); setEnv(null) }
  }, [siteId])

  async function loadSnapshots() {
    const res = await apiGet<ResticSnapshot[]>('/api/v1/snapshots')
    if (res.success && res.data) {
      const all = Array.isArray(res.data) ? res.data : []
      setSnapshots(all.filter(s => s.tags?.includes(siteId)))
    }
  }

  async function checkRunningRehearsal() {
    const res = await apiGet<RehearsalJob>('/api/v1/jobs?site_id=' + siteId + '&type=rehearsal')
    if (res.success && res.data) {
      const j = res.data
      if (j.status === 'completed' && j.result) {
        try {
          setEnv(JSON.parse(j.result))
          setActiveJobId(j.id)
        } catch { /* ignore */ }
      } else if (j.status === 'running' || j.status === 'queued') {
        setActiveJobId(j.id)
      }
    }
  }

  async function handleStart(e: React.FormEvent) {
    e.preventDefault()
    if (!siteId || !selectedSnapshot) { toast('Please select a snapshot', 'error'); return }
    const res = await apiPost<{ job_id: string }>('/api/v1/rehearsal', {
      site_id: siteId,
      snapshot_id: selectedSnapshot,
    })
    if (res.success && res.data) {
      toast('Rehearsal queued', 'info')
      setActiveJobId(res.data.job_id)
    } else {
      toast(res.error || 'Rehearsal failed', 'error')
    }
  }

  async function handleStop() {
    if (!activeJobId) return
    setStopping(true)
    try {
      const res = await apiPost('/api/v1/rehearsal/' + activeJobId + '/stop')
      if (res.success) {
        toast('Rehearsal stopped', 'success')
        setEnv(null)
        setActiveJobId(null)
      } else {
        toast(res.error || 'Failed to stop', 'error')
      }
    } catch (e: any) {
      toast(e.message || 'Failed to stop', 'error')
    }
    setStopping(false)
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
        <h1>Staging Rehearsal</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {siteId && (
        <div className="card">
          <h3>Start a Rehearsal</h3>
          <p>Restore a backup snapshot into an isolated Docker environment to test upgrades and verify site health before applying to production.</p>
          <form onSubmit={handleStart}>
            <div className="form-group">
              <label>Snapshot</label>
              <select value={selectedSnapshot} onChange={e => setSelectedSnapshot(e.target.value)} required>
                <option value="" disabled>Select a snapshot...</option>
                {snapshots.map(s => (
                  <option key={s.short_id || s.id} value={s.short_id || s.id}>
                    {(s.short_id || s.id).substring(0, 16)} &mdash; {s.time || ''}
                  </option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <button type="submit" className="btn btn-primary">Start Rehearsal</button>
            </div>
          </form>
        </div>
      )}

      <JobStatus jobType="rehearsal" siteId={siteId} onReady={onRehearsalReady} />

      {env && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>Active Rehearsal</h3>
          <p><strong>Staging URL:</strong> <a href={env.health_url} target="_blank" rel="noopener">{env.health_url}</a></p>
          <p><strong>WordPress Version:</strong> {env.wp_version || '-'}</p>
          <p><strong>Database:</strong> {env.db_name || '-'}</p>
          <p><strong>Snapshot:</strong> {env.snapshot_id || '-'}</p>
          {stopping && <div className="result-box info"><span className="spinner" /> Stopping rehearsal...</div>}
          <div className="form-group" style={{ marginTop: 16 }}>
            <button onClick={handleStop} className="btn btn-danger" disabled={stopping}>Stop Rehearsal</button>
          </div>
        </div>
      )}
    </>
  )
}
