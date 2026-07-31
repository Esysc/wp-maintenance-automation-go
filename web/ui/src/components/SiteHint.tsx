import { useLanguage } from '../context/LanguageContext'

export default function SiteHint() {
  const { t } = useLanguage()
  return (
    <div className="card">
      <p>{t('hint_select_site')}</p>
    </div>
  )
}
