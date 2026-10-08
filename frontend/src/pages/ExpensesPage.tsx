import { useCallback, useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ExpenseForm } from '@/features/expenses/ExpenseForm'
import {
  formatDate,
  sortExpenses,
  type Category,
  type Expense,
} from '@/features/expenses/expenses'
import { api } from '@/lib/api'
import { formatMoney } from '@/lib/format'

type Data = { expenses: Expense[]; categories: Category[] }
type State =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; data: Data }

export default function ExpensesPage() {
  const [state, setState] = useState<State>({ status: 'loading' })

  const load = useCallback(async () => {
    setState({ status: 'loading' })
    try {
      const [expenses, categories] = await Promise.all([
        api<Expense[]>('/api/expenses'),
        api<Category[]>('/api/categories'),
      ])
      setState({
        status: 'ready',
        data: { expenses: sortExpenses(Array.isArray(expenses) ? expenses : []), categories: Array.isArray(categories) ? categories : [] },
      })
    } catch {
      setState({ status: 'error' })
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  function onAdded(expense: Expense) {
    setState((s) =>
      s.status === 'ready'
        ? { status: 'ready', data: { ...s.data, expenses: sortExpenses([...s.data.expenses, expense]) } }
        : s,
    )
  }

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 px-4 py-6">
      <h1 className="text-2xl font-semibold">Expenses</h1>

      {state.status === 'loading' && (
        <div role="status" aria-label="Loading expenses" className="flex flex-col gap-2">
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      )}

      {state.status === 'error' && (
        <div role="alert" className="flex flex-col items-start gap-3">
          <p className="text-sm text-destructive">
            Could not load your expenses. Check your connection and try again.
          </p>
          <Button variant="outline" onClick={() => void load()}>
            Try again
          </Button>
        </div>
      )}

      {state.status === 'ready' && (
        <>
          <ExpenseForm categories={state.data.categories} onAdded={onAdded} />
          <ExpenseTable data={state.data} />
        </>
      )}
    </div>
  )
}

function ExpenseTable({ data }: { data: Data }) {
  if (data.expenses.length === 0) {
    return (
      <div className="rounded-xl border border-border bg-card p-6 text-center">
        <p className="font-semibold">No expenses yet</p>
        <p className="text-sm text-muted-foreground">Add your first expense with the form above.</p>
      </div>
    )
  }
  const names = new Map(data.categories.map((c) => [String(c.id), c.name]))
  return (
    <div className="overflow-x-auto rounded-xl border border-border bg-card">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="text-right">Amount</TableHead>
            <TableHead>Category</TableHead>
            <TableHead>Date</TableHead>
            <TableHead>Note</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {data.expenses.map((e) => (
            <TableRow key={e.id}>
              <TableCell className="text-right tabular-nums">{formatMoney(e.amount)}</TableCell>
              <TableCell>{names.get(String(e.category_id)) ?? 'Unknown'}</TableCell>
              <TableCell className="tabular-nums">{formatDate(e.date)}</TableCell>
              <TableCell className="text-muted-foreground">{e.note ?? ''}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
