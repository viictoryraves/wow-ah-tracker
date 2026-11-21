import { formatAgo, formatSold, formatClock, formatMoney, formatPct } from './format'

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
