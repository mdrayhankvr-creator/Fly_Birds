import type { ReactNode } from 'react'
import { tone, type Module } from '../modules'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError } from '../../api/client'
import { ui } from '../../pages/attendanceUi'
import { Icon, type IconName } from '../icons'

// Shared building blocks for dashboards and module screens. Colours come from
// the design tokens in styles.css, so every part follows the school's brand
// and the light or dark theme.

export { navModule, tone, type Module } from '../modules'

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

// ---------- dashboard layout and headers ----------

// DashLayout: header, quick actions, then a main column and a side column on
// desktop; on tablets the side column becomes a two-up grid.
export function DashLayout({ head, actions, main, side }: { head: ReactNode; actions?: ReactNode; main: ReactNode; side?: ReactNode }) {
  return <div className="space-y-4 md:space-y-5">
    {head}
    {actions}
    <div className="grid grid-cols-1 gap-4 md:gap-5 lg:grid-cols-12">
      <div className={`min-w-0 space-y-4 md:space-y-5 ${side ? 'lg:col-span-8' : 'lg:col-span-12'}`}>{main}</div>
      {side && <div className="grid min-w-0 content-start items-start gap-4 md:grid-cols-2 md:gap-5 lg:col-span-4 lg:grid-cols-1">{side}</div>}
    </div>
  </div>
}

export type Stat = { label: string; value: ReactNode; hint?: ReactNode; icon: IconName; color?: Module | string; alert?: boolean; to?: string }

export function StatTile({ stat }: { stat: Stat }) {
  const body = <>
    <p className="flex items-center gap-1.5"><Icon name={stat.icon} className={`h-4 w-4 shrink-0 ${stat.alert ? 'text-rose-700' : 'ds-ink'}`} />
      <span className={`truncate text-lg font-extrabold tabular-nums tracking-tight sm:text-xl ${stat.alert ? 'text-rose-700' : 'text-slate-900'}`}>{stat.value}</span></p>
    <p className="mt-0.5 line-clamp-2 text-xs font-medium leading-tight text-slate-600">{stat.label}</p>
    {stat.hint && <p className="mt-1 line-clamp-2 text-[11px] leading-tight text-slate-500">{stat.hint}</p>}
  </>
  const cls = 'ds-stat block min-w-0 px-3 py-2.5 sm:px-3.5'
  return stat.to
    ? <Link to={stat.to} className={`${cls} transition hover:-translate-y-px focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] motion-reduce:hover:translate-y-0`} style={tone(stat.color ?? 'home')}>{body}</Link>
    : <div className={cls} style={tone(stat.color ?? 'home')}>{body}</div>
}

// RoleHead: a compact header tinted with the role's accent colour.
export function RoleHead({ accent, eyebrow, title, sub, lead, top, stats, actions }: {
  accent: string; eyebrow: ReactNode; title: ReactNode; sub?: ReactNode; lead?: ReactNode; top?: ReactNode; stats?: Stat[]; actions?: ReactNode
}) {
  return <section className="ds-role-head ds-rise p-4 sm:p-5" style={tone(accent)}>
    {top}
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="flex min-w-0 items-start gap-3">
        {lead}
        <div className="min-w-0">
          <p className="text-xs font-semibold text-slate-600">{eyebrow}</p>
          <h1 className="mt-0.5 text-[22px] font-extrabold leading-tight tracking-tight text-slate-900 sm:text-[26px]">{title}</h1>
          {sub && <p className="mt-0.5 text-sm leading-snug text-slate-600">{sub}</p>}
        </div>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </div>
    {stats && stats.length > 0 && <div className={`mt-4 grid grid-cols-2 gap-2 sm:gap-2.5 ${stats.length >= 4 ? 'lg:grid-cols-4' : 'sm:grid-cols-3'}`}>
      {stats.map(s => <StatTile key={s.label} stat={s} />)}
    </div>}
  </section>
}

export function Avatar({ name, className = 'h-11 w-11 text-sm' }: { name: string; className?: string }) {
  const letters = name.split(/[\s@.]+/).filter(Boolean).slice(0, 2).map(part => [...part][0]).join('').toUpperCase()
  return <span aria-hidden="true" className={`ds-fill grid shrink-0 place-items-center rounded-2xl font-bold ${className}`}>{letters}</span>
}

export type QuickAction = { key: string; to: string; icon: IconName; label: string; module: Module }
const COLS = ['grid-cols-1', 'grid-cols-2', 'grid-cols-3', 'grid-cols-4', 'grid-cols-3 sm:grid-cols-5']

// QuickActions: up to five tinted shortcuts. Callers pass only links the user may open.
export function QuickActions({ items, label }: { items: QuickAction[]; label: string }) {
  if (!items.length) return null
  const list = items.slice(0, 5)
  return <nav aria-label={label}>
    <ul className={`grid gap-2 sm:gap-3 ${COLS[list.length - 1]}`}>
      {list.map(it => <li key={it.key} className="min-w-0">
        <Link to={it.to} style={tone(it.module)} className="ds-tint flex h-full min-h-[76px] min-w-0 flex-col items-center justify-center gap-1.5 px-1 py-2.5 text-center transition hover:-translate-y-px focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] motion-reduce:hover:translate-y-0 sm:flex-row sm:justify-start sm:gap-3 sm:px-3.5 sm:text-left">
          <span className="ds-fill grid h-9 w-9 shrink-0 place-items-center rounded-xl sm:h-10 sm:w-10"><Icon name={it.icon} className="h-[18px] w-[18px]" /></span>
          <span className="line-clamp-2 text-[11.5px] font-bold leading-tight text-slate-900 sm:text-sm">{it.label}</span>
        </Link>
      </li>)}
    </ul>
  </nav>
}

// Ring shows a share from 0 to 100; the figure itself is printed in the middle.
export function Ring({ value, size = 76, stroke = 8, color, children }: { value: number; size?: number; stroke?: number; color: string; children?: ReactNode }) {
  const r = (size - stroke) / 2, c = 2 * Math.PI * r
  const v = Math.max(0, Math.min(100, value))
  return <span className="relative inline-grid shrink-0 place-items-center" style={{ width: size, height: size }}>
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden="true" className="-rotate-90">
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--ds-surface-2)" strokeWidth={stroke} />
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round" strokeDasharray={`${(v / 100) * c} ${c}`} />
    </svg>
    <span className="absolute inset-0 grid place-items-center text-center">{children}</span>
  </span>
}
