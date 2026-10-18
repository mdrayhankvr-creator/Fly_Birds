import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError, getClassRoutine, getMyPermissions, getOwnAssignments, type AuthUser, type ClassRoutineSlot, type SchoolBranding } from '../api/client'
import { getAdminOverview } from '../api/admin'
import { getAttendance, getChildren, getFees, getMySummary, getOverview, getRisks, type StudentSummary } from '../api/analytics'
import { getDiaryFeed, getEvents, getNoticeFeed, type Notice, type SchoolEvent } from '../api/communication'
import { getFeePayments } from '../api/fees'
import { Icon, type IconName } from '../components/icons'
import { useNavGroups } from '../components/navContext'
import { LineChart } from './analyticsCharts'
import { useFmt } from './AnalyticsPages'
import { StatusBadge, ui } from './attendanceUi'
import { useApi, useLocal } from './commShared'

// Role home dashboards (M12). Every figure comes from an existing API. A card
// whose module is off, or that the viewer may not see, is not shown; a card
// whose request fails says so instead of showing a number.

const DHAKA = 'Asia/Dhaka'
function dhakaNow() {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: DHAKA, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', hour12: false, weekday: 'short' }).formatToParts(new Date())
  const get = (type: string) => parts.find(p => p.type === type)?.value ?? ''
  const weekday = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].indexOf(get('weekday'))
  return { day: `${get('year')}-${get('month')}-${get('day')}`, hour: Number(get('hour')) % 24, weekday }
}
function shift(day: string, days: number) {
  const d = new Date(`${day}T00:00:00Z`); d.setUTCDate(d.getUTCDate() + days); return d.toISOString().slice(0, 10)
}

export function Panel({ title, icon, action, children, className = '' }: { title: string; icon?: IconName; action?: ReactNode; children: ReactNode; className?: string }) {
  return <section className={`${ui.card} flex flex-col p-5 ${className}`} aria-label={title}>
    <header className="mb-4 flex items-center justify-between gap-3">
      <h2 className="flex items-center gap-2 text-[15px] font-semibold text-slate-900">{icon && <Icon name={icon} className="h-[18px] w-[18px] text-slate-500" />}{title}</h2>{action}
    </header>
    <div className="min-w-0 flex-1">{children}</div>
  </section>
}

function ViewAll({ to, label }: { to: string; label?: string }) {
  const { t } = useTranslation()
  return <Link to={to} className="inline-flex shrink-0 items-center gap-1 text-sm font-semibold text-[var(--brand-ink)] hover:underline">{label ?? t('m12ViewAll')}<Icon name="arrowRight" className="h-3.5 w-3.5" /></Link>
}

export function Metric({ label, value, hint, icon, to, tone }: { label: string; value: ReactNode; hint?: ReactNode; icon: IconName; to?: string; tone?: 'alert' | 'good' }) {
  const body = <>
    <div className="flex items-start justify-between gap-3"><p className="text-sm font-medium text-slate-600">{label}</p>
      <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${tone === 'alert' ? 'bg-rose-50 text-rose-700' : 'shell-mark'}`}><Icon name={icon} className="h-[18px] w-[18px]" /></span></div>
    <p className={`mt-2 text-2xl font-bold tabular-nums tracking-tight ${tone === 'alert' ? 'text-rose-700' : 'text-slate-900'}`}>{value}</p>
    {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
  </>
  return to ? <Link to={to} className={`${ui.card} block p-4 transition hover:-translate-y-px hover:shadow-md focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] motion-reduce:hover:translate-y-0`}>{body}</Link>
    : <div className={`${ui.card} p-4`}>{body}</div>
}

function Unavailable({ error }: { error: unknown }) {
  const { t } = useTranslation()
  const text = error instanceof ApiError && error.code === 'module_disabled' ? t('m12ModuleOff') : error instanceof ApiError && error.status === 403 ? t('m12NoAccess') : t('m12LoadFailed')
  return <p className="flex items-center gap-2 rounded-xl bg-slate-50 px-3 py-3 text-sm text-slate-600"><Icon name="alert" className="h-4 w-4 shrink-0 text-slate-500" />{text}</p>
}

function Lines({ rows = 3 }: { rows?: number }) {
  return <div className="space-y-2" aria-hidden="true">{Array.from({ length: rows }, (_, i) => <div key={i} className="h-10 animate-pulse rounded-lg bg-slate-100 motion-reduce:animate-none" />)}</div>
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="rounded-xl border border-dashed border-slate-200 px-4 py-6 text-center text-sm text-slate-500">{children}</p>
}

function Greeting({ user, branding, role, children }: { user: AuthUser; branding?: SchoolBranding; role: string; children?: ReactNode }) {
  const { t, i18n } = useTranslation()
  const now = dhakaNow()
  const part = now.hour < 12 ? 'm12GoodMorning' : now.hour < 17 ? 'm12GoodAfternoon' : 'm12GoodEvening'
  const date = new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: DHAKA }).format(new Date())
  const school = branding ? (i18n.language === 'bn' ? branding.name_bn : branding.name_en) : ''
  return <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
    <div className="min-w-0">
      <p className="text-sm text-slate-500">{date}{school && <> · {school}</>}</p>
      <h1 className="mt-1 text-2xl font-bold tracking-tight text-slate-900 sm:text-[28px]">{t(part, { name: (user.display_name || user.email).split(/[\s@]/)[0] })}</h1>
      <p className="mt-1 text-sm text-slate-600">{t(`m12RoleIntro_${role}`)}</p>
    </div>
    {children && <div className="flex flex-wrap gap-2">{children}</div>}
  </header>
}

function NoticesPanel() {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const q = useQuery({ queryKey: ['dash', 'notices'], queryFn: () => api(token => getNoticeFeed(token, new URLSearchParams({ page: '1', page_size: '4' }))), retry: false })
  return <Panel title={t('m12LatestNotices')} icon="megaphone" action={<ViewAll to="/notices" />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : q.data.items.length === 0 ? <Empty>{t('m12NoNotices')}</Empty> :
      <ul className="divide-y divide-slate-100">{q.data.items.map((n: Notice) => <li key={n.id} className="py-2.5 first:pt-0 last:pb-0">
        <Link to="/notices" className="group block">
          <p className="flex items-start gap-2"><span className="min-w-0 flex-1 text-sm font-medium text-slate-900 group-hover:text-[var(--brand-ink)]">{local(n.title_en, n.title_bn)}</span>
            {n.priority !== 'normal' && <StatusBadge tone={n.priority === 'urgent' ? 'absent' : 'late'}>{t(`m9Priority_${n.priority}`)}</StatusBadge>}</p>
          <p className="mt-0.5 text-xs text-slate-500">{n.publish_at ? f.fullDay(new Date(n.publish_at).toISOString().slice(0, 10)) : ''}</p>
        </Link></li>)}</ul>}
  </Panel>
}

function EventsPanel() {
  const { t, i18n } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const today = dhakaNow().day
  const q = useQuery({ queryKey: ['dash', 'events', today], queryFn: () => api(token => getEvents(token, new URLSearchParams({ from: today, to: shift(today, 45) }))), retry: false })
  return <Panel title={t('m12UpcomingEvents')} icon="calendar" action={<ViewAll to="/events" />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : q.data.length === 0 ? <Empty>{t('m12NoEvents')}</Empty> :
      <ul className="space-y-3">{q.data.filter((e: SchoolEvent) => e.status === 'published').slice(0, 4).map(e => {
        const d = new Date(`${e.starts_on}T00:00:00Z`)
        return <li key={e.id} className="flex items-center gap-3">
          <span className="flex h-11 w-11 shrink-0 flex-col items-center justify-center rounded-xl border border-slate-200 bg-white leading-none">
            <span className="text-[10px] font-semibold uppercase text-[var(--brand-ink)]">{new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { month: 'short', timeZone: 'UTC' }).format(d)}</span>
            <span className="mt-0.5 text-base font-bold tabular-nums text-slate-900">{f.num(d.getUTCDate())}</span></span>
          <span className="min-w-0"><span className="block truncate text-sm font-medium text-slate-900">{local(e.title_en, e.title_bn)}</span><span className="text-xs text-slate-500">{t(`m9Kind_${e.kind}`)}{e.ends_on !== e.starts_on ? ` · ${f.fullDay(e.starts_on)} – ${f.fullDay(e.ends_on)}` : ''}</span></span>
        </li>
      })}</ul>}
  </Panel>
}

function minutes(time: string) { const [h, m] = time.split(':').map(Number); return h * 60 + m }

function RoutineToday({ path, title }: { path: string; title: string }) {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const now = dhakaNow()
  const q = useQuery({ queryKey: ['dash', 'routine', path], queryFn: () => api(token => getClassRoutine(token, path, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const nowMin = (() => { const p = new Intl.DateTimeFormat('en-GB', { timeZone: DHAKA, hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date()).split(':'); return Number(p[0]) * 60 + Number(p[1]) })()
  const slots = (q.data?.items ?? []).filter((s: ClassRoutineSlot) => s.day_of_week === now.weekday).sort((a, b) => a.start_time.localeCompare(b.start_time))
  const routineLink = path.includes('children') ? '/guardian/children' : path.endsWith('/me') ? undefined : '/admin/routines'
  return <Panel title={title} icon="clock" action={routineLink ? <ViewAll to={routineLink} /> : undefined}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : slots.length === 0 ? <Empty>{now.weekday === 5 ? t('m12WeeklyHoliday') : t('m12NoClassesToday')}</Empty> :
      <ol className="space-y-2">{slots.map(s => {
        const live = nowMin >= minutes(s.start_time) && nowMin < minutes(s.end_time)
        const done = nowMin >= minutes(s.end_time)
        return <li key={s.id} className={`flex items-center gap-3 rounded-xl border px-3 py-2.5 ${live ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200'} ${done ? 'opacity-60' : ''}`}>
          <span className="w-[4.5rem] shrink-0 text-xs font-semibold tabular-nums text-slate-600">{s.start_time.slice(0, 5)}<br />{s.end_time.slice(0, 5)}</span>
          <span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold text-slate-900">{local(s.subject_name, s.subject_name_bn)}</span>
            <span className="block truncate text-xs text-slate-500">{s.class_name} · {s.section_name} · {s.room}</span></span>
          {live && <StatusBadge tone="present">{t('m12Now')}</StatusBadge>}
        </li>
      })}</ol>}
  </Panel>
}

function QuickLinks({ exclude = [] }: { exclude?: string[] }) {
  const { t } = useTranslation()
  const groups = useNavGroups()
  const items = groups.flatMap(g => g.items).filter(i => i.key !== 'dashboard' && !exclude.includes(i.key)).slice(0, 12)
  if (!items.length) return null
  return <Panel title={t('m12QuickAccess')} icon="layers">
    <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4">{items.map(i => <li key={i.path}>
      <Link to={i.path} className="flex h-full items-center gap-2.5 rounded-xl border border-slate-200 px-3 py-2.5 text-sm font-medium text-slate-700 transition hover:border-[var(--brand-primary)] hover:bg-[var(--brand-soft)] hover:text-[var(--brand-ink)] focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
        <Icon name={i.icon} className="h-[18px] w-[18px] shrink-0 text-slate-500" /><span className="min-w-0 leading-snug">{t(i.label)}</span></Link></li>)}</ul>
  </Panel>
}

function AttendanceTrend({ title }: { title: string }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'attendance-trend'], queryFn: () => api(token => getAttendance(token, {})), retry: false })
  return <Panel title={title} icon="trend" action={q.data ? <span className="text-sm font-semibold tabular-nums text-slate-900">{f.pct(q.data.summary.rate)}</span> : undefined}>
    {q.isPending ? <Lines rows={4} /> : q.isError ? <Unavailable error={q.error} /> :
      <LineChart title={title} empty={t('m11NoAttendance')} format={v => f.pct(v)} height={190} points={q.data.trend.map(p => ({ label: f.day(p.period), value: p.rate, detail: t('m11TrendDetail', { present: f.num(p.present), late: f.num(p.late), absent: f.num(p.absent) }) }))} />}
  </Panel>
}

// ---------- role dashboards ----------

type Ctx = { user: AuthUser; branding?: SchoolBranding; nav: Set<string>; permissions: string[] }

function SchoolOverview({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'overview'], queryFn: () => api(token => getOverview(token)), retry: false })
  if (q.isPending) return <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">{Array.from({ length: 4 }, (_, i) => <div key={i} className={`${ui.card} h-[112px] animate-pulse motion-reduce:animate-none`} />)}</div>
  if (q.isError) return <Unavailable error={q.error} />
  const d = q.data
  return <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <Metric label={t('m11ActiveStudents')} value={f.num(d.active_students)} hint={t('m11StaffLine', { teachers: f.num(d.teachers), staff: f.num(d.non_teaching_staff) })} icon="users" to={ctx.nav.has('students') ? '/admin/students' : undefined} />
    <Metric label={t('m11TodayAttendance')} value={f.pct(d.today_students?.rate)} hint={d.today_students ? t('m11MarkedOf', { marked: f.num(d.today_students.marked), total: f.num(d.active_students) }) : t('m11ModuleOffShort')} icon="check" to={ctx.nav.has('student_attendance') ? '/admin/attendance/students' : undefined} />
    <Metric label={t('m11PassRate')} value={f.pct(d.results?.pass_rate)} hint={d.results ? t('m11AvgGpa', { gpa: f.gpa(d.results.avg_gpa) }) : t('m11ModuleOffShort')} icon="grade" to="/analytics" />
    {d.fees ? <Metric label={t('m11Outstanding')} value={f.money(d.fees.outstanding)} hint={t('m11OverdueAmount', { amount: f.money(d.fees.overdue) })} icon="money" tone={d.fees.overdue > 0 ? 'alert' : undefined} to="/accounts/fees" />
      : <Metric label={t('m11DropoutAlerts')} value={f.num(d.risks?.dropout ?? 0)} hint={t('m11AtRiskCount', { n: f.num(d.risks?.at_risk ?? 0) })} icon="alert" tone={d.risks?.dropout ? 'alert' : undefined} to="/analytics" />}
  </div>
}

function Insights() {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'overview'], queryFn: () => api(token => getOverview(token)), retry: false })
  const d = q.data
  const items: { icon: IconName; text: string; to: string; tone: 'alert' | 'info' }[] = []
  if (d?.risks?.dropout) items.push({ icon: 'alert', text: t('m11InsightDropout', { n: f.num(d.risks.dropout), count: d.risks.dropout }), to: '/analytics', tone: 'alert' })
  if (d?.risks?.at_risk) items.push({ icon: 'trend', text: t('m11InsightAtRisk', { n: f.num(d.risks.at_risk), count: d.risks.at_risk }), to: '/analytics', tone: 'info' })
  if (d?.today_students && d.active_students > d.today_students.marked) items.push({ icon: 'clipboard', text: t('m11InsightUnmarked', { n: f.num(d.active_students - d.today_students.marked), count: d.active_students - d.today_students.marked }), to: '/admin/attendance/students', tone: 'info' })
  if (d?.fees && d.fees.overdue > 0) items.push({ icon: 'money', text: t('m11InsightOverdue', { amount: f.money(d.fees.overdue) }), to: '/accounts/fees', tone: 'alert' })
  return <Panel title={t('m11Insights')} icon="alert">
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : items.length === 0 ? <Empty>{t('m11NoInsights')}</Empty> :
      <ul className="space-y-2">{items.map((i, n) => <li key={n}><Link to={i.to} className="group flex items-center gap-3 rounded-xl border border-slate-200 px-3 py-2.5 hover:border-[var(--brand-primary)]">
        <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${i.tone === 'alert' ? 'bg-rose-50 text-rose-700' : 'shell-mark'}`}><Icon name={i.icon} className="h-4 w-4" /></span>
        <span className="min-w-0 flex-1 text-sm text-slate-800">{i.text}</span><Icon name="chevronRight" className="h-4 w-4 shrink-0 text-slate-400 group-hover:text-[var(--brand-ink)]" /></Link></li>)}</ul>}
  </Panel>
}

function SystemPanel() {
  const { t, i18n } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'system'], queryFn: () => api(getAdminOverview), retry: false })
  const d = q.data
  return <Panel title={t('m12SystemHealth')} icon="shield" action={<ViewAll to="/admin/control" />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : d && <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
      <div><dt className="text-slate-500">{t('m12Accounts')}</dt><dd className="font-semibold tabular-nums text-slate-900">{f.num(d.users.reduce((s, r) => s + r.active, 0))}</dd></div>
      <div><dt className="text-slate-500">{t('m10Modules')}</dt><dd className="font-semibold tabular-nums text-slate-900">{t('m10ModulesOn', { enabled: f.num(d.modules_enabled), total: f.num(d.modules_total) })}</dd></div>
      <div><dt className="text-slate-500">{t('m10LastBackup')}</dt><dd className="font-semibold text-slate-900">{d.last_backup ? new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { dateStyle: 'medium', timeZone: DHAKA }).format(new Date(d.last_backup.started_at)) : <span className="text-amber-700">{t('m10Never')}</span>}</dd></div>
      <div><dt className="text-slate-500">{t('m10DeniedAttempts')}</dt><dd className={`font-semibold tabular-nums ${d.denied_last_day ? 'text-rose-700' : 'text-slate-900'}`}>{f.num(d.denied_last_day)}</dd></div>
    </dl>}
  </Panel>
}

function UsersSummary() {
  // Shown when analytics is off, so administrators still see real figures.
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'system'], queryFn: () => api(getAdminOverview), retry: false })
  if (q.isPending || q.isError || !q.data) return null
  const d = q.data
  return <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
    <Metric label={t('m10ActiveStudents')} value={f.num(d.active_students)} icon="users" />
    <Metric label={t('m10ActiveStaff')} value={f.num(d.active_staff)} icon="teacher" />
    <Metric label={t('m10Guardians')} value={f.num(d.guardians)} icon="user" />
    <Metric label={t('m10FeesOutstanding')} value={f.money(d.fees_outstanding)} icon="money" />
  </div>
}

function AdminDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const analytics = ctx.nav.has('analytics') && ctx.permissions.includes('analytics:read')
  return <div className="space-y-6">
    <Greeting user={ctx.user} branding={ctx.branding} role={ctx.user.role}>
      {ctx.nav.has('student_attendance') && <Link to="/admin/attendance/students" className={ui.secondary}><Icon name="check" className="h-4 w-4" />{t('m12TakeAttendance')}</Link>}
      {analytics && <Link to="/analytics" className={ui.primary}><Icon name="chart" className="h-4 w-4" />{t('m12OpenAnalytics')}</Link>}
    </Greeting>
    {analytics ? <SchoolOverview ctx={ctx} /> : ctx.permissions.includes('system:read') ? <UsersSummary /> : null}
    <div className="grid gap-6 xl:grid-cols-3">
      {analytics && ctx.nav.has('student_attendance') && <AttendanceTrend title={t('m12AttendanceLast30')} />}
      {analytics && <Insights />}
      {ctx.user.role === 'admin' && ctx.permissions.includes('system:read') && <SystemPanel />}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </div>
    <QuickLinks />
  </div>
}

function TeacherDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const local = useLocal()
  const assignments = useQuery({ queryKey: ['dash', 'assignments'], enabled: ctx.permissions.includes('staff:read_self') && ctx.nav.has('staff_self'), queryFn: () => api(token => getOwnAssignments(token, new URLSearchParams({ page: '1', page_size: '50' }))), retry: false })
  const analytics = ctx.nav.has('analytics_teacher')
  const risks = useQuery({ queryKey: ['dash', 'teacher-risks'], enabled: analytics, queryFn: () => api(token => getRisks(token, {}, '', '', 1, 5)), retry: false })
  const att = useQuery({ queryKey: ['dash', 'attendance-trend'], enabled: analytics && ctx.nav.has('student_attendance'), queryFn: () => api(token => getAttendance(token, {})), retry: false })
  const classes = [...new Map((assignments.data?.items ?? []).map(a => [a.class_id + a.subject_id, a])).values()]
  return <div className="space-y-6">
    <Greeting user={ctx.user} branding={ctx.branding} role="teacher">
      {ctx.nav.has('student_attendance') && <Link to="/admin/attendance/students" className={ui.primary}><Icon name="check" className="h-4 w-4" />{t('m12TakeAttendance')}</Link>}
      {ctx.nav.has('diary_teacher') && <Link to="/teacher/diary" className={ui.secondary}><Icon name="notebook" className="h-4 w-4" />{t('m12WriteDiary')}</Link>}
    </Greeting>
    {analytics && <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Metric label={t('m12MyClasses')} value={assignments.data ? f.num(new Set(classes.map(c => c.class_id)).size) : '—'} hint={assignments.data ? t('m12SubjectsTaught', { n: f.num(classes.length), count: classes.length }) : undefined} icon="layers" />
      <Metric label={t('m11Last30')} value={f.pct(att.data?.summary.rate)} hint={t('m12ForMyClasses')} icon="check" to="/teacher/analytics" />
      <Metric label={t('m11DropoutAlerts')} value={risks.data ? f.num(risks.data.summary.dropout) : '—'} tone={risks.data?.summary.dropout ? 'alert' : undefined} icon="alert" to="/teacher/analytics" />
      <Metric label={t('m11AtRiskAlerts')} value={risks.data ? f.num(risks.data.summary.at_risk) : '—'} icon="trend" to="/teacher/analytics" />
    </div>}
    <div className="grid gap-6 xl:grid-cols-3">
      {ctx.nav.has('routine_teacher') && <RoutineToday path="/routines/classes/me" title={t('m12TodaysClasses')} />}
      {assignments.data && <Panel title={t('m12AssignedClasses')} icon="teacher">
        {classes.length === 0 ? <Empty>{t('m12NoAssignments')}</Empty> : <ul className="divide-y divide-slate-100">{classes.map(a => <li key={a.id} className="flex items-center justify-between gap-3 py-2.5 first:pt-0">
          <span className="min-w-0"><span className="block truncate text-sm font-semibold text-slate-900">{a.class_name}</span><span className="text-xs text-slate-500">{local(a.subject_name_en, a.subject_name_bn)}</span></span>
          {ctx.nav.has('exam_teacher') && <Link to="/teacher/exams" className="text-xs font-semibold text-[var(--brand-ink)] hover:underline">{t('m12EnterMarks')}</Link>}</li>)}</ul>}
      </Panel>}
      {analytics && <Panel title={t('m12StudentsNeedingSupport')} icon="alert" action={<ViewAll to="/teacher/analytics" />}>
        {risks.isPending ? <Lines /> : risks.isError ? <Unavailable error={risks.error} /> : risks.data.items.length === 0 ? <Empty>{t('m11NoAlerts')}</Empty> :
          <ul className="space-y-2">{risks.data.items.map(r => <li key={r.student_id}><Link to={`/analytics/students/${r.student_id}`} className="flex items-center justify-between gap-3 rounded-xl border border-slate-200 px-3 py-2 hover:border-[var(--brand-primary)]">
            <span className="min-w-0"><span className="block truncate text-sm font-medium text-slate-900">{local(r.name_en, r.name_bn)}</span><span className="text-xs text-slate-500">{r.class_name} · {r.section_name}</span></span>
            <StatusBadge tone={r.level === 'dropout' ? 'absent' : 'late'}>{t(`m11Level_${r.level}`)}</StatusBadge></Link></li>)}</ul>}
      </Panel>}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </div>
    <QuickLinks />
  </div>
}

function StudentBody({ data, children }: { data: StudentSummary; children?: ReactNode }) {
  const { t } = useTranslation()
  const f = useFmt()
  const latest = data.results[data.results.length - 1]
  return <>
    {data.alert && <div role="status" className="flex items-start gap-3 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
      <Icon name="alert" className="mt-0.5 h-5 w-5 shrink-0" /><span><strong className="font-semibold">{t(`m11AlertTitle_${data.alert.level}`)}</strong> — {data.alert.reasons.map(r => t(`m11Reason_${r}`)).join(', ')}</span></div>}
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Metric label={t('m11Last30')} value={f.pct(data.attendance?.last_30_days.rate)} hint={data.attendance ? t('m11AbsentDays', { n: f.num(data.attendance.last_30_days.absent), count: data.attendance.last_30_days.absent }) : t('m11ModuleOffShort')} icon="check" />
      <Metric label={t('m11SessionAttendance')} value={f.pct(data.attendance?.session.rate)} hint={data.attendance ? t('m11DaysMarked', { n: f.num(data.attendance.session.marked), count: data.attendance.session.marked }) : undefined} icon="calendar" />
      <Metric label={t('m12LatestGpa')} value={latest ? `${latest.grade} · ${f.gpa(latest.gpa)}` : '—'} hint={latest ? latest.name : t('m11NoResultsYet')} tone={latest && !latest.passed ? 'alert' : undefined} icon="grade" />
      {data.fees ? <Metric label={t('m11FeesDue')} value={f.money(data.fees.outstanding)} tone={data.fees.overdue > 0 ? 'alert' : undefined} hint={data.fees.overdue > 0 ? t('m11OverdueAmount', { amount: f.money(data.fees.overdue) }) : data.fees.next_due ? t('m11NextDue', { date: f.fullDay(data.fees.next_due) }) : t('m11NothingDue')} icon="money" />
        : <Metric label={t('m11Position')} value={latest ? f.num(latest.section_position) : '—'} hint={latest ? t('m11ClassPosition', { n: f.num(latest.class_position) }) : undefined} icon="trend" />}
    </div>
    {children}
  </>
}

function DiaryPanel({ studentId, link }: { studentId: string | null; link: string }) {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const q = useQuery({ queryKey: ['dash', 'diary', studentId], queryFn: () => api(token => getDiaryFeed(token, studentId, new URLSearchParams({ page: '1', page_size: '3' }))), retry: false })
  return <Panel title={t('m12RecentDiary')} icon="notebook" action={<ViewAll to={link} />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : q.data.items.length === 0 ? <Empty>{t('m12NoDiary')}</Empty> :
      <ul className="space-y-3">{q.data.items.map(d => <li key={d.id} className="rounded-xl border border-slate-200 px-3 py-2.5">
        <p className="flex items-center justify-between gap-2 text-xs text-slate-500"><span className="font-semibold text-slate-700">{local(d.subject_name_en, d.subject_name_bn)}</span>{f.fullDay(d.diary_date)}</p>
        <p className="mt-1 text-sm text-slate-900">{d.lesson_topics || d.class_notes}</p>
        {d.homework && <p className="mt-1 text-xs text-slate-600"><span className="font-semibold">{t('m12Homework')}:</span> {d.homework}</p>}</li>)}</ul>}
  </Panel>
}

function StudentDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const api = useApi()
  const analytics = ctx.nav.has('analytics_student')
  const q = useQuery({ queryKey: ['analytics', 'me'], enabled: analytics, queryFn: () => api(getMySummary), retry: false })
  return <div className="space-y-6">
    <Greeting user={ctx.user} branding={ctx.branding} role="student">
      {ctx.nav.has('exam_student') && <Link to="/student/results" className={ui.secondary}><Icon name="grade" className="h-4 w-4" />{t('m12ViewResults')}</Link>}
      {analytics && <Link to="/student/progress" className={ui.primary}><Icon name="trend" className="h-4 w-4" />{t('m12MyProgress')}</Link>}
    </Greeting>
    {analytics && (q.isPending ? <Lines rows={2} /> : q.isError ? <Unavailable error={q.error} /> : <StudentBody data={q.data} />)}
    <div className="grid gap-6 xl:grid-cols-3">
      {ctx.nav.has('routine_student') && <RoutineToday path="/routines/classes/me" title={t('m12TodaysClasses')} />}
      {ctx.nav.has('diary_student') && <DiaryPanel studentId={null} link="/student/diary" />}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </div>
    <QuickLinks />
  </div>
}

function GuardianDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const analytics = ctx.nav.has('analytics_guardian')
  const q = useQuery({ queryKey: ['analytics', 'children'], enabled: analytics, queryFn: () => api(getChildren), retry: false })
  const [selected, setSelected] = useState('')
  const children = q.data?.items ?? []
  const child = children.find(c => c.student.id === selected) ?? children[0]
  type ChildLink = { key: string; to: string; icon: IconName; label: string }
  const links: ChildLink[] = child ? ([
    { key: 'guardian_attendance', to: `/guardian/children/${child.student.id}/attendance`, icon: 'check', label: 'm12ChildAttendance' },
    { key: 'exam_guardian', to: `/guardian/children/${child.student.id}/results`, icon: 'grade', label: 'm12ChildResults' },
    { key: 'routine_guardian', to: `/guardian/children/${child.student.id}/routine`, icon: 'clock', label: 'm12ChildRoutine' },
    { key: 'fees_guardian', to: `/guardian/children/${child.student.id}/fees`, icon: 'money', label: 'm12ChildFees' },
    { key: 'diary_guardian', to: `/guardian/children/${child.student.id}/diary`, icon: 'notebook', label: 'm12ChildDiary' },
  ] as ChildLink[]).filter(l => ctx.nav.has(l.key)) : []
  return <div className="space-y-6">
    <Greeting user={ctx.user} branding={ctx.branding} role="guardian" />
    {analytics && (q.isPending ? <Lines rows={2} /> : q.isError ? <Unavailable error={q.error} /> : children.length === 0 ? <Empty>{t('m11NoChildrenHint')}</Empty> : <>
      {children.length > 1 && <div role="radiogroup" aria-label={t('m11ChooseChild')} className="flex flex-wrap gap-2">
        {children.map(c => <button key={c.student.id} type="button" role="radio" aria-checked={c.student.id === child?.student.id} onClick={() => setSelected(c.student.id)}
          className={`flex w-full items-center gap-3 rounded-xl border px-3 py-2 text-left transition sm:w-auto focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${c.student.id === child?.student.id ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200 bg-white hover:bg-slate-50'}`}>
          <span aria-hidden="true" className="shell-avatar flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold">{[...local(c.student.name_en, c.student.name_bn)][0]}</span>
          <span><span className="block text-sm font-semibold text-slate-900">{local(c.student.name_en, c.student.name_bn)}</span><span className="text-xs text-slate-500">{c.student.class_name} · {c.student.section_name}</span></span>
          {c.alert && <span className="h-2 w-2 rounded-full bg-amber-500" aria-label={t('m11NeedsAttention')} />}</button>)}
      </div>}
      {child && <StudentBody data={child}>
        {links.length > 0 && <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">{links.map(l => <li key={l.key}><Link to={l.to} className="flex h-full items-center gap-2.5 rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm font-medium text-slate-700 hover:border-[var(--brand-primary)] hover:text-[var(--brand-ink)]"><Icon name={l.icon} className="h-[18px] w-[18px] text-slate-500" />{t(l.label)}</Link></li>)}</ul>}
      </StudentBody>}
    </>)}
    <div className="grid gap-6 xl:grid-cols-3">
      {child && ctx.nav.has('routine_guardian') && <RoutineToday key={child.student.id} path={`/routines/classes/children/${encodeURIComponent(child.student.id)}`} title={t('m12ChildToday', { name: local(child.student.name_en, child.student.name_bn).split(' ')[0] })} />}
      {child && ctx.nav.has('diary_guardian') && <DiaryPanel key={child.student.id} studentId={child.student.id} link={`/guardian/children/${child.student.id}/diary`} />}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </div>
    {!analytics && <QuickLinks />}
  </div>
}

function AccountantDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const today = dhakaNow().day
  const analytics = ctx.nav.has('analytics') && ctx.permissions.includes('analytics:finance')
  const year = useQuery({ queryKey: ['dash', 'fees-year'], enabled: analytics, queryFn: () => api(token => getFees(token, {})), retry: false })
  const todayFees = useQuery({ queryKey: ['dash', 'fees-today', today], enabled: analytics, queryFn: () => api(token => getFees(token, { from: today, to: today })), retry: false })
  const receipts = useQuery({ queryKey: ['dash', 'receipts'], enabled: ctx.nav.has('fees_admin'), queryFn: () => api(token => getFeePayments(token, new URLSearchParams({ page: '1', page_size: '6' }))), retry: false })
  const local = useLocal()
  const s = year.data?.summary
  return <div className="space-y-6">
    <Greeting user={ctx.user} branding={ctx.branding} role="accountant">
      {ctx.nav.has('fees_admin') && <Link to="/accounts/fees" className={ui.primary}><Icon name="receipt" className="h-4 w-4" />{t('m12CollectPayment')}</Link>}
      {ctx.nav.has('reports') && <Link to="/reports" className={ui.secondary}><Icon name="report" className="h-4 w-4" />{t('m12FinanceReports')}</Link>}
    </Greeting>
    {analytics && (year.isPending ? <Lines rows={2} /> : year.isError ? <Unavailable error={year.error} /> : s && <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Metric label={t('m12CollectedToday')} value={todayFees.data ? f.money(todayFees.data.summary.collected) : '—'} hint={f.fullDay(today)} icon="receipt" />
      <Metric label={t('m12Collected12m')} value={f.money(s.collected)} hint={t('m11CollectionRule', { paid: f.money(s.paid_against_billed), billed: f.money(s.net_billed) })} icon="money" />
      <Metric label={t('m11Outstanding')} value={f.money(s.outstanding)} hint={t('m11AsOfToday')} icon="clipboard" />
      <Metric label={t('m11Overdue')} value={f.money(s.overdue)} tone={s.overdue > 0 ? 'alert' : undefined} hint={t('m11OverdueStudents', { n: f.num(s.overdue_students), count: s.overdue_students })} icon="alert" to="/analytics" />
    </div>)}
    <div className="grid gap-6 xl:grid-cols-3">
      {ctx.nav.has('fees_admin') && <Panel title={t('m12RecentReceipts')} icon="receipt" action={<ViewAll to="/accounts/fees" />} className="xl:col-span-2">
        {receipts.isPending ? <Lines rows={4} /> : receipts.isError ? <Unavailable error={receipts.error} /> : receipts.data.items.length === 0 ? <Empty>{t('m12NoReceipts')}</Empty> :
          <div className="-mx-1 overflow-x-auto"><table className="w-full min-w-[30rem] text-left text-sm">
            <thead className="text-xs text-slate-500"><tr><th className="px-1 py-2 font-medium">{t('m12Receipt')}</th><th className="px-1 py-2 font-medium">{t('m12Student')}</th><th className="px-1 py-2 font-medium">{t('m12Method')}</th><th className="px-1 py-2 text-right font-medium">{t('m12Amount')}</th></tr></thead>
            <tbody className="divide-y divide-slate-100">{receipts.data.items.map(p => <tr key={p.id}>
              <td className="px-1 py-2.5"><span className="font-medium text-slate-900">{p.receipt_no}</span><span className="block text-xs text-slate-500">{f.fullDay(p.paid_on)}</span></td>
              <td className="px-1 py-2.5"><span className="text-slate-900">{local(p.student_name, p.student_name_bn)}</span><span className="block text-xs text-slate-500">{p.class_name} · {p.section_name}</span></td>
              <td className="px-1 py-2.5 text-slate-700">{t(`m11Method_${p.method}`)}</td>
              <td className="px-1 py-2.5 text-right font-semibold tabular-nums text-slate-900">{f.money(p.amount)}</td></tr>)}</tbody></table></div>}
      </Panel>}
      {analytics && year.data && <Panel title={t('m11Aging')} icon="clock">
        <ul className="space-y-3">{year.data.aging.map(a => <li key={a.bucket} className="flex items-center justify-between gap-3 text-sm"><span className="text-slate-700">{t(`m11Aging_${a.bucket}`)}</span><span className="font-semibold tabular-nums text-slate-900">{f.money(a.amount)}</span></li>)}</ul>
      </Panel>}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </div>
    <QuickLinks />
  </div>
}

export function RoleDashboard({ user, branding, navigation }: { user: AuthUser; branding?: SchoolBranding; navigation: string[] }) {
  const { t } = useTranslation()
  const api = useApi()
  const perms = useQuery({ queryKey: ['my-permissions', user.id], queryFn: () => api(getMyPermissions), retry: false })
  if (perms.isPending) return <Lines rows={5} />
  if (perms.isError) return <p role="alert" className="text-rose-700">{t('dashboardError')}</p>
  const ctx: Ctx = { user, branding, nav: new Set(navigation), permissions: perms.data }
  switch (user.role) {
    case 'teacher': return <TeacherDashboard ctx={ctx} />
    case 'student': return <StudentDashboard ctx={ctx} />
    case 'guardian': return <GuardianDashboard ctx={ctx} />
    case 'accountant': return <AccountantDashboard ctx={ctx} />
    default: return <AdminDashboard ctx={ctx} />
  }
}
