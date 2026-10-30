import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { assignFeeDue, getFeeReceipt, getFeeStructures, getStudentFeeStatement, recordFeePayment, searchFeeStudents, type FeeMethod, type FeeOptions, type FeeReceipt, type FeeStudent } from '../api/fees'
import { Icon } from '../components/icons'
import { usePeopleApi } from './peopleShared'
import { Avatar, CardTitle, EmptyState, Skeleton, ui } from './attendanceUi'
import { Notice, PaymentsList, ReceiptView, SimulatedLabel, StatusBadge, feeCard, feeField, newKey, todayInDhaka, useFeeAction, useLocalName, useMoney, useMonthLabel, type FeeAccess } from './feeShared'

// The fee counter (checkpoint 3): find a student by admission number, name,
// roll or guardian phone, see their dues oldest first, and collect in two or
// three clicks. Built for fast repeated use from the keyboard:
//   type → Enter picks the first match (arrows choose another),
//   Enter in the amount collects, Esc returns to the search.

const methods: FeeMethod[] = ['cash', 'bkash', 'nagad', 'bank']
const toLatin = (v: string) => v.replace(/[০-৯]/g, d => String('০১২৩৪৫৬৭৮৯'.indexOf(d)))

export function CounterPanel({ access, options }: { access: FeeAccess; options: FeeOptions }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const month = useMonthLabel()
  const action = useFeeAction()
  const searchRef = useRef<HTMLInputElement>(null)
  const amountRef = useRef<HTMLInputElement>(null)
  const submitting = useRef(false)
  const [search, setSearch] = useState('')
  const [active, setActive] = useState(0)
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

  const term = search.trim()
  const searchable = term.length >= 2 || /^[0-9০-৯]$/.test(term)
  const results = useQuery({ queryKey: ['fees', 'students', 'counter', term], enabled: searchable && !student, queryFn: () => authorized(token => searchFeeStudents(token, new URLSearchParams({ page: '1', page_size: '8', search: term }))), retry: false, placeholderData: previous => previous })
  const statement = useQuery({ queryKey: ['fees', 'statement', student?.id], enabled: !!student, queryFn: () => authorized(token => getStudentFeeStatement(token, student!.id)), retry: false })
  const structures = useQuery({ queryKey: ['fees', 'structures', 'student', student?.session_id, student?.class_id], enabled: !!student && access.canManage, queryFn: () => authorized(token => getFeeStructures(token, new URLSearchParams({ session_id: student!.session_id, class_id: student!.class_id, page_size: '100' }))), retry: false })
  const outstanding = (statement.data?.dues ?? []).filter(d => d.balance > 0).sort((a, b) => a.due_date.localeCompare(b.due_date) || a.id.localeCompare(b.id))
  const selectedTotal = outstanding.filter(d => selected.includes(d.id)).reduce((sum, d) => sum + d.balance, 0)
  const due = selected.length ? selectedTotal : statement.data?.totals.balance ?? 0
  const value = amount === '' ? due : Number(toLatin(amount))
  const valid = value > 0 && value <= due + 0.0001 && (method === 'cash' || reference.trim() !== '')
  const items = searchable && !student ? results.data?.items ?? [] : []

  useEffect(() => { setActive(0) }, [term])
  // After a student is picked the amount is ready for Enter.
  useEffect(() => { if (student && statement.data) amountRef.current?.focus() }, [student, statement.data])

  function pick(next: FeeStudent) {
    setStudent(next); setSelected([]); setAmount(''); setReference(''); setNote(''); setReceipt(null); setAttempt(newKey()); setPaidOn(todayInDhaka()); action.clear()
  }
  function reset() {
    setStudent(null); setSearch(''); setReceipt(null); setSelected([]); setAmount(''); setReference(''); setNote(''); action.clear()
    window.setTimeout(() => searchRef.current?.focus(), 0)
  }
  function onSearchKey(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' && items.length) { event.preventDefault(); setActive(i => (i + 1) % items.length) }
    else if (event.key === 'ArrowUp' && items.length) { event.preventDefault(); setActive(i => (i - 1 + items.length) % items.length) }
    else if (event.key === 'Enter' && items[active]) { event.preventDefault(); pick(items[active]) }
    else if (event.key === 'Escape') setSearch('')
  }
  async function collect(event: FormEvent) {
    event.preventDefault()
    if (!student || !valid || submitting.current) return
    submitting.current = true
    try {
      // The same key is resent if this attempt is retried, so a double
      // submit or a lost response never records the payment twice.
      const result = await action.run(() => authorized(token => recordFeePayment(token, { student_id: student.id, amount: Math.round(value * 100) / 100, method, reference: reference.trim(), paid_on: paidOn, note: note.trim(), idempotency_key: attempt, fee_ids: selected })), r => t(r.replayed ? 'feeReplayed' : 'feePaymentSaved', { receipt: r.payment.receipt_no }))
      if (result) { setReceipt(result); setSelected([]); setAmount(''); setReference(''); setNote(''); setAttempt(newKey()) }
    } finally { submitting.current = false }
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

  return <div className="space-y-5" onKeyDown={event => { if (event.key === 'Escape' && student && !(event.target instanceof HTMLSelectElement)) { event.preventDefault(); reset() } }}>
    {!student && <section className={`${feeCard} print:hidden`} aria-labelledby="counter-find">
      <CardTitle id="counter-find" icon="search">{t('c3CounterFind')}</CardTitle>
      <div className="relative mt-4">
        <Icon name="search" className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-slate-400" />
        <input ref={searchRef} id="counter-search" autoFocus type="search" role="combobox" aria-expanded={items.length > 0} aria-controls="counter-results" aria-activedescendant={items[active] ? `counter-result-${items[active].id}` : undefined} aria-label={t('c3CounterFind')} autoComplete="off"
          className={`${feeField} mt-0 min-h-14 pl-12 text-lg`} placeholder={t('c3CounterHint')} value={search} onChange={e => setSearch(e.target.value)} onKeyDown={onSearchKey} />
      </div>
      <p className="mt-2 text-xs text-slate-500">{t('c3CounterKeys')}</p>
      {results.isError && <p role="alert" className="mt-3 text-sm text-rose-700">{t('feeErrGeneric')}</p>}
      {searchable && results.data && (items.length === 0 ? <div className="mt-3"><EmptyState icon="search" title={t('feeNoStudents')} /></div> :
        <ul id="counter-results" role="listbox" aria-label={t('c3CounterResults')} className="mt-3 divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200">{items.map((s, i) =>
          <li key={s.id} id={`counter-result-${s.id}`} role="option" aria-selected={i === active} className={`flex min-h-14 cursor-pointer items-center gap-3 px-3 py-2.5 sm:px-4 ${i === active ? 'ds-softer' : 'hover:bg-slate-50'}`} onMouseEnter={() => setActive(i)} onClick={() => pick(s)}>
            <Avatar name={s.name_en} />
            <span className="min-w-0 flex-1"><strong className="block text-slate-900" style={{ overflowWrap: 'anywhere' }}>{name(s.name_en, s.name_bn)}</strong><span className="block text-xs text-slate-600">{s.admission_no} · {s.class_name} / {s.section_name}{s.roll_no != null && ` · ${t('p2HistoryRoll', { roll: s.roll_no })}`} · {s.session_name}</span></span>
            {i === active && <kbd className="hidden rounded border border-slate-300 bg-white px-1.5 text-xs text-slate-500 sm:inline">Enter</kbd>}
          </li>)}</ul>)}
      {!searchable && options.categories.length === 0 && <p className="mt-3 text-sm text-slate-600">{t('feeSetupFirst')}</p>}
    </section>}

    <Notice error={action.error} message={action.message} />

    {student && <>
      {receipt && <div className="space-y-3">
        <ReceiptView receipt={receipt} />
        <div className="flex flex-wrap gap-2 print:hidden">
          <button type="button" autoFocus className={`${ui.primary} min-h-12`} onClick={() => window.print()}><Icon name="print" className="h-4 w-4" />{t('feePrintReceipt')}</button>
          <button type="button" className={`${ui.secondary} min-h-12`} onClick={reset}><Icon name="search" className="h-4 w-4" />{t('c3NextStudent')} <kbd className="ml-1 rounded border border-slate-300 px-1 text-xs">Esc</kbd></button>
          <button type="button" className={`${ui.secondary} min-h-12`} onClick={() => setReceipt(null)}>{t('c3BackToDues')}</button>
        </div>
      </div>}

      {!receipt && <>
        <section className={`${feeCard} print:hidden`} aria-label={t('feeSelectedStudent')}>
          <div className="flex flex-wrap items-center gap-3">
            <Avatar name={student.name_en} />
            <div className="min-w-0 flex-1">
              <p className="text-lg font-bold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{student.name_en}{student.name_bn && <span lang="bn" className="ml-2 font-semibold text-slate-600">{student.name_bn}</span>}</p>
              <p className="text-sm text-slate-600">{student.admission_no} · {student.class_name} / {student.section_name}{student.roll_no != null && ` · ${t('p2HistoryRoll', { roll: student.roll_no })}`} · {student.session_name}{student.status !== 'active' && <span className="ml-2 rounded bg-slate-200 px-2 text-xs">{t(`feeStudentStatus_${student.status}`, { defaultValue: student.status })}</span>}</p>
            </div>
            <button type="button" className={`${ui.secondary} min-h-11`} onClick={reset}><Icon name="search" className="h-4 w-4" />{t('c3ChangeStudent')} <kbd className="ml-1 hidden rounded border border-slate-300 px-1 text-xs sm:inline">Esc</kbd></button>
          </div>
          {statement.data && statement.data.totals.simulated_unreviewed > 0 && <p role="alert" className="mt-3 flex flex-wrap items-center gap-2 rounded-xl border-2 border-rose-700 bg-rose-50 px-4 py-3 text-sm text-rose-900"><SimulatedLabel />{t('feeSimulatedUnreviewedStaff', { amount: money(statement.data.totals.simulated_unreviewed) })}</p>}
        </section>

        {statement.isPending ? <div className={feeCard} role="status"><span className="sr-only">{t('feeLoading')}</span><Skeleton rows={4} /></div> : statement.isError ? <p role="alert" className={`${feeCard} text-rose-700`}>{t('feeErrGeneric')}</p> :
          <div className="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(20rem,26rem)] lg:items-start">
            <section className={`${feeCard} print:hidden`} aria-labelledby="counter-dues">
              <div className="flex flex-wrap items-baseline justify-between gap-2"><CardTitle id="counter-dues" icon="receipt">{t('c3DuesOldestFirst')}</CardTitle><p className="text-sm font-semibold text-slate-700">{t('c3TotalDue')}: <span className="tabular-nums">{money(statement.data.totals.balance)}</span></p></div>
              {outstanding.length === 0 ? <div className="mt-3"><EmptyState icon="check" title={t('feeNothingOutstanding')} /></div> : <>
                <p className="mt-1 text-sm text-slate-600">{selected.length ? t('feePayingSelected', { count: selected.length, amount: money(selectedTotal) }) : t('c3PayingOldest')}</p>
                <ul className="mt-3 divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200">{outstanding.map(d => {
                  const checked = selected.includes(d.id)
                  return <li key={d.id}><label className={`flex min-h-14 cursor-pointer items-center gap-3 px-3 py-2.5 sm:px-4 ${checked ? 'ds-softer' : 'hover:bg-slate-50'}`}>
                    {access.canCollect && <input type="checkbox" className="h-5 w-5 shrink-0 accent-[var(--brand-primary)]" checked={checked} onChange={() => { setSelected(c => c.includes(d.id) ? c.filter(v => v !== d.id) : [...c, d.id]); setAmount('') }} />}
                    <span className="min-w-0 flex-1"><span className="block font-semibold text-slate-900">{name(d.category_name_en, d.category_name_bn)}{d.period && <span className="font-normal text-slate-600"> · {month(d.period)}</span>}</span>
                      <span className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-slate-600">{t('feeDueDate')} {d.due_date}<StatusBadge due={d} /></span></span>
                    <span className="text-right font-bold tabular-nums text-slate-900">{money(d.balance)}</span>
                  </label></li>
                })}</ul>
              </>}
            </section>

            {access.canCollect && outstanding.length > 0 && <form onSubmit={e => void collect(e)} className={`${feeCard} space-y-4 print:hidden lg:sticky lg:top-24`} aria-labelledby="counter-collect">
              <CardTitle id="counter-collect" icon="money">{t('feeRecordPayment')}</CardTitle>
              <div>
                <label className={ui.label} htmlFor="counter-amount">{t('feeAmount')}</label>
                <div className="mt-1.5 flex gap-2">
                  <input ref={amountRef} id="counter-amount" inputMode="decimal" autoComplete="off" className={`${feeField} mt-0 min-h-12 text-lg font-bold tabular-nums`} placeholder={due.toFixed(2)} value={amount} aria-describedby="counter-amount-help" aria-invalid={amount !== '' && !(value > 0 && value <= due + 0.0001)} onChange={e => setAmount(e.target.value)} />
                  <button type="button" className={`${ui.secondary} min-h-12 shrink-0`} onClick={() => setAmount('')} aria-pressed={amount === ''}>{t('c3Full')}</button>
                </div>
                <p id="counter-amount-help" className="mt-1 text-xs text-slate-500">{amount !== '' && value > due + 0.0001 ? <span className="font-semibold text-rose-700">{t('c3OverDue', { amount: money(due) })}</span> : amount === '' ? t('c3FullHint', { amount: money(due) }) : t('c3PartialHint', { left: money(Math.max(0, due - value)) })}</p>
              </div>
              <fieldset>
                <legend className={ui.label}>{t('feeMethod')}</legend>
                <div className="mt-1.5 grid grid-cols-4 gap-1.5" role="radiogroup" aria-label={t('feeMethod')}>
                  {methods.map(m => <button key={m} type="button" role="radio" aria-checked={method === m} onClick={() => setMethod(m)} className={`min-h-11 rounded-xl border px-2 text-sm font-semibold transition ${method === m ? 'border-[var(--brand-primary)] bg-[var(--brand-primary)] text-[var(--brand-on)]' : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50'}`}>{t(`feeMethod_${m}`)}</button>)}
                </div>
              </fieldset>
              {method !== 'cash' && <label className={ui.label}>{t(method === 'bank' ? 'feeReference' : 'c3TrxId')}<input required maxLength={80} autoComplete="off" className={`${feeField} font-mono`} placeholder={t('feeReferenceHint')} value={reference} onChange={e => setReference(e.target.value)} /></label>}
              <details className="text-sm">
                <summary className="cursor-pointer font-semibold text-slate-700">{t('c3DateAndNote', { date: paidOn })}</summary>
                <div className="mt-2 grid gap-3">
                  <label className={ui.label}>{t('feePaidOn')}<input required type="date" max={todayInDhaka()} className={feeField} value={paidOn} onChange={e => setPaidOn(e.target.value)} /></label>
                  <label className={ui.label}>{t('feeNote')}<input maxLength={240} className={feeField} value={note} onChange={e => setNote(e.target.value)} /></label>
                </div>
              </details>
              <button disabled={action.busy || !valid} className={`${ui.primary} min-h-14 w-full text-base`}><Icon name="check" className="h-5 w-5" />{action.busy ? t('feeSaving') : t('c3CollectAmount', { amount: money(value > 0 ? value : 0) })}</button>
            </form>}
          </div>}

        {statement.data && <section className={`${feeCard} print:hidden`}><CardTitle icon="receipt">{t('feeReceipts')}</CardTitle><PaymentsList payments={statement.data.payments} onOpen={id => void openReceipt(id)} /></section>}

        {access.canManage && <details className={`${feeCard} print:hidden`}>
          <summary className="cursor-pointer text-base font-bold text-slate-900">{t('feeAssignTitle')}</summary>
          <p className="mt-1 text-sm text-slate-600">{t('feeAssignHint')}</p>
          <form onSubmit={assignDue} className="mt-3 grid gap-3 sm:grid-cols-3">
            <label className={ui.label}>{t('feeStructure')}<select required className={feeField} value={assign.structure_id} onChange={e => setAssign({ ...assign, structure_id: e.target.value })}><option value="">{t('feeChoose')}</option>{structures.data?.items.filter(s => s.is_active).map(s => <option key={s.id} value={s.id}>{name(s.category_name_en, s.category_name_bn)} · {money(s.amount)}</option>)}</select></label>
            <label className={ui.label}>{t('feeMonth')}<input type="month" required={selectedStructure?.frequency !== 'one_time'} className={feeField} value={assign.period} onChange={e => setAssign({ ...assign, period: e.target.value })} /></label>
            <div className="flex items-end"><button disabled={action.busy} className={`${ui.primary} w-full`}>{t('feeAssign')}</button></div>
          </form>
        </details>}
      </>}
    </>}
  </div>
}
