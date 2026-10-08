import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import { BudgetRow, type Budget } from '@/features/budgets/BudgetRow'
import { MonthPicker } from '@/components/MonthPicker'
import { useMonth } from '@/features/month/month'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableHead, TableHeader, TableRow } from '@/components/ui/table'

type Category = { id: number; name: string; color: string; is_default: boolean }

type State =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; categories: Category[]; budgets: Budget[] }

export default function BudgetsPage() {
  const [month, setMonth] = useMonth()
  const [state, setState] = useState<State>({ status: 'loading' })
  const latest = useRef(0)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const id = ++latest.current
    setState({ status: 'loading' })
    Promise.all([api<Category[]>('/api/categories'), api<Budget[]>(`/api/budgets?month=${month}`)])
      .then(([categories, budgets]) => {
        if (id !== latest.current) return
        setState({ status: 'ready', categories: categories ?? [], budgets: budgets ?? [] })
      })
      .catch(() => {
        if (id !== latest.current) return
        setState({ status: 'error' })
      })
    return () => {
      latest.current++
    }
  }, [month, attempt])

  const amountFor = (categoryId: number | null) =>
    state.status === 'ready'
      ? state.budgets.find((b) => (b.category_id ?? null) === categoryId)?.amount
      : undefined

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold">Budgets</h1>
        <MonthPicker month={month} onChange={setMonth} />
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
            <Button variant="outline" onClick={() => setAttempt((n) => n + 1)}>
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
