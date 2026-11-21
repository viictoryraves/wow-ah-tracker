import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PriceChart } from './PriceChart'
import type { Point } from './api'

const pt = (hour: number, min: number, median: number): Point => ({
  time: new Date(Date.UTC(2026, 10, 1, hour)).toISOString(),
  min,
  median,
  avg: median,
  quantity: 1,
})

describe('PriceChart', () => {
  it('tells the user when there is no history yet', () => {
    render(<PriceChart points={[]} />)
    expect(screen.getByText(/no price history yet/i)).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('draws one line for the lowest price and one for the median, one vertex per point', () => {
    render(<PriceChart points={[pt(0, 100, 150), pt(1, 200, 250), pt(2, 300, 350)]} />)
    expect(screen.getByRole('img', { name: /price history/i })).toBeInTheDocument()
    const lowest = screen.getByLabelText('Lowest price')
    const median = screen.getByLabelText('Median price')
    expect(lowest.getAttribute('points')!.trim().split(' ')).toHaveLength(3)
    expect(median.getAttribute('points')!.trim().split(' ')).toHaveLength(3)
  })

  it('plots higher prices higher up (smaller y) and later times further right', () => {
    render(<PriceChart points={[pt(0, 100, 100), pt(1, 300, 300)]} />)
    const [first, second] = screen
      .getByLabelText('Lowest price')
      .getAttribute('points')!
      .trim()
      .split(' ')
      .map((p) => p.split(',').map(Number))
    expect(second[0]).toBeGreaterThan(first[0])
    expect(second[1]).toBeLessThan(first[1])
  })

  it('labels the price axis with the lowest and highest price in WoW money', () => {
    render(<PriceChart points={[pt(0, 10000, 20000), pt(1, 30000, 50000)]} />)
    expect(screen.getByText('1g')).toBeInTheDocument()
    expect(screen.getByText('5g')).toBeInTheDocument()
  })

  it('still shows a single point, even though one point cannot make a line', () => {
    render(<PriceChart points={[pt(0, 100, 100)]} />)
    expect(screen.getByRole('img', { name: /price history/i })).toBeInTheDocument()
    expect(screen.getAllByTestId('point').length).toBeGreaterThan(0)
  })

  it('does not break when every price is identical', () => {
    render(<PriceChart points={[pt(0, 100, 100), pt(1, 100, 100)]} />)
    const ys = screen
      .getByLabelText('Lowest price')
      .getAttribute('points')!
      .split(' ')
      .map((p) => Number(p.split(',')[1]))
    expect(ys.every((y) => Number.isFinite(y))).toBe(true)
  })

  it('describes the chart in text for screen readers', () => {
    render(<PriceChart points={[pt(0, 10000, 20000), pt(1, 30000, 50000), pt(2, 20000, 40000)]} />)
    expect(screen.getByRole('img', { name: /price history/i })).toHaveAccessibleDescription(
      /lowest price.*from 1g to 3g.*3 snapshots.*latest 2g/i,
    )
  })

  describe('intraday', () => {
    const hourly = (n: number, firstHour = 0) =>
      Array.from({ length: n }, (_, i) => pt(firstHour + i, 100 * (i + 1), 120 * (i + 1)))
    const axis = () => within(screen.getByRole('img', { name: /price history/i }))

    it('labels the time axis with the time of day when the data spans less than two days', () => {
      render(<PriceChart points={hourly(5)} timeZone="UTC" />)
      for (const t of ['00:00', '01:00', '02:00', '03:00', '04:00']) {
        expect(axis().getByText(t)).toBeInTheDocument()
      }
    })

    it('labels the time axis with dates when the data spans several days', () => {
      const days = [0, 24, 48, 72].map((h) => pt(h, 100, 100))
      render(<PriceChart points={days} timeZone="UTC" />)
      expect(axis().getByText('1 Nov')).toBeInTheDocument()
      expect(axis().getByText('4 Nov')).toBeInTheDocument()
    })

    it('shows at most five time labels, always including the first and last point', () => {
      render(<PriceChart points={hourly(24)} timeZone="UTC" />)
      expect(axis().getByText('00:00')).toBeInTheDocument()
      expect(axis().getByText('23:00')).toBeInTheDocument()
      const labels = Array.from(screen.getByRole('img', { name: /price history/i }).querySelectorAll('text')).filter(
        (t) => /^\d\d:\d\d$/.test(t.textContent ?? ''),
      )
      expect(labels.length).toBeLessThanOrEqual(5)
    })

    it('spaces the time labels by time, so a gap in the data does not bunch them up', () => {
      // hourly for 8 hours, then nothing until 24h: labels belong near 0h, 6h, 12h, 18h and 24h
      const withGap = [0, 1, 2, 3, 4, 5, 6, 7, 8, 24].map((h) => pt(h, 100, 100))
      render(<PriceChart points={withGap} timeZone="UTC" />)
      expect(axis().getByText('06:00')).toBeInTheDocument() // 6h target -> the 6h snapshot
      expect(axis().queryByText('02:00')).not.toBeInTheDocument() // index-based picking would label this
      expect(axis().queryByText('05:00')).not.toBeInTheDocument()
    })

    it('marks every hourly snapshot with a point', () => {
      render(<PriceChart points={hourly(6)} timeZone="UTC" />)
      expect(screen.getAllByTestId('point')).toHaveLength(6)
    })

    it('drops the per-snapshot markers when there are too many to tell apart', () => {
      render(<PriceChart points={hourly(200)} timeZone="UTC" />)
      expect(screen.queryAllByTestId('point')).toHaveLength(0)
    })

    it('reads out the latest snapshot by default', () => {
      render(<PriceChart points={[pt(0, 10000, 20000), pt(4, 30000, 50000)]} timeZone="UTC" />)
      const readout = screen.getByTestId('readout')
      expect(readout).toHaveTextContent('04:00')
      expect(readout).toHaveTextContent('3g') // lowest
      expect(readout).toHaveTextContent('5g') // median
      expect(readout).toHaveTextContent('1 listed')
    })

    it('lets keyboard users step through the snapshots', async () => {
      const user = userEvent.setup()
      render(<PriceChart points={hourly(3)} timeZone="UTC" />)
      const readout = screen.getByTestId('readout')
      screen.getByRole('figure').focus()

      await user.keyboard('{ArrowLeft}')
      expect(readout).toHaveTextContent('01:00')
      await user.keyboard('{ArrowLeft}{ArrowLeft}{ArrowLeft}') // stops at the first point
      expect(readout).toHaveTextContent('00:00')
      await user.keyboard('{End}')
      expect(readout).toHaveTextContent('02:00')
      await user.keyboard('{ArrowRight}') // already at the last point
      expect(readout).toHaveTextContent('02:00')
      await user.keyboard('{Home}')
      expect(readout).toHaveTextContent('00:00')
    })

    it('reads out the snapshot nearest to the pointer and returns to the latest when it leaves', () => {
      render(<PriceChart points={hourly(3)} timeZone="UTC" />)
      const svg = screen.getByRole('img', { name: /price history/i })
      // viewBox is 480 wide; the plot starts at x=112, so clientX 112 is the first point
      vi.spyOn(svg, 'getBoundingClientRect').mockReturnValue({ left: 0, width: 480, top: 0, height: 260 } as DOMRect)

      fireEvent.pointerMove(svg, { clientX: 112 })
      expect(screen.getByTestId('readout')).toHaveTextContent('00:00')
      fireEvent.pointerLeave(svg)
      expect(screen.getByTestId('readout')).toHaveTextContent('02:00')
    })

    it('highlights the point being read', () => {
      render(<PriceChart points={hourly(3)} timeZone="UTC" />)
      expect(screen.getAllByTestId('active-point')).toHaveLength(1)
    })

    it('says which time zone the times are in', () => {
      render(<PriceChart points={hourly(3)} timeZone="UTC" />)
      expect(screen.getByText(/times in utc/i)).toBeInTheDocument()
    })
  })
})
