import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { deleteFeeConcession, deleteLedgerEntry, getAccountingSummary, getFeeCategories, getFeeCollections, getFeeConcessions, getFeeDueReport, getFeePayments, getFeeReceipt, getFeeStructures, getLedgerEntries, ledgerCategories, saveFeeCategory, saveFeeConcession, saveFeeStructure, saveLedgerEntry, type FeeBucket, type FeeCategoryInput, type FeeOptions, type FeeReceipt, type FeeStructureInput, type FeeStudent, type LedgerEntryInput } from '../api/fees'
import { usePeopleApi } from './peopleShared'
import { Notice, Pager, ReceiptView, StudentPicker, TableWrap, feeButton, feeCard, feeField, feeSecondary, monthStart, todayInDhaka, useFeeAction, useLocalName, useMoney, type FeeAccess } from './feeShared'
import { ui } from './attendanceUi'

const blankCategory: FeeCategoryInput = { code: '', name_en: '', name_bn: '', frequency: 'monthly', is_active: true }

export function FeeSetupPanel({ access, options }: { access: FeeAccess; options: FeeOptions }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const action = useFeeAction()
  const activeSession = options.sessions.find(s => s.is_active)?.id ?? options.sessions[0]?.id ?? ''
  const blankStructure: FeeStructureInput = { session_id: activeSession, class_id: '', category_id: '', amount: 0, due_day: 10, is_active: true }
  const [category, setCategory] = useState<FeeCategoryInput>(blankCategory)
  const [categoryId, setCategoryId] = useState('')
  const [structure, setStructure] = useState<FeeStructureInput>(blankStructure)
  const [structureId, setStructureId] = useState('')
  const [structureSession, setStructureSession] = useState(activeSession)
  const [concessionStudent, setConcessionStudent] = useState<FeeStudent | null>(null)
  const [concession, setConcession] = useState({ category_id: '', percent: '', reason: '' })
  const all = new URLSearchParams({ page: '1', page_size: '100' })
  const categories = useQuery({ queryKey: ['fees', 'categories'], queryFn: () => authorized(token => getFeeCategories(token, all)), retry: false })
  const structureParams = new URLSearchParams({ page: '1', page_size: '100', ...(structureSession ? { session_id: structureSession } : {}) })
  const structures = useQuery({ queryKey: ['fees', 'structures', structureParams.toString()], queryFn: () => authorized(token => getFeeStructures(token, structureParams)), retry: false })
  const concessions = useQuery({ queryKey: ['fees', 'concessions'], queryFn: () => authorized(token => getFeeConcessions(token, all)), retry: false })

  async function submitCategory(event: FormEvent) {
    event.preventDefault()
    const saved = await action.run(() => authorized(token => saveFeeCategory(token, category, categoryId || undefined)), t('feeSaved'))
    if (saved) { setCategory(blankCategory); setCategoryId('') }
  }
  async function submitStructure(event: FormEvent) {
    event.preventDefault()
    const saved = await action.run(() => authorized(token => saveFeeStructure(token, { ...structure, amount: Number(structure.amount) }, structureId || undefined)), t('feeSaved'))
    if (saved) { setStructure(blankStructure); setStructureId('') }
  }
  async function submitConcession(event: FormEvent) {
    event.preventDefault()
    if (!concessionStudent) return
    const saved = await action.run(() => authorized(token => saveFeeConcession(token, { student_id: concessionStudent.id, category_id: concession.category_id, percent: Number(concession.percent), reason: concession.reason.trim() })), t('feeSaved'))
    if (saved) { setConcession({ category_id: '', percent: '', reason: '' }); setConcessionStudent(null) }
  }
  const allCategories = categories.data?.items ?? []

  return <div className="space-y-5">
    <Notice error={action.error} message={action.message} />
    <section className={feeCard}><h2 className={ui.sectionTitle}>{t('feeCategories')}</h2>
      {access.canManage && <form onSubmit={submitCategory} className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
        <label className={ui.label}>{t('feeCode')}<input required pattern="[A-Za-z0-9_\-]{2,20}" maxLength={20} className={`${feeField} uppercase`} value={category.code} onChange={e => setCategory({ ...category, code: e.target.value.toUpperCase() })} /></label>
        <label className={ui.label}>{t('feeNameEn')}<input required maxLength={120} className={feeField} value={category.name_en} onChange={e => setCategory({ ...category, name_en: e.target.value })} /></label>
        <label className={ui.label}>{t('feeNameBn')}<input maxLength={80} lang="bn" className={feeField} value={category.name_bn} onChange={e => setCategory({ ...category, name_bn: e.target.value })} /></label>
        <label className={ui.label}>{t('feeFrequency')}<select className={feeField} value={category.frequency} onChange={e => setCategory({ ...category, frequency: e.target.value as FeeCategoryInput['frequency'] })}><option value="monthly">{t('feeFrequency_monthly')}</option><option value="one_time">{t('feeFrequency_one_time')}</option></select></label>
        <label className="flex items-center gap-2 self-end pb-2 text-sm"><input type="checkbox" checked={category.is_active} onChange={e => setCategory({ ...category, is_active: e.target.checked })} />{t('feeActive')}</label>
        <div className="flex items-end gap-2"><button disabled={action.busy} className={feeButton}>{categoryId ? t('feeUpdate') : t('feeAdd')}</button>{categoryId && <button type="button" className={feeSecondary} onClick={() => { setCategory(blankCategory); setCategoryId('') }}>{t('feeCancel')}</button>}</div>
      </form>}
      {categories.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : allCategories.length === 0 ? <p className="mt-3 text-slate-600">{t('feeNoCategories')}</p> :
        <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeCode')}</th><th className="px-3 py-2.5">{t('feeName')}</th><th className="px-3 py-2.5">{t('feeFrequency')}</th><th className="px-3 py-2.5">{t('feeStatus')}</th>{access.canManage && <th className="px-3 py-2.5"><span className="sr-only">{t('feeActions')}</span></th>}</tr></thead>
          <tbody>{allCategories.map(c => <tr key={c.id} className="border-b border-slate-100"><td className="p-2 font-mono">{c.code}</td><td className="px-3 py-2.5">{name(c.name_en, c.name_bn)}</td><td className="px-3 py-2.5">{t(`feeFrequency_${c.frequency}`)}</td><td className="px-3 py-2.5">{t(c.is_active ? 'feeActive' : 'feeInactive')}</td>
            {access.canManage && <td className="px-3 py-2.5"><button type="button" className={ui.link} onClick={() => { setCategoryId(c.id); setCategory({ code: c.code, name_en: c.name_en, name_bn: c.name_bn, frequency: c.frequency, is_active: c.is_active }) }}>{t('feeEdit')}</button></td>}</tr>)}</tbody></TableWrap>}
    </section>

    <section className={feeCard}><h2 className={ui.sectionTitle}>{t('feeStructures')}</h2><p className="mt-1 text-sm text-slate-600">{t('feeStructuresHint')}</p>
      {access.canManage && <form onSubmit={submitStructure} className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-7">
        <label className={ui.label}>{t('feeSession')}<select required disabled={!!structureId} className={feeField} value={structure.session_id} onChange={e => setStructure({ ...structure, session_id: e.target.value })}><option value="">{t('feeChoose')}</option>{options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeClass')}<select required disabled={!!structureId} className={feeField} value={structure.class_id} onChange={e => setStructure({ ...structure, class_id: e.target.value })}><option value="">{t('feeChoose')}</option>{options.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeCategory')}<select required disabled={!!structureId} className={feeField} value={structure.category_id} onChange={e => setStructure({ ...structure, category_id: e.target.value })}><option value="">{t('feeChoose')}</option>{allCategories.filter(c => c.is_active || c.id === structure.category_id).map(c => <option key={c.id} value={c.id}>{name(c.name_en, c.name_bn)}</option>)}</select></label>
        <label className={ui.label}>{t('feeAmount')}<input required type="number" min="0.01" step="0.01" className={feeField} value={structure.amount || ''} onChange={e => setStructure({ ...structure, amount: Number(e.target.value) })} /></label>
        <label className={ui.label}>{t('feeDueDay')}<input required type="number" min="1" max="28" className={feeField} value={structure.due_day} onChange={e => setStructure({ ...structure, due_day: Number(e.target.value) })} /></label>
        <label className="flex items-center gap-2 self-end pb-2 text-sm"><input type="checkbox" checked={structure.is_active} onChange={e => setStructure({ ...structure, is_active: e.target.checked })} />{t('feeActive')}</label>
        <div className="flex items-end gap-2"><button disabled={action.busy} className={feeButton}>{structureId ? t('feeUpdate') : t('feeAdd')}</button>{structureId && <button type="button" className={feeSecondary} onClick={() => { setStructure(blankStructure); setStructureId('') }}>{t('feeCancel')}</button>}</div>
      </form>}
      <label className="mt-4 block max-w-xs text-sm font-semibold">{t('feeSession')}<select className={feeField} value={structureSession} onChange={e => setStructureSession(e.target.value)}><option value="">{t('feeAll')}</option>{options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
      {structures.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : !structures.data?.items.length ? <p className="mt-3 text-slate-600">{t('feeNoStructures')}</p> :
        <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeSession')}</th><th className="px-3 py-2.5">{t('feeClass')}</th><th className="px-3 py-2.5">{t('feeCategory')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeAmount')}</th><th className="px-3 py-2.5">{t('feeDueDay')}</th><th className="px-3 py-2.5">{t('feeStatus')}</th>{access.canManage && <th className="px-3 py-2.5"><span className="sr-only">{t('feeActions')}</span></th>}</tr></thead>
          <tbody>{structures.data.items.map(s => <tr key={s.id} className="border-b border-slate-100"><td className="px-3 py-2.5">{s.session_name}</td><td className="px-3 py-2.5">{s.class_name}</td><td className="px-3 py-2.5">{name(s.category_name_en, s.category_name_bn)} <span className="text-xs text-slate-500">({t(`feeFrequency_${s.frequency}`)})</span></td><td className="px-3 py-2.5 text-right tabular-nums">{money(s.amount)}</td><td className="px-3 py-2.5">{s.due_day}</td><td className="px-3 py-2.5">{t(s.is_active ? 'feeActive' : 'feeInactive')}</td>
            {access.canManage && <td className="px-3 py-2.5"><button type="button" className={ui.link} onClick={() => { setStructureId(s.id); setStructure({ session_id: s.session_id, class_id: s.class_id, category_id: s.category_id, amount: s.amount, due_day: s.due_day, is_active: s.is_active }) }}>{t('feeEdit')}</button></td>}</tr>)}</tbody></TableWrap>}
    </section>

    <section className={feeCard}><h2 className={ui.sectionTitle}>{t('feeConcessions')}</h2><p className="mt-1 text-sm text-slate-600">{t('feeConcessionsHint')}</p>
      {access.canManage && <div className="mt-3 space-y-3">
        {concessionStudent ? <p className="text-sm">{t('feeSelectedStudent')}: <strong>{name(concessionStudent.name_en, concessionStudent.name_bn)}</strong> · {concessionStudent.admission_no} <button type="button" className="ml-2 underline" onClick={() => setConcessionStudent(null)}>{t('feeChange')}</button></p> : <StudentPicker onPick={setConcessionStudent} />}
        {concessionStudent && <form onSubmit={submitConcession} className="grid gap-3 sm:grid-cols-4">
          <label className={ui.label}>{t('feeCategory')}<select required className={feeField} value={concession.category_id} onChange={e => setConcession({ ...concession, category_id: e.target.value })}><option value="">{t('feeChoose')}</option>{allCategories.filter(c => c.is_active).map(c => <option key={c.id} value={c.id}>{name(c.name_en, c.name_bn)}</option>)}</select></label>
          <label className={ui.label}>{t('feePercent')}<input required type="number" min="0.01" max="100" step="0.01" className={feeField} value={concession.percent} onChange={e => setConcession({ ...concession, percent: e.target.value })} /></label>
          <label className={ui.label}>{t('feeReason')}<input required maxLength={240} className={feeField} value={concession.reason} onChange={e => setConcession({ ...concession, reason: e.target.value })} /></label>
          <div className="flex items-end"><button disabled={action.busy} className={`${feeButton} w-full`}>{t('feeSave')}</button></div>
        </form>}
      </div>}
      {!concessions.data?.items.length ? <p className="mt-3 text-slate-600">{t('feeNoConcessions')}</p> :
        <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeStudent')}</th><th className="px-3 py-2.5">{t('feeCategory')}</th><th className="px-3 py-2.5">{t('feePercent')}</th><th className="px-3 py-2.5">{t('feeReason')}</th>{access.canManage && <th className="px-3 py-2.5"><span className="sr-only">{t('feeActions')}</span></th>}</tr></thead>
          <tbody>{concessions.data.items.map(c => <tr key={c.id} className="border-b border-slate-100"><td className="px-3 py-2.5">{c.student_name} · {c.admission_no}</td><td className="px-3 py-2.5">{name(c.category_name_en, c.category_name_bn)}</td><td className="px-3 py-2.5">{c.percent}%</td><td className="px-3 py-2.5">{c.reason}</td>
            {access.canManage && <td className="px-3 py-2.5"><button type="button" className="font-semibold text-rose-700 underline" onClick={() => void action.run(() => authorized(token => deleteFeeConcession(token, c.id)), t('feeDeleted'))}>{t('feeDelete')}</button></td>}</tr>)}</tbody></TableWrap>}
    </section>
  </div>
}

function Breakdown({ title, buckets, label }: { title: string; buckets: FeeBucket[]; label: (bucket: FeeBucket) => string }) {
  const { t } = useTranslation()
  const money = useMoney()
  const max = Math.max(1, ...buckets.map(b => b.total))
  return <div><h3 className="font-bold">{title}</h3>{buckets.length === 0 ? <p className="mt-2 text-sm text-slate-600">{t('feeNoData')}</p> :
    <ul className="mt-2 space-y-2">{buckets.map(b => <li key={b.key} className="text-sm"><div className="flex justify-between gap-2"><span className="min-w-0 truncate">{label(b)}</span><span className="whitespace-nowrap font-semibold">{money(b.total)} <span className="font-normal text-slate-500">({b.count})</span></span></div>
      <div className="mt-1 h-2 rounded bg-slate-100" aria-hidden="true"><div className="h-2 rounded bg-[var(--brand-primary)]" style={{ width: `${(b.total / max) * 100}%` }} /></div></li>)}</ul>}</div>
}

export function FeeReportsPanel({ options }: { options: FeeOptions }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const name = useLocalName()
  const money = useMoney()
  const action = useFeeAction()
  const [range, setRange] = useState({ from: monthStart(), to: todayInDhaka(), class_id: '', method: '' })
  const [dueFilters, setDueFilters] = useState({ session_id: options.sessions.find(s => s.is_active)?.id ?? '', class_id: '', overdue_only: 'true', search: '' })
  const [duePage, setDuePage] = useState(1)
  const [paymentPage, setPaymentPage] = useState(1)
  const [paymentSearch, setPaymentSearch] = useState('')
  const [receipt, setReceipt] = useState<FeeReceipt | null>(null)
  const clean = (values: Record<string, string>) => Object.fromEntries(Object.entries(values).filter(([, v]) => v))
  const collectionParams = new URLSearchParams(clean(range))
  const collections = useQuery({ queryKey: ['fees', 'collections', collectionParams.toString()], queryFn: () => authorized(token => getFeeCollections(token, collectionParams)), retry: false })
  const dueParams = new URLSearchParams({ page: String(duePage), page_size: '20', ...clean(dueFilters) })
  const dueReport = useQuery({ queryKey: ['fees', 'due-report', dueParams.toString()], queryFn: () => authorized(token => getFeeDueReport(token, dueParams)), retry: false })
  const paymentParams = new URLSearchParams({ page: String(paymentPage), page_size: '20', ...clean({ from: range.from, to: range.to, method: range.method, class_id: range.class_id, search: paymentSearch }) })
  const payments = useQuery({ queryKey: ['fees', 'payments', paymentParams.toString()], queryFn: () => authorized(token => getFeePayments(token, paymentParams)), retry: false })
  async function open(id: string) {
    const value = await action.run(() => authorized(token => getFeeReceipt(token, `/fees/payments/${encodeURIComponent(id)}`)), '')
    if (value) setReceipt(value)
  }

  return <div className="space-y-5">
    <Notice error={action.error} message="" />
    {receipt && <ReceiptView receipt={receipt} onClose={() => setReceipt(null)} />}
    <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeCollectionReport')}</h2>
      <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <label className={ui.label}>{t('feeFrom')}<input type="date" className={feeField} value={range.from} onChange={e => { setRange({ ...range, from: e.target.value }); setPaymentPage(1) }} /></label>
        <label className={ui.label}>{t('feeTo')}<input type="date" className={feeField} value={range.to} onChange={e => { setRange({ ...range, to: e.target.value }); setPaymentPage(1) }} /></label>
        <label className={ui.label}>{t('feeClass')}<select className={feeField} value={range.class_id} onChange={e => { setRange({ ...range, class_id: e.target.value }); setPaymentPage(1) }}><option value="">{t('feeAllClasses')}</option>{options.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeMethod')}<select className={feeField} value={range.method} onChange={e => { setRange({ ...range, method: e.target.value }); setPaymentPage(1) }}><option value="">{t('feeAll')}</option>{['cash', 'bank', 'bkash', 'nagad'].map(m => <option key={m} value={m}>{t(`feeMethod_${m}`)}</option>)}</select></label>
      </div>
      {collections.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : collections.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrRange')}</p> : <>
        <p className="mt-4 text-lg">{t('feeCollected')}: <strong>{money(collections.data.total)}</strong> · {t('feeReceiptCount', { count: collections.data.count })}</p>
        <div className="mt-4 grid gap-6 md:grid-cols-2">
          <Breakdown title={t('feeByMethod')} buckets={collections.data.by_method} label={b => t(`feeMethod_${b.key}`)} />
          <Breakdown title={t('feeByCategory')} buckets={collections.data.by_category} label={b => name(b.name_en, b.name_bn)} />
          <Breakdown title={t('feeByClass')} buckets={collections.data.by_class} label={b => b.name_en} />
          <Breakdown title={t('feeByDay')} buckets={collections.data.by_day} label={b => b.key} />
        </div>
      </>}
    </section>

    <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeArrearsReport')}</h2>
      <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <label className={ui.label}>{t('feeSession')}<select className={feeField} value={dueFilters.session_id} onChange={e => { setDueFilters({ ...dueFilters, session_id: e.target.value }); setDuePage(1) }}><option value="">{t('feeAll')}</option>{options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeClass')}<select className={feeField} value={dueFilters.class_id} onChange={e => { setDueFilters({ ...dueFilters, class_id: e.target.value }); setDuePage(1) }}><option value="">{t('feeAllClasses')}</option>{options.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('feeSearch')}<input className={feeField} value={dueFilters.search} onChange={e => { setDueFilters({ ...dueFilters, search: e.target.value }); setDuePage(1) }} /></label>
        <label className="flex items-center gap-2 self-end pb-2 text-sm"><input type="checkbox" checked={dueFilters.overdue_only === 'true'} onChange={e => { setDueFilters({ ...dueFilters, overdue_only: e.target.checked ? 'true' : '' }); setDuePage(1) }} />{t('feeOverdueOnly')}</label>
      </div>
      {dueReport.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : dueReport.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : <>
        <p className="mt-3">{t('feeOutstanding')}: <strong>{money(dueReport.data.total_balance)}</strong> · {t('feeArrears')}: <strong className="text-rose-700">{money(dueReport.data.total_overdue)}</strong></p>
        {dueReport.data.items.length === 0 ? <p className="mt-3 text-slate-600">{t('feeNoArrears')}</p> : <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeStudent')}</th><th className="px-3 py-2.5">{t('feeClassSection')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeCharged')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feePaid')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeBalance')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeArrears')}</th><th className="px-3 py-2.5">{t('feeOldestDue')}</th></tr></thead>
          <tbody>{dueReport.data.items.map(r => <tr key={r.student_id} className="border-b border-slate-100"><td className="px-3 py-2.5"><strong>{name(r.name_en, r.name_bn)}</strong><span className="block text-xs text-slate-600">{r.admission_no}</span></td><td className="px-3 py-2.5">{r.class_name} / {r.section_name}</td><td className="px-3 py-2.5 text-right tabular-nums">{money(r.charged)}</td><td className="px-3 py-2.5 text-right tabular-nums">{money(r.paid)}</td><td className="px-3 py-2.5 text-right tabular-nums">{money(r.balance)}</td><td className="p-2 text-right font-semibold text-rose-700">{money(r.overdue)}</td><td className="px-3 py-2.5">{r.oldest_due ?? '—'}</td></tr>)}</tbody></TableWrap>}
        <Pager page={duePage} total={dueReport.data.total} size={20} onPage={setDuePage} />
      </>}
    </section>

    <section className={`${feeCard} print:hidden`}><h2 className={ui.sectionTitle}>{t('feeReceiptRegister')}</h2>
      <label className="mt-3 block max-w-sm text-sm font-semibold">{t('feeSearchReceipts')}<input className={feeField} value={paymentSearch} onChange={e => { setPaymentSearch(e.target.value); setPaymentPage(1) }} /></label>
      {payments.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : payments.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrGeneric')}</p> : payments.data.items.length === 0 ? <p className="mt-3 text-slate-600">{t('feeNoPayments')}</p> : <>
        <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('feeReceiptNo')}</th><th className="px-3 py-2.5">{t('feePaidOn')}</th><th className="px-3 py-2.5">{t('feeStudent')}</th><th className="px-3 py-2.5">{t('feeMethod')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeAmount')}</th><th className="px-3 py-2.5"><span className="sr-only">{t('feeActions')}</span></th></tr></thead>
          <tbody>{payments.data.items.map(p => <tr key={p.id} className="border-b border-slate-100"><td className="p-2 font-mono">{p.receipt_no}</td><td className="px-3 py-2.5">{p.paid_on}</td><td className="px-3 py-2.5">{name(p.student_name, p.student_name_bn)}<span className="block text-xs text-slate-600">{p.admission_no} · {p.class_name}</span></td><td className="px-3 py-2.5">{t(`feeMethod_${p.method}`)}</td><td className="px-3 py-2.5 text-right tabular-nums">{money(p.amount)}</td>
            <td className="px-3 py-2.5"><button type="button" className={ui.link} onClick={() => void open(p.id)}>{t('feeViewReceipt')}</button></td></tr>)}</tbody></TableWrap>
        <Pager page={paymentPage} total={payments.data.total} size={20} onPage={setPaymentPage} />
      </>}
    </section>
  </div>
}

export function LedgerPanel({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const money = useMoney()
  const action = useFeeAction()
  const [range, setRange] = useState({ from: monthStart(), to: todayInDhaka() })
  const [type, setType] = useState('')
  const [page, setPage] = useState(1)
  const [entry, setEntry] = useState<LedgerEntryInput>({ entry_type: 'expense', category: 'utilities', amount: 0, entry_date: todayInDhaka(), description: '', reference: '' })
  const rangeParams = new URLSearchParams(range)
  const summary = useQuery({ queryKey: ['fees', 'summary', rangeParams.toString()], queryFn: () => authorized(token => getAccountingSummary(token, rangeParams)), retry: false })
  const entryParams = new URLSearchParams({ ...range, page: String(page), page_size: '20', ...(type ? { type } : {}) })
  const entries = useQuery({ queryKey: ['fees', 'entries', entryParams.toString()], queryFn: () => authorized(token => getLedgerEntries(token, entryParams)), retry: false })
  async function submit(event: FormEvent) {
    event.preventDefault()
    const saved = await action.run(() => authorized(token => saveLedgerEntry(token, { ...entry, amount: Number(entry.amount), description: entry.description.trim(), reference: entry.reference.trim() })), t('feeSaved'))
    if (saved) setEntry({ ...entry, amount: 0, description: '', reference: '' })
  }
  const categoryLabel = (key: string) => t(`ledger_${key}`)

  return <div className="space-y-5">
    <Notice error={action.error} message={action.message} />
    <section className={feeCard}><h2 className={ui.sectionTitle}>{t('ledgerSummary')}</h2>
      <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <label className={ui.label}>{t('feeFrom')}<input type="date" className={feeField} value={range.from} onChange={e => { setRange({ ...range, from: e.target.value }); setPage(1) }} /></label>
        <label className={ui.label}>{t('feeTo')}<input type="date" className={feeField} value={range.to} onChange={e => { setRange({ ...range, to: e.target.value }); setPage(1) }} /></label>
      </div>
      {summary.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : summary.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrRange')}</p> : <>
        <dl className="mt-4 grid grid-cols-2 gap-3 lg:grid-cols-5">{([['ledgerIncome', summary.data.income, 'text-emerald-800'], ['ledgerExpense', summary.data.expense, 'text-rose-700'], ['ledgerNet', summary.data.net, summary.data.net < 0 ? 'text-rose-700' : 'text-slate-900'], ['feeOutstanding', summary.data.outstanding, ''], ['feeArrears', summary.data.arrears, 'text-rose-700']] as [string, number, string][]).map(([key, value, tone]) =>
          <div key={key} className="rounded-xl bg-slate-50 p-3"><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t(key)}</dt><dd className={`mt-1 text-lg font-bold ${tone}`}>{money(value)}</dd></div>)}</dl>
        <p className="mt-2 text-xs text-slate-500">{t('ledgerOutstandingNote')}</p>
        <div className="mt-4 grid gap-6 md:grid-cols-2">
          <Breakdown title={t('ledgerIncomeByCategory')} buckets={summary.data.income_by_category} label={b => categoryLabel(b.key)} />
          <Breakdown title={t('ledgerExpenseByCategory')} buckets={summary.data.expense_by_category} label={b => categoryLabel(b.key)} />
        </div>
      </>}
    </section>
    {canManage && <section className={feeCard}><h2 className={ui.sectionTitle}>{t('ledgerAdd')}</h2><p className="mt-1 text-sm text-slate-600">{t('ledgerAddHint')}</p>
      <form onSubmit={submit} className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
        <label className={ui.label}>{t('ledgerType')}<select className={feeField} value={entry.entry_type} onChange={e => { const kind = e.target.value as LedgerEntryInput['entry_type']; setEntry({ ...entry, entry_type: kind, category: ledgerCategories[kind][0] }) }}><option value="expense">{t('ledgerExpense')}</option><option value="income">{t('ledgerIncome')}</option></select></label>
        <label className={ui.label}>{t('feeCategory')}<select className={feeField} value={entry.category} onChange={e => setEntry({ ...entry, category: e.target.value })}>{ledgerCategories[entry.entry_type].map(c => <option key={c} value={c}>{categoryLabel(c)}</option>)}</select></label>
        <label className={ui.label}>{t('feeAmount')}<input required type="number" min="0.01" step="0.01" className={feeField} value={entry.amount || ''} onChange={e => setEntry({ ...entry, amount: Number(e.target.value) })} /></label>
        <label className={ui.label}>{t('ledgerDate')}<input required type="date" max={todayInDhaka()} className={feeField} value={entry.entry_date} onChange={e => setEntry({ ...entry, entry_date: e.target.value })} /></label>
        <label className={ui.label}>{t('ledgerDescription')}<input required maxLength={240} className={feeField} value={entry.description} onChange={e => setEntry({ ...entry, description: e.target.value })} /></label>
        <label className={ui.label}>{t('feeReference')}<input maxLength={80} className={feeField} value={entry.reference} onChange={e => setEntry({ ...entry, reference: e.target.value })} /></label>
        <div className="sm:col-span-2 lg:col-span-6"><button disabled={action.busy} className={feeButton}>{t('feeAdd')}</button></div>
      </form>
    </section>}
    <section className={feeCard}><h2 className={ui.sectionTitle}>{t('ledgerEntries')}</h2>
      <label className="mt-3 block max-w-xs text-sm font-semibold">{t('ledgerType')}<select className={feeField} value={type} onChange={e => { setType(e.target.value); setPage(1) }}><option value="">{t('feeAll')}</option><option value="income">{t('ledgerIncome')}</option><option value="expense">{t('ledgerExpense')}</option></select></label>
      {entries.isPending ? <p role="status" className="mt-3">{t('feeLoading')}</p> : entries.isError ? <p role="alert" className="mt-3 text-rose-700">{t('feeErrRange')}</p> : entries.data.items.length === 0 ? <p className="mt-3 text-slate-600">{t('ledgerNoEntries')}</p> : <>
        <TableWrap><thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th className="px-3 py-2.5">{t('ledgerDate')}</th><th className="px-3 py-2.5">{t('ledgerType')}</th><th className="px-3 py-2.5">{t('feeCategory')}</th><th className="px-3 py-2.5">{t('ledgerDescription')}</th><th className="px-3 py-2.5 text-right tabular-nums">{t('feeAmount')}</th><th className="px-3 py-2.5">{t('ledgerBy')}</th>{canManage && <th className="px-3 py-2.5"><span className="sr-only">{t('feeActions')}</span></th>}</tr></thead>
          <tbody>{entries.data.items.map(e => <tr key={e.id} className="border-b border-slate-100"><td className="whitespace-nowrap p-2">{e.entry_date}</td><td className="px-3 py-2.5">{t(e.entry_type === 'income' ? 'ledgerIncome' : 'ledgerExpense')}</td><td className="px-3 py-2.5">{categoryLabel(e.category)}</td><td className="px-3 py-2.5">{e.description}{e.reference && <span className="block text-xs text-slate-500">{e.reference}</span>}</td>
            <td className={`p-2 text-right font-semibold ${e.entry_type === 'expense' ? 'text-rose-700' : 'text-emerald-800'}`}>{e.entry_type === 'expense' ? '−' : '+'}{money(e.amount)}</td><td className="px-3 py-2.5">{e.created_by_name}</td>
            {canManage && <td className="px-3 py-2.5">{e.payment_id ? <span className="text-xs text-slate-500">{t('ledgerFromReceipt')}</span> : <button type="button" className="font-semibold text-rose-700 underline" onClick={async () => { if ((await confirmAction(t('ledgerConfirmDelete')))) void action.run(() => authorized(token => deleteLedgerEntry(token, e.id)), t('feeDeleted')) }}>{t('feeDelete')}</button>}</td>}</tr>)}</tbody></TableWrap>
        <Pager page={page} total={entries.data.total} size={20} onPage={setPage} />
      </>}
    </section>
  </div>
}
