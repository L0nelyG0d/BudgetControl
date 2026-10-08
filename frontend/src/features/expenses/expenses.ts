export type Expense = {
  id: number | string
  category_id: number | string
  amount: number
  date: string // YYYY-MM-DD
  note: string | null
  created_at: string
}

export type Category = {
  id: number | string
  name: string
  color: string
  is_default: boolean
}

/** Newest first: by date, then by creation time, then by id. */
export function sortExpenses(list: Expense[]): Expense[] {
  return [...list].sort((a, b) => {
    if (a.date !== b.date) return a.date < b.date ? 1 : -1
    if (a.created_at !== b.created_at) return a.created_at < b.created_at ? 1 : -1
    return Number(b.id) - Number(a.id)
  })
}

/** "2026-10-08" -> "08.10.2026". */
export function formatDate(iso: string): string {
  const [y, m, d] = iso.slice(0, 10).split('-')
  return `${d}.${m}.${y}`
}

/** Today's local date as YYYY-MM-DD. */
export function todayISO(): string {
  const now = new Date()
  const mm = String(now.getMonth() + 1).padStart(2, '0')
  const dd = String(now.getDate()).padStart(2, '0')
  return `${now.getFullYear()}-${mm}-${dd}`
}

/** Returns an error message, or null when the amount is a positive whole number. */
export function validateAmount(raw: string): string | null {
  const value = raw.trim()
  if (value === '') return 'Enter an amount.'
  if (!/^\d+$/.test(value) || Number(value) < 1) {
    return 'Amount must be a whole number greater than 0.'
  }
  if (!Number.isSafeInteger(Number(value))) return 'Amount is too large.'
  return null
}
