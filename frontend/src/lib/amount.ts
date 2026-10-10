import { formatMoney } from './format'

/**
 * Largest allowed expense or budget amount in whole tenge.
 * Keep in sync with `maxAmount` in backend/api/expenses.go.
 */
export const MAX_AMOUNT = 1_000_000_000_000

/**
 * Shared amount check for the expense form and the budget inputs.
 * Returns an error message, or null when the amount is a whole number
 * from 1 to MAX_AMOUNT. Surrounding spaces are ignored; leading zeros are fine.
 */
export function validateAmount(raw: string): string | null {
  const value = raw.trim()
  if (value === '') return 'Enter an amount.'
  if (!/^\d+$/.test(value)) return 'Amount must be a whole number greater than 0.'
  // Strip leading zeros and compare by length first, so very long digit
  // strings never go through Number() and lose precision.
  const digits = value.replace(/^0+/, '')
  if (digits === '') return 'Amount must be a whole number greater than 0.'
  const max = String(MAX_AMOUNT)
  if (digits.length > max.length || (digits.length === max.length && digits > max)) {
    return `Amount must be at most ${formatMoney(MAX_AMOUNT)}.`
  }
  return null
}

/** Converts an amount that passed validateAmount to a number. */
export function parseAmount(raw: string): number {
  return Number(raw.trim())
}
