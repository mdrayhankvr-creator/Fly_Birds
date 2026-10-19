import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Icon, type IconName } from './icons'
import { Skeleton, ui } from '../pages/attendanceUi'

export type StateKind = 'loading' | 'denied' | 'disabled' | 'error' | 'notFound'

const look: Record<Exclude<StateKind, 'loading'>, { icon: IconName; tint: string; title: string; hint: string }> = {
  denied: { icon: 'lock', tint: 'bg-slate-100 text-slate-600', title: 'uxDeniedTitle', hint: 'uxDeniedHint' },
  disabled: { icon: 'layers', tint: 'bg-amber-50 text-amber-800', title: 'uxDisabledTitle', hint: 'uxDisabledHint' },
  error: { icon: 'alert', tint: 'bg-rose-50 text-rose-700', title: 'uxErrorTitle', hint: 'uxErrorHint' },
  notFound: { icon: 'search', tint: 'bg-[var(--brand-soft)] text-[var(--brand-ink)]', title: 'notFound', hint: 'uxNotFoundBody' },
}

// StateScreen is the one full-page state used by every route guard: loading,
// no permission, module switched off, load failure and page not found. The
// message names the specific area; the title and hint explain what to do.
export function StateScreen({ kind, message, action }: { kind: StateKind; message?: string; action?: ReactNode }) {
  const { t } = useTranslation()
  if (kind === 'loading') return <div role="status" aria-live="polite" className="space-y-4">
    <span className="sr-only">{message ?? t('uxLoading')}</span>
    <div aria-hidden="true" className="h-9 w-64 max-w-full animate-pulse rounded-lg bg-slate-100" />
    <Skeleton rows={4} />
  </div>
  const l = look[kind]
  const defaultAction = kind === 'error'
    ? <button type="button" className={ui.primary} onClick={() => window.location.reload()}>{t('uxRetry')}</button>
    : kind === 'notFound' ? <Link to="/" className={ui.primary}>{t('backHome')}</Link>
      : <Link to="/dashboard" className={ui.secondary}><Icon name="home" className="h-4 w-4" />{t('uxGoDashboard')}</Link>
  return <section role={kind === 'error' || kind === 'denied' ? 'alert' : 'status'} className={`${ui.card} mx-auto max-w-xl px-6 py-10 text-center sm:px-10`}>
    <span aria-hidden="true" className={`mx-auto grid h-14 w-14 place-items-center rounded-2xl ${l.tint}`}><Icon name={l.icon} className="h-7 w-7" /></span>
    <h1 className="mt-5 text-xl font-bold tracking-tight text-slate-900 sm:text-2xl">{t(l.title)}</h1>
    {message && message !== t(l.title) && <p className="mt-2 text-sm font-medium text-slate-700">{message}</p>}
    <p className="mt-2 text-sm leading-6 text-slate-600">{t(l.hint)}</p>
    <div className="mt-6 flex flex-wrap justify-center gap-2">{action ?? defaultAction}</div>
  </section>
}
