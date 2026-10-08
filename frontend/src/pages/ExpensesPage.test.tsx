import { fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ExpensesPage from './ExpensesPage'
import { todayISO } from '@/features/expenses/expenses'

const categories = [
  { id: 1, name: 'Food', color: '#f00', is_default: true },
  { id: 2, name: 'Transport', color: '#0f0', is_default: true },
]
const existing = [
  { id: 1, category_id: 1, amount: 1500, date: '2026-10-05', note: 'Lunch', created_at: '2026-10-05T10:00:00Z' },
  { id: 2, category_id: 2, amount: 800, date: '2026-10-01', note: null, created_at: '2026-10-01T10:00:00Z' },
]

const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status }))

type Handler = (url: string, init?: RequestInit) => Promise<Response>
function mockFetch(handler: Handler) {
  const fn = vi.fn(handler)
  vi.stubGlobal('fetch', fn)
  return fn
}
const standard =
  (expenses: unknown, post?: Handler): Handler =>
  (url, init) => {
    if (init?.method === 'POST' && post) return post(url, init)
    if (url === '/api/expenses') return json(200, expenses)
    if (url === '/api/categories') return json(200, categories)
    return json(404, { error: 'not found' })
  }

const postsTo = (fn: ReturnType<typeof mockFetch>) =>
  fn.mock.calls.filter(([, init]) => init?.method === 'POST')

afterEach(() => vi.unstubAllGlobals())

function rowTexts() {
  return screen.getAllByRole('row').slice(1).map((r) => r.textContent ?? '')
}

describe('ExpensesPage', () => {
  it('shows a loading state, then expenses newest first with formatted values', async () => {
    mockFetch(standard([...existing].reverse()))
    render(<ExpensesPage />)
    expect(screen.getByRole('status')).toBeInTheDocument()
    await screen.findByRole('table')
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows).toHaveLength(2)
    const first = within(rows[0]).getAllByRole('cell').map((c) => c.textContent)
    expect(first[0]).toMatch(/^1\s500\s₸$/)
    expect(first.slice(1)).toEqual(['Food', '05.10.2026', 'Lunch'])
    expect(rows[1]).toHaveTextContent('01.10.2026')
  })

  it('shows the empty state', async () => {
    mockFetch(standard([]))
    render(<ExpensesPage />)
    expect(await screen.findByText('No expenses yet')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows an error with a retry that reloads', async () => {
    let fail = true
    mockFetch((url, init) =>
      fail && url === '/api/expenses' ? json(500, { error: 'boom' }) : standard(existing)(url, init),
    )
    render(<ExpensesPage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load your expenses')
    fail = false
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('table')).toBeInTheDocument()
  })

  it.each([
    ['empty', ''],
    ['zero', '0'],
    ['negative', '-5'],
    ['decimal', '12.5'],
  ])('rejects a %s amount inline without a request', async (_n, value) => {
    const fetchMock = mockFetch(standard(existing))
    render(<ExpensesPage />)
    await screen.findByRole('table')
    fireEvent.change(screen.getByLabelText(/^Amount/), { target: { value } })
    fireEvent.click(screen.getByRole('button', { name: 'Add expense' }))
    expect(await screen.findByText(/amount/i, { selector: 'p' })).toBeInTheDocument()
    expect(screen.getByLabelText(/^Amount/)).toHaveAttribute('aria-invalid', 'true')
    expect(postsTo(fetchMock)).toHaveLength(0)
  })

  it('adds an expense at its sorted position and clears the form but keeps the date', async () => {
    const created = { id: 3, category_id: 1, amount: 2000, date: '2026-10-03', note: 'Taxi', created_at: '2026-10-08T10:00:00Z' }
    const fetchMock = mockFetch(standard(existing, () => json(201, created)))
    render(<ExpensesPage />)
    await screen.findByRole('table')
    expect(screen.getByLabelText('Date')).toHaveValue(todayISO())
    fireEvent.change(screen.getByLabelText(/^Amount/), { target: { value: '2000' } })
    fireEvent.change(screen.getByLabelText('Date'), { target: { value: '2026-10-03' } })
    fireEvent.change(screen.getByLabelText(/^Note/), { target: { value: 'Taxi' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add expense' }))
    await screen.findByText('03.10.2026')

    const rows = rowTexts()
    expect(rows).toHaveLength(3)
    expect(rows[1]).toContain('03.10.2026')
    expect(JSON.parse(String(postsTo(fetchMock)[0][1]?.body))).toEqual({
      amount: 2000, category_id: 1, date: '2026-10-03', note: 'Taxi',
    })
    expect(screen.getByLabelText(/^Amount/)).toHaveValue('')
    expect(screen.getByLabelText(/^Note/)).toHaveValue('')
    expect(screen.getByLabelText('Date')).toHaveValue('2026-10-03')
  })

  it('keeps input and shows an error when the add fails, disabling submit while pending', async () => {
    let resolve!: (r: Response) => void
    mockFetch(standard(existing, () => new Promise<Response>((r) => (resolve = r))))
    render(<ExpensesPage />)
    await screen.findByRole('table')
    fireEvent.change(screen.getByLabelText(/^Amount/), { target: { value: '700' } })
    fireEvent.change(screen.getByLabelText(/^Note/), { target: { value: 'Bus' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add expense' }))
    expect(await screen.findByRole('button', { name: /Adding/ })).toBeDisabled()
    resolve(new Response(JSON.stringify({ error: 'amount is too large' }), { status: 400 }))
    expect(await screen.findByRole('alert')).toHaveTextContent('amount is too large')
    expect(screen.getByLabelText(/^Amount/)).toHaveValue('700')
    expect(screen.getByLabelText(/^Note/)).toHaveValue('Bus')
    expect(screen.getByRole('button', { name: 'Add expense' })).toBeEnabled()
    expect(rowTexts()).toHaveLength(2)
  })
})
