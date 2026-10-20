import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { adjustFeeDue, assignFeeDue, deleteFeeDue, generateFeeDues, getFeeAdjustments, getFeeDues, getFeeOptions, getFeeReceipt, getFeeStructures, getStudentFeeStatement, recordFeePayment, type FeeDue, type FeeMethod, type FeeOptions, type FeeReceipt, type FeeStudent } from '../api/fees'
import { usePeopleApi } from './peopleShared'
import { SimulatedLabel, DuesTable, Notice, useFeeAction, type FeeAccess, Pager, PaymentsList, ReceiptView, StudentPicker, Totals, feeButton, feeCard, feeField, feeSecondary, newKey, todayInDhaka, useLocalName, useMoney } from './feeShared'
import { FeeReportsPanel, FeeSetupPanel, LedgerPanel } from './FeeAdminPanels'
import { ui, PageHeader, Tabs } from './attendanceUi'

type Tab = 'collect' | 'dues' | 'setup' | 'reports' | 'ledger'

export function FeesPage({ access }: { access: FeeAccess }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const [tab, setTab] = useState<Tab>('collect')
  const options = useQuery({ queryKey: ['fees', 'options'], queryFn: () => authorized(token => getFeeOptions(token)), retry: false })
  const tabs: Tab[] = ['collect', 'dues', 'setup', 'reports', ...(access.canReadAccounts ? ['ledger' as const] : [])]
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_finance')} title={t('feeHeading')} description={t('feeIntro')} />
    <Tabs label={t('feeHeading')} value={tab} onChange={setTab} items={tabs.map(key => ({ value: key, label: t(`feeTab_${key}`) }))} />
    {options.isPending ? <p role="status">{t('feeLoading')}</p> : options.isError ? <p role="alert" className="text-rose-700">{t('feeErrGeneric')}</p> : <>
      {tab === 'collect' && <CollectPanel access={access} options={options.data} />}
      {tab === 'dues' && <DuesPanel access={access} options={options.data} />}
      {tab === 'setup' && <FeeSetupPanel access={access} options={options.data} />}
      {tab === 'reports' && <FeeReportsPanel options={options.data} />}
      {tab === 'ledger' && access.canReadAccounts && <LedgerPanel canManage={access.canManageAccounts} />}
    </>}
  </div>
}

function CollectPanel({ access, options }: { access: FeeAccess; options: FeeOptions }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const action = useFeeAction()
  const [student, setStudent] = useState<FeeStudent | null>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [amount, setAmount] = useState('')
  const [method, setMethod] = useState<FeeMethod>('cash')
  const [reference, setReference] = useState('')
  const [paidOn, setPaidOn] = useState(todayInDhaka())
  const [note, setNote] = useState('')
  const [attempt, setAttempt] = useState(newKey())
  const [receipt, setReceipt] = useState<FeeReceipt | null>(null)
  const [assign, setAssign] = useState({ structure_id: '', period: todayInDhaka().slice(0, 7) })
  const statement = useQuery({ queryKey: ['fees', 'statement', student?.id], enabled: !!student, queryFn: () => authorized(token => getStudentFeeStatement(token, student!.id)), retry: false })
  const structures = useQuery({ queryKey: ['fees', 'structures', 'student', student?.session_id, student?.class_id], enabled: !!student && access.canManage, queryFn: () => authorized(token => getFeeStructures(token, new URLSearchParams({ session_id: student!.session_id, class_id: student!.class_id, page_size: '100' }))), retry: false })
  const outstanding = statement.data?.dues.filter(d => d.balance > 0) ?? []
  const selectedBalance = outstanding.filter(d => selected.includes(d.id)).reduce((sum, d) => sum + d.balance, 0)
  const suggested = selected.length ? selectedBalance : statement.data?.totals.balance ?? 0

  function pick(next: FeeStudent) {
    setStudent(next); setSelected([]); setAmount(''); setReference(''); setNote(''); setReceipt(null); setAttempt(newKey()); action.clear()
  }
  function toggle(id: string) { setSelected(current => current.includes(id) ? current.filter(v => v !== id) : [...current, id]) }
  async function pay(event: FormEvent) {
    event.preventDefault()
    if (!student) return
    const value = Number(amount || suggested)
    const result = await action.run(() => authorized(token => recordFeePayment(token, { student_id: student.id, amount: value, method, reference: reference.trim(), paid_on: paidOn, note: note.trim(), idempotency_key: attempt, fee_ids: selected })), r => t(r.replayed ? 'feeReplayed' : 'feePaymentSaved', { receipt: r.payment.receipt_no }))
    if (result) { setReceipt(result); setSelected([]); setAmount(''); setReference(''); setNote(''); setAttempt(newKey()) }
  }
  async function openReceipt(id: string) {
    const result = await action.run(() => authorized(token => getFeeReceipt(token, `/fees/payments/${encodeURIComponent(id)}`)), '')
    if (result) setReceipt(result)
  }
  async function assignDue(event: FormEvent) {
    event.preventDefault()
    if (student) await action.run(() => authorized(token => assignFeeDue(token, { student_id: student.id, ...assign })), t('feeAssigned'))
  }
  const selectedStructure = structures.data?.items.find(s => s.id === assign.structure_id)

  return <div className="space-y-5">
    <section className={`${feeCard} print:hidden`}><StudentPicker onPick={pick} />
      {student && <p className="mt-3 text-sm">{t('feeSelectedStudent')}: <strong>{name(student.name_en, student.name_bn)}</strong> · {student.admission_no} · {student.class_name} / {student.section_name} · {student.session_name}{student.status !== 'active' && <span className="ml-2 rounded bg-slate-200 px-2 text-xs">{t(`feeStudentStatus_${student.status}`, { defaultValue: student.status })}</span>}</p>}
    </section>
    <Notice error={action.error} message={action.message} />
    {statement.data && statement.data.totals.simulated_unreviewed > 0 && <p role="alert" className="flex flex-wrap items-center gap-2 rounded-xl border-2 border-rose-700 bg-rose-50 px-4 py-3 text-sm text-rose-900 print:hidden"><SimulatedLabel />{t('feeSimulatedUnreviewedStaff', { amount: money(statement.data.totals.simulated_unreviewed) })}</p>}
    {receipt && <ReceiptView receipt={receipt} onClose={() => setReceipt(null)} />}
    {student && (statement.isPending ? <p role="status">{t('feeLoading')}</p> : statement.isError ? <p role="alert" className="text-rose-700">{t('feeErrGeneric')}</p> : <>
      <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeStatement')}</h2><div className="mt-3"><Totals totals={statement.data.totals} /></div>
        <DuesTable dues={statement.data.dues} selectable={access.canCollect} selected={selected} onToggle={toggle} />
      </section>
      {access.canCollect && <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeRecordPayment')}</h2>
        {outstanding.length === 0 ? <p className="mt-2 text-slate-600">{t('feeNothingOutstanding')}</p> :
          <form onSubmit={pay} className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <p className="text-sm text-slate-600 sm:col-span-2 lg:col-span-3">{selected.length ? t('feePayingSelected', { count: selected.length, amount: money(selectedBalance) }) : t('feePayingOldest')}</p>
            <label className={ui.label}>{t('feeAmount')}<input type="number" inputMode="decimal" min="0.01" step="0.01" max={suggested || undefined} className={feeField} value={amount} placeholder={suggested.toFixed(2)} onChange={e => setAmount(e.target.value)} /></label>
            <label className={ui.label}>{t('feeMethod')}<select className={feeField} value={method} onChange={e => setMethod(e.target.value as FeeMethod)}>{(['cash', 'bank', 'bkash', 'nagad'] as const).map(m => <option key={m} value={m}>{t(`feeMethod_${m}`)}</option>)}</select></label>
            <label className={ui.label}>{t('feeReference')}<input required={method !== 'cash'} maxLength={80} className={feeField} value={reference} placeholder={method === 'cash' ? t('feeOptional') : t('feeReferenceHint')} onChange={e => setReference(e.target.value)} /></label>
            <label className={ui.label}>{t('feePaidOn')}<input required type="date" max={todayInDhaka()} className={feeField} value={paidOn} onChange={e => setPaidOn(e.target.value)} /></label>
            <label className={`${ui.label} sm:col-span-2`}>{t('feeNote')}<input maxLength={240} className={feeField} value={note} onChange={e => setNote(e.target.value)} /></label>
            <div className="flex items-end"><button disabled={action.busy} className={`${feeButton} w-full`}>{action.busy ? t('feeSaving') : t('feeCollect')}</button></div>
          </form>}
      </section>}
      {access.canManage && <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeAssignTitle')}</h2><p className="mt-1 text-sm text-slate-600">{t('feeAssignHint')}</p>
        <form onSubmit={assignDue} className="mt-3 grid gap-3 sm:grid-cols-3">
          <label className={ui.label}>{t('feeStructure')}<select required className={feeField} value={assign.structure_id} onChange={e => setAssign({ ...assign, structure_id: e.target.value })}><option value="">{t('feeChoose')}</option>{structures.data?.items.filter(s => s.is_active).map(s => <option key={s.id} value={s.id}>{name(s.category_name_en, s.category_name_bn)} · {money(s.amount)}</option>)}</select></label>
          <label className={ui.label}>{t('feeMonth')}<input type="month" required={selectedStructure?.frequency !== 'one_time'} className={feeField} value={assign.period} onChange={e => setAssign({ ...assign, period: e.target.value })} /></label>
          <div className="flex items-end"><button disabled={action.busy} className={`${feeButton} w-full`}>{t('feeAssign')}</button></div>
        </form>
      </section>}
      <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeReceipts')}</h2><PaymentsList payments={statement.data.payments} onOpen={id => void openReceipt(id)} /></section>
    </>)}
    {!student && options.categories.length === 0 && <p className={`${feeCard} text-slate-600`}>{t('feeSetupFirst')}</p>}
  </div>
}

function DuesPanel({ access, options }: { access: FeeAccess; options: FeeOptions }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const action = useFeeAction()
  const activeSession = options.sessions.find(s => s.is_active)?.id ?? options.sessions[0]?.id ?? ''
  const [generate, setGenerate] = useState({ session_id: activeSession, class_id: '', period: todayInDhaka().slice(0, 7), include_one_time: false })
  const [filters, setFilters] = useState({ session_id: activeSession, class_id: '', status: 'outstanding', period: '', search: '' })
  const [page, setPage] = useState(1)
  const [adjusting, setAdjusting] = useState<{ due: FeeDue; kind: 'discount' | 'waiver'; amount: string; reason: string } | null>(null)
  const params = new URLSearchParams({ page: String(page), page_size: '20', ...Object.fromEntries(Object.entries(filters).filter(([, v]) => v)) })
  const dues = useQuery({ queryKey: ['fees', 'dues', params.toString()], queryFn: () => authorized(token => getFeeDues(token, params)), retry: false })
  const history = useQuery({ queryKey: ['fees', 'adjustments', adjusting?.due.id], enabled: !!adjusting, queryFn: () => authorized(token => getFeeAdjustments(token, adjusting!.due.id)), retry: false })
  const setFilter = (key: keyof typeof filters, value: string) => { setFilters({ ...filters, [key]: value }); setPage(1) }

  async function runGenerate(event: FormEvent) {
    event.preventDefault()
    await action.run(() => authorized(token => generateFeeDues(token, generate)), r => t('feeGenerated', { created: r.created, skipped: r.skipped }))
  }
  async function saveAdjustment(event: FormEvent) {
    event.preventDefault()
    if (!adjusting) return
    const done = await action.run(() => authorized(token => adjustFeeDue(token, adjusting.due.id, { kind: adjusting.kind, amount: Number(adjusting.amount || 0), reason: adjusting.reason.trim() })), t('feeAdjusted'))
    if (done) setAdjusting(null)
  }
  async function remove(due: FeeDue) {
    if ((await confirmAction(t('feeConfirmDeleteDue')))) await action.run(() => authorized(token => deleteFeeDue(token, due.id)), t('feeDeleted'))
  }

  return <div className="space-y-5">
    <Notice error={action.error} message={action.message} />
    {access.canManage && <section className={feeCard}><h2 className={ui.sectionTitle}>{t('feeGenerateTitle')}</h2><p className="mt-1 text-sm text-slate-600">{t('feeGenerateHint')}</p>
      <form onSubmit={runGenerate} className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <label className={ui.label}>{t('feeSession')}<select required className={feeField} value={generate.session_id} onChange={e => setGenerate({ ...generate, session_id: e.target.value })}><option value="">{t('feeChoose')}</option>{options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeClass')}<select className={feeField} value={generate.class_id} onChange={e => setGenerate({ ...generate, class_id: e.target.value })}><option value="">{t('feeAllClasses')}</option>{options.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeMonth')}<input required type="month" className={feeField} value={generate.period} onChange={e => setGenerate({ ...generate, period: e.target.value })} /></label>
        <label className="flex items-center gap-2 self-end pb-2 text-sm"><input type="checkbox" checked={generate.include_one_time} onChange={e => setGenerate({ ...generate, include_one_time: e.target.checked })} />{t('feeIncludeOneTime')}</label>
        <div className="flex items-end"><button disabled={action.busy} className={`${feeButton} w-full`}>{t('feeGenerate')}</button></div>
      </form>
    </section>}
    <section className={feeCard}><h2 className={ui.sectionTitle}>{t('feeDuesTitle')}</h2>
      <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <label className={ui.label}>{t('feeSession')}<select className={feeField} value={filters.session_id} onChange={e => setFilter('session_id', e.target.value)}><option value="">{t('feeAll')}</option>{options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeClass')}<select className={feeField} value={filters.class_id} onChange={e => setFilter('class_id', e.target.value)}><option value="">{t('feeAllClasses')}</option>{options.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeStatus')}<select className={feeField} value={filters.status} onChange={e => setFilter('status', e.target.value)}><option value="">{t('feeAll')}</option>{['outstanding', 'overdue', 'unpaid', 'partial', 'paid', 'waived'].map(s => <option key={s} value={s}>{t(`feeStatus_${s}`)}</option>)}</select></label>
        <label className={ui.label}>{t('feeMonth')}<input type="month" className={feeField} value={filters.period} onChange={e => setFilter('period', e.target.value)} /></label>
        <label className={ui.label}>{t('feeSearch')}<input className={feeField} value={filters.search} onChange={e => setFilter('search', e.target.value)} /></label>
      </div>
      {dues.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : dues.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : dues.data.items.length === 0 ? <p className="mt-3 text-slate-600">{t('feeNoDues')}</p> : <>
        <div className="mt-3 overflow-x-auto"><table className="w-full min-w-[48rem] border-collapse text-left text-sm"><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeStudent')}</th><th className="px-3 py-2.5">{t('feeCategory')}</th><th className="px-3 py-2.5">{t('feeDueDate')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeAmount')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeDiscount')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feePaid')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeBalance')}</th><th className="px-3 py-2.5">{t('feeStatus')}</th>{access.canManage && <th className="px-3 py-2.5">{t('feeActions')}</th>}</tr></thead>
          <tbody>{dues.data.items.map(d => <tr key={d.id} className="border-b align-top"><td className="px-3 py-2.5"><strong>{name(d.student_name, d.student_name_bn)}</strong><span className="block text-xs text-slate-600">{d.admission_no} · {d.class_name}</span></td>
            <td className="px-3 py-2.5">{name(d.category_name_en, d.category_name_bn)}{d.period && <span className="block text-xs text-slate-600">{d.period}</span>}</td><td className="whitespace-nowrap p-2">{d.due_date}</td>
            <td className="px-3 py-2.5 text-right tabular-nums">{money(d.amount)}</td><td className="px-3 py-2.5 text-right tabular-nums">{money(d.discount_amount)}</td><td className="px-3 py-2.5 text-right tabular-nums">{money(d.paid_amount)}</td><td className="p-2 text-right font-semibold">{money(d.balance)}</td>
            <td className="px-3 py-2.5"><span className={`inline-block rounded-full px-2 py-0.5 text-xs font-semibold ${d.overdue ? 'bg-rose-100 text-rose-800' : 'bg-slate-100'}`}>{t(`feeStatus_${d.status}`)}{d.overdue ? ` · ${t('feeOverdue')}` : ''}</span></td>
            {access.canManage && <td className="px-3 py-2.5"><div className="flex flex-wrap gap-1">
              {d.balance > 0 && <button type="button" className={feeSecondary} onClick={() => setAdjusting({ due: d, kind: 'discount', amount: '', reason: '' })}>{t('feeDiscountAction')}</button>}
              {d.balance > 0 && access.role === 'admin' && <button type="button" className={feeSecondary} onClick={() => setAdjusting({ due: d, kind: 'waiver', amount: '', reason: '' })}>{t('feeWaive')}</button>}
              {d.paid_amount === 0 && <button type="button" className={`${feeSecondary} text-rose-700`} onClick={() => void remove(d)}>{t('feeDelete')}</button>}
            </div></td>}</tr>)}</tbody></table></div>
        <Pager page={page} total={dues.data.total} size={20} onPage={setPage} />
      </>}
    </section>
    {adjusting && <section className={feeCard} aria-live="polite"><h2 className={ui.sectionTitle}>{t(adjusting.kind === 'waiver' ? 'feeWaiveTitle' : 'feeDiscountTitle')}</h2>
      <p className="mt-1 text-sm">{name(adjusting.due.student_name, adjusting.due.student_name_bn)} · {name(adjusting.due.category_name_en, adjusting.due.category_name_bn)} · {t('feeBalance')} {money(adjusting.due.balance)}</p>
      <form onSubmit={saveAdjustment} className="mt-3 grid gap-3 sm:grid-cols-3">
        {adjusting.kind === 'discount' ? <label className={ui.label}>{t('feeAmount')}<input required type="number" min="0.01" step="0.01" max={adjusting.due.balance} className={feeField} value={adjusting.amount} onChange={e => setAdjusting({ ...adjusting, amount: e.target.value })} /></label> : <p className="text-sm text-amber-900 sm:col-span-1">{t('feeWaiveHint', { amount: money(adjusting.due.balance) })}</p>}
        <label className={ui.label}>{t('feeReason')}<input required maxLength={240} className={feeField} value={adjusting.reason} onChange={e => setAdjusting({ ...adjusting, reason: e.target.value })} /></label>
        <div className="flex items-end gap-2"><button disabled={action.busy} className={feeButton}>{t('feeSave')}</button><button type="button" className={feeSecondary} onClick={() => setAdjusting(null)}>{t('feeCancel')}</button></div>
      </form>
      {history.data && history.data.length > 0 && <ul className="mt-3 space-y-1 text-sm text-slate-700">{history.data.map(a => <li key={a.id}>{a.created_at} · {t(`feeAdjust_${a.kind}`)} · {money(a.amount)} · {a.reason}</li>)}</ul>}
    </section>}
  </div>
}
