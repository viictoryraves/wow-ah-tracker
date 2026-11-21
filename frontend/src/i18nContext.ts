import { createContext, createElement, Fragment, useContext } from 'react'
import type { ReactNode } from 'react'
import { messages, translate } from './i18n'
import type { Key, Lang, Vars } from './i18n'

export interface I18n {
  lang: Lang
  setLang: (lang: Lang) => void
  t: (key: Key, vars?: Vars) => string
  /** Like t, but the placeholders are filled with elements (a price, say). */
  tn: (key: Key, nodes: Record<string, ReactNode>) => ReactNode
}

export function makeI18n(lang: Lang, setLang: (lang: Lang) => void): I18n {
  return {
    lang,
    setLang,
    t: (key, vars) => translate(lang, key, vars),
    tn: (key, nodes) =>
      messages[lang][key].split(/(\{\w+\})/).map((part, i) => {
        const name = /^\{(\w+)\}$/.exec(part)?.[1]
        return createElement(Fragment, { key: i }, name && name in nodes ? nodes[name] : part)
      }),
  }
}

// Without a provider (a component on its own) everything is English.
export const I18nContext = createContext<I18n>(makeI18n('en', () => {}))

export const useI18n = () => useContext(I18nContext)
