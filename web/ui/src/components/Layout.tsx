import { useEffect, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useToast } from '../context/ToastContext'
import { LANGUAGE_OPTIONS, getPreferredLanguage, getTranslation, setLanguage as setI18nLang, type Language } from '../i18n'
import { useState } from 'react'

const NAV_ITEMS = [
  { to: '/dashboard', key: 'nav_dashboard' },
  { to: '/sites', key: 'nav_sites' },
  { to: '/backups', key: 'nav_backups' },
  { to: '/restore', key: 'nav_restore' },
  { to: '/upgrade', key: 'nav_upgrade' },
  { to: '/rehearsal', key: 'nav_rehearsal' },
  { to: '/snapshots', key: 'nav_snapshots' },
  { to: '/healthcheck', key: 'nav_healthcheck' },
  { to: '/users', key: 'nav_users' },
  { to: '/tokens', key: 'nav_tokens' },
  { to: '/system', key: 'nav_system' },
]

export default function Layout({ children }: { children: ReactNode }) {
  const { logout } = useAuth()
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [lang, setLang] = useState<Language>(getPreferredLanguage())

  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  const currentPage = NAV_ITEMS.find(i => location.pathname.startsWith(i.to))
  const pageTitle = currentPage ? getTranslation(currentPage.key, lang) : ''

  const now = new Date()
  const dateLabel = now.toLocaleDateString(lang, { weekday: 'short', month: 'short', day: 'numeric' })

  return (
    <>
      <button type="button" className="sidebar-toggle" onClick={() => setSidebarOpen(!sidebarOpen)} aria-label="Toggle menu">
        <span /><span /><span />
      </button>
      {sidebarOpen && <button type="button" className="sidebar-backdrop" onClick={() => setSidebarOpen(false)} />}
      <nav className={`sidebar${sidebarOpen ? ' sidebar-open' : ''}`}>
        <div className="sidebar-header"><h2>WP Maintenance</h2></div>
        <ul className="nav-list">
          {NAV_ITEMS.map(item => (
            <li key={item.to}>
              <NavLink to={item.to} end={item.to === '/dashboard'} onClick={() => setSidebarOpen(false)}
                className={({ isActive }) => isActive ? 'active' : ''}>
                {getTranslation(item.key, lang)}
              </NavLink>
            </li>
          ))}
        </ul>
        <div className="sidebar-footer">
          <a href="/login" onClick={e => { e.preventDefault(); logout() }}>
            {getTranslation('nav_logout', lang)}
          </a>
        </div>
      </nav>
      <main className="main-content">
        <div className="page-topbar">
          <div className="crumbs">
            <span className="crumb-chip">{getTranslation('header_control_panel', lang)}</span>
            <span className="crumb-sep">/</span>
            <span className="crumb-current">{pageTitle || getTranslation('nav_dashboard', lang)}</span>
          </div>
          <div className="topbar-meta">
            <span className="meta-pill">{getTranslation('header_live_api', lang)}</span>
            <span className="meta-pill">{dateLabel}</span>
          </div>
        </div>
        {children}
      </main>
    </>
  )
}
