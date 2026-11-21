import { detectLang, messages, translate } from './i18n'

const placeholders = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort()

describe('messages', () => {
  it('has a Spanish text for every English one, with the same placeholders', () => {
    for (const [key, english] of Object.entries(messages.en)) {
      const spanish = (messages.es as Record<string, string>)[key]
      expect(spanish, key).toBeTruthy()
      expect(placeholders(spanish), key).toEqual(placeholders(english))
    }
  })

  it('has no Spanish text without an English one', () => {
    expect(Object.keys(messages.es).sort()).toEqual(Object.keys(messages.en).sort())
  })
})

describe('translate', () => {
  it('fills the placeholders', () => {
    expect(translate('en', 'mostListedIn', { name: 'Armor' })).toBe('Most listed in Armor')
    expect(translate('es', 'mostListedIn', { name: 'Armadura' })).toBe('Más listados en Armadura')
  })
})

describe('detectLang', () => {
  it('picks Spanish for any Spanish browser and English otherwise', () => {
    expect(detectLang(['es-ES', 'en'])).toBe('es')
    expect(detectLang(['es'])).toBe('es')
    expect(detectLang(['en-GB'])).toBe('en')
    expect(detectLang(['fr-FR', 'es-ES'])).toBe('en') // the first language is the one that counts
    expect(detectLang([])).toBe('en')
  })
})
