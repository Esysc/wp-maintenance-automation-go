function getToken(): string {
  const match = document.cookie.match(/(?:^| )token=([^;]+)/)
  return match ? match[1] : ''
}

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Authorization': 'Bearer ' + getToken(),
    ...(options.headers as Record<string, string> || {}),
  }
  if (options.body && typeof options.body === 'string') {
    headers['Content-Type'] = 'application/json'
  }
  const resp = await fetch(url, { ...options, headers })
  if (resp.redirected && resp.url.includes('/login')) {
    window.location.href = '/login'
    throw new Error('redirected to login')
  }
  if (resp.status === 401) {
    document.cookie = 'token=; Max-Age=0; path=/'
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  if (!resp.ok) {
    const body = await resp.text()
    let msg: string
    try {
      msg = JSON.parse(body).error || body
    } catch {
      msg = body
    }
    throw new Error(msg || `HTTP ${resp.status}`)
  }
  const text = await resp.text()
  if (!text) return {} as T
  return JSON.parse(text)
}

export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

export async function apiGet<T>(url: string): Promise<ApiResponse<T>> {
  return request<ApiResponse<T>>(url)
}

export async function apiPost<T>(url: string, body?: unknown): Promise<ApiResponse<T>> {
  return request<ApiResponse<T>>(url, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiPut<T>(url: string, body?: unknown): Promise<ApiResponse<T>> {
  return request<ApiResponse<T>>(url, {
    method: 'PUT',
    body: body ? JSON.stringify(body) : undefined,
  })
}

export async function apiDelete<T>(url: string): Promise<ApiResponse<T>> {
  return request<ApiResponse<T>>(url, { method: 'DELETE' })
}
