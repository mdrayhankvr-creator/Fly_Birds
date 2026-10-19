import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { NavLink } from 'react-router'
import { Icon, type IconName } from '../components/icons'

// Shared attendance styling. Brand colour comes from the school's white-label
// theme via --brand-primary; status colours are fixed so they stay meaningful.
export const ui = {
  card: 'min-w-0 rounded-2xl border border-slate-200/80 bg-white shadow-[0_1px_2px_rgba(15,23,42,0.04),0_8px_24px_-12px_rgba(15,23,42,0.12)]',
  field: 'mt-1.5 block w-full min-w-0 rounded-xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 shadow-sm transition focus:border-[var(--brand-primary)] focus:outline-none focus:ring-4 focus:ring-[color-mix(in_srgb,var(--brand-primary)_18%,transparent)] disabled:bg-slate-50 disabled:text-slate-500',
  label: 'block text-xs font-semibold uppercase tracking-wide text-slate-500',
  primary: 'inline-flex items-center justify-center gap-2 rounded-xl bg-[var(--brand-primary)] px-4 py-2.5 text-sm font-semibold text-[var(--brand-on)] shadow-sm transition hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-not-allowed disabled:opacity-50',
  secondary: 'inline-flex items-center justify-center gap-2 rounded-xl border border-slate-300 bg-white px-3.5 py-2.5 text-sm font-semibold text-slate-700 shadow-sm transition hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-not-allowed disabled:opacity-50',
  ghost: 'inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-sm font-semibold text-[var(--brand-ink)] hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]',
  danger: 'inline-flex items-center justify-center gap-2 rounded-xl border border-rose-200 bg-white px-3.5 py-2.5 text-sm font-semibold text-rose-700 shadow-sm transition hover:bg-rose-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-rose-600 disabled:cursor-not-allowed disabled:opacity-50',
  link: 'font-semibold text-[var(--brand-ink)] underline-offset-4 hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)]',
  sectionTitle: 'text-base font-bold tracking-tight text-slate-900',
  table: 'w-full text-left text-sm',
  th: 'whitespace-nowrap border-b border-slate-200 bg-slate-50 px-3 py-2.5 text-xs font-semibold uppercase tracking-wide text-slate-500 first:rounded-l-lg last:rounded-r-lg',
  td: 'border-b border-slate-100 px-3 py-3 align-middle',
}

export type Tone = 'present' | 'absent' | 'late' | 'leave' | 'unmarked'
export const tones: Record<Tone, { solid: string; soft: string; dot: string; text: string }> = {
  present: { solid: 'bg-emerald-600 text-white border-emerald-600', soft: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20', dot: 'bg-emerald-500', text: 'text-emerald-700' },
  absent: { solid: 'bg-rose-600 text-white border-rose-600', soft: 'bg-rose-50 text-rose-800 ring-rose-600/20', dot: 'bg-rose-500', text: 'text-rose-700' },
  late: { solid: 'bg-amber-500 text-white border-amber-500', soft: 'bg-amber-50 text-amber-900 ring-amber-600/25', dot: 'bg-amber-400', text: 'text-amber-700' },
  leave: { solid: 'bg-sky-600 text-white border-sky-600', soft: 'bg-sky-50 text-sky-800 ring-sky-600/20', dot: 'bg-sky-500', text: 'text-sky-700' },
  unmarked: { solid: 'bg-slate-600 text-white border-slate-600', soft: 'bg-slate-100 text-slate-700 ring-slate-500/20', dot: 'bg-slate-300', text: 'text-slate-600' },
}

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description?: string; actions?: ReactNode }) {
  return <header className="flex flex-wrap items-end justify-between gap-4 print:hidden">
    <div className="min-w-0">{eyebrow && <p className="text-xs font-bold uppercase tracking-[0.14em] text-[var(--brand-ink)]">{eyebrow}</p>}
      <h1 className="mt-1 text-2xl font-bold tracking-tight text-slate-900 sm:text-3xl">{title}</h1>
      {description && <p className="mt-1.5 max-w-2xl text-sm text-slate-600 sm:text-base">{description}</p>}</div>
    {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
  </header>
}

export function StatTile({ tone, label, value, hint }: { tone: Tone; label: string; value: ReactNode; hint?: ReactNode }) {
  return <div className={`rounded-xl px-3.5 py-3 ring-1 ring-inset ${tones[tone].soft}`}>
    <dt className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide"><span aria-hidden="true" className={`h-2 w-2 rounded-full ${tones[tone].dot}`} />{label}</dt>
    <dd className="mt-1 text-2xl font-bold tabular-nums">{value}</dd>{hint && <dd className="text-xs opacity-80">{hint}</dd>}
  </div>
}

export function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${tones[tone].soft}`}><span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${tones[tone].dot}`} />{children}</span>
}

export type SegmentOption<T extends string> = { value: T; label: string; short: string; tone: Tone; hotkey: string }

// SegmentedStatus is a radio group: Left/Right move the choice, and the
// option hotkeys (P, A, L…) select directly while the group has focus.
export function SegmentedStatus<T extends string>({ name, value, options, onChange, disabled, compact }: { name: string; value: T | ''; options: SegmentOption<T>[]; onChange: (value: T) => void; disabled?: boolean; compact?: boolean }) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])
  const index = options.findIndex(o => o.value === value)
  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (disabled) return
    const step = event.key === 'ArrowRight' || event.key === 'ArrowDown' ? 1 : event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 0
    if (step) {
      event.preventDefault()
      const next = (Math.max(index, step > 0 ? -1 : 0) + step + options.length) % options.length
      onChange(options[next].value); refs.current[next]?.focus()
      return
    }
    const hit = options.findIndex(o => o.hotkey === event.key.toLowerCase())
    if (hit >= 0 && !event.ctrlKey && !event.metaKey && !event.altKey) { event.preventDefault(); onChange(options[hit].value); refs.current[hit]?.focus() }
  }
  return <div role="radiogroup" aria-label={name} onKeyDown={onKeyDown} className="inline-flex w-full rounded-xl border border-slate-200 bg-slate-50 p-1 sm:w-auto">
    {options.map((o, i) => {
      const selected = o.value === value
      return <button key={o.value} ref={el => { refs.current[i] = el }} type="button" role="radio" aria-checked={selected} aria-label={o.label} title={`${o.label} (${o.hotkey.toUpperCase()})`} disabled={disabled}
        tabIndex={selected || (index < 0 && i === 0) ? 0 : -1} onClick={() => onChange(o.value)}
        className={`flex min-h-10 flex-1 items-center justify-center gap-1.5 rounded-lg border px-3 text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--brand-primary)] disabled:cursor-not-allowed sm:flex-none ${selected ? `${tones[o.tone].solid} shadow-sm` : 'border-transparent text-slate-600 hover:bg-white hover:text-slate-900'}`}>
        {selected && <svg aria-hidden="true" viewBox="0 0 16 16" className="h-3.5 w-3.5 fill-current"><path d="M6.2 11.6 2.8 8.2l1.1-1.1 2.3 2.3 5.9-5.9 1.1 1.1z" /></svg>}
        <span className={compact ? 'sm:hidden' : 'hidden'}>{o.short}</span><span className={compact ? 'hidden sm:inline' : ''}>{o.label}</span>
      </button>
    })}
  </div>
}

export function Tabs<T extends string>({ label, value, items, onChange }: { label: string; value: T; items: { value: T; label: string; badge?: ReactNode }[]; onChange: (value: T) => void }) {
  return <div role="tablist" aria-label={label} className="flex gap-1 overflow-x-auto rounded-xl bg-slate-100 p-1 print:hidden">
    {items.map(item => <button key={item.value} role="tab" type="button" aria-selected={value === item.value} onClick={() => onChange(item.value)}
      className={`flex shrink-0 items-center gap-2 rounded-lg px-3.5 py-2 text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${value === item.value ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'}`}>{item.label}{item.badge}</button>)}
  </div>
}

// LinkTabs is Tabs for sibling pages: each item is a route, and the current
// page is marked for assistive technology.
export function LinkTabs({ label, items }: { label: string; items: { to: string; label: string }[] }) {
  return <nav aria-label={label} className="flex gap-1 overflow-x-auto rounded-xl bg-slate-100 p-1 print:hidden">
    {items.map(item => <NavLink key={item.to} to={item.to} end className={({ isActive }) => `flex shrink-0 items-center rounded-lg px-3.5 py-2 text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${isActive ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'}`}>{item.label}</NavLink>)}
  </nav>
}

export function Notice({ tone, children, action }: { tone: 'success' | 'error' | 'info' | 'warning'; children: ReactNode; action?: ReactNode }) {
  const style = { success: 'border-emerald-200 bg-emerald-50 text-emerald-900', error: 'border-rose-200 bg-rose-50 text-rose-900', info: 'border-sky-200 bg-sky-50 text-sky-900', warning: 'border-amber-200 bg-amber-50 text-amber-900' }[tone]
  return <div role={tone === 'error' || tone === 'warning' ? 'alert' : 'status'} className={`flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 text-sm ${style}`}><div className="min-w-0">{children}</div>{action}</div>
}

export function EmptyState({ title, children, icon = 'search', action }: { title: string; children?: ReactNode; icon?: IconName; action?: ReactNode }) {
  return <div className="flex flex-col items-center rounded-xl border border-dashed border-slate-300 bg-slate-50 px-6 py-10 text-center">
    <span aria-hidden="true" className="grid h-11 w-11 place-items-center rounded-xl bg-white text-slate-400 shadow-sm ring-1 ring-slate-200"><Icon name={icon} className="h-5 w-5" /></span>
    <p className="mt-3 font-semibold text-slate-800">{title}</p>{children && <p className="mt-1 max-w-md text-sm leading-6 text-slate-600">{children}</p>}
    {action && <div className="mt-4">{action}</div>}
  </div>
}

// SectionCard is the standard titled panel: heading, optional description
// and actions on the right, then the body.
export function SectionCard({ title, description, actions, children, id, className = '' }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; children: ReactNode; id?: string; className?: string }) {
  return <section className={`${ui.card} p-4 sm:p-6 ${className}`} aria-labelledby={id}>
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0"><h2 id={id} className={ui.sectionTitle}>{title}</h2>{description && <p className="mt-1 text-sm leading-6 text-slate-600">{description}</p>}</div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
    <div className="mt-4">{children}</div>
  </section>
}

export function InfoRow({ label, children }: { label: ReactNode; children: ReactNode }) {
  return <div className="grid gap-0.5 py-2.5 sm:grid-cols-[minmax(0,11rem)_minmax(0,1fr)] sm:gap-4"><dt className="text-sm text-slate-500">{label}</dt><dd className="min-w-0 break-words text-sm font-medium text-slate-900">{children}</dd></div>
}

export function Skeleton({ rows = 5 }: { rows?: number }) {
  return <div className="space-y-2" aria-hidden="true">{Array.from({ length: rows }, (_, i) => <div key={i} className="h-14 animate-pulse rounded-xl bg-slate-100" />)}</div>
}

export function Pager({ page, total, size, onPage, previous, next, label }: { page: number; total: number; size: number; onPage: (page: number) => void; previous: string; next: string; label: string }) {
  const { t, i18n } = useTranslation()
  const pages = Math.max(1, Math.ceil(total / size))
  if (pages <= 1) return null
  const n = new Intl.NumberFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB')
  return <nav aria-label={label} className="mt-4 flex items-center justify-between gap-3 border-t border-slate-100 pt-4 text-sm">
    <button type="button" className={ui.secondary} disabled={page <= 1} onClick={() => onPage(page - 1)}><Icon name="chevronLeft" className="h-4 w-4" />{previous}</button>
    <span className="tabular-nums text-slate-600" aria-live="polite">{t('uxPageOf', { page: n.format(page), pages: n.format(pages) })}</span>
    <button type="button" className={ui.secondary} disabled={page >= pages} onClick={() => onPage(page + 1)}>{next}<Icon name="chevronRight" className="h-4 w-4" /></button>
  </nav>
}

// useNumber formats counts and rolls with the reader's digits (Bangla or Latin).
export function useNumber() {
  const { i18n } = useTranslation()
  const nf = new Intl.NumberFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB')
  return (n: number) => nf.format(n)
}

export function Avatar({ name }: { name: string }) {
  const initials = name.split(/\s+/).filter(Boolean).slice(0, 2).map(part => [...part][0]).join('').toUpperCase()
  return <span aria-hidden="true" className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[color-mix(in_srgb,var(--brand-primary)_12%,white)] text-sm font-bold text-[var(--brand-ink)]">{initials}</span>
}
