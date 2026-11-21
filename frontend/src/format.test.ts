import { formatMoney, formatPct } from './format'

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
