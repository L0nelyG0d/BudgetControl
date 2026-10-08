import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/utils'

const links = [
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/expenses', label: 'Expenses' },
  { to: '/categories', label: 'Categories' },
  { to: '/budgets', label: 'Budgets' },
]

/** Authenticated shell: nav bar plus the page content. */
export function Layout() {
  const { setUser } = useAuth()
  const navigate = useNavigate()
  const [loggingOut, setLoggingOut] = useState(false)
  const [logoutError, setLogoutError] = useState<string | null>(null)

  async function logout() {
    setLoggingOut(true)
    setLogoutError(null)
    try {
      await api('/api/auth/logout', { method: 'POST' })
      setUser(null)
      navigate('/login', { replace: true })
    } catch {
      setLogoutError('Could not log out. Check your connection and try again.')
      setLoggingOut(false)
    }
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border bg-card">
        <nav
          aria-label="Main"
          className="mx-auto flex max-w-4xl items-center gap-1 overflow-x-auto px-4 py-2"
        >
          {links.map((l) => (
            <NavLink
              key={l.to}
              to={l.to}
              className={({ isActive }) =>
                cn(
                  'rounded-lg px-3 py-2 text-sm whitespace-nowrap focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
                  isActive
                    ? 'bg-primary font-semibold text-primary-foreground'
                    : 'text-muted-foreground hover:text-foreground',
                )
              }
            >
              {l.label}
            </NavLink>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="ml-auto"
            onClick={() => void logout()}
            disabled={loggingOut}
          >
            Log out
          </Button>
        </nav>
        {logoutError && (
          <p role="alert" className="mx-auto max-w-4xl px-4 pb-2 text-sm text-destructive">
            {logoutError}
          </p>
        )}
      </header>
      <main className="mx-auto max-w-4xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
