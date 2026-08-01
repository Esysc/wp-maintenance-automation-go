import { useEffect, useState } from 'react'
import { apiGet, apiDelete, apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useLanguage } from '../context/LanguageContext'
import { jobTypeLabel, jobStatusLabel, progressLabel } from '../i18n'
import ConfirmDialog from './ConfirmDialog'

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
}

export default function JobHistory({ jobType, siteId }: Props) {
  const [jobs, setJobs] = useState<Job[]>([])
  const [confirm, setConfirm] = useState<{ id: string; action: 'cancel' | 'delete' } | null>(null)
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => {
    if (!siteId) { setJobs([]); return }
    load()
  }, [siteId, jobType])

  async function load() {
    let url = `/api/v1/jobs?site_id=${siteId}&all=1`
    if (jobType) url += `&type=${jobType}`
    const res = await apiGet<Job[]>(url)
    if (res.success && res.data) {
      setJobs(Array.isArray(res.data) ? res.data : [res.data])
    } else if (res.error) {
      toast(res.error, 'error')
    }
  }

  async function handleCancel(jobId: string) {
    setBusy(true)
    try {
      await apiPost(`/api/v1/jobs/${jobId}`)
      toast(t('job_cancelled'), 'success')
    } catch (e: any) {
      toast(e.message || t('job_cancelled'), 'error')
    }
    setBusy(false)
    load()
  }

  async function handleDelete(jobId: string) {
    setBusy(true)
    try {
      await apiDelete(`/api/v1/jobs/${jobId}`)
      toast(t('job_deleted'), 'success')
    } catch (e: any) {
      toast(e.message || t('job_deleted'), 'error')
    }
    setBusy(false)
    load()
  }

  if (jobs.length === 0) return null

  return (
    <div className="card" style={{ marginTop: 20 }}>
      <h2>{t('job_history_title')}</h2>
      <table>
        <thead>
          <tr>
            <th>{t('table_type')}</th>
            <th>{t('table_status')}</th>
            <th>{t('table_progress')}</th>
            <th>{t('table_error')}</th>
            <th>{t('table_updated')}</th>
            <th>{t('table_actions')}</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map(job => {
            const isActive = job.status === 'running' || job.status === 'queued'
            let details = ''
            if (job.type === 'restore' && job.result) {
              try {
                const r = JSON.parse(job.result)
                const parts: string[] = []
                if (r.apply_db) parts.push('DB')
                if (r.apply_files) parts.push('Files')
                if (parts.length) details = parts.join('+')
              } catch { /* ignore */ }
            }
            return (
              <tr key={job.id}>
                <td>{jobTypeLabel(t, job.type)}{details ? <><br /><small>{details}</small></> : ''}</td>
                <td>{jobStatusLabel(t, job.status)}</td>
                <td>{progressLabel(t, job.progress) || jobStatusLabel(t, job.status)}</td>
                <td title={job.error || undefined}>{(job.error || '').substring(0, 80)}</td>
                <td>{(job.updated_at || '').substring(0, 19).replace('T', ' ')}</td>
                <td>
                  {isActive
                    ? <button onClick={() => setConfirm({ id: job.id, action: 'cancel' })} className="btn btn-danger btn-sm">{t('btn_stop')}</button>
                    : <button onClick={() => setConfirm({ id: job.id, action: 'delete' })} className="btn btn-danger btn-sm">{t('btn_delete')}</button>
                  }
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <ConfirmDialog
        open={confirm !== null}
        title={confirm?.action === 'cancel' ? t('modal_stop_job') : t('modal_delete_job')}
        message={confirm?.action === 'cancel' ? t('modal_job_stop') : t('modal_job_delete')}
        onConfirm={async () => {
          if (confirm?.action === 'cancel') await handleCancel(confirm.id)
          else if (confirm?.action === 'delete') await handleDelete(confirm.id)
          setConfirm(null)
        }}
        onCancel={() => setConfirm(null)}
        busy={busy}
      />
    </div>
  )
}
