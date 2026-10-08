import { useCallback, useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { BudgetRow, type Budget } from '@/features/budgets/BudgetRow'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableHead, TableHeader, TableRow } from '@/components/ui/table'

type Category = { id: number; name: string; color: string; is_default: boolean }

type State =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; categories: Category[]; budgets: Budget[] }

const MONTHS = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]

export default function BudgetsPage() {
  // Computed once per mount, in local time.
  const [now] = useState(() => new Date())
  const month = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const heading = `${MONTHS[now.getMonth()]} ${now.getFullYear()}`
  const [state, setState] = useState<State>({ status: 'loading' })

  const load = useCallback(async () => {
    setState({ status: 'loading' })
    try {
      const [categories, budgets] = await Promise.all([
        api<Category[]>('/api/categories'),
        api<Budget[]>(`/api/budgets?month=${month}`),
      ])
      setState({ status: 'ready', categories: categories ?? [], budgets: budgets ?? [] })
    } catch {
      setState({ status: 'error' })
    }
  }, [month])

  useEffect(() => {
    void load()
  }, [load])

  const amountFor = (categoryId: number | null) =>
    state.status === 'ready'
      ? state.budgets.find((b) => (b.category_id ?? null) === categoryId)?.amount
      : undefined

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold">Budgets</h1>
        <h2 className="text-muted-foreground">{heading}</h2>
      </div>

      {state.status === 'loading' && (
        <div role="status" aria-label="Loading budgets" className="flex flex-col gap-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      )}

      {state.status === 'error' && (
        <Card>
          <CardContent className="flex flex-col items-start gap-4">
            <p role="alert">Could not load your budgets. Check your connection and try again.</p>
            <Button variant="outline" onClick={() => void load()}>
              Try again
            </Button>
          </CardContent>
        </Card>
      )}

      {state.status === 'ready' && (
        <Card>
          <CardContent>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Category</TableHead>
                    <TableHead className="text-right">Current budget</TableHead>
                    <TableHead>New amount (₸)</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <BudgetRow
                    name="Overall"
                    categoryId={null}
                    month={month}
                    initialAmount={amountFor(null)}
                  />
                  {state.categories.map((c) => (
                    <BudgetRow
                      key={c.id}
                      name={c.name}
                      color={c.color}
                      categoryId={c.id}
                      month={month}
                      initialAmount={amountFor(c.id)}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
