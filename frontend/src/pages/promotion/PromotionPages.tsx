import { useMemo, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, Route, Routes, useNavigate } from 'react-router'
import { ApiError, getAcademicClasses, getAcademicSessions, type AcademicSession } from '../../api/client'
import { createBatch, getBatches, getConcessionReview, getProgression, setProgression, type ClassProgression, type PromotionBatch } from '../../api/promotions'
import { Icon } from '../../components/icons'
import { StateScreen } from '../../components/StateScreen'
import { EmptyState, Notice, SectionCard, Skeleton, ui } from '../attendanceUi'
import { BatchPage } from './BatchPage'
import { PromotionFrame, StatusPill, useAuthorized, useDateTime, useErrorText, useNum, type Caps } from './promotionShared'

// Year-end promotion screens (M15 stage B2): class progression, batches,
// batch review and approval, apply and revert, and the concession review.
// Administrators and the head teacher only; the server enforces every rule.
export function PromotionGate({ permissions, role, enabled, feesEnabled, loading, error }: {
  permissions: string[] | undefined; role: string | undefined; enabled: boolean; feesEnabled: boolean; loading: boolean; error: boolean
}) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  const has = (p: string) => !!permissions?.includes(p)
  const admin = role === 'admin'
  if (!has('promotions:read') || !(admin || role === 'head_teacher')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" message={t('pmModuleOff')} />
  const caps: Caps = {
    manage: admin && has('promotions:manage'),
    approve: role === 'head_teacher' && has('promotions:approve'),
    apply: admin && has('promotions:apply'),
    fees: admin && has('fees:read') && feesEnabled,
  }
  return <Routes>
    <Route index element={<BatchesPage caps={caps} />} />
    <Route path="progression" element={<ProgressionPage caps={caps} />} />
    <Route path="concessions" element={caps.fees ? <ConcessionsPage caps={caps} /> : <StateScreen kind="denied" message={t('permissionDenied')} />} />
    <Route path=":id" element={<BatchPage caps={caps} />} />
  </Routes>
}

const all = new URLSearchParams({ page: '1', page_size: '100' })

function useSessions() {
  const authorized = useAuthorized()
  return useQuery({ queryKey: ['pm-sessions'], retry: false, queryFn: () => authorized(token => getAcademicSessions(token, all)) })
}
function useProgression() {
  const authorized = useAuthorized()
  return useQuery({ queryKey: ['pm-progression'], retry: false, queryFn: () => authorized(getProgression) })
}
// defaultSessions: this year (the active session) and the one after it.
function defaultSessions(sessions: AcademicSession[] | undefined) {
  const sorted = [...(sessions ?? [])].sort((a, b) => a.start_date.localeCompare(b.start_date))
  const from = sorted.find(s => s.is_active) ?? sorted[sorted.length - 2] ?? sorted[0]
  const to = from ? sorted.find(s => s.start_date > from.start_date) : undefined
  return { from: from?.id ?? '', to: to?.id ?? '', sorted }
}

// ---------- Class progression ----------

function ProgressionPage({ caps }: { caps: Caps }) {
  const { t } = useTranslation()
  const num = useNum()
  const progression = useProgression()
  const rows = useMemo(() => [...(progression.data ?? [])].sort((a, b) => a.display_order - b.display_order || a.class_name.localeCompare(b.class_name)), [progression.data])
  const unset = rows.filter(r => r.is_active && r.setting === 'unset').length
  return <PromotionFrame caps={caps} title={t('pmProgressionTitle')} description={t(caps.manage ? 'pmProgressionIntro' : 'pmProgressionIntroRead')}>
    {progression.isPending ? <Skeleton rows={4} /> : progression.isError ? <Notice tone="error">{t('pmLoadFailed')}</Notice> : <>
      {unset > 0 ? <Notice tone="warning"><span className="font-semibold">{t('pmUnsetCount', { count: unset, n: num(unset) })}</span> {t('pmUnsetHint')}</Notice>
        : <Notice tone="success">{t('pmAllSet')}</Notice>}
      <ul className="grid gap-3 lg:grid-cols-2">
        {rows.map(row => <ProgressionRow key={row.class_id + row.setting + (row.next_class_id ?? '')} row={row} rows={rows} canEdit={caps.manage} />)}
      </ul>
    </>}
  </PromotionFrame>
}

function ProgressionRow({ row, rows, canEdit }: { row: ClassProgression; rows: ClassProgression[]; canEdit: boolean }) {
  const { t } = useTranslation()
  const authorized = useAuthorized()
  const errorText = useErrorText()
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<'next' | 'final' | ''>(row.setting === 'unset' ? '' : row.setting)
  const [next, setNext] = useState(row.next_class_id ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const dirty = mode !== (row.setting === 'unset' ? '' : row.setting) || (mode === 'next' && next !== (row.next_class_id ?? ''))
  const valid = mode === 'final' || (mode === 'next' && next !== '')
  const others = rows.filter(r => r.class_id !== row.class_id && (r.is_active || r.class_id === row.next_class_id))

  async function save(input: { next_class_id: string | null } | { final: true }) {
    setBusy(true); setError('')
    try {
      const data = await authorized(token => setProgression(token, row.class_id, input))
      queryClient.setQueryData(['pm-progression'], data)
      void queryClient.invalidateQueries({ queryKey: ['pm-batch'] })
    } catch (cause) { setError(errorText(cause)) } finally { setBusy(false) }
  }
  function submit(e: FormEvent) {
    e.preventDefault()
    if (!valid) return
    void save(mode === 'final' ? { final: true } : { next_class_id: next })
  }
  const groupName = `pm-prog-${row.class_id}`
  const badge = row.setting === 'next'
    ? <span className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-semibold text-emerald-800 ring-1 ring-inset ring-emerald-600/25"><Icon name="arrowRight" className="h-3 w-3" />{row.next_class_name}</span>
    : row.setting === 'final'
      ? <span className="inline-flex items-center gap-1 rounded-full bg-violet-50 px-2.5 py-0.5 text-xs font-semibold text-violet-900 ring-1 ring-inset ring-violet-600/25"><Icon name="grade" className="h-3 w-3" />{t('pmFinalClass')}</span>
      : <span className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2.5 py-0.5 text-xs font-semibold text-amber-900 ring-1 ring-inset ring-amber-600/30"><Icon name="alert" className="h-3 w-3" />{t('pmNotSet')}</span>
  return <li className={`${ui.card} p-4 ${row.setting === 'unset' && row.is_active ? 'border-amber-300' : ''}`} aria-label={row.class_name}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h2 className="min-w-0 text-base font-bold text-slate-900">{row.class_name}{!row.is_active && <span className="ml-2 text-xs font-medium text-slate-500">{t('pmInactive')}</span>}</h2>
      {badge}
    </div>
    {row.setting === 'unset' && row.suggested_next_class_name && <p className="mt-2 text-sm text-slate-600">
      {t('pmSuggested', { name: row.suggested_next_class_name })}{' '}
      {canEdit && <button type="button" className={ui.link} onClick={() => { setMode('next'); setNext(row.suggested_next_class_id ?? '') }}>{t('pmUseSuggestion')}</button>}
    </p>}
    {canEdit && <form onSubmit={submit} className="mt-3 space-y-3">
      <fieldset className="space-y-2">
        <legend className="sr-only">{t('pmProgressionFor', { name: row.class_name })}</legend>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <label className="inline-flex min-h-10 items-center gap-2 text-sm font-medium text-slate-800">
            <input type="radio" name={groupName} checked={mode === 'next'} onChange={() => setMode('next')} className="h-4 w-4 accent-[var(--brand-primary)]" />{t('pmPromotesTo')}</label>
          <select aria-label={t('pmNextClassFor', { name: row.class_name })} value={next} disabled={mode !== 'next'}
            onChange={e => setNext(e.target.value)} className={`${ui.field} mt-0 w-full sm:w-56`}>
            <option value="">{t('pmChooseClass')}</option>
            {others.map(o => <option key={o.class_id} value={o.class_id}>{o.class_name}</option>)}
          </select>
        </div>
        <label className="flex min-h-10 items-center gap-2 text-sm font-medium text-slate-800">
          <input type="radio" name={groupName} checked={mode === 'final'} onChange={() => setMode('final')} className="h-4 w-4 accent-[var(--brand-primary)]" />{t('pmFinalOption')}</label>
      </fieldset>
      {dirty && <p className="text-xs font-semibold text-amber-800">{t('pmUnsaved')}</p>}
      {error && <p role="alert" className="text-sm text-rose-700">{error}</p>}
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={busy || !dirty || !valid} className={ui.primary}>{t('pmSave')}</button>
        {row.setting !== 'unset' && <button type="button" disabled={busy} className={ui.secondary} onClick={() => void save({ next_class_id: null })}>{t('pmClear')}</button>}
      </div>
    </form>}
  </li>
}

// ---------- Batches ----------

function BatchesPage({ caps }: { caps: Caps }) {
  const { t } = useTranslation()
  const authorized = useAuthorized()
  const when = useDateTime()
  const batches = useQuery({ queryKey: ['pm-batches'], retry: false, queryFn: () => authorized(getBatches) })
  const [filter, setFilter] = useState<'open' | 'applied' | 'all'>('open')
  const shown = (batches.data ?? []).filter(b => filter === 'all' || (filter === 'open' ? b.status === 'draft' || b.status === 'approved' : b.status === 'applied'))
  const filters: { value: typeof filter; label: string }[] = [{ value: 'open', label: t('pmFilterOpen') }, { value: 'applied', label: t('pmFilterApplied') }, { value: 'all', label: t('pmFilterAll') }]
  return <PromotionFrame caps={caps} title={t('pmBatchesTitle')} description={t(caps.manage ? 'pmBatchesIntro' : caps.approve ? 'pmBatchesIntroHead' : 'pmBatchesIntroRead')}>
    {caps.manage && <CreateBatch batches={batches.data ?? []} />}
    <SectionCard id="pm-batch-list" title={t('pmBatchList')} actions={<div role="group" aria-label={t('pmFilterLabel')} className="flex gap-1 rounded-xl bg-slate-100 p-1">
      {filters.map(f => <button key={f.value} type="button" aria-pressed={filter === f.value} onClick={() => setFilter(f.value)}
        className={`min-h-11 rounded-lg px-3 py-1.5 text-sm font-semibold sm:min-h-0 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${filter === f.value ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'}`}>{f.label}</button>)}
    </div>}>
      {batches.isPending ? <Skeleton rows={3} /> : batches.isError ? <Notice tone="error">{t('pmLoadFailed')}</Notice>
        : shown.length === 0 ? <EmptyState icon="layers" title={t('pmNoBatches')}>{t(caps.manage ? 'pmNoBatchesHint' : 'pmNoBatchesHintRead')}</EmptyState>
          : <ul className="divide-y divide-slate-100">{shown.map(b => <li key={b.id}>
            <Link to={`/admin/promotions/${b.id}`} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-xl px-2 py-3 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
              <span className="min-w-0">
                <span className="block font-semibold text-slate-900">{b.class_name}</span>
                <span className="block text-sm text-slate-600">{t('pmFromTo', { from: b.from_session_name, to: b.to_session_name })}</span>
                <span className="block text-xs text-slate-500">{t('pmCreatedBy', { name: b.created_by, when: when(b.created_at) })}</span>
              </span>
              <span className="flex items-center gap-2"><StatusPill status={b.status} /><Icon name="chevronRight" className="h-4 w-4 text-slate-400" /></span>
            </Link></li>)}</ul>}
    </SectionCard>
  </PromotionFrame>
}

function CreateBatch({ batches }: { batches: PromotionBatch[] }) {
  const { t } = useTranslation()
  const authorized = useAuthorized()
  const errorText = useErrorText()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const sessions = useSessions()
  const classes = useQuery({ queryKey: ['pm-classes'], retry: false, queryFn: () => authorized(token => getAcademicClasses(token, all)) })
  const progression = useProgression()
  const defaults = defaultSessions(sessions.data?.items)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [classId, setClassId] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<{ text: string; code: string } | null>(null)
  const fromId = from || defaults.from, toId = to || defaults.to
  const setting = new Map((progression.data ?? []).map(p => [p.class_id, p.setting]))
  const classList = [...(classes.data?.items ?? [])].filter(c => c.is_active).sort((a, b) => a.display_order - b.display_order)
  const chosenUnset = classId !== '' && setting.get(classId) === 'unset'
  const open = batches.find(b => b.from_session_id === fromId && b.class_id === classId && (b.status === 'draft' || b.status === 'approved'))

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!fromId || !toId || !classId || fromId === toId) return
    setBusy(true); setError(null)
    try {
      const b = await authorized(token => createBatch(token, { from_session_id: fromId, to_session_id: toId, class_id: classId }))
      void queryClient.invalidateQueries({ queryKey: ['pm-batches'] })
      navigate(`/admin/promotions/${b.id}`)
    } catch (cause) {
      setError({ text: errorText(cause), code: cause instanceof ApiError ? cause.code : '' })
    } finally { setBusy(false) }
  }
  return <SectionCard id="pm-create" title={t('pmCreateTitle')} description={t('pmCreateIntro')}>
    {sessions.isPending || classes.isPending ? <Skeleton rows={1} /> : sessions.isError || classes.isError ? <Notice tone="error">{t('pmLoadFailed')}</Notice> :
      <form onSubmit={submit} className="grid gap-4 sm:grid-cols-3">
        <label className={ui.label}>{t('pmFromSession')}
          <select className={ui.field} required value={fromId} onChange={e => setFrom(e.target.value)}>
            {defaults.sorted.map(s => <option key={s.id} value={s.id}>{s.name}{s.is_active ? ` · ${t('pmActive')}` : ''}</option>)}
          </select></label>
        <label className={ui.label}>{t('pmClass')}
          <select className={ui.field} required value={classId} onChange={e => setClassId(e.target.value)} aria-describedby={chosenUnset ? 'pm-create-unset' : undefined}>
            <option value="">{t('pmChooseClass')}</option>
            {classList.map(c => <option key={c.id} value={c.id}>{c.name}{setting.get(c.id) === 'unset' ? ` · ${t('pmNotSet')}` : ''}</option>)}
          </select></label>
        <label className={ui.label}>{t('pmToSession')}
          <select className={ui.field} required value={toId} onChange={e => setTo(e.target.value)}>
            <option value="">{t('pmChooseSession')}</option>
            {defaults.sorted.filter(s => s.id !== fromId).map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select></label>
        {chosenUnset && <p id="pm-create-unset" className="text-sm text-amber-800 sm:col-span-3"><Icon name="alert" className="mr-1 inline h-4 w-4" />{t('pmCreateUnset')} <Link className={ui.link} to="/admin/promotions/progression">{t('pmSetProgression')}</Link></p>}
        {open && <p className="text-sm text-slate-700 sm:col-span-3">{t('pmCreateOpen')} <Link className={ui.link} to={`/admin/promotions/${open.id}`}>{t('pmOpenBatch')}</Link></p>}
        {error && <div role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900 sm:col-span-3">{error.text}
          {error.code === 'progression_unset' && <> <Link className={ui.link} to="/admin/promotions/progression">{t('pmSetProgression')}</Link></>}</div>}
        <div className="sm:col-span-3"><button type="submit" disabled={busy || !classId || !toId || fromId === toId || !!open} className={ui.primary}><Icon name="layers" className="h-4 w-4" />{t('pmCreate')}</button></div>
      </form>}
  </SectionCard>
}

// ---------- Concession review ----------

function ConcessionsPage({ caps }: { caps: Caps }) {
  const { t, i18n } = useTranslation()
  const authorized = useAuthorized()
  const num = useNum()
  const sessions = useSessions()
  const defaults = defaultSessions(sessions.data?.items)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const fromId = from || defaults.from, toId = to || defaults.to
  const review = useQuery({
    queryKey: ['pm-concessions', fromId, toId], retry: false, enabled: !!fromId && !!toId && fromId !== toId,
    queryFn: () => authorized(token => getConcessionReview(token, fromId, toId)),
  })
  const rows = review.data ?? []
  const pending = rows.filter(r => r.to_percent === null).length
  const bn = i18n.language === 'bn'
  return <PromotionFrame caps={caps} title={t('pmConcessionsTitle')} description={t('pmConcessionsIntro')}>
    <SectionCard id="pm-conc" title={t('pmConcessionsList')} actions={<Link to="/accounts/fees" className={ui.secondary}><Icon name="money" className="h-4 w-4" />{t('pmOpenFees')}</Link>}>
      {sessions.isPending ? <Skeleton rows={1} /> : <div className="grid gap-4 sm:grid-cols-2">
        <label className={ui.label}>{t('pmFromSession')}<select className={ui.field} value={fromId} onChange={e => setFrom(e.target.value)}>
          {defaults.sorted.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('pmToSession')}<select className={ui.field} value={toId} onChange={e => setTo(e.target.value)}>
          <option value="">{t('pmChooseSession')}</option>
          {defaults.sorted.filter(s => s.id !== fromId).map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
      </div>}
      <div className="mt-5">
        {!toId || fromId === toId ? <EmptyState icon="calendar" title={t('pmChooseSessions')} />
          : review.isPending ? <Skeleton rows={3} /> : review.isError ? <Notice tone="error">{t('pmLoadFailed')}</Notice>
            : rows.length === 0 ? <EmptyState icon="money" title={t('pmNoConcessions')}>{t('pmNoConcessionsHint')}</EmptyState>
              : <>
                <p className="mb-3 text-sm text-slate-700" aria-live="polite">{t('pmConcessionsCount', { count: rows.length, n: num(rows.length), pending: num(pending) })}</p>
                <ul className="grid gap-3 md:hidden">{rows.map(r => <li key={r.id} className="rounded-xl border border-slate-200 p-3">
                  <p className="font-semibold text-slate-900">{r.student_name} <span className="text-xs font-normal text-slate-500">{r.admission_no}</span></p>
                  <p className="text-sm text-slate-600">{bn ? r.category_name_bn : r.category_name_en} · {t('pmPercent', { n: num(r.percent) })}{r.reason && ` · ${r.reason}`}</p>
                  <p className="mt-1 text-sm text-slate-600">{t('pmNowIn', { cls: r.to_class_name, section: r.to_section_name })}</p>
                  <p className="mt-1 text-sm">{r.to_percent === null ? <span className="font-semibold text-amber-800">{t('pmNewNotSet')}</span> : <span className="font-semibold text-emerald-700">{t('pmNewSet', { n: num(r.to_percent) })}</span>}</p>
                </li>)}</ul>
                <div className="hidden overflow-x-auto md:block"><table className={ui.table}>
                  <caption className="sr-only">{t('pmConcessionsList')}</caption>
                  <thead><tr><th scope="col" className={ui.th}>{t('pmStudent')}</th><th scope="col" className={ui.th}>{t('pmCategory')}</th><th scope="col" className={ui.th}>{t('pmOldConcession')}</th><th scope="col" className={ui.th}>{t('pmNewPlace')}</th><th scope="col" className={ui.th}>{t('pmNewConcession')}</th></tr></thead>
                  <tbody>{rows.map(r => <tr key={r.id}>
                    <td className={ui.td}><span className="font-semibold text-slate-900">{r.student_name}</span><span className="block text-xs text-slate-500">{r.admission_no}</span></td>
                    <td className={ui.td}>{bn ? r.category_name_bn : r.category_name_en}</td>
                    <td className={ui.td}>{t('pmPercent', { n: num(r.percent) })}{r.reason && <span className="block text-xs text-slate-500">{r.reason}</span>}</td>
                    <td className={ui.td}>{r.to_class_name} · {r.to_section_name}</td>
                    <td className={ui.td}>{r.to_percent === null ? <span className="font-semibold text-amber-800">{t('pmNewNotSet')}</span> : <span className="font-semibold text-emerald-700">{t('pmNewSet', { n: num(r.to_percent) })}</span>}</td>
                  </tr>)}</tbody></table></div>
              </>}
      </div>
    </SectionCard>
  </PromotionFrame>
}
