import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiGet } from '../api/client'
import { useLanguage } from '../context/LanguageContext'

interface StatusData {
  server: string
  sites: number
  backups: number
  snapshots: number
  uptime: number
}

interface MetricContainer {
  name: string
  image: string
  state: string
  status: string
  running_for: string
  ports: string
  cpu_percent?: number
  memory_used?: number
  memory_limit?: number
  memory_percent?: number
}

interface DockerHost {
  name: string
  operating_system: string
  os_type: string
  architecture: string
  kernel_version: string
  server_version: string
  docker_root_dir: string
  ncpu: number
  mem_total: number
}

interface HostMetrics {
  hostname: string
  platform: string
  os: string
  arch: string
  kernel: string
  uptime_seconds: number
  cpus: number
  load_avg: number[]
  cpu_percent: number
  memory_total: number
  memory_used: number
  memory_percent: number
  disk_total: number
  disk_used: number
  disk_percent: number
  available: boolean
  docker?: DockerHost
}

interface MetricsData {
  host: HostMetrics
  containers: MetricContainer[]
  containers_available: boolean
  compose_managed: boolean
}

function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

function formatUptime(seconds?: number): string {
  if (!seconds) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function stateBadge(state: string): string {
  switch ((state || '').toLowerCase()) {
    case 'running': return 'badge badge-success'
    case 'exited':
    case 'removed':
    case 'created': return 'badge badge-muted'
    case 'dead': return 'badge badge-error'
    default: return 'badge badge-warning'
  }
}

function Meter({ label, percent, detail }: { label: string; percent: number; detail: string }) {
  const pct = Math.max(0, Math.min(100, Math.round(percent || 0)))
  return (
    <div className="metric">
      <div className="metric-head">
        <span className="metric-label">{label}</span>
        <span className="metric-detail">{detail}</span>
      </div>
      <div className="meter-track">
        <div className="meter-fill" style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

export default function System() {
  const [status, setStatus] = useState<StatusData | null>(null)
  const [metrics, setMetrics] = useState<MetricsData | null>(null)
  const [metricsRaw, setMetricsRaw] = useState('')
  const inFlight = useRef(false)
  const { t } = useLanguage()

  useEffect(() => {
    loadStatus()
    loadMetrics()
    const id = setInterval(loadMetrics, 10000)
    return () => clearInterval(id)
  }, [])

  async function loadStatus() {
    const sr = await apiGet('/api/v1/status')
    if (sr.success && sr.data) setStatus(sr.data as StatusData)
  }

  async function loadMetrics() {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const mr = await apiGet<MetricsData>('/api/v1/metrics')
      setMetricsRaw(JSON.stringify(mr, null, 2))
      if (mr.success && mr.data) setMetrics(mr.data)
    } finally {
      inFlight.current = false
    }
  }

  const host = metrics?.host
  const hostDetails: Array<[string, string]> = [
    [t('metric_hostname'), host?.hostname || '-'],
    [t('metric_os'), host?.platform || host?.os || '-'],
    [t('metric_kernel'), host?.kernel || '-'],
    [t('metric_uptime'), formatUptime(host?.uptime_seconds)],
    [t('metric_load'), (host?.load_avg || []).length ? host!.load_avg.map(l => l.toFixed(2)).join(' / ') : '-'],
    [t('metric_cpus'), host?.cpus != null ? String(host.cpus) : '-'],
    [t('metric_docker'), host?.docker?.server_version ? `v${host.docker.server_version}` : '-'],
  ]

  return (
    <>
      <header className="page-header"><h1>{t('page_title_system')}</h1></header>

      <div className="stats-grid">
        <div className="stat-card">
          <h3>{t('label_server')}</h3>
          <p className="stat-value">{status?.server || '-'}</p>
        </div>
        <div className="stat-card">
          <h3>{t('status_sites')}</h3>
          <p className="stat-value">{status?.sites ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>{t('status_backups')}</h3>
          <p className="stat-value">{status?.backups ?? '-'}</p>
        </div>
        <div className="stat-card">
          <h3>{t('status_snapshots')}</h3>
          <p className="stat-value">{status?.snapshots ?? '-'}</p>
        </div>
      </div>

      <div className="card">
        <h3>{t('system_host_metrics')}</h3>
        {host?.available ? (
          <>
            <div className="metrics-grid">
              <Meter label={t('metric_cpu')} percent={host.cpu_percent} detail={`${Math.round(host.cpu_percent || 0)}%`} />
              <Meter label={t('metric_memory')} percent={host.memory_percent} detail={`${formatBytes(host.memory_used)} / ${formatBytes(host.memory_total)}`} />
              <Meter label={t('metric_disk')} percent={host.disk_percent} detail={`${formatBytes(host.disk_used)} / ${formatBytes(host.disk_total)}`} />
            </div>
            <div className="metrics-details">
              {hostDetails.map(([label, value]) => (
                <span key={label}>{label}: <strong>{value}</strong></span>
              ))}
            </div>
          </>
        ) : (
          <p>{t('host_metrics_unavailable')}</p>
        )}
      </div>

      <div className="card">
        <h3>{t('system_containers')}</h3>
        {!metrics ? (
          <p>{t('status_loading_system')}</p>
        ) : !metrics.containers_available ? (
          <p>{t('containers_unavailable')}</p>
        ) : !metrics.compose_managed ? (
          <p>{t('containers_no_compose')}</p>
        ) : metrics.containers.length === 0 ? (
          <p>{t('containers_none')}</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>{t('container_name')}</th>
                <th>{t('container_image')}</th>
                <th>{t('container_state')}</th>
                <th>{t('container_status')}</th>
                <th>{t('container_ports')}</th>
                <th>{t('container_cpu')}</th>
                <th>{t('container_memory')}</th>
              </tr>
            </thead>
            <tbody>
              {metrics.containers.map(c => (
                <tr key={c.name}>
                  <td><code>{c.name}</code></td>
                  <td>{c.image || '-'}</td>
                  <td><span className={stateBadge(c.state)}>{c.state || '-'}</span></td>
                  <td>{c.status || '-'}</td>
                  <td>{c.ports || '-'}</td>
                  <td>{c.state === 'running' && c.cpu_percent != null ? `${c.cpu_percent.toFixed(1)}%` : '-'}</td>
                  <td>{c.state === 'running' && c.memory_used ? `${formatBytes(c.memory_used)}${c.memory_limit ? ` / ${formatBytes(c.memory_limit)}` : ''}` : '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {metricsRaw && (
          <details>
            <summary>{t('status_raw_json')}</summary>
            <pre>{metricsRaw}</pre>
          </details>
        )}
      </div>

      <div className="card">
        <h3>{t('action_note')}</h3>
        <p>{t('action_site_config_managed')} <Link to="/sites">{t('nav_sites')}</Link></p>
      </div>
    </>
  )
}
