import { useCallback, useEffect, useState } from 'react'
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
import {
  currentMonth,
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

export default function DashboardPage() {
  const [state, setState] = useState<State>({ status: 'loading' })
  const month = currentMonth()

  const load = useCallback(() => {
    setState({ status: 'loading' })
    Promise.all([
      api<Expense[]>(`/api/expenses?month=${month}`),
      api<Category[]>('/api/categories'),
      api<Budget[]>(`/api/budgets?month=${month}`),
    ])
      .then(([expenses, categories, budgets]) => {
        if (![expenses, categories, budgets].every(Array.isArray)) throw new Error('Unexpected server response')
        setState({ status: 'ready', summary: summarize(expenses, categories, budgets) })
      })
      .catch((err: unknown) =>
        setState({
          status: 'error',
          message: err instanceof Error ? err.message : 'Something went wrong.',
        }),
    )
  }, [month])

  useEffect(() => {
    load()
  }, [load])

  const heading = new Date(`${month}-01T00:00:00`).toLocaleString('en-US', { month: 'long', year: 'numeric' })

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Dashboard</h1>
      <p className="text-sm text-muted-foreground">{heading}</p>

      {state.status === 'loading' && (
        <div className="space-y-4" role="status" aria-label="Loading dashboard">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      )}

      {state.status === 'error' && (
        <Card>
          <CardContent className="space-y-4">
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
      <Card>
        <CardHeader>
          <CardTitle>Spent this month</CardTitle>
          <p className="text-3xl font-semibold tabular-nums" data-testid="total-spent">
            {formatMoney(summary.total)}
          </p>
        </CardHeader>
        <CardContent>
          {spending.length === 0 ? (
            <p className="text-muted-foreground">No expenses this month</p>
          ) : (
            <div className="space-y-4">
              <ChartContainer config={config} className="mx-auto max-h-64">
                <PieChart>
                  <ChartTooltip content={<ChartTooltipContent nameKey="name" />} />
                  <Pie data={spending} dataKey="spent" nameKey="name" innerRadius={50} isAnimationActive={false}>
                    {spending.map((r) => (
                      <Cell key={r.categoryId} fill={r.color} />
                    ))}
                  </Pie>
                </PieChart>
              </ChartContainer>
              <ul aria-label="Chart legend" className="grid gap-2 text-sm sm:grid-cols-2">
                {spending.map((r) => (
                  <li key={r.categoryId} className="flex items-center justify-between gap-2">
                    <span className="flex items-center gap-2">
                      <span
                        className="size-3 shrink-0 rounded-sm"
                        style={{ backgroundColor: r.color }}
                        aria-hidden="true"
                      />
                      {r.name}
                    </span>
                    <span className="tabular-nums">{formatMoney(r.spent)}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Budgets</CardTitle>
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
                      <TableCell>
                        <span className="flex items-center gap-2">
                          <span
                            className="size-3 shrink-0 rounded-sm"
                            style={{ backgroundColor: r.color }}
                            aria-hidden="true"
                          />
                          {r.name}
                        </span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{formatMoney(r.spent)}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {r.budget === null ? '-' : formatMoney(r.budget)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {r.remaining === null ? '-' : over ? <Status remaining={r.remaining} /> : formatMoney(r.remaining)}
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
