import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { apiGet, apiPost, checkAuth, type ApiResponse } from '../api/client'

interface AuthState {
  authenticated: boolean
  setupRequired: boolean
  loading: boolean
  login: (password: string, passwordConfirm?: string) => Promise<string | null>
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [authenticated, setAuthenticated] = useState(false)
  const [setupRequired, setSetupRequired] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    (async () => {
      try {
        const res = await apiGet<{ setup_required?: boolean }>('/api/v1/auth/state')
        if (res.success && res.data) {
          setSetupRequired(!!res.data.setup_required)
        }
        setAuthenticated(await checkAuth())
      } catch {
        setAuthenticated(false)
      }
      setLoading(false)
    })()
  }, [])

  const login = useCallback(async (password: string, passwordConfirm?: string): Promise<string | null> => {
    const body: Record<string, string> = { password }
    if (passwordConfirm) body.passwordConfirm = passwordConfirm
    const res = await apiPost<{ token?: string }>('/api/login', body)
    if (res.success && res.data?.token) {
      document.cookie = 'token=' + res.data.token + '; path=/; max-age=86400'
      setAuthenticated(true)
      setSetupRequired(false)
      return null
    }
    return res.error || 'Login failed'
  }, [])

  const logout = useCallback(() => {
    document.cookie = 'token=; Max-Age=0; path=/'
    setAuthenticated(false)
  }, [])

  return (
    <AuthContext.Provider value={{ authenticated, setupRequired, loading, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
