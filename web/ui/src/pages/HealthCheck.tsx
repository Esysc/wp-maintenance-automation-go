import { useState } from 'react'
import { apiPost } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'
import SiteSelector from '../components/SiteSelector'

interface HealthResult {
  status_code: number
  expected_code: number
  passed: boolean
  response_time_ms: number
}

export default function HealthCheck() {
  const [url, setUrl] = useState('')
  const [expectedCode, setExpectedCode] = useState(200)
  const [result, setResult] = useState<HealthResult | null>(null)
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(false)
  const { toast } = useToast()
  const { sites, siteId, setSiteId } = useSite()
  const { t } = useLanguage()

  function handleSiteChange(id: string) {
    setSiteId(id)
    const site = sites.find(s => s.id === id)
    if (site?.healthcheck_url) setUrl(site.healthcheck_url)
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setResult(null)
    setError('')
    setChecking(true)
    try {
      const res = await apiPost<HealthResult>('/api/v1/healthcheck', {
        url,
        expected_code: expectedCode,
      })
      if (res.success && res.data) {
        setResult(res.data)
        toast(res.data.passed ? t('health_check_passed') : t('health_check_failed'), res.data.passed ? 'success' : 'error')
      } else {
        setError(res.error || t('error_check_failed'))
        toast(res.error || t('error_check_failed'), 'error')
      }
    } catch (e: any) {
      setError(e.message || t('error_check_failed'))
      toast(e.message || t('error_check_failed'), 'error')
    }
    setChecking(false)
  }

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_healthcheck')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={handleSiteChange} label={t('label_select_site')} />
        </div>
      </header>
      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="hcUrl">{t('form_url_check')}</label>
            <input id="hcUrl" type="url" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://example.com" required />
          </div>
          <div className="form-group">
            <label htmlFor="hcCode">{t('form_expected_code')}</label>
            <input id="hcCode" type="number" value={expectedCode} onChange={e => setExpectedCode(parseInt(e.target.value) || 200)} />
          </div>
          <div className="form-group">
            <button type="submit" className="btn btn-primary" disabled={checking}>
              {checking && <span className="spinner" />}{t('btn_run_check')}
            </button>
          </div>
        </form>
      </div>

      {result && (
        <div className={`result-box ${result.passed ? 'success' : 'error'}`}>
          <div className="health-stats">
            <div>
              <span className="health-label">{t('health_label_status')}</span>
              <strong>{result.status_code}</strong>
            </div>
            <div>
              <span className="health-label">{t('health_label_expected')}</span>
              <strong>{result.expected_code}</strong>
            </div>
            <div>
              <span className="health-label">{t('health_label_response')}</span>
              <strong>{result.response_time_ms}ms</strong>
            </div>
            <div>
              <span className="health-label">{t('health_label_passed')}</span>
              <strong>{result.passed ? t('option_yes') : t('option_no')}</strong>
            </div>
          </div>
        </div>
      )}

      {error && <div className="result-box error" role="alert">{error}</div>}
    </>
  )
}
