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
    ['1 500', null],
    ['1,500', null],
    ['1 000', null],
    ['12 345 678', null],
    ['1,234,567', null],
    ['1\u00A0500', null],
    ['1\u202F500', null],
    [' 1 500\n', null],
    ['\t1,500 ', null],
    ['1 000,500', null],
    ['1 000 000 000 000', null],
    ['1,000,000,000,000', null],
    ['1 000 000 000 001', capMessage],
    ['1,000,000,000,001', capMessage],
    ['1,00', wholeMessage],
    ['1,5', wholeMessage],
    ['1.500', wholeMessage],
    ['1,0000', wholeMessage],
    ['12 34', wholeMessage],
    ['1 5000', wholeMessage],
    [',500', wholeMessage],
    ['500,', wholeMessage],
    ['1,,000', wholeMessage],
    ['1  000', wholeMessage],
    ['1 , 000', wholeMessage],
    ['0,500', wholeMessage],
    ['0 500', wholeMessage],
    ['01,000', wholeMessage],
    ['0 000', wholeMessage],
    ['-1 500', wholeMessage],
    ['1 500 ₸', wholeMessage],
    ['1 500 abc', wholeMessage],
    ['1\t500', wholeMessage],
    ['1\n500', wholeMessage],
  ])('%j -> %j', (raw, want) => {
    expect(validateAmount(raw)).toBe(want)
  })

  it('parses with leading zeros and spaces', () => {
    expect(parseAmount(' 007 ')).toBe(7)
  })

  it.each([
    ['1 500', 1500],
    ['1,500', 1500],
    ['12 345 678', 12345678],
    ['1\u00A0500', 1500],
    ['1\u202F500', 1500],
    [' 1 500\n', 1500],
    ['1500', 1500],
  ])('parseAmount %j -> %j', (raw, want) => {
    expect(parseAmount(raw)).toBe(want)
  })
})
