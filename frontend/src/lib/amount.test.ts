import { describe, expect, it } from 'vitest'
import { MAX_AMOUNT, parseAmount, validateAmount } from './amount'
import { formatMoney } from './format'

const capMessage = `Amount must be at most ${formatMoney(MAX_AMOUNT)}.`
const wholeMessage = 'Amount must be a whole number greater than 0.'

describe('validateAmount', () => {
  it.each([
    ['1000000000001', capMessage],
    ['12345678901234567890', capMessage],
    ['00000000000000000000001000000000001', capMessage],
    ['1000000000000', null],
    [' 1000000000000 ', null],
    ['007', null],
    ['1', null],
    ['', 'Enter an amount.'],
    ['   ', 'Enter an amount.'],
    ['0', wholeMessage],
    ['000', wholeMessage],
    ['-5', wholeMessage],
    ['-1000000000001', wholeMessage],
    ['1500.5', wholeMessage],
    ['1500,5', wholeMessage],
    ['abc', wholeMessage],
    ['1e3', wholeMessage],
  ])('%j -> %j', (raw, want) => {
    expect(validateAmount(raw)).toBe(want)
  })

  it('parses with leading zeros and spaces', () => {
    expect(parseAmount(' 007 ')).toBe(7)
  })
})
