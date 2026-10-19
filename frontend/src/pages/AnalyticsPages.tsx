import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
import { ApiError, getMyPermissions } from '../api/client'
import { saveBlob } from '../api/admin'
import {
  downloadReport, getAttendance, getChildren, getExams, getFees, getMySummary, getOptions, getOverview, getReport, getReports, getRisks, getSessions,
  getStudentSummary, saveAlertSettings, type AlertSettings, type Filter, type Options, type ReportKey, type Risk, type StudentSummary,
} from '../api/analytics'
import { EmptyState, Notice, PageHeader, Pager, Skeleton, StatusBadge, Tabs, ui, type Tone } from './attendanceUi'
import { useApi, useLocal } from './commShared'
import { BarList, ColumnChart, LineChart, Meter } from './analyticsCharts'

// ---------- formatting ----------

export function useFmt() {
  const { i18n } = useTranslation()
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  const num = (n: number, digits = 0) => new Intl.NumberFormat(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(n)
  return {
    num: (n: number) => num(n),
    pct: (n: number | null | undefined) => (n === null || n === undefined ? '—' : `${num(n, 1)}%`),
    gpa: (n: number | null | undefined) => (n === null || n === undefined ? '—' : num(n, 2)),
    money: (n: number) => `${n < 0 ? '-' : ''}৳ ${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(Math.abs(n))}`,
    day: (d: string) => new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(`${d}T00:00:00Z`)),
    fullDay: (d: string) => new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${d}T00:00:00Z`)),
    month: (d: string) => new Intl.DateTimeFormat(locale, { month: 'short', year: '2-digit', timeZone: 'UTC' }).format(new Date(`${d.slice(0, 7)}-01T00:00:00Z`)),
  }
}

function useAnalyticsError() {
  const { t } = useTranslation()
  return (cause: unknown) => {
    if (cause instanceof ApiError) {
      if (cause.code === 'module_disabled') return t('m11ErrModule')
      if (cause.status === 403) return t('m11ErrForbidden')
      if (cause.status === 400) return t('m11ErrInvalid')
      if (cause.status === 404) return t('m11ErrNotFound')
    }
    return t('m11ErrGeneric')
  }
}

function Failure({ error, retry }: { error: unknown; retry?: () => void }) {
  const { t } = useTranslation()
  const text = useAnalyticsError()
  return <Notice tone="error" action={retry && <button type="button" className={ui.secondary} onClick={retry}>{t('m11Retry')}</button>}>{text(error)}</Notice>
}

function Kpi({ label, value, hint, meter, tone }: { label: string; value: ReactNode; hint?: ReactNode; meter?: number | null; tone?: 'alert' | 'good' }) {
  return <div className={`${ui.card} p-4`}>
    <p className="text-xs font-semibold tracking-wide text-slate-500">{label}</p>
    <p className={`mt-1 text-2xl font-bold tabular-nums ${tone === 'alert' ? 'text-rose-700' : 'text-slate-900'}`}>{value}</p>
    {meter !== undefined && <Meter value={meter} label={label} />}
    {hint && <p className="mt-1 text-sm text-slate-600">{hint}</p>}
  </div>
}

function Card({ title, children, actions, className = '' }: { title: string; children: ReactNode; actions?: ReactNode; className?: string }) {
  return <section className={`${ui.card} p-4 sm:p-5 ${className}`}>
    <div className="mb-3 flex flex-wrap items-center justify-between gap-2"><h2 className="text-base font-semibold text-slate-900">{title}</h2>{actions}</div>
    {children}
  </section>
}

// ---------- filters ----------

type FilterKeys = { dates?: boolean; section?: boolean; exam?: boolean }

function FilterBar({ options, filter, onChange, keys }: { options: Options; filter: Filter; onChange: (f: Filter) => void; keys: FilterKeys }) {
  const { t } = useTranslation()
  const sections = options.sections.filter(s => !filter.class_id || s.class_id === filter.class_id)
  const exams = options.exams.filter(e => (!filter.class_id || e.class_id === filter.class_id) && (!filter.section_id || e.section_id === filter.section_id))
  const set = (patch: Filter) => onChange({ ...filter, ...patch })
  const field = 'min-w-0 flex-1 basis-36'
  return <form className={`${ui.card} flex flex-wrap items-end gap-3 p-3 print:hidden`} onSubmit={e => e.preventDefault()} aria-label={t('m11Filters')}>
    <label className={field}><span className={ui.label}>{t('m11Session')}</span>
      <select className={ui.field} value={filter.session_id ?? ''} onChange={e => onChange({ session_id: e.target.value, from: filter.from, to: filter.to })}>
        {options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}{s.is_active ? ` · ${t('m11Active')}` : ''}</option>)}</select></label>
    <label className={field}><span className={ui.label}>{t('m11Class')}</span>
      <select className={ui.field} value={filter.class_id ?? ''} onChange={e => set({ class_id: e.target.value, section_id: '', exam_id: '' })}>
        <option value="">{t('m11AllClasses')}</option>{options.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
    {keys.section && <label className={field}><span className={ui.label}>{t('m11Section')}</span>
      <select className={ui.field} value={filter.section_id ?? ''} onChange={e => set({ section_id: e.target.value, exam_id: '' })}>
        <option value="">{t('m11AllSections')}</option>{sections.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>}
    {keys.exam && <label className={field}><span className={ui.label}>{t('m11Exam')}</span>
      <select className={ui.field} value={filter.exam_id ?? ''} onChange={e => set({ exam_id: e.target.value })}>
        <option value="">{t('m11AllExams')}</option>{exams.map(x => <option key={x.id} value={x.id}>{x.name}</option>)}</select></label>}
    {keys.dates && <>
      <label className={field}><span className={ui.label}>{t('m11From')}</span><input type="date" className={ui.field} value={filter.from ?? ''} max={filter.to || options.today} onChange={e => set({ from: e.target.value })} /></label>
      <label className={field}><span className={ui.label}>{t('m11To')}</span><input type="date" className={ui.field} value={filter.to ?? ''} max={options.today} onChange={e => set({ to: e.target.value })} /></label>
    </>}
    <button type="button" className={ui.secondary} onClick={() => onChange({ session_id: filter.session_id })}>{t('m11Reset')}</button>
  </form>
}

function useOptions(session: string) {
  const api = useApi()
  return useQuery({ queryKey: ['analytics', 'options', session], queryFn: () => api(token => getOptions(token, session)), retry: false, staleTime: 60_000 })
}

// ---------- school and teacher analytics ----------

type TabKey = 'overview' | 'attendance' | 'results' | 'fees' | 'alerts' | 'sessions'
export type AnalyticsAccess = { school: boolean; teacher: boolean; finance: boolean; manage: boolean }

export function AnalyticsPage({ access }: { access: AnalyticsAccess }) {
  const { t } = useTranslation()
  const academic = access.school || access.teacher
  const tabs: TabKey[] = ([
    access.school && 'overview', academic && 'attendance', academic && 'results', access.finance && 'fees', academic && 'alerts', access.school && 'sessions',
  ].filter(Boolean) as TabKey[])
  const [tab, setTab] = useState<TabKey>(tabs[0] ?? 'fees')
  const [filter, setFilter] = useState<Filter>({})
  const options = useOptions(filter.session_id ?? '')
  const opts = options.data
  const active: Filter = { ...filter, session_id: filter.session_id || opts?.session?.id || '' }
  const moduleFor: Partial<Record<TabKey, string>> = { attendance: 'attendance', results: 'examinations', fees: 'fees' }
  const off = opts && moduleFor[tab] && !opts.modules[moduleFor[tab]!]
  return <section className="space-y-5 analytics-print">
    <PageHeader eyebrow={t('m11Eyebrow')} title={access.teacher && !access.school ? t('m11TeacherTitle') : t('m11Title')}
      description={access.teacher && !access.school ? t('m11TeacherIntro') : access.school ? t('m11Intro') : t('m11FinanceIntro')}
      actions={<><Link to="/reports" className={ui.secondary}>{t('m11Reports')}</Link><button type="button" className={ui.secondary} onClick={() => window.print()}>{t('m11Print')}</button></>} />
    {tabs.length > 1 && <Tabs label={t('m11Sections')} value={tab} onChange={setTab} items={tabs.map(k => ({ value: k, label: t(`m11Tab_${k}`) }))} />}
    {options.isPending ? <Skeleton rows={4} /> : options.isError ? <Failure error={options.error} retry={() => void options.refetch()} /> : !opts?.session ? <EmptyState title={t('m11NoSession')}>{t('m11NoSessionHint')}</EmptyState> : <>
      {tab !== 'sessions' && <FilterBar options={opts} filter={active} onChange={setFilter} keys={{ dates: tab === 'attendance' || tab === 'fees', section: tab !== 'fees', exam: tab === 'results' }} />}
      {off ? <Notice tone="info">{t('m11ModuleOff')}</Notice> : <>
        {tab === 'overview' && <OverviewTab session={active.session_id!} onOpen={setTab} />}
        {tab === 'attendance' && <AttendanceTab filter={active} />}
        {tab === 'results' && <ResultsTab filter={active} />}
        {tab === 'fees' && <FeesTab filter={active} />}
        {tab === 'alerts' && <AlertsTab filter={active} settings={opts.settings} canManage={access.manage} modules={opts.modules} />}
        {tab === 'sessions' && <SessionsTab />}
      </>}
    </>}
  </section>
}

function OverviewTab({ session, onOpen }: { session: string; onOpen: (tab: TabKey) => void }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'overview', session], queryFn: () => api(token => getOverview(token, session)), retry: false })
  if (q.isPending) return <Skeleton rows={4} />
  if (q.isError) return <Failure error={q.error} retry={() => void q.refetch()} />
  const d = q.data
  const insights: { tone: 'warning' | 'info' | 'success'; text: string; tab: TabKey }[] = []
  if (d.risks && d.risks.dropout > 0) insights.push({ tone: 'warning', text: t('m11InsightDropout', { n: f.num(d.risks.dropout), count: d.risks.dropout }), tab: 'alerts' })
  if (d.risks && d.risks.at_risk > 0) insights.push({ tone: 'info', text: t('m11InsightAtRisk', { n: f.num(d.risks.at_risk), count: d.risks.at_risk }), tab: 'alerts' })
  if (d.today_students && d.active_students > 0 && d.today_students.marked < d.active_students) insights.push({ tone: 'info', text: t('m11InsightUnmarked', { n: f.num(d.active_students - d.today_students.marked), count: d.active_students - d.today_students.marked }), tab: 'attendance' })
  if (d.fees && d.fees.overdue > 0) insights.push({ tone: 'warning', text: t('m11InsightOverdue', { amount: f.money(d.fees.overdue) }), tab: 'fees' })
  if (d.results && d.results.pass_rate !== null && d.results.pass_rate < 80) insights.push({ tone: 'info', text: t('m11InsightPassRate', { rate: f.pct(d.results.pass_rate) }), tab: 'results' })
  return <div className="space-y-5">
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Kpi label={t('m11ActiveStudents')} value={f.num(d.active_students)} hint={t('m11StaffLine', { teachers: f.num(d.teachers), staff: f.num(d.non_teaching_staff) })} />
      <Kpi label={t('m11TodayAttendance')} value={f.pct(d.today_students?.rate)} meter={d.today_students?.rate} hint={d.today_students ? t('m11MarkedOf', { marked: f.num(d.today_students.marked), total: f.num(d.active_students) }) : t('m11ModuleOffShort')} />
      <Kpi label={t('m11Last30')} value={f.pct(d.last_30_days?.rate)} meter={d.last_30_days?.rate} hint={d.today_teachers ? t('m11TeachersToday', { rate: f.pct(d.today_teachers.rate) }) : undefined} />
      <Kpi label={t('m11PassRate')} value={f.pct(d.results?.pass_rate)} meter={d.results?.pass_rate} hint={d.results ? t('m11AvgGpa', { gpa: f.gpa(d.results.avg_gpa) }) : t('m11ModuleOffShort')} />
      <Kpi label={t('m11DropoutAlerts')} value={f.num(d.risks?.dropout ?? 0)} tone={d.risks?.dropout ? 'alert' : undefined} hint={t('m11AtRiskCount', { n: f.num(d.risks?.at_risk ?? 0) })} />
      <Kpi label={t('m11Guardians')} value={f.num(d.guardians)} />
      {d.fees && <><Kpi label={t('m11Outstanding')} value={f.money(d.fees.outstanding)} hint={t('m11OverdueAmount', { amount: f.money(d.fees.overdue) })} />
        <Kpi label={t('m11CollectionRate')} value={f.pct(d.fees.collection_rate)} meter={d.fees.collection_rate} hint={t('m11Last12Months')} /></>}
    </div>
    <div className="grid gap-5 lg:grid-cols-2">
      <Card title={t('m11Insights')}>
        {insights.length === 0 ? <p className="text-sm text-slate-600">{t('m11NoInsights')}</p> :
          <ul className="space-y-2">{insights.map((i, n) => <li key={n}><Notice tone={i.tone} action={<button type="button" className={ui.ghost} onClick={() => onOpen(i.tab)}>{t('m11Review')}</button>}>{i.text}</Notice></li>)}</ul>}
      </Card>
      <Card title={t('m11StudentsByClass')}>
        <BarList title={t('m11StudentsByClass')} empty={t('m11NoData')} items={d.by_class.map(c => ({ key: c.id, label: c.name, value: c.students, display: f.num(c.students) }))} />
      </Card>
    </div>
  </div>
}

function periodLabel(f: ReturnType<typeof useFmt>, group: string | undefined, period: string) {
  return group === 'month' ? f.month(period) : f.day(period)
}

function AttendanceTab({ filter }: { filter: Filter }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'attendance', filter], queryFn: () => api(token => getAttendance(token, filter)), retry: false })
  if (q.isPending) return <Skeleton rows={4} />
  if (q.isError) return <Failure error={q.error} retry={() => void q.refetch()} />
  const d = q.data, s = d.summary, group = d.filter.group
  return <div className="space-y-5">
    <p className="text-sm text-slate-600">{t('m11RangeShown', { from: f.fullDay(d.filter.from!), to: f.fullDay(d.filter.to!) })}</p>
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Kpi label={t('m11AttendanceRate')} value={f.pct(s.rate)} meter={s.rate} hint={t('m11RateRule')} />
      <Kpi label={t('m11Marks')} value={f.num(s.marked)} hint={t('m11DaysStudents', { days: f.num(d.days), students: f.num(d.students) })} />
      <Kpi label={t('m11PresentLate')} value={`${f.num(s.present)} · ${f.num(s.late)}`} />
      <Kpi label={t('m11Absences')} value={f.num(s.absent)} tone={s.absent ? 'alert' : undefined} />
    </div>
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
      <Card title={t(`m11Trend_${group ?? 'day'}`)}>
        <LineChart title={t('m11AttendanceTrend')} empty={t('m11NoAttendance')} format={v => f.pct(v)}
          points={d.trend.map(p => ({ label: periodLabel(f, group, p.period), value: p.rate, detail: t('m11TrendDetail', { present: f.num(p.present), late: f.num(p.late), absent: f.num(p.absent) }) }))} />
      </Card>
      <Card title={t(d.group_by === 'class' ? 'm11ByClass' : 'm11BySection')}>
        <BarList title={t('m11AttendanceRate')} max={100} empty={t('m11NoAttendance')}
          items={d.groups.map(g => ({ key: g.id, label: g.name, value: g.rate, display: f.pct(g.rate), hint: t('m11GroupHint', { marked: f.num(g.marked), absent: f.num(g.absent) }) }))} />
      </Card>
    </div>
    {d.teachers && <Card title={t('m11TeacherAttendance')}>
      <div className="grid gap-5 md:grid-cols-[14rem_minmax(0,1fr)]">
        <dl className="grid grid-cols-2 gap-3 text-sm md:grid-cols-1">
          <div><dt className="text-slate-500">{t('m11AttendanceRate')}</dt><dd className="text-xl font-bold tabular-nums text-slate-900">{f.pct(d.teachers.rate)}</dd></div>
          <div><dt className="text-slate-500">{t('m11TeacherCounts')}</dt><dd className="tabular-nums text-slate-800">{t('m11TeacherCountsValue', { present: f.num(d.teachers.present), absent: f.num(d.teachers.absent), leave: f.num(d.teachers.leave) })}</dd></div>
          <p className="col-span-2 text-xs text-slate-500 md:col-span-1">{t('m11TeacherRateRule')}</p>
        </dl>
        <LineChart title={t('m11TeacherAttendance')} empty={t('m11NoAttendance')} format={v => f.pct(v)} height={160}
          points={(d.teacher_trend ?? []).map(p => ({ label: periodLabel(f, group, p.period), value: p.rate }))} />
      </div>
    </Card>}
  </div>
}

function ResultsTab({ filter }: { filter: Filter }) {
  const { t } = useTranslation()
  const f = useFmt()
  const local = useLocal()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'exams', filter], queryFn: () => api(token => getExams(token, { ...filter, from: '', to: '' })), retry: false })
  if (q.isPending) return <Skeleton rows={4} />
  if (q.isError) return <Failure error={q.error} retry={() => void q.refetch()} />
  const d = q.data, s = d.summary
  if (s.results === 0) return <EmptyState title={t('m11NoResults')}>{t('m11NoResultsHint')}</EmptyState>
  return <div className="space-y-5">
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Kpi label={t('m11PassRate')} value={f.pct(s.pass_rate)} meter={s.pass_rate} hint={t('m11PassedOf', { passed: f.num(s.passed), total: f.num(s.results) })} />
      <Kpi label={t('m11AverageGpa')} value={f.gpa(s.avg_gpa)} />
      <Kpi label={t('m11Gpa5')} value={f.num(s.gpa5)} />
      <Kpi label={t('m11Failed')} value={f.num(s.failed)} tone={s.failed ? 'alert' : undefined} />
    </div>
    <div className="grid gap-5 lg:grid-cols-2">
      <Card title={t('m11GpaDistribution')}>
        <ColumnChart title={t('m11GpaDistribution')} empty={t('m11NoResults')} format={v => f.num(v)} labels={d.distribution.map(g => g.grade)}
          series={[{ key: 'count', label: t('m11Students'), values: d.distribution.map(g => g.count) }]} />
      </Card>
      <Card title={t(d.group_by === 'class' ? 'm11PassRateByClass' : 'm11PassRateBySection')}>
        <BarList title={t('m11PassRate')} max={100} empty={t('m11NoResults')} items={d.groups.map(g => ({ key: g.id, label: g.name, value: g.pass_rate, display: f.pct(g.pass_rate), hint: t('m11GroupResultHint', { n: f.num(g.results), gpa: f.gpa(g.avg_gpa) }) }))} />
      </Card>
      <Card title={t('m11SubjectPerformance')}>
        <BarList title={t('m11SubjectPerformance')} max={100} empty={t('m11NoResults')} items={d.subjects.map(s => ({ key: s.subject_id, label: local(s.name_en, s.name_bn), value: s.pass_rate, display: f.pct(s.pass_rate), hint: t('m11SubjectHint', { avg: f.pct(s.avg_percent), n: f.num(s.entries) }) }))} />
      </Card>
      <Card title={t('m11ExamPassRates')}>
        <BarList title={t('m11ExamPassRates')} max={100} empty={t('m11NoResults')} items={d.exams.map(e => ({ key: e.exam_id, label: `${e.name} · ${e.class_name} ${e.section_name}`, value: e.pass_rate, display: f.pct(e.pass_rate), hint: t('m11ExamHint', { date: e.published_on ? f.fullDay(e.published_on) : '—', n: f.num(e.results), gpa: f.gpa(e.avg_gpa) }) }))} />
      </Card>
    </div>
  </div>
}

function FeesTab({ filter }: { filter: Filter }) {
  const { t } = useTranslation()
  const f = useFmt()
  const local = useLocal()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'fees', filter], queryFn: () => api(token => getFees(token, { session_id: filter.session_id, class_id: filter.class_id, from: filter.from, to: filter.to })), retry: false })
  if (q.isPending) return <Skeleton rows={4} />
  if (q.isError) return <Failure error={q.error} retry={() => void q.refetch()} />
  const d = q.data, s = d.summary
  return <div className="space-y-5">
    <p className="text-sm text-slate-600">{t('m11RangeShown', { from: f.fullDay(d.filter.from!), to: f.fullDay(d.filter.to!) })}</p>
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Kpi label={t('m11Collected')} value={f.money(s.collected)} hint={t('m11InRange')} />
      <Kpi label={t('m11CollectionRate')} value={f.pct(s.collection_rate)} meter={s.collection_rate} hint={t('m11CollectionRule', { paid: f.money(s.paid_against_billed), billed: f.money(s.net_billed) })} />
      <Kpi label={t('m11Outstanding')} value={f.money(s.outstanding)} hint={t('m11AsOfToday')} />
      <Kpi label={t('m11Overdue')} value={f.money(s.overdue)} tone={s.overdue ? 'alert' : undefined} hint={t('m11OverdueStudents', { n: f.num(s.overdue_students), count: s.overdue_students })} />
      <Kpi label={t('m11Billed')} value={f.money(s.net_billed)} hint={t('m11Discounts', { amount: f.money(s.discounts) })} />
      <Kpi label={t('m11Income')} value={f.money(s.income)} hint={t('m11SchoolWide')} />
      <Kpi label={t('m11Expense')} value={f.money(s.expense)} hint={t('m11SchoolWide')} />
      <Kpi label={t('m11Net')} value={f.money(s.net)} tone={s.net < 0 ? 'alert' : undefined} />
    </div>
    <div className="grid gap-5 lg:grid-cols-2">
      <Card title={t('m11BilledVsCollected')}>
        <ColumnChart title={t('m11BilledVsCollected')} empty={t('m11NoFees')} format={v => f.money(v)} labels={d.months.map(m => f.month(m.month))}
          series={[{ key: 'billed', label: t('m11Billed'), values: d.months.map(m => m.billed) }, { key: 'collected', label: t('m11Collected'), values: d.months.map(m => m.collected) }]} />
      </Card>
      <Card title={t('m11IncomeVsExpense')}>
        <ColumnChart title={t('m11IncomeVsExpense')} empty={t('m11NoFees')} format={v => f.money(v)} labels={d.months.map(m => f.month(m.month))}
          series={[{ key: 'income', label: t('m11Income'), values: d.months.map(m => m.income) }, { key: 'expense', label: t('m11Expense'), values: d.months.map(m => m.expense) }]} />
      </Card>
      <Card title={t('m11ByMethod')}>
        <BarList title={t('m11ByMethod')} empty={t('m11NoFees')} items={d.methods.map(m => ({ key: m.method, label: t(`m11Method_${m.method}`), value: m.amount, display: f.money(m.amount), hint: t('m11Receipts', { n: f.num(m.receipts), count: m.receipts }) }))} />
      </Card>
      <Card title={t('m11Aging')}>
        <BarList title={t('m11Aging')} empty={t('m11NoFees')} items={d.aging.map(a => ({ key: a.bucket, label: t(`m11Aging_${a.bucket}`), value: a.amount, display: f.money(a.amount), hint: t('m11StudentsN', { n: f.num(a.students), count: a.students }) }))} />
      </Card>
      <Card title={t('m11ClassCollection')}>
        <BarList title={t('m11ClassCollection')} max={100} empty={t('m11NoFees')} items={d.classes.map(c => ({ key: c.id, label: c.name, value: c.rate, display: f.pct(c.rate), hint: t('m11ClassFeeHint', { paid: f.money(c.paid), billed: f.money(c.net_billed), due: f.money(c.outstanding) }) }))} />
      </Card>
      <Card title={t('m11TopOverdue')}>
        {d.top_overdue.length === 0 ? <p className="text-sm text-slate-600">{t('m11NoOverdue')}</p> :
          <ul className="divide-y divide-slate-100">{d.top_overdue.map(o => <li key={o.student_id} className="flex items-center justify-between gap-3 py-2 text-sm">
            <span className="min-w-0"><span className="block truncate font-semibold text-slate-900">{local(o.name_en, o.name_bn)}</span><span className="text-xs text-slate-500">{o.admission_no} · {o.class_name} {o.section_name} · {t('m11Since', { date: f.fullDay(o.oldest_due) })}</span></span>
            <span className="shrink-0 font-semibold tabular-nums text-rose-700">{f.money(o.overdue)}</span></li>)}</ul>}
      </Card>
    </div>
  </div>
}

const reasonTone: Record<string, Tone> = { consecutive_absences: 'absent', no_recent_presence: 'absent', low_attendance: 'late', failed_exam: 'absent', low_gpa: 'late' }

function RiskRow({ r }: { r: Risk }) {
  const { t } = useTranslation()
  const f = useFmt()
  const local = useLocal()
  return <li className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between">
    <div className="min-w-0">
      <p className="flex flex-wrap items-center gap-2"><Link to={`/analytics/students/${r.student_id}`} className="font-semibold text-slate-900 hover:underline">{local(r.name_en, r.name_bn)}</Link>
        <StatusBadge tone={r.level === 'dropout' ? 'absent' : 'late'}>{t(`m11Level_${r.level}`)}</StatusBadge></p>
      <p className="text-xs text-slate-500">{r.admission_no} · {r.class_name} {r.section_name}{r.roll_no ? ` · ${t('m11Roll', { n: f.num(r.roll_no) })}` : ''}</p>
      <ul className="mt-1.5 flex flex-wrap gap-1.5">{r.reasons.map(reason => <li key={reason}><StatusBadge tone={reasonTone[reason] ?? 'unmarked'}>{t(`m11Reason_${reason}`)}</StatusBadge></li>)}</ul>
    </div>
    <dl className="grid shrink-0 grid-cols-3 gap-3 text-xs sm:text-right">
      <div><dt className="text-slate-500">{t('m11AttendanceShort')}</dt><dd className="font-semibold tabular-nums text-slate-900">{f.pct(r.attendance_rate)}</dd></div>
      <div><dt className="text-slate-500">{t('m11AbsentRun')}</dt><dd className="font-semibold tabular-nums text-slate-900">{f.num(r.absent_streak)}</dd></div>
      <div><dt className="text-slate-500">{t('m11LatestGpa')}</dt><dd className="font-semibold tabular-nums text-slate-900">{r.latest_passed === false ? t('m11FailedShort') : f.gpa(r.latest_gpa)}</dd></div>
    </dl>
  </li>
}

function AlertsTab({ filter, settings, canManage, modules }: { filter: Filter; settings: AlertSettings; canManage: boolean; modules: Record<string, boolean> }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const [level, setLevel] = useState('')
  const [reason, setReason] = useState('')
  const [page, setPage] = useState(1)
  const scoped: Filter = { session_id: filter.session_id, class_id: filter.class_id, section_id: filter.section_id }
  const q = useQuery({ queryKey: ['analytics', 'risks', scoped, level, reason, page], queryFn: () => api(token => getRisks(token, scoped, level, reason, page)), retry: false })
  const reasons = ['consecutive_absences', 'no_recent_presence', 'low_attendance', 'failed_exam', 'low_gpa']
  return <div className="space-y-5">
    {q.isPending ? <Skeleton rows={5} /> : q.isError ? <Failure error={q.error} retry={() => void q.refetch()} /> : <>
      <div className="grid grid-cols-3 gap-3">
        <Kpi label={t('m11DropoutAlerts')} value={f.num(q.data.summary.dropout)} tone={q.data.summary.dropout ? 'alert' : undefined} />
        <Kpi label={t('m11AtRiskAlerts')} value={f.num(q.data.summary.at_risk)} />
        <Kpi label={t('m11StudentsChecked')} value={f.num(q.data.summary.students)} />
      </div>
      <Card title={t('m11AlertList')} actions={<div className="flex flex-wrap gap-2 print:hidden">
        <select aria-label={t('m11Level')} className={`${ui.field} mt-0 w-auto`} value={level} onChange={e => { setLevel(e.target.value); setPage(1) }}>
          <option value="">{t('m11AllLevels')}</option><option value="dropout">{t('m11Level_dropout')}</option><option value="at_risk">{t('m11Level_at_risk')}</option></select>
        <select aria-label={t('m11Reason')} className={`${ui.field} mt-0 w-auto`} value={reason} onChange={e => { setReason(e.target.value); setPage(1) }}>
          <option value="">{t('m11AllReasons')}</option>{reasons.map(r => <option key={r} value={r}>{t(`m11Reason_${r}`)} ({f.num(q.data.summary.reasons[r] ?? 0)})</option>)}</select></div>}>
        {q.data.items.length === 0 ? <EmptyState title={t('m11NoAlerts')}>{t('m11NoAlertsHint')}</EmptyState> :
          <ul className="divide-y divide-slate-100">{q.data.items.map(r => <RiskRow key={r.student_id} r={r} />)}</ul>}
        <Pager page={page} total={q.data.total} size={q.data.page_size} onPage={setPage} previous={t('m11Previous')} next={t('m11Next')} label={t('m11Pages')} />
      </Card>
    </>}
    <RulesCard settings={settings} canManage={canManage} modules={modules} />
  </div>
}

function RulesCard({ settings, canManage, modules }: { settings: AlertSettings; canManage: boolean; modules: Record<string, boolean> }) {
  const { t } = useTranslation()
  const f = useFmt()
  const [editing, setEditing] = useState(false)
  const s = settings
  return <Card title={t('m11Rules')} actions={canManage && !editing ? <button type="button" className={ui.secondary} onClick={() => setEditing(true)}>{t('m11EditThresholds')}</button> : undefined}>
    <p className="text-sm text-slate-600">{t('m11RulesIntro')}</p>
    <div className="mt-3 grid gap-3 text-sm md:grid-cols-2">
      <div><p className="font-semibold text-slate-900">{t('m11Level_dropout')}</p><ul className="mt-1 list-disc space-y-1 pl-5 text-slate-700">
        <li>{t('m11RuleRun', { n: f.num(s.consecutive_absences) })}</li><li>{t('m11RuleNoPresence', { days: f.num(s.inactivity_days), min: f.num(s.min_marked_days) })}</li></ul></div>
      <div><p className="font-semibold text-slate-900">{t('m11Level_at_risk')}</p><ul className="mt-1 list-disc space-y-1 pl-5 text-slate-700">
        <li>{t('m11RuleLowAttendance', { rate: f.num(s.attendance_threshold), days: f.num(s.attendance_window_days), min: f.num(s.min_marked_days) })}</li>
        <li>{t('m11RuleFailed')}</li><li>{t('m11RuleLowGpa', { gpa: f.gpa(s.gpa_threshold) })}</li></ul></div>
    </div>
    {(!modules.attendance || !modules.examinations) && <p className="mt-3 text-xs text-slate-500">{t('m11RulesModules')}</p>}
    {editing && <SettingsForm settings={settings} onDone={() => setEditing(false)} />}
  </Card>
}

function SettingsForm({ settings, onDone }: { settings: AlertSettings; onDone: () => void }) {
  const { t } = useTranslation()
  const api = useApi()
  const client = useQueryClient()
  const text = useAnalyticsError()
  const [form, setForm] = useState({ ...settings })
  const save = useMutation({
    mutationFn: () => api(token => saveAlertSettings(token, { attendance_threshold: Number(form.attendance_threshold), attendance_window_days: Number(form.attendance_window_days), min_marked_days: Number(form.min_marked_days), gpa_threshold: Number(form.gpa_threshold), consecutive_absences: Number(form.consecutive_absences), inactivity_days: Number(form.inactivity_days) })),
    onSuccess: async () => { await client.invalidateQueries({ queryKey: ['analytics'] }); onDone() },
  })
  const fields: [keyof AlertSettings, number, number, number][] = [['attendance_threshold', 1, 100, 1], ['attendance_window_days', 7, 120, 1], ['min_marked_days', 1, 60, 1], ['gpa_threshold', 0, 5, 0.25], ['consecutive_absences', 2, 60, 1], ['inactivity_days', 7, 120, 1]]
  function submit(e: FormEvent) { e.preventDefault(); save.mutate() }
  return <form onSubmit={submit} className="mt-4 space-y-3 border-t border-slate-100 pt-4">
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {fields.map(([key, min, max, step]) => <label key={key}><span className={ui.label}>{t(`m11Setting_${key}`)}</span>
        <input type="number" required className={ui.field} min={min} max={max} step={step} value={form[key] as number} onChange={e => setForm({ ...form, [key]: e.target.value })} /></label>)}
    </div>
    {save.isError && <Notice tone="error">{text(save.error)}</Notice>}
    <div className="flex gap-2"><button type="submit" className={ui.primary} disabled={save.isPending}>{t(save.isPending ? 'm11Saving' : 'm11Save')}</button><button type="button" className={ui.secondary} onClick={onDone}>{t('m11Cancel')}</button></div>
  </form>
}

function SessionsTab() {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'sessions'], queryFn: () => api(getSessions), retry: false })
  if (q.isPending) return <Skeleton rows={3} />
  if (q.isError) return <Failure error={q.error} retry={() => void q.refetch()} />
  const rows = q.data.items
  const finance = rows.some(r => r.net_billed !== undefined)
  if (!rows.length) return <EmptyState title={t('m11NoSession')} />
  return <div className="space-y-5">
    <div className="grid gap-5 lg:grid-cols-2">
      <Card title={t('m11AttendanceBySession')}><BarList title={t('m11AttendanceBySession')} max={100} empty={t('m11NoData')} items={rows.map(r => ({ key: r.id, label: r.name, value: r.attendance_rate, display: f.pct(r.attendance_rate) }))} /></Card>
      <Card title={t('m11PassRateBySession')}><BarList title={t('m11PassRateBySession')} max={100} empty={t('m11NoData')} items={rows.map(r => ({ key: r.id, label: r.name, value: r.pass_rate, display: f.pct(r.pass_rate), hint: t('m11AvgGpa', { gpa: f.gpa(r.avg_gpa) }) }))} /></Card>
    </div>
    <Card title={t('m11SessionComparison')}>
      <div className="overflow-x-auto"><table className="w-full min-w-[36rem] text-left text-sm">
        <thead className="text-xs uppercase tracking-wide text-slate-500"><tr><th className="py-2 pr-3">{t('m11Session')}</th><th className="py-2 pr-3">{t('m11ActiveStudents')}</th><th className="py-2 pr-3">{t('m11AttendanceRate')}</th><th className="py-2 pr-3">{t('m11PassRate')}</th><th className="py-2 pr-3">{t('m11AverageGpa')}</th>{finance && <><th className="py-2 pr-3">{t('m11Billed')}</th><th className="py-2 pr-3">{t('m11CollectionRate')}</th></>}</tr></thead>
        <tbody className="divide-y divide-slate-100 tabular-nums">{rows.map(r => <tr key={r.id}><th scope="row" className="py-2 pr-3 font-semibold text-slate-900">{r.name}{r.is_active && <span className="ml-1 text-xs font-normal text-slate-500">· {t('m11Active')}</span>}</th>
          <td className="py-2 pr-3">{f.num(r.active_students)}</td><td className="py-2 pr-3">{f.pct(r.attendance_rate)}</td><td className="py-2 pr-3">{f.pct(r.pass_rate)}</td><td className="py-2 pr-3">{f.gpa(r.avg_gpa)}</td>
          {finance && <><td className="py-2 pr-3">{f.money(r.net_billed ?? 0)}</td><td className="py-2 pr-3">{f.pct(r.collection_rate)}</td></>}</tr>)}</tbody>
      </table></div>
    </Card>
  </div>
}

// ---------- reports ----------

export function ReportsPage({ access }: { access: AnalyticsAccess }) {
  const { t } = useTranslation()
  const api = useApi()
  const text = useAnalyticsError()
  const defs = useQuery({ queryKey: ['analytics', 'reports'], queryFn: () => api(getReports), retry: false })
  const allowed = (defs.data?.items ?? []).filter(r => r.access === 'finance' ? access.finance : r.access === 'school' ? access.school : access.school || access.teacher)
  const [chosen, setChosen] = useState<ReportKey | ''>('')
  const [filter, setFilter] = useState<Filter>({})
  const [page, setPage] = useState(1)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [downloading, setDownloading] = useState(false)
  const key = (chosen || allowed[0]?.key || '') as ReportKey | ''
  const def = allowed.find(r => r.key === key)
  const options = useOptions(filter.session_id ?? '')
  const active: Filter = { ...filter, session_id: filter.session_id || options.data?.session?.id || '' }
  const modulesOn = !def?.module || options.data?.modules[def.module] !== false
  const report = useQuery({
    queryKey: ['analytics', 'report', key, active, page], enabled: !!def && !!active.session_id && modulesOn, retry: false,
    queryFn: () => api(token => getReport(token, key as ReportKey, active, page)),
  })
  async function download() {
    if (!def) return
    setDownloading(true); setMessage(null)
    try {
      const blob = await api(token => downloadReport(token, def.key, active))
      const name = `flybirds-${def.key}${active.from ? `-${active.from}-to-${active.to}` : ''}.csv`
      saveBlob(blob, name); setMessage({ tone: 'success', text: t('m11Downloaded', { name }) })
    } catch (cause) { setMessage({ tone: 'error', text: text(cause) }) } finally { setDownloading(false) }
  }
  return <section className="space-y-5 analytics-print">
    <PageHeader eyebrow={t('m11Eyebrow')} title={t('m11ReportsTitle')} description={t('m11ReportsIntro')}
      actions={def && <><button type="button" className={ui.secondary} onClick={() => window.print()} disabled={!report.data}>{t('m11Print')}</button>
        <button type="button" className={ui.primary} onClick={() => void download()} disabled={downloading || !report.data}>{t(downloading ? 'm11Preparing' : 'm11DownloadCsv')}</button></>} />
    {defs.isPending || options.isPending ? <Skeleton rows={3} /> : defs.isError ? <Failure error={defs.error} /> : options.isError ? <Failure error={options.error} /> : allowed.length === 0 ? <Notice tone="info">{t('m11NoReports')}</Notice> : <>
      <div role="radiogroup" aria-label={t('m11ChooseReport')} className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5 print:hidden">
        {allowed.map(r => <button key={r.key} type="button" role="radio" aria-checked={r.key === key} onClick={() => { setChosen(r.key); setPage(1); setMessage(null) }}
          className={`rounded-xl border p-3 text-left transition focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${r.key === key ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200 bg-white hover:bg-slate-50'}`}>
          <span className="block text-sm font-semibold text-slate-900">{t(`m11Report_${r.key}`)}</span><span className="mt-0.5 block text-xs text-slate-600">{t(`m11Report_${r.key}_desc`)}</span></button>)}
      </div>
      {options.data && def && <FilterBar options={options.data} filter={active} onChange={next => { setFilter(next); setPage(1) }}
        keys={{ dates: def.ranged, section: def.key !== 'fee_balances' && def.key !== 'teacher_attendance', exam: def.key === 'exam_results' }} />}
      {message && <Notice tone={message.tone}>{message.text}</Notice>}
      {!modulesOn ? <Notice tone="info">{t('m11ModuleOff')}</Notice> : !active.session_id ? <EmptyState title={t('m11NoSession')} /> : report.isPending ? <Skeleton rows={6} /> : report.isError ? <Failure error={report.error} retry={() => void report.refetch()} /> :
        <section className={`${ui.card} p-4`}>
          <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2"><h2 className="text-base font-semibold text-slate-900">{t(`m11Report_${key}`)}</h2>
            <p className="text-sm text-slate-600">{t('m11Rows', { n: report.data.total, count: report.data.total })}{report.data.filter.from && ` · ${report.data.filter.from} – ${report.data.filter.to}`}</p></div>
          {report.data.rows.length === 0 ? <EmptyState title={t('m11NoData')}>{t('m11NoReportRows')}</EmptyState> : <ReportTable columns={report.data.columns} rows={report.data.rows} />}
          <Pager page={page} total={report.data.total} size={report.data.page_size} onPage={setPage} previous={t('m11Previous')} next={t('m11Next')} label={t('m11Pages')} />
        </section>}
    </>}
  </section>
}

const numericColumns = new Set(['marked', 'present', 'late', 'absent', 'leave', 'rate', 'roll_no', 'total_marks', 'full_marks', 'percent', 'gpa', 'section_position', 'class_position', 'net_billed', 'paid', 'balance', 'overdue', 'attendance_rate', 'absent_streak', 'latest_gpa'])

function ReportTable({ columns, rows }: { columns: string[]; rows: string[][] }) {
  const { t, i18n } = useTranslation()
  const bn = i18n.language === 'bn'
  const show = columns.map((c, i) => ({ c, i })).filter(({ c }) => c !== (bn ? 'name_en' : 'name_bn'))
  const cell = (column: string, value: string) => {
    if (!value) return '—'
    if (column === 'reasons') return value.split(' ').map(r => t(`m11Reason_${r}`)).join(', ')
    if (column === 'level') return t(`m11Level_${value}`)
    if (column === 'passed') return t(value === 'yes' ? 'm11Yes' : 'm11No')
    if (column === 'status') return t(`m11Status_${value}`, { defaultValue: value })
    if (column === 'exam_type') return t(`m11ExamType_${value}`, { defaultValue: value })
    if (numericColumns.has(column) && /^-?\d+(\.\d+)?$/.test(value)) return new Intl.NumberFormat(bn ? 'bn-BD' : 'en-GB', { maximumFractionDigits: 2 }).format(Number(value))
    return value
  }
  return <div className="overflow-x-auto"><table className="w-full text-left text-sm">
    <thead className="text-xs uppercase tracking-wide text-slate-500"><tr>{show.map(({ c }) => <th key={c} scope="col" className={`whitespace-nowrap py-2 pr-3 ${numericColumns.has(c) ? 'text-right' : ''}`}>{t(`m11Col_${c}`)}</th>)}</tr></thead>
    <tbody className="divide-y divide-slate-100">{rows.map((row, r) => <tr key={r}>{show.map(({ c, i }) => <td key={c} className={`py-2 pr-3 ${numericColumns.has(c) ? 'text-right tabular-nums' : ''} ${c.startsWith('name') ? 'font-medium text-slate-900' : 'text-slate-700'}`}>{cell(c, row[i])}</td>)}</tr>)}</tbody>
  </table></div>
}

// ---------- student progress (student, guardian, staff) ----------

const statusTone: Record<string, Tone> = { present: 'present', late: 'late', absent: 'absent' }

export function StudentSummaryView({ data }: { data: StudentSummary }) {
  const { t } = useTranslation()
  const f = useFmt()
  const local = useLocal()
  const s = data.student, a = data.attendance
  const latest = data.results[data.results.length - 1]
  return <div className="space-y-5">
    <div className={`${ui.card} flex flex-wrap items-center justify-between gap-3 p-4`}>
      <div className="min-w-0"><h2 className="truncate text-lg font-bold text-slate-900">{local(s.name_en, s.name_bn)}</h2>
        <p className="text-sm text-slate-600">{s.class_name} · {s.section_name}{s.roll_no ? ` · ${t('m11Roll', { n: f.num(s.roll_no) })}` : ''} · {s.admission_no}</p></div>
      <p className="text-sm text-slate-600">{t('m11SessionName', { name: s.session_name })}</p>
    </div>
    {data.alert && <Notice tone="warning"><p className="font-semibold">{t(`m11AlertTitle_${data.alert.level}`)}</p>
      <ul className="mt-1 list-disc pl-5">{data.alert.reasons.map(r => <li key={r}>{t(`m11ReasonExplain_${r}`, { n: f.num(data.settings.consecutive_absences), days: f.num(data.settings.inactivity_days), rate: f.num(data.settings.attendance_threshold), window: f.num(data.settings.attendance_window_days), gpa: f.gpa(data.settings.gpa_threshold) })}</li>)}</ul>
      <p className="mt-1">{t('m11AlertAdvice')}</p></Notice>}
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Kpi label={t('m11SessionAttendance')} value={f.pct(a?.session.rate)} meter={a?.session.rate} hint={a ? t('m11DaysMarked', { n: f.num(a.session.marked), count: a.session.marked }) : t('m11ModuleOffShort')} />
      <Kpi label={t('m11Last30')} value={f.pct(a?.last_30_days.rate)} meter={a?.last_30_days.rate} hint={a ? t('m11AbsentDays', { n: f.num(a.last_30_days.absent), count: a.last_30_days.absent }) : undefined} />
      <Kpi label={t('m11LatestResult')} value={latest ? `${latest.grade} · ${f.gpa(latest.gpa)}` : '—'} hint={latest ? latest.name : t('m11NoResultsYet')} tone={latest && !latest.passed ? 'alert' : undefined} />
      {data.fees ? <Kpi label={t('m11FeesDue')} value={f.money(data.fees.outstanding)} tone={data.fees.overdue > 0 ? 'alert' : undefined} hint={data.fees.overdue > 0 ? t('m11OverdueAmount', { amount: f.money(data.fees.overdue) }) : data.fees.next_due ? t('m11NextDue', { date: f.fullDay(data.fees.next_due) }) : t('m11NothingDue')} />
        : latest ? <Kpi label={t('m11Position')} value={f.num(latest.section_position)} hint={t('m11ClassPosition', { n: f.num(latest.class_position) })} /> : null}
    </div>
    <div className="grid gap-5 lg:grid-cols-2">
      {a && <Card title={t('m11MonthlyAttendance')}>
        <LineChart title={t('m11MonthlyAttendance')} empty={t('m11NoAttendance')} format={v => f.pct(v)} height={180} points={a.months.map(m => ({ label: f.month(m.period), value: m.rate }))} />
        {a.recent.length > 0 && <><h3 className="mt-4 text-sm font-semibold text-slate-800">{t('m11RecentDays')}</h3>
          <ul className="mt-2 flex flex-wrap gap-1.5">{[...a.recent].reverse().map(d => <li key={d.date}><StatusBadge tone={statusTone[d.status] ?? 'unmarked'}>{f.day(d.date)} · {t(`m11Status_${d.status}`)}</StatusBadge></li>)}</ul></>}
      </Card>}
      <Card title={t('m11ResultTrend')}>
        {data.results.length === 0 ? <p className="text-sm text-slate-600">{t('m11NoResultsYet')}</p> : <>
          <LineChart title={t('m11ResultTrend')} empty={t('m11NoResultsYet')} format={v => f.pct(v)} height={160} points={data.results.map(r => ({ label: r.name, value: r.percent, detail: `${r.grade} · GPA ${f.gpa(r.gpa)}` }))} />
          <ul className="mt-3 divide-y divide-slate-100 text-sm">{[...data.results].reverse().map(r => <li key={r.exam_id} className="flex items-center justify-between gap-3 py-2">
            <span className="min-w-0"><span className="block truncate font-medium text-slate-900">{r.name}</span><span className="text-xs text-slate-500">{r.published_on ? f.fullDay(r.published_on) : ''} · {t('m11Positions', { section: f.num(r.section_position), cls: f.num(r.class_position) })}</span></span>
            <span className={`shrink-0 font-semibold tabular-nums ${r.passed ? 'text-slate-900' : 'text-rose-700'}`}>{r.grade} · {f.gpa(r.gpa)}</span></li>)}</ul></>}
      </Card>
      {data.latest_subjects.length > 0 && <Card title={t('m11LatestSubjects', { exam: latest?.name ?? '' })} className="lg:col-span-2">
        <BarList title={t('m11LatestSubjects', { exam: latest?.name ?? '' })} max={100} empty={t('m11NoResultsYet')} items={data.latest_subjects.map((s, i) => ({ key: String(i), label: local(s.name_en, s.name_bn), value: s.percent, display: `${f.num(s.marks)}/${f.num(s.full_marks)}${s.grade ? ` · ${s.grade}` : ''}`, hint: s.passed === false ? t('m11SubjectFailed') : undefined }))} />
      </Card>}
    </div>
  </div>
}

export function MyProgressPage() {
  const { t } = useTranslation()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'me'], queryFn: () => api(getMySummary), retry: false })
  return <section className="space-y-5">
    <PageHeader eyebrow={t('m11Eyebrow')} title={t('m11MyProgress')} description={t('m11MyProgressIntro')} />
    {q.isPending ? <Skeleton rows={4} /> : q.isError ? <Failure error={q.error} retry={() => void q.refetch()} /> : <StudentSummaryView data={q.data} />}
  </section>
}

export function ChildrenProgressPage() {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const q = useQuery({ queryKey: ['analytics', 'children'], queryFn: () => api(getChildren), retry: false })
  const [selected, setSelected] = useState('')
  const items = q.data?.items ?? []
  const current = items.find(c => c.student.id === selected) ?? items[0]
  return <section className="space-y-5">
    <PageHeader eyebrow={t('m11Eyebrow')} title={t('m11ChildrenProgress')} description={t('m11ChildrenProgressIntro')} />
    {q.isPending ? <Skeleton rows={4} /> : q.isError ? <Failure error={q.error} retry={() => void q.refetch()} /> : items.length === 0 ? <EmptyState title={t('m11NoChildren')}>{t('m11NoChildrenHint')}</EmptyState> : <>
      {items.length > 1 && <div role="radiogroup" aria-label={t('m11ChooseChild')} className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {items.map(c => <button key={c.student.id} type="button" role="radio" aria-checked={c.student.id === current?.student.id} onClick={() => setSelected(c.student.id)}
          className={`rounded-xl border p-3 text-left ${c.student.id === current?.student.id ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200 bg-white'}`}>
          <span className="block font-semibold text-slate-900">{local(c.student.name_en, c.student.name_bn)}</span>
          <span className="text-xs text-slate-600">{c.student.class_name} · {t('m11Last30Short', { rate: f.pct(c.attendance?.last_30_days.rate) })}</span>
          {c.alert && <span className="mt-1 block"><StatusBadge tone="late">{t('m11NeedsAttention')}</StatusBadge></span>}</button>)}
      </div>}
      {current && <StudentSummaryView data={current} />}
    </>}
  </section>
}

export function StaffStudentPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'student', id], queryFn: () => api(token => getStudentSummary(token, id)), retry: false })
  return <section className="space-y-5">
    <PageHeader eyebrow={t('m11Eyebrow')} title={t('m11StudentProgress')} actions={<button type="button" className={ui.secondary} onClick={() => history.back()}>{t('m11Back')}</button>} />
    {q.isPending ? <Skeleton rows={4} /> : q.isError ? <Failure error={q.error} retry={() => void q.refetch()} /> : <StudentSummaryView data={q.data} />}
  </section>
}

// ---------- dashboard widgets ----------

export function DashboardAnalytics({ navigation }: { navigation: string[] }) {
  const api = useApi()
  const perms = useQuery({ queryKey: ['analytics', 'my-permissions'], queryFn: () => api(getMyPermissions), retry: false, staleTime: 60_000, enabled: navigation.some(k => k.startsWith('analytics')) })
  const has = (p: string) => !!perms.data?.includes(p)
  if (!perms.data) return null
  if (navigation.includes('analytics') && has('analytics:read')) return <SchoolWidget />
  if (navigation.includes('analytics') && has('analytics:finance')) return <FinanceWidget />
  if (navigation.includes('analytics_teacher')) return <TeacherWidget />
  if (navigation.includes('analytics_student')) return <StudentWidget />
  if (navigation.includes('analytics_guardian')) return <GuardianWidget />
  return null
}

function Widget({ title, to, children, loading, error }: { title: string; to: string; children?: ReactNode; loading: boolean; error: unknown }) {
  const { t } = useTranslation()
  return <section className={`${ui.card} p-5`} aria-label={title}>
    <div className="mb-3 flex items-center justify-between gap-2"><h2 className="text-lg font-semibold text-slate-900">{title}</h2><Link to={to} className={ui.ghost}>{t('m11Open')}</Link></div>
    {loading ? <Skeleton rows={1} /> : error ? <Failure error={error} /> : children}
  </section>
}

function MiniStats({ items }: { items: { label: string; value: string; alert?: boolean }[] }) {
  return <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">{items.map(i => <div key={i.label}><dt className="text-xs text-slate-500">{i.label}</dt><dd className={`text-xl font-bold tabular-nums ${i.alert ? 'text-rose-700' : 'text-slate-900'}`}>{i.value}</dd></div>)}</dl>
}

function SchoolWidget() {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'overview', ''], queryFn: () => api(token => getOverview(token)), retry: false })
  const d = q.data
  return <Widget title={t('m11SchoolAtAGlance')} to="/analytics" loading={q.isPending} error={q.error}>
    {d && <MiniStats items={[
      { label: t('m11TodayAttendance'), value: f.pct(d.today_students?.rate) }, { label: t('m11Last30'), value: f.pct(d.last_30_days?.rate) },
      { label: t('m11PassRate'), value: f.pct(d.results?.pass_rate) }, { label: t('m11DropoutAlerts'), value: f.num(d.risks?.dropout ?? 0), alert: !!d.risks?.dropout },
      ...(d.fees ? [{ label: t('m11Outstanding'), value: f.money(d.fees.outstanding) }, { label: t('m11Overdue'), value: f.money(d.fees.overdue), alert: d.fees.overdue > 0 }] : []),
    ]} />}
  </Widget>
}

function FinanceWidget() {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'fees', 'widget'], queryFn: () => api(token => getFees(token, {})), retry: false })
  const s = q.data?.summary
  return <Widget title={t('m11FinanceAtAGlance')} to="/analytics" loading={q.isPending} error={q.error}>
    {s && <MiniStats items={[{ label: t('m11Collected'), value: f.money(s.collected) }, { label: t('m11CollectionRate'), value: f.pct(s.collection_rate) },
      { label: t('m11Outstanding'), value: f.money(s.outstanding) }, { label: t('m11Overdue'), value: f.money(s.overdue), alert: s.overdue > 0 }]} />}
  </Widget>
}

function TeacherWidget() {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'risks', 'widget'], queryFn: () => api(token => getRisks(token, {}, '', '', 1, 1)), retry: false })
  const att = useQuery({ queryKey: ['analytics', 'attendance', 'widget'], queryFn: () => api(token => getAttendance(token, {})), retry: false })
  return <Widget title={t('m11MyClassesAtAGlance')} to="/teacher/analytics" loading={q.isPending} error={q.error}>
    {q.data && <MiniStats items={[{ label: t('m11Last30'), value: f.pct(att.data?.summary.rate) }, { label: t('m11StudentsChecked'), value: f.num(q.data.summary.students) },
      { label: t('m11DropoutAlerts'), value: f.num(q.data.summary.dropout), alert: q.data.summary.dropout > 0 }, { label: t('m11AtRiskAlerts'), value: f.num(q.data.summary.at_risk) }]} />}
  </Widget>
}

function StudentWidget() {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'me'], queryFn: () => api(getMySummary), retry: false })
  const d = q.data, latest = d?.results[d.results.length - 1]
  return <Widget title={t('m11MyProgress')} to="/student/progress" loading={q.isPending} error={q.error}>
    {d && <MiniStats items={[{ label: t('m11Last30'), value: f.pct(d.attendance?.last_30_days.rate) }, { label: t('m11SessionAttendance'), value: f.pct(d.attendance?.session.rate) },
      { label: t('m11LatestResult'), value: latest ? `${latest.grade} · ${f.gpa(latest.gpa)}` : '—', alert: latest ? !latest.passed : false },
      ...(d.fees ? [{ label: t('m11FeesDue'), value: f.money(d.fees.outstanding), alert: d.fees.overdue > 0 }] : [])]} />}
  </Widget>
}

function GuardianWidget() {
  const { t } = useTranslation()
  const f = useFmt()
  const local = useLocal()
  const api = useApi()
  const q = useQuery({ queryKey: ['analytics', 'children'], queryFn: () => api(getChildren), retry: false })
  return <Widget title={t('m11ChildrenProgress')} to="/guardian/progress" loading={q.isPending} error={q.error}>
    {q.data && (q.data.items.length === 0 ? <p className="text-sm text-slate-600">{t('m11NoChildren')}</p> :
      <ul className="divide-y divide-slate-100">{q.data.items.map(c => {
        const latest = c.results[c.results.length - 1]
        return <li key={c.student.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
          <span className="font-semibold text-slate-900">{local(c.student.name_en, c.student.name_bn)}{c.alert && <span className="ml-2 align-middle"><StatusBadge tone="late">{t('m11NeedsAttention')}</StatusBadge></span>}</span>
          <span className="tabular-nums text-slate-600">{t('m11Last30Short', { rate: f.pct(c.attendance?.last_30_days.rate) })} · {latest ? `${latest.grade} · ${f.gpa(latest.gpa)}` : '—'}</span></li>
      })}</ul>)}
  </Widget>
}
