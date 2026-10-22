import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { createTermExams, getTermSetup, saveExamTerm, saveTermSetup, type ExamTerm, type SetupPlan, type SetupResult, type TermSetup } from '../../api/client'
import { Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { TermChip, useExplain } from './setupShared'

const steps = ['term', 'classes', 'assessments', 'sheets', 'review'] as const
type Step = typeof steps[number]

// TermWizard sets up a whole term (UX spec §6): the term, the classes, the
// assessments' marks due dates, the sheets and their teachers, then one
// Create that makes every exam and sheet. Each step's choices are kept on
// the term, so leaving halfway loses nothing and nothing reaches teachers
// before Create.
export function TermWizard({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const { termId = '' } = useParams()
  const [params] = useSearchParams()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const client = useQueryClient()
  const plan = useQuery({ queryKey: ['e5', 'setup', termId], queryFn: () => authorized(token => getTermSetup(token, termId)), retry: false, gcTime: 0 })
  const [step, setStep] = useState<Step>('term')
  const [setup, setSetup] = useState<TermSetup>({ class_ids: [], due: {}, assignees: {} })
  const [term, setTerm] = useState<Pick<ExamTerm, 'name' | 'kind' | 'starts_on' | 'ends_on'>>({ name: '', kind: 'term', starts_on: null, ends_on: null })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<SetupResult | null>(null)
  const p = plan.data
  const session = params.get('session') || p?.term.session_id || ''

  useEffect(() => {
    if (!p) return
    setSetup({ class_ids: p.setup.class_ids || [], due: p.setup.due || {}, assignees: p.setup.assignees || {} })
    setTerm({ name: p.term.name, kind: p.term.kind, starts_on: p.term.starts_on, ends_on: p.term.ends_on })
  }, [p])

  // Every step saves the choices so far, then moves on.
  async function next(to: Step) {
    setBusy(true); setError('')
    try {
      if (step === 'term' && canManage && p) await authorized(token => saveExamTerm(token, { session_id: p.term.session_id, name: term.name.trim(), kind: term.kind, starts_on: term.starts_on || '', ends_on: term.ends_on || '' }, termId))
      else if (canManage && step !== 'review') await authorized(token => saveTermSetup(token, termId, setup))
      await client.invalidateQueries({ queryKey: ['e5', 'setup', termId] })
      setStep(to)
    } catch (cause) { setError(explain(cause)) }
    finally { setBusy(false) }
  }
  async function create() {
    setBusy(true); setError('')
    try {
      await authorized(token => saveTermSetup(token, termId, setup))
      const done = await authorized(token => createTermExams(token, termId))
      setResult(done)
      // The reply carries the new plan: show it at once; other lists refresh behind.
      client.setQueryData(['e5', 'setup', termId], done.plan)
      void client.invalidateQueries({ queryKey: ['e5'], predicate: q => q.queryKey[1] !== 'setup' })
      void client.invalidateQueries({ queryKey: ['m7-exams'] })
    } catch (cause) { setError(explain(cause)) }
    finally { setBusy(false) }
  }

  if (plan.isPending) return <Skeleton rows={6} />
  if (plan.isError || !p) return <Notice tone="error">{t('bpErrGeneric')}</Notice>
  const index = steps.indexOf(step)
  const ready = p.classes.filter(c => c.state === 'ready')
  const missingDue = p.assessments.filter(a => !setup.due[a.key])
  const blocked = p.new_exams === 0 && p.exams === 0 || p.conflicts.length > 0 || missingDue.length > 0

  return <div className="space-y-5">
    <PageHeader eyebrow={t('bpSetupTitle')} title={t('bpWizardTitle', { name: p.term.name })} description={t('bpWizardIntro')}
      actions={<Link to={`/admin/exams/setup?session=${session}`} className={ui.secondary}>{t('bpBackToSetup')}</Link>} />
    {!canManage && <Notice tone="info">{t('bpReadOnlyRole')}</Notice>}
    <div className="grid gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
      <nav aria-label={t('bpSteps')} className={`${ui.card} p-3`}>
        <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-slate-100 lg:hidden" role="progressbar" aria-valuemin={1} aria-valuemax={steps.length} aria-valuenow={index + 1} aria-label={t('bpStepOf', { n: index + 1, total: steps.length })}>
          <div className="h-full rounded-full bg-[var(--brand-primary)]" style={{ width: `${((index + 1) / steps.length) * 100}%` }} />
        </div>
        <ol className="flex gap-1 overflow-x-auto lg:flex-col">
          {steps.map((s, i) => <li key={s} className="shrink-0">
            <button type="button" aria-current={s === step ? 'step' : undefined} onClick={() => setStep(s)}
              className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm ${s === step ? 'bg-[var(--brand-soft)] font-semibold text-slate-900' : 'text-slate-600 hover:bg-slate-50'}`}>
              <span aria-hidden="true" className={`grid h-6 w-6 place-items-center rounded-full text-xs font-bold ${i < index ? 'bg-emerald-600 text-white' : s === step ? 'bg-[var(--brand-primary)] text-[var(--brand-on)]' : 'bg-slate-100 text-slate-600'}`}>{i < index ? '✓' : i + 1}</span>
              {t(`bpStep_${s}`)}
            </button>
          </li>)}
        </ol>
      </nav>

      <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="bp-step-title">
        <h2 id="bp-step-title" className="text-lg font-bold text-slate-900">{t(`bpStep_${step}`)}</h2>
        <p className="mt-1 text-sm text-slate-600">{t(`bpStepIntro_${step}`)}</p>
        {error && <div className="mt-4"><Notice tone="error">{error}</Notice></div>}
        <div className="mt-5">
          {step === 'term' && <div className="grid gap-3 sm:grid-cols-2">
            <label className={`${ui.label} sm:col-span-2`}>{t('bpTermName')}<input disabled={!canManage} required maxLength={120} className={ui.field} value={term.name} onChange={e => setTerm({ ...term, name: e.target.value })} /></label>
            <label className={ui.label}>{t('bpTermKind')}<select disabled={!canManage || p.term.exams > 0} className={ui.field} value={term.kind} onChange={e => setTerm({ ...term, kind: e.target.value as ExamTerm['kind'] })}><option value="term">{t('bpKindTerm')}</option><option value="annual">{t('bpKindAnnual')}</option></select></label>
            <div className="flex items-end"><TermChip status={p.term.status} /></div>
            <label className={ui.label}>{t('bpStarts')}<input disabled={!canManage} type="date" className={ui.field} value={term.starts_on || ''} onChange={e => setTerm({ ...term, starts_on: e.target.value || null })} /></label>
            <label className={ui.label}>{t('bpEnds')}<input disabled={!canManage} type="date" className={ui.field} value={term.ends_on || ''} onChange={e => setTerm({ ...term, ends_on: e.target.value || null })} /></label>
          </div>}

          {step === 'classes' && <>
            {canManage && ready.length > 0 && <button type="button" className={ui.ghost} onClick={() => setSetup({ ...setup, class_ids: ready.map(c => c.class_id) })}>{t('bpSelectReady', { count: ready.length })}</button>}
            <ul className="mt-2 divide-y divide-slate-100 rounded-xl border border-slate-200">
              {p.classes.map(c => {
                const checked = setup.class_ids.includes(c.class_id)
                return <li key={c.class_id} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between">
                  <label className={`flex items-start gap-3 ${c.state === 'ready' && canManage ? 'cursor-pointer' : 'opacity-75'}`}>
                    <input type="checkbox" className="mt-1 h-4 w-4" disabled={c.state !== 'ready' || !canManage} checked={checked}
                      onChange={e => setSetup({ ...setup, class_ids: e.target.checked ? [...setup.class_ids, c.class_id] : setup.class_ids.filter(id => id !== c.class_id) })} />
                    <span><span className="block font-semibold text-slate-900">{c.class_name}</span>
                      <span className="block text-sm text-slate-600">{c.state === 'ready' ? t('bpClassReady', { version: c.version, assessments: c.assessments.join(', '), sections: c.sections, students: c.students }) : t(`bpClassState_${c.state}`)}</span></span>
                  </label>
                  {c.state !== 'ready' && canManage && <Link to={`/admin/exams/setup?session=${session}`} className={`${ui.link} text-sm`}>{t(c.has_draft ? 'bpFinishBlueprint' : 'bpMakeBlueprint')}</Link>}
                  {c.existing > 0 && <span className="text-xs text-slate-500">{t('bpAlreadyCreated', { count: c.existing })}</span>}
                </li>
              })}
            </ul>
          </>}

          {step === 'assessments' && (p.assessments.length === 0 ? <p className="text-sm text-slate-600">{t('bpChooseClassesFirst')}</p> : <>
            <DueForAll disabled={!canManage} onApply={date => setSetup({ ...setup, due: Object.fromEntries(p.assessments.map(a => [a.key, date])) })} />
            <ul className="mt-4 divide-y divide-slate-100 rounded-xl border border-slate-200">
              {p.assessments.map(a => <li key={a.key} className="grid gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_12rem] sm:items-center">
                <div><p className="font-semibold text-slate-900">{a.name} <span className="font-mono text-xs font-normal text-slate-500">{a.key}</span></p><p className="text-sm text-slate-600">{t(`bpKind_${a.kind}`)} · {t('bpClassesCount', { count: a.classes })}</p></div>
                <label className="text-xs font-semibold text-slate-600">{t('bpDue')}<input disabled={!canManage} type="date" required className={`${ui.field} ${!setup.due[a.key] ? 'border-amber-400' : ''}`} value={setup.due[a.key] || ''} onChange={e => setSetup({ ...setup, due: { ...setup.due, [a.key]: e.target.value } })} /></label>
              </li>)}
            </ul>
          </>)}

          {step === 'sheets' && <SheetsStep p={p} setup={setup} setSetup={setSetup} canManage={canManage} />}

          {step === 'review' && <div className="space-y-4">
            <dl className="grid gap-3 sm:grid-cols-4">
              <Figure label={t('bpFigClasses')} value={setup.class_ids.length} />
              <Figure label={t('bpFigExams')} value={p.new_exams} hint={p.exams > p.new_exams ? t('bpFigExisting', { count: p.exams - p.new_exams }) : undefined} />
              <Figure label={t('bpFigSheets')} value={p.to_create} hint={p.sheets > p.to_create ? t('bpFigExisting', { count: p.sheets - p.to_create }) : undefined} />
              <Figure label={t('bpFigGaps')} value={p.gaps.filter(g => !setup.assignees[`${g.class_id}|${g.subject_id}`]).length} />
            </dl>
            {missingDue.length > 0 && <Notice tone="warning">{t('bpMissingDue', { names: missingDue.map(a => a.name).join(', ') })}</Notice>}
            {p.conflicts.length > 0 && <Notice tone="warning">{t('bpConflicts', { count: p.conflicts.length })}</Notice>}
            {p.gaps.some(g => !setup.assignees[`${g.class_id}|${g.subject_id}`]) && <Notice tone="info">{t('bpGapsLeft')}</Notice>}
            {result && <Notice tone="success">{t('bpCreated', { exams: result.exams_created, sheets: result.sheets_created, assigned: result.assigned })}</Notice>}
            {canManage && <div className="flex flex-wrap items-center gap-3">
              <button type="button" disabled={busy || blocked} aria-describedby={blocked ? 'bp-create-why' : undefined} className={ui.primary} onClick={() => void create()}>{t(p.exams > p.new_exams ? 'bpCreateMissing' : 'bpCreate')}</button>
              {blocked && <p id="bp-create-why" className="text-sm text-slate-600">{t(p.conflicts.length ? 'bpWhyConflicts' : missingDue.length ? 'bpWhyDue' : 'bpWhyClasses')}</p>}
              {(result || p.exams > 0) && <Link to={`/admin/exams/terms/${termId}/results?session=${session}`} className={ui.secondary}>{t('bpTermResults')}</Link>}
              {(result || p.exams > 0) && <Link to="/admin/exams" className={ui.ghost}>{t('bpGoToExams')}</Link>}
            </div>}
          </div>}
        </div>
        {step !== 'review' && <div className="mt-6 flex justify-between border-t border-slate-100 pt-4">
          <button type="button" className={ui.ghost} disabled={index === 0} onClick={() => setStep(steps[index - 1])}>{t('bpBack')}</button>
          <button type="button" disabled={busy || (step === 'term' && !term.name.trim())} className={ui.primary} onClick={() => void next(steps[index + 1])}>{t(canManage ? 'bpSaveContinue' : 'bpContinue')}</button>
        </div>}
      </section>
    </div>
  </div>
}

function DueForAll({ onApply, disabled }: { onApply: (date: string) => void; disabled: boolean }) {
  const { t } = useTranslation()
  const [date, setDate] = useState('')
  return <div className="flex flex-wrap items-end gap-2">
    <label className={`${ui.label} w-48`}>{t('bpDueAll')}<input disabled={disabled} type="date" className={ui.field} value={date} onChange={e => setDate(e.target.value)} /></label>
    <button type="button" disabled={disabled || !date} className={ui.secondary} onClick={() => onApply(date)}>{t('bpApplyDate')}</button>
  </div>
}

function Figure({ label, value, hint }: { label: string; value: number; hint?: string }) {
  return <div className="rounded-xl border border-slate-200 p-3"><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt><dd className="mt-1 text-2xl font-bold tabular-nums text-slate-900">{value}</dd>{hint && <dd className="text-xs text-slate-500">{hint}</dd>}</div>
}

function SheetsStep({ p, setup, setSetup, canManage }: { p: SetupPlan; setup: TermSetup; setSetup: (s: TermSetup) => void; canManage: boolean }) {
  const { t } = useTranslation()
  return <div className="space-y-4">
    <p className="text-sm text-slate-700">{t('bpSheetSummary', { sheets: p.sheets, exams: p.exams, created: p.sheets - p.to_create })}</p>
    {p.conflicts.length > 0 && <div className="rounded-xl border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
      <p className="font-semibold">{t('bpConflictTitle')}</p>
      <ul className="mt-1 list-disc pl-5">{p.conflicts.map((c, i) => <li key={i}>{t('bpConflictLine', { class: c.class_name, section: c.section_name, exam: c.exam_name })}</li>)}</ul>
    </div>}
    {p.gaps.length === 0 ? <p className="rounded-xl bg-emerald-50 px-3 py-2 text-sm font-semibold text-emerald-800">{t('bpNoGaps')}</p> : <>
      <h3 className="text-sm font-bold text-slate-900">{t('bpGapsTitle')}</h3>
      <ul className="divide-y divide-slate-100 rounded-xl border border-slate-200">
        {p.gaps.map(g => {
          const key = `${g.class_id}|${g.subject_id}`
          return <li key={key} className="grid gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_16rem] sm:items-center">
            <p><span className="font-semibold text-slate-900">{g.class_name} · {g.subject_name}</span><span className="block text-sm text-slate-600">{t('bpGapSheets', { count: g.sheets })}</span></p>
            <label className="sr-only" htmlFor={`gap-${key}`}>{t('bpAssignTeacher', { subject: g.subject_name, class: g.class_name })}</label>
            <select id={`gap-${key}`} disabled={!canManage} className="min-h-10 rounded-lg border border-slate-300 bg-white px-2 text-sm" value={setup.assignees[key] || ''}
              onChange={e => { const assignees = { ...setup.assignees }; if (e.target.value) assignees[key] = e.target.value; else delete assignees[key]; setSetup({ ...setup, assignees }) }}>
              <option value="">{t('bpNoTeacher')}</option>
              {p.teachers.map(teacher => <option key={teacher.employee_id} value={teacher.employee_id}>{teacher.name}</option>)}
            </select>
          </li>
        })}
      </ul>
    </>}
  </div>
}
