import { useId, useState } from 'react'
import { Check, Trash2, TriangleAlert } from 'lucide-react'
import { api } from '@/lib/api'
import { formatMoney } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { TableCell, TableRow } from '@/components/ui/table'

export type Budget = { category_id: number | null; amount: number }

type Props = {
  name: string
  color?: string
  categoryId: number | null
  month: string
  initialAmount: number | undefined
}

type Status = { kind: 'idle' } | { kind: 'saved' } | { kind: 'removed' } | { kind: 'error'; message: string }

const WHOLE_POSITIVE = /^[1-9]\d{0,14}$/

export function validateAmount(raw: string): string | null {
  const value = raw.trim()
  if (value === '') return 'Enter an amount'
  if (!WHOLE_POSITIVE.test(value)) return 'Enter a whole number greater than 0'
  return null
}

export function BudgetRow({ name, color, categoryId, month, initialAmount }: Props) {
  const inputId = useId()
  const errorId = `${inputId}-error`
  const [saved, setSaved] = useState<number | undefined>(initialAmount)
  const [value, setValue] = useState(initialAmount === undefined ? '' : String(initialAmount))
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [status, setStatus] = useState<Status>({ kind: 'idle' })
  const [saving, setSaving] = useState(false)
  const [removing, setRemoving] = useState(false)

  async function save(e: React.FormEvent) {
    e.preventDefault()
    const problem = validateAmount(value)
    setFieldError(problem)
    if (problem) {
      setStatus({ kind: 'idle' })
      return
    }
    setSaving(true)
    setStatus({ kind: 'idle' })
    try {
      const body: { amount: number; category_id?: number } = { amount: Number(value.trim()) }
      if (categoryId !== null) body.category_id = categoryId
      const result = await api<Budget>(`/api/budgets?month=${month}`, { method: 'PUT', body })
      const amount = typeof result?.amount === 'number' ? result.amount : body.amount
      setSaved(amount)
      setValue(String(amount))
      setStatus({ kind: 'saved' })
    } catch (err) {
      const message = err instanceof Error && err.message ? err.message : 'Could not save'
      setStatus({
        kind: 'error',
        message: `${message}. Check your connection and try again.`,
      })
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    setRemoving(true)
    setStatus({ kind: 'idle' })
    setFieldError(null)
    try {
      const query = categoryId === null ? '' : `&category_id=${categoryId}`
      await api(`/api/budgets?month=${month}${query}`, { method: 'DELETE' })
      setSaved(undefined)
      setValue('')
      setStatus({ kind: 'removed' })
    } catch (err) {
      const message = err instanceof Error && err.message ? err.message : 'Could not remove'
      setStatus({
        kind: 'error',
        message: `${message}. Check your connection and try again.`,
      })
    } finally {
      setRemoving(false)
    }
  }

  return (
    <TableRow>
      <TableCell className="font-medium">
        <span className="flex items-center gap-2">
          {color && (
            <span
              aria-hidden="true"
              className="size-3 shrink-0 rounded-full"
              style={{ backgroundColor: color }}
            />
          )}
          {name}
        </span>
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {saved === undefined ? (
          <span className="text-muted-foreground">Not set</span>
        ) : (
          formatMoney(saved)
        )}
      </TableCell>
      <TableCell>
        <form onSubmit={save} noValidate className="flex flex-wrap items-start gap-2">
          <div className="flex min-w-32 flex-1 flex-col gap-1">
            <Label htmlFor={inputId} className="sr-only">
              {`${name} budget in tenge`}
            </Label>
            <Input
              id={inputId}
              inputMode="numeric"
              autoComplete="off"
              className="text-right tabular-nums"
              value={value}
              aria-invalid={fieldError ? true : undefined}
              aria-describedby={fieldError ? errorId : undefined}
              onChange={(e) => {
                setValue(e.target.value)
                setFieldError(null)
                setStatus({ kind: 'idle' })
              }}
            />
            {fieldError && (
              <p id={errorId} role="alert" className="text-xs text-destructive">
                {fieldError}
              </p>
            )}
          </div>
          <Button type="submit" disabled={saving} aria-label={`Save ${name} budget`}>
            {saving ? 'Saving…' : 'Save'}
          </Button>
          {saved !== undefined && (
            <Button
              type="button"
              variant="outline"
              disabled={removing || saving}
              onClick={() => void remove()}
              aria-label={`Remove ${name} budget`}
            >
              <Trash2 className="size-4" aria-hidden="true" />
              {removing ? 'Removing…' : 'Remove'}
            </Button>
          )}
          <div role="status" className="basis-full text-xs">
            {status.kind === 'saved' && (
              <span className="flex items-center gap-1 text-success">
                <Check className="size-3" aria-hidden="true" /> Saved
              </span>
            )}
            {status.kind === 'removed' && (
              <span className="flex items-center gap-1 text-success">
                <Check className="size-3" aria-hidden="true" /> Removed
              </span>
            )}
          </div>
          {status.kind === 'error' && (
            <p role="alert" className="flex basis-full items-center gap-1 text-xs text-destructive">
              <TriangleAlert className="size-3" aria-hidden="true" /> {status.message}
            </p>
          )}
        </form>
      </TableCell>
    </TableRow>
  )
}
