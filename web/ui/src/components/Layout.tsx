import { useEffect, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useLanguage, LANGUAGE_OPTIONS } from '../context/LanguageContext'
import { useState } from 'react'
import { ICONS } from './icons'

const NAV_ITEMS = [
  { to: '/dashboard', key: 'nav_dashboard', icon: 'dashboard' },
  { to: '/sites', key: 'nav_sites', icon: 'sites' },
  { to: '/backups', key: 'nav_backups', icon: 'backups' },
  { to: '/restore', key: 'nav_restore', icon: 'restore' },
  { to: '/upgrade', key: 'nav_upgrade', icon: 'upgrade' },
  { to: '/rehearsal', key: 'nav_rehearsal', icon: 'rehearsal' },
  { to: '/snapshots', key: 'nav_snapshots', icon: 'snapshots' },
  { to: '/healthcheck', key: 'nav_healthcheck', icon: 'healthcheck' },
  { to: '/users', key: 'nav_users', icon: 'users' },
  { to: '/tokens', key: 'nav_tokens', icon: 'tokens' },
  { to: '/system', key: 'nav_system', icon: 'system' },
]

export default function Layout({ children }: { children: ReactNode }) {
  const { logout } = useAuth()
  const { lang, setLang, t } = useLanguage()
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(false)

  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  useEffect(() => {
    document.body.classList.toggle('sidebar-open', sidebarOpen)
    return () => document.body.classList.remove('sidebar-open')
  }, [sidebarOpen])

  const currentPage = NAV_ITEMS.find(i => location.pathname.startsWith(i.to))
  const pageTitle = currentPage ? t(currentPage.key) : ''

  useEffect(() => {
    document.title = (pageTitle || t('nav_dashboard')) + ' — WP Maintenance Automation'
  }, [pageTitle, lang, t])

  const now = new Date()
  const dateLabel = now.toLocaleDateString(lang, { weekday: 'short', month: 'short', day: 'numeric' })

  return (
    <>
      <button type="button" className="sidebar-toggle" onClick={() => setSidebarOpen(!sidebarOpen)} aria-label={t('menu_toggle')}>
        <span /><span /><span />
      </button>
      {sidebarOpen && <button type="button" className="sidebar-backdrop" tabIndex={-1} onClick={() => setSidebarOpen(false)} aria-label={t('btn_close')} />}
      <nav className={`sidebar${sidebarOpen ? ' sidebar-open' : ''}`}>
        <div className="sidebar-header">
          <h2>WP Maintenance</h2>
          <select
            className="lang-select"
            value={lang}
            onChange={e => setLang(e.target.value as typeof lang)}
            aria-label={t('form_language')}
          >
            {LANGUAGE_OPTIONS.map(opt => (
              <option key={opt} value={opt}>{opt.toUpperCase()}</option>
            ))}
          </select>
        </div>
        <ul className="nav-list">
          {NAV_ITEMS.map(item => (
            <li key={item.to}>
              <NavLink to={item.to} end={item.to === '/dashboard'} onClick={() => setSidebarOpen(false)}
                className={({ isActive }) => isActive ? 'active' : ''}>
                <span className="nav-icon"><svg viewBox="0 0 24 24" aria-hidden="true" dangerouslySetInnerHTML={{ __html: ICONS[item.icon] }} /></span>
                {t(item.key)}
              </NavLink>
            </li>
          ))}
        </ul>
        <div className="sidebar-footer">
          <a href="/login" onClick={e => { e.preventDefault(); logout() }}>
            {t('nav_logout')}
          </a>
        </div>
      </nav>
      <main className="main-content">
        <div className="page-topbar">
          <div className="crumbs">
            <span className="crumb-chip">{t('header_control_panel')}</span>
            <span className="crumb-sep">/</span>
            <span className="crumb-current">{pageTitle || t('nav_dashboard')}</span>
          </div>
          <div className="topbar-meta">
            <span className="meta-pill">{t('header_live_api')}</span>
            <span className="meta-pill">{dateLabel}</span>
          </div>
        </div>
        {children}
      </main>
    </>
  )
}
