import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { MAX_MONTH, MIN_MONTH, monthLabel, shiftMonth } from '@/features/month/month'

export function MonthPicker({ month, onChange }: { month: string; onChange: (month: string) => void }) {
  return (
    <div className="flex items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label="Previous month"
        disabled={month <= MIN_MONTH}
        onClick={() => onChange(shiftMonth(month, -1))}
      >
        <ChevronLeft className="size-4" aria-hidden="true" />
      </Button>
      <span className="min-w-36 text-center font-medium" aria-live="polite">
        {monthLabel(month)}
      </span>
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label="Next month"
        disabled={month >= MAX_MONTH}
        onClick={() => onChange(shiftMonth(month, 1))}
      >
        <ChevronRight className="size-4" aria-hidden="true" />
      </Button>
    </div>
  )
}
