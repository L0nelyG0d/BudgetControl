export type Expense = {
  id: number
  category_id: number
  amount: number
  date: string
  note?: string | null
  created_at?: string
}
export type Category = { id: number; name: string; color: string; is_default?: boolean }
export type Budget = { category_id: number | null; amount: number }

export type CategoryRow = {
  categoryId: number
  name: string
  color: string
  spent: number
  budget: number | null
  /** budget - spent; null without a budget. Negative means over budget. */
  remaining: number | null
}

export type Summary = {
  total: number
  rows: CategoryRow[]
  overall: { budget: number; spent: number; remaining: number } | null
}

/** Current month in local time as YYYY-MM. */
export function currentMonth(now: Date = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

/** Pure aggregation: spending per category joined with budgets. */
export function summarize(expenses: Expense[], categories: Category[], budgets: Budget[]): Summary {
  const spent = new Map<number, number>()
  let total = 0
  for (const e of expenses) {
    spent.set(e.category_id, (spent.get(e.category_id) ?? 0) + e.amount)
    total += e.amount
  }
  const budgetBy = new Map<number, number>()
  let overallBudget: number | null = null
  for (const b of budgets) {
    if (b.category_id === null) overallBudget = b.amount
    else budgetBy.set(b.category_id, b.amount)
  }

  const rows: CategoryRow[] = []
  const known = new Set<number>()
  for (const c of categories) {
    known.add(c.id)
    const s = spent.get(c.id) ?? 0
    const b = budgetBy.get(c.id) ?? null
    if (s === 0 && b === null) continue
    rows.push({ categoryId: c.id, name: c.name, color: c.color, spent: s, budget: b, remaining: b === null ? null : b - s })
  }
  for (const [id, s] of spent) {
    if (known.has(id)) continue
    const b = budgetBy.get(id) ?? null
    rows.push({ categoryId: id, name: 'Uncategorized', color: 'var(--muted-foreground)', spent: s, budget: b, remaining: b === null ? null : b - s })
  }
  rows.sort((a, b) => b.spent - a.spent || a.name.localeCompare(b.name))

  return {
    total,
    rows,
    overall: overallBudget === null ? null : { budget: overallBudget, spent: total, remaining: overallBudget - total },
  }
}
