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
  const [version, setVersion] = useState(0)

  useEffect(() => {
    loadLocale(lang)
      .then(() => setI18nLang(lang))
      .then(() => setVersion(v => v + 1))
  }, [lang])

  const setLang = (next: Language) => {
    setI18nLang(next)
    setLangState(next)
    loadLocale(next).then(() => setVersion(v => v + 1))
  }

  const t = (key: string) => getTranslation(key, lang)

  void version

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
