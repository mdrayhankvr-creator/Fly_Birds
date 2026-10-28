import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError } from '../../api/client'
import { ui } from '../../pages/attendanceUi'
import { Icon, type IconName } from '../icons'

// Shared building blocks for dashboards and module screens. Colours come from
// the design tokens in styles.css, so every part follows the school's brand
// and the light or dark theme.

export type Module = 'home' | 'routine' | 'homework' | 'diary' | 'notices' | 'exams' | 'attendance' | 'fees' | 'people' | 'analytics' | 'admin'

// tone passes a module (or any CSS colour) to the ds-* classes through --s.
export function tone(color: Module | string): CSSProperties {
  return { '--s': color.startsWith('#') || color.startsWith('var(') ? color : `var(--mod-${color})` } as CSSProperties
}

// Module for each navigation key, so links and tabs share their module's colour.
const NAV_MODULE: Record<string, Module> = {
  dashboard: 'home', analytics: 'analytics', analytics_teacher: 'analytics', progress: 'analytics', reports: 'analytics',
  students: 'people', guardians: 'people', staff: 'people', me: 'people', children: 'people', staff_me: 'people',
  classes: 'exams', sections: 'exams', subjects: 'exams', sessions: 'routine',
  routines: 'routine', routine_me: 'routine',
  exams: 'exams', exams_me: 'exams', results: 'exams',
  diary: 'diary', diary_me: 'diary', diary_admin: 'diary',
  student_attendance: 'attendance', teacher_attendance: 'attendance', attendance_me: 'attendance', attendance_self: 'attendance',
  fees: 'fees', fees_me: 'fees',
  notices: 'notices', events: 'notices', notifications: 'notices', delivery: 'notices',
}
export const navModule = (key: string): Module => NAV_MODULE[key] ?? 'admin'

export function Panel({ title, icon, action, children, className = '', module }: { title: string; icon?: IconName; action?: ReactNode; children: ReactNode; className?: string; module?: Module }) {
  return <section className={`${ui.card} flex flex-col p-4 sm:p-5 ${className}`} aria-label={title} style={module ? tone(module) : undefined}>
    <header className="mb-4 flex items-center justify-between gap-3">
      <h2 className="flex min-w-0 items-center gap-2.5 text-[15px] font-semibold text-slate-900">
        {icon && (module
          ? <span className="ds-fill grid h-8 w-8 shrink-0 place-items-center rounded-xl"><Icon name={icon} className="h-4 w-4" /></span>
          : <Icon name={icon} className="h-[18px] w-[18px] shrink-0 text-slate-500" />)}
        <span className="min-w-0">{title}</span>
      </h2>{action}
    </header>
    <div className="min-w-0 flex-1">{children}</div>
  </section>
}

export function ViewAll({ to, label }: { to: string; label?: string }) {
  const { t } = useTranslation()
  return <Link to={to} className="inline-flex min-h-[32px] shrink-0 items-center gap-1 text-sm font-semibold text-[var(--brand-ink)] hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">{label ?? t('m12ViewAll')}<Icon name="arrowRight" className="h-3.5 w-3.5" /></Link>
}

export function Metric({ label, value, hint, icon, to, tone: kind }: { label: string; value: ReactNode; hint?: ReactNode; icon: IconName; to?: string; tone?: 'alert' | 'good' }) {
  const body = <>
    <div className="flex items-start justify-between gap-3"><p className="text-sm font-medium text-slate-600">{label}</p>
      <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${kind === 'alert' ? 'bg-rose-50 text-rose-700' : 'shell-mark'}`}><Icon name={icon} className="h-[18px] w-[18px]" /></span></div>
    <p className={`mt-2 text-2xl font-bold tabular-nums tracking-tight ${kind === 'alert' ? 'text-rose-700' : 'text-slate-900'}`}>{value}</p>
    {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
  </>
  return to ? <Link to={to} className={`${ui.card} block p-4 transition hover:-translate-y-px hover:shadow-md focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] motion-reduce:hover:translate-y-0`}>{body}</Link>
    : <div className={`${ui.card} p-4`}>{body}</div>
}

// Unavailable explains why a card has no figures instead of showing a number.
export function Unavailable({ error }: { error: unknown }) {
  const { t } = useTranslation()
  const text = error instanceof ApiError && error.code === 'module_disabled' ? t('m12ModuleOff') : error instanceof ApiError && error.status === 403 ? t('m12NoAccess') : t('m12LoadFailed')
  return <p className="flex items-center gap-2 rounded-xl bg-slate-50 px-3 py-3 text-sm text-slate-600"><Icon name="alert" className="h-4 w-4 shrink-0 text-slate-500" />{text}</p>
}

export function Lines({ rows = 3 }: { rows?: number }) {
  return <div className="space-y-2" aria-hidden="true">{Array.from({ length: rows }, (_, i) => <div key={i} className="h-10 animate-pulse rounded-lg bg-slate-100 motion-reduce:animate-none" />)}</div>
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="rounded-xl border border-dashed border-slate-200 px-4 py-6 text-center text-sm text-slate-500">{children}</p>
}
