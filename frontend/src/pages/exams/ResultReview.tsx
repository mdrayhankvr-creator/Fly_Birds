import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { approveResults, getResultReview, publishResults, type ResultReview as Review, type ResultScope, type ResultScopeRef } from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { CardTitle, EmptyState, Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { DiffTable, ProvisionalFrame, ResultMatrix, StateChip, VersionHistory, useOutputChanges, useResultError, useWhen, type MatrixRow } from './resultUi'

// Result Review & Approval and publication (UX spec §9 and §10) for one
// scope of one class: a term, the annual result, or a correction of an
// exam. Results are computed now and shown provisionally beside what is
// published. The head teacher approves exactly what was shown (the server
// binds the approval to the inputs' hash); an administrator publishes.
export function ResultReview({ canApprove, canPublish }: { canApprove: boolean; canPublish: boolean }) {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const explain = useResultError()
  const when = useWhen()
  const changesOf = useOutputChanges()
  const ref: ResultScopeRef = { scope: (params.get('scope') || 'term') as ResultScope, scope_id: params.get('scope_id') || '', class_id: params.get('class_id') || undefined }
  const session = params.get('session') || ''
  const review = useQuery({ queryKey: ['e6', 'review', ref.scope, ref.scope_id, ref.class_id], enabled: !!ref.scope_id, queryFn: () => authorized(token => getResultReview(token, ref)), retry: false, gcTime: 0 })
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [onlyChanges, setOnlyChanges] = useState(false)
  const [history, setHistory] = useState<{ id: string; name: string } | null>(null)

  async function act(task: () => Promise<Review>, done: string) {
    setBusy(true); setError(''); setMessage('')
    try {
      const v = await task()
      client.setQueryData(['e6', 'review', ref.scope, ref.scope_id, ref.class_id], v)
      void client.invalidateQueries({ queryKey: ['e6', 'overview'] })
      setMessage(done); setReason('')
    } catch (cause) {
      setError(explain(cause))
      void review.refetch()
    } finally { setBusy(false) }
  }

  const back = ref.scope === 'assessment' ? '/admin/exams' : `/admin/exams/results${session ? `?session=${session}` : ''}`
  const v = review.data
  const title = v ? (ref.scope === 'annual' ? t('rvAnnualTitle', { session: v.title, cls: v.class_name }) : ref.scope === 'term' ? t('rvTermTitle', { term: v.title, cls: v.class_name }) : t('rvCorrectionTitle', { exam: v.title })) : t('rvReviewTitle')
  return <div className="space-y-5">
    <PageHeader eyebrow={t('rvCentreTitle')} title={title} description={t(`rvIntro_${ref.scope}`)} actions={<Link to={back} className={ui.secondary}>{t(ref.scope === 'assessment' ? 'rvBackToExams' : 'rvBackToCentre')}</Link>} />
    {(error || message) && <Notice tone={error ? 'error' : 'success'}>{error || message}</Notice>}
    {!ref.scope_id ? <EmptyState icon="grade" title={t('rvNothingChosen')} /> : review.isPending ? <Skeleton rows={6} /> : review.isError ? <Notice tone="error">{explain(review.error)}</Notice> : v && <>
      <section className={`${ui.card} space-y-3 p-4 sm:p-5`} aria-labelledby="rv-status-title">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <h2 id="rv-status-title" className="flex flex-wrap items-center gap-2 font-semibold text-slate-900"><StateChip state={v.state} /><span>{t(`rvStateLine_${v.state}`, { n: v.changed })}</span></h2>
            <p className="mt-1 text-sm text-slate-600">{v.published ? t('rvPublishedRevision', { revision: v.published.revision, name: v.published.published_by || '—', when: when(v.published.published_at) }) : t('rvNeverPublished')}</p>
            {v.locked && <p className="mt-1 text-sm font-semibold text-slate-700">{t('rvLockedNote')}</p>}
          </div>
          <dl className="flex flex-wrap gap-4 text-sm">{(['passed', 'failed', 'withheld', 'incomplete'] as const).map(o => <div key={o}><dt className="text-xs text-slate-500">{t(`rvOutcome_${o}`)}</dt><dd className="text-2xl font-bold tabular-nums text-slate-900">{v.outcomes[o] || 0}</dd></div>)}</dl>
        </div>
        <p className="text-xs text-slate-600">{t(v.rules.bands_source === 'blueprint_version' ? 'rvGradedWithVersion' : 'rvGradedWithSchool')}{v.rules.annual ? ` · ${methodLabel(t, v.rules.annual)}` : ''}</p>
        {v.blockers.length > 0 && <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900" role="status">
          <p className="font-semibold">{t('rvBlocked')}</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5">{v.blockers.map((b, i) => <li key={i}>{t(`rvBlocker_${b.code.startsWith('sheet_') ? 'sheet' : b.code}`, { detail: b.detail || '', status: t(`e3SheetStatus_${b.code.replace('sheet_', '')}`) })}{b.sheet_id && b.exam_id && <> · <Link className={ui.link} to={`/admin/exams/${b.exam_id}/sheets/${b.sheet_id}`}>{t('rvOpenSheet')}</Link></>}</li>)}</ul>
        </div>}
        {v.outcomes.incomplete > 0 && v.blockers.length === 0 && <Notice tone="warning">{t('rvIncompleteStudents', { count: v.outcomes.incomplete })}</Notice>}
        {v.corrections.length > 0 && <div className="rounded-xl border border-violet-300 px-4 py-3 text-sm"><p className="font-semibold text-violet-900">{t('rvOpenCorrections', { count: v.corrections.length })}</p><ul className="mt-1 list-disc pl-5 text-slate-700">{v.corrections.map(c => <li key={c.id}>{c.student_name} · {c.reason}</li>)}</ul></div>}
        {v.other_position_changes > 0 && <p className="text-sm text-slate-700">{t('rvOtherPositions', { count: v.other_position_changes })}</p>}
        {v.approval && <div className={`rounded-xl border px-4 py-3 text-sm ${v.approval.stale ? 'border-amber-300 bg-amber-50 text-amber-900' : 'border-emerald-200 bg-emerald-50 text-emerald-900'}`} role="status">
          <p className="font-semibold">{t(v.approval.stale ? 'rvApprovalStale' : 'rvApprovedBy', { name: v.approval.approved_by || '—', when: when(v.approval.approved_at) })}</p>
          {v.approval.reason && <p className="mt-0.5">{t('rvBecause', { reason: v.approval.reason })}</p>}
        </div>}
        <Actions v={v} reason={reason} setReason={setReason} busy={busy} canApprove={canApprove} canPublish={canPublish}
          onApprove={() => act(() => authorized(token => approveResults(token, ref, v.inputs_sha256, reason.trim())), t('rvApprovedMessage'))}
          onPublish={async () => {
            if (!v.approval) return
            const ok = await confirmAction(v.published ? t('rvConfirmRevision', { count: v.changed, positions: v.other_position_changes }) : t('rvConfirmPublish', { count: v.students.length, cls: v.class_name }), { tone: 'default', confirmLabel: t(v.published ? 'rvPublishRevision' : 'rvPublish') })
            if (ok) void act(() => authorized(token => publishResults(token, ref, v.approval!.id)), t('rvPublishedMessage'))
          }} />
      </section>

      {history && <VersionHistory scope={ref.scope} scopeId={ref.scope_id} studentId={history.id} studentName={history.name} onClose={() => setHistory(null)} />}

      {v.published && v.changed > 0 && <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="rv-changes-title">
        <CardTitle id="rv-changes-title" icon="alert">{t('rvChangesTitle', { count: v.changed })}</CardTitle>
        <p className="mt-1 text-sm text-slate-600">{t('rvChangesIntro')}</p>
        <ul className="mt-3 space-y-3">{v.students.filter(s => s.change !== 'same').map(s => <li key={s.student_id} className="rounded-xl border border-slate-200 p-3">
          <p className="font-semibold text-slate-900">{s.name} <span className="text-xs font-normal text-slate-500">{s.section_name} · {s.admission_no}</span></p>
          <div className="mt-2"><DiffTable rows={changesOf(s.previous, s.outputs)} caption={t('rvChangesFor', { name: s.name })} /></div>
        </li>)}</ul>
      </section>}

      <ProvisionalFrame id="rv-table-title" label={t(v.state === 'published' ? 'rvMatchesPublished' : 'rvProvisional')}>
        {v.published && v.changed > 0 && <label className="mb-3 flex min-h-11 items-center gap-2 text-sm font-medium text-slate-700"><input type="checkbox" className="h-4 w-4" checked={onlyChanges} onChange={e => setOnlyChanges(e.target.checked)} />{t('rvOnlyChanges')}</label>}
        {v.students.length === 0 ? <p className="text-sm text-slate-600">{t('rvNoStudents')}</p> :
          <ResultMatrix label={t('rvTableLabel')} subjects={v.subjects} rows={v.students.filter(s => !onlyChanges || s.change !== 'same').map((s): MatrixRow => ({
            id: s.student_id, name: s.name, meta: `${s.section_name} · ${s.roll_no ?? '—'}`, outputs: s.outputs, blocker: s.blocker, highlight: !!v.published && s.change !== 'same',
            action: s.current ? <button type="button" className={`${ui.link} min-h-11 text-sm`} onClick={() => setHistory({ id: s.student_id, name: s.name })}>{t('rvHistory')}</button> : undefined,
          }))} />}
      </ProvisionalFrame>
    </>}
  </div>
}

function methodLabel(t: (k: string, o?: Record<string, unknown>) => string, a: { method: string; final_must_pass?: boolean; terms: { name: string; weight: number }[] }) {
  if (a.method === 'weighted') return t('rvMethod_weighted', { terms: a.terms.map(x => `${x.weight} % ${x.name}`).join(', ') })
  return t(`rvMethod_${a.method}`)
}

function Actions({ v, reason, setReason, busy, canApprove, canPublish, onApprove, onPublish }: { v: Review; reason: string; setReason: (s: string) => void; busy: boolean; canApprove: boolean; canPublish: boolean; onApprove: () => void; onPublish: () => void }) {
  const { t } = useTranslation()
  const approved = !!v.approval && !v.approval.stale
  const needsReason = !!v.published && v.corrections.length === 0
  if (!canApprove && !canPublish) return <p className="text-sm text-slate-600">{t('rvReadOnly')}</p>
  return <div className="flex flex-col gap-3 border-t border-slate-100 pt-3 sm:flex-row sm:items-end sm:justify-between">
    {canApprove && v.can_approve && !approved ? <form className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:items-end" onSubmit={e => { e.preventDefault(); onApprove() }}>
      {v.published && <label className={`${ui.label} min-w-0 flex-1`}>{t(needsReason ? 'rvReasonRequired' : 'rvReasonOptional')}
        <input className={ui.field} maxLength={500} required={needsReason} value={reason} onChange={e => setReason(e.target.value)} placeholder={t('rvReasonPlaceholder')} />
      </label>}
      <button type="submit" disabled={busy || (needsReason && reason.trim().length < 3)} className={ui.primary}>{t(v.published ? 'rvApproveRevision' : 'rvApprove')}</button>
    </form> : <p className="text-sm text-slate-600">{approved ? t(canPublish ? 'rvReadyToPublish' : 'rvWaitingForAdmin') : v.state === 'blocked' ? t('rvResolveFirst') : v.state === 'published' ? t('rvNothingToDo') : canApprove ? '' : t('rvWaitingForApproval')}</p>}
    {canPublish && approved && <button type="button" disabled={busy} className={ui.primary} onClick={onPublish}>{t(v.published ? 'rvPublishRevision' : 'rvPublish')}</button>}
  </div>
}
