import { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from 'react'
import { apiGet } from '../api/client'

export interface SiteSummary {
  id: string
  name: string
  healthcheck_url?: string
}

interface SiteState {
  sites: SiteSummary[]
  siteId: string
  setSiteId: (id: string) => void
  loading: boolean
  reload: () => Promise<void>
}

const SiteContext = createContext<SiteState | null>(null)

export function SiteProvider({ children }: { children: ReactNode }) {
  const [sites, setSites] = useState<SiteSummary[]>([])
  const [siteId, setSiteId] = useState('')
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    const res = await apiGet<SiteSummary[]>('/api/v1/sites')
    if (res.success && res.data) {
      const list = Array.isArray(res.data) ? res.data : []
      setSites(list)
      setSiteId(prev => (list.some(s => s.id === prev) ? prev : ''))
    }
    setLoading(false)
  }, [])

  useEffect(() => { reload() }, [reload])

  return (
    <SiteContext.Provider value={{ sites, siteId, setSiteId, loading, reload }}>
      {children}
    </SiteContext.Provider>
  )
}

export function useSite() {
  const ctx = useContext(SiteContext)
  if (!ctx) throw new Error('useSite must be used within SiteProvider')
  return ctx
}
