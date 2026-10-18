import { useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { searchFeeStudents, type FeeDue, type FeeReceipt, type FeeStatement, type FeeStudent } from '../api/fees'
import { usePeopleApi } from './peopleShared'

export const feeField = 'mt-1 w-full min-w-0 rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:outline-[var(--brand-primary)]'
export const feeButton = 'rounded-lg bg-[var(--brand-primary)] px-4 py-2 font-semibold text-[var(--brand-on)] disabled:opacity-50'
export const feeSecondary = 'rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 disabled:opacity-50'
export const feeCard = 'rounded-2xl border border-slate-200 bg-white p-5 shadow-sm'

export function useMoney() {
  const { i18n } = useTranslation()
  const format = new Intl.NumberFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return (value: number) => `৳ ${format.format(value)}`
}

export function useMonthLabel() {
  const { i18n } = useTranslation()
  return (period: string | null) => {
    if (!period) return '—'
    const [year, month] = period.split('-').map(Number)
    return new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(year, month - 1, 1)))
  }
}

export function useLocalName() {
  const { i18n } = useTranslation()
  return (en: string, bn?: string | null) => (i18n.language === 'bn' && bn ? bn : en)
}

// newKey returns a UUID for payment idempotency. crypto.randomUUID only exists
// in secure contexts, so plain-HTTP school networks use getRandomValues.
export function newKey(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

export function todayInDhaka(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date())
}

export function monthStart(): string {
  return `${todayInDhaka().slice(0, 7)}-01`
}

const errorKeys: Record<string, string> = {
  overpayment: 'feeErrOverpayment', duplicate_payment: 'feeErrDuplicate', fee_conflict: 'feeErrConflict', payment_declined: 'feeErrDeclined',
  payment_expired: 'feeErrExpired', forbidden: 'feeErrForbidden', not_found: 'feeErrNotFound', invalid_request: 'feeErrInvalid', module_disabled: 'feeErrModule',
}

export function useFeeError() {
  const { t } = useTranslation()
  return (cause: unknown) => t(cause instanceof ApiError && errorKeys[cause.code] ? errorKeys[cause.code] : 'feeErrGeneric')
}

export function Notice({ error, message }: { error: string; message: string }) {
  if (!error && !message) return null
  return <p role={error ? 'alert' : 'status'} className={`rounded-lg p-3 print:hidden ${error ? 'bg-rose-50 text-rose-800' : 'bg-emerald-50 text-emerald-800'}`}>{error || message}</p>
}

export function Pager({ page, total, size, onPage }: { page: number; total: number; size: number; onPage: (page: number) => void }) {
  const { t } = useTranslation()
  const pages = Math.max(1, Math.ceil(total / size))
  return <div className="mt-3 flex flex-wrap items-center gap-2 text-sm print:hidden">
    <button type="button" className={feeSecondary} disabled={page <= 1} onClick={() => onPage(page - 1)}>{t('feePrev')}</button>
    <span>{t('feePageOf', { page, pages })}</span>
    <button type="button" className={feeSecondary} disabled={page >= pages} onClick={() => onPage(page + 1)}>{t('feeNext')}</button>
  </div>
}

export function StatusBadge({ due }: { due: Pick<FeeDue, 'status' | 'overdue'> }) {
  const { t } = useTranslation()
  const tone = due.overdue ? 'bg-rose-100 text-rose-800' : due.status === 'paid' ? 'bg-emerald-100 text-emerald-800' : due.status === 'waived' ? 'bg-slate-200 text-slate-700' : due.status === 'partial' ? 'bg-amber-100 text-amber-900' : 'bg-sky-100 text-sky-900'
  return <span className={`inline-block whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold ${tone}`}>{t(`feeStatus_${due.status}`)}{due.overdue ? ` · ${t('feeOverdue')}` : ''}</span>
}

export function TableWrap({ children }: { children: ReactNode }) {
  return <div className="mt-3 overflow-x-auto"><table className="w-full min-w-[36rem] border-collapse text-left text-sm">{children}</table></div>
}

export function StudentPicker({ onPick, label }: { onPick: (student: FeeStudent) => void; label?: string }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const [search, setSearch] = useState('')
  const params = new URLSearchParams({ page: '1', page_size: '8', search })
  const students = useQuery({ queryKey: ['fees', 'students', search], enabled: search.trim().length >= 2, queryFn: () => authorized(token => searchFeeStudents(token, params)), retry: false })
  return <div>
    <label className="block text-sm font-semibold">{label ?? t('feeFindStudent')}<input className={feeField} value={search} placeholder={t('feeFindStudentHint')} onChange={e => setSearch(e.target.value)} /></label>
    {students.isError && <p role="alert" className="mt-2 text-sm text-rose-700">{t('feeErrGeneric')}</p>}
    {students.data && (students.data.items.length === 0 ? <p className="mt-2 text-sm text-slate-600">{t('feeNoStudents')}</p> :
      <ul className="mt-2 grid gap-2 sm:grid-cols-2">{students.data.items.map(s => <li key={s.id}><button type="button" className="w-full rounded-lg border p-3 text-left hover:border-[var(--brand-primary)]" onClick={() => { onPick(s); setSearch('') }}>
        <strong>{name(s.name_en, s.name_bn)}</strong><span className="block text-xs text-slate-600">{s.admission_no} · {s.class_name} / {s.section_name} · {s.session_name}</span></button></li>)}</ul>)}
  </div>
}

export function ReceiptView({ receipt, onClose }: { receipt: FeeReceipt; onClose?: () => void }) {
  const { t, i18n } = useTranslation()
  const money = useMoney()
  const month = useMonthLabel()
  const name = useLocalName()
  const p = receipt.payment
  const school = name(receipt.school_name_en, receipt.school_name_bn)
  return <article className="fee-receipt rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8" lang={i18n.language}>
    <header className="flex flex-wrap items-center gap-4 border-b pb-4">
      {receipt.logo_url && <img src={receipt.logo_url} alt="" className="h-14 w-14 object-contain" />}
      <div className="min-w-0 flex-1"><p className="text-xl font-bold">{school}</p>{(receipt.address || receipt.phone) && <p className="text-sm text-slate-600">{[receipt.address, receipt.phone].filter(Boolean).join(' · ')}</p>}<h2 className="mt-1 text-lg font-semibold">{t('feeReceiptTitle')}</h2></div>
      <div className="text-right text-sm"><p className="font-mono text-base font-bold">{p.receipt_no}</p><p>{t('feePaidOn')}: {p.paid_on}</p></div>
    </header>
    {receipt.replayed && <p className="mt-3 rounded-lg bg-amber-50 p-2 text-sm text-amber-900 print:hidden">{t('feeReplayed')}</p>}
    <dl className="mt-4 grid gap-2 text-sm sm:grid-cols-2">
      <div><dt className="text-slate-500">{t('feeStudent')}</dt><dd className="font-semibold">{name(p.student_name, p.student_name_bn)} · {p.admission_no}</dd></div>
      <div><dt className="text-slate-500">{t('feeClassSection')}</dt><dd>{p.class_name} / {p.section_name}</dd></div>
      <div><dt className="text-slate-500">{t('feeMethod')}</dt><dd>{t(`feeMethod_${p.method}`)}{p.source === 'online' ? ` · ${t('feeOnline')}` : ''}</dd></div>
      {p.reference && <div><dt className="text-slate-500">{t('feeReference')}</dt><dd className="break-all font-mono">{p.reference}</dd></div>}
    </dl>
    <TableWrap><thead><tr className="border-b bg-slate-50"><th className="p-2">{t('feeCategory')}</th><th className="p-2">{t('feePeriod')}</th><th className="p-2">{t('feeDueDate')}</th><th className="p-2 text-right">{t('feeAmount')}</th></tr></thead>
      <tbody>{receipt.allocations.map(a => <tr key={a.student_fee_id} className="border-b"><td className="p-2">{name(a.category_name_en, a.category_name_bn)}</td><td className="p-2">{month(a.period)}</td><td className="p-2">{a.due_date}</td><td className="p-2 text-right">{money(a.amount)}</td></tr>)}</tbody>
      <tfoot><tr className="font-bold"><td className="p-2" colSpan={3}>{t('feeTotalPaid')}</td><td className="p-2 text-right">{money(p.amount)}</td></tr></tfoot>
    </TableWrap>
    <div className="mt-4 flex flex-wrap justify-between gap-2 text-sm"><p>{t('feeBalanceAfter')}: <strong>{money(receipt.balance_after)}</strong></p>{p.received_by_name && <p>{t('feeReceivedBy')}: {p.received_by_name}</p>}</div>
    {p.note && <p className="mt-2 text-sm text-slate-600">{p.note}</p>}
    <div className="mt-5 flex flex-wrap gap-2 print:hidden"><button type="button" className={feeButton} onClick={() => window.print()}>{t('feePrintReceipt')}</button>{onClose && <button type="button" className={feeSecondary} onClick={onClose}>{t('feeClose')}</button>}</div>
  </article>
}

export function Totals({ totals }: { totals: FeeStatement['totals'] }) {
  const { t } = useTranslation()
  const money = useMoney()
  const items: [string, number, string][] = [['feeCharged', totals.charged, ''], ['feeDiscount', totals.discount, ''], ['feePaid', totals.paid, 'text-emerald-800'], ['feeBalance', totals.balance, 'text-slate-900'], ['feeArrears', totals.overdue, totals.overdue > 0 ? 'text-rose-700' : '']]
  return <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">{items.map(([key, value, tone]) => <div key={key} className="rounded-xl bg-slate-50 p-3"><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t(key)}</dt><dd className={`mt-1 text-lg font-bold ${tone}`}>{money(value)}</dd></div>)}</dl>
}

export function DuesTable({ dues, selectable, selected, onToggle, actions }: { dues: FeeDue[]; selectable?: boolean; selected?: string[]; onToggle?: (id: string) => void; actions?: (due: FeeDue) => ReactNode }) {
  const { t } = useTranslation()
  const money = useMoney()
  const month = useMonthLabel()
  const name = useLocalName()
  if (dues.length === 0) return <p className="mt-3 text-slate-600">{t('feeNoDues')}</p>
  return <TableWrap><thead><tr className="border-b bg-slate-50">{selectable && <th className="p-2"><span className="sr-only">{t('feeSelect')}</span></th>}<th className="p-2">{t('feeCategory')}</th><th className="p-2">{t('feePeriod')}</th><th className="p-2">{t('feeDueDate')}</th><th className="p-2 text-right">{t('feeAmount')}</th><th className="p-2 text-right">{t('feeDiscount')}</th><th className="p-2 text-right">{t('feePaid')}</th><th className="p-2 text-right">{t('feeBalance')}</th><th className="p-2">{t('feeStatus')}</th>{actions && <th className="p-2 print:hidden">{t('feeActions')}</th>}</tr></thead>
    <tbody>{dues.map(d => <tr key={d.id} className="border-b align-top">
      {selectable && <td className="p-2">{d.balance > 0 && <input type="checkbox" aria-label={`${t('feeSelect')} ${name(d.category_name_en, d.category_name_bn)} ${d.due_date}`} checked={selected?.includes(d.id) ?? false} onChange={() => onToggle?.(d.id)} />}</td>}
      <td className="p-2">{name(d.category_name_en, d.category_name_bn)}</td><td className="p-2">{month(d.period)}</td><td className="p-2 whitespace-nowrap">{d.due_date}</td>
      <td className="p-2 text-right">{money(d.amount)}</td><td className="p-2 text-right">{money(d.discount_amount)}</td><td className="p-2 text-right">{money(d.paid_amount)}</td><td className="p-2 text-right font-semibold">{money(d.balance)}</td>
      <td className="p-2"><StatusBadge due={d} /></td>{actions && <td className="p-2 print:hidden">{actions(d)}</td>}</tr>)}</tbody>
  </TableWrap>
}

export function PaymentsList({ payments, onOpen }: { payments: FeeStatement['payments']; onOpen: (id: string) => void }) {
  const { t } = useTranslation()
  const money = useMoney()
  if (payments.length === 0) return <p className="mt-3 text-slate-600">{t('feeNoPayments')}</p>
  return <TableWrap><thead><tr className="border-b bg-slate-50"><th className="p-2">{t('feeReceiptNo')}</th><th className="p-2">{t('feePaidOn')}</th><th className="p-2">{t('feeMethod')}</th><th className="p-2 text-right">{t('feeAmount')}</th><th className="p-2 print:hidden"><span className="sr-only">{t('feeActions')}</span></th></tr></thead>
    <tbody>{payments.map(p => <tr key={p.id} className="border-b"><td className="p-2 font-mono">{p.receipt_no}</td><td className="p-2">{p.paid_on}</td><td className="p-2">{t(`feeMethod_${p.method}`)}</td><td className="p-2 text-right">{money(p.amount)}</td>
      <td className="p-2 print:hidden"><button type="button" className="font-semibold text-[var(--brand-ink)] underline" onClick={() => onOpen(p.id)}>{t('feeViewReceipt')}</button></td></tr>)}</tbody>
  </TableWrap>
}

export type FeeAccess = { role: string; canManage: boolean; canCollect: boolean; canReadAccounts: boolean; canManageAccounts: boolean }

export function useFeeAction() {
  const client = useQueryClient()
  const describe = useFeeError()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  async function run<T>(task: () => Promise<T>, success: string | ((value: T) => string)): Promise<T | undefined> {
    setBusy(true); setError(''); setMessage('')
    try {
      const value = await task()
      await client.invalidateQueries({ queryKey: ['fees'] })
      setMessage(typeof success === 'function' ? success(value) : success)
      return value
    } catch (cause) {
      setError(describe(cause))
      return undefined
    } finally { setBusy(false) }
  }
  return { busy, error, message, run, clear: () => { setError(''); setMessage('') } }
}

