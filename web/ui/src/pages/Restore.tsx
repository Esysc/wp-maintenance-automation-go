import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import JobHistory from '../components/JobHistory'
import { apiGet, apiPost, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

interface Backup {
  id: string
  timestamp: string
  snapshot_id: string
  wp_version: string
  host: string
}

export default function Restore() {
  const [siteId, setSiteId] = useState('')
  const [backups, setBackups] = useState<Backup[]>([])
  const [snapshotId, setSnapshotId] = useState('')
  const [applyDb, setApplyDb] = useState(true)
  const [applyFiles, setApplyFiles] = useState(true)
  const { toast } = useToast()

  useEffect(() => {
    if (siteId) loadBackups()
    else { setBackups([]); setSnapshotId('') }
  }, [siteId])

  async function loadBackups() {
    setSnapshotId('')
    const res = await apiGet<Backup[]>('/api/v1/backups?site_id=' + siteId)
    if (res.success && res.data) {
      setBackups(Array.isArray(res.data) ? res.data : [])
    }
  }

  async function handleRestore(e: React.FormEvent) {
    e.preventDefault()
    if (!siteId || !snapshotId) { toast('Please select a snapshot', 'error'); return }
    const res = await apiPost<{ job_id: string }>('/api/v1/restore', {
      site_id: siteId,
      snapshot_id: snapshotId,
      apply_db: applyDb,
      apply_files: applyFiles,
      confirm: true,
    })
    if (res.success) {
      toast('Restore queued', 'info')
    } else {
      toast(res.error || 'Restore failed', 'error')
    }
  }

  return (
    <>
      <header className="page-header">
        <h1>Restore Backup</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {siteId && backups.length === 0 && (
        <div className="card"><p>No backups available for this site.</p></div>
      )}

      {siteId && backups.length > 0 && (
        <div className="card">
          <form onSubmit={handleRestore}>
            <div className="form-group">
              <label>Snapshot</label>
              <select value={snapshotId} onChange={e => setSnapshotId(e.target.value)} required>
                <option value="" disabled>Select a snapshot...</option>
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
                <span>Apply Database Restore</span>
              </label>
            </div>
            <div className="form-group">
              <label className="checkbox-label">
                <input type="checkbox" checked={applyFiles} onChange={e => setApplyFiles(e.target.checked)} />
                <span>Apply File Restore</span>
              </label>
            </div>
            <div className="form-group">
              <button type="submit" className="btn btn-warning">Restore</button>
            </div>
          </form>
        </div>
      )}

      <JobStatus jobType="restore" siteId={siteId} />
      <JobHistory jobType="restore" siteId={siteId} />
    </>
  )
}
