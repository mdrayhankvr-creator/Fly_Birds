import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getStudentEnrolments, type EnrolmentEvent, type StudentEnrolment } from '../api/client'
import { EmptyState, Notice, SectionCard, Skeleton, useNumber } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

function day(value: string | null, lang: string) {
  if (!value) return ''
  const [y, m, d] = value.slice(0, 10).split('-').map(Number)
  return new Intl.DateTimeFormat(lang === 'bn' ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}

const OUTCOME_TONE: Record<string, string> = {
  promoted: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20', graduated: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20',
  repeated: 'bg-amber-50 text-amber-900 ring-amber-600/25', withdrawn: 'bg-rose-50 text-rose-800 ring-rose-600/20',
  transferred: 'bg-sky-50 text-sky-800 ring-sky-600/20', moved: 'bg-sky-50 text-sky-800 ring-sky-600/20',
}

// AcademicHistory lists a student's school years, oldest first, from
// GET /students/{id}/enrolments. The server decides who may read it and shows
// who made each change only to the administrator and head teacher.
export function AcademicHistory({ studentId }: { studentId: string }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const history = useQuery({ queryKey: ['people', 'enrolments', studentId], queryFn: () => authorized(token => getStudentEnrolments(token, studentId)), retry: false })
  return <SectionCard title={t('p2HistoryTitle')} description={t('p2HistoryHelp')} id="student-history" icon="calendar">
    {history.isPending ? <Skeleton rows={2} /> : history.isError ? <Notice tone="error">{t('peopleLoadError')}</Notice>
      : history.data.length === 0 ? <EmptyState icon="calendar" title={t('p2HistoryEmpty')} />
        : <ol className="relative space-y-4 border-l-2 border-dashed border-slate-200 pl-5 sm:pl-6">
          {[...history.data].reverse().map(year => <Year key={year.id} year={year} lang={i18n.language} />)}
        </ol>}
  </SectionCard>
}

function Year({ year, lang }: { year: StudentEnrolment; lang: string }) {
  const { t } = useTranslation()
  const num = useNumber()
  const current = year.state === 'current'
  const dates = year.started_on && year.ended_on ? t('p2HistoryDates', { from: day(year.started_on, lang), to: day(year.ended_on, lang) })
    : year.started_on ? t('p2HistorySince', { date: day(year.started_on, lang) }) : ''
  return <li className="relative">
    <span aria-hidden="true" className={`absolute -left-[29px] top-4 grid h-4 w-4 place-items-center rounded-full ring-4 ring-[var(--ds-surface)] sm:-left-[33px] ${current ? 'ds-fill' : 'bg-slate-300'}`} />
    <article className={`rounded-2xl border p-4 ${current ? 'ds-tint' : 'border-slate-200'}`} aria-label={year.session_name}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="text-base font-bold text-slate-900">{year.session_name}</h3>
          <p className="mt-0.5 text-sm text-slate-700">{year.class_name} · {year.section_name}{year.roll_no != null && <> · {t('p2HistoryRoll', { roll: num(year.roll_no) })}</>}</p>
          {dates && <p className="mt-0.5 text-xs text-slate-500">{dates}</p>}
        </div>
        <div className="flex flex-wrap gap-1.5">
          <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ring-1 ring-inset ${current ? 'ds-soft ds-ink ring-transparent' : 'bg-slate-100 text-slate-700 ring-slate-500/20'}`}>{t(current ? 'p2HistoryCurrent' : 'p2HistoryClosed')}</span>
          {year.outcome && <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ring-1 ring-inset ${OUTCOME_TONE[year.outcome]}`}>{t(`p2Outcome_${year.outcome}`)}</span>}
        </div>
      </div>
      {(year.source === 'backfill' || year.source === 'inferred') && <p className="mt-2 text-xs italic text-slate-500">{t('p2HistoryEarlier')}</p>}
      {year.events.length > 0 && <div className="mt-3 border-t border-slate-100 pt-3">
        <h4 className="text-xs font-semibold text-slate-500">{t('p2HistoryChanges')}</h4>
        <ul className="mt-1.5 space-y-1.5">{year.events.map((e, i) => <li key={i} className="flex flex-wrap items-baseline gap-x-2 text-sm">
          <span className="font-semibold text-slate-800">{t(`p2Event_${e.kind}`)}</span>
          <Change event={e} />
          <span className="text-xs text-slate-500">{day(e.occurred_at, lang)}{e.actor_name && <> · {t('p2HistoryBy', { name: e.actor_name })}</>}</span>
        </li>)}</ul>
      </div>}
    </article>
  </li>
}

// Change shows what moved, when the event carries a before and after.
function Change({ event: e }: { event: EnrolmentEvent }) {
  const { t } = useTranslation()
  const num = useNumber()
  const show = (v: string) => e.kind === 'status_changed' ? t(`${v}Student`) : e.kind === 'roll_changed' && /^\d+$/.test(v) ? num(Number(v)) : v
  const [from, to] = e.kind === 'class_changed' ? [e.from_class_name, e.to_class_name]
    : e.kind === 'section_changed' ? [e.from_section_name, e.to_section_name]
      : e.kind === 'roll_changed' || e.kind === 'status_changed' || e.kind === 'session_corrected' ? [e.from_value && show(e.from_value), e.to_value && show(e.to_value)] : [null, null]
  if (!from && !to) return null
  return <span className="text-slate-600">{from || '—'} <span aria-hidden="true">→</span><span className="sr-only">,</span> {to || '—'}</span>
}
