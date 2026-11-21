import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { I18nContext, makeI18n } from './i18nContext'
import type { Lang } from './i18n'

export function I18nProvider({ lang, setLang, children }: { lang: Lang; setLang: (l: Lang) => void; children: ReactNode }) {
  const value = useMemo(() => makeI18n(lang, setLang), [lang, setLang])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}
