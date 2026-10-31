import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError } from '../api/client'
import { getChildren, getMySummary, getStudentSummary, type Counts, type StudentSummary } from '../api/analytics'
import { getChildFeeStatement, getOwnFeeStatement, getStudentFeeStatement, type FeeStatement } from '../api/fees'
import { usePeopleApi } from './peopleShared'
import { EmptyState, SectionCard, Skeleton, StatusBadge, ui, useNumber } from './attendanceUi'

// Attendance, results and fees on the student profile (checkpoint 4). Each
// comes from an API the viewer already uses elsewhere (staff analytics and
// fee statements, or the student's and guardian's own views), so no access
// is widened. A section the viewer may not read, or whose module is off, is
// left out.

export type InsightAccess = { mode: 'staff' | 'self' | 'child'; analytics: boolean; fees: boolean }

// Hidden, not an error: no grant, module off or no record for this viewer.
const hidden = (e: unknown) => e instanceof ApiError && [401, 403, 404].includes(e.status)

const rate = (c: Counts | undefined | null) => c && c.marked > 0 ? Math.round(((c.present + c.late) / c.marked) * 100) : null

export function StudentInsights({ studentId, access }: { studentId: string; access: InsightAccess }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const num = useNumber()
  const bn = i18n.language === 'bn'
  const summary = useQuery({
    queryKey: ['profile-insights', 'summary', access.mode, studentId], enabled: access.analytics, retry: false,
    queryFn: () => authorized(async token => {
      if (access.mode === 'staff') return getStudentSummary(token, studentId)
      if (access.mode === 'self') return getMySummary(token)
      const all = await getChildren(token)
      const mine = all.items.find(s => s.student.id === studentId)
      if (!mine) throw new ApiError(404, 'not_found')
      return mine
    }),
  })
  const fees = useQuery({
    queryKey: ['profile-insights', 'fees', access.mode, studentId], enabled: access.fees, retry: false,
    queryFn: () => authorized(token => access.mode === 'staff' ? getStudentFeeStatement(token, studentId) : access.mode === 'self' ? getOwnFeeStatement(token) : getChildFeeStatement(token, studentId)),
  })
  const money = (n: number) => `৳ ${new Intl.NumberFormat(bn ? 'bn-BD' : 'en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(n)}`
  const showSummary = access.analytics && !(summary.isError && hidden(summary.error))
  const showFees = access.fees && !(fees.isError && hidden(fees.error))
  const cardLink = (examId: string) => access.mode === 'staff' ? `/admin/exams?exam=${encodeURIComponent(examId)}&student=${encodeURIComponent(studentId)}` : access.mode === 'self' ? `/student/results?exam=${encodeURIComponent(examId)}` : ''

  return <>
    {showSummary && <SectionCard title={t('c4AttendanceSummary')} id="student-attendance" icon="check">
      {summary.isPending ? <Skeleton rows={2} /> : summary.isError ? <p role="alert" className="text-sm text-rose-700">{t('peopleLoadError')}</p> : !summary.data.attendance ? <EmptyState icon="check" title={t('c4NoAttendance')} /> :
        <Attendance data={summary.data} num={num} />}
    </SectionCard>}

    {showSummary && <SectionCard title={t('c4LatestResults')} id="student-results" icon="grade">
      {summary.isPending ? <Skeleton rows={2} /> : summary.isError ? <p role="alert" className="text-sm text-rose-700">{t('peopleLoadError')}</p> : summary.data.results.length === 0 ? <EmptyState icon="grade" title={t('c4NoResults')} /> :
        <ul className="divide-y divide-slate-100 rounded-xl border border-slate-200">{summary.data.results.slice(0, 5).map(r => <li key={r.exam_id} className="flex flex-wrap items-center gap-3 px-3 py-3">
          <span className="w-full min-w-0 sm:w-auto sm:flex-1"><strong className="block text-slate-900">{r.name}</strong><span className="block text-xs text-slate-600">{r.published_on}{r.class_position > 0 && ` · ${t('c4Position', { n: num(r.class_position) })}`}</span></span>
          <span className="mr-auto sm:mr-0 sm:text-right"><span className="block text-lg font-extrabold tabular-nums text-slate-900">{r.grade} · {num(Number(r.gpa.toFixed(2)))}</span><span className="block text-xs text-slate-600">{num(r.total_marks)}/{num(r.full_marks)}</span></span>
          <StatusBadge tone={r.passed ? 'present' : 'absent'}>{t(r.passed ? 'c4Passed' : 'c4Failed')}</StatusBadge>
          {cardLink(r.exam_id) && <Link className={`${ui.secondary} btn-sm`} to={cardLink(r.exam_id)}>{t('c4ReportCard')}</Link>}
        </li>)}</ul>}
    </SectionCard>}

    {showFees && <SectionCard title={t('c4Fees')} id="student-fees" icon="money">
      {fees.isPending ? <Skeleton rows={2} /> : fees.isError ? <p role="alert" className="text-sm text-rose-700">{t('peopleLoadError')}</p> : <Fees data={fees.data} money={money} staff={access.mode === 'staff'} />}
    </SectionCard>}
  </>
}

function Attendance({ data, num }: { data: StudentSummary; num: (n: number) => string }) {
  const { t } = useTranslation()
  const a = data.attendance!
  const month = a.months.find(m => m.period === data.today.slice(0, 7))
  const tiles: [string, Counts | undefined][] = [['c4ThisMonthRate', month], ['c4ThisYearRate', a.session]]
  return <div>
    <dl className="grid gap-3 sm:grid-cols-2">{tiles.map(([key, c]) => {
      const pct = rate(c)
      return <div key={key} className="ds-stat px-4 py-3"><dt className="text-xs font-semibold text-slate-600">{t(key)}</dt>
        <dd className="mt-1 text-2xl font-extrabold tabular-nums text-slate-900">{pct == null ? '—' : `${num(pct)}%`}</dd>
        {c && c.marked > 0 && <dd className="text-xs text-slate-600">{t('c4AttendanceCounts', { present: num(c.present), late: num(c.late), absent: num(c.absent) })}</dd>}
        {pct != null && <dd className="mt-2 h-2 overflow-hidden rounded-full bg-slate-100" aria-hidden="true"><span className="block h-full rounded-full bg-emerald-500" style={{ width: `${pct}%` }} /></dd>}
      </div>
    })}</dl>
  </div>
}

function Fees({ data, money, staff }: { data: FeeStatement; money: (n: number) => string; staff: boolean }) {
  const { t, i18n } = useTranslation()
  const bn = i18n.language === 'bn'
  const due = data.dues.filter(d => d.balance > 0).sort((a, b) => a.due_date.localeCompare(b.due_date))
  const paid = [...data.payments].filter(p => !p.voided_at).sort((a, b) => b.paid_on.localeCompare(a.paid_on)).slice(0, 3)
  return <div className="space-y-4">
    <dl className="grid grid-cols-2 gap-3">
      <div className="ds-tint px-4 py-3"><dt className="text-xs font-semibold text-slate-600">{t('c3TotalDue')}</dt><dd className="mt-1 text-xl font-extrabold tabular-nums text-slate-900">{money(data.totals.balance)}</dd></div>
      <div className="ds-stat px-4 py-3"><dt className="text-xs font-semibold text-slate-600">{t('feeArrears')}</dt><dd className={`mt-1 text-xl font-extrabold tabular-nums ${data.totals.overdue > 0 ? 'text-rose-700' : 'text-slate-900'}`}>{money(data.totals.overdue)}</dd></div>
    </dl>
    {due.length > 0 && <div><p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('c4OpenDues')}</p>
      <ul className="mt-1 divide-y divide-slate-100 rounded-xl border border-slate-200">{due.slice(0, 4).map(d => <li key={d.id} className="flex justify-between gap-2 px-3 py-2 text-sm"><span>{bn && d.category_name_bn ? d.category_name_bn : d.category_name_en}{d.period && ` · ${d.period}`}<span className="block text-xs text-slate-500">{d.due_date}</span></span><strong className="tabular-nums">{money(d.balance)}</strong></li>)}</ul></div>}
    <div><p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('c4LastPayments')}</p>
      {paid.length === 0 ? <p className="mt-1 text-sm text-slate-500">{t('feeNoPayments')}</p> :
        <ul className="mt-1 divide-y divide-slate-100 rounded-xl border border-slate-200">{paid.map(p => <li key={p.id} className="flex justify-between gap-2 px-3 py-2 text-sm"><span><span className="font-mono">{p.receipt_no}</span><span className="block text-xs text-slate-500">{p.paid_on} · {t(`feeMethod_${p.method}`)}</span></span><strong className="tabular-nums">{money(p.amount)}</strong></li>)}</ul>}</div>
    <Link className={`${ui.link} text-sm`} to={staff ? '/accounts/fees' : '/student/fees'}>{t(staff ? 'c4OpenFeeCounter' : 'c4OpenMyFees')}</Link>
  </div>
}
