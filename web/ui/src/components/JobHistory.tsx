import { useEffect, useState } from 'react'
import { apiGet, apiDelete, apiPost, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'
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
  const { toast } = useToast()

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
    }
  }

  async function handleCancel(jobId: string) {
    await apiPost(`/api/v1/jobs/${jobId}`)
    toast('Job cancelled', 'success')
    load()
  }

  async function handleDelete(jobId: string) {
    await apiDelete(`/api/v1/jobs/${jobId}`)
    toast('Job deleted', 'success')
    load()
  }

  if (jobs.length === 0) return null

  return (
    <div className="card" style={{ marginTop: 20 }}>
      <h2>Job History</h2>
      <table>
        <thead>
          <tr>
            <th>Type</th>
            <th>Status</th>
            <th>Progress</th>
            <th>Error</th>
            <th>Updated</th>
            <th>Actions</th>
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
                <td>{job.type}{details ? <><br /><small>{details}</small></> : ''}</td>
                <td>{job.status}</td>
                <td>{job.progress || job.status}</td>
                <td>{(job.error || '').substring(0, 80)}</td>
                <td>{(job.updated_at || '').substring(0, 19).replace('T', ' ')}</td>
                <td>
                  {isActive
                    ? <button onClick={() => setConfirm({ id: job.id, action: 'cancel' })} className="btn btn-danger btn-sm">Stop</button>
                    : <button onClick={() => setConfirm({ id: job.id, action: 'delete' })} className="btn btn-danger btn-sm">Delete</button>
                  }
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <ConfirmDialog
        open={confirm !== null}
        title={confirm?.action === 'cancel' ? 'Stop Job' : 'Delete Job'}
        message={confirm?.action === 'cancel' ? 'Stop this job?' : 'Delete this job entry?'}
        onConfirm={async () => {
          if (confirm?.action === 'cancel') await handleCancel(confirm.id)
          else if (confirm?.action === 'delete') await handleDelete(confirm.id)
          setConfirm(null)
        }}
        onCancel={() => setConfirm(null)}
      />
    </div>
  )
}
