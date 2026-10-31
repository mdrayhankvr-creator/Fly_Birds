import { useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
import { ApiError, getAcademicSections } from '../../api/client'
import {
  applyBatch, approveBatch, arrangeRolls, discardBatch, getBatch, refreshBatch, revertBatch, updateItem,
  type Decision, type ItemInput, type PromotionBatch, type PromotionItem, type RollPolicy,
} from '../../api/promotions'
import { confirmAction } from '../../components/ConfirmDialog'
import { Icon } from '../../components/icons'
import { tone } from '../../components/modules'
import { StateScreen } from '../../components/StateScreen'
import { Notice, SectionCard, ui } from '../attendanceUi'
import {
  DECISIONS, DecisionPill, Dialog, OutcomePill, StatusPill, useAuthorized, useDateTime, useErrorText, useIssueText, useNum, type Caps,
} from './promotionShared'

type Filter = 'all' | 'undecided' | 'override' | 'issues'

export function BatchPage({ caps }: { caps: Caps }) {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const authorized = useAuthorized()
  const errorText = useErrorText()
  const queryClient = useQueryClient()
  const batch = useQuery({ queryKey: ['pm-batch', id], retry: false, queryFn: () => authorized(token => getBatch(token, id)) })
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [editing, setEditing] = useState<PromotionItem | null>(null)
  const [applying, setApplying] = useState(false)

  if (batch.isPending) return <StateScreen kind="loading" message={t('pmLoading')} />
  if (batch.isError) return batch.error instanceof ApiError && batch.error.status === 404 ? <StateScreen kind="notFound" message={t('pmBatchMissing')} /> : <StateScreen kind="error" message={t('pmLoadFailed')} />
  const b = batch.data

  // run performs one action, keeps the returned batch and reports the result.
  async function run(action: (token: string) => Promise<PromotionBatch>, done: string) {
    setBusy(true); setMessage(null)
    try {
      const next = await authorized(action)
      queryClient.setQueryData(['pm-batch', id], next)
      void queryClient.invalidateQueries({ queryKey: ['pm-batches'] })
      setMessage({ tone: 'success', text: done })
      return true
    } catch (cause) {
      setMessage({ tone: 'error', text: errorText(cause) })
      if (cause instanceof ApiError && cause.status === 409) void batch.refetch()
      return false
    } finally { setBusy(false) }
  }

  const open = b.status === 'draft' || b.status === 'approved'
  const editable = caps.manage && open
  return <div className="space-y-5" style={tone('people')}>
    <Link to="/admin/promotions" className={ui.ghost}><Icon name="chevronLeft" className="h-4 w-4" />{t('pmAllBatches')}</Link>
    <BatchHeader b={b} />
    {message && <Notice tone={message.tone}>{message.text}</Notice>}
    <Counts b={b} />
    <NextStep b={b} caps={caps} busy={busy} run={run} onApply={() => setApplying(true)} />
    <Issues b={b} />
    {editable && <Tools b={b} busy={busy} run={run} />}
    <Students b={b} editable={editable} onEdit={setEditing} />
    {editing && <EditItem b={b} item={editing} onClose={() => setEditing(null)}
      onSave={async input => { if (await run(token => updateItem(token, b.id, editing.student_id, input), t('pmSaved'))) setEditing(null) }} busy={busy} />}
    {applying && <ApplyDialog b={b} busy={busy} onClose={() => setApplying(false)}
      onConfirm={async () => { if (await run(token => applyBatch(token, b.id), t('pmApplied'))) setApplying(false) }} />}
  </div>
}

function BatchHeader({ b }: { b: PromotionBatch }) {
  const { t } = useTranslation()
  const steps: { key: string; label: string; who: string }[] = [
    { key: 'prepare', label: t('pmStepPrepare'), who: t('pmByAdmin') },
    { key: 'approve', label: t('pmStepApprove'), who: t('pmByHead') },
    { key: 'apply', label: t('pmStepApply'), who: t('pmByAdmin') },
  ]
  const current = b.status === 'draft' ? 0 : b.status === 'approved' ? 2 : 3
  const target = b.class_is_final ? t('pmGraduatesFrom') : b.next_class_name ? t('pmPromotesInto', { name: b.next_class_name }) : t('pmNoProgression')
  return <header className="ds-role-head p-4 sm:p-5" style={tone('people')}>
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <p className="text-xs font-semibold text-slate-600">{t('pmEyebrow')}</p>
        <h1 className="mt-0.5 text-[22px] font-extrabold leading-tight tracking-tight text-slate-900 sm:text-[26px]">{b.class_name}</h1>
        <p className="mt-1 text-sm text-slate-700">{t('pmFromTo', { from: b.from_session_name, to: b.to_session_name })} · {target}</p>
      </div>
      <StatusPill status={b.status} />
    </div>
    {b.status !== 'discarded' && b.status !== 'reverted' && <ol className="mt-4 grid grid-cols-3 gap-2" aria-label={t('pmSteps')}>
      {steps.map((s, i) => {
        const done = i < current, now = i === current
        return <li key={s.key} aria-current={now ? 'step' : undefined}
          className={`min-w-0 rounded-xl border px-2.5 py-2 text-xs sm:px-3 sm:text-sm ${now ? 'border-[var(--brand-primary)] bg-white font-semibold text-slate-900' : done ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-slate-200 bg-white text-slate-500'}`}>
          <span className="flex items-center gap-1.5">{done ? <Icon name="check" className="h-3.5 w-3.5 shrink-0" /> : <span aria-hidden="true" className="tabular-nums">{i + 1}.</span>}<span className="truncate">{s.label}</span></span>
          <span className="block truncate text-[11px] font-normal text-slate-500">{s.who}</span>
          {done && <span className="sr-only">{t('pmStepDone')}</span>}
        </li>
      })}
    </ol>}
  </header>
}

function Counts({ b }: { b: PromotionBatch }) {
  const { t } = useTranslation()
  const num = useNum()
  const tiles = [...DECISIONS.map(d => ({ key: d, label: t(`pmCount_${d}`), n: b.counts[d] ?? 0 })), { key: 'undecided', label: t('pmCount_undecided'), n: b.counts.undecided ?? 0 }]
  return <dl className="grid grid-cols-3 gap-2 sm:grid-cols-6" aria-label={t('pmSummary')}>
    {tiles.map(x => <div key={x.key} className={`rounded-xl border px-3 py-2.5 ${x.key === 'undecided' && x.n > 0 ? 'border-amber-300 bg-amber-50' : 'border-slate-200 bg-white'}`}>
      <dt className="truncate text-xs font-medium text-slate-600">{x.label}</dt>
      <dd className="text-xl font-extrabold tabular-nums text-slate-900">{num(x.n)}</dd>
    </div>)}
  </dl>
}

type Run = (action: (token: string) => Promise<PromotionBatch>, done: string) => Promise<boolean>

// NextStep: what happens next and who does it; the action only for a role
// the API allows.
function NextStep({ b, caps, busy, run, onApply }: { b: PromotionBatch; caps: Caps; busy: boolean; run: Run; onApply: () => void }) {
  const { t } = useTranslation()
  const when = useDateTime()
  const num = useNum()
  const issue = useIssueText(b)
  const [reason, setReason] = useState('')
  const blocked = b.blockers.length > 0
  const stale = b.status === 'approved' && b.approved_sha256 !== b.inputs_sha256
  if (b.status === 'discarded') return <Notice tone="info">{t('pmDiscardedNote')}</Notice>
  if (b.status === 'reverted') return <SectionCard id="pm-reverted" title={t('pmRevertedTitle')}>
    <p className="text-sm text-slate-700">{t('pmRevertedBy', { name: b.reverted_by ?? '', when: when(b.reverted_at) })}</p>
    <p className="mt-1 text-sm text-slate-700"><span className="font-semibold">{t('pmReason')}:</span> {b.revert_reason}</p>
  </SectionCard>

  if (b.status === 'applied') {
    const refused = b.revert_blockers.length > 0
    return <SectionCard id="pm-applied" title={t('pmAppliedTitle')} description={t('pmAppliedBy', { name: b.applied_by ?? '', when: when(b.applied_at) })}>
      {!caps.apply ? <p className="text-sm text-slate-600">{t('pmAppliedNote')}</p>
        : refused ? <Notice tone="warning">
          <p className="font-semibold">{t('pmRevertRefused')}</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5">{b.revert_blockers.slice(0, 8).map((x, i) => <li key={i}>{issue(x)}</li>)}</ul>
          {b.revert_blockers.length > 8 && <p className="mt-1">{t('pmAndMore', { n: num(b.revert_blockers.length - 8) })}</p>}
        </Notice>
          : <form className="space-y-3" onSubmit={async e => {
            e.preventDefault()
            if (!reason.trim() || !(await confirmAction(t('pmRevertConfirm', { name: b.class_name, to: b.to_session_name }), { confirmLabel: t('pmRevert') }))) return
            if (await run(token => revertBatch(token, b.id, reason.trim()), t('pmReverted'))) setReason('')
          }}>
            <p className="text-sm text-slate-700">{t('pmRevertIntro')}</p>
            <label className={ui.label}>{t('pmRevertReason')}
              <textarea className={ui.field} rows={2} maxLength={500} required value={reason} onChange={e => setReason(e.target.value)} /></label>
            <button type="submit" disabled={busy || !reason.trim()} className={ui.danger}><Icon name="arrowRight" className="h-4 w-4 rotate-180" />{t('pmRevert')}</button>
          </form>}
    </SectionCard>
  }

  // Draft or approved.
  if (b.status === 'approved' && !stale) return <SectionCard id="pm-approved" title={t('pmApprovedTitle')} description={t('pmApprovedBy', { name: b.approved_by ?? '', when: when(b.approved_at) })}>
    {caps.apply ? <div className="flex flex-wrap items-center gap-3">
      <button type="button" disabled={busy || blocked} onClick={onApply} className={ui.primary}><Icon name="check" className="h-4 w-4" />{t('pmApplyOpen')}</button>
      {blocked && <span className="text-sm text-rose-700">{t('pmFixBlockers')}</span>}
    </div> : <p className="text-sm text-slate-600">{t('pmWaitingApply')}</p>}
  </SectionCard>

  if (caps.approve) return <ApprovalPanel b={b} busy={busy} run={run} stale={stale} />
  return <Notice tone={blocked ? 'warning' : 'info'}>{stale ? t('pmStale') + ' ' : ''}{blocked ? t('pmBlockedNote', { count: b.blockers.length, n: num(b.blockers.length) }) : t('pmWaitingApproval')}</Notice>
}

function ApprovalPanel({ b, busy, run, stale }: { b: PromotionBatch; busy: boolean; run: Run; stale: boolean }) {
  const { t } = useTranslation()
  const num = useNum()
  const overrides = b.items.filter(i => i.decision && i.decision !== i.recommendation)
  const blocked = b.blockers.length > 0
  return <SectionCard id="pm-approve" title={t('pmApproveTitle')} description={t('pmApproveIntro', { cls: b.class_name, from: b.from_session_name, to: b.to_session_name })}>
    {stale && <p className="mb-3 text-sm font-semibold text-amber-800">{t('pmStale')}</p>}
    <ul className="space-y-1 text-sm text-slate-800">
      {DECISIONS.filter(d => (b.counts[d] ?? 0) > 0).map(d => <li key={d} className="flex items-center gap-2"><DecisionPill value={d} /><span>{t(`pmWill_${d}`, { n: num(b.counts[d]), next: b.next_class_name ?? '', cls: b.class_name, to: b.to_session_name })}</span></li>)}
      {(b.counts.logins_to_disable ?? 0) > 0 && <li className="flex items-center gap-2 text-rose-800"><Icon name="lock" className="h-4 w-4" />{t('pmLoginsDisable', { count: b.counts.logins_to_disable, n: num(b.counts.logins_to_disable) })}</li>}
    </ul>
    {overrides.length > 0 && <div className="mt-4">
      <h3 className="text-sm font-bold text-slate-900">{t('pmOverrides', { n: num(overrides.length) })}</h3>
      <ul className="mt-2 space-y-2">{overrides.map(o => <li key={o.student_id} className="rounded-lg bg-amber-50 px-3 py-2 text-sm">
        <span className="font-semibold text-slate-900">{o.student_name}</span>: {t(`pmDecision_${o.recommendation}`)} → {t(`pmDecision_${o.decision}`)}
        <span className="block text-slate-700">{t('pmReason')}: {o.reason}</span></li>)}</ul>
    </div>}
    <div className="mt-4 flex flex-wrap items-center gap-3">
      <button type="button" disabled={busy || blocked} className={ui.primary}
        onClick={async () => { if (await confirmAction(t('pmApproveConfirm', { cls: b.class_name }), { tone: 'default', confirmLabel: t('pmApprove') })) await run(token => approveBatch(token, b.id, b.inputs_sha256), t('pmApprovedDone')) }}>
        <Icon name="check" className="h-4 w-4" />{t('pmApprove')}</button>
      {blocked && <span className="text-sm text-rose-700">{t('pmFixBlockersHead', { count: b.blockers.length, n: num(b.blockers.length) })}</span>}
    </div>
  </SectionCard>
}

function Issues({ b }: { b: PromotionBatch }) {
  const { t } = useTranslation()
  const issue = useIssueText(b)
  if (b.status !== 'draft' && b.status !== 'approved') return null
  if (!b.blockers.length && !b.warnings.length) return <Notice tone="success">{t('pmNoIssues')}</Notice>
  const list = (items: typeof b.blockers) => <ul className="mt-1 list-disc space-y-0.5 pl-5">{items.slice(0, 10).map((x, i) => <li key={i}>{issue(x)}</li>)}{items.length > 10 && <li>{t('pmAndMore', { n: items.length - 10 })}</li>}</ul>
  return <div className="grid gap-3 lg:grid-cols-2">
    {b.blockers.length > 0 && <div role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900"><p className="font-semibold">{t('pmBlockersTitle', { n: b.blockers.length })}</p>{list(b.blockers)}</div>}
    {b.warnings.length > 0 && <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"><p className="font-semibold">{t('pmWarningsTitle', { n: b.warnings.length })}</p>{list(b.warnings)}</div>}
  </div>
}

function Tools({ b, busy, run }: { b: PromotionBatch; busy: boolean; run: Run }) {
  const { t } = useTranslation()
  const [policy, setPolicy] = useState<RollPolicy>(b.roll_policy)
  return <SectionCard id="pm-tools" title={t('pmToolsTitle')} description={t('pmToolsIntro')}>
    <div className="flex flex-wrap items-end gap-3">
      <label className={`${ui.label} w-full sm:w-auto`}>{t('pmRollPolicy')}
        <select className={`${ui.field} sm:w-56`} value={policy} onChange={e => setPolicy(e.target.value as RollPolicy)}>
          {(['keep', 'merit', 'name', 'manual'] as RollPolicy[]).map(p => <option key={p} value={p}>{t(`pmRoll_${p}`)}</option>)}
        </select></label>
      <button type="button" disabled={busy} className={ui.secondary} onClick={() => void run(token => arrangeRolls(token, b.id, policy), t('pmArranged'))}>{t('pmArrange')}</button>
      <button type="button" disabled={busy} className={ui.secondary} onClick={() => void run(token => refreshBatch(token, b.id), t('pmRefreshed'))}><Icon name="clock" className="h-4 w-4" />{t('pmRefresh')}</button>
      <button type="button" disabled={busy} className={ui.danger} onClick={async () => { if (await confirmAction(t('pmDiscardConfirm', { cls: b.class_name }), { confirmLabel: t('pmDiscard') })) await run(token => discardBatch(token, b.id), t('pmDiscarded')) }}>{t('pmDiscard')}</button>
    </div>
    <p className="mt-2 text-xs text-slate-500">{t(`pmRollHint_${policy}`)}</p>
  </SectionCard>
}

function Students({ b, editable, onEdit }: { b: PromotionBatch; editable: boolean; onEdit: (item: PromotionItem) => void }) {
  const { t } = useTranslation()
  const num = useNum()
  const issue = useIssueText(b)
  const [filter, setFilter] = useState<Filter>('all')
  const byStudent = new Map<string, string[]>()
  for (const x of [...b.blockers, ...b.warnings]) if (x.student_id) byStudent.set(x.student_id, [...(byStudent.get(x.student_id) ?? []), issue(x)])
  const blockedIds = new Set(b.blockers.map(x => x.student_id))
  const match = (i: PromotionItem) => filter === 'all' || (filter === 'undecided' ? !i.decision : filter === 'override' ? !!i.decision && i.decision !== i.recommendation : blockedIds.has(i.student_id))
  const shown = b.items.filter(match)
  const filters: { value: Filter; label: string; n: number }[] = [
    { value: 'all', label: t('pmF_all'), n: b.items.length },
    { value: 'undecided', label: t('pmF_undecided'), n: b.items.filter(i => !i.decision).length },
    { value: 'override', label: t('pmF_override'), n: b.items.filter(i => i.decision && i.decision !== i.recommendation).length },
    { value: 'issues', label: t('pmF_issues'), n: b.items.filter(i => blockedIds.has(i.student_id)).length },
  ]
  const place = (i: PromotionItem): ReactNode => i.target_class_name
    ? <><span className="font-medium text-slate-900">{i.target_class_name}</span>{' · '}{i.target_section_name ?? <span className="text-rose-700">{t('pmNoSection')}</span>}{i.target_roll_no !== null && <> · {t('pmRoll', { n: num(i.target_roll_no) })}</>}</>
    : <span className="text-slate-500">{i.decision ? t(`pmLeaves_${i.decision}`) : '—'}</span>
  const notes = (i: PromotionItem) => <>
    {i.decision && i.decision !== i.recommendation && <span className="mt-1 block text-xs text-amber-800">{t('pmReason')}: {i.reason}</span>}
    {(byStudent.get(i.student_id) ?? []).map((s, k) => <span key={k} className={`mt-1 block text-xs ${blockedIds.has(i.student_id) ? 'text-rose-700' : 'text-amber-800'}`}>{s}</span>)}
  </>
  const edit = (i: PromotionItem) => editable && <button type="button" onClick={() => onEdit(i)} className={ui.secondary} aria-label={t('pmEditFor', { name: i.student_name })}><Icon name="settings" className="h-4 w-4" />{t('pmEdit')}</button>
  return <SectionCard id="pm-students" title={t('pmStudents', { n: num(b.items.length) })}
    actions={<div role="group" aria-label={t('pmFilterLabel')} className="flex flex-wrap gap-1 rounded-xl bg-slate-100 p-1">
      {filters.map(f => <button key={f.value} type="button" aria-pressed={filter === f.value} onClick={() => setFilter(f.value)}
        className={`rounded-lg px-2.5 py-1.5 text-xs font-semibold focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] sm:text-sm ${filter === f.value ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'}`}>{f.label} <span className="tabular-nums">{num(f.n)}</span></button>)}
    </div>}>
    {shown.length === 0 ? <p className="text-sm text-slate-600">{t('pmNoneMatch')}</p> : <>
      <ul className="grid gap-3 md:hidden">{shown.map(i => <li key={i.student_id} className={`rounded-xl border p-3 ${blockedIds.has(i.student_id) ? 'border-rose-200' : 'border-slate-200'}`}>
        <div className="flex items-start justify-between gap-2">
          <p className="min-w-0"><span className="block font-semibold text-slate-900">{i.student_name}</span><span className="text-xs text-slate-500">{i.admission_no} · {i.source_section_name}{i.source_roll_no !== null && ` · ${t('pmRoll', { n: num(i.source_roll_no) })}`}</span></p>
          {edit(i)}
        </div>
        <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1.5 text-sm">
          <dt className="text-slate-500">{t('pmAnnual')}</dt><dd><OutcomePill outcome={i.annual_outcome} position={i.annual_class_position} /></dd>
          <dt className="text-slate-500">{t('pmRecommended')}</dt><dd><DecisionPill value={i.recommendation} /></dd>
          <dt className="text-slate-500">{t('pmDecision')}</dt><dd>{i.decision ? <DecisionPill value={i.decision} /> : <span className="font-semibold text-amber-800">{t('pmUndecided')}</span>}</dd>
          <dt className="text-slate-500">{t('pmMovesTo')}</dt><dd className="min-w-0 break-words">{place(i)}</dd>
        </dl>
        {notes(i)}
      </li>)}</ul>
      <div className="hidden overflow-x-auto md:block"><table className={ui.table}>
        <caption className="sr-only">{t('pmStudents', { n: num(b.items.length) })}</caption>
        <thead><tr>
          <th scope="col" className={ui.th}>{t('pmStudent')}</th><th scope="col" className={ui.th}>{t('pmAnnual')}</th><th scope="col" className={ui.th}>{t('pmRecommended')}</th>
          <th scope="col" className={ui.th}>{t('pmDecision')}</th><th scope="col" className={ui.th}>{t('pmMovesTo')}</th>{editable && <th scope="col" className={ui.th}><span className="sr-only">{t('pmEdit')}</span></th>}
        </tr></thead>
        <tbody>{shown.map(i => <tr key={i.student_id} className={blockedIds.has(i.student_id) ? 'bg-rose-50/40' : ''}>
          <td className={ui.td}><span className="font-semibold text-slate-900">{i.student_name}</span><span className="block text-xs text-slate-500">{i.admission_no} · {i.source_section_name}{i.source_roll_no !== null && ` · ${t('pmRoll', { n: num(i.source_roll_no) })}`}</span></td>
          <td className={ui.td}><OutcomePill outcome={i.annual_outcome} position={i.annual_class_position} /></td>
          <td className={ui.td}><DecisionPill value={i.recommendation} /></td>
          <td className={ui.td}>{i.decision ? <DecisionPill value={i.decision} /> : <span className="font-semibold text-amber-800">{t('pmUndecided')}</span>}{notes(i)}</td>
          <td className={ui.td}>{place(i)}</td>
          {editable && <td className={`${ui.td} text-right`}>{edit(i)}</td>}
        </tr>)}</tbody>
      </table></div>
    </>}
  </SectionCard>
}

function EditItem({ b, item, busy, onClose, onSave }: { b: PromotionBatch; item: PromotionItem; busy: boolean; onClose: () => void; onSave: (input: ItemInput) => Promise<void> }) {
  const { t } = useTranslation()
  const authorized = useAuthorized()
  const [decision, setDecision] = useState<Decision | ''>(item.decision ?? '')
  const [reason, setReason] = useState(item.reason)
  const [section, setSection] = useState(item.target_section_id ?? '')
  const [roll, setRoll] = useState(item.target_roll_no === null ? '' : String(item.target_roll_no))
  const targetClass = decision === 'promote' ? b.next_class_id : decision === 'repeat' ? b.class_id : null
  const sections = useQuery({
    queryKey: ['pm-sections', b.to_session_id, targetClass], retry: false, enabled: !!targetClass,
    queryFn: () => authorized(token => getAcademicSections(token, new URLSearchParams({ session_id: b.to_session_id, class_id: targetClass ?? '', page: '1', page_size: '100' }))),
  })
  const override = decision !== '' && decision !== item.recommendation
  const rollNo = roll.trim() === '' ? null : Number(roll)
  const rollOk = rollNo === null || (Number.isInteger(rollNo) && rollNo >= 1 && rollNo <= 999999)
  const options = DECISIONS.filter(d => !(d === 'promote' && b.class_is_final))
  const sameClass = decision === item.decision
  const valid = decision !== '' && (!override || reason.trim() !== '') && rollOk

  function save() {
    if (!valid) return
    const input: ItemInput = {}
    if (decision !== item.decision) input.decision = decision as Decision
    if (reason.trim() !== item.reason) input.reason = reason.trim()
    if (targetClass && sameClass && section && section !== item.target_section_id) input.target_section_id = section
    if (targetClass && sameClass && rollNo !== null && rollNo !== item.target_roll_no) input.target_roll_no = rollNo
    if (Object.keys(input).length === 0) { onClose(); return }
    void onSave(input)
  }
  return <Dialog title={t('pmEditTitle', { name: item.student_name })} onClose={onClose} wide
    footer={<><button type="button" onClick={onClose} className={ui.secondary}>{t('pmCancel')}</button><button type="button" disabled={busy || !valid} onClick={save} className={ui.primary}>{t('pmSave')}</button></>}>
    <div className="space-y-4">
      <p className="text-sm text-slate-700">{t('pmAnnual')}: <OutcomePill outcome={item.annual_outcome} position={item.annual_class_position} /> · {t('pmRecommended')}: <DecisionPill value={item.recommendation} /></p>
      <fieldset>
        <legend className={ui.label}>{t('pmDecision')}</legend>
        <div className="mt-2 grid gap-2 sm:grid-cols-2">{options.map(d => <label key={d} className={`flex min-h-11 cursor-pointer items-center gap-2 rounded-xl border px-3 py-2 text-sm font-medium ${decision === d ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)] text-slate-900' : 'border-slate-200 text-slate-700'}`}>
          <input type="radio" name="pm-decision" value={d} checked={decision === d} onChange={() => setDecision(d)} className="h-4 w-4 accent-[var(--brand-primary)]" />
          <span>{t(`pmDecision_${d}`)}{d === item.recommendation && <span className="ml-1 text-xs text-slate-500">({t('pmRecommendedShort')})</span>}</span></label>)}</div>
      </fieldset>
      {(decision === 'withdraw' || decision === 'transfer') && <p className="rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-900"><Icon name="lock" className="mr-1 inline h-4 w-4" />{t('pmLoginWarning')}</p>}
      {decision === 'graduate' && <p className="rounded-lg bg-violet-50 px-3 py-2 text-sm text-violet-900">{t('pmGraduateNote')}</p>}
      <label className={ui.label}>{override ? t('pmReasonRequired') : t('pmReasonOptional')}
        <textarea className={ui.field} rows={2} maxLength={500} required={override} value={reason} onChange={e => setReason(e.target.value)} aria-describedby="pm-reason-hint" /></label>
      <p id="pm-reason-hint" className="-mt-2 text-xs text-slate-500">{t('pmReasonHint')}</p>
      {targetClass && <div className="grid gap-4 sm:grid-cols-2">
        <label className={ui.label}>{t('pmTargetSection')}
          <select className={ui.field} value={section} disabled={!sameClass} onChange={e => setSection(e.target.value)}>
            <option value="">{t('pmAutoSection')}</option>
            {(sections.data?.items ?? []).filter(s => s.is_active).map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select></label>
        <label className={ui.label}>{t('pmTargetRoll')}
          <input className={ui.field} inputMode="numeric" value={roll} disabled={!sameClass} onChange={e => setRoll(e.target.value.replace(/[^0-9]/g, ''))} aria-invalid={!rollOk} /></label>
        <p className="text-xs text-slate-500 sm:col-span-2">{sameClass ? t('pmPlaceHint') : t('pmPlaceAfterSave')}</p>
      </div>}
    </div>
  </Dialog>
}

function ApplyDialog({ b, busy, onClose, onConfirm }: { b: PromotionBatch; busy: boolean; onClose: () => void; onConfirm: () => void }) {
  const { t } = useTranslation()
  const num = useNum()
  const logins = b.counts.logins_to_disable ?? 0
  return <Dialog title={t('pmApplyTitle', { cls: b.class_name })} onClose={onClose}
    footer={<><button type="button" onClick={onClose} className={ui.secondary}>{t('pmCancel')}</button><button type="button" disabled={busy} onClick={onConfirm} className={ui.primary}><Icon name="check" className="h-4 w-4" />{t('pmApplyNow')}</button></>}>
    <p className="text-sm text-slate-700">{t('pmApplyIntro', { from: b.from_session_name, to: b.to_session_name })}</p>
    <ul className="mt-3 space-y-2 text-sm text-slate-800">
      {DECISIONS.map(d => <li key={d} className="flex items-center justify-between gap-3 rounded-lg bg-slate-50 px-3 py-2">
        <span>{t(`pmWill_${d}`, { n: num(b.counts[d] ?? 0), next: b.next_class_name ?? '', cls: b.class_name, to: b.to_session_name })}</span>
        <span className="font-bold tabular-nums">{num(b.counts[d] ?? 0)}</span></li>)}
      <li className={`flex items-center justify-between gap-3 rounded-lg px-3 py-2 ${logins > 0 ? 'bg-rose-50 text-rose-900' : 'bg-slate-50'}`}>
        <span className="flex items-center gap-2"><Icon name="lock" className="h-4 w-4" />{t('pmLoginsDisableLine')}</span><span className="font-bold tabular-nums">{num(logins)}</span></li>
    </ul>
    <p className="mt-3 text-sm text-slate-600">{t('pmApplyKeeps')}</p>
    <p className="mt-2 text-sm text-slate-600">{t('pmApplyRevertNote', { to: b.to_session_name })}</p>
  </Dialog>
}
