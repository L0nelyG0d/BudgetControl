import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from 'sonner'
import { api, ApiError } from '@/lib/api'
import { todayISO, validateAmount, type Category, type Expense } from './expenses'

export function ExpenseForm({
  categories,
  expense,
  onSaved,
  onCancel,
}: {
  categories: Category[]
  /** When set, the form edits this expense instead of adding a new one. */
  expense?: Expense
  onSaved: (expense: Expense) => void
  onCancel?: () => void
}) {
  const editing = expense !== undefined
  const [amount, setAmount] = useState(expense ? String(expense.amount) : '')
  const [categoryId, setCategoryId] = useState(() =>
    expense ? String(expense.category_id) : categories[0] ? String(categories[0].id) : '',
  )
  const [date, setDate] = useState(expense ? expense.date.slice(0, 10) : todayISO)
  const [note, setNote] = useState(expense?.note ?? '')
  const [pending, setPending] = useState(false)
  const [amountError, setAmountError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (pending) return
    setSaved(false)
    setFormError(null)
    const err = validateAmount(amount)
    setAmountError(err)
    if (err) return
    if (!categoryId) return setFormError('Choose a category.')
    if (!date) return setFormError('Choose a date.')

    setPending(true)
    try {
      const category = categories.find((c) => String(c.id) === categoryId)
      const saved = await api<Expense>(
        expense ? `/api/expenses/${expense.id}` : '/api/expenses',
        {
        method: expense ? 'PUT' : 'POST',
        body: {
          amount: Number(amount.trim()),
          category_id: category ? category.id : Number(categoryId),
          date,
          ...(note.trim() ? { note: note.trim() } : {}),
        },
        },
      )
      onSaved(saved)
      if (editing) {
        toast.success('Expense updated')
      } else {
        setAmount('')
        setNote('')
        setSaved(true)
      }
    } catch (error) {
      setFormError(
        error instanceof ApiError && error.status === 400
          ? error.message
          : error instanceof ApiError
            ? 'Something went wrong on our side. Your entries are kept, try again.'
            : 'Could not save the expense. Check your connection and try again.',
      )
    } finally {
      setPending(false)
    }
  }

  return (
    <form
      onSubmit={onSubmit}
      noValidate
      className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4"
    >
      <h2 className="text-base font-semibold">{editing ? 'Edit expense' : 'Add expense'}</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-2">
          <Label htmlFor="expense-amount">Amount, ₸</Label>
          <Input
            id="expense-amount"
            inputMode="numeric"
            autoComplete="off"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            aria-invalid={amountError ? true : undefined}
            aria-describedby={amountError ? 'expense-amount-error' : undefined}
          />
          {amountError && (
            <p id="expense-amount-error" className="text-xs text-destructive">
              {amountError}
            </p>
          )}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="expense-category">Category</Label>
          <Select value={categoryId} onValueChange={setCategoryId}>
            <SelectTrigger id="expense-category" className="w-full">
              <SelectValue placeholder="Choose a category" />
            </SelectTrigger>
            <SelectContent>
              {categories.map((c) => (
                <SelectItem key={c.id} value={String(c.id)}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="expense-date">Date</Label>
          <Input
            id="expense-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="expense-note">Note (optional)</Label>
          <Input
            id="expense-note"
            autoComplete="off"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
      </div>
      {formError && (
        <p role="alert" className="text-sm text-destructive">
          {formError}
        </p>
      )}
      {saved && (
        <p role="status" className="text-sm text-success">
          Expense added.
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" disabled={pending}>
          {editing
            ? pending
              ? 'Saving…'
              : 'Save changes'
            : pending
              ? 'Adding…'
              : 'Add expense'}
        </Button>
        {editing && (
          <Button type="button" variant="outline" onClick={onCancel} disabled={pending}>
            Cancel
          </Button>
        )}
      </div>
    </form>
  )
}
