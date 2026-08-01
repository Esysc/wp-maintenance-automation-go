import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './context/AuthContext'
import { ToastProvider } from './context/ToastContext'
import { LanguageProvider, useLanguage } from './context/LanguageContext'
import { SiteProvider } from './context/SiteContext'
import { loadLocale, getPreferredLanguage } from './i18n'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Backups from './pages/Backups'
import Restore from './pages/Restore'
import Upgrade from './pages/Upgrade'
import Rehearsal from './pages/Rehearsal'
import Snapshots from './pages/Snapshots'
import Sites from './pages/Sites'
import Users from './pages/Users'
import Tokens from './pages/Tokens'
import System from './pages/System'
import LoadingState from './components/LoadingState'
import type { ReactNode } from 'react'

function AuthGuard({ children }: { children: ReactNode }) {
  const { authenticated, loading } = useAuth()
  const { t } = useLanguage()
  if (loading) {
    return (
      <div className="main-content" style={{ marginLeft: 0, width: '100%' }}>
        <LoadingState text={t('loading')} />
      </div>
    )
  }
  if (!authenticated) return <Navigate to="/login" replace />
  return <Layout>{children}</Layout>
}

function App() {
  const [localeReady, setLocaleReady] = useState(false)

  useEffect(() => {
    loadLocale(getPreferredLanguage()).then(() => setLocaleReady(true))
  }, [])

  if (!localeReady) {
    return (
      <div className="login-container">
        <LoadingState text="Loading..." />
      </div>
    )
  }

  return (
    <BrowserRouter>
      <LanguageProvider>
        <ToastProvider>
          <AuthProvider>
            <SiteProvider>
            <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/dashboard" element={<AuthGuard><Dashboard /></AuthGuard>} />
            <Route path="/backups" element={<AuthGuard><Backups /></AuthGuard>} />
            <Route path="/restore" element={<AuthGuard><Restore /></AuthGuard>} />
            <Route path="/upgrade" element={<AuthGuard><Upgrade /></AuthGuard>} />
            <Route path="/rehearsal" element={<AuthGuard><Rehearsal /></AuthGuard>} />
            <Route path="/snapshots" element={<AuthGuard><Snapshots /></AuthGuard>} />
            <Route path="/sites" element={<AuthGuard><Sites /></AuthGuard>} />
            <Route path="/users" element={<AuthGuard><Users /></AuthGuard>} />
            <Route path="/tokens" element={<AuthGuard><Tokens /></AuthGuard>} />
            <Route path="/system" element={<AuthGuard><System /></AuthGuard>} />
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
            </SiteProvider>
          </AuthProvider>
        </ToastProvider>
      </LanguageProvider>
    </BrowserRouter>
  )
}

export default App
