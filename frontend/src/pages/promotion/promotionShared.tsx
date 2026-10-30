import { useEffect, useId, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../api/client'
import type { BatchStatus, Decision, PromotionBatch, PromotionIssue, Recommendation } from '../../api/promotions'
import { useAuth } from '../../auth/AuthProvider'
import { Icon } from '../../components/icons'
import { LinkTabs, PageHeader } from '../attendanceUi'

// Shared parts of the year-end promotion screens (M15 stage B2).

export type Caps = { manage: boolean; approve: boolean; apply: boolean; fees: boolean }

// useAuthorized runs a request with the access token, renewing it once on 401.
export function useAuthorized() {
  const { accessToken, renew, expire } = useAuth()
  return async function authorized<T>(request: (token: string) => Promise<T>): Promise<T> {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await request(accessToken) }
    catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await request(await renew()) }
        catch (retry) { if (retry instanceof ApiError && retry.status === 401) expire(); throw retry }
      }
      throw cause
    }
  }
}

const ERRORS: Record<string, string> = {
  progression_unset: 'pmErr_progression_unset', batch_open: 'pmErr_batch_open', batch_state: 'pmErr_batch_state',
  batch_blocked: 'pmErr_batch_blocked', inputs_changed: 'pmErr_inputs_changed', revert_refused: 'pmErr_revert_refused',
  promotion_conflict: 'pmErr_promotion_conflict', reason_required: 'pmErr_reason_required', invalid_request: 'pmErr_invalid',
  forbidden: 'pmErr_forbidden', module_disabled: 'pmErr_module_disabled', not_found: 'pmErr_not_found',
}
export function useErrorText() {
  const { t } = useTranslation()
  return (cause: unknown) => t(cause instanceof ApiError && ERRORS[cause.code] ? ERRORS[cause.code] : cause instanceof ApiError && cause.status === 403 ? 'pmErr_forbidden' : 'pmErr_generic')
}

export function useDateTime() {
  const { i18n } = useTranslation()
  const f = new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' })
  return (iso: string | null) => iso ? f.format(new Date(iso)) : ''
}

export function PromotionFrame({ caps, title, description, actions, children }: { caps: Caps; title: string; description?: string; actions?: ReactNode; children: ReactNode }) {
  const { t } = useTranslation()
  const tabs = [
    { to: '/admin/promotions', label: t('pmTabBatches') },
    { to: '/admin/promotions/progression', label: t('pmTabProgression') },
    ...(caps.fees ? [{ to: '/admin/promotions/concessions', label: t('pmTabConcessions') }] : []),
  ]
  return <div className="space-y-6">
    <PageHeader eyebrow={t('pmEyebrow')} title={title} description={description} actions={actions} />
    <LinkTabs label={t('pmEyebrow')} items={tabs} />
    {children}
  </div>
}

const STATUS_STYLE: Record<BatchStatus, string> = {
  draft: 'bg-slate-100 text-slate-700 ring-slate-500/20',
  approved: 'bg-sky-50 text-sky-800 ring-sky-600/25',
  applied: 'bg-emerald-50 text-emerald-800 ring-emerald-600/25',
  reverted: 'bg-amber-50 text-amber-900 ring-amber-600/25',
  discarded: 'bg-slate-100 text-slate-600 ring-slate-500/20',
}
export function StatusPill({ status }: { status: BatchStatus }) {
  const { t } = useTranslation()
  return <span className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-semibold ring-1 ring-inset ${STATUS_STYLE[status]}`}>
    <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />{t(`pmStatus_${status}`)}</span>
}

const DECISION_STYLE: Record<Recommendation, string> = {
  promote: 'bg-emerald-50 text-emerald-800 ring-emerald-600/25',
  repeat: 'bg-amber-50 text-amber-900 ring-amber-600/25',
  graduate: 'bg-violet-50 text-violet-900 ring-violet-600/25',
  withdraw: 'bg-rose-50 text-rose-800 ring-rose-600/25',
  transfer: 'bg-sky-50 text-sky-800 ring-sky-600/25',
  blocked: 'bg-slate-100 text-slate-700 ring-slate-500/20',
}
export function DecisionPill({ value }: { value: Recommendation }) {
  const { t } = useTranslation()
  return <span className={`inline-flex whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${DECISION_STYLE[value]}`}>{t(`pmDecision_${value}`)}</span>
}

export function OutcomePill({ outcome, position }: { outcome: string | null; position: number | null }) {
  const { t } = useTranslation()
  const num = useNum()
  if (!outcome) return <span className="text-xs font-medium text-slate-500">{t('pmNoAnnual')}</span>
  const tone = outcome === 'passed' ? 'text-emerald-700' : outcome === 'failed' ? 'text-rose-700' : 'text-amber-700'
  const known = ['passed', 'failed', 'withheld'].includes(outcome)
  return <span className="text-sm"><span className={`font-semibold ${tone}`}>{known ? t(`pmOutcome_${outcome}`) : outcome}</span>
    {position !== null && <span className="text-slate-500"> · {t('pmPosition', { n: num(position) })}</span>}</span>
}

export function useNum() {
  const { i18n } = useTranslation()
  const nf = new Intl.NumberFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB')
  return (n: number) => nf.format(n)
}

export const DECISIONS: Decision[] = ['promote', 'repeat', 'graduate', 'withdraw', 'transfer']

// issueText explains one blocker or warning, naming the student when known.
export function useIssueText(batch: PromotionBatch) {
  const { t } = useTranslation()
  const names = new Map(batch.items.map(i => [i.student_id, i.student_name]))
  return (issue: PromotionIssue) => {
    const name = issue.student_id ? names.get(issue.student_id) ?? t('pmUnknownStudent') : ''
    const other = issue.detail && names.get(issue.detail)
    const key = `pmIssue_${issue.code}`
    const text = t(key, { name, other: other || '', detail: issue.detail || '', defaultValue: '' })
    return text || t('pmIssue_unknown', { code: issue.code, name })
  }
}

// Dialog: a modal with a title, focus kept inside, Escape and the backdrop close it.
export function Dialog({ title, onClose, children, footer, wide }: { title: string; onClose: () => void; children: ReactNode; footer: ReactNode; wide?: boolean }) {
  const { t } = useTranslation()
  const titleId = useId()
  const box = useRef<HTMLDivElement>(null)
  const back = useRef<Element | null>(null)
  useEffect(() => {
    back.current = document.activeElement
    const first = box.current?.querySelector<HTMLElement>('input, select, textarea, button:not([data-close])')
    first?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.preventDefault(); onClose() }
      if (e.key === 'Tab' && box.current) {
        const items = [...box.current.querySelectorAll<HTMLElement>('input, select, textarea, button, a[href]')].filter(el => !el.hasAttribute('disabled'))
        if (!items.length) return
        const i = items.indexOf(document.activeElement as HTMLElement)
        if (e.shiftKey && i <= 0) { e.preventDefault(); items[items.length - 1].focus() }
        else if (!e.shiftKey && i === items.length - 1) { e.preventDefault(); items[0].focus() }
      }
    }
    document.addEventListener('keydown', onKey)
    return () => { document.removeEventListener('keydown', onKey); (back.current as HTMLElement | null)?.focus?.() }
  }, [onClose])
  return <div className="fixed inset-0 z-50 flex items-end justify-center sm:items-center sm:p-4">
    <button type="button" tabIndex={-1} aria-label={t('pmClose')} className="absolute inset-0 bg-slate-900/50" onClick={onClose} />
    <div ref={box} role="dialog" aria-modal="true" aria-labelledby={titleId}
      className={`relative flex max-h-[92vh] w-full flex-col rounded-t-2xl border border-slate-200 bg-white shadow-xl sm:rounded-2xl ${wide ? 'sm:max-w-2xl' : 'sm:max-w-lg'}`}>
      <header className="flex items-start justify-between gap-3 border-b border-slate-200 px-5 py-4">
        <h2 id={titleId} className="min-w-0 text-base font-bold text-slate-900">{title}</h2>
        <button type="button" data-close onClick={onClose} aria-label={t('pmClose')} className="grid h-9 w-9 shrink-0 place-items-center rounded-lg text-slate-500 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]"><Icon name="close" className="h-4 w-4" /></button>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
      <footer className="flex flex-col-reverse gap-2 border-t border-slate-200 px-5 py-4 sm:flex-row sm:justify-end">{footer}</footer>
    </div>
  </div>
}
