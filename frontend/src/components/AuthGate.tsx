import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '@/lib/auth'

function Loading() {
  return (
    <div
      role="status"
      className="flex min-h-screen items-center justify-center text-muted-foreground"
    >
      Loading...
    </div>
  )
}

function SessionError() {
  const auth = useAuth()
  if (auth.status !== 'error') return null
  return (
    <div className="mx-auto flex min-h-screen max-w-4xl flex-col items-center justify-center gap-4 px-4">
      <p role="alert" className="text-destructive">
        {auth.message}
      </p>
      <button
        type="button"
        onClick={() => void auth.refresh()}
        className="rounded-lg border border-border bg-card px-4 py-2 text-sm font-semibold focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        Try again
      </button>
    </div>
  )
}

/** Layout route: renders children only for a logged-in user, else /login. */
export function RequireAuth() {
  const auth = useAuth()
  if (auth.status === 'loading') return <Loading />
  if (auth.status === 'error') return <SessionError />
  if (auth.status === 'unauthenticated') return <Navigate to="/login" replace />
  return <Outlet />
}

/** Layout route for /login and /register: logged-in users go to /dashboard. */
export function PublicOnly() {
  const auth = useAuth()
  if (auth.status === 'loading') return <Loading />
  if (auth.status === 'error') return <SessionError />
  if (auth.status === 'authenticated') return <Navigate to="/dashboard" replace />
  return <Outlet />
}
