import { useEffect, useState } from 'react'
import { apiGet } from '../api/client'
import { useToast } from '../context/ToastContext'
import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'
import SiteSelector from '../components/SiteSelector'
import LoadingState from '../components/LoadingState'

interface Snapshot {
  id: string
  short_id: string
  time: string
  hostname: string
  host: string
  tags: string[]
  paths: string[]
}

export default function Snapshots() {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const { siteId, setSiteId } = useSite()
  const { toast } = useToast()
  const { t } = useLanguage()

  useEffect(() => { load() }, [])

  async function load() {
    setLoading(true)
    const res = await apiGet<Snapshot[]>('/api/v1/snapshots')
    if (res.success && res.data) {
      setSnapshots(Array.isArray(res.data) ? res.data : [])
    } else if (res.error) {
      toast(res.error, 'error')
    }
    setLoading(false)
  }

  async function refresh() {
    setRefreshing(true)
    await load()
    setRefreshing(false)
  }

  const filtered = siteId
    ? snapshots.filter(s => (s.tags || []).includes(siteId))
    : snapshots

  return (
    <>
      <header className="page-header">
        <h1>{t('page_title_snapshots')}</h1>
        <div className="header-actions">
          <SiteSelector value={siteId} onChange={setSiteId} label={t('snapshot_site_filter')} />
          <button className="btn btn-secondary" onClick={refresh} disabled={refreshing}>
            {refreshing && <span className="spinner" />}{t('btn_refresh')}
          </button>
        </div>
      </header>
      <div className="card">
        {loading ? (
          <LoadingState text={t('loading')} />
        ) : filtered.length === 0 ? (
          <p>{t('error_no_snapshots')}</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>{t('table_id')}</th>
                <th>{t('table_time')}</th>
                <th>{t('table_host')}</th>
                <th>{t('table_tags')}</th>
                <th>{t('table_paths')}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(s => (
                <tr key={s.id || s.short_id}>
                  <td><code>{(s.short_id || s.id || '-').substring(0, 16)}</code></td>
                  <td>{s.time || '-'}</td>
                  <td>{s.hostname || s.host || '-'}</td>
                  <td>{(s.tags || []).join(', ') || '-'}</td>
                  <td title={(s.paths || []).join(', ') || undefined}>{(s.paths || []).join(', ').substring(0, 60) || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  )
}
