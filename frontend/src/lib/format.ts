const money = new Intl.NumberFormat('ru-KZ', {
  style: 'currency',
  currency: 'KZT',
  maximumFractionDigits: 0,
})

/** Format whole tenge, e.g. 1500 -> "1 500 ₸". */
export function formatMoney(amount: number): string {
  return money.format(amount)
}
