import { useLayoutEffect, useRef, useState, type ChangeEvent, type ClipboardEvent } from 'react'

/** Grouping character shown in amount fields: a regular space (U+0020). */
export const GROUP_SEPARATOR = ' '

// Same strict shapes the validator accepts (see amount.ts), kept in sync by tests.
const PLAIN = /^\d+$/
const GROUPED = /^[1-9]\d{0,2}(?:[   ,]\d{3})+$/
const TYPED = /^[\d ]*$/

// Currency marker accepted on PASTE only (task 35). The marker list (₸, KZT, тг and
// "тг." with one trailing dot), case-insensitivity, prefix or suffix, optional
// space/NBSP/narrow NBSP between marker and number, and "one marker only" are PM
// defaults, NOT yet confirmed by the user. validateAmount/parseAmount stay strict.
const MARKER = '(?:₸|KZT|тг\\.?)'
const SEP = '[ \\u00a0\\u202f]*'
const PREFIXED = new RegExp(`^${MARKER}${SEP}(.+)$`, 'iu')
const SUFFIXED = new RegExp(`^(.+?)${SEP}${MARKER}$`, 'iu')

/** Removes one currency marker around a strict number; otherwise returns null. */
function stripMarker(trimmed: string): string | null {
  const m = PREFIXED.exec(trimmed) ?? SUFFIXED.exec(trimmed)
  if (!m) return null
  const number = m[1].trim()
  return PLAIN.test(number) || GROUPED.test(number) ? number : null
}

export type FormatInput = {
  /** The field value before the change. */
  prev: string
  /** The field value after the browser applied the change. */
  raw: string
  /** Caret position in `raw` after the change. */
  caret: number
  /** The DOM `inputType` of the change, e.g. `deleteContentBackward`. */
  inputType?: string
  /** True when the change came from a paste (separators in pasted text are normalized). */
  pasted?: boolean
}

export type FormatResult = { value: string; caret: number }

function group(digits: string): string {
  let out = ''
  for (let i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 === 0) out += GROUP_SEPARATOR
    out += digits[i]
  }
  return out
}

/** Groups a whole-number amount for display, e.g. 1500000 -> "1 500 000". */
export function formatAmountValue(amount: number | string): string {
  const digits = String(amount)
  return PLAIN.test(digits) ? group(digits.replace(/^0+(?=\d)/, '')) : digits
}

/**
 * Pure live formatter for an amount field. Digits are grouped by thousands with
 * a space and leading zeros are dropped. Anything that is not digits and spaces
 * is left exactly as typed (so the validator rejects it on submit), and a
 * hand-typed comma stays a comma. A paste is normalized only when the whole
 * value is a plain or strictly grouped number (space, NBSP, narrow NBSP or comma).
 * The caret keeps its place relative to the digits.
 */
export function formatAmountInput({ prev, raw, caret, inputType, pasted }: FormatInput): FormatResult {
  let text = raw
  let at = caret

  // Backspace/Delete that only removed a separator also removes the neighbouring digit.
  if (text.length === prev.length - 1) {
    if (inputType === 'deleteContentBackward' && prev[at] === GROUP_SEPARATOR && at > 0) {
      text = text.slice(0, at - 1) + text.slice(at)
      at -= 1
    } else if (inputType === 'deleteContentForward' && prev[at] === GROUP_SEPARATOR) {
      text = text.slice(0, at) + text.slice(at + 1)
    }
  }

  let trimmed = text.trim()
  const isPaste = !!pasted || (inputType?.startsWith('insertFromPaste') ?? false)
  const bare = isPaste ? stripMarker(trimmed) : null
  if (bare !== null) {
    // Pasted "1 500 ₸": format the bare number and put the caret after the last digit.
    text = trimmed = bare
    at = bare.length
  }
  const strict = PLAIN.test(trimmed) || GROUPED.test(trimmed)
  // Real keystrokes carry an inputType; pastes and programmatic changes are held to the strict shapes.
  const keystroke = !pasted && inputType !== undefined && !inputType.startsWith('insertFromPaste')
  const formattable = keystroke ? TYPED.test(text) : strict
  if (!formattable) return { value: raw, caret }

  const before = countDigits(text.slice(0, at))
  const digits = text.replace(/\D/g, '')
  const stripped = digits.replace(/^0+(?=\d)/, '')
  const dropped = digits.length - stripped.length
  const n = Math.max(0, before - dropped)
  const value = group(stripped)
  return { value, caret: caretAfterDigits(value, n) }
}

function countDigits(s: string): number {
  return s.replace(/\D/g, '').length
}

function caretAfterDigits(value: string, n: number): number {
  if (n <= 0) return 0
  let seen = 0
  for (let i = 0; i < value.length; i++) {
    if (/\d/.test(value[i])) seen++
    if (seen === n) return i + 1
  }
  return value.length
}

/**
 * Wires a controlled text input to the live formatter. Spread the result on the
 * `Input`; it handles `onChange`, `onPaste`, and restores the caret after React
 * re-renders. `onEdit` runs after every edit (e.g. to clear an error).
 */
export function useAmountInput(setValue: (value: string) => void, prevValue: string, onEdit?: () => void) {
  const ref = useRef<HTMLInputElement>(null)
  const pending = useRef<number | null>(null)
  const pasted = useRef(false)
  const [, bump] = useState(0)

  useLayoutEffect(() => {
    const el = ref.current
    if (pending.current !== null && el) {
      el.setSelectionRange(pending.current, pending.current)
    }
    pending.current = null
  })

  return {
    ref,
    onPaste: (_e: ClipboardEvent<HTMLInputElement>) => {
      pasted.current = true
      // The change event follows in the same task; clear the flag even if none comes.
      setTimeout(() => {
        pasted.current = false
      }, 0)
    },
    onChange: (e: ChangeEvent<HTMLInputElement>) => {
      const native = e.nativeEvent as InputEvent
      const result = formatAmountInput({
        prev: prevValue,
        raw: e.target.value,
        caret: e.target.selectionStart ?? e.target.value.length,
        inputType: native.inputType,
        pasted: pasted.current,
      })
      pasted.current = false
      pending.current = result.caret
      setValue(result.value)
      onEdit?.()
      bump((n) => n + 1)
    },
  }
}
