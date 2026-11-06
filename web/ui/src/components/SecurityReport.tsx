import { useLanguage } from '../context/LanguageContext'

export interface SecurityFinding {
  id: string
  severity: string
  title: string
  description: string
  recommendation: string
  action?: string
  evidence?: string
  url?: string
  reference?: string
}

export interface SecurityReport {
  target: string
  scanned_at: string
  findings: SecurityFinding[]
  counts: Record<string, number>
}

export default function SecurityReportCard({ report, note }: { report?: SecurityReport; note?: string }) {
  const { t } = useLanguage()
  if (!report) return null

  const findings = Array.isArray(report.findings) ? report.findings : []
  const counts = report.counts || {}
  const summary = ['critical', 'high', 'medium', 'low', 'info']
    .filter(level => (counts[level] || 0) > 0)
    .map(level => `${counts[level]} ${t('severity_' + level)}`)
    .join(', ')

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h4 style={{ marginTop: 0 }}>{t('rehearsal_security_title')}</h4>
      <p className="help-text">{note || t('rehearsal_security_note')}</p>
      <p><strong>{t('rehearsal_security_summary')}:</strong> {findings.length === 0 ? t('rehearsal_security_none') : summary}</p>
      {findings.map((finding, i) => {
        const level = finding.severity === 'critical' || finding.severity === 'high'
          ? 'badge-error'
          : finding.severity === 'medium' ? 'badge-warning' : 'badge-muted'
        return (
          <details key={`${finding.id}-${i}`} style={{ marginBottom: 8 }}>
            <summary>
              <span className={`badge ${level}`} style={{ marginRight: 8 }}>{t('severity_' + finding.severity)}</span>
              {finding.title}
            </summary>
            <p style={{ marginTop: 8 }}>{finding.description}</p>
            <p><strong>{t('rehearsal_security_recommendation')}:</strong> {finding.recommendation}</p>
            {finding.action && (
              <div style={{ marginTop: 8 }}>
                <strong>{t('rehearsal_security_action')}:</strong>
                <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12, marginTop: 4 }}>{finding.action}</pre>
              </div>
            )}
            {finding.reference && (
              <p><a href={finding.reference} target="_blank" rel="noopener">{t('rehearsal_security_reference')}</a></p>
            )}
            {finding.evidence && <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>{finding.evidence}</pre>}
          </details>
        )
      })}
    </div>
  )
}
