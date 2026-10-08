import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { api, ApiError } from '@/lib/api'

export type User = { id: number | string; email: string }

export type AuthState =
  | { status: 'loading' }
  | { status: 'authenticated'; user: User }
  | { status: 'unauthenticated' }
  | { status: 'error'; message: string }

type AuthContextValue = AuthState & {
  /** Re-check the session with GET /api/auth/me. */
  refresh: () => Promise<void>
  /** Set state directly, e.g. after login (#10) or logout. */
  setUser: (user: User | null) => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  const refresh = useCallback(async () => {
    setState({ status: 'loading' })
    try {
      const user = await api<User>('/api/auth/me')
      setState({ status: 'authenticated', user })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState({ status: 'unauthenticated' })
      } else {
        setState({
          status: 'error',
          message: 'Could not check your session. Check your connection and try again.',
        })
      }
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const setUser = useCallback((user: User | null) => {
    setState(user ? { status: 'authenticated', user } : { status: 'unauthenticated' })
  }, [])

  const value = useMemo(() => ({ ...state, refresh, setUser }), [state, refresh, setUser])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
