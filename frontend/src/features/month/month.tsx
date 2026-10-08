import { createContext, useContext, useState, type ReactNode } from 'react'
import { currentMonth } from '@/features/dashboard/summary'

export const MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]

export const MIN_MONTH = '0001-01'
export const MAX_MONTH = '9999-12'

/** Move a YYYY-MM month by delta months, clamped to years 1..9999. */
export function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  const index = Math.min(Math.max(y * 12 + (m - 1) + delta, 12), 9999 * 12 + 11)
  const year = Math.floor(index / 12)
  return `${String(year).padStart(4, '0')}-${String((index % 12) + 1).padStart(2, '0')}`
}

/** "2026-10" -> "October 2026". */
export function monthLabel(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return `${MONTH_NAMES[m - 1]} ${y}`
}

type MonthState = [string, (month: string) => void]

const MonthContext = createContext<MonthState | null>(null)

/** Keeps the selected month while the user moves between pages. */
export function MonthProvider({ children }: { children: ReactNode }) {
  const value = useState(() => currentMonth())
  return <MonthContext.Provider value={value}>{children}</MonthContext.Provider>
}

/** Selected month; falls back to page-local state without a provider. */
export function useMonth(): MonthState {
  const shared = useContext(MonthContext)
  const local = useState(() => currentMonth())
  return shared ?? local
}
