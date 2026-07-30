import { useEffect, useState } from 'react'
import { apiGet, type ApiResponse } from '../api/client'

interface Site {
  id: string
  name: string
}

interface Props {
  value: string
  onChange: (id: string) => void
  label?: string
}

export default function SiteSelector({ value, onChange, label }: Props) {
  const [sites, setSites] = useState<Site[]>([])

  useEffect(() => {
    (async () => {
      const res = await apiGet<Site[]>('/api/v1/sites')
      if (res.success && res.data) {
        setSites(res.data)
      }
    })()
  }, [])

  return (
    <select
      className="site-selector"
      value={value}
      onChange={e => onChange(e.target.value)}
    >
      <option value="" disabled>{label || 'Select Site'}</option>
      {sites.map(s => (
        <option key={s.id} value={s.id}>{s.name || '-'}</option>
      ))}
    </select>
  )
}
