import { useState } from 'react'
import SiteSelector from '../components/SiteSelector'
import SiteHint from '../components/SiteHint'
import ConfirmDialog from '../components/ConfirmDialog'
import LoadingState from '../components/LoadingState'
import SecurityReportCard, { type SecurityReport } from '../components/SecurityReport'
import { apiPost } from '../api/client'
import { useSite } from '../context/SiteContext'
import { useToast } from '../context/ToastContext'
import { useLanguage } from '../context/LanguageContext'

export default function Security() {
  const { siteId, setSiteId } = useSite()
  const [report, setReport] = useState<SecurityReport | null>(null)
  const [scanning, setScanning] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const { toast } = useToast()
  const { t } = useLanguage()

  async function runScan() {
    if (!siteId) {
      toast(t('error_site_id_required'), 'error')
      return
    }
    setConfirm(false)
    setScanning(true)
    setReport(null)
    try {
      const res = await apiPost<SecurityReport>('/api/v1/security/scan', { site_id: siteId })
      if (res.success && res.data) {
        setReport(res.data)
        toast(t('security_scan_done'), 'success')
      } else {
        toast(res.error || t('security_scan_failed'), 'error')
      }
    } catch (e: any) {
      toast(e.message || t('security_scan_failed'), 'error')
    }
    setScanning(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_security')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} />
        </div>
      </header>

      {!siteId && <SiteHint />}

      {siteId && (
        <div className="card">
          <h3>{t('security_scan_title')}</h3>
          <p>{t('security_scan_desc')}</p>
          <div className="form-group">
            <button className="btn btn-primary" onClick={() => setConfirm(true)} disabled={scanning}>
              {scanning && <span className="spinner" />}{t('security_scan_btn')}
            </button>
          </div>
        </div>
      )}

      {scanning && (
        <div style={{ marginTop: 16 }}>
          <LoadingState text={t('security_scan_running')} />
        </div>
      )}

      {report && <SecurityReportCard report={report} note={t('security_scan_note')} />}

      <ConfirmDialog
        open={confirm}
        title={t('security_scan_confirm_title')}
        message={t('security_scan_confirm_body')}
        onConfirm={runScan}
        onCancel={() => setConfirm(false)}
        busy={scanning}
      />
    </>
  )
}
