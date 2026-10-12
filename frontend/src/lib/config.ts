import { useSyncExternalStore } from 'react'
import { api } from './api'

// The amount limit is owned by the backend (GET /config). It is requested at
// most once per page load and kept in memory only. `null` means unknown:
// still loading, or the request failed. Callers then skip the client-side
// cap check and the backend's 400 message is shown as is.
let maxAmount: number | null = null
let started = false
const listeners = new Set<() => void>()

function set(value: number | null) {
  maxAmount = value
  listeners.forEach((l) => l())
}

/** Starts the one-time config request if it has not started yet. */
export function loadConfig(): void {
  if (started) return
  started = true
  api<{ maxAmount?: unknown }>('/api/config')
    .then((data) => {
      const v = data?.maxAmount
      if (typeof v === 'number' && Number.isSafeInteger(v) && v > 0) set(v)
    })
    .catch(() => {})
}

/** The loaded amount limit in whole tenge, or null while unknown. */
export function useMaxAmount(): number | null {
  loadConfig()
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    () => maxAmount,
  )
}

/** Forgets the loaded config so the next use requests it again. For tests. */
export function resetConfigForTests(): void {
  started = false
  maxAmount = null
}
