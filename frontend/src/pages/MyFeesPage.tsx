import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { confirmFeeIntent, getChildFeeStatement, getFeeReceipt, getOnlinePaymentStatus, getOwnFeeStatement, startFeeIntent, type FeeIntent, type FeeReceipt, type FeeSimulation } from '../api/fees'
import { usePeopleApi } from './peopleShared'
import { DuesTable, Notice, PaymentsList, ReceiptView, SimulatedLabel, Totals, feeButton, feeCard, feeField, feeSecondary, useFeeAction, useLocalName, useMoney } from './feeShared'
import { ui, PageHeader, EmptyState, Notice as Banner } from './attendanceUi'
import { StateScreen } from '../components/StateScreen'

// MyFeesPage shows a student's own fees or a guardian's linked child's fees.
// Access is enforced by the API; unlinked children read as not found.
export function MyFeesPage({ mode, canPay }: { mode: 'student' | 'guardian'; canPay: boolean }) {
  const { t } = useTranslation()
  const { id: childId = '' } = useParams()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const action = useFeeAction()
  const [receipt, setReceipt] = useState<FeeReceipt | null>(null)
  const [provider, setProvider] = useState<FeeIntent['provider']>('bkash')
  const [amount, setAmount] = useState('')
  const [intent, setIntent] = useState<FeeIntent | null>(null)
  const [otp, setOtp] = useState('')
  const [simulation, setSimulation] = useState<FeeSimulation | null>(null)
  // Online payment exists only as a simulation that a school switches on for testing.
  const online = useQuery({ queryKey: ['fees', 'online-status'], enabled: canPay, queryFn: () => authorized(token => getOnlinePaymentStatus(token)), retry: false })
  const statement = useQuery({ queryKey: ['fees', 'mine', mode, childId], queryFn: () => authorized(token => mode === 'student' ? getOwnFeeStatement(token) : getChildFeeStatement(token, childId)), retry: false })
  const receiptBase = mode === 'student' ? '/fees/me/payments' : `/fees/children/${encodeURIComponent(childId)}/payments`
  const balance = statement.data?.totals.balance ?? 0

  async function open(id: string) {
    const value = await action.run(() => authorized(token => getFeeReceipt(token, `${receiptBase}/${encodeURIComponent(id)}`)), '')
    if (value) { setReceipt(value); action.clear() }
  }
  async function start(event: FormEvent) {
    event.preventDefault()
    const value = await action.run(() => authorized(token => startFeeIntent(token, { ...(mode === 'guardian' ? { student_id: childId } : {}), provider, amount: Number(amount || balance) })), t('feeCheckoutStarted'))
    if (value) { setIntent(value); setOtp('') }
  }
  async function confirm(event: FormEvent) {
    event.preventDefault()
    if (!intent) return
    const value = await action.run(() => authorized(token => confirmFeeIntent(token, intent.id, otp)), t('feeSimulationDone'))
    if (value) { setSimulation(value); setIntent(null); setAmount('') }
    else setIntent(null)
  }

  const heading = mode === 'student' ? t('feeMyFees') : t('feeChildFees')
  if (statement.isPending) return <StateScreen kind="loading" message={t('feeLoading')} />
  if (statement.isError) return <div className="space-y-6"><PageHeader eyebrow={t('m12Group_finance')} title={heading} /><EmptyState icon="receipt" title={t('feeStatementUnavailable')} /></div>
  const s = statement.data
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_finance')} title={heading} description={`${name(s.student.name_en, s.student.name_bn)} · ${s.student.admission_no} · ${s.student.class_name} · ${s.student.section_name} · ${s.student.session_name}`} />
    <section className={`${feeCard} print:hidden`} aria-label={t('uxFeeSummary')}><Totals totals={s.totals} />
      {s.totals.overdue > 0 && <div className="mt-4"><Banner tone="warning">{t('feeArrearsNotice', { amount: money(s.totals.overdue) })}</Banner></div>}
    </section>
    <Notice error={action.error} message={action.message} />
    {s.totals.simulated_unreviewed > 0 && <Banner tone="warning">{t('feeSimulatedUnreviewed', { amount: money(s.totals.simulated_unreviewed) })}</Banner>}
    {receipt && <ReceiptView receipt={receipt} onClose={() => setReceipt(null)} />}
    {simulation && <section role="status" className={`${feeCard} border-2 border-rose-700 print:hidden`}><SimulatedLabel /><h2 className={`${ui.sectionTitle} mt-2`}>{t('feeSimulationResultTitle')}</h2>
      <p className="mt-2 text-sm">{t(`feeMethod_${simulation.intent.provider}`)} · {money(simulation.intent.amount)} · {t('feeProviderRef')}: <span className="break-all font-mono">{simulation.intent.provider_ref}</span></p>
      <p className="mt-1 text-sm font-semibold">{t('feeBalanceUnchanged', { amount: money(simulation.balance) })}</p>
      <button type="button" className={`${feeSecondary} mt-3`} onClick={() => setSimulation(null)}>{t('feeClose')}</button></section>}
    {canPay && online.data?.mode === 'simulation' && balance > 0 && <section className={`${feeCard} border-2 border-rose-700 print:hidden`}><SimulatedLabel /><h2 className={`${ui.sectionTitle} mt-2`}>{t('feePayOnline')}</h2>
      <div className="mt-3"><Banner tone="warning">{t('feeSimulationModeNotice')}</Banner></div>
      {!intent ? <form onSubmit={start} className="mt-3 grid gap-3 sm:grid-cols-3">
        <fieldset className="text-sm font-semibold"><legend>{t('feeProvider')}</legend><div className="mt-2 flex gap-4">{(['bkash', 'nagad'] as const).map(p => <label key={p} className="flex items-center gap-2 font-normal"><input type="radio" name="provider" value={p} checked={provider === p} onChange={() => setProvider(p)} />{t(`feeMethod_${p}`)}</label>)}</div></fieldset>
        <label className={ui.label}>{t('feeAmount')}<input type="number" inputMode="decimal" min="0.01" max={balance} step="0.01" className={feeField} value={amount} placeholder={balance.toFixed(2)} onChange={e => setAmount(e.target.value)} /></label>
        <div className="flex items-end"><button disabled={action.busy} className={`${feeButton} w-full`}>{t('feeContinue')}</button></div>
      </form> : <form onSubmit={confirm} className="mt-3 space-y-3 rounded-xl border-2 border-dashed border-[var(--brand-primary)] p-4">
        <p className="font-semibold">{t(`feeMethod_${intent.provider}`)} · {money(intent.amount)}</p>
        <p className="text-sm">{t('feeProviderRef')}: <span className="break-all font-mono">{intent.provider_ref}</span> · {t('feeExpiresAt', { time: intent.expires_at.slice(11, 16) })}</p>
        <label className="block max-w-xs text-sm font-semibold">{t('feeOtp')}<input required inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{4,8}" maxLength={8} className={feeField} value={otp} onChange={e => setOtp(e.target.value)} /></label>
        <p className="text-xs text-slate-600">{t('feeOtpHint')}</p>
        <div className="flex flex-wrap gap-2"><button disabled={action.busy} className={feeButton}>{t('feeConfirmPayment')}</button><button type="button" className={feeSecondary} onClick={() => { setIntent(null); action.clear() }}>{t('feeCancel')}</button></div>
      </form>}
    </section>}
    <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeDuesTitle')}</h2><DuesTable dues={s.dues} /></section>
    <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeReceipts')}</h2><PaymentsList payments={s.payments} onOpen={id => void open(id)} /></section>
  </div>
}
