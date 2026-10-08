import '@testing-library/jest-dom/vitest'
import { render, screen, within, fireEvent } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import DashboardPage from './DashboardPage'
import { currentMonth, summarize } from '@/features/dashboard/summary'

const cats = [
  { id: 1, name: 'Food', color: '#ef4444', is_default: false },
  { id: 2, name: 'Transport', color: '#3b82f6', is_default: false },
  { id: 3, name: 'Fun', color: '#22c55e', is_default: false },
]
const exp = (id: number, category_id: number, amount: number, date = '2026-10-05') => ({
  id, category_id, amount, date, note: '', created_at: '',
})
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })

function mock(data: { expenses: unknown; budgets: unknown }, fail = false) {
  const fn = vi.fn(async (url: string) => {
    if (fail) return json(500, { error: 'boom' })
    if (url.startsWith('/api/expenses')) return json(200, data.expenses)
    if (url === '/api/categories') return json(200, cats)
    if (url.startsWith('/api/budgets')) return json(200, data.budgets)
    return json(404, {})
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

beforeAll(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
})
afterEach(() => vi.clearAllMocks())

describe('summarize', () => {
  it('aggregates per category with budgets, including budget-only categories', () => {
    const s = summarize(
      [exp(1, 1, 3000), exp(2, 1, 2000), exp(3, 2, 500)],
      cats,
      [{ category_id: 1, amount: 4000 }, { category_id: 3, amount: 1000 }, { category_id: null, amount: 10000 }],
    )
    expect(s.total).toBe(5500)
    expect(s.rows.map((r) => [r.name, r.spent, r.budget, r.remaining])).toEqual([
      ['Food', 5000, 4000, -1000],
      ['Transport', 500, null, null],
      ['Fun', 0, 1000, 1000],
    ])
    expect(s.overall).toEqual({ budget: 10000, spent: 5500, remaining: 4500 })
  })

  it('has no overall without an overall budget and formats the month', () => {
    expect(summarize([], cats, []).overall).toBeNull()
    expect(currentMonth(new Date(2026, 0, 31))).toBe('2026-01')
  })
})

describe('DashboardPage', () => {
  it('shows chart legend, total and table; requests the current month', async () => {
    const fn = mock({
      expenses: [exp(1, 1, 5000), exp(2, 2, 1500, '2026-10-30')],
      budgets: [{ category_id: 1, amount: 8000 }],
    })
    render(<DashboardPage />)
    const legend = await screen.findByRole('list', { name: 'Chart legend' })
    expect(within(legend).getByText('Food')).toBeInTheDocument()
    expect(within(legend).getByText(/5\s000/)).toBeInTheDocument()
    expect(within(legend).getByText('Transport')).toBeInTheDocument()
    expect(screen.getByTestId('total-spent').textContent).toMatch(/6\s500/)
    const urls = fn.mock.calls.map(([u]) => u)
    expect(urls).toContain(`/api/expenses?month=${currentMonth()}`)
    expect(urls).toContain(`/api/budgets?month=${currentMonth()}`)
  })

  it('shows the empty message but still the budget table', async () => {
    mock({ expenses: [], budgets: [{ category_id: 3, amount: 1000 }, { category_id: null, amount: 9000 }] })
    render(<DashboardPage />)
    expect(await screen.findByText('No expenses this month')).toBeInTheDocument()
    expect(screen.queryByRole('list', { name: 'Chart legend' })).not.toBeInTheDocument()
    const fun = screen.getByRole('row', { name: /Fun/ })
    expect(within(fun).getAllByRole('cell')[1].textContent).toMatch(/^0/)
    expect(screen.getByRole('row', { name: /Overall budget/ })).toBeInTheDocument()
  })

  it('flags an over-budget category and the overall budget with text', async () => {
    mock({
      expenses: [exp(1, 1, 5000), exp(2, 2, 4000)],
      budgets: [{ category_id: 1, amount: 3000 }, { category_id: null, amount: 8000 }],
    })
    render(<DashboardPage />)
    const food = await screen.findByRole('row', { name: /Food/ })
    expect(food.textContent).toMatch(/Over by 2\s000/)
    expect(screen.getByRole('row', { name: /Overall budget/}).textContent).toMatch(/Over by 1\s000/)
    expect(screen.getByRole('row', { name: /Transport/ }).textContent).not.toMatch(/Over by/)
  })

  it('shows loading, then an error with retry', async () => {
    mock({ expenses: [], budgets: [] }, true)
    render(<DashboardPage />)
    expect(screen.getByRole('status', { name: 'Loading dashboard' })).toBeInTheDocument()
    expect(await screen.findByRole('alert')).toHaveTextContent('boom')
    mock({ expenses: [], budgets: [] })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No expenses this month')).toBeInTheDocument()
  })
})
