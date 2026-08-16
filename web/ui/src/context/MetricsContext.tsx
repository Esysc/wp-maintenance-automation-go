import { createContext, useContext, useEffect, useRef, useState, useCallback, type ReactNode } from 'react'
import { apiGet } from '../api/client'
import { useAuth } from './AuthContext'

export interface MetricContainer {
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

export interface DockerHost {
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

export interface HostMetrics {
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

export interface MetricsData {
  host: HostMetrics
  containers: MetricContainer[]
  containers_available: boolean
  compose_managed: boolean
}

interface MetricsState {
  metrics: MetricsData | null
  metricsRaw: string
  loading: boolean
}

const POLL_INTERVAL = 10000

const MetricsContext = createContext<MetricsState | null>(null)

export function MetricsProvider({ children }: { children: ReactNode }) {
  const { authenticated } = useAuth()
  const [metrics, setMetrics] = useState<MetricsData | null>(null)
  const [metricsRaw, setMetricsRaw] = useState('')
  const [loading, setLoading] = useState(true)
  const inFlight = useRef(false)

  const load = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const mr = await apiGet<MetricsData>('/api/v1/metrics')
      setMetricsRaw(JSON.stringify(mr, null, 2))
      if (mr.success && mr.data) {
        setMetrics(mr.data)
      } else if (!mr.success) {
        setMetricsRaw(JSON.stringify({ success: false, error: mr.error || 'metrics unavailable' }, null, 2))
      }
    } catch (e: any) {
      setMetricsRaw(JSON.stringify({ success: false, error: e?.message || 'failed to fetch metrics' }, null, 2))
    } finally {
      inFlight.current = false
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!authenticated) return
    setLoading(true)
    load()
    const id = setInterval(load, POLL_INTERVAL)
    return () => clearInterval(id)
  }, [authenticated, load])

  return (
    <MetricsContext.Provider value={{ metrics, metricsRaw, loading }}>
      {children}
    </MetricsContext.Provider>
  )
}

export function useMetrics() {
  const ctx = useContext(MetricsContext)
  if (!ctx) throw new Error('useMetrics must be used within MetricsProvider')
  return ctx
}
