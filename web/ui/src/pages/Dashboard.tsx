import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiGet, apiPost, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

interface StatusData {
  backups: number
  snapshots: number
  sites: number
  server: string
  api: string
}

export default function Dashboard() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const { toast } = useToast()
  const navigate = useNavigate()

  useEffect(() => {
    loadStatus()
    const iv = setInterval(loadStatus, 30000)
    return () => clearInterval(iv)
  }, [])

  async function loadStatus() {
    const res = await apiGet<StatusData>('/api/v1/status')
    if (res.success && res.data) setStatus(res.data)
  }

  async function doBackup() {
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/backup')
      if (res.success) {
        toast('Backup started successfully', 'success')
        setTimeout(loadStatus, 2000)
      } else {
        toast(res.error || 'Backup failed', 'error')
      }
    } catch (e: any) {
      toast(e.message || 'Backup failed', 'error')
    }
  }

  return (
    <>
      <header className="page-header">
        <h1>Dashboard</h1>
        <button className="btn btn-primary" onClick={() => navigate('/sites')}>Manage Sites</button>
      </header>

      <div className="stats-grid">
        <div className="stat-card">
          <h3>Backups</h3>
          <p className="stat-value">{status?.backups ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>Restic Snapshots</h3>
          <p className="stat-value">{status?.snapshots ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>Server Status</h3>
          <p className="stat-value">{status?.server ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>API Status</h3>
          <p className="stat-value">{status?.api ?? 'online'}</p>
        </div>
      </div>

      <div className="action-grid">
        <div className="action-card" onClick={doBackup}>
          <h3>Run Backup</h3>
          <p>Create a new backup now</p>
        </div>
        <div className="action-card" onClick={() => navigate('/upgrade')}>
          <h3>Run Upgrade</h3>
          <p>Upgrade WordPress with rollback</p>
        </div>
        <div className="action-card" onClick={() => navigate('/healthcheck')}>
          <h3>Health Check</h3>
          <p>Verify site is responding correctly</p>
        </div>
        <div className="action-card" onClick={() => navigate('/snapshots')}>
          <h3>Snapshots</h3>
          <p>View restic snapshots</p>
        </div>
      </div>
    </>
  )
}
