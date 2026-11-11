import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { confirmAction } from '../components/ConfirmDialog'
import { StudentName } from '../components/StudentLink'
import { getFeeCancellations, reverseFeePayment, revertFeeCancellation, type FeeCancellation, type FeePayment, type FeeReversalKind } from '../api/fees'
import { usePeopleApi } from './peopleShared'
import { CardTitle, EmptyState, ui } from './attendanceUi'
import { Notice, Pager, feeCard, feeField, useFeeAction, useLocalName, useMoney, useMonthLabel, type FeeAccess } from './feeShared'

// Lifecycle audit QA-08: the dues cancelled when students left, with the
// administrator's revert.
export function CancellationsPanel({ access }: { access: FeeAccess }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const month = useMonthLabel()
  const action = useFeeAction()
  const [show, setShow] = useState<'open' | 'reverted' | ''>('open')
  const [page, setPage] = useState(1)
  const [reverting, setReverting] = useState<{ item: FeeCancellation; reason: string } | null>(null)
  const params = new URLSearchParams({ page: String(page), page_size: '10', ...(show ? { status: show } : {}) })
  const list = useQuery({ queryKey: ['fees', 'cancellations', params.toString()], queryFn: () => authorized(token => getFeeCancellations(token, params)), retry: false })
  const isAdmin = access.role === 'admin'

  async function revert(event: FormEvent) {
    event.preventDefault()
    if (!reverting) return
    const { item, reason } = reverting
    if (!(await confirmAction(t('afRevertConfirm', { amount: money(item.amount), name: name(item.student_name, item.student_name_bn), months: item.periods.map(month).join(', ') })))) return
    const done = await action.run(() => authorized(token => revertFeeCancellation(token, item.id, reason.trim())), t('afReverted'))
    if (done) setReverting(null)
  }

  return <section className={feeCard} aria-labelledby="fee-cancellations">
    <CardTitle id="fee-cancellations" icon="receipt">{t('afCancelTitle')}</CardTitle>
    <p className="mt-1 max-w-3xl text-sm text-slate-600">{t('afCancelHint')}</p>
    <div className="mt-3"><Notice error={action.error} message={action.message} /></div>
    <fieldset className="mt-3">
      <legend className={ui.label}>{t('afCancelShow')}</legend>
      <div className="mt-1.5 flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('afCancelShow')}>
        {(['open', 'reverted', ''] as const).map(value => <button key={value || 'all'} type="button" role="radio" aria-checked={show === value} onClick={() => { setShow(value); setPage(1) }}
          className={`min-h-11 rounded-xl border px-3 text-sm font-semibold transition ${show === value ? 'border-[var(--brand-primary)] bg-[var(--brand-primary)] text-[var(--brand-on)]' : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50'}`}>{t(`afCancelShow_${value || 'all'}`)}</button>)}
      </div>
    </fieldset>
    {list.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : list.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : list.data.items.length === 0 ? <div className="mt-3"><EmptyState icon="check" title={t('afCancelNone')} /></div> : <>
      <ul className="mt-3 space-y-3">{list.data.items.map(c => <li key={c.id} className="rounded-xl border border-slate-200 p-3 sm:p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="font-semibold text-slate-900" style={{ overflowWrap: 'anywhere' }}><StudentName id={c.student_id}>{name(c.student_name, c.student_name_bn)}</StudentName></p>
            <p className="text-xs text-slate-600">{c.admission_no} · {c.class_name} · {c.session_name} · {t(`feeStudentStatus_${c.student_status}`, { defaultValue: c.student_status })}</p>
          </div>
          <span className={`inline-block whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold ${c.reverted_at ? 'bg-slate-200 text-slate-700' : 'bg-amber-100 text-amber-900'}`}>{t(c.reverted_at ? 'afCancelShow_reverted' : 'afCancelShow_open')}</span>
        </div>
        <p className="mt-2 text-sm">{t(`afCancelReason_${c.reason}`)} · {t('afCancelLeftOn', { date: c.left_on })} · <strong className="tabular-nums">{t('afCancelCount', { count: c.dues, amount: money(c.amount) })}</strong></p>
        <p className="mt-1 text-sm"><span className="text-slate-600">{t('afCancelMonths')}:</span> {c.periods.map(month).join(', ')}</p>
        <p className="mt-1 text-xs text-slate-500">{t('afCancelRecorded', { date: c.created_at, name: c.created_by_name || t('afCancelAutomatic') })}</p>
        {c.reverted_at && <p className="mt-1 text-xs text-slate-600">{t('afRevertedOn', { date: c.reverted_at, name: c.reverted_by_name, reason: c.revert_reason })}</p>}
        {!c.reverted_at && isAdmin && reverting?.item.id !== c.id && <button type="button" className={`${ui.secondary} mt-2`} onClick={() => setReverting({ item: c, reason: '' })}>{t('afRevert')}</button>}
        {reverting?.item.id === c.id && <form onSubmit={e => void revert(e)} className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_auto]" aria-label={t('afRevertTitle')}>
          <label className={ui.label}>{t('afRevertReason')}<input required autoFocus maxLength={240} className={feeField} value={reverting.reason} onChange={e => setReverting({ ...reverting, reason: e.target.value })} /></label>
          <div className="flex items-end gap-2"><button disabled={action.busy || !reverting.reason.trim()} className={ui.primary}>{t('afRevert')}</button><button type="button" className={ui.secondary} onClick={() => setReverting(null)}>{t('feeCancel')}</button></div>
        </form>}
      </li>)}</ul>
      <Pager page={page} total={list.data.total} size={10} onPage={setPage} />
      {!isAdmin && <p className="mt-3 text-xs text-slate-500">{t('afAdminOnly')}</p>}
    </>}
  </section>
}

// Lifecycle audit QA-09: the administrator reverses a counter payment from
// its receipt.
export function ReversePaymentPanel({ payment, onDone }: { payment: FeePayment; onDone: (payment: FeePayment) => void }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const money = useMoney()
  const action = useFeeAction()
  const [kind, setKind] = useState<FeeReversalKind>('error')
  const [reason, setReason] = useState('')
  if (payment.source !== 'counter' || payment.simulated || payment.reversed_at) return null

  async function reverse(event: FormEvent) {
    event.preventDefault()
    if (!(await confirmAction(t('afReverseConfirm', { receipt: payment.receipt_no, amount: money(payment.amount) })))) return
    const done = await action.run(() => authorized(token => reverseFeePayment(token, payment.id, { kind, reason: reason.trim() })), t('afReversedDone'))
    if (done) { setReason(''); onDone(done) }
  }

  return <details className={`${feeCard} print:hidden`}>
    <summary className="cursor-pointer text-base font-bold text-slate-900">{t('afReverseTitle')}</summary>
    <p className="mt-1 max-w-3xl text-sm text-slate-600">{t('afReverseHint')}</p>
    <div className="mt-3"><Notice error={action.error} message={action.message} /></div>
    <form onSubmit={e => void reverse(e)} className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-[auto_minmax(0,1fr)_auto]">
      <fieldset>
        <legend className={ui.label}>{t('afReverseKind')}</legend>
        <div className="mt-1.5 flex flex-wrap gap-3 text-sm">
          {(['error', 'refund'] as const).map(value => <label key={value} className="flex min-h-11 items-center gap-2"><input type="radio" name="reversal-kind" checked={kind === value} onChange={() => setKind(value)} />{t(`afReverseKind_${value}`)}</label>)}
        </div>
      </fieldset>
      <label className={ui.label}>{t('afReverseReason')}<input required maxLength={240} className={feeField} value={reason} onChange={e => setReason(e.target.value)} /></label>
      <div className="flex items-end"><button disabled={action.busy || !reason.trim()} className={`${ui.dangerSolid} w-full`}>{t('afReverse')}</button></div>
    </form>
  </details>
}
