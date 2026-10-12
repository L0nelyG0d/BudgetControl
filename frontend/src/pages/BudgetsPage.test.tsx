import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BudgetsPage from './BudgetsPage'
import { formatMoney } from '@/lib/format'

const categories = [
  { id: 1, name: 'Food', color: '#ff0000', is_default: true },
  { id: 2, name: 'Gym', color: '#00ff00', is_default: false },
]

const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status }))

type Handler = (url: string, init?: RequestInit) => Promise<Response>

function mockApi(budgets: unknown[], put?: Handler, del?: Handler) {
  const fn = vi.fn<Handler>((url, init) => {
    if (init?.method === 'PUT') return put ? put(url, init) : json(200, {})
    if (init?.method === 'DELETE') return del ? del(url, init) : Promise.resolve(new Response(null, { status: 204 }))
    if (url === '/api/categories') return json(200, categories)
    if (url === '/api/config') return json(200, { maxAmount: 1_000_000_000_000 })
    if (url.startsWith('/api/budgets')) return json(200, budgets)
    return json(404, { error: 'not found' })
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

const putCalls = (fn: ReturnType<typeof mockApi>) =>
  fn.mock.calls.filter(([, init]) => init?.method === 'PUT')

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'], now: new Date(2026, 9, 8, 12) })
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('BudgetsPage', () => {
  it('loads categories and budgets for the current month', async () => {
    const fetchMock = mockApi([{ category_id: 1, amount: 150000 }])
    render(<BudgetsPage />)
    expect(await screen.findByText('October 2026')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/budgets?month=2026-10', expect.anything())
    expect(screen.getByLabelText('Overall budget in tenge')).toHaveValue('')
    expect(screen.getByLabelText('Food budget in tenge')).toHaveValue('150 000')
    expect(screen.getByLabelText('Gym budget in tenge')).toHaveValue('')
    expect(screen.getByText(/150\s000\s₸/)).toBeInTheDocument()
  })

  it('shows a loading state, then an error with retry', async () => {
    const fn = vi.fn<Handler>(() => json(500, { error: 'boom' }))
    vi.stubGlobal('fetch', fn)
    render(<BudgetsPage />)
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load your budgets')
    mockApi([])
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByLabelText('Overall budget in tenge')).toBeInTheDocument()
  })

  it('saves a category budget', async () => {
    const fetchMock = mockApi([], () => json(200, { category_id: 2, amount: 20000 }))
    render(<BudgetsPage />)
    fireEvent.change(await screen.findByLabelText('Gym budget in tenge'), { target: { value: '20000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Gym budget' }))
    expect(await screen.findByText('Saved')).toBeInTheDocument()
    const [url, init] = putCalls(fetchMock)[0]
    expect(url).toBe('/api/budgets?month=2026-10')
    expect(JSON.parse(init!.body as string)).toEqual({ category_id: 2, amount: 20000 })
    const row = screen.getByLabelText('Gym budget in tenge').closest('tr')!
    expect(within(row).getByText(/20\s000\s₸/)).toBeInTheDocument()
  })

  it('saves the overall budget without category_id', async () => {
    const fetchMock = mockApi([], () => json(200, { category_id: null, amount: 500000 }))
    render(<BudgetsPage />)
    fireEvent.change(await screen.findByLabelText('Overall budget in tenge'), { target: { value: '500000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Overall budget' }))
    await screen.findByText('Saved')
    const body = JSON.parse(putCalls(fetchMock)[0][1]!.body as string)
    expect(body).toEqual({ amount: 500000 })
    expect('category_id' in body).toBe(false)
  })

  it('shows an error and keeps the typed value when saving fails', async () => {
    mockApi([], () => json(500, { error: 'Server broke' }))
    render(<BudgetsPage />)
    const input = await screen.findByLabelText('Food budget in tenge')
    fireEvent.change(input, { target: { value: '1000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
    expect(await screen.findByText(/Server broke/)).toBeInTheDocument()
    expect(input).toHaveValue('1 000')
    expect(screen.queryByText('Saved')).not.toBeInTheDocument()
  })

  it.each(['', '0', '-5', '12.5', '1,5', 'abc'])('rejects %j before any request', async (raw) => {
    const fetchMock = mockApi([])
    render(<BudgetsPage />)
    const input = await screen.findByLabelText('Food budget in tenge')
    if (raw) fireEvent.change(input, { target: { value: raw } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
    expect(putCalls(fetchMock)).toHaveLength(0)
  })

  it('removes a category budget', async () => {
    const fetchMock = mockApi([{ category_id: 1, amount: 150000 }])
    render(<BudgetsPage />)
    expect(await screen.findByLabelText('Food budget in tenge')).toHaveValue('150 000')
    expect(screen.queryByRole('button', { name: 'Remove Gym budget' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Remove Food budget' }))
    expect(await screen.findByText('Removed')).toBeInTheDocument()
    const [url, init] = fetchMock.mock.calls.find(([, i]) => i?.method === 'DELETE')!
    expect(url).toBe('/api/budgets?month=2026-10&category_id=1')
    expect(init?.method).toBe('DELETE')
    const input = screen.getByLabelText('Food budget in tenge')
    expect(input).toHaveValue('')
    const row = input.closest('tr')!
    expect(within(row).getByText('Not set')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove Food budget' })).not.toBeInTheDocument()
  })

  it('removes the overall budget without category_id', async () => {
    const fetchMock = mockApi([{ category_id: null, amount: 500000 }])
    render(<BudgetsPage />)
    await screen.findByLabelText('Overall budget in tenge')
    fireEvent.click(screen.getByRole('button', { name: 'Remove Overall budget' }))
    await screen.findByText('Removed')
    const [url] = fetchMock.mock.calls.find(([, i]) => i?.method === 'DELETE')!
    expect(url).toBe('/api/budgets?month=2026-10')
  })

  it('keeps the budget and shows an error when removing fails', async () => {
    mockApi([{ category_id: 1, amount: 150000 }], undefined, () => json(500, { error: 'Server broke' }))
    render(<BudgetsPage />)
    await screen.findByLabelText('Food budget in tenge')
    fireEvent.click(screen.getByRole('button', { name: 'Remove Food budget' }))
    expect(await screen.findByText(/Server broke/)).toBeInTheDocument()
    expect(screen.getByLabelText('Food budget in tenge')).toHaveValue('150 000')
    expect(screen.getByRole('button', { name: 'Remove Food budget' })).toBeInTheDocument()
  })

  describe('amount cap', () => {
    const capMessage = `Amount must be at most ${formatMoney(1_000_000_000_000)}.`
    // Exact match: the default text normalizer would collapse the non-breaking spaces.
    const capMatcher = (_: string, el: Element | null) => el?.tagName === 'P' && el.textContent === capMessage
    const rows = [['Overall', 'Overall budget in tenge'], ['Food', 'Food budget in tenge']]

    it.each(rows)('%s: over the cap shows the error, sends nothing, clears on edit', async (name, label) => {
      const fetchMock = mockApi([])
      render(<BudgetsPage />)
      const input = await screen.findByLabelText(label)
      fireEvent.change(input, { target: { value: '1000000000001' } })
      fireEvent.click(screen.getByRole('button', { name: `Save ${name} budget` }))
      const error = await screen.findByText(capMatcher)
      expect(error).toHaveClass('text-destructive')
      expect(input).toHaveAttribute('aria-invalid', 'true')
      expect(input.getAttribute('aria-describedby')).toBe(error.id)
      expect(putCalls(fetchMock)).toHaveLength(0)
      fireEvent.change(input, { target: { value: '1000' } })
      expect(screen.queryByText(capMatcher)).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: `Save ${name} budget` })).toBeEnabled()
    })

    it('shows the cap error for a 20 digit value', async () => {
      mockApi([])
      render(<BudgetsPage />)
      fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: '12345678901234567890' } })
      fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
      expect(await screen.findByText(capMatcher)).toBeInTheDocument()
    })

    it('accepts exactly the cap and sends it, trimming spaces', async () => {
      const fetchMock = mockApi([], () => json(200, { category_id: 1, amount: 1_000_000_000_000 }))
      render(<BudgetsPage />)
      fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: ' 1000000000000 ' } })
      fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
      await screen.findByText('Saved')
      expect(JSON.parse(putCalls(fetchMock)[0][1]!.body as string)).toEqual({ category_id: 1, amount: 1_000_000_000_000 })
    })

    it('accepts 007 and sends 7', async () => {
      const fetchMock = mockApi([], () => json(200, { category_id: 1, amount: 7 }))
      render(<BudgetsPage />)
      fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: '007' } })
      fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
      await screen.findByText('Saved')
      expect(JSON.parse(putCalls(fetchMock)[0][1]!.body as string)).toEqual({ category_id: 1, amount: 7 })
    })

    it.each(rows.flatMap(([name, label]) => ['1 500', '1,500', '1\u00A0500', '1\u202F500', ' 1 500\n'].map((raw) => [name, label, raw])))(
      '%s: %j is sent as 1500 and the input shows 1500 after saving',
      async (name, label, raw) => {
        const fetchMock = mockApi([], () => json(200, { category_id: name === 'Overall' ? null : 1, amount: 1500 }))
        render(<BudgetsPage />)
        const input = await screen.findByLabelText(label)
        fireEvent.change(input, { target: { value: raw } })
        fireEvent.click(screen.getByRole('button', { name: `Save ${name} budget` }))
        await screen.findByText('Saved')
        expect(JSON.parse(putCalls(fetchMock)[0][1]!.body as string)).toMatchObject({ amount: 1500 })
        expect(input).toHaveValue('1 500')
      },
    )

    it.each(['1,00', '1,5', '1.500', '12 34', '1  000', '0,500', '1 500 ₸', '500,'])(
      'rejects %j with the whole-number message and no request',
      async (raw) => {
        const fetchMock = mockApi([])
        render(<BudgetsPage />)
        fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: raw } })
        fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
        expect(await screen.findByText('Amount must be a whole number greater than 0.')).toBeInTheDocument()
        expect(putCalls(fetchMock)).toHaveLength(0)
      },
    )

    it('negative above the cap shows the whole-number message, not the cap', async () => {
      const fetchMock = mockApi([])
      render(<BudgetsPage />)
      fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: '-1000000000001' } })
      fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
      expect(await screen.findByText('Amount must be a whole number greater than 0.')).toBeInTheDocument()
      expect(putCalls(fetchMock)).toHaveLength(0)
    })

    it('shows the backend 400 message as is, without the connection suffix', async () => {
      mockApi([], () => json(400, { error: 'amount must be at most 1000000000000 tenge' }))
      render(<BudgetsPage />)
      fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: '1000' } })
      fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
      const alert = await screen.findByText(/amount must be at most/)
      expect(alert).toHaveTextContent(/^amount must be at most 1000000000000 tenge$/)
      expect(alert).not.toHaveTextContent('Check your connection')
    })

    it('keeps the connection suffix for 5xx errors', async () => {
      mockApi([], () => json(500, { error: 'Server broke' }))
      render(<BudgetsPage />)
      fireEvent.change(await screen.findByLabelText('Food budget in tenge'), { target: { value: '1000' } })
      fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' }))
      expect(await screen.findByText(/Server broke\. Check your connection and try again\./)).toBeInTheDocument()
    })
  })
})
