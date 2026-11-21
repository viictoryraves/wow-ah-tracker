import { render, screen } from '@testing-library/react'
import { Money } from './Money'

const coins = (container: HTMLElement) => Array.from(container.querySelectorAll('.coin')).map((c) => c.getAttribute('data-coin'))

describe('Money', () => {
  it('shows each amount followed by its gold, silver or copper coin', () => {
    const { container } = render(<Money copper={123456} />)
    expect(coins(container)).toEqual(['gold', 'silver', 'copper'])
    expect(container).toHaveTextContent('12g 34s 56c') // the unit letters are there for screen readers
    expect(container.querySelectorAll('.coin[aria-hidden="true"]')).toHaveLength(3)
  })

  it('leaves out the coins that are zero, but never shows nothing', () => {
    expect(coins(render(<Money copper={10000} />).container)).toEqual(['gold'])
    expect(coins(render(<Money copper={250} />).container)).toEqual(['silver', 'copper'])
    const zero = render(<Money copper={0} />).container
    expect(coins(zero)).toEqual(['copper'])
    expect(zero).toHaveTextContent('0c')
  })

  it('groups thousands of gold', () => {
    render(<Money copper={12345600000} />)
    expect(screen.getByText('1,234,560')).toBeInTheDocument()
  })

  it('puts the sign of a loss in front of the coins', () => {
    const { container } = render(<Money copper={-12345} />)
    expect(container).toHaveTextContent('-1g 23s 45c')
    expect(coins(container)).toEqual(['gold', 'silver', 'copper'])
  })

  it('rounds to whole copper and never shows 100 silver or copper', () => {
    expect(render(<Money copper={99.6} />).container).toHaveTextContent('1s')
    expect(render(<Money copper={56.4} />).container).toHaveTextContent('56c')
  })

  it('shows a dash for something that is not a number', () => {
    const { container } = render(<Money copper={NaN} />)
    expect(container).toHaveTextContent('–')
    expect(coins(container)).toEqual([])
  })
})
