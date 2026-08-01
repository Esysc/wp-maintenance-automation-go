import { useEffect, useState, useRef } from 'react'
import { apiGet, apiPost } from '../api/client'
import { useLanguage } from '../context/LanguageContext'
import { jobTypeLabel, jobStatusLabel, progressLabel } from '../i18n'

interface Job {
  id: string
  type: string
  site_id: string
  status: string
  progress: string
  progress_percent: number
  result: string
  error: string
  updated_at: string
}

interface Props {
  jobType?: string
  siteId: string
  onReady?: (jobId: string) => void
}

export default function JobStatus({ jobType, siteId, onReady }: Props) {
  const [job, setJob] = useState<Job | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const intervalRef = useRef<number | undefined>(undefined)
  const { t } = useLanguage()

  useEffect(() => {
    if (!siteId) { setJob(null); return }
    loadLatest()
    return () => { if (intervalRef.current) clearInterval(intervalRef.current) }
  }, [siteId, jobType])

  async function loadLatest() {
    let url = `/api/v1/jobs?site_id=${siteId}`
    if (jobType) url += `&type=${jobType}`
    const res = await apiGet<Job>(url)
    if (res.success && res.data) {
      const j = res.data
      if (j.status === 'running' || j.status === 'queued') {
        setJob(j)
        poll(j.id)
      } else {
        setJob(null)
      }
    }
  }

  async function poll(jobId: string) {
    if (intervalRef.current) clearInterval(intervalRef.current)
    intervalRef.current = window.setInterval(async () => {
      const res = await apiGet<Job>(`/api/v1/jobs/${jobId}`)
      if (res.success && res.data) {
        const j = res.data
        if (j.status === 'completed' || j.status === 'failed' || j.status === 'cancelled') {
          setJob(null)
          if (intervalRef.current) clearInterval(intervalRef.current)
          if (j.status === 'completed' && onReady) onReady(jobId)
          return
        }
        setJob(j)
      }
    }, 2000)
  }

  async function cancel() {
    if (!job) return
    setCancelling(true)
    try {
      await apiPost(`/api/v1/jobs/${job.id}`)
    } catch { /* ignore */ }
    setJob(null)
    if (intervalRef.current) clearInterval(intervalRef.current)
    setCancelling(false)
  }

  if (!job) return null

  const pct = Math.max(0, Math.min(100, job.progress_percent))
  const isActive = job.status === 'running' || job.status === 'queued'

  return (
    <div className={`result-box ${job.status === 'completed' ? 'success' : job.status === 'failed' ? 'error' : 'info'}`}>
      <div className="job-progress-header">
        <span>{jobTypeLabel(t, job.type)} &bull; {jobStatusLabel(t, job.status)}</span>
        <span>{pct}%</span>
      </div>
      <div className="job-progress-detail">{progressLabel(t, job.progress) || jobStatusLabel(t, job.status)}</div>
      <div className="job-progress-track">
        <div className="job-progress-fill" style={{ width: pct + '%' }} />
      </div>
      {isActive && (
        <div style={{ marginTop: 8 }}>
          <button onClick={cancel} className="btn btn-danger btn-sm" disabled={cancelling}>
            {cancelling && <span className="spinner" />}{t('btn_stop')}
          </button>
        </div>
      )}
    </div>
  )
}
