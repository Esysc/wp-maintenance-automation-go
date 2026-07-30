import { useState } from 'react'
import { apiPost, type ApiResponse } from '../api/client'
import { useToast } from '../context/ToastContext'

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
  const { toast } = useToast()

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setResult(null)
    setError('')
    const res = await apiPost<HealthResult>('/api/v1/healthcheck', {
      url,
      expected_code: expectedCode,
    })
    if (res.success && res.data) {
      setResult(res.data)
      toast(res.data.passed ? 'Health check passed' : 'Health check failed', res.data.passed ? 'success' : 'error')
    } else {
      setError(res.error || 'Check failed')
      toast(res.error || 'Check failed', 'error')
    }
  }

  return (
    <>
      <header className="page-header"><h1>Health Check</h1></header>
      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>URL to check</label>
            <input type="url" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://example.com" required />
          </div>
          <div className="form-group">
            <label>Expected HTTP Status</label>
            <input type="number" value={expectedCode} onChange={e => setExpectedCode(parseInt(e.target.value) || 200)} />
          </div>
          <div className="form-group">
            <button type="submit" className="btn btn-primary">Run Check</button>
          </div>
        </form>
      </div>

      {result && (
        <div className={`result-box ${result.passed ? 'success' : 'error'}`}>
          Status: {result.status_code}, Expected: {result.expected_code},
          Passed: {result.passed ? 'Yes' : 'No'}, Response: {result.response_time_ms}ms
        </div>
      )}

      {error && <div className="result-box error">{error}</div>}
    </>
  )
}
