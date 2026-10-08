import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response>

function mockApi(handler: Handler) {
  const fn = vi.fn(async (url: string, init?: RequestInit) => handler(url, init))
  vi.stubGlobal('fetch', fn)
  return fn
}

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })
const user = { id: 1, email: 'a@b.co' }

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
}

async function fill(email: string, password: string) {
  fireEvent.change(await screen.findByLabelText('Email'), { target: { value: email } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: password } })
}

const callsTo = (fn: ReturnType<typeof mockApi>, path: string) =>
  fn.mock.calls.filter(([u]) => u === path)

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
  sessionStorage.clear()
})

describe('login', () => {
  it('logs in and goes to the dashboard', async () => {
    let loggedIn = false
    const fetchMock = mockApi((url) => {
      if (url === '/api/auth/me') return loggedIn ? json(200, user) : json(401, { error: 'no' })
      if (url === '/api/auth/login') {
        loggedIn = true
        return json(200, user)
      }
      return json(404, {})
    })
    renderAt('/login')
    await fill('a@b.co', 'password1')
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    const [, init] = callsTo(fetchMock, '/api/auth/login')[0]
    expect(init).toMatchObject({ method: 'POST', credentials: 'include' })
    expect(JSON.parse(init!.body as string)).toEqual({ email: 'a@b.co', password: 'password1' })
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
  })

  it('shows an inline message for a wrong password and re-enables the button', async () => {
    mockApi((url) =>
      url === '/api/auth/me' ? json(401, {}) : json(401, { error: 'invalid credentials' }),
    )
    renderAt('/login')
    await fill('a@b.co', 'wrongpass1')
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Wrong email or password')
    expect(screen.getByRole('button', { name: 'Log in' })).toBeEnabled()
  })

  it('disables the button while the request is pending', async () => {
    mockApi((url) => (url === '/api/auth/me' ? json(401, {}) : new Promise<Response>(() => {})))
    renderAt('/login')
    await fill('a@b.co', 'password1')
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Log in' })).toBeDisabled())
  })

  it('shows a generic message on a network error', async () => {
    mockApi((url) => {
      if (url === '/api/auth/me') return json(401, {})
      throw new TypeError('Failed to fetch')
    })
    renderAt('/login')
    await fill('a@b.co', 'password1')
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('try again')
  })

  it('links to register', async () => {
    mockApi(() => json(401, {}))
    renderAt('/login')
    expect(await screen.findByRole('link', { name: 'Create account' })).toHaveAttribute(
      'href',
      '/register',
    )
  })
})

describe('register', () => {
  it('registers, then logs in with the same credentials', async () => {
    let loggedIn = false
    const fetchMock = mockApi((url) => {
      if (url === '/api/auth/me') return loggedIn ? json(200, user) : json(401, {})
      if (url === '/api/auth/register') return json(201, user)
      if (url === '/api/auth/login') {
        loggedIn = true
        return json(200, user)
      }
      return json(404, {})
    })
    renderAt('/register')
    await fill('a@b.co', 'password1')
    fireEvent.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    const paths = fetchMock.mock.calls.map(([u]) => u)
    expect(paths.indexOf('/api/auth/register')).toBeLessThan(paths.indexOf('/api/auth/login'))
    for (const p of ['/api/auth/register', '/api/auth/login']) {
      const [, init] = callsTo(fetchMock, p)[0]
      expect(init).toMatchObject({ method: 'POST', credentials: 'include' })
      expect(JSON.parse(init!.body as string)).toEqual({ email: 'a@b.co', password: 'password1' })
    }
    expect(localStorage.length).toBe(0)
  })

  it('rejects a short password before sending a request', async () => {
    const fetchMock = mockApi(() => json(401, {}))
    renderAt('/register')
    await screen.findByLabelText('Email')
    await fill('a@b.co', 'short')
    fireEvent.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByText(/at least 8 characters/)).toBeInTheDocument()
    expect(callsTo(fetchMock, '/api/auth/register')).toHaveLength(0)
  })

  it('shows a message for a taken email', async () => {
    mockApi((url) =>
      url === '/api/auth/me' ? json(401, {}) : json(409, { error: 'email taken' }),
    )
    renderAt('/register')
    await fill('a@b.co', 'password1')
    fireEvent.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('This email is already registered')
  })

  it("shows the server's message for a 400", async () => {
    mockApi((url) =>
      url === '/api/auth/me' ? json(401, {}) : json(400, { error: 'email is invalid' }),
    )
    renderAt('/register')
    await fill('a@b.co', 'password1')
    fireEvent.click(screen.getByRole('button', { name: 'Create account' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('email is invalid')
  })

  it('links to login', async () => {
    mockApi(() => json(401, {}))
    renderAt('/register')
    expect(await screen.findByRole('link', { name: 'Log in' })).toHaveAttribute('href', '/login')
  })
})

describe('logout', () => {
  it('calls logout, goes to /login, and the dashboard is then guarded', async () => {
    let loggedIn = true
    const fetchMock = mockApi((url) => {
      if (url === '/api/auth/me') return loggedIn ? json(200, user) : json(401, {})
      if (url === '/api/auth/logout') {
        loggedIn = false
        return new Response(null, { status: 204 })
      }
      return json(404, {})
    })
    renderAt('/dashboard')
    fireEvent.click(await screen.findByRole('button', { name: 'Log out' }))
    expect(await screen.findByRole('heading', { name: 'Log in' })).toBeInTheDocument()
    const [, init] = callsTo(fetchMock, '/api/auth/logout')[0]
    expect(init).toMatchObject({ method: 'POST', credentials: 'include' })
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })
})
