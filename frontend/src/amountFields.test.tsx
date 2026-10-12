import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExpenseForm } from '@/features/expenses/ExpenseForm'
import { BudgetRow } from '@/features/budgets/BudgetRow'

const categories = [{ id: 1, name: 'Food', color: '#f00', is_default: true }]
const json = (status: number, body: unknown) => Promise.resolve(new Response(JSON.stringify(body), { status }))

function mockFetch() {
  const fn = vi.fn((url: string, init?: RequestInit) => {
    if (url === '/api/config') return json(200, { maxAmount: 1_000_000_000_000 })
    if (init?.method === 'POST' || init?.method === 'PUT') {
      const body = JSON.parse(init.body as string)
      return json(200, { id: 1, category_id: 1, date: '2026-10-05', note: null, created_at: 'x', ...body })
    }
    return json(404, { error: 'not found' })
  })
  vi.stubGlobal('fetch', fn)
  return fn
}
const writes = (fn: ReturnType<typeof mockFetch>) =>
  fn.mock.calls.filter(([, init]) => init?.method === 'POST' || init?.method === 'PUT')

afterEach(() => vi.unstubAllGlobals())

/** Simulates one browser edit: new value, caret after the edit, and the DOM inputType. */
function edit(el: HTMLInputElement, value: string, caret: number, inputType = 'insertText') {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  act(() => {
    setter.call(el, value)
    el.setSelectionRange(caret, caret)
    el.dispatchEvent(new InputEvent('input', { bubbles: true, inputType }))
  })
}
function typeKeys(el: HTMLInputElement, text: string) {
  for (const ch of text) edit(el, el.value + ch, el.value.length + 1)
}
function paste(el: HTMLInputElement, text: string) {
  fireEvent.paste(el)
  edit(el, text, text.length, 'insertFromPaste')
}

type Setup = {
  name: string
  render: (saved?: number) => void
  input: () => HTMLInputElement
  submit: () => void
  error: () => HTMLElement | null
}
const expenseSetup: Setup = {
  name: 'ExpenseForm',
  render: (saved) =>
    render(
      <ExpenseForm
        categories={categories}
        onSaved={() => {}}
        expense={
          saved === undefined
            ? undefined
            : { id: 1, category_id: 1, amount: saved, date: '2026-10-05', note: null, created_at: 'x' }
        }
      />,
    ),
  input: () => screen.getByLabelText(/^Amount/),
  submit: () => fireEvent.click(screen.getByRole('button', { name: /expense|Save changes/ })),
  error: () => screen.queryByText(/whole number|Enter an amount/),
}
const budgetSetup: Setup = {
  name: 'BudgetRow',
  render: (saved) =>
    render(
      <table>
        <tbody>
          <BudgetRow name="Food" categoryId={1} month="2026-10" initialAmount={saved} />
        </tbody>
      </table>,
    ),
  input: () => screen.getByLabelText('Food budget in tenge'),
  submit: () => fireEvent.click(screen.getByRole('button', { name: 'Save Food budget' })),
  error: () => screen.queryByText(/whole number|Enter an amount/),
}

describe.each([expenseSetup, budgetSetup])('$name amount grouping', (s) => {
  it('groups while typing and sends a plain number', async () => {
    const fetchMock = mockFetch()
    s.render()
    const input = s.input() as HTMLInputElement
    typeKeys(input, '1500000')
    expect(input).toHaveValue('1 500 000')
    s.submit()
    await waitFor(() => expect(writes(fetchMock)).toHaveLength(1))
    expect(JSON.parse(writes(fetchMock)[0][1]!.body as string).amount).toBe(1500000)
  })

  it('shows a saved value grouped', () => {
    mockFetch()
    s.render(1500000)
    expect(s.input()).toHaveValue('1 500 000')
  })

  it('keeps the caret after a digit typed in the middle', () => {
    mockFetch()
    s.render(1500)
    const input = s.input() as HTMLInputElement
    edit(input, '12 500', 2)
    expect(input).toHaveValue('12 500')
    expect(input.selectionStart).toBe(2)
    expect(input.selectionEnd).toBe(2)
  })

  it('keeps the caret after the typed digit when a separator is added', () => {
    mockFetch()
    s.render(123)
    const input = s.input() as HTMLInputElement
    edit(input, '1923', 2)
    expect(input).toHaveValue('1 923')
    expect(input.selectionStart).toBe(3)
  })

  it('pastes a grouped value normalized', () => {
    mockFetch()
    s.render()
    const input = s.input() as HTMLInputElement
    paste(input, '1,500,000')
    expect(input).toHaveValue('1 500 000')
  })

  it('pastes a value with a currency marker and submits the plain number', async () => {
    const fetchMock = mockFetch()
    s.render()
    const input = s.input() as HTMLInputElement
    paste(input, '1 500 ₸')
    expect(input).toHaveValue('1 500')
    expect(s.error()).toBeNull()
    s.submit()
    await waitFor(() => expect(writes(fetchMock)).toHaveLength(1))
    expect(JSON.parse(writes(fetchMock)[0][1]!.body as string).amount).toBe(1500)
  })

  it('rejects a typed currency marker on submit', async () => {
    mockFetch()
    s.render()
    const input = s.input() as HTMLInputElement
    typeKeys(input, '1500 ₸')
    expect(input.value).toContain('₸')
    s.submit()
    expect(await screen.findByText('Amount must be a whole number greater than 0.')).toBeInTheDocument()
  })

  it('a paste with no change event does not make the next keystroke strict', () => {
    vi.useFakeTimers()
    try {
      mockFetch()
      s.render()
      const input = s.input() as HTMLInputElement
      fireEvent.paste(input)
      act(() => {
        vi.runAllTimers()
      })
      typeKeys(input, '1500')
      expect(input).toHaveValue('1 500')
    } finally {
      vi.useRealTimers()
    }
  })

  it('leaves a pasted decimal as pasted and shows the validator error on submit', async () => {
    const fetchMock = mockFetch()
    s.render()
    const input = s.input() as HTMLInputElement
    paste(input, '1500.5')
    expect(input).toHaveValue('1500.5')
    s.submit()
    expect(await screen.findByText('Amount must be a whole number greater than 0.')).toBeInTheDocument()
    expect(writes(fetchMock)).toHaveLength(0)
  })

  it('keeps a hand-typed comma and rejects it on submit', async () => {
    mockFetch()
    s.render()
    const input = s.input() as HTMLInputElement
    typeKeys(input, '1,5')
    expect(input).toHaveValue('1,5')
    s.submit()
    expect(await screen.findByText('Amount must be a whole number greater than 0.')).toBeInTheDocument()
  })

  it('Backspace after a separator removes the digit before it', () => {
    mockFetch()
    s.render(1500)
    const input = s.input() as HTMLInputElement
    edit(input, '1500', 1, 'deleteContentBackward')
    expect(input).toHaveValue('500')
    expect(input.selectionStart).toBe(0)
  })
})
