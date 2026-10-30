import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getFeePayments, getOnlinePaymentStatus, saveOnlinePaymentSettings, voidSimulatedPayment } from '../api/fees'
import { confirmAction } from '../components/ConfirmDialog'
import { usePeopleApi } from './peopleShared'
import { Notice, SimulatedLabel, TableWrap, feeButton, feeCard, feeField, feeSecondary, useFeeAction, useLocalName, useMoney } from './feeShared'
import { CardTitle, ui } from './attendanceUi'

// OnlinePaymentsPanel is the administrator's view of online payment: whether
// simulation (test mode) is on, and the simulated payments recorded before
// the payment-safety upgrade that still reduce dues until voided.
export function OnlinePaymentsPanel() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const money = useMoney()
  const name = useLocalName()
  const action = useFeeAction()
  const [reason, setReason] = useState('')
  const status = useQuery({ queryKey: ['fees', 'online-status'], queryFn: () => authorized(token => getOnlinePaymentStatus(token)), retry: false })
  const review = useQuery({ queryKey: ['fees', 'payments', 'simulated-unreviewed'], queryFn: () => authorized(token => getFeePayments(token, new URLSearchParams({ simulated: 'unreviewed', page: '1', page_size: '100' }))), retry: false })

  async function setSimulation(enabled: boolean) {
    if (enabled && !(await confirmAction(t('feeConfirmEnableSimulation')))) return
    await action.run(() => authorized(token => saveOnlinePaymentSettings(token, enabled)), t('feeSaved'))
  }
  async function voidPayment(id: string, receipt: string, amount: number) {
    if (!(await confirmAction(t('feeConfirmVoid', { receipt, amount: money(amount) })))) return
    const done = await action.run(() => authorized(token => voidSimulatedPayment(token, id, reason.trim())), t('feeVoided'))
    if (done) setReason('')
  }

  const s = status.data
  return <section className={feeCard} aria-labelledby="online-payments-heading">
    <CardTitle id="online-payments-heading" icon="globe">{t('feeOnlinePayments')}</CardTitle>
    <div className="mt-3"><Notice error={action.error} message={action.message} /></div>
    {status.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : !s ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : <>
      {s.mode === 'simulation'
        ? <p className="mt-3 flex flex-wrap items-center gap-2 text-sm"><SimulatedLabel />{t('feeOnlineSimulation')}</p>
        : <p className="mt-3 text-sm">{t('feeOnlineOff')}</p>}
      {s.simulation_allowed
        ? <div className="mt-3">{s.simulation_enabled
          ? <button type="button" className={feeSecondary} disabled={action.busy} onClick={() => void setSimulation(false)}>{t('feeDisableSimulation')}</button>
          : <button type="button" className={feeButton} disabled={action.busy} onClick={() => void setSimulation(true)}>{t('feeEnableSimulation')}</button>}</div>
        : <p className="mt-2 text-sm text-slate-600">{t('feeSimulationNotAllowed')}</p>}
    </>}
    <h3 className="mt-6 font-bold">{t('feeSimulatedToReview')}</h3>
    <p className="mt-1 text-sm text-slate-600">{t('feeSimulatedToReviewHint')}</p>
    {review.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : review.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : review.data.items.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('feeNoSimulatedToReview')}</p> : <>
      <label className={`${ui.label} mt-3 max-w-md`}>{t('feeVoidReason')}<input className={feeField} maxLength={240} value={reason} onChange={e => setReason(e.target.value)} /></label>
      <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeReceiptNo')}</th><th className="px-3 py-2.5">{t('feePaidOn')}</th><th className="px-3 py-2.5">{t('feeStudent')}</th><th className="px-3 py-2.5 text-right">{t('feeAmount')}</th><th className="px-3 py-2.5"><span className="sr-only">{t('feeVoid')}</span></th></tr></thead>
        <tbody>{review.data.items.map(p => <tr key={p.id} className="border-b border-slate-100"><td className="p-2 font-mono">{p.receipt_no}<SimulatedLabel className="mt-1 block font-sans" /></td><td className="px-3 py-2.5">{p.paid_on}</td><td className="px-3 py-2.5">{name(p.student_name, p.student_name_bn)}<span className="block text-xs text-slate-600">{p.admission_no} · {p.class_name}</span></td><td className="px-3 py-2.5 text-right tabular-nums">{money(p.amount)}</td>
          <td className="px-3 py-2.5"><button type="button" className={feeSecondary} disabled={action.busy || reason.trim() === ''} onClick={() => void voidPayment(p.id, p.receipt_no, p.amount)}>{t('feeVoid')}</button></td></tr>)}</tbody>
      </TableWrap>
    </>}
  </section>
}
