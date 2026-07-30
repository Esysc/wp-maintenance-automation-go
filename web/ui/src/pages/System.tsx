import { useEffect, useState } from 'react'
import { apiGet, type ApiResponse } from '../api/client'

interface StatusData {
  server: string
  sites: number
  backups: number
  snapshots: number
  uptime: number
}

interface HealthData {
  status: string
  time: string
}

export default function System() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [health, setHealth] = useState<HealthData | null>(null)
  const [statusRaw, setStatusRaw] = useState('Loading...')
  const [healthRaw, setHealthRaw] = useState('Loading...')

  useEffect(() => { refresh() }, [])

  async function refresh() {
    const sr = await apiGet('/api/v1/status')
    setStatusRaw(JSON.stringify(sr, null, 2))
    if (sr.success && sr.data) setStatus(sr.data as StatusData)

    const hr = await apiGet('/api/v1/health')
    setHealthRaw(JSON.stringify(hr, null, 2))
    if (hr.success && hr.data) setHealth(hr.data as HealthData)
  }

  return (
    <>
      <header className="page-header"><h1>System Status</h1></header>

      <div className="stats-grid">
        <div className="stat-card">
          <h3>Server</h3>
          <p className="stat-value">{status?.server || '-'}</p>
        </div>
        <div className="stat-card">
          <h3>Sites</h3>
          <p className="stat-value">{status?.sites ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>Backups</h3>
          <p className="stat-value">{status?.backups ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>Snapshots</h3>
          <p className="stat-value">{status?.snapshots ?? '-'}</p>
        </div>
      </div>

      <div className="card">
        <h3>Status Overview</h3>
        <p id="statusSummary">
          {status
            ? `Server: ${status.server || 'unknown'} | Reported at: ${status.uptime ? new Date(status.uptime * 1000).toLocaleString() : 'N/A'}`
            : 'Loading system status...'}
        </p>
        <details>
          <summary>Raw status JSON</summary>
          <pre>{statusRaw}</pre>
        </details>
      </div>

      <div className="card">
        <h3>Health Overview</h3>
        <p>
          {health
            ? `Health: ${health.status || 'unknown'}${health.time ? ' | Reported at: ' + new Date(health.time).toLocaleString() : ''}`
            : 'Loading health status...'}
        </p>
        <details>
          <summary>Raw health JSON</summary>
          <pre>{healthRaw}</pre>
        </details>
      </div>

      <div className="card">
        <h3>Actions</h3>
        <button className="btn btn-primary" onClick={refresh}>Refresh</button>
      </div>

      <div className="card">
        <h3>Note</h3>
        <p>Site configuration is managed through the Sites page. <a href="/sites">Sites</a></p>
      </div>
    </>
  )
}
