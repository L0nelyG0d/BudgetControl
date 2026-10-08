import { useCallback, useEffect, useState, type FormEvent } from 'react'
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
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { Category } from '@/features/categories/types'
import { api, ApiError } from '@/lib/api'

const DEFAULT_COLOR = '#4F46E5'
const DUPLICATE_MESSAGE = 'A category with this name already exists'

function saveErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 409) return DUPLICATE_MESSAGE
    if (err.status === 400) return err.message
    if (err.status === 403) return 'This category cannot be changed.'
    return 'Something went wrong on our side. Please try again.'
  }
  return 'Could not reach the server. Check your connection and try again.'
}

function ColorSwatch({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block size-4 shrink-0 rounded-full border border-border"
      style={{ backgroundColor: color }}
    />
  )
}

export default function CategoriesPage() {
  const [categories, setCategories] = useState<Category[] | null>(null)
  const [loadError, setLoadError] = useState(false)

  const [editing, setEditing] = useState<Category | null>(null)
  const [name, setName] = useState('')
  const [color, setColor] = useState(DEFAULT_COLOR)
  const [nameError, setNameError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const [deleting, setDeleting] = useState<Category | null>(null)
  const [deletePending, setDeletePending] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoadError(false)
    setCategories(null)
    try {
      setCategories(await api<Category[]>('/api/categories'))
    } catch {
      setLoadError(true)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  function resetForm() {
    setEditing(null)
    setName('')
    setColor(DEFAULT_COLOR)
    setNameError(null)
    setFormError(null)
  }

  function startEdit(category: Category) {
    setEditing(category)
    setName(category.name)
    setColor(category.color)
    setNameError(null)
    setFormError(null)
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (saving) return
    setFormError(null)
    const trimmed = name.trim()
    if (!trimmed) {
      setNameError('Enter a name for the category.')
      return
    }
    setNameError(null)
    setSaving(true)
    try {
      const body = { name: trimmed, color: color.toUpperCase() }
      if (editing) {
        const updated = await api<Category>(`/api/categories/${editing.id}`, {
          method: 'PUT',
          body,
        })
        setCategories((list) => list?.map((c) => (c.id === updated.id ? updated : c)) ?? list)
        toast.success('Category updated')
      } else {
        const created = await api<Category>('/api/categories', { method: 'POST', body })
        setCategories((list) => [...(list ?? []), created])
        toast.success('Category added')
      }
      resetForm()
    } catch (err) {
      const message = saveErrorMessage(err)
      if (err instanceof ApiError && err.status === 409) setNameError(message)
      else setFormError(message)
    } finally {
      setSaving(false)
    }
  }

  async function confirmDelete() {
    if (!deleting || deletePending) return
    setDeletePending(true)
    setDeleteError(null)
    try {
      await api(`/api/categories/${deleting.id}`, { method: 'DELETE' })
      const id = deleting.id
      setCategories((list) => list?.filter((c) => c.id !== id) ?? list)
      if (editing?.id === id) resetForm()
      setDeleting(null)
      toast.success('Category deleted')
    } catch (err) {
      setDeleteError(
        err instanceof ApiError && err.status !== 403
          ? 'Could not delete the category. Please try again.'
          : err instanceof ApiError
            ? 'This category cannot be deleted.'
            : 'Could not reach the server. Check your connection and try again.',
      )
    } finally {
      setDeletePending(false)
    }
  }

  const hasCustom = categories?.some((c) => !c.is_default) ?? false

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold">Categories</h1>

      <Card>
        <CardHeader>
          <CardTitle>{editing ? 'Edit category' : 'Add category'}</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4 sm:flex-row sm:items-start">
            <div className="flex flex-1 flex-col gap-2">
              <Label htmlFor="category-name">Name</Label>
              <Input
                id="category-name"
                value={name}
                maxLength={50}
                onChange={(e) => setName(e.target.value)}
                aria-invalid={nameError ? true : undefined}
                aria-describedby={nameError ? 'category-name-error' : undefined}
              />
              {nameError && (
                <p id="category-name-error" className="text-xs text-destructive">
                  {nameError}
                </p>
              )}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="category-color">Color</Label>
              <input
                id="category-color"
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                className="h-8 w-16 cursor-pointer rounded-lg border border-border bg-card p-1 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
              />
            </div>
            <div className="flex gap-2 sm:mt-6">
              <Button type="submit" disabled={saving}>
                {editing ? 'Save changes' : 'Add category'}
              </Button>
              {editing && (
                <Button type="button" variant="outline" onClick={resetForm}>
                  Cancel
                </Button>
              )}
            </div>
          </form>
          {formError && (
            <p role="alert" className="mt-4 text-sm text-destructive">
              {formError}
            </p>
          )}
        </CardContent>
      </Card>

      {loadError ? (
        <Card>
          <CardContent className="flex flex-col items-start gap-4">
            <p role="alert" className="text-sm text-destructive">
              Could not load categories. Check your connection and try again.
            </p>
            <Button variant="outline" onClick={() => void load()}>
              Try again
            </Button>
          </CardContent>
        </Card>
      ) : categories === null ? (
        <div className="flex flex-col gap-2" role="status" aria-label="Loading categories">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : (
        <Card>
          <CardContent className="flex flex-col gap-4">
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Category</TableHead>
                    <TableHead>Color</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {categories.map((c) => (
                    <TableRow key={c.id}>
                      <TableCell>
                        <span className="flex items-center gap-2">
                          <ColorSwatch color={c.color} />
                          <span className="font-medium">{c.name}</span>
                          {c.is_default && (
                            <span className="rounded-md border border-border px-2 py-0.5 text-xs text-muted-foreground">
                              Default
                            </span>
                          )}
                        </span>
                      </TableCell>
                      <TableCell className="text-muted-foreground tabular-nums">{c.color}</TableCell>
                      <TableCell className="text-right">
                        {!c.is_default && (
                          <span className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              aria-label={`Edit ${c.name}`}
                              onClick={() => startEdit(c)}
                            >
                              Edit
                            </Button>
                            <Button
                              variant="destructive"
                              size="sm"
                              aria-label={`Delete ${c.name}`}
                              onClick={() => {
                                setDeleteError(null)
                                setDeleting(c)
                              }}
                            >
                              Delete
                            </Button>
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {!hasCustom && (
              <p className="text-sm text-muted-foreground">
                You have no custom categories yet. Use the form above to add one.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      <AlertDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open && !deletePending) setDeleting(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {deleting?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Expenses in this category will move to Other. Its budgets will be removed. This cannot
              be undone.
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
              Delete category
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
