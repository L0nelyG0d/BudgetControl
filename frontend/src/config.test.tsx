import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ExpensesPage from '@/pages/ExpensesPage'
import BudgetsPage from '@/pages/BudgetsPage'
import { formatMoney } from '@/lib/format'

const categories = [{ id: 1, name: 'Food', color: '#f00', is_default: true }]
const saved = { id: 5, category_id: 1, amount: 1, date: '2026-10-05', note: null, created_at: '2026-10-05T10:00:00Z' }
const backend400 = 'amount must be at most 1000000000000 tenge'

const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status }))

type ConfigReply = () => Promise<Response>

function mockApi(config: ConfigReply) {
  const fn = vi.fn((url: string, init?: RequestInit) => {
    if (url === '/api/config') return config()
    if (init?.method === 'POST') {
      const amount = JSON.parse(String(init.body)).amount as number
      return amount > 1_000_000_000_000 ? json(400, { error: backend400 }) : json(201, saved)
    }
    if (init?.method === 'PUT') {
      const amount = JSON.parse(String(init.body)).amount as number
      return amount > 1_000_000_000_000 ? json(400, { error: backend400 }) : json(200, { category_id: 1, amount })
    }
    if (url === '/api/expenses') return json(200, [])
    if (url === '/api/categories') return json(200, categories)
    if (url.startsWith('/api/budgets')) return json(200, [])
    return json(404, { error: 'not found' })
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

const configCalls = (fn: ReturnType<typeof mockApi>) => fn.mock.calls.filter(([url]) => url === '/api/config')
const writes = (fn: ReturnType<typeof mockApi>) =>
  fn.mock.calls.filter(([, init]) => init?.method === 'POST' || init?.method === 'PUT')

const matcher = (text: string) => (_: string, el: Element | null) => el?.tagName === 'P' && el.textContent === text

async function addExpense(value: string) {
  fireEvent.change(await screen.findByLabelText(/^Amount/), { target: { value } })
  fireEvent.click(screen.getByRole('button', { name: 'Add expense' }))
}
async function saveFood(value: string) {
  fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value } })
  fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
}

afterEach(() => vi.unstubAllGlobals())

describe('amount limit from GET /config', () => {
  it('enforces the loaded limit and uses it in the message (not a constant)', async () => {
    const fn = mockApi(() => json(200, { maxAmount: 5000 }))
    render(<ExpensesPage />)
    await screen.findByText('No expenses yet')
    await waitFor(() => expect(configCalls(fn)).toHaveLength(1))
    await new Promise((r) => setTimeout(r, 0))
    await addExpense('5001')
    expect(await screen.findByText(matcher(`Amount must be at most ${formatMoney(5000)}.`))).toBeInTheDocument()
    expect(writes(fn)).toHaveLength(0)
  })

  it('accepts exactly the loaded limit', async () => {
    const fn = mockApi(() => json(200, { maxAmount: 5000 }))
    render(<ExpensesPage />)
    await screen.findByText('No expenses yet')
    await new Promise((r) => setTimeout(r, 0))
    await addExpense('5 000')
    await waitFor(() => expect(writes(fn)).toHaveLength(1))
    expect(JSON.parse(String(writes(fn)[0][1]?.body)).amount).toBe(5000)
  })

  it('enforces the loaded limit in budget rows', async () => {
    const fn = mockApi(() => json(200, { maxAmount: 5000 }))
    render(<BudgetsPage />)
    await screen.findByLabelText('Food budget in tenge')
    await new Promise((r) => setTimeout(r, 0))
    await saveFood('5001')
    expect(await screen.findByText(matcher(`Amount must be at most ${formatMoney(5000)}.`))).toBeInTheDocument()
    expect(writes(fn)).toHaveLength(0)
    await saveFood('5000')
    await screen.findByText('Saved')
    expect(JSON.parse(String(writes(fn)[0][1]?.body))).toEqual({ category_id: 1, amount: 5000 })
  })

  describe('limit unknown: forms stay usable and the backend 400 is shown', () => {
    const cases: [string, ConfigReply][] = [
      ['request pending', () => new Promise<Response>(() => {})],
      ['network error', () => Promise.reject(new TypeError('Failed to fetch'))],
      ['500', () => json(500, { error: 'boom' })],
      ['body without maxAmount', () => json(200, {})],
      ['maxAmount is a string', () => json(200, { maxAmount: '5000' })],
      ['maxAmount is zero', () => json(200, { maxAmount: 0 })],
      ['maxAmount is negative', () => json(200, { maxAmount: -5 })],
      ['maxAmount is a decimal', () => json(200, { maxAmount: 10.5 })],
      ['body is not JSON', () => Promise.resolve(new Response('<html>', { status: 200 }))],
    ]

    it.each(cases)('expense form, %s', async (_, reply) => {
      const fn = mockApi(reply)
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await new Promise((r) => setTimeout(r, 0))
      expect(screen.getByRole('button', { name: 'Add expense' })).toBeEnabled()
      await addExpense('1000000000001')
      expect(await screen.findByText(backend400)).toBeInTheDocument()
      expect(writes(fn)).toHaveLength(1)
      expect(screen.queryByText(/Amount must be at most/)).not.toBeInTheDocument()
    })

    it.each(cases)('budget row, %s', async (_, reply) => {
      const fn = mockApi(reply)
      render(<BudgetsPage />)
      await screen.findByLabelText('Food budget in tenge')
      await new Promise((r) => setTimeout(r, 0))
      expect(screen.getByRole('button', { name: 'Save Food budget' })).toBeEnabled()
      await saveFood('1000000000001')
      expect(await screen.findByText(new RegExp(backend400))).toBeInTheDocument()
      expect(writes(fn)).toHaveLength(1)
      expect(screen.queryByText(/Amount must be at most/)).not.toBeInTheDocument()
    })

    it.each(cases)('other amount rules still apply, %s', async (_, reply) => {
      const fn = mockApi(reply)
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      for (const [value, message] of [
        ['', /Enter an amount/],
        ['0', /whole number/],
        ['-5', /whole number/],
        ['1.5', /whole number/],
        ['abc', /whole number/],
        ['12 34', /whole number/],
      ] as const) {
        await addExpense(value)
        expect(await screen.findByText(message)).toBeInTheDocument()
      }
      expect(writes(fn)).toHaveLength(0)
    })

    it('shows no error banner when the load fails', async () => {
      mockApi(() => Promise.reject(new TypeError('Failed to fetch')))
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await new Promise((r) => setTimeout(r, 0))
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })
  })

  it('requests /api/config once for the expense form and every budget row', async () => {
    const fn = mockApi(() => json(200, { maxAmount: 5000 }))
    render(<ExpensesPage />)
    await screen.findByText('No expenses yet')
    cleanup()
    render(<BudgetsPage />)
    await screen.findByLabelText('Food budget in tenge')
    expect(screen.getByLabelText('Overall budget in tenge')).toBeInTheDocument()
    await new Promise((r) => setTimeout(r, 0))
    expect(configCalls(fn)).toHaveLength(1)
  })

  describe('retry after a failed load (#36)', () => {
    const failures: [string, ConfigReply][] = [
      ['network error', () => Promise.reject(new TypeError('Failed to fetch'))],
      ['500', () => json(500, { error: 'boom' })],
      ['malformed body', () => json(200, { maxAmount: 'lots' })],
    ]
    const tick = () => new Promise((r) => setTimeout(r, 0))

    // First /api/config call gets `first`, later ones get `later`.
    function mockSequence(first: ConfigReply, later: ConfigReply) {
      let n = 0
      return mockApi(() => (n++ === 0 ? first() : later()))
    }

    it.each(failures)('%s: the next form open retries once and the limit is enforced', async (_, fail) => {
      const fn = mockSequence(fail, () => json(200, { maxAmount: 5000 }))
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      expect(configCalls(fn)).toHaveLength(1)
      cleanup()
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await waitFor(() => expect(configCalls(fn)).toHaveLength(2))
      await tick()
      await addExpense('5001')
      expect(await screen.findByText(matcher(`Amount must be at most ${formatMoney(5000)}.`))).toBeInTheDocument()
      expect(writes(fn)).toHaveLength(0)
      await addExpense('5000')
      await waitFor(() => expect(writes(fn)).toHaveLength(1))
    })

    it('a failed retry leaves the limit unknown, shows the backend 400 and sends nothing more', async () => {
      const fn = mockApi(() => Promise.reject(new TypeError('Failed to fetch')))
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      cleanup()
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await waitFor(() => expect(configCalls(fn)).toHaveLength(2))
      await tick()
      await addExpense('1000000000001')
      expect(await screen.findByText(backend400)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Add expense' })).toBeEnabled()
      await tick()
      expect(configCalls(fn)).toHaveLength(2)
      // Each later open retries again.
      cleanup()
      render(<ExpensesPage />)
      await waitFor(() => expect(configCalls(fn)).toHaveLength(3))
    })

    it('forms mounting together send one request, also while one is in flight', async () => {
      const fn = mockApi(() => Promise.reject(new TypeError('Failed to fetch')))
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      cleanup()
      const fn2 = mockApi(() => new Promise<Response>(() => {}))
      render(<BudgetsPage />)
      await screen.findByLabelText('Food budget in tenge')
      render(<ExpensesPage />)
      await tick()
      expect(configCalls(fn)).toHaveLength(1)
      expect(configCalls(fn2)).toHaveLength(1)
    })

    it('typing and validation errors never start a request', async () => {
      const fn = mockApi(() => Promise.reject(new TypeError('Failed to fetch')))
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      await addExpense('abc')
      await screen.findByText(/whole number/)
      await addExpense('')
      await addExpense('1000000000001')
      await screen.findByText(backend400)
      await tick()
      expect(configCalls(fn)).toHaveLength(1)
    })

    it('no request after a successful load, however many forms open', async () => {
      const fn = mockSequence(
        () => Promise.reject(new TypeError('Failed to fetch')),
        () => json(200, { maxAmount: 5000 }),
      )
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      cleanup()
      render(<ExpensesPage />)
      await waitFor(() => expect(configCalls(fn)).toHaveLength(2))
      await tick()
      cleanup()
      render(<BudgetsPage />)
      await screen.findByLabelText('Food budget in tenge')
      cleanup()
      render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      expect(configCalls(fn)).toHaveLength(2)
      expect(localStorage.length).toBe(0)
    })

    it('an already open form picks up the limit when the retry succeeds', async () => {
      let release: (r: Response) => void = () => {}
      const fn = mockSequence(
        () => Promise.reject(new TypeError('Failed to fetch')),
        () => new Promise<Response>((r) => (release = r)),
      )
      const first = render(<ExpensesPage />)
      await screen.findByText('No expenses yet')
      await tick()
      const open = within(first.container)
      // A second form opens (the retry) while the first stays open.
      render(<BudgetsPage />)
      await waitFor(() => expect(configCalls(fn)).toHaveLength(2))
      release(new Response(JSON.stringify({ maxAmount: 5000 }), { status: 200 }))
      await tick()
      fireEvent.change(await open.findByLabelText(/^Amount/), { target: { value: '5001' } })
      fireEvent.click(open.getByRole('button', { name: 'Add expense' }))
      expect(await open.findByText(matcher(`Amount must be at most ${formatMoney(5000)}.`))).toBeInTheDocument()
      expect(writes(fn)).toHaveLength(0)
    })
  })
})
