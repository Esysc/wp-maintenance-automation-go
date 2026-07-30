import { useState, useEffect } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import JobHistory from '../components/JobHistory'
import ConfirmDialog from '../components/ConfirmDialog'
import { apiGet, apiPost, apiDelete, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

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
  const [siteId, setSiteId] = useState('')
  const [backups, setBackups] = useState<Backup[]>([])
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const { toast } = useToast()

  useEffect(() => {
    if (siteId) loadBackups()
    else setBackups([])
  }, [siteId])

  async function loadBackups() {
    const res = await apiGet<Backup[]>('/api/v1/backups?site_id=' + siteId)
    if (res.success && res.data) {
      setBackups(Array.isArray(res.data) ? res.data : [])
    }
  }

  async function runBackup() {
    if (!siteId) { toast('Please select a site first', 'error'); return }
    const res = await apiPost<{ job_id: string }>('/api/v1/backup', { site_id: siteId })
    if (res.success) {
      toast('Backup queued', 'info')
    } else {
      toast(res.error || 'Backup failed', 'error')
    }
  }

  async function deleteBackup(id: string) {
    await apiDelete('/api/v1/backups/' + id + '?site_id=' + siteId)
    toast('Backup deleted', 'success')
    loadBackups()
  }

  return (
    <>
      <header className="page-header">
        <h1>Backups</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
          <button className="btn btn-primary" onClick={runBackup}>Run Backup Now</button>
        </div>
      </header>

      {siteId && (
        <div className="card">
          <table>
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Host</th>
                <th>WordPress Version</th>
                <th>Database</th>
                <th>Files</th>
                <th>Actions</th>
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
                    <button onClick={() => setDeleteTarget(b.id)} className="btn btn-danger btn-sm">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <JobStatus jobType="backup" siteId={siteId} />
      <JobHistory jobType="backup" siteId={siteId} />

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Backup"
        message="Delete this backup?"
        onConfirm={() => { if (deleteTarget) deleteBackup(deleteTarget); setDeleteTarget(null) }}
        onCancel={() => setDeleteTarget(null)}
      />
    </>
  )
}
