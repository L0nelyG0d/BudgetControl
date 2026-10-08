import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import CategoriesPage from './CategoriesPage'

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status })

const base = [
  { id: 1, name: 'Food', color: '#16A34A', is_default: true },
  { id: 2, name: 'Other', color: '#64748B', is_default: true },
  { id: 10, name: 'Pets', color: '#F59E0B', is_default: false },
]

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response>
function mockApi(handler: Handler) {
  const fn = vi.fn(async (url: string, init?: RequestInit) => handler(url, init))
  vi.stubGlobal('fetch', fn)
  return fn
}
const callsWith = (fn: ReturnType<typeof mockApi>, method: string) =>
  fn.mock.calls.filter(([, init]) => (init?.method ?? 'GET') === method)

afterEach(() => vi.unstubAllGlobals())

describe('CategoriesPage', () => {
  it('lists default and custom categories, with edit/delete only on custom ones', async () => {
    mockApi(() => json(200, base))
    render(<CategoriesPage />)
    expect(await screen.findByText('Pets')).toBeInTheDocument()
    expect(screen.getAllByText('Default')).toHaveLength(2)
    expect(screen.getByText('#F59E0B')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Edit Pets' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Edit Food' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Delete Food' })).not.toBeInTheDocument()
  })

  it('shows an empty hint when there are no custom categories', async () => {
    mockApi(() => json(200, base.slice(0, 2)))
    render(<CategoriesPage />)
    expect(await screen.findByText(/no custom categories yet/i)).toBeInTheDocument()
  })

  it('shows an error with retry when loading fails', async () => {
    let fail = true
    mockApi(() => (fail ? json(500, { error: 'boom' }) : json(200, base)))
    render(<CategoriesPage />)
    expect(await screen.findByText(/could not load categories/i)).toBeInTheDocument()
    fail = false
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Pets')).toBeInTheDocument()
  })

  it('creates a category and adds it to the list', async () => {
    const fn = mockApi((_url, init) => {
      if (init?.method === 'POST') return json(201, { id: 11, name: 'Gifts', color: '#112233', is_default: false })
      return json(200, base)
    })
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Gifts' } })
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: '#112233' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add category' }))
    expect(await screen.findByText('Gifts')).toBeInTheDocument()
    const [url, init] = callsWith(fn, 'POST')[0]
    expect(url).toBe('/api/categories')
    expect(JSON.parse(init!.body as string)).toEqual({ name: 'Gifts', color: '#112233' })
    expect(screen.getByLabelText('Name')).toHaveValue('')
  })

  it('rejects an empty name inline without calling the API', async () => {
    const fn = mockApi(() => json(200, base))
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.click(screen.getByRole('button', { name: 'Add category' }))
    expect(screen.getByText('Enter a name for the category.')).toBeInTheDocument()
    expect(callsWith(fn, 'POST')).toHaveLength(0)
  })

  it('shows the duplicate message and keeps what the user typed', async () => {
    mockApi((_url, init) =>
      init?.method === 'POST' ? json(409, { error: 'duplicate' }) : json(200, base),
    )
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Pets' } })
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: '#112233' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add category' }))
    expect(await screen.findByText('A category with this name already exists')).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('Pets')
    expect(screen.getByLabelText('Color')).toHaveValue('#112233')
  })

  it('edits a custom category through PUT and updates the list', async () => {
    const fn = mockApi((_url, init) =>
      init?.method === 'PUT'
        ? json(200, { id: 10, name: 'Cats', color: '#AA0000', is_default: false })
        : json(200, base),
    )
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.click(screen.getByRole('button', { name: 'Edit Pets' }))
    expect(screen.getByLabelText('Name')).toHaveValue('Pets')
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Cats' } })
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: '#aa0000' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(await screen.findByText('Cats')).toBeInTheDocument()
    expect(screen.queryByText('Pets')).not.toBeInTheDocument()
    const [url, init] = callsWith(fn, 'PUT')[0]
    expect(url).toBe('/api/categories/10')
    expect(JSON.parse(init!.body as string)).toEqual({ name: 'Cats', color: '#AA0000' })
  })

  it('deletes after confirmation, mentioning the move to Other', async () => {
    const fn = mockApi((_url, init) => (init?.method === 'DELETE' ? json(204) : json(200, base)))
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.click(screen.getByRole('button', { name: 'Delete Pets' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/expenses in this category will move to other/i)).toBeInTheDocument()
    expect(callsWith(fn, 'DELETE')).toHaveLength(0)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete category' }))
    await waitFor(() => expect(screen.queryByText('Pets')).not.toBeInTheDocument())
    expect(callsWith(fn, 'DELETE')[0][0]).toBe('/api/categories/10')
  })

  it('keeps the category when delete is cancelled', async () => {
    const fn = mockApi(() => json(200, base))
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.click(screen.getByRole('button', { name: 'Delete Pets' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(screen.getByText('Pets')).toBeInTheDocument()
    expect(callsWith(fn, 'DELETE')).toHaveLength(0)
  })

  it('shows an error and keeps the list when delete fails', async () => {
    mockApi((_url, init) => (init?.method === 'DELETE' ? json(500, { error: 'x' }) : json(200, base)))
    render(<CategoriesPage />)
    await screen.findByText('Pets')
    fireEvent.click(screen.getByRole('button', { name: 'Delete Pets' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete category' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(/could not delete/i)
    expect(screen.getAllByText('Pets').length).toBeGreaterThan(0)
  })
})
