import { formatAgo, formatClock, formatDecimal, formatMoney, formatPct, formatPercent, formatSold, moneyParts } from './format'

describe('formatMoney', () => {
  it('splits copper into gold, silver and copper', () => {
    expect(formatMoney(123456)).toBe('12g 34s 56c')
  })

  it('omits zero parts but keeps the ones that matter', () => {
    expect(formatMoney(10000)).toBe('1g')
    expect(formatMoney(10005)).toBe('1g 5c')
    expect(formatMoney(250)).toBe('2s 50c')
    expect(formatMoney(7)).toBe('7c')
  })

  it('shows zero as 0c', () => {
    expect(formatMoney(0)).toBe('0c')
  })

  it('separates thousands of gold', () => {
    expect(formatMoney(1234567800)).toBe('123,456g 78s')
  })
})

describe('formatPct', () => {
  it('always shows the sign of a change, with one decimal', () => {
    expect(formatPct(12.34)).toBe('+12.3%')
    expect(formatPct(-50)).toBe('-50.0%')
    expect(formatPct(0)).toBe('0.0%')
  })

  it('shows n/a when there is no value', () => {
    expect(formatPct(null)).toBe('n/a')
  })
})

describe('formatAgo', () => {
  const min = 60_000
  it('says "just now" within the first minute, and for a clock that is slightly behind', () => {
    expect(formatAgo(0)).toBe('just now')
    expect(formatAgo(59_000)).toBe('just now')
    expect(formatAgo(-5_000)).toBe('just now')
  })

  it('counts minutes up to an hour', () => {
    expect(formatAgo(min)).toBe('1 min ago')
    expect(formatAgo(5 * min + 20_000)).toBe('5 min ago')
    expect(formatAgo(59 * min)).toBe('59 min ago')
  })

  it('counts whole hours and then days', () => {
    expect(formatAgo(60 * min)).toBe('1 h ago')
    expect(formatAgo(150 * min)).toBe('2 h ago')
    expect(formatAgo(24 * 60 * min)).toBe('1 d ago')
    expect(formatAgo(3 * 24 * 60 * min + 5 * min)).toBe('3 d ago')
  })
})

describe('formatClock', () => {
  it('shows hours and minutes in the viewer\'s own time zone', () => {
    const iso = '2026-11-21T10:22:56Z'
    const d = new Date(iso)
    expect(formatClock(iso)).toBe(d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }))
    expect(formatClock(iso)).toMatch(/\d{1,2}[:.]\d{2}/)
  })
})

describe('formatSold', () => {
  it('shows a range, or one number when the estimates agree', () => {
    expect(formatSold(12, 18)).toBe('12–18 sold')
    expect(formatSold(7, 7)).toBe('7 sold')
    expect(formatSold(0, 0)).toBe('0 sold')
  })
  it('groups thousands', () => {
    expect(formatSold(1200, 3400)).toBe('1,200–3,400 sold')
  })
})

describe('in Spanish', () => {
  it('uses the game\'s coin letters: oro, plata and cobre', () => {
    expect(formatMoney(123456, 'es')).toBe('12o 34p 56c')
    expect(formatMoney(7, 'es')).toBe('7c')
  })
  it('writes percentages the Spanish way', () => {
    expect(formatPct(12.34, 'es')).toBe('+12,3 %')
    expect(formatPct(-2, 'es')).toBe('-2,0 %')
    expect(formatPct(null, 'es')).toBe('n/d')
  })
  it('says how long ago', () => {
    expect(formatAgo(10_000, 'es')).toBe('ahora mismo')
    expect(formatAgo(5 * 60_000, 'es')).toBe('hace 5 min')
    expect(formatAgo(2 * 3_600_000, 'es')).toBe('hace 2 h')
    expect(formatAgo(3 * 86_400_000, 'es')).toBe('hace 3 d')
  })
  it('counts units sold', () => {
    expect(formatSold(12, 18, 'es')).toBe('12–18 vendidas')
    expect(formatSold(7, 7, 'es')).toBe('7 vendidas')
  })
})

describe('moneyParts', () => {
  it('splits copper at 100 per silver and 100 per gold', () => {
    expect(moneyParts(123456)).toEqual({ negative: false, gold: 12, silver: 34, copper: 56 })
    expect(moneyParts(10000)).toEqual({ negative: false, gold: 1, silver: 0, copper: 0 })
    expect(moneyParts(99)).toEqual({ negative: false, gold: 0, silver: 0, copper: 99 })
    expect(moneyParts(100)).toEqual({ negative: false, gold: 0, silver: 1, copper: 0 })
  })
  it('never lets silver or copper reach 100', () => {
    for (const c of [9999, 10001, 10099, 10100, 999999, 2147483647, 99999999999]) {
      const p = moneyParts(c)!
      expect(p.silver).toBeLessThan(100)
      expect(p.copper).toBeLessThan(100)
      expect(p.gold * 10000 + p.silver * 100 + p.copper).toBe(c)
    }
  })
  it('rounds a fraction to the nearest copper, because there is no smaller coin', () => {
    expect(moneyParts(56.4)).toEqual({ negative: false, gold: 0, silver: 0, copper: 56 })
    expect(moneyParts(99.6)).toEqual({ negative: false, gold: 0, silver: 1, copper: 0 }) // not "99.6c", not "0s 100c"
  })
  it('keeps the sign apart from the amount', () => {
    expect(moneyParts(-12345)).toEqual({ negative: true, gold: 1, silver: 23, copper: 45 })
    expect(moneyParts(-0.2)).toEqual({ negative: false, gold: 0, silver: 0, copper: 0 }) // rounds to nothing: not "-0c"
  })
  it('has no amount for something that is not a number', () => {
    expect(moneyParts(NaN)).toBeNull()
    expect(moneyParts(Infinity)).toBeNull()
  })
})

describe('formatMoney edge cases', () => {
  it('writes a negative amount with its sign in front, the way a loss reads', () => {
    expect(formatMoney(-12345)).toBe('-1g 23s 45c')
    expect(formatMoney(-5)).toBe('-5c')
  })
  it('rounds to whole copper', () => {
    expect(formatMoney(12345.6)).toBe('1g 23s 46c')
    expect(formatMoney(99.6)).toBe('1s')
  })
  it('shows a dash for something that is not a number', () => {
    expect(formatMoney(NaN)).toBe('–')
  })
  it('keeps the most a character can hold readable', () => {
    expect(formatMoney(2147483647)).toBe('214,748g 36s 47c')
    expect(formatMoney(99999999999, 'es')).toBe('9.999.999o 99p 99c')
  })
})

describe('numbers the way each language writes them', () => {
  it('uses a decimal comma and a space before % in Spanish', () => {
    expect(formatDecimal(0.1, 'es')).toBe('0,1')
    expect(formatDecimal(0.1, 'en')).toBe('0.1')
    expect(formatPercent(5, 'es')).toBe('5 %')
    expect(formatPercent(5, 'en')).toBe('5%')
    expect(formatPercent(15, 'en')).toBe('15%')
  })
})
