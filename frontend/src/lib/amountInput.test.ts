import { describe, expect, it } from 'vitest'
import { formatAmountInput, formatAmountValue } from './amountInput'
import { validateAmount } from './amount'

const type = (prev: string, raw: string, caret: number, extra = {}) =>
  formatAmountInput({ prev, raw, caret, inputType: 'insertText', ...extra })

describe('formatAmountValue', () => {
  it.each([
    ['1', '1'],
    ['12', '12'],
    ['123', '123'],
    ['1234', '1 234'],
    ['12345', '12 345'],
    ['123456', '123 456'],
    ['1234567', '1 234 567'],
    ['12345678', '12 345 678'],
    ['123456789', '123 456 789'],
    ['1234567890', '1 234 567 890'],
    ['12345678901', '12 345 678 901'],
    ['123456789012', '123 456 789 012'],
    ['1000000000000', '1 000 000 000 000'],
  ])('groups %s as %s', (digits, grouped) => {
    expect(formatAmountValue(digits)).toBe(grouped)
    expect(formatAmountValue(Number(digits))).toBe(grouped)
    expect(validateAmount(grouped, null)).toBeNull()
  })
})

describe('formatAmountInput value', () => {
  it('groups digits as they are typed one at a time', () => {
    let value = ''
    const shown: string[] = []
    for (const ch of '1500000') {
      value = type(value, value + ch, value.length + 1).value
      shown.push(value)
    }
    expect(shown).toEqual(['1', '15', '150', '1 500', '15 000', '150 000', '1 500 000'])
  })

  it('keeps an empty field empty', () => {
    expect(type('1', '', 0, { inputType: 'deleteContentBackward' })).toEqual({ value: '', caret: 0 })
  })

  it('regroups after deleting at the end', () => {
    expect(type('1 500', '1 50', 4, { inputType: 'deleteContentBackward' }).value).toBe('150')
  })

  it.each([
    ['05', '5'],
    ['0', '0'],
    ['00', '0'],
    ['0123', '123'],
  ])('drops leading zeros: %s -> %s', (raw, value) => {
    expect(type('', raw, raw.length).value).toBe(value)
  })

  it.each(['12a', '1500.5', '1,5', '1,500', '₸5', '-5', '1 500 ₸'])('leaves %s untouched when typed', (raw) => {
    expect(type('', raw, raw.length)).toEqual({ value: raw, caret: raw.length })
  })

  it.each([
    ['1500000', '1 500 000'],
    ['1 500', '1 500'],
    ['1 500', '1 500'],
    ['1 500', '1 500'],
    ['1,500', '1 500'],
    ['  1,500,000  ', '1 500 000'],
  ])('normalizes pasted %j to %j', (raw, value) => {
    expect(formatAmountInput({ prev: '', raw, caret: raw.length, inputType: 'insertFromPaste', pasted: true }).value).toBe(value)
  })

  it.each(['1500.5', '1,5', '1,00', '1.500', '12 34', 'abc', '1  000'])(
    'leaves pasted %j as pasted',
    (raw) => {
      expect(formatAmountInput({ prev: '', raw, caret: raw.length, pasted: true })).toEqual({ value: raw, caret: raw.length })
    },
  )

  it('treats a change without an inputType like a paste (strict)', () => {
    expect(formatAmountInput({ prev: '', raw: '12 34', caret: 5 }).value).toBe('12 34')
    expect(formatAmountInput({ prev: '', raw: '1500', caret: 4 }).value).toBe('1 500')
  })

  it('always produces a strictly grouped string for digits', () => {
    for (let n = 1; n <= 13; n++) {
      const digits = '1234567890123'.slice(0, n)
      expect(validateAmount(type('', digits, n).value, null)).toBeNull()
    }
  })
})

describe('formatAmountInput caret', () => {
  it('insert in the middle keeps the caret after the typed digit', () => {
    // "1 500", caret after "1", type "2" -> raw "12 500", caret 2
    expect(type('1 500', '12 500', 2)).toEqual({ value: '12 500', caret: 2 })
  })

  it('insert that adds a separator at the end puts the caret at the end', () => {
    expect(type('999', '9999', 4)).toEqual({ value: '9 999', caret: 5 })
  })

  it('insert in the middle that shifts a separator keeps the caret after the typed digit', () => {
    // "1 234", caret after "1", type "9" -> raw "19 234" -> "19 234" (caret 2)
    expect(type('1 234', '19 234', 2)).toEqual({ value: '19 234', caret: 2 })
    // "123", caret after "1", type "9" -> raw "1923" -> "1 923", caret after the 9
    expect(type('123', '1923', 2)).toEqual({ value: '1 923', caret: 3 })
  })

  it('Backspace right after a separator removes the digit before it', () => {
    // "1 |500" Backspace removes the space: raw "1500", caret 1
    expect(type('1 500', '1500', 1, { inputType: 'deleteContentBackward' })).toEqual({ value: '500', caret: 0 })
  })

  it('Backspace after a separator in a longer value keeps the caret by the digits', () => {
    // "12 |345" -> removes "2"
    expect(type('12 345', '12345', 2, { inputType: 'deleteContentBackward' })).toEqual({ value: '1 345', caret: 1 })
  })

  it('Delete right before a separator removes the digit after it', () => {
    // "1| 500" Delete removes the space: raw "1500", caret 1
    expect(type('1 500', '1500', 1, { inputType: 'deleteContentForward' })).toEqual({ value: '100', caret: 1 })
  })

  it('a replaced selection is regrouped with the caret after the inserted text', () => {
    // "1 500 000" with "500 0" selected, type "7" -> raw "1 700", caret 3
    expect(type('1 500 000', '1 700', 3)).toEqual({ value: '1 700', caret: 3 })
    // select all and type "5"
    expect(type('1 500', '5', 1)).toEqual({ value: '5', caret: 1 })
  })

  it('dropping a leading zero moves the caret with the digits', () => {
    expect(type('0', '05', 2)).toEqual({ value: '5', caret: 1 })
  })

  it('a deleted selection that leaves a gap regroups', () => {
    expect(type('1 234 567', '1 267', 2, { inputType: 'deleteContentBackward' })).toEqual({ value: '1 267', caret: 1 })
  })
})

// Currency marker on paste (task 35). Marker list, case, prefix/suffix, spacing and
// paste-only are PM defaults, not yet confirmed by the user.
describe('pasted currency marker', () => {
  const pasteOf = (raw: string) =>
    formatAmountInput({ prev: '', raw, caret: raw.length, inputType: 'insertFromPaste', pasted: true })

  it.each([
    ['1 500 ₸', '1 500'],
    ['1500 ₸', '1 500'],
    ['1500₸', '1 500'],
    ['₸1500', '1 500'],
    ['₸ 1 500', '1 500'],
    ['KZT 1,500', '1 500'],
    ['тг 1500', '1 500'],
    ['1500 тг', '1 500'],
    ['1500 KZT', '1 500'],
    ['1500тг.', '1 500'],
    ['тг. 1500', '1 500'],
    ['kzt 1500', '1 500'],
    ['1500 Kzt', '1 500'],
    ['1500 ТГ', '1 500'],
    ['1500 Тг.', '1 500'],
    ['1 500\u00a0₸', '1 500'],
    ['1 500\u202f₸', '1 500'],
    ['₸\u00a01 500', '1 500'],
    ['  1 500 ₸  ', '1 500'],
    ['1 500 000 ₸', '1 500 000'],
    ['₸ 0015', '15'],
  ])('strips the marker from pasted %j -> %j', (raw, value) => {
    const r = pasteOf(raw)
    expect(r.value).toBe(value)
    expect(r.caret).toBe(value.length)
    expect(validateAmount(r.value, null)).toBeNull()
  })

  it('gives the same result as the bare number', () => {
    expect(pasteOf('₸ 1500000')).toEqual(pasteOf('1500000'))
  })

  it.each([
    '1 500,50 ₸', '1500.5 ₸', '12 34 ₸', '1,5 ₸',
    '₸1500₸', '₸ 1500 KZT', '₸', 'KZT', 'тг.', '1₸500',
    'тенге', 'tg', '$1500', '1500 тенге', '1500 тг..',
  ])('leaves pasted %j as pasted', (raw) => {
    expect(pasteOf(raw)).toEqual({ value: raw, caret: raw.length })
  })

  it.each(['1 500 ₸', '₸1500', 'KZT 1500'])('does not strip a typed %j', (raw) => {
    expect(type('', raw, raw.length)).toEqual({ value: raw, caret: raw.length })
  })

  it('formatAmountValue and validateAmount stay strict', () => {
    expect(validateAmount('1 500 ₸', null)).not.toBeNull()
    expect(formatAmountValue('1 500 ₸')).toBe('1 500 ₸')
  })
})
