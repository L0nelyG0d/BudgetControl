import { formatMoney } from './format'

/**
 * Largest allowed expense or budget amount in whole tenge.
 * Keep in sync with `maxAmount` in backend/api/expenses.go.
 */
export const MAX_AMOUNT = 1_000_000_000_000

const WHOLE_MESSAGE = 'Amount must be a whole number greater than 0.'

// Plain digits, or strictly grouped thousands: a first group of 1-3 digits not
// starting with 0, then groups of exactly 3, joined by a space, NBSP (U+00A0),
// narrow NBSP (U+202F) or comma. Strict so a ru-KZ decimal comma is never
// read as a thousands separator.
const PLAIN = /^\d+$/
const GROUPED = /^[1-9]\d{0,2}(?:[ \u00A0\u202F,]\d{3})+$/

/** Trims and removes thousands separators. Returns null if the format is invalid. */
function cleanAmount(raw: string): string | null {
  const value = raw.trim()
  if (PLAIN.test(value)) return value
  if (GROUPED.test(value)) return value.replace(/[ \u00A0\u202F,]/g, '')
  return null
}

/**
 * Shared amount check for the expense form and the budget inputs.
 * Returns an error message, or null when the amount is a whole number
 * from 1 to MAX_AMOUNT. Surrounding whitespace is ignored, thousands separators are accepted; leading zeros are fine.
 */
export function validateAmount(raw: string): string | null {
  if (raw.trim() === '') return 'Enter an amount.'
  const value = cleanAmount(raw)
  if (value === null) return WHOLE_MESSAGE
  // Strip leading zeros and compare by length first, so very long digit
  // strings never go through Number() and lose precision.
  const digits = value.replace(/^0+/, '')
  if (digits === '') return WHOLE_MESSAGE
  const max = String(MAX_AMOUNT)
  if (digits.length > max.length || (digits.length === max.length && digits > max)) {
    return `Amount must be at most ${formatMoney(MAX_AMOUNT)}.`
  }
  return null
}

/** Converts an amount that passed validateAmount to a number. */
export function parseAmount(raw: string): number {
  return Number(cleanAmount(raw) ?? raw.trim())
}
