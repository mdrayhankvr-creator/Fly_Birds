import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getMyPermissions, getOwnAssignments, type AuthUser, type SchoolBranding } from '../api/client'
import { getAdminOverview } from '../api/admin'
import { getAttendance, getChildren, getFees, getMySummary, getOverview, getRisks, type StudentSummary } from '../api/analytics'
import { getDiaryFeed, getEvents, getNoticeFeed, type Notice, type SchoolEvent } from '../api/communication'
import { getFeePayments } from '../api/fees'
import { Icon, type IconName } from '../components/icons'
import { ClassStrip, subjectColour } from '../components/dashboard/ClassStrip'
import { Avatar, DashLayout, Empty, Lines, Panel, QuickActions, Ring, RoleHead, Unavailable, ViewAll, navModule, tone, type QuickAction, type Stat } from '../components/dashboard/kit'
import { useNavGroups } from '../components/navContext'
import { LineChart } from './analyticsCharts'
import { useFmt } from './AnalyticsPages'
import { StatusBadge, ui } from './attendanceUi'
import { useApi, useLocal } from './commShared'

// Role home dashboards. Every figure comes from an existing API. A card whose
// module is off, or that the viewer may not see, is not shown; a card whose
// request fails says so instead of showing a number. Each role opens with
// its own header so the six dashboards are recognisable at a glance.

const DHAKA = 'Asia/Dhaka'
function dhakaNow() {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: DHAKA, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', hour12: false }).formatToParts(new Date())
  const get = (type: string) => parts.find(p => p.type === type)?.value ?? ''
  return { day: `${get('year')}-${get('month')}-${get('day')}`, hour: Number(get('hour')) % 24 }
}
function shift(day: string, days: number) {
  const d = new Date(`${day}T00:00:00Z`); d.setUTCDate(d.getUTCDate() + days); return d.toISOString().slice(0, 10)
}

const ACCENT: Record<string, string> = { admin: '#4338ca', head_teacher: '#be185d', teacher: '#0369a1', accountant: '#047857', student: '#c2410c', guardian: '#0f766e' }

function useGreeting(user: AuthUser, branding?: SchoolBranding) {
  const { t, i18n } = useTranslation()
  const part = (h => h < 12 ? 'm12GoodMorning' : h < 17 ? 'm12GoodAfternoon' : 'm12GoodEvening')(dhakaNow().hour)
  const date = new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: DHAKA }).format(new Date())
  const school = branding ? (i18n.language === 'bn' ? branding.name_bn : branding.name_en) : ''
  const name = user.display_name || user.email
  return { name, title: t(part, { name: name.split(/[\s@]/)[0] }), eyebrow: <>{date}{school && <> · {school}</>}</> }
}

// useNavActions turns navigation keys into quick actions. Navigation is built
// from live permissions and modules, so only links the user may open appear.
function useNavActions(keys: string[]): QuickAction[] {
  const { t } = useTranslation()
  const items = useNavGroups().flatMap(g => g.items)
  return keys.flatMap(key => {
    const item = items.find(i => i.key === key)
    return item ? [{ key: item.key, to: item.path, icon: item.icon, label: t(item.label), module: navModule(item.key) }] : []
  })
}

function Actions({ keys }: { keys: string[] }) {
  const { t } = useTranslation()
  return <QuickActions items={useNavActions(keys)} label={t('p1QuickActions')} />
}

// ---------- shared panels ----------

function NoticesPanel() {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const q = useQuery({ queryKey: ['dash', 'notices'], queryFn: () => api(token => getNoticeFeed(token, new URLSearchParams({ page: '1', page_size: '4' }))), retry: false })
  return <Panel title={t('m12LatestNotices')} icon="megaphone" module="notices" action={<ViewAll to="/notices" />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : q.data.items.length === 0 ? <Empty>{t('m12NoNotices')}</Empty> :
      <ul className="space-y-2">{q.data.items.map((n: Notice) => <li key={n.id}>
        <Link to="/notices" className={`group block rounded-xl px-3 py-2.5 ds-softer ${n.priority !== 'normal' ? 'ds-edge' : ''} focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]`} style={tone(n.priority === 'urgent' ? 'notices' : n.priority === 'normal' ? 'admin' : 'alerts')}>
          <p className="flex items-start gap-2"><span className="min-w-0 flex-1 text-sm font-semibold text-slate-900 group-hover:underline">{local(n.title_en, n.title_bn)}</span>
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
  return <Panel title={t('m12UpcomingEvents')} icon="calendar" module="events" action={<ViewAll to="/events" />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : q.data.length === 0 ? <Empty>{t('m12NoEvents')}</Empty> :
      <ul className="space-y-3">{q.data.filter((e: SchoolEvent) => e.status === 'published').slice(0, 4).map(e => {
        const d = new Date(`${e.starts_on}T00:00:00Z`)
        return <li key={e.id} className="flex items-center gap-3">
          <span className="ds-soft flex h-11 w-11 shrink-0 flex-col items-center justify-center rounded-xl leading-none">
            <span className="ds-ink text-[10px] font-bold uppercase">{new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { month: 'short', timeZone: 'UTC' }).format(d)}</span>
            <span className="mt-0.5 text-base font-bold tabular-nums text-slate-900">{f.num(d.getUTCDate())}</span></span>
          <span className="min-w-0"><span className="block truncate text-sm font-medium text-slate-900">{local(e.title_en, e.title_bn)}</span><span className="text-xs text-slate-500">{t(`m9Kind_${e.kind}`)}{e.ends_on !== e.starts_on ? ` · ${f.fullDay(e.starts_on)} – ${f.fullDay(e.ends_on)}` : ''}</span></span>
        </li>
      })}</ul>}
  </Panel>
}

function AttendanceTrend({ title }: { title: string }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'attendance-trend'], queryFn: () => api(token => getAttendance(token, {})), retry: false })
  return <Panel title={title} icon="trend" module="attendance" action={q.data ? <span className="text-sm font-semibold tabular-nums text-slate-900">{f.pct(q.data.summary.rate)}</span> : undefined}>
    {q.isPending ? <Lines rows={4} /> : q.isError ? <Unavailable error={q.error} /> :
      <LineChart title={title} empty={t('m11NoAttendance')} format={v => f.pct(v)} height={190} points={q.data.trend.map(p => ({ label: f.day(p.period), value: p.rate, detail: t('m11TrendDetail', { present: f.num(p.present), late: f.num(p.late), absent: f.num(p.absent) }) }))} />}
  </Panel>
}

function useOverview(enabled: boolean) {
  const api = useApi()
  return useQuery({ queryKey: ['dash', 'overview'], enabled, queryFn: () => api(token => getOverview(token)), retry: false })
}

function Insights() {
  const { t } = useTranslation()
  const f = useFmt()
  const q = useOverview(true)
  const d = q.data
  const items: { icon: IconName; text: string; to: string; alert: boolean }[] = []
  if (d?.risks?.dropout) items.push({ icon: 'alert', text: t('m11InsightDropout', { n: f.num(d.risks.dropout), count: d.risks.dropout }), to: '/analytics', alert: true })
  if (d?.risks?.at_risk) items.push({ icon: 'trend', text: t('m11InsightAtRisk', { n: f.num(d.risks.at_risk), count: d.risks.at_risk }), to: '/analytics', alert: false })
  if (d?.today_students && d.active_students > d.today_students.marked) items.push({ icon: 'clipboard', text: t('m11InsightUnmarked', { n: f.num(d.active_students - d.today_students.marked), count: d.active_students - d.today_students.marked }), to: '/admin/attendance/students', alert: false })
  if (d?.fees && d.fees.overdue > 0) items.push({ icon: 'money', text: t('m11InsightOverdue', { amount: f.money(d.fees.overdue) }), to: '/accounts/fees', alert: true })
  return <Panel title={t('m11Insights')} icon="alert" module="alerts">
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : items.length === 0 ? <Empty>{t('m11NoInsights')}</Empty> :
      <ul className="space-y-2">{items.map((i, n) => <li key={n}><Link to={i.to} style={tone(i.alert ? 'notices' : 'alerts')} className="ds-tint group flex items-center gap-3 px-3 py-2.5 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
        <span className="ds-fill flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"><Icon name={i.icon} className="h-4 w-4" /></span>
        <span className="min-w-0 flex-1 text-sm font-medium text-slate-800">{i.text}</span><Icon name="chevronRight" className="h-4 w-4 shrink-0 text-slate-500" /></Link></li>)}</ul>}
  </Panel>
}

function SystemPanel() {
  const { t, i18n } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const q = useQuery({ queryKey: ['dash', 'system'], queryFn: () => api(getAdminOverview), retry: false })
  const d = q.data
  return <Panel title={t('m12SystemHealth')} icon="shield" module="admin" action={<ViewAll to="/admin/control" />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : d && <dl className="grid grid-cols-2 gap-2">
      {[
        { label: t('m12Accounts'), value: f.num(d.users.reduce((s, r) => s + r.active, 0)) },
        { label: t('m10Modules'), value: t('m10ModulesOn', { enabled: f.num(d.modules_enabled), total: f.num(d.modules_total) }) },
        { label: t('m10LastBackup'), value: d.last_backup ? new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { dateStyle: 'medium', timeZone: DHAKA }).format(new Date(d.last_backup.started_at)) : <span className="text-amber-700">{t('m10Never')}</span> },
        { label: t('m10DeniedAttempts'), value: <span className={d.denied_last_day ? 'text-rose-700' : ''}>{f.num(d.denied_last_day)}</span> },
      ].map(x => <div key={x.label} className="ds-sunken min-w-0 px-3 py-2.5"><dt className="truncate text-xs text-slate-500">{x.label}</dt><dd className="mt-0.5 text-sm font-semibold tabular-nums text-slate-900">{x.value}</dd></div>)}
    </dl>}
  </Panel>
}

function DiaryPanel({ studentId, link }: { studentId: string | null; link: string }) {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const q = useQuery({ queryKey: ['dash', 'diary', studentId], queryFn: () => api(token => getDiaryFeed(token, studentId, new URLSearchParams({ page: '1', page_size: '3' }))), retry: false })
  return <Panel title={t('m12RecentDiary')} icon="notebook" module="diary" action={<ViewAll to={link} />}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : q.data.items.length === 0 ? <Empty>{t('m12NoDiary')}</Empty> :
      <ul className="space-y-2.5">{q.data.items.map(d => <li key={d.id} className="ds-softer ds-edge rounded-xl px-3.5 py-2.5" style={tone(subjectColour(d.subject_id))}>
        <p className="flex items-center justify-between gap-2 text-xs"><span className="ds-ink font-bold">{local(d.subject_name_en, d.subject_name_bn)}</span><span className="text-slate-500">{f.fullDay(d.diary_date)}</span></p>
        <p className="mt-1 text-sm text-slate-900">{d.lesson_topics || d.class_notes}</p>
        {d.homework && <p className="mt-1.5 flex items-start gap-1.5 text-xs text-slate-700" style={tone('homework')}><Icon name="book" className="ds-ink mt-px h-3.5 w-3.5 shrink-0" /><span><span className="ds-ink font-bold">{t('m12Homework')}:</span> {d.homework}</span></p>}</li>)}</ul>}
  </Panel>
}

// ---------- administrator ----------

type Ctx = { user: AuthUser; branding?: SchoolBranding; nav: Set<string>; permissions: string[] }

// The school pulse: a deep panel with today's headline figures from the analytics overview.
function AdminPulse({ ctx, analytics, actions }: { ctx: Ctx; analytics: boolean; actions: ReactNode }) {
  const { t } = useTranslation()
  const f = useFmt()
  const g = useGreeting(ctx.user, ctx.branding)
  const q = useOverview(analytics)
  const d = q.data
  type Cell = { label: string; value: string; foot?: string; icon: IconName; color: string; meter?: number | null; to?: string; alert?: boolean }
  const cells: Cell[] = d ? [
    { label: t('m11ActiveStudents'), value: f.num(d.active_students), foot: t('m11StaffLine', { teachers: f.num(d.teachers), staff: f.num(d.non_teaching_staff) }), icon: 'users', color: '#a5b4fc', to: ctx.nav.has('students') ? '/admin/students' : undefined },
    { label: t('p1StudentsToday'), value: f.pct(d.today_students?.rate), foot: d.today_students ? t('m11MarkedOf', { marked: f.num(d.today_students.marked), total: f.num(d.active_students) }) : t('m11ModuleOffShort'), icon: 'check', color: '#34d399', meter: d.today_students?.rate, to: ctx.nav.has('student_attendance') ? '/admin/attendance/students' : undefined },
    { label: t('p1TeachersPresent'), value: f.pct(d.today_teachers?.rate), foot: d.today_teachers ? t('p1TeachersMarked', { marked: f.num(d.today_teachers.marked), total: f.num(d.teachers) }) : t('m11ModuleOffShort'), icon: 'teacher', color: '#7dd3fc', meter: d.today_teachers?.rate, to: ctx.nav.has('teacher_attendance') ? '/admin/attendance/teachers' : undefined },
    { label: t('m11PassRate'), value: f.pct(d.results?.pass_rate), foot: d.results ? t('m11AvgGpa', { gpa: f.gpa(d.results.avg_gpa) }) : t('m11ModuleOffShort'), icon: 'grade', color: '#d8b4fe', meter: d.results?.pass_rate, to: '/analytics' },
    d.fees
      ? { label: t('m11Outstanding'), value: f.money(d.fees.outstanding), foot: t('m11OverdueAmount', { amount: f.money(d.fees.overdue) }), icon: 'money', color: '#fcd34d', to: '/accounts/fees', alert: d.fees.overdue > 0 }
      : { label: t('m11DropoutAlerts'), value: f.num(d.risks?.dropout ?? 0), foot: t('m11AtRiskCount', { n: f.num(d.risks?.at_risk ?? 0) }), icon: 'alert', color: '#fda4af', to: '/analytics', alert: !!d.risks?.dropout },
  ] : []
  return <section className="ds-pulse ds-rise p-4 sm:p-5">
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <p className="ds-pulse-soft text-xs font-semibold">{g.eyebrow}</p>
        <h1 className="mt-0.5 text-[22px] font-extrabold leading-tight tracking-tight text-white sm:text-[26px]">{g.title}</h1>
        <p className="ds-pulse-soft mt-0.5 text-sm">{t(`m12RoleIntro_${ctx.user.role}`)}</p>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>
    </div>
    {analytics && (q.isPending ? <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5" aria-hidden="true">{Array.from({ length: 5 }, (_, i) => <div key={i} className={`ds-pulse-cell h-[104px] animate-pulse motion-reduce:animate-none ${i === 0 ? 'col-span-2 sm:col-span-1' : ''}`} />)}</div>
      : q.isError ? <div className="mt-4"><Unavailable error={q.error} /></div>
      : <ul className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-2.5 lg:grid-cols-5">{cells.map((c, i) => {
        const body = <>
          <p className="ds-pulse-soft flex items-center gap-1.5 text-[11px] font-semibold"><Icon name={c.icon} className="h-3.5 w-3.5 shrink-0" style={{ color: c.color }} /><span className="truncate">{c.label}</span></p>
          <p className={`mt-1 truncate text-[22px] font-extrabold tabular-nums tracking-tight ${c.alert ? 'text-rose-300' : 'text-white'}`}>{c.value}</p>
          <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-white/10">{c.meter != null && <span className="block h-full rounded-full" style={{ width: `${Math.max(0, Math.min(100, c.meter))}%`, background: c.color }} />}</div>
          {c.foot && <p className="ds-pulse-soft mt-1.5 truncate text-[11px]">{c.foot}</p>}
        </>
        return <li key={c.label} className={`min-w-0 ${i === 0 ? 'col-span-2 sm:col-span-1' : ''}`}>
          {c.to ? <Link to={c.to} className="ds-pulse-cell block h-full p-3 transition hover:bg-white/10">{body}</Link> : <div className="ds-pulse-cell h-full p-3">{body}</div>}
        </li>
      })}</ul>)}
  </section>
}

function AdminDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const g = useGreeting(ctx.user, ctx.branding)
  const analytics = ctx.nav.has('analytics') && ctx.permissions.includes('analytics:read')
  const system = ctx.user.role === 'admin' && ctx.permissions.includes('system:read')
  // Without analytics, administrators still see real figures from the control-centre summary.
  const summary = useQuery({ queryKey: ['dash', 'system'], enabled: !analytics && ctx.permissions.includes('system:read'), queryFn: () => api(getAdminOverview), retry: false })
  const buttons = <>
    {ctx.nav.has('student_attendance') && <Link to="/admin/attendance/students" className={ui.secondary}><Icon name="check" className="h-4 w-4" />{t('m12TakeAttendance')}</Link>}
    {analytics && <Link to="/analytics" className={ui.primary}><Icon name="chart" className="h-4 w-4" />{t('m12OpenAnalytics')}</Link>}
  </>
  const fallbackStats: Stat[] | undefined = summary.data ? [
    { label: t('m10ActiveStudents'), value: f.num(summary.data.active_students), icon: 'users', color: 'people' },
    { label: t('m10ActiveStaff'), value: f.num(summary.data.active_staff), icon: 'teacher', color: 'people' },
    { label: t('m10Guardians'), value: f.num(summary.data.guardians), icon: 'user', color: 'people' },
    { label: t('m10FeesOutstanding'), value: f.money(summary.data.fees_outstanding), icon: 'money', color: 'fees' },
  ] : undefined
  return <DashLayout
    head={analytics ? <AdminPulse ctx={ctx} analytics={analytics} actions={buttons} />
      : <RoleHead accent={ACCENT.admin} eyebrow={g.eyebrow} title={g.title} sub={t(`m12RoleIntro_${ctx.user.role}`)} lead={<Avatar name={g.name} />} actions={buttons} stats={fallbackStats} />}
    actions={<Actions keys={['students', 'fees', 'notices', 'staff', 'settings']} />}
    main={<>
      {analytics && ctx.nav.has('student_attendance') && <AttendanceTrend title={t('m12AttendanceLast30')} />}
      <div className="grid gap-4 md:grid-cols-2 md:gap-5">
        {analytics && <Insights />}
        {ctx.nav.has('notices') && <NoticesPanel />}
      </div>
    </>}
    side={<>
      {system && <SystemPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </>} />
}

// ---------- head teacher ----------

function HeadTeacherDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const g = useGreeting(ctx.user, ctx.branding)
  const analytics = ctx.nav.has('analytics') && ctx.permissions.includes('analytics:read')
  const q = useOverview(analytics)
  const d = q.data
  const stats: Stat[] | undefined = d ? [
    { label: t('p1StudentsToday'), value: f.pct(d.today_students?.rate), hint: d.today_students ? t('m11MarkedOf', { marked: f.num(d.today_students.marked), total: f.num(d.active_students) }) : t('m11ModuleOffShort'), icon: 'check', color: 'attendance', to: ctx.nav.has('student_attendance') ? '/admin/attendance/students' : undefined },
    { label: t('p1TeachersPresent'), value: f.pct(d.today_teachers?.rate), hint: d.today_teachers ? t('p1TeachersMarked', { marked: f.num(d.today_teachers.marked), total: f.num(d.teachers) }) : t('m11ModuleOffShort'), icon: 'teacher', color: 'people', to: ctx.nav.has('teacher_attendance') ? '/admin/attendance/teachers' : undefined },
    { label: t('m11PassRate'), value: f.pct(d.results?.pass_rate), hint: d.results ? t('m11AvgGpa', { gpa: f.gpa(d.results.avg_gpa) }) : t('m11ModuleOffShort'), icon: 'grade', color: 'exams', to: '/analytics' },
    { label: t('m11DropoutAlerts'), value: f.num(d.risks?.dropout ?? 0), hint: t('m11AtRiskCount', { n: f.num(d.risks?.at_risk ?? 0) }), icon: 'alert', color: 'alerts', alert: !!d.risks?.dropout, to: '/analytics' },
  ] : undefined
  return <DashLayout
    head={<>
      <RoleHead accent={ACCENT.head_teacher} eyebrow={g.eyebrow} title={g.title} sub={t('m12RoleIntro_head_teacher')} lead={<Avatar name={g.name} />} stats={stats}
        actions={<>
          {ctx.nav.has('student_attendance') && <Link to="/admin/attendance/students" className={ui.secondary}><Icon name="check" className="h-4 w-4" />{t('m12TakeAttendance')}</Link>}
          {analytics && <Link to="/analytics" className={ui.primary}><Icon name="chart" className="h-4 w-4" />{t('m12OpenAnalytics')}</Link>}
        </>} />
      {analytics && (q.isPending ? <Lines rows={1} /> : q.isError ? <Unavailable error={q.error} /> : null)}
    </>}
    actions={<Actions keys={['student_attendance', 'teacher_attendance', 'exams', 'routines', 'notices']} />}
    main={<>
      {analytics && ctx.nav.has('student_attendance') && <AttendanceTrend title={t('m12AttendanceLast30')} />}
      <div className="grid gap-4 md:grid-cols-2 md:gap-5">
        {analytics && <Insights />}
        {ctx.nav.has('notices') && <NoticesPanel />}
      </div>
    </>}
    side={ctx.nav.has('events') ? <EventsPanel /> : undefined} />
}

// ---------- teacher ----------

function TeacherDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const local = useLocal()
  const g = useGreeting(ctx.user, ctx.branding)
  const assignments = useQuery({ queryKey: ['dash', 'assignments'], enabled: ctx.permissions.includes('staff:read_self') && ctx.nav.has('staff_self'), queryFn: () => api(token => getOwnAssignments(token, new URLSearchParams({ page: '1', page_size: '50' }))), retry: false })
  const analytics = ctx.nav.has('analytics_teacher')
  const risks = useQuery({ queryKey: ['dash', 'teacher-risks'], enabled: analytics, queryFn: () => api(token => getRisks(token, {}, '', '', 1, 5)), retry: false })
  const att = useQuery({ queryKey: ['dash', 'attendance-trend'], enabled: analytics && ctx.nav.has('student_attendance'), queryFn: () => api(token => getAttendance(token, {})), retry: false })
  const classes = [...new Map((assignments.data?.items ?? []).map(a => [a.class_id + a.subject_id, a])).values()]
  const classCount = new Set(classes.map(c => c.class_id)).size
  const stats: Stat[] | undefined = analytics ? [
    { label: t('m12MyClasses'), value: assignments.data ? f.num(classCount) : '—', hint: assignments.data ? t('m12SubjectsTaught', { n: f.num(classes.length), count: classes.length }) : undefined, icon: 'layers', color: 'routine' },
    { label: t('m11Last30'), value: f.pct(att.data?.summary.rate), hint: t('m12ForMyClasses'), icon: 'check', color: 'attendance', to: '/teacher/analytics' },
    { label: t('m11DropoutAlerts'), value: risks.data ? f.num(risks.data.summary.dropout) : '—', icon: 'alert', color: 'notices', alert: !!risks.data?.summary.dropout, to: '/teacher/analytics' },
    { label: t('m11AtRiskAlerts'), value: risks.data ? f.num(risks.data.summary.at_risk) : '—', icon: 'trend', color: 'alerts', to: '/teacher/analytics' },
  ] : undefined
  return <DashLayout
    head={<RoleHead accent={ACCENT.teacher} eyebrow={g.eyebrow} title={g.title} lead={<Avatar name={g.name} />} stats={stats}
      sub={assignments.data && classes.length ? t('p1TeacherSub', { classes: f.num(classCount), subjects: f.num(classes.length) }) : t('m12RoleIntro_teacher')}
      actions={<>
        {ctx.nav.has('student_attendance') && <Link to="/admin/attendance/students" className={ui.primary}><Icon name="check" className="h-4 w-4" />{t('m12TakeAttendance')}</Link>}
        {ctx.nav.has('diary_teacher') && <Link to="/teacher/diary" className={ui.secondary}><Icon name="notebook" className="h-4 w-4" />{t('m12WriteDiary')}</Link>}
      </>} />}
    actions={<Actions keys={['student_attendance', 'diary', 'exams_me', 'routine_me', 'analytics_teacher']} />}
    main={<>
      {ctx.nav.has('routine_teacher') && <ClassStrip path="/routines/classes/me" title={t('m12TodaysClasses')} teacherView link="/teacher/routine" />}
      {assignments.data && <Panel title={t('m12AssignedClasses')} icon="teacher" module="people">
        {classes.length === 0 ? <Empty>{t('m12NoAssignments')}</Empty> : <ul className="grid gap-2 sm:grid-cols-2">{classes.map(a => <li key={a.id} className="ds-softer ds-edge flex items-center justify-between gap-3 rounded-xl px-3.5 py-2.5" style={tone(subjectColour(a.subject_id))}>
          <span className="min-w-0"><span className="block truncate text-sm font-semibold text-slate-900">{a.class_name}</span><span className="ds-ink text-xs font-semibold">{local(a.subject_name_en, a.subject_name_bn)}</span></span>
          {ctx.nav.has('exam_teacher') && <Link to="/teacher/exams" className="shrink-0 text-xs font-semibold text-[var(--brand-ink)] hover:underline">{t('m12EnterMarks')}</Link>}</li>)}</ul>}
      </Panel>}
    </>}
    side={<>
      {analytics && <Panel title={t('m12StudentsNeedingSupport')} icon="alert" module="alerts" action={<ViewAll to="/teacher/analytics" />}>
        {risks.isPending ? <Lines /> : risks.isError ? <Unavailable error={risks.error} /> : risks.data.items.length === 0 ? <Empty>{t('m11NoAlerts')}</Empty> :
          <ul className="space-y-2">{risks.data.items.map(r => <li key={r.student_id}><Link to={`/analytics/students/${r.student_id}`} className="flex items-center justify-between gap-3 rounded-xl border border-slate-200 px-3 py-2 hover:border-[var(--brand-primary)]">
            <span className="min-w-0"><span className="block truncate text-sm font-medium text-slate-900">{local(r.name_en, r.name_bn)}</span><span className="text-xs text-slate-500">{r.class_name} · {r.section_name}</span></span>
            <StatusBadge tone={r.level === 'dropout' ? 'absent' : 'late'}>{t(`m11Level_${r.level}`)}</StatusBadge></Link></li>)}</ul>}
      </Panel>}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </>} />
}

// ---------- student and guardian ----------

function studentStats(data: StudentSummary, t: (key: string, o?: Record<string, unknown>) => string, f: ReturnType<typeof useFmt>): Stat[] {
  const latest = data.results[data.results.length - 1]
  return [
    { label: t('m11Last30'), value: f.pct(data.attendance?.last_30_days.rate), hint: data.attendance ? t('m11AbsentDays', { n: f.num(data.attendance.last_30_days.absent), count: data.attendance.last_30_days.absent }) : t('m11ModuleOffShort'), icon: 'check', color: 'attendance' },
    { label: t('m11SessionAttendance'), value: f.pct(data.attendance?.session.rate), hint: data.attendance ? t('m11DaysMarked', { n: f.num(data.attendance.session.marked), count: data.attendance.session.marked }) : undefined, icon: 'calendar', color: 'routine' },
    { label: t('m12LatestGpa'), value: latest ? `${latest.grade} · ${f.gpa(latest.gpa)}` : '—', hint: latest ? latest.name : t('m11NoResultsYet'), icon: 'grade', color: 'exams', alert: !!latest && !latest.passed },
    data.fees
      ? { label: t('m11FeesDue'), value: f.money(data.fees.outstanding), alert: data.fees.overdue > 0, hint: data.fees.overdue > 0 ? t('m11OverdueAmount', { amount: f.money(data.fees.overdue) }) : data.fees.next_due ? t('m11NextDue', { date: f.fullDay(data.fees.next_due) }) : t('m11NothingDue'), icon: 'money', color: 'fees' }
      : { label: t('m11Position'), value: latest ? f.num(latest.section_position) : '—', hint: latest ? t('m11ClassPosition', { n: f.num(latest.class_position) }) : undefined, icon: 'trend', color: 'analytics' },
  ]
}

function RiskAlert({ data }: { data: StudentSummary }) {
  const { t } = useTranslation()
  if (!data.alert) return null
  return <div role="status" className="flex items-start gap-3 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
    <Icon name="alert" className="mt-0.5 h-5 w-5 shrink-0" /><span><strong className="font-semibold">{t(`m11AlertTitle_${data.alert.level}`)}</strong> — {data.alert.reasons.map(r => t(`m11Reason_${r}`)).join(', ')}</span></div>
}

function studentLine(s: StudentSummary['student'], t: (key: string, o?: Record<string, unknown>) => string, f: ReturnType<typeof useFmt>) {
  return [t('p1StudentSub', { class: s.class_name, section: s.section_name }), s.roll_no != null ? t('p1StudentRoll', { roll: f.num(s.roll_no) }) : ''].filter(Boolean).join(' · ')
}

function StudentDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const g = useGreeting(ctx.user, ctx.branding)
  const analytics = ctx.nav.has('analytics_student')
  const q = useQuery({ queryKey: ['analytics', 'me'], enabled: analytics, queryFn: () => api(getMySummary), retry: false })
  return <DashLayout
    head={<>
      <RoleHead accent={ACCENT.student} eyebrow={g.eyebrow} title={g.title} lead={<Avatar name={g.name} />}
        sub={q.data ? studentLine(q.data.student, t, f) : t('m12RoleIntro_student')} stats={q.data ? studentStats(q.data, t, f) : undefined}
        actions={<>
          {ctx.nav.has('exam_student') && <Link to="/student/results" className={ui.secondary}><Icon name="grade" className="h-4 w-4" />{t('m12ViewResults')}</Link>}
          {analytics && <Link to="/student/progress" className={ui.primary}><Icon name="trend" className="h-4 w-4" />{t('m12MyProgress')}</Link>}
        </>} />
      {analytics && (q.isPending ? <Lines rows={1} /> : q.isError ? <Unavailable error={q.error} /> : <RiskAlert data={q.data} />)}
    </>}
    actions={<Actions keys={['routine_me', 'diary_me', 'results', 'fees_me', 'attendance_self']} />}
    main={<>
      {ctx.nav.has('routine_student') && <ClassStrip path="/routines/classes/me" title={t('m12TodaysClasses')} link="/student/routine" />}
      {ctx.nav.has('diary_student') && <DiaryPanel studentId={null} link="/student/diary" />}
    </>}
    side={<>
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </>} />
}

function GuardianDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const local = useLocal()
  const g = useGreeting(ctx.user, ctx.branding)
  const analytics = ctx.nav.has('analytics_guardian')
  const q = useQuery({ queryKey: ['analytics', 'children'], enabled: analytics, queryFn: () => api(getChildren), retry: false })
  const [selected, setSelected] = useState('')
  const children = q.data?.items ?? []
  const child = children.find(c => c.student.id === selected) ?? children[0]
  const fallback = useNavActions(['children', 'notifications', 'notices', 'events'])
  // Links for the selected child; each is shown only when its module is on for guardians.
  const childLinks: QuickAction[] = child ? ([
    { key: 'guardian_attendance', to: `/guardian/children/${child.student.id}/attendance`, icon: 'check', label: t('m12ChildAttendance'), module: 'attendance' },
    { key: 'exam_guardian', to: `/guardian/children/${child.student.id}/results`, icon: 'grade', label: t('m12ChildResults'), module: 'exams' },
    { key: 'routine_guardian', to: `/guardian/children/${child.student.id}/routine`, icon: 'clock', label: t('m12ChildRoutine'), module: 'routine' },
    { key: 'fees_guardian', to: `/guardian/children/${child.student.id}/fees`, icon: 'money', label: t('m12ChildFees'), module: 'fees' },
    { key: 'diary_guardian', to: `/guardian/children/${child.student.id}/diary`, icon: 'notebook', label: t('m12ChildDiary'), module: 'diary' },
  ] as QuickAction[]).filter(l => ctx.nav.has(l.key)) : []
  const picker = children.length > 1 && <div role="radiogroup" aria-label={t('m11ChooseChild')} className="ds-scroll-x -mx-1 mb-4 flex gap-2 px-1 pb-0.5">
    {children.map(c => {
      const on = c.student.id === child?.student.id
      return <button key={c.student.id} type="button" role="radio" aria-checked={on} onClick={() => setSelected(c.student.id)}
        className={`flex min-h-[44px] shrink-0 items-center gap-2.5 rounded-full border py-1 pl-1 pr-4 text-left transition focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${on ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200 bg-white hover:bg-slate-50'}`}>
        <span aria-hidden="true" className="ds-fill grid h-9 w-9 place-items-center rounded-full text-xs font-bold">{[...local(c.student.name_en, c.student.name_bn)][0]}</span>
        <span><span className="block text-sm font-semibold leading-tight text-slate-900">{local(c.student.name_en, c.student.name_bn)}</span><span className="text-xs text-slate-500">{c.student.class_name} · {c.student.section_name}</span></span>
        {c.alert && <span className="h-2 w-2 rounded-full bg-amber-500" aria-label={t('m11NeedsAttention')} />}</button>
    })}
  </div>
  return <DashLayout
    head={<>
      <RoleHead accent={ACCENT.guardian} top={picker || undefined} eyebrow={g.eyebrow} title={g.title} lead={<Avatar name={g.name} />}
        sub={child ? `${local(child.student.name_en, child.student.name_bn)} · ${studentLine(child.student, t, f)}` : t('m12RoleIntro_guardian')}
        stats={child ? studentStats(child, t, f) : undefined} />
      {analytics && (q.isPending ? <Lines rows={1} /> : q.isError ? <Unavailable error={q.error} /> : children.length === 0 ? <Empty>{t('m11NoChildrenHint')}</Empty> : child && <RiskAlert data={child} />)}
    </>}
    actions={<QuickActions items={child ? childLinks : fallback} label={t('p1QuickActions')} />}
    main={<>
      {child && ctx.nav.has('routine_guardian') && <ClassStrip key={child.student.id} path={`/routines/classes/children/${encodeURIComponent(child.student.id)}`} link={`/guardian/children/${child.student.id}/routine`}
        title={t('m12ChildToday', { name: local(child.student.name_en, child.student.name_bn).split(' ')[0] })} />}
      {child && ctx.nav.has('diary_guardian') && <DiaryPanel key={child.student.id} studentId={child.student.id} link={`/guardian/children/${child.student.id}/diary`} />}
    </>}
    side={<>
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </>} />
}

// ---------- accountant ----------

// The till: today's collection first, then the year's position, cut like a receipt.
function Till({ ctx, analytics }: { ctx: Ctx; analytics: boolean }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const g = useGreeting(ctx.user, ctx.branding)
  const today = dhakaNow().day
  const year = useQuery({ queryKey: ['dash', 'fees-year'], enabled: analytics, queryFn: () => api(token => getFees(token, {})), retry: false })
  const todayFees = useQuery({ queryKey: ['dash', 'fees-today', today], enabled: analytics, queryFn: () => api(token => getFees(token, { from: today, to: today })), retry: false })
  const s = year.data?.summary
  const buttons = <>
    {ctx.nav.has('fees_admin') && <Link to="/accounts/fees" className={ui.primary}><Icon name="receipt" className="h-4 w-4" />{t('m12CollectPayment')}</Link>}
    {ctx.nav.has('reports') && <Link to="/reports" className={ui.secondary}><Icon name="report" className="h-4 w-4" />{t('m12FinanceReports')}</Link>}
  </>
  if (!analytics) return <RoleHead accent={ACCENT.accountant} eyebrow={g.eyebrow} title={g.title} sub={t('m12RoleIntro_accountant')} lead={<Avatar name={g.name} />} actions={buttons} />
  return <section className="ds-role-head ds-rise" style={tone(ACCENT.accountant)}>
    <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between sm:p-5">
      <div className="min-w-0">
        <p className="text-xs font-semibold text-slate-600">{g.eyebrow}</p>
        <h1 className="mt-0.5 text-[22px] font-extrabold leading-tight tracking-tight text-slate-900 sm:text-[26px]">{g.title}</h1>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">{buttons}</div>
    </div>
    {year.isPending ? <div className="px-4 pb-4 sm:px-5 sm:pb-5"><Lines rows={2} /></div> : year.isError ? <div className="px-4 pb-4 sm:px-5 sm:pb-5"><Unavailable error={year.error} /></div> : s && <>
      <div className="flex items-center gap-4 px-4 sm:px-5">
        <div className="min-w-0 flex-1">
          <p className="ds-eyebrow text-slate-600">{t('m12CollectedToday')} · {f.fullDay(today)}</p>
          <p className="mt-1 truncate text-[32px] font-extrabold leading-none tabular-nums tracking-tight text-slate-900 sm:text-[38px]">{todayFees.data ? f.money(todayFees.data.summary.collected) : todayFees.isError ? '—' : '…'}</p>
          <p className="mt-1.5 text-xs text-slate-600">{t('m11CollectionRule', { paid: f.money(s.paid_against_billed), billed: f.money(s.net_billed) })}</p>
        </div>
        {s.collection_rate != null && <Ring value={s.collection_rate} size={88} color="var(--s)">
          <span><span className="block text-lg font-extrabold leading-none tabular-nums text-slate-900">{f.pct(s.collection_rate)}</span><span className="mt-0.5 block max-w-[4.5rem] text-[10px] font-semibold leading-tight text-slate-600">{t('p1CollectionRate')}</span></span>
        </Ring>}
      </div>
      <div className="ds-ledger-cut mx-4 mt-4 sm:mx-5" />
      <dl className="grid grid-cols-3 gap-2 p-4 sm:p-5">
        {[
          { label: t('m12Collected12m'), value: f.money(s.collected) },
          { label: t('m11Outstanding'), value: f.money(s.outstanding), hint: t('m11AsOfToday') },
          { label: t('m11Overdue'), value: f.money(s.overdue), hint: t('m11OverdueStudents', { n: f.num(s.overdue_students), count: s.overdue_students }), alert: s.overdue > 0 },
        ].map(x => <div key={x.label} className="min-w-0"><dt className="truncate text-[11px] font-medium text-slate-600">{x.label}</dt>
          <dd className={`truncate text-[15px] font-extrabold tabular-nums sm:text-lg ${x.alert ? 'text-rose-700' : 'text-slate-900'}`}>{x.value}</dd>
          {x.hint && <dd className="truncate text-[11px] text-slate-500">{x.hint}</dd>}</div>)}
      </dl>
    </>}
  </section>
}

function AccountantDashboard({ ctx }: { ctx: Ctx }) {
  const { t } = useTranslation()
  const f = useFmt()
  const api = useApi()
  const local = useLocal()
  const analytics = ctx.nav.has('analytics') && ctx.permissions.includes('analytics:finance')
  const year = useQuery({ queryKey: ['dash', 'fees-year'], enabled: analytics, queryFn: () => api(token => getFees(token, {})), retry: false })
  const receipts = useQuery({ queryKey: ['dash', 'receipts'], enabled: ctx.nav.has('fees_admin'), queryFn: () => api(token => getFeePayments(token, new URLSearchParams({ page: '1', page_size: '6' }))), retry: false })
  return <DashLayout
    head={<Till ctx={ctx} analytics={analytics} />}
    actions={<Actions keys={['fees', 'reports', 'analytics', 'notices']} />}
    main={ctx.nav.has('fees_admin') && <Panel title={t('m12RecentReceipts')} icon="receipt" module="fees" action={<ViewAll to="/accounts/fees" />}>
      {receipts.isPending ? <Lines rows={4} /> : receipts.isError ? <Unavailable error={receipts.error} /> : receipts.data.items.length === 0 ? <Empty>{t('m12NoReceipts')}</Empty> :
        <div className="-mx-1 overflow-x-auto"><table className="w-full min-w-[30rem] text-left text-sm">
          <thead className="text-xs text-slate-500"><tr><th className="px-1 py-2 font-medium">{t('m12Receipt')}</th><th className="px-1 py-2 font-medium">{t('m12Student')}</th><th className="px-1 py-2 font-medium">{t('m12Method')}</th><th className="px-1 py-2 text-right font-medium">{t('m12Amount')}</th></tr></thead>
          <tbody className="divide-y divide-slate-100">{receipts.data.items.map(p => <tr key={p.id}>
            <td className="px-1 py-2.5"><span className="font-medium text-slate-900">{p.receipt_no}</span><span className="block text-xs text-slate-500">{f.fullDay(p.paid_on)}</span></td>
            <td className="px-1 py-2.5"><span className="text-slate-900">{local(p.student_name, p.student_name_bn)}</span><span className="block text-xs text-slate-500">{p.class_name} · {p.section_name}</span></td>
            <td className="px-1 py-2.5 text-slate-700">{t(`m11Method_${p.method}`)}</td>
            <td className="px-1 py-2.5 text-right font-semibold tabular-nums text-slate-900">{f.money(p.amount)}</td></tr>)}</tbody></table></div>}
    </Panel>}
    side={<>
      {analytics && year.data && <Panel title={t('m11Aging')} icon="clock" module="alerts">
        <ul className="space-y-3">{year.data.aging.map(a => {
          const max = Math.max(1, ...year.data.aging.map(x => x.amount))
          return <li key={a.bucket}><p className="flex items-center justify-between gap-3 text-sm"><span className="text-slate-700">{t(`m11Aging_${a.bucket}`)}</span><span className="font-semibold tabular-nums text-slate-900">{f.money(a.amount)}</span></p>
            <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-[var(--ds-surface-2)]"><span className="block h-full rounded-full bg-[var(--s)]" style={{ width: `${(a.amount / max) * 100}%` }} /></div></li>
        })}</ul>
      </Panel>}
      {ctx.nav.has('notices') && <NoticesPanel />}
      {ctx.nav.has('events') && <EventsPanel />}
    </>} />
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
    case 'head_teacher': return <HeadTeacherDashboard ctx={ctx} />
    default: return <AdminDashboard ctx={ctx} />
  }
}
