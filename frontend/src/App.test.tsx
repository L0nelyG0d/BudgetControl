import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

function mockFetch(impl: () => Promise<Response>) {
  const fn = vi.fn(impl)
  vi.stubGlobal('fetch', fn)
  return fn
}

const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status }))

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('App routing', () => {
  it('shows a loading state with no protected content while pending', () => {
    mockFetch(() => new Promise(() => {}))
    renderAt('/expenses')
    expect(screen.getByRole('status')).toHaveTextContent('Loading')
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })

  it('renders the page and nav for a logged-in user, marking the current page', async () => {
    const fetchMock = mockFetch(() => json(200, { id: 1, email: 'a@b.c' }))
    renderAt('/expenses')
    expect(await screen.findByRole('heading', { name: 'Expenses' })).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/auth/me',
      expect.objectContaining({ credentials: 'include' }),
    )
    // The guard asks /api/auth/me once; the page itself may fetch its own data.
    expect(fetchMock.mock.calls.filter((call: unknown[]) => call[0] === '/api/auth/me')).toHaveLength(1)
    expect(screen.getByRole('link', { name: 'Expenses' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Budgets' })).not.toHaveAttribute('aria-current')
    for (const name of ['Dashboard', 'Categories', 'Budgets']) {
      expect(screen.getByRole('link', { name })).toBeInTheDocument()
    }
  })

  it('redirects / to the dashboard', async () => {
    mockFetch(() => json(200, { id: 1, email: 'a@b.c' }))
    renderAt('/')
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
  })

  it('redirects a logged-out visitor to /login without a nav bar', async () => {
    mockFetch(() => json(401, { error: 'unauthorized' }))
    renderAt('/budgets')
    expect(await screen.findByRole('heading', { name: 'Log in' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })

  it.each(['/login', '/register'])('redirects a logged-in user from %s to the dashboard', async (path) => {
    mockFetch(() => json(200, { id: 1, email: 'a@b.c' }))
    renderAt(path)
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
  })

  it('shows an error, not a redirect, on a server error', async () => {
    const fetchMock = mockFetch(() => json(500, { error: 'boom' }))
    renderAt('/dashboard')
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not check your session')
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('shows an error on a network failure', async () => {
    mockFetch(() => Promise.reject(new TypeError('Failed to fetch')))
    renderAt('/dashboard')
    expect(await screen.findByRole('alert')).toBeInTheDocument()
  })

  it('shows page not found for unknown paths', async () => {
    mockFetch(() => json(401, {}))
    renderAt('/nope')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })
})
