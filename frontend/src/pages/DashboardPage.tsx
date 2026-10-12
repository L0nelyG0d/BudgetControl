import { useEffect, useRef, useState } from 'react'
import { Check, TriangleAlert } from 'lucide-react'
import { Cell, Pie, PieChart } from 'recharts'
import { api } from '@/lib/api'
import { formatMoney } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { MonthPicker } from '@/components/MonthPicker'
import { useMonth } from '@/features/month/month'
import {
  summarize,
  type Budget,
  type Category,
  type Expense,
  type Summary,
} from '@/features/dashboard/summary'

type State =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; summary: Summary }

function Status({ remaining }: { remaining: number }) {
  if (remaining < 0) {
    return (
      <span className="inline-flex items-center gap-1 font-semibold text-destructive">
        <TriangleAlert className="size-4" aria-hidden="true" />
        Over by {formatMoney(-remaining)}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-success">
      <Check className="size-4" aria-hidden="true" />
      Within budget
    </span>
  )
}

/** Spent / budget bar: fill capped at 100%, overage is shown as text elsewhere. */
function BudgetBar({ spent, budget, label }: { spent: number; budget: number; label: string }) {
  const pct = budget > 0 ? Math.min(100, Math.round((spent / budget) * 100)) : spent > 0 ? 100 : 0
  const over = spent > budget
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={pct}
      className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
    >
      <div
        className={`h-full rounded-full ${over ? 'bg-destructive' : 'bg-success'}`}
        style={{ width: `${pct}%` }}
      />
    </div>
  )
}

function Stat({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl bg-card p-3 shadow-xs ring-1 ring-foreground/10">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-0.5 text-base font-semibold tabular-nums">{children}</div>
    </div>
  )
}

export default function DashboardPage() {
  const [state, setState] = useState<State>({ status: 'loading' })
  const [month, setMonth] = useMonth()
  const latest = useRef(0)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const id = ++latest.current
    setState({ status: 'loading' })
    Promise.all([
      api<Expense[]>(`/api/expenses?month=${month}`),
      api<Category[]>('/api/categories'),
      api<Budget[]>(`/api/budgets?month=${month}`),
    ])
      .then(([expenses, categories, budgets]) => {
        if (id !== latest.current) return
        if (![expenses, categories, budgets].every(Array.isArray)) throw new Error('Unexpected server response')
        setState({ status: 'ready', summary: summarize(expenses, categories, budgets) })
      })
      .catch((err: unknown) => {
        if (id !== latest.current) return
        setState({
          status: 'error',
          message: err instanceof Error ? err.message : 'Something went wrong.',
        })
      })
    return () => {
      latest.current++
    }
  }, [month, attempt])

  const load = () => setAttempt((n) => n + 1)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-semibold">Dashboard</h1>
        <MonthPicker month={month} onChange={setMonth} />
      </div>

      {state.status === 'loading' && (
        <div className="space-y-3" role="status" aria-label="Loading dashboard">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-48 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      )}

      {state.status === 'error' && (
        <Card>
          <CardContent className="space-y-3">
            <p role="alert" className="text-destructive">
              Could not load the dashboard: {state.message}. Check your connection and try again.
            </p>
            <Button variant="outline" onClick={load}>
              Retry
            </Button>
          </CardContent>
        </Card>
      )}

      {state.status === 'ready' && <Ready summary={state.summary} />}
    </div>
  )
}

function Ready({ summary }: { summary: Summary }) {
  const spending = summary.rows.filter((r) => r.spent > 0)
  const config: ChartConfig = Object.fromEntries(
    spending.map((r) => [String(r.categoryId), { label: r.name, color: r.color }]),
  )
  const o = summary.overall

  return (
    <>
      <div className="grid grid-cols-3 gap-2">
        <Stat label="Budget">{o ? formatMoney(o.budget) : <span className="text-muted-foreground">Not set</span>}</Stat>
        <Stat label="Remaining">
          {o ? (o.remaining < 0 ? <Status remaining={o.remaining} /> : formatMoney(o.remaining)) : '-'}
        </Stat>
        <Stat label="Categories">{spending.length}</Stat>
      </div>
      {o && (
        <BudgetBar spent={o.spent} budget={o.budget} label="Overall budget used" />
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium text-muted-foreground">Spent this month</CardTitle>
        </CardHeader>
        <CardContent>
          {spending.length === 0 ? (
            <div className="space-y-1">
              <p className="text-3xl font-bold tabular-nums" data-testid="total-spent">
                {formatMoney(summary.total)}
              </p>
              <p className="text-muted-foreground">No expenses this month</p>
            </div>
          ) : (
            <div className="flex flex-col items-center gap-3 sm:flex-row">
              <div className="relative w-44 shrink-0">
                <ChartContainer config={config} className="aspect-square w-full">
                  <PieChart>
                    <ChartTooltip content={<ChartTooltipContent nameKey="name" />} />
                    <Pie
                      data={spending}
                      dataKey="spent"
                      nameKey="name"
                      innerRadius="68%"
                      outerRadius="100%"
                      paddingAngle={2}
                      stroke="none"
                      isAnimationActive={false}
                    >
                      {spending.map((r) => (
                        <Cell key={r.categoryId} fill={r.color} style={{ fill: r.color }} />
                      ))}
                    </Pie>
                  </PieChart>
                </ChartContainer>
                <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
                  <span className="text-xs text-muted-foreground">Total</span>
                  <span className="text-xl font-bold tabular-nums" data-testid="total-spent">
                    {formatMoney(summary.total)}
                  </span>
                </div>
              </div>
              <ul aria-label="Chart legend" className="grid w-full flex-1 gap-1.5 text-sm">
                {spending.map((r) => (
                  <li key={r.categoryId} className="flex items-center justify-between gap-2 rounded-lg bg-muted/50 px-2.5 py-1.5">
                    <span className="flex min-w-0 items-center gap-2">
                      <span
                        className="size-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: r.color }}
                        aria-hidden="true"
                      />
                      <span className="truncate">{r.name}</span>
                    </span>
                    <span className="flex items-baseline gap-2">
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {Math.round((r.spent / summary.total) * 100)}%
                      </span>
                      <span className="font-semibold tabular-nums">{formatMoney(r.spent)}</span>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium text-muted-foreground">Budgets</CardTitle>
        </CardHeader>
        <CardContent>
          {summary.rows.length === 0 && !o ? (
            <p className="text-muted-foreground">No budgets or spending yet. Add an expense or set a budget.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Category</TableHead>
                  <TableHead className="text-right">Spent</TableHead>
                  <TableHead className="text-right">Budget</TableHead>
                  <TableHead className="text-right">Remaining</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {summary.rows.map((r) => {
                  const over = r.remaining !== null && r.remaining < 0
                  return (
                    <TableRow key={r.categoryId} className={over ? 'bg-destructive/10' : undefined}>
                      <TableCell className="min-w-32">
                        <span className="flex items-center gap-2 font-medium">
                          <span
                            className="size-2.5 shrink-0 rounded-full"
                            style={{ backgroundColor: r.color }}
                            aria-hidden="true"
                          />
                          {r.name}
                        </span>
                        {r.budget !== null && (
                          <div className="mt-1">
                            <BudgetBar spent={r.spent} budget={r.budget} label={`${r.name} budget used`} />
                          </div>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-semibold tabular-nums">{formatMoney(r.spent)}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {r.budget === null ? '-' : formatMoney(r.budget)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {r.remaining === null ? (
                          '-'
                        ) : over ? (
                          <Status remaining={r.remaining} />
                        ) : (
                          <span className="inline-flex items-center gap-1 text-success">
                            <Check className="size-4" aria-hidden="true" />
                            {formatMoney(r.remaining)}
                            <span className="sr-only">Within budget</span>
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
                {o && (
                  <TableRow
                    className={o.remaining < 0 ? 'bg-destructive/10 font-semibold' : 'font-semibold'}
                  >
                    <TableCell>Overall budget</TableCell>
                    <TableCell className="text-right tabular-nums">{formatMoney(o.spent)}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatMoney(o.budget)}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {o.remaining < 0 ? <Status remaining={o.remaining} /> : formatMoney(o.remaining)}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </>
  )
}
