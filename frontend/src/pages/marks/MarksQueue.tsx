import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { getMyExamSheets, type ExamSheetQueueItem } from '../../api/client'
import { EmptyState, Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { Progress, SheetChip } from './examUi'

type Filter = 'todo' | 'submitted' | 'approved' | 'all'
const inFilter: Record<Filter, (s: ExamSheetQueueItem) => boolean> = {
  todo: s => s.exam_status === 'draft' && (s.status === 'not_started' || s.status === 'in_progress' || s.status === 'returned'),
  submitted: s => s.status === 'submitted',
  approved: s => s.status === 'approved',
  all: () => true,
}

// MarksQueue is the teacher's "My exam sheets": every sheet of an assigned
// subject, work that needs the teacher first, one tap to open a sheet.
export function MarksQueue() {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const [filter, setFilter] = useState<Filter>('todo')
  const sheets = useQuery({ queryKey: ['marks', 'queue'], gcTime: 0, queryFn: () => authorized(token => getMyExamSheets(token)), retry: false })
  const items = (sheets.data || []).filter(inFilter[filter])
  const counts = Object.fromEntries((Object.keys(inFilter) as Filter[]).map(f => [f, (sheets.data || []).filter(inFilter[f]).length])) as Record<Filter, number>
  const subject = (s: ExamSheetQueueItem) => i18n.language === 'bn' && s.subject_name_bn ? s.subject_name_bn : s.subject_name_en

  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_academics')} title={t('mwQueueTitle')} description={t('mwQueueIntro')} />
    <div role="tablist" aria-label={t('mwRowFilter')} className="flex flex-wrap gap-1 rounded-xl bg-slate-100 p-1 sm:w-fit">
      {(['todo', 'submitted', 'approved', 'all'] as const).map(f => <button key={f} type="button" role="tab" aria-selected={filter === f} onClick={() => setFilter(f)}
        className={`min-h-9 rounded-lg px-3 text-sm font-semibold transition-colors ${filter === f ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'}`}>
        {t(`mwFilter${f === 'todo' ? 'Todo' : f === 'submitted' ? 'Submitted' : f === 'approved' ? 'Approved' : 'All'}`)} <span className="tabular-nums text-slate-500">{counts[f]}</span>
      </button>)}
    </div>
    {sheets.isPending ? <Skeleton rows={4} /> : sheets.isError ? <Notice tone="error">{t('mwLoadError')}</Notice> : items.length === 0
      ? <EmptyState icon="grade" title={t(filter === 'todo' ? 'mwQueueEmptyTodo' : 'mwQueueEmpty')} />
      : <ul className={`${ui.card} divide-y divide-slate-100 overflow-hidden`}>
        {items.map(s => {
          const writable = s.exam_status === 'draft' && (s.status === 'not_started' || s.status === 'in_progress' || s.status === 'returned')
          return <li key={s.sheet_id}>
            <Link to={`/teacher/exams/${s.exam_id}/sheets/${s.sheet_id}`} className="grid gap-x-4 gap-y-2 px-4 py-3.5 transition-colors hover:bg-slate-50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-[var(--brand-primary)] sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:px-5">
              <span className="min-w-0">
                <span className="flex flex-wrap items-center gap-2"><span className="font-semibold text-slate-900">{subject(s)}</span><SheetChip status={s.status} /></span>
                <span className="mt-0.5 block truncate text-sm text-slate-600">{s.class_name} · {s.section_name} · {s.exam_name} · {s.session_name}</span>
                {s.status === 'returned' && s.return_reason && <span className="mt-1 block text-sm text-rose-800">{t('e3ReturnedBecause', { reason: s.return_reason })}</span>}
              </span>
              <span className="flex flex-wrap items-center gap-x-4 gap-y-1 sm:justify-end">
                <Progress done={s.entered} total={s.students} label={s.missing > 0 ? t('mwMissing', { n: s.missing }) : t('mwComplete')} />
                <span className="text-sm font-semibold text-[var(--brand-ink)]">{writable ? t('mwOpen') : t('mwView')} →</span>
              </span>
            </Link>
          </li>
        })}
      </ul>}
  </div>
}
