import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api, ApiError } from '@/lib/api'
import { useAuth, type User } from '@/lib/auth'

const MIN_PASSWORD_LENGTH = 8

function messageFor(err: unknown, mode: 'login' | 'register'): string {
  if (err instanceof ApiError) {
    if (err.status === 429) return 'Too many attempts. Try again in a few minutes.'
    if (err.status === 401) return 'Wrong email or password. Check them and try again.'
    if (err.status === 409) return 'This email is already registered. Try logging in instead.'
    if (err.status === 400) return err.message
    return 'Something went wrong on our side. Please try again.'
  }
  return mode === 'login'
    ? 'Could not log in. Check your connection and try again.'
    : 'Could not create your account. Check your connection and try again.'
}

/** Email + password form shared by the login and register pages. */
export function AuthForm({ mode }: { mode: 'login' | 'register' }) {
  const isRegister = mode === 'register'
  const { setUser } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [passwordError, setPasswordError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (pending) return
    setFormError(null)
    if (isRegister && password.length < MIN_PASSWORD_LENGTH) {
      setPasswordError('Password must be at least 8 characters.')
      return
    }
    setPasswordError(null)
    setPending(true)
    try {
      const credentials = { email, password }
      if (isRegister) {
        // Registering sets no cookie, so log in right after.
        await api('/api/auth/register', { method: 'POST', body: credentials })
      }
      const user = await api<User>('/api/auth/login', { method: 'POST', body: credentials })
      setUser(user)
      navigate('/dashboard', { replace: true })
    } catch (err) {
      setFormError(messageFor(err, mode))
      setPending(false)
    }
  }

  return (
    <main className="mx-auto max-w-sm px-4 py-12">
      <form
        onSubmit={onSubmit}
        noValidate
        className="flex flex-col gap-4 rounded-xl border border-border bg-card p-6"
      >
        <h1 className="text-2xl font-semibold">{isRegister ? 'Create account' : 'Log in'}</h1>
        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            autoComplete={isRegister ? 'new-password' : 'current-password'}
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-invalid={passwordError ? true : undefined}
            aria-describedby={passwordError ? 'password-error' : undefined}
          />
          {passwordError && (
            <p id="password-error" className="text-xs text-destructive">
              {passwordError}
            </p>
          )}
        </div>
        {formError && (
          <p role="alert" className="text-sm text-destructive">
            {formError}
          </p>
        )}
        <Button type="submit" disabled={pending}>
          {isRegister ? 'Create account' : 'Log in'}
        </Button>
        <p className="text-sm text-muted-foreground">
          {isRegister ? 'Already have an account? ' : 'No account yet? '}
          <Link
            to={isRegister ? '/login' : '/register'}
            className="text-primary underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            {isRegister ? 'Log in' : 'Create account'}
          </Link>
        </p>
      </form>
    </main>
  )
}
