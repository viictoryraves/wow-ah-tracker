import { useI18n } from './i18nContext'
import type { Lang } from './i18n'

// Each language is named in itself, so it can be found from the wrong one. No flags: a language is not a country.
const LANGUAGES: [Lang, string][] = [
  ['en', 'English'],
  ['es', 'Español'],
]

/** A two-way toggle: both choices stay in view and the current one is marked. */
export function LanguageSwitch() {
  const { lang, setLang, t } = useI18n()
  return (
    <div role="group" aria-label={t('language')} className="lang-switch">
      {LANGUAGES.map(([value, name]) => (
        <button
          key={value}
          type="button"
          lang={value}
          aria-pressed={lang === value}
          onClick={() => lang !== value && setLang(value)}
        >
          {name}
        </button>
      ))}
    </div>
  )
}
