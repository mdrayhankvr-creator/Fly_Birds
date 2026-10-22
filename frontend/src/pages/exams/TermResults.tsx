import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { getClassBlueprints, getTermPreview, getExamTerms, type TermPreview, type TermSubjectResult } from '../../api/client'
import { EmptyState, Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { useSubjectName } from './setupShared'

// TermResults shows a class's provisional term results for staff: each
// subject's term mark from its contributions (CT + final, best-N …), grades,
// GPA, outcome and positions, with the sheets still awaiting approval.
// Nothing here is published; students see term results from E6.
export function TermResults() {
  const { t } = useTranslation()
  const { termId = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const authorized = usePeopleApi()
  const session = params.get('session') || ''
  const terms = useQuery({ queryKey: ['e5', 'terms', session], enabled: !!session, queryFn: () => authorized(token => getExamTerms(token, session)), retry: false })
  const classes = useQuery({ queryKey: ['e5', 'blueprints', session], enabled: !!session, queryFn: () => authorized(token => getClassBlueprints(token, session)), retry: false })
  const term = terms.data?.find(x => x.id === termId)
  const withBlueprint = (classes.data || []).filter(c => c.active || c.versions.length > 0)
  const classId = params.get('class') || withBlueprint[0]?.class_id || ''
  const preview = useQuery({ queryKey: ['e5', 'preview', termId, classId], enabled: !!classId, queryFn: () => authorized(token => getTermPreview(token, termId, classId)), retry: false, gcTime: 0 })
  const choose = (id: string) => setParams(prev => { const next = new URLSearchParams(prev); next.set('class', id); return next }, { replace: true })

  return <div className="space-y-5">
    <PageHeader eyebrow={t('bpSetupTitle')} title={t('bpResultsTitle', { name: term?.name || '' })} description={t('bpResultsIntro')}
      actions={<Link to={`/admin/exams/setup?session=${session}`} className={ui.secondary}>{t('bpBackToSetup')}</Link>} />
    <label className={`${ui.label} max-w-xs`}>{t('bpClass')}
      <select className={ui.field} value={classId} onChange={e => choose(e.target.value)}>
        {withBlueprint.map(c => <option key={c.class_id} value={c.class_id}>{c.class_name}</option>)}
      </select>
    </label>
    {preview.isPending ? <Skeleton rows={6} /> : preview.isError ? <Notice tone="error">{t('bpErrGeneric')}</Notice> : <PreviewBody p={preview.data} />}
  </div>
}

function PreviewBody({ p }: { p: TermPreview }) {
  const { t } = useTranslation()
  const subjectName = useSubjectName()
  const [open, setOpen] = useState('')
  if (p.state === 'no_exams') return <EmptyState icon="grade" title={t('bpPreviewNoExams')}>{t('bpPreviewNoExamsHint')}</EmptyState>
  if (p.state === 'mixed_versions') return <Notice tone="warning">{t('bpPreviewMixed')}</Notice>
  // Columns: graded lines in blueprint order (combined papers as one).
  const first = p.students.find(s => s.result.subjects.length > 0)
  const columns = (first?.result.subjects || []).map(s => s.combined_key || s.subject_ids[0])
  const allColumns = Array.from(new Set(p.students.flatMap(s => s.result.subjects.map(x => x.combined_key || x.subject_ids[0]))))
  const ordered = [...columns, ...allColumns.filter(c => !columns.includes(c))]
  const label = (key: string) => {
    const ref = p.subjects.find(s => s.id === key)
    if (ref) return subjectName(ref)
    const papers = p.students.flatMap(s => s.result.subjects).find(x => x.combined_key === key)?.subject_ids || []
    return papers.map(id => subjectName(p.subjects.find(s => s.id === id))).join(' + ') || key
  }
  const cell = (subjects: TermSubjectResult[], key: string) => subjects.find(x => (x.combined_key || x.subject_ids[0]) === key)
  const assessments = p.definition?.assessments || []
  const assessmentName = (key: string) => assessments.find(a => a.key === key)?.name || key

  return <>
    <section className={`${ui.card} flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between`} aria-label={t('bpReadiness')}>
      <div>
        <p className="font-semibold text-slate-900">{t(p.state === 'ready' ? 'bpPreviewReady' : 'bpPreviewPending')}</p>
        <p className="text-sm text-slate-600">{t('bpSheetsApproved', { approved: p.approved, sheets: p.sheets })} · {t('bpVersionUsed', { version: p.version })}</p>
      </div>
      <dl className="flex flex-wrap gap-4 text-sm">{(['passed', 'failed', 'incomplete', 'withheld'] as const).map(o => <div key={o}><dt className="text-xs text-slate-500">{t(`bpOutcome_${o}`)}</dt><dd className="text-lg font-bold tabular-nums text-slate-900">{p.outcomes[o] || 0}</dd></div>)}</dl>
    </section>
    {p.pending.length > 0 && <details className={`${ui.card} p-4`}>
      <summary className="cursor-pointer font-semibold text-slate-900">{t('bpPendingSheets', { count: p.pending.length })}</summary>
      <ul className="mt-3 divide-y divide-slate-100 text-sm">{p.pending.map(s => <li key={s.sheet_id} className="flex flex-wrap items-center justify-between gap-2 py-2">
        <span>{s.section_name} · {assessmentName(s.assessment)} · {s.subject_name} <span className="text-slate-500">· {t(`e3SheetStatus_${s.status}`)}{s.missing ? ` · ${t('bpMissingCount', { count: s.missing })}` : ''}</span></span>
        <Link to={`/admin/exams/${s.exam_id}/sheets/${s.sheet_id}`} className={`${ui.link} text-sm`}>{t('mwOpen')}</Link>
      </li>)}</ul>
    </details>}

    {/* Nothing provisional looks final: dashed frame and a label (UX spec §1.6). */}
    <section aria-labelledby="bp-preview-title" className="relative rounded-2xl border-2 border-dashed border-indigo-300 bg-white p-3 sm:p-4">
      <p id="bp-preview-title" className="mb-3 inline-flex items-center rounded-full bg-indigo-50 px-3 py-1 text-xs font-bold uppercase tracking-wide text-indigo-800 ring-1 ring-inset ring-indigo-600/20">{t('bpProvisional')}</p>
      {/* Wide screens: a tabulation with the student column pinned. */}
      <div className="hidden max-h-[70vh] overflow-auto md:block" tabIndex={0} role="region" aria-label={t('bpResultsTable')}>
        <table className="w-full border-separate border-spacing-0 text-left text-sm">
          <thead><tr>
            <th scope="col" className={`${ui.th} sticky left-0 top-0 z-20`}>{t('bpStudent')}</th>
            {ordered.map(k => <th key={k} scope="col" className={`${ui.th} sticky top-0 z-10 text-right`}>{label(k)}</th>)}
            <th scope="col" className={`${ui.th} sticky top-0 z-10 text-right`}>{t('bpGpa')}</th>
            <th scope="col" className={`${ui.th} sticky top-0 z-10`}>{t('bpOutcome')}</th>
            <th scope="col" className={`${ui.th} sticky top-0 z-10 text-right`}>{t('bpPositions')}</th>
          </tr></thead>
          <tbody>{p.students.map(s => <tr key={s.student_id} className="group">
            <th scope="row" className="sticky left-0 z-[1] min-w-[12rem] border-b border-slate-100 bg-white px-3 py-2 text-left font-medium group-hover:bg-slate-50">{s.name}<span className="block text-xs font-normal text-slate-500">{s.section_name} · {s.roll_no ?? '—'}</span></th>
            {ordered.map(k => { const c = cell(s.result.subjects, k); return <td key={k} className="whitespace-nowrap border-b border-slate-100 px-3 py-2 text-right tabular-nums group-hover:bg-slate-50" title={c ? explainCell(c, assessmentName, t) : undefined}><SubjectCell c={c} /></td> })}
            <td className="border-b border-slate-100 px-3 py-2 text-right font-semibold tabular-nums group-hover:bg-slate-50">{s.result.gpa || '—'}</td>
            <td className="border-b border-slate-100 px-3 py-2 group-hover:bg-slate-50"><Outcome o={s.result.outcome} /></td>
            <td className="border-b border-slate-100 px-3 py-2 text-right tabular-nums text-slate-600 group-hover:bg-slate-50">{s.result.class_position ? `${s.result.class_position} · ${s.result.section_position}` : '—'}</td>
          </tr>)}</tbody>
        </table>
      </div>
      {/* Phones: one card per student, subjects listed. */}
      <ul className="space-y-3 md:hidden">{p.students.map(s => <li key={s.student_id} className="rounded-xl border border-slate-200 p-3">
        <button type="button" aria-expanded={open === s.student_id} onClick={() => setOpen(open === s.student_id ? '' : s.student_id)} className="flex w-full items-start justify-between gap-2 text-left">
          <span><span className="block font-semibold text-slate-900">{s.name}</span><span className="block text-xs text-slate-500">{s.section_name} · {s.roll_no ?? '—'}</span></span>
          <span className="text-right"><span className="block text-lg font-bold tabular-nums text-slate-900">{s.result.gpa || '—'}</span><Outcome o={s.result.outcome} /></span>
        </button>
        {open === s.student_id && <dl className="mt-3 divide-y divide-slate-100 text-sm">{ordered.map(k => { const c = cell(s.result.subjects, k); return <div key={k} className="flex justify-between gap-2 py-1.5"><dt className="text-slate-600">{label(k)}</dt><dd className="tabular-nums"><SubjectCell c={c} /></dd></div> })}</dl>}
      </li>)}</ul>
    </section>
  </>
}

function explainCell(c: TermSubjectResult, name: (key: string) => string, t: (key: string, o?: Record<string, unknown>) => string) {
  const parts = c.contributions.map(x => `${x.from.map(name).join(', ')}: ${x.exempt ? t('bpExempt') : `${x.value} / ${x.worth}`}`)
  const reasons = c.reasons.map(r => t(`bpReason_${r.code}`, { assessment: r.assessment ? name(r.assessment) : '', component: r.component }))
  return [...parts, ...reasons].join(' · ')
}

function SubjectCell({ c }: { c?: TermSubjectResult }) {
  const { t } = useTranslation()
  if (!c) return <span className="text-slate-500">—</span>
  if (c.status !== 'graded') return <span className="font-semibold text-slate-600">{t(`bpLine_${c.status}`)}</span>
  return <span className={c.passed ? 'text-slate-900' : 'font-semibold text-rose-700'}>{c.marks} <span className="text-xs font-semibold">{c.grade}</span>{c.optional && <span className="ml-1 text-xs text-slate-500">({t('bpOptionalShort')})</span>}</span>
}

function Outcome({ o }: { o: TermPreview['students'][number]['result']['outcome'] }) {
  const { t } = useTranslation()
  const tone = { passed: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20', failed: 'bg-rose-50 text-rose-800 ring-rose-600/20', incomplete: 'bg-amber-50 text-amber-900 ring-amber-600/25', withheld: 'bg-slate-100 text-slate-700 ring-slate-500/20' }[o]
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${tone}`}>{t(`bpOutcome_${o}`)}</span>
}
