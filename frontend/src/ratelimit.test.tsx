import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

const json = (status: number, body: unknown, headers?: HeadersInit) =>
  new Response(JSON.stringify(body), { status, headers })

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('login rate limit', () => {
  it('shows a too many attempts message on 429', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url === '/api/auth/me') return json(401, { error: 'no' })
        if (url === '/api/auth/login') {
          return json(429, { error: 'too many login attempts, try again later' }, { 'Retry-After': '900' })
        }
        return json(404, {})
      }),
    )
    render(
      <MemoryRouter initialEntries={['/login']}>
        <App />
      </MemoryRouter>,
    )
    fireEvent.change(await screen.findByLabelText('Email'), { target: { value: 'a@b.co' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'password1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Too many attempts. Try again in a few minutes.',
    )
    expect(screen.getByRole('button', { name: 'Log in' })).toBeEnabled()
  })
})
