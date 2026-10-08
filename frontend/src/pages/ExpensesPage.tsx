import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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
import { api, ApiError } from '@/lib/api'
import { formatMoney } from '@/lib/format'

type Data = { expenses: Expense[]; categories: Category[] }
type State =
  | { status: 'loading' }
  | { status: 'error' }
  | { status: 'ready'; data: Data }

export default function ExpensesPage() {
  const [state, setState] = useState<State>({ status: 'loading' })
  const [editing, setEditing] = useState<Expense | null>(null)
  const [deleting, setDeleting] = useState<Expense | null>(null)
  const [deletePending, setDeletePending] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

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

  function onEdited(expense: Expense) {
    setState((s) =>
      s.status === 'ready'
        ? {
            status: 'ready',
            data: {
              ...s.data,
              expenses: sortExpenses(
                s.data.expenses.map((e) => (String(e.id) === String(expense.id) ? expense : e)),
              ),
            },
          }
        : s,
    )
    setEditing(null)
  }

  async function confirmDelete() {
    if (!deleting || deletePending) return
    const id = deleting.id
    setDeletePending(true)
    setDeleteError(null)
    try {
      await api(`/api/expenses/${id}`, { method: 'DELETE' })
      setState((s) =>
        s.status === 'ready'
          ? {
              status: 'ready',
              data: { ...s.data, expenses: s.data.expenses.filter((e) => String(e.id) !== String(id)) },
            }
          : s,
      )
      if (editing && String(editing.id) === String(id)) setEditing(null)
      setDeleting(null)
      toast.success('Expense deleted')
    } catch (err) {
      setDeleteError(
        err instanceof ApiError
          ? 'Could not delete the expense. Please try again.'
          : 'Could not reach the server. Check your connection and try again.',
      )
    } finally {
      setDeletePending(false)
    }
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
          {editing ? (
            <ExpenseForm
              key={editing.id}
              categories={state.data.categories}
              expense={editing}
              onSaved={onEdited}
              onCancel={() => setEditing(null)}
            />
          ) : (
            <ExpenseForm categories={state.data.categories} onSaved={onAdded} />
          )}
          <ExpenseTable
            data={state.data}
            onEdit={setEditing}
            onDelete={(e) => {
              setDeleteError(null)
              setDeleting(e)
            }}
          />
        </>
      )}

      <AlertDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open && !deletePending) setDeleting(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this expense?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting
                ? `${formatMoney(deleting.amount)} on ${formatDate(deleting.date)} will be removed from your expenses and budget totals. This cannot be undone.`
                : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <p role="alert" className="text-sm text-destructive">
              {deleteError}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletePending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deletePending}
              onClick={(e) => {
                e.preventDefault()
                void confirmDelete()
              }}
            >
              Delete expense
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ExpenseTable({
  data,
  onEdit,
  onDelete,
}: {
  data: Data
  onEdit: (e: Expense) => void
  onDelete: (e: Expense) => void
}) {
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
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {data.expenses.map((e) => (
            <TableRow key={e.id}>
              <TableCell className="text-right tabular-nums">{formatMoney(e.amount)}</TableCell>
              <TableCell>{names.get(String(e.category_id)) ?? 'Unknown'}</TableCell>
              <TableCell className="tabular-nums">{formatDate(e.date)}</TableCell>
              <TableCell className="text-muted-foreground">{e.note ?? ''}</TableCell>
              <TableCell className="text-right">
                <span className="flex justify-end gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    aria-label={`Edit expense of ${formatDate(e.date)}, ${names.get(String(e.category_id)) ?? 'Unknown'}`}
                    onClick={() => onEdit(e)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    aria-label={`Delete expense of ${formatDate(e.date)}, ${names.get(String(e.category_id)) ?? 'Unknown'}`}
                    onClick={() => onDelete(e)}
                  >
                    Delete
                  </Button>
                </span>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
