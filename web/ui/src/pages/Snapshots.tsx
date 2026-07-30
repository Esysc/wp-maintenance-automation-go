import { useEffect, useState } from 'react'
import { apiGet, type ApiResponse } from '../api/client'

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

  useEffect(() => { load() }, [])

  async function load() {
    setLoading(true)
    const res = await apiGet<Snapshot[]>('/api/v1/snapshots')
    if (res.success && res.data) {
      setSnapshots(Array.isArray(res.data) ? res.data : [])
    }
    setLoading(false)
  }

  return (
    <>
      <header className="page-header"><h1>Restic Snapshots</h1></header>
      <div className="card">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Time</th>
              <th>Host</th>
              <th>Tags</th>
              <th>Paths</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={5}>Loading...</td></tr>
            ) : snapshots.length === 0 ? (
              <tr><td colSpan={5}>No snapshots found.</td></tr>
            ) : snapshots.map(s => (
              <tr key={s.id || s.short_id}>
                <td><code>{(s.short_id || s.id || '-').substring(0, 16)}</code></td>
                <td>{s.time || '-'}</td>
                <td>{s.hostname || s.host || '-'}</td>
                <td>{(s.tags || []).join(', ') || '-'}</td>
                <td>{(s.paths || []).join(', ').substring(0, 60) || '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}
