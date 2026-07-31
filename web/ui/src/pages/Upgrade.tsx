import { useState } from 'react'
import SiteSelector from '../components/SiteSelector'
import JobStatus from '../components/JobStatus'
import JobHistory from '../components/JobHistory'
import ConfirmDialog from '../components/ConfirmDialog'
import SiteHint from '../components/SiteHint'
import { apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'

export default function Upgrade() {
  const { siteId, setSiteId } = useSite()
  const [autoRollback, setAutoRollback] = useState(true)
  const [healthcheckUrl, setHealthcheckUrl] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  function requestSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!siteId) { toast(t('error_site_id_required'), 'error'); return }
    setConfirm(true)
  }

  async function handleSubmit() {
    if (!siteId) return
    setSubmitting(true)
    try {
      const res = await apiPost<{ job_id: string }>('/api/v1/upgrade', {
        site_id: siteId,
        auto_rollback: autoRollback,
        healthcheck_url: healthcheckUrl,
      })
      if (res.success) {
        toast(t('upgrade_initiated'), 'info')
      } else {
        toast(res.error || t('upgrade_failed_start'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('upgrade_failed_start'), 'error')
    }
    setSubmitting(false)
    setConfirm(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_upgrade')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {!siteId && (
        <SiteHint />
      )}

      {siteId && (
        <div className="card">
          <p>{t('upgrade_pipeline_summary')}</p>
          <form onSubmit={requestSubmit}>
            <div className="form-group">
              <label className="checkbox-label">
                <input type="checkbox" checked={autoRollback} onChange={e => setAutoRollback(e.target.checked)} />
                <span>{t('form_auto_rollback')}</span>
              </label>
            </div>
            <div className="form-group">
              <label>{t('form_healthcheck_url_upgrade')}</label>
              <input type="url" value={healthcheckUrl} onChange={e => setHealthcheckUrl(e.target.value)} placeholder="https://example.com" />
            </div>
            <div className="form-group">
              <button type="submit" className="btn btn-primary">{t('btn_run_upgrade')}</button>
            </div>
          </form>
        </div>
      )}

      <JobStatus jobType="upgrade" siteId={siteId} />
      <JobHistory jobType="upgrade" siteId={siteId} />

      <ConfirmDialog
        open={confirm}
        title={t('modal_upgrade_title')}
        message={t('modal_upgrade_body')}
        onConfirm={handleSubmit}
        onCancel={() => setConfirm(false)}
        busy={submitting}
      />
    </>
  )
}
