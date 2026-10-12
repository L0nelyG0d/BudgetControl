import { useEffect, useSyncExternalStore } from 'react'
import { api } from './api'

// The amount limit is owned by the backend (GET /config). It is kept in
// memory only. `null` means unknown: still loading, or the request failed.
// Callers then skip the client-side cap check and the backend's 400 message
// is shown as is. A failed load is retried the next time a form mounts while
// the limit is unknown and no request is in flight; after a success it is
// never requested again.
let maxAmount: number | null = null
let inFlight = false
let generation = 0
const listeners = new Set<() => void>()

function set(value: number | null) {
  maxAmount = value
  listeners.forEach((l) => l())
}

/** Requests the config unless the limit is known or a request is in flight. */
export function loadConfig(): void {
  if (maxAmount !== null || inFlight) return
  inFlight = true
  const gen = generation
  api<{ maxAmount?: unknown }>('/api/config')
    .then((data) => {
      if (gen !== generation) return
      const v = data?.maxAmount
      if (typeof v === 'number' && Number.isSafeInteger(v) && v > 0) set(v)
    })
    .catch(() => {})
    .finally(() => {
      if (gen === generation) inFlight = false
    })
}

/** The loaded amount limit in whole tenge, or null while unknown. */
export function useMaxAmount(): number | null {
  // Only a mount starts a request; re-renders never do.
  useEffect(() => loadConfig(), [])
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
  generation++
  inFlight = false
  maxAmount = null
}
