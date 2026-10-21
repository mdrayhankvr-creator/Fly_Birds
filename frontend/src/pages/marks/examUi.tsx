import { useTranslation } from 'react-i18next'
import type { ExamSheet } from '../../api/client'

// Sheet states use the exam status palette of the E1 UX spec, never the
// attendance tones: words and colour, and a dashed outline for "not
// started". Every class here has a dark-theme override in styles.css.
export const sheetTones: Record<ExamSheet['status'], string> = {
  not_started: 'border border-dashed border-slate-300 text-slate-700 bg-white',
  in_progress: 'bg-sky-50 text-sky-800 ring-1 ring-inset ring-sky-600/25',
  submitted: 'bg-amber-50 text-amber-900 ring-1 ring-inset ring-amber-600/30',
  returned: 'bg-rose-50 text-rose-800 ring-1 ring-inset ring-rose-600/25',
  approved: 'bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-600/25',
}

export function SheetChip({ status }: { status: ExamSheet['status'] }) {
  const { t } = useTranslation()
  return <span className={`inline-flex shrink-0 items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${sheetTones[status]}`}>{t(`e3SheetStatus_${status}`)}</span>
}

export function Progress({ done, total, label }: { done: number; total: number; label: string }) {
  const pct = total > 0 ? Math.round(done * 100 / total) : 0
  return <div className="flex min-w-0 items-center gap-2">
    <div className="h-1.5 w-24 shrink-0 overflow-hidden rounded-full bg-slate-200" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={done} aria-label={label}>
      <div className="h-full rounded-full bg-[var(--brand-primary)] transition-[width] duration-300 motion-reduce:transition-none" style={{ width: `${pct}%` }} />
    </div>
    <span className="truncate text-xs tabular-nums text-slate-600">{label}</span>
  </div>
}
