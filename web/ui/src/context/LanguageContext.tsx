import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'
import {
  LANGUAGE_OPTIONS,
  getPreferredLanguage,
  getTranslation,
  loadLocale,
  setLanguage as setI18nLang,
  type Language,
} from '../i18n'

interface LanguageState {
  lang: Language
  setLang: (lang: Language) => void
  t: (key: string) => string
}

const LanguageContext = createContext<LanguageState | null>(null)

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Language>(getPreferredLanguage())

  useEffect(() => {
    loadLocale(lang).then(() => setI18nLang(lang))
  }, [lang])

  const setLang = (next: Language) => {
    setLangState(next)
    setI18nLang(next)
  }

  const t = (key: string) => getTranslation(key, lang)

  return (
    <LanguageContext.Provider value={{ lang, setLang, t }}>
      {children}
    </LanguageContext.Provider>
  )
}

export function useLanguage() {
  const ctx = useContext(LanguageContext)
  if (!ctx) throw new Error('useLanguage must be used within LanguageProvider')
  return ctx
}

export { LANGUAGE_OPTIONS }
