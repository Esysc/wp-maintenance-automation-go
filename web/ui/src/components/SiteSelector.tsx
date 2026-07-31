import { useSite } from '../context/SiteContext'
import { useLanguage } from '../context/LanguageContext'

interface Props {
  value: string
  onChange: (id: string) => void
  label?: string
}

export default function SiteSelector({ value, onChange, label }: Props) {
  const { sites } = useSite()
  const { t } = useLanguage()

  return (
    <select
      className="site-selector"
      value={value}
      onChange={e => onChange(e.target.value)}
      aria-label={label || t('label_select_site')}
    >
      {sites.length === 0
        ? <option value="" disabled>{t('hint_no_sites')}</option>
        : <option value="" disabled>{label || t('label_select_site')}</option>}
      {sites.map(s => (
        <option key={s.id} value={s.id}>{s.name || '-'}</option>
      ))}
    </select>
  )
}
