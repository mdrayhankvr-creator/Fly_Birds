import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { saveBlob } from '../../api/admin'
import { downloadMyRoutinePdf, getMyDuties, getMyTermRoutines, type StudentExamPaper, type StudentRoutine, type TeacherDuty } from '../../api/routinePlanner'
import { Icon } from '../../components/icons'
import { Empty, Lines, Panel, Unavailable, ViewAll } from '../../components/dashboard/kit'
import { usePeopleApi } from '../peopleShared'
import { CardTitle, EmptyState, Notice, ui } from '../attendanceUi'
import { daysBetween, routineErrorOf, todayDhaka, useDay, useName, useNum, usePaperName } from './routineShared'

// The published term exam routine as students and teachers see it (E8).

function useWhen() {
  const { t } = useTranslation()
  const num = useNum()
  return (date: string) => {
    const n = daysBetween(todayDhaka(), date)
    return n === 0 ? t('e8Today') : n === 1 ? t('e8Tomorrow') : t('e8InDays', { n: num(n) })
  }
}

function nextPaper(routines: StudentRoutine[] | undefined) {
  const today = todayDhaka()
  const all = (routines ?? []).flatMap(r => r.papers.map(p => ({ ...p, term: r.term_name })))
  return all.filter(p => p.exam_date >= today).sort((a, b) => a.exam_date.localeCompare(b.exam_date) || a.start_time.localeCompare(b.start_time))[0]
}

// NextExamCard is the student's dashboard card.
export function NextExamCard() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const day = useDay()
  const when = useWhen()
  const name = useName()
  const paperName = usePaperName()
  const num = useNum()
  const q = useQuery({ queryKey: ['routine', 'term', 'me'], queryFn: () => authorized(getMyTermRoutines), retry: false })
  const next = nextPaper(q.data)
  return <Panel title={t('e8NextExam')} icon="calendar" module="exams" action={<ViewAll to="/student/routine?tab=exam" label={t('e8ViewRoutine')} />}>
    {q.isPending ? <Lines rows={2} /> : q.isError ? <Unavailable error={q.error} /> : !q.data.length ? <Empty>{t('e8NoPublished')}</Empty> : !next ? <Empty>{t('e8NoNextExam')}</Empty> :
      <div className="ds-softer ds-edge rounded-xl px-4 py-3" data-testid="next-exam">
        <p className="text-xs font-semibold uppercase tracking-wide text-[var(--brand-ink)]">{when(next.exam_date)} · {next.term}</p>
        <p className="mt-1 text-lg font-bold text-slate-900">{paperName(next)}{next.changed && <span className="ml-2 align-middle rounded-full bg-amber-50 px-2 py-0.5 text-xs font-semibold text-amber-900 ring-1 ring-inset ring-amber-600/25">{t('e8ChangedBadge')}</span>}</p>
        <p className="mt-1 text-sm text-slate-700">{day(next.exam_date, true)} · <span className="tabular-nums">{num(next.start_time)}–{num(next.end_time)}</span> · {name(next.sitting_name, next.sitting_name_bn)}</p>
        {next.room_name && <p className="mt-0.5 text-sm font-semibold text-slate-800">{t('e8RoomLabel', { room: name(next.room_name, next.room_name_bn) })}</p>}
      </div>}
  </Panel>
}

function PaperRow({ p, today }: { p: StudentExamPaper; today: string }) {
  const { t } = useTranslation()
  const day = useDay()
  const name = useName()
  const num = useNum()
  const paperName = usePaperName()
  const done = p.exam_date < today
  return <tr className={`border-b border-slate-100 ${p.changed ? 'bg-amber-50' : ''} ${done ? 'text-slate-500' : ''}`}>
    <td className={ui.td}><span className="font-semibold text-slate-900">{day(p.exam_date)}</span>{done && <span className="ml-2 text-xs">{t('e8Done')}</span>}</td>
    <td className={`${ui.td} tabular-nums`}>{num(p.start_time)}–{num(p.end_time)}<span className="block text-xs text-slate-500">{name(p.sitting_name, p.sitting_name_bn)}</span></td>
    <td className={ui.td}><span className="font-semibold text-slate-900">{paperName(p)}</span>{p.optional && <span className="ml-1 text-xs text-slate-500">({t('e8Optional')})</span>}
      {p.changed && <span className="ml-2 rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-amber-900 ring-1 ring-inset ring-amber-600/25">{t('e8ChangedBadge')}</span>}</td>
    <td className={ui.td}>{p.room_name ? name(p.room_name, p.room_name_bn) : '—'}</td>
  </tr>
}

// StudentExamRoutine is the student's full term routine with downloads.
export function StudentExamRoutine() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const day = useDay()
  const [error, setError] = useState('')
  const q = useQuery({ queryKey: ['routine', 'term', 'me'], queryFn: () => authorized(getMyTermRoutines), retry: false })
  const today = todayDhaka()
  async function download(r: StudentRoutine, lang: 'en' | 'bn') {
    setError('')
    try { saveBlob(await authorized(token => downloadMyRoutinePdf(token, r.version_id, lang)), `exam-routine-${lang}.pdf`) }
    catch (cause) { setError(t(routineErrorOf(cause).key === 'e8DocUnavailable' ? 'e8DocUnavailable' : 'e8DocError')) }
  }
  if (q.isPending) return <section className={`${ui.card} p-4 sm:p-6`}><Lines rows={3} /></section>
  if (q.isError) return <section className={`${ui.card} p-4 sm:p-6`}><Unavailable error={q.error} /></section>
  if (!q.data.length) return null
  return <>{q.data.map(r => <section key={r.version_id} className={`${ui.card} p-4 sm:p-6`} aria-labelledby={`term-${r.version_id}`}>
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <CardTitle id={`term-${r.version_id}`} icon="calendar">{t('e8MyExams')} · {r.term_name}</CardTitle>
        <p className="mt-1 text-sm text-slate-600">{r.class_name} · {r.section_name}</p>
      </div>
      <div className="flex flex-wrap gap-2 print:hidden">
        <button type="button" className={ui.secondary} onClick={() => void download(r, 'en')}><Icon name="download" className="h-4 w-4" />{t('e8DownloadMine')} ({t('e8English')})</button>
        <button type="button" className={ui.secondary} onClick={() => void download(r, 'bn')}><Icon name="download" className="h-4 w-4" />{t('e8DownloadMine')} ({t('e8Bangla')})</button>
      </div>
    </div>
    {r.amended_on && <div className="mt-3"><Notice tone="warning">{t('e8AmendedOn', { date: day(r.amended_on.slice(0, 10), true) })}{r.reason ? ` — ${r.reason}` : ''}</Notice></div>}
    {error && <div className="mt-3"><Notice tone="error">{error}</Notice></div>}
    {r.papers.length === 0 ? <div className="mt-4"><EmptyState title={t('e8NoNextExam')} icon="calendar" /></div> :
      <div className="mt-4 overflow-x-auto"><table className={`${ui.table} min-w-[36rem]`}>
        <thead><tr><th className={ui.th}>{t('e8Date')}</th><th className={ui.th}>{t('e8Sitting')}</th><th className={ui.th}>{t('e8Paper')}</th><th className={ui.th}>{t('e8Room')}</th></tr></thead>
        <tbody>{r.papers.map(p => <PaperRow key={p.paper_id} p={p} today={today} />)}</tbody>
      </table></div>}
  </section>)}</>
}

function DutyLine({ d }: { d: TeacherDuty }) {
  const { t, i18n } = useTranslation()
  const day = useDay()
  const name = useName()
  const num = useNum()
  return <>
    <p className="font-semibold text-slate-900">{day(d.exam_date)} · <span className="tabular-nums">{num(d.start_time)}–{num(d.end_time)}</span></p>
    <p className="text-sm text-slate-700">{name(d.room_name, d.room_name_bn)} · {name(d.sitting_name, d.sitting_name_bn)} · {t('e8DutyStudents', { n: num(d.students) })}</p>
    <p className="text-xs text-slate-500">{(i18n.language === 'bn' ? d.papers_bn : d.papers).join('; ')}</p>
  </>
}

// DutiesCard is the teacher's dashboard card.
export function DutiesCard() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const when = useWhen()
  const q = useQuery({ queryKey: ['routine', 'duties', 'me'], queryFn: () => authorized(getMyDuties), retry: false })
  const today = todayDhaka()
  const next = (q.data ?? []).filter(d => d.exam_date >= today).slice(0, 3)
  return <Panel title={t('e8NextDuty')} icon="clock" module="exams" action={<ViewAll to="/teacher/routine?tab=duties" />}>
    {q.isPending ? <Lines rows={2} /> : q.isError ? <Unavailable error={q.error} /> : next.length === 0 ? <Empty>{t('e8NoMyDuties')}</Empty> :
      <ul className="space-y-2" data-testid="next-duties">{next.map((d, i) => <li key={`${d.version_id}-${d.exam_date}-${d.start_time}-${i}`} className="ds-softer ds-edge rounded-xl px-3.5 py-2.5">
        <p className="text-xs font-semibold uppercase tracking-wide text-[var(--brand-ink)]">{when(d.exam_date)}</p><DutyLine d={d} /></li>)}</ul>}
  </Panel>
}

// TeacherDutyList is the teacher's full list of duties.
export function TeacherDutyList() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const q = useQuery({ queryKey: ['routine', 'duties', 'me'], queryFn: () => authorized(getMyDuties), retry: false })
  const today = todayDhaka()
  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="my-duties">
    <CardTitle id="my-duties" icon="clock">{t('e8MyDuties')}</CardTitle>
    {q.isPending ? <div className="mt-4"><Lines rows={3} /></div> : q.isError ? <div className="mt-4"><Unavailable error={q.error} /></div> : q.data.length === 0 ? <div className="mt-4"><EmptyState title={t('e8NoMyDuties')} icon="calendar" /></div> :
      <ul className="mt-4 grid gap-2 sm:grid-cols-2">{q.data.map((d, i) => <li key={`${d.version_id}-${d.exam_date}-${d.start_time}-${i}`} className={`rounded-xl border px-3.5 py-2.5 ${d.exam_date < today ? 'border-slate-200 opacity-70' : 'border-slate-200 border-l-[3px] border-l-[var(--s)]'}`}>
        <p className="text-xs font-semibold text-slate-500">{d.term_name}{d.exam_date < today ? ` · ${t('e8Done')}` : ''}</p><DutyLine d={d} /></li>)}</ul>}
  </section>
}
