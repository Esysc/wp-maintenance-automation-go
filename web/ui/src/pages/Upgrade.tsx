import { useState } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import JobHistory from '../components/JobHistory'
import { apiPost, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

export default function Upgrade() {
  const [siteId, setSiteId] = useState('')
  const [autoRollback, setAutoRollback] = useState(true)
  const [healthcheckUrl, setHealthcheckUrl] = useState('')
  const { toast } = useToast()

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!siteId) { toast('Please select a site first', 'error'); return }
    const res = await apiPost<{ job_id: string }>('/api/v1/upgrade', {
      site_id: siteId,
      auto_rollback: autoRollback,
      healthcheck_url: healthcheckUrl,
    })
    if (res.success) {
      toast('Upgrade queued', 'info')
    } else {
      toast(res.error || 'Upgrade failed', 'error')
    }
  }

  return (
    <>
      <header className="page-header">
        <h1>WordPress Upgrade</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {siteId && (
        <div className="card">
          <p>This will run the full upgrade pipeline: backup &rarr; validate &rarr; upgrade &rarr; healthcheck &rarr; rollback on failure.</p>
          <form onSubmit={handleSubmit}>
            <div className="form-group">
              <label className="checkbox-label">
                <input type="checkbox" checked={autoRollback} onChange={e => setAutoRollback(e.target.checked)} />
                <span>Auto-Rollback on Failure</span>
              </label>
            </div>
            <div className="form-group">
              <label>Healthcheck URL (optional)</label>
              <input type="url" value={healthcheckUrl} onChange={e => setHealthcheckUrl(e.target.value)} placeholder="https://example.com" />
            </div>
            <div className="form-group">
              <button type="submit" className="btn btn-primary">Run Upgrade</button>
            </div>
          </form>
        </div>
      )}

      <JobStatus jobType="upgrade" siteId={siteId} />
      <JobHistory jobType="upgrade" siteId={siteId} />
    </>
  )
}
