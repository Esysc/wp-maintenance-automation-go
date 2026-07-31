const LANGUAGE_STORAGE_KEY = 'wpma-language'

const SUPPORTED = ['en', 'fr', 'it', 'es', 'pt', 'zh', 'ja', 'ko', 'ru'] as const
export type Language = typeof SUPPORTED[number]

const cache: Record<string, Record<string, string>> = {}

function normalize(lang: string): Language {
  const norm = lang.split('-')[0].toLowerCase()
  return SUPPORTED.includes(norm as Language) ? norm as Language : 'en'
}

export function getPreferredLanguage(): Language {
  try {
    const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY)
    if (stored) return normalize(stored)
  } catch { /* ignore */ }
  return normalize(navigator.language)
}

export async function loadLocale(language: Language): Promise<Record<string, string>> {
  const norm = normalize(language)
  if (cache[norm]) return cache[norm]
  try {
    const resp = await fetch('/static/locales/' + norm + '.json')
    if (!resp.ok) throw new Error('failed to load locale')
    const data = await resp.json()
    cache[norm] = data
    return data
  } catch {
    if (norm !== 'en') return loadLocale('en')
    cache[norm] = {}
    return {}
  }
}

export function getTranslation(key: string, lang?: Language): string {
  const l = lang || getPreferredLanguage()
  if (cache[l]?.[key]) return cache[l][key]
  if (cache.en?.[key]) return cache.en[key]
  return key
}

export function setLanguage(lang: Language) {
  try {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, lang)
  } catch { /* ignore */ }
  document.documentElement.lang = lang
}

export function jobTypeLabel(t: (k: string) => string, type: string): string {
  const key = 'job_type_' + type
  const label = t(key)
  return label === key ? type : label
}

export function jobStatusLabel(t: (k: string) => string, status: string): string {
  const key = 'job_status_' + status
  const label = t(key)
  return label === key ? status : label
}

export { SUPPORTED as LANGUAGE_OPTIONS }
export type { Language as LanguageCode }
