import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  ApiError, activateBlueprint, deleteBlueprintDraft, getBlueprintVersion, newBlueprintVersion, sampleBlueprintSubject, saveBlueprintDraft, setAssessmentVisibility,
  type AssessmentKind, type BlueprintComponent, type BlueprintContribution, type BlueprintDefinition, type BlueprintIssue, type BlueprintSheet, type BlueprintSubject,
  type BlueprintTermRule, type BlueprintVersion, type CombineMethod, type SampleMark, type StudentVisibility, type SubjectRole, type TermSubjectResult,
} from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { CardTitle, Notice, PageHeader, Skeleton, Tabs, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import {
  addAssessment, applyToAll, componentPresets, defaultPresetOptions, issueSubject, presetOf, presetRule, removeAssessment, setTermRule, sheetFull, sumMarks, twoDecimals,
  type Preset, type PresetOptions,
} from './blueprintModel'
import { VersionChip, useExplain, useSubjectName } from './setupShared'

type Pane = 'structure' | 'rules' | 'check'
type SaveState = 'idle' | 'saving' | 'saved' | 'error'

const kinds: AssessmentKind[] = ['class_test', 'continuous', 'performance', 'term_exam']
const visibilities: StudentVisibility[] = ['hidden', 'after_term_publication', 'with_assessment_publication']
const combines: CombineMethod[] = ['single', 'sum', 'average', 'best', 'latest']
const num = (v: string) => (v.trim() === '' ? NaN : Number(v.replace(',', '.')))

// BlueprintBuilder edits one blueprint version: assessments by term, each
// subject's sheets, components and term rule, and the school's policies,
// with the server's validation and a worked sample beside them. A draft
// saves itself; an active version is read-only.
export function BlueprintBuilder({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const { versionId = '' } = useParams()
  const [params] = useSearchParams()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const client = useQueryClient()
  const navigate = useNavigate()
  const subjectName = useSubjectName()
  const version = useQuery({ queryKey: ['e5', 'version', versionId], queryFn: () => authorized(token => getBlueprintVersion(token, versionId)), retry: false, gcTime: 0 })
  const [def, setDef] = useState<BlueprintDefinition | null>(null)
  const [issues, setIssues] = useState<BlueprintIssue[]>([])
  const [save, setSave] = useState<SaveState>('idle')
  const [subjectIndex, setSubjectIndex] = useState(0)
  const [termId, setTermId] = useState('')
  const [pane, setPane] = useState<Pane>('rules')
  const [notice, setNotice] = useState<{ tone: 'success' | 'error' | 'info'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  const pending = useRef<BlueprintDefinition | null>(null)
  const v = version.data
  const editable = canManage && v?.status === 'draft'
  const session = params.get('session') || v?.session_id || ''

  useEffect(() => {
    if (!v) return
    setDef(structuredClone(v.definition))
    setIssues(v.issues)
    const firstTerm = v.terms.find(term => v.definition.assessments.some(a => a.term_id === term.id))
    setTermId(prev => prev || firstTerm?.id || v.terms[0]?.id || '')
  }, [v])

  // Drafts save themselves a moment after each change; the reply carries the
  // validation issues of what was saved.
  function change(next: BlueprintDefinition) {
    setDef(next)
    if (!editable) return
    pending.current = next
    setSave('saving')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => void flush(), 700)
  }
  async function flush() {
    const body = pending.current
    if (!body) return
    pending.current = null
    try {
      const saved = await authorized(token => saveBlueprintDraft(token, versionId, body))
      if (!pending.current) { setIssues(saved.issues); setSave('saved') }
    } catch (cause) { setSave('error'); setNotice({ tone: 'error', text: explain(cause) }) }
  }
  useEffect(() => () => window.clearTimeout(timer.current), [])

  async function activate() {
    window.clearTimeout(timer.current)
    await flush()
    setBusy(true); setNotice(null)
    try {
      await authorized(token => activateBlueprint(token, versionId))
      await client.invalidateQueries({ queryKey: ['e5'] })
      setNotice({ tone: 'success', text: t('bpActivated') })
    } catch (cause) {
      if (cause instanceof ApiError && cause.code === 'blueprint_invalid') {
        const fresh = await authorized(token => getBlueprintVersion(token, versionId))
        setIssues(fresh.issues); setPane('check')
        setNotice({ tone: 'error', text: t('bpActivateRefused', { count: fresh.issues.length }) })
      } else setNotice({ tone: 'error', text: explain(cause) })
    } finally { setBusy(false) }
  }
  async function discard() {
    if (!await confirmAction(t('bpDiscardConfirm'))) return
    setBusy(true)
    try { await authorized(token => deleteBlueprintDraft(token, versionId)); await client.invalidateQueries({ queryKey: ['e5'] }); navigate(`/admin/exams/setup?session=${session}`) }
    catch (cause) { setNotice({ tone: 'error', text: explain(cause) }); setBusy(false) }
  }
  async function newVersion() {
    if (!v) return
    setBusy(true)
    try { const next = await authorized(token => newBlueprintVersion(token, v.blueprint_id, v.id)); await client.invalidateQueries({ queryKey: ['e5'] }); navigate(`/admin/exams/blueprints/${next.id}?session=${session}`) }
    catch (cause) { setNotice({ tone: 'error', text: explain(cause) }); setBusy(false) }
  }
  async function visibility(key: string, value: StudentVisibility) {
    // On an active version, visibility is the one thing that may change.
    if (editable && def) { change({ ...def, assessments: def.assessments.map(a => (a.key === key ? { ...a, visibility: value } : a)) }); return }
    try { await authorized(token => setAssessmentVisibility(token, versionId, key, value)); await client.invalidateQueries({ queryKey: ['e5', 'version', versionId] }); setNotice({ tone: 'success', text: t('bpVisibilitySaved') }) }
    catch (cause) { setNotice({ tone: 'error', text: explain(cause) }) }
  }

  if (version.isPending || (v && !def)) return <Skeleton rows={6} />
  if (version.isError || !v || !def) return <Notice tone="error">{t('bpErrGeneric')}</Notice>
  const subject = def.subjects[subjectIndex]
  const ref = v.subjects.find(s => s.id === subject?.subject_id)
  const termsWith = v.terms.filter(term => def.assessments.some(a => a.term_id === term.id))
  // The chosen term, or the first with assessments once it has none left.
  const activeTerm = termsWith.some(term => term.id === termId) ? termId : termsWith[0]?.id || ''

  return <div className="space-y-5">
    <PageHeader eyebrow={t('bpSetupTitle')} title={`${v.class_name} · ${v.session_name}`} description={t(editable ? 'bpBuilderIntro' : 'bpBuilderReadOnly')}
      actions={<Link to={`/admin/exams/setup?session=${session}`} className={ui.secondary}>{t('bpBackToSetup')}</Link>} />
    <section className={`${ui.card} flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between`}>
      <div className="flex flex-wrap items-center gap-2">
        <VersionChip version={v} />
        {v.activated_at && <span className="text-sm text-slate-600">{t('bpActivatedAt', { time: v.activated_at })}</span>}
        {editable && <span data-save-state={save} aria-live="polite" className="text-sm text-slate-600">{t(`bpSave_${save}`)}</span>}
        {v.status === 'draft' && <span className="text-sm text-slate-600">{t(issues.length ? 'bpIssueCount' : 'bpNoIssues', { count: issues.length })}</span>}
      </div>
      <div className="flex flex-wrap gap-2">
        {editable && <button type="button" disabled={busy} className={ui.danger} onClick={() => void discard()}>{t('bpDiscard')}</button>}
        {editable && <button type="button" disabled={busy || save === 'saving' || issues.length > 0} title={issues.length ? t('bpActivateBlocked') : undefined} className={ui.primary} onClick={() => void activate()}>{t('bpActivate')}</button>}
        {canManage && v.status === 'active' && !v.versions.some(x => x.status === 'draft') && <button type="button" disabled={busy} className={ui.primary} onClick={() => void newVersion()}>{t('bpNewVersion')}</button>}
      </div>
    </section>
    {editable && issues.length > 0 && <p id="bp-activate-why" className="-mt-3 text-sm text-slate-600">{t('bpActivateBlocked')}</p>}
    {v.legacy && <Notice tone="info">{t('bpLegacyNote')}</Notice>}
    {notice && <Notice tone={notice.tone}>{notice.text}</Notice>}

    <div className="lg:hidden"><Tabs label={t('bpBuilder')} value={pane} onChange={setPane} items={[{ value: 'structure', label: t('bpPaneStructure') }, { value: 'rules', label: t('bpPaneRules') }, { value: 'check', label: t('bpPaneCheck'), badge: issues.length ? <span className="rounded-full bg-rose-600 px-1.5 text-xs text-white">{issues.length}</span> : undefined }]} /></div>
    <div className="grid gap-5 lg:grid-cols-[19rem_minmax(0,1fr)_21rem] lg:items-start">
      <div className={`${pane === 'structure' ? '' : 'hidden'} space-y-5 lg:block`}>
        <Structure v={v} def={def} editable={editable} change={change} onVisibility={visibility} canChangeVisibility={canManage && v.status === 'active'} />
        <SubjectList v={v} def={def} issues={issues} selected={subjectIndex} onSelect={i => { setSubjectIndex(i); setPane('rules') }} editable={editable} change={change} />
      </div>
      <div className={`${pane === 'rules' ? '' : 'hidden'} min-w-0 lg:block`}>
        {/* Keyed by subject and term, so a Custom choice belongs to one rule. */}
        {subject && <SubjectRules key={`${subject.subject_id}|${activeTerm}`} v={v} def={def} index={subjectIndex} name={subjectName(ref)} fourth={!!ref?.fourth} editable={editable} change={change}
          terms={termsWith} termId={activeTerm} onTerm={setTermId} issues={issues} />}
      </div>
      <div className={`${pane === 'check' ? '' : 'hidden'} space-y-5 lg:block`}>
        <Check v={v} def={def} issues={issues} onPick={issue => { const i = issueSubject(issue); if (i != null) { setSubjectIndex(i); setPane('rules') } }} />
        {subject && activeTerm && subject.role !== 'non_graded' && <Sample versionId={versionId} def={def} subject={subject} termId={activeTerm} draft={editable} />}
        <Policies def={def} terms={v.terms} editable={editable} change={change} />
        <Bands v={v} />
      </div>
    </div>
  </div>
}

// ---- structure: terms and their assessments ----------------------------------

function Structure({ v, def, editable, change, onVisibility, canChangeVisibility }: {
  v: BlueprintVersion; def: BlueprintDefinition; editable: boolean; change: (d: BlueprintDefinition) => void
  onVisibility: (key: string, value: StudentVisibility) => void; canChangeVisibility: boolean
}) {
  const { t } = useTranslation()
  const rename = (key: string, name: string) => change({ ...def, assessments: def.assessments.map(a => (a.key === key ? { ...a, name } : a)) })
  const setKind = (key: string, kind: AssessmentKind) => change({ ...def, assessments: def.assessments.map(a => (a.key === key ? { ...a, kind } : a)) })
  return <section className={`${ui.card} p-4`} aria-labelledby="bp-structure">
    <CardTitle id="bp-structure" icon="layers">{t('bpAssessments')}</CardTitle>
    {v.terms.length === 0 && <p className="mt-2 text-sm text-slate-600">{t('bpNoTermsForBlueprint')}</p>}
    <ol className="mt-3 space-y-4">
      {/* Terms the upgrade made belong to the legacy blueprint: shown only when used. */}
      {v.terms.filter(term => !term.legacy || def.assessments.some(a => a.term_id === term.id)).map(term => {
        const list = def.assessments.filter(a => a.term_id === term.id)
        return <li key={term.id}>
          <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{term.name}</p>
          <ul className="mt-1.5 space-y-2">
            {list.map(a => <li key={a.key} className="rounded-xl border border-slate-200 p-2.5">
              <div className="flex items-center gap-2">
                <span className="rounded-md bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-slate-700">{a.key}</span>
                <input aria-label={t('bpAssessmentName', { key: a.key })} disabled={!editable} maxLength={80} value={a.name} onChange={e => rename(a.key, e.target.value)}
                  className="min-h-9 min-w-0 flex-1 rounded-lg border border-slate-300 bg-white px-2 text-sm disabled:border-transparent disabled:bg-transparent disabled:font-semibold disabled:text-slate-900" />
                {editable && <button type="button" aria-label={t('bpRemoveAssessment', { name: a.name })} className="grid h-9 w-9 place-items-center rounded-lg text-slate-500 hover:bg-rose-50 hover:text-rose-700"
                  onClick={() => void (async () => { if (await confirmAction(t('bpRemoveAssessmentConfirm', { name: a.name }))) change(removeAssessment(def, a.key)) })()}>×</button>}
              </div>
              <div className="mt-2 grid grid-cols-2 gap-2">
                <label className="text-xs text-slate-600">{t('bpKind')}
                  <select disabled={!editable} value={a.kind} onChange={e => setKind(a.key, e.target.value as AssessmentKind)} className="mt-1 block min-h-9 w-full rounded-lg border border-slate-300 bg-white px-1.5 text-sm disabled:bg-slate-50">
                    {kinds.map(k => <option key={k} value={k}>{t(`bpKind_${k}`)}</option>)}
                  </select>
                </label>
                <label className="text-xs text-slate-600">{t('bpVisibility')}
                  <select disabled={!editable && !canChangeVisibility} value={a.visibility} onChange={e => onVisibility(a.key, e.target.value as StudentVisibility)} className="mt-1 block min-h-9 w-full rounded-lg border border-slate-300 bg-white px-1.5 text-sm disabled:bg-slate-50">
                    {visibilities.map(x => <option key={x} value={x}>{t(`bpVis_${x}`)}</option>)}
                  </select>
                </label>
              </div>
            </li>)}
          </ul>
          {editable && <div className="mt-2 flex flex-wrap gap-1">
            <button type="button" className={ui.ghost} onClick={() => change(addAssessment(def, term.id, 'class_test', t('bpNewClassTest', { n: list.filter(a => a.kind === 'class_test').length + 1 })))}>+ {t('bpAddClassTest')}</button>
            {!list.some(a => a.kind === 'term_exam') && <button type="button" className={ui.ghost} onClick={() => change(addAssessment(def, term.id, 'term_exam', term.name))}>+ {t('bpAddTermExam')}</button>}
          </div>}
        </li>
      })}
    </ol>
  </section>
}

function SubjectList({ v, def, issues, selected, onSelect, editable, change }: { v: BlueprintVersion; def: BlueprintDefinition; issues: BlueprintIssue[]; selected: number; onSelect: (i: number) => void; editable: boolean; change: (d: BlueprintDefinition) => void }) {
  const { t } = useTranslation()
  const subjectName = useSubjectName()
  // Class subjects the design does not cover yet (e.g. added after it was made).
  const missing = v.subjects.filter(s => !def.subjects.some(x => x.subject_id === s.id))
  return <section className={`${ui.card} p-4`} aria-labelledby="bp-subjects">
    <CardTitle id="bp-subjects" icon="book">{t('bpSubjects')}</CardTitle>
    <ul className="mt-3 space-y-1">
      {def.subjects.map((s, i) => {
        const ref = v.subjects.find(x => x.id === s.subject_id)
        const count = issues.filter(issue => issueSubject(issue) === i).length
        return <li key={s.subject_id}>
          <button type="button" data-subject-id={s.subject_id} aria-current={selected === i ? 'true' : undefined} onClick={() => onSelect(i)}
            className={`flex w-full items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${selected === i ? 'bg-[var(--brand-soft)] font-semibold text-slate-900' : 'text-slate-700 hover:bg-slate-50'}`}>
            <span className="min-w-0 truncate">{subjectName(ref) || s.subject_id}{s.combined_key && <span className="ml-1.5 rounded bg-slate-100 px-1 font-mono text-xs text-slate-600">{s.combined_key}</span>}</span>
            <span className="flex shrink-0 items-center gap-1.5">
              {s.role !== 'required' && <span className="text-xs text-slate-500">{t(`bpRole_${s.role}`)}</span>}
              {count > 0 && <span className="rounded-full bg-rose-600 px-1.5 text-xs font-semibold text-white" aria-label={t('bpIssueCount', { count })}>{count}</span>}
            </span>
          </button>
        </li>
      })}
    </ul>
    {editable && missing.length > 0 && <div className="mt-3 border-t border-slate-100 pt-3">
      <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('bpMissingSubjects')}</p>
      <div className="mt-1.5 flex flex-wrap gap-1">{missing.map(s => <button key={s.id} type="button" className={ui.ghost}
        onClick={() => { change({ ...def, subjects: [...def.subjects, { subject_id: s.id, role: s.fourth ? 'optional' : 'required', sheets: [], terms: [] }] }); onSelect(def.subjects.length) }}>+ {subjectName(s)}</button>)}</div>
    </div>}
  </section>
}

// ---- rules of one subject ----------------------------------------------------

function SubjectRules({ v, def, index, name, fourth, editable, change, terms, termId, onTerm, issues }: {
  v: BlueprintVersion; def: BlueprintDefinition; index: number; name: string; fourth: boolean; editable: boolean; change: (d: BlueprintDefinition) => void
  terms: BlueprintVersion['terms']; termId: string; onTerm: (id: string) => void; issues: BlueprintIssue[]
}) {
  const { t } = useTranslation()
  const subject = def.subjects[index]
  const rule = subject.terms.find(x => x.term_id === termId)
  // No rule yet: no preset is chosen.
  // Custom is a way of editing: chosen, it stays even while the rule still
  // matches a preset, so a preset can be adjusted contribution by contribution.
  const [custom, setCustom] = useState(false)
  const preset = custom && rule ? 'custom' : rule ? presetOf(rule, def) : null
  const [options, setOptions] = useState<PresetOptions>(defaultPresetOptions)
  const update = (s: BlueprintSubject) => change({ ...def, subjects: def.subjects.map((x, i) => (i === index ? s : x)) })
  const setRule = (r: BlueprintTermRule) => update(setTermRule(subject, r, def))
  const choose = (p: Preset, opts = options) => {
    setCustom(p === 'custom')
    if (p === 'custom') { if (!rule) setRule({ term_id: termId, full: 100, pass: 33, contributions: [] }); return }
    const r = presetRule(def, termId, p, opts)
    if (r) setRule(r)
  }
  const termAssessments = def.assessments.filter(a => a.term_id === termId)
  const usedSheets = subject.sheets.filter(s => termAssessments.some(a => a.key === s.assessment))
  const subjectIssues = issues.filter(i => issueSubject(i) === index)

  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="bp-rules">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0"><h2 id="bp-rules" className="text-lg font-bold text-slate-900">{name}</h2><p className="text-sm text-slate-600">{t('bpRulesIntro')}</p></div>
      {editable && subject.role !== 'non_graded' && rule && <button type="button" className={ui.secondary} onClick={() => void (async () => { if (await confirmAction(t('bpApplyAllConfirm', { name }), { tone: 'default' })) change(applyToAll(def, subject.subject_id, termId)) })()}>{t('bpApplyAll')}</button>}
    </div>
    <div className="mt-4 grid gap-3 sm:grid-cols-2">
      <label className={ui.label}>{t('bpRole')}
        <select disabled={!editable} className={ui.field} value={subject.role} onChange={e => {
          const role = e.target.value as SubjectRole
          update(role === 'non_graded' ? { ...subject, role, sheets: [], terms: [], combined_key: undefined } : { ...subject, role })
        }}>
          {(['required', 'optional', 'non_graded'] as const).filter(r => r !== (fourth ? 'required' : 'optional')).map(r => <option key={r} value={r}>{t(`bpRole_${r}`)}</option>)}
        </select>
      </label>
      {subject.role !== 'non_graded' && <label className={ui.label}>{t('bpCombined')}
        <input disabled={!editable} maxLength={16} placeholder={t('bpCombinedHint')} className={`${ui.field} font-mono uppercase`} value={subject.combined_key || ''}
          onChange={e => update({ ...subject, combined_key: e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '') || undefined })} />
      </label>}
    </div>
    {subjectIssues.length > 0 && <ul className="mt-3 space-y-1 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-900">{subjectIssues.map((issue, i) => <li key={i}><IssueText issue={issue} v={v} /></li>)}</ul>}
    {subject.role === 'non_graded' ? <p className="mt-4 text-sm text-slate-600">{t('bpNonGradedNote')}</p> : <>
      {terms.length > 1 && <div className="mt-4"><Tabs label={t('bpTerm')} value={termId} onChange={onTerm} items={terms.map(term => ({ value: term.id, label: term.name }))} /></div>}
      <fieldset className="mt-4">
        <legend className={ui.label}>{t('bpTermMark')}</legend>
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          {(['final_only', 'ct_final', 'best_ct_final', 'custom'] as const).map(p => {
            const possible = p === 'custom' || presetRule(def, termId, p, options) !== null
            return <label key={p} className={`flex cursor-pointer items-start gap-2.5 rounded-xl border p-3 text-sm transition ${preset === p ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)]' : 'border-slate-200 hover:border-slate-300'} ${!possible || !editable ? 'cursor-not-allowed opacity-60' : ''}`}>
              <input type="radio" name="bp-preset" className="mt-0.5" checked={preset === p} disabled={!possible || !editable} onChange={() => choose(p)} />
              <span><span className="block font-semibold text-slate-900">{t(`bpPreset_${p}`)}</span><span className="block text-xs text-slate-600">{t(`bpPresetHint_${p}`)}</span></span>
            </label>
          })}
        </div>
      </fieldset>
      {(preset === 'ct_final' || preset === 'best_ct_final') && editable && <div className="mt-3 grid grid-cols-3 gap-3">
        <NumberField label={t('bpCtWorth')} value={options.ctWorth} onChange={n => { const o = { ...options, ctWorth: n }; setOptions(o); choose(preset, o) }} />
        <NumberField label={t('bpFinalWorth')} value={options.finalWorth} onChange={n => { const o = { ...options, finalWorth: n }; setOptions(o); choose(preset, o) }} />
        {preset === 'best_ct_final' && <NumberField label={t('bpBestN')} value={options.best} step="1" onChange={n => { const o = { ...options, best: n }; setOptions(o); choose(preset, o) }} />}
      </div>}
      {rule && <Contributions def={def} rule={rule} editable={editable && preset === 'custom'} termAssessments={termAssessments} onChange={setRule} />}
      {rule && <div className="mt-3 grid grid-cols-2 gap-3 sm:max-w-sm">
        <NumberField label={t('bpTermFull')} value={rule.full} disabled={!editable || preset !== 'custom'} onChange={n => setRule({ ...rule, full: n })} />
        <NumberField label={t('bpTermPass')} value={rule.pass} disabled={!editable} onChange={n => setRule({ ...rule, pass: n })} />
      </div>}
      <h3 className="mt-6 text-sm font-bold text-slate-900">{t('bpSheets')}</h3>
      {usedSheets.length === 0 ? <p className="mt-1 text-sm text-slate-600">{t('bpNoSheets')}</p> : <div className="mt-2 space-y-3">
        {usedSheets.map(sheet => <SheetEditor key={sheet.assessment} sheet={sheet} name={def.assessments.find(a => a.key === sheet.assessment)?.name || sheet.assessment} editable={editable}
          onChange={next => update({ ...subject, sheets: subject.sheets.map(s => (s.assessment === sheet.assessment ? next : s)) })} />)}
      </div>}
    </>}
  </section>
}

function NumberField({ label, value, onChange, disabled, step = '0.01' }: { label: string; value: number; onChange: (n: number) => void; disabled?: boolean; step?: string }) {
  const [text, setText] = useState(String(value))
  useEffect(() => setText(String(value)), [value])
  return <label className="block text-xs font-semibold text-slate-600">{label}
    <input type="number" inputMode="decimal" min="0" step={step} disabled={disabled} value={text}
      onChange={e => { setText(e.target.value); const n = num(e.target.value); if (Number.isFinite(n)) onChange(n) }}
      className="mt-1 block min-h-10 w-full rounded-lg border border-slate-300 bg-white px-2.5 text-right text-sm tabular-nums disabled:bg-slate-50" />
  </label>
}

function Contributions({ def, rule, editable, termAssessments, onChange }: {
  def: BlueprintDefinition; rule: BlueprintTermRule; editable: boolean; termAssessments: BlueprintDefinition['assessments']; onChange: (r: BlueprintTermRule) => void
}) {
  const { t } = useTranslation()
  const set = (i: number, c: BlueprintContribution) => onChange({ ...rule, contributions: rule.contributions.map((x, j) => (j === i ? c : x)) })
  const total = sumMarks(rule.contributions.map(c => c.worth))
  const name = (key: string) => def.assessments.find(a => a.key === key)?.name || key
  return <div className="mt-4 rounded-xl border border-slate-200">
    <ul className="divide-y divide-slate-100">
      {rule.contributions.map((c, i) => <li key={i} className="grid gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_9rem_6rem_auto] sm:items-end">
        {editable
          ? <fieldset><legend className="text-xs font-semibold text-slate-600">{t('bpFrom')}</legend><div className="mt-1 flex flex-wrap gap-2">{termAssessments.map(a => <label key={a.key} className="flex items-center gap-1.5 text-sm"><input type="checkbox" checked={c.from.includes(a.key)} onChange={e => set(i, { ...c, from: e.target.checked ? [...c.from, a.key] : c.from.filter(f => f !== a.key) })} />{a.name}</label>)}</div></fieldset>
          : <p className="text-sm"><span className="font-semibold text-slate-900">{c.from.map(name).join(', ')}</span><span className="block text-xs text-slate-600">{t(`bpCombine_${c.combine}`, { n: c.best })}</span></p>}
        {editable && <label className="text-xs font-semibold text-slate-600">{t('bpCombine')}
          <select value={c.combine} onChange={e => set(i, { ...c, combine: e.target.value as CombineMethod, best: e.target.value === 'best' ? Math.max(1, c.best || 1) : undefined })} className="mt-1 block min-h-10 w-full rounded-lg border border-slate-300 bg-white px-1.5 text-sm">
            {combines.map(x => <option key={x} value={x}>{t(`bpCombineName_${x}`)}</option>)}
          </select>
        </label>}
        {editable && c.combine === 'best' && <NumberField label={t('bpBestN')} step="1" value={c.best || 1} onChange={n => set(i, { ...c, best: Math.round(n) })} />}
        {editable ? <NumberField label={t('bpWorth')} value={c.worth} onChange={n => set(i, { ...c, worth: n })} /> : <p className="text-right text-sm tabular-nums text-slate-900">{t('bpWorthValue', { worth: twoDecimals(c.worth) })}</p>}
        {editable && <button type="button" aria-label={t('bpRemoveContribution')} className="grid h-10 w-10 place-items-center rounded-lg text-slate-500 hover:bg-rose-50 hover:text-rose-700" onClick={() => onChange({ ...rule, contributions: rule.contributions.filter((_, j) => j !== i) })}>×</button>}
      </li>)}
    </ul>
    <div className="flex flex-wrap items-center justify-between gap-2 border-t border-slate-100 px-3 py-2 text-sm">
      <span className={total === rule.full ? 'text-emerald-800' : 'font-semibold text-rose-700'}>{t('bpWorthTotal', { sum: twoDecimals(total), full: twoDecimals(rule.full) })}</span>
      {editable && <button type="button" className={ui.ghost} onClick={() => onChange({ ...rule, contributions: [...rule.contributions, { from: [], combine: 'single', worth: 0 }] })}>+ {t('bpAddContribution')}</button>}
    </div>
  </div>
}

function SheetEditor({ sheet, name, editable, onChange }: { sheet: BlueprintSheet; name: string; editable: boolean; onChange: (s: BlueprintSheet) => void }) {
  const { t } = useTranslation()
  const setComp = (i: number, c: BlueprintComponent) => onChange({ ...sheet, components: sheet.components.map((x, j) => (j === i ? c : x)) })
  return <div className="rounded-xl border border-slate-200 p-3">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p className="font-semibold text-slate-900">{name} <span className="font-normal tabular-nums text-slate-600">· {t('bpSheetFull', { full: twoDecimals(sheetFull(sheet)), pass: twoDecimals(sheet.pass) })}</span></p>
      {editable && <div className="flex flex-wrap gap-1">{Object.keys(componentPresets).map(k => <button key={k} type="button" className={ui.ghost} onClick={() => onChange({ ...sheet, components: structuredClone(componentPresets[k]) })}>{t(`bpComp_${k}`)}</button>)}</div>}
    </div>
    <table className="mt-2 w-full text-sm">
      <thead><tr className="text-left text-xs uppercase tracking-wide text-slate-500"><th scope="col" className="py-1 pr-2 font-semibold">{t('bpComponent')}</th><th scope="col" className="py-1 pr-2 text-right font-semibold">{t('bpFull')}</th><th scope="col" className="py-1 pr-2 text-right font-semibold">{t('bpPassMark')}</th>{editable && <th scope="col" className="w-10"><span className="sr-only">{t('bpRemove')}</span></th>}</tr></thead>
      <tbody>{sheet.components.map((c, i) => <tr key={i} className="border-t border-slate-100">
        <td className="py-1.5 pr-2">{editable ? <input aria-label={t('bpComponentKey')} maxLength={16} value={c.key} onChange={e => setComp(i, { ...c, key: e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '') })} className="min-h-9 w-24 rounded-lg border border-slate-300 bg-white px-2 font-mono text-sm" /> : <span className="font-mono">{c.key}</span>}</td>
        <td className="py-1.5 pr-2 text-right">{editable ? <input aria-label={t('bpComponentFull', { key: c.key })} type="number" min="0" step="0.01" value={c.full} onChange={e => setComp(i, { ...c, full: num(e.target.value) || 0 })} className="min-h-9 w-20 rounded-lg border border-slate-300 bg-white px-2 text-right tabular-nums" /> : <span className="tabular-nums">{twoDecimals(c.full)}</span>}</td>
        <td className="py-1.5 pr-2 text-right">{editable ? <input aria-label={t('bpComponentPass', { key: c.key })} type="number" min="0" step="0.01" placeholder="—" value={c.pass ?? ''} onChange={e => setComp(i, { ...c, pass: e.target.value === '' ? null : num(e.target.value) })} className="min-h-9 w-20 rounded-lg border border-slate-300 bg-white px-2 text-right tabular-nums" /> : <span className="tabular-nums">{c.pass == null ? '—' : twoDecimals(c.pass)}</span>}</td>
        {editable && <td>{sheet.components.length > 1 && <button type="button" aria-label={t('bpRemove')} className="grid h-9 w-9 place-items-center rounded-lg text-slate-500 hover:bg-rose-50 hover:text-rose-700" onClick={() => onChange({ ...sheet, components: sheet.components.filter((_, j) => j !== i) })}>×</button>}</td>}
      </tr>)}</tbody>
    </table>
    {editable && <div className="mt-2 flex flex-wrap items-end justify-between gap-2">
      <button type="button" className={ui.ghost} onClick={() => onChange({ ...sheet, components: [...sheet.components, { key: `C${sheet.components.length + 1}`, full: 10 }] })}>+ {t('bpAddComponent')}</button>
      <div className="w-32"><NumberField label={t('bpSheetPass')} value={sheet.pass} onChange={n => onChange({ ...sheet, pass: n })} /></div>
    </div>}
  </div>
}

// ---- check: issues, sample, policies, bands -----------------------------------

function IssueText({ issue, v }: { issue: BlueprintIssue; v: BlueprintVersion }) {
  const { t, i18n } = useTranslation()
  // The server names subjects in English; show them in the reader's language.
  const params = { ...(issue.params || {}) }
  if (params.subject && i18n.language === 'bn') {
    const ref = v.subjects.find(s => s.name_en === params.subject)
    if (ref?.name_bn) params.subject = ref.name_bn
  }
  return <>{i18n.exists(`bpIssue_${issue.code}`) ? t(`bpIssue_${issue.code}`, params) : issue.message}</>
}

function Check({ v, def, issues, onPick }: { v: BlueprintVersion; def: BlueprintDefinition; issues: BlueprintIssue[]; onPick: (issue: BlueprintIssue) => void }) {
  const { t } = useTranslation()
  const graded = def.subjects.filter(s => s.role !== 'non_graded').length
  return <section className={`${ui.card} p-4`} aria-labelledby="bp-check">
    <CardTitle id="bp-check" icon="check">{t('bpCheck')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('bpCheckSummary', { assessments: def.assessments.length, subjects: graded })}</p>
    {v.status !== 'draft' ? <p className="mt-3 text-sm font-semibold text-emerald-800">{t('bpFrozen')}</p>
      : issues.length === 0 ? <p className="mt-3 rounded-xl bg-emerald-50 px-3 py-2 text-sm font-semibold text-emerald-800" role="status">{t('bpReady')}</p>
        : <ul className="mt-3 max-h-80 space-y-1.5 overflow-auto" aria-live="polite">{issues.map((issue, i) => <li key={i}>
          <button type="button" onClick={() => onPick(issue)} className="w-full rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-left text-sm text-rose-900 hover:border-rose-300">
            <IssueText issue={issue} v={v} />
          </button>
        </li>)}</ul>}
  </section>
}

function Sample({ versionId, def, subject, termId, draft }: { versionId: string; def: BlueprintDefinition; subject: BlueprintSubject; termId: string; draft: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const rule = subject.terms.find(r => r.term_id === termId)
  const sheets = subject.sheets.filter(s => rule?.contributions.some(c => c.from.includes(s.assessment)))
  const [marks, setMarks] = useState<Record<string, Record<string, string>>>({})
  const [result, setResult] = useState<TermSubjectResult | null>(null)
  const [error, setError] = useState('')
  useEffect(() => { setResult(null); setError('') }, [subject.subject_id, termId])
  const name = (key: string) => def.assessments.find(a => a.key === key)?.name || key
  const complete = sheets.every(s => s.components.every(c => (marks[s.assessment]?.[c.key] ?? '') !== ''))

  async function work() {
    setError('')
    const input: Record<string, SampleMark> = {}
    for (const s of sheets) {
      const values = marks[s.assessment] || {}
      if (s.components.length === 1 && s.components[0].key === 'TOTAL') input[s.assessment] = { marks: num(values.TOTAL ?? '') }
      else input[s.assessment] = { components: Object.fromEntries(s.components.map(c => [c.key, num(values[c.key] ?? '')])) }
    }
    try { setResult(await authorized(token => sampleBlueprintSubject(token, versionId, { subject_id: subject.subject_id, term_id: termId, marks: input, ...(draft ? { definition: def } : {}) }))) }
    catch (cause) { setResult(null); setError(explain(cause)) }
  }
  if (!rule || sheets.length === 0) return null
  return <section className={`${ui.card} p-4`} aria-labelledby="bp-sample">
    <CardTitle id="bp-sample" icon="eye">{t('bpSample')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('bpSampleIntro')}</p>
    <div className="mt-3 space-y-2">{sheets.map(s => <fieldset key={s.assessment} className="rounded-lg border border-slate-200 p-2">
      <legend className="px-1 text-xs font-semibold text-slate-600">{name(s.assessment)} · {twoDecimals(sheetFull(s))}</legend>
      <div className="flex flex-wrap gap-2">{s.components.map(c => <label key={c.key} className="text-xs text-slate-600">{c.key === 'TOTAL' ? t('bpMarks') : c.key}
        <input type="text" inputMode="decimal" aria-label={t('bpSampleFor', { name: `${name(s.assessment)} ${c.key === 'TOTAL' ? '' : c.key}`.trim(), full: twoDecimals(c.full) })} value={marks[s.assessment]?.[c.key] ?? ''}
          onChange={e => setMarks(prev => ({ ...prev, [s.assessment]: { ...prev[s.assessment], [c.key]: e.target.value } }))}
          className="mt-1 block min-h-9 w-20 rounded-lg border border-slate-300 bg-white px-2 text-right text-sm tabular-nums" />
      </label>)}</div>
    </fieldset>)}</div>
    <button type="button" disabled={!complete} className={`${ui.secondary} mt-3 w-full`} onClick={() => void work()}>{t('bpWorkItOut')}</button>
    {error && <p role="alert" className="mt-2 text-sm text-rose-700">{error}</p>}
    {result && <div className="mt-3 rounded-xl bg-slate-50 p-3 text-sm" aria-live="polite">
      <ol className="space-y-1">{result.contributions.map((c, i) => <li key={i} className="flex justify-between gap-2"><span className="text-slate-600">{c.from.map(name).join(', ')} · {t(`bpCombineName_${c.combine}`)}</span><span className="tabular-nums font-semibold">{c.exempt ? t('bpExempt') : `${c.value} / ${c.worth}`}</span></li>)}</ol>
      <p className="mt-2 flex items-baseline justify-between border-t border-slate-200 pt-2">
        <span className="font-semibold text-slate-900">{t('bpTermMark')}</span>
        <span className="text-lg font-bold tabular-nums text-slate-900">{result.status === 'graded' ? `${result.marks} / ${result.full} · ${result.grade} (${result.point})` : t(`bpSampleStatus_${result.status}`)}</span>
      </p>
      {result.reasons.map((r, i) => <p key={i} className="mt-1 text-rose-800">{t(`bpReason_${r.code}`, { assessment: r.assessment ? name(r.assessment) : '', component: r.component })}</p>)}
    </div>}
  </section>
}

function Policies({ def, terms, editable, change }: { def: BlueprintDefinition; terms: BlueprintVersion['terms']; editable: boolean; change: (d: BlueprintDefinition) => void }) {
  const { t } = useTranslation()
  const p = def.policy
  const usedTerms = terms.filter(term => def.assessments.some(a => a.term_id === term.id))
  const select = (label: string, value: string, options: string[], onChange: (v: string) => void, prefix: string) => <label className="block text-xs font-semibold text-slate-600">{label}
    <select disabled={!editable} value={value} onChange={e => onChange(e.target.value)} className="mt-1 block min-h-10 w-full rounded-lg border border-slate-300 bg-white px-2 text-sm disabled:bg-slate-50">
      {options.map(o => <option key={o} value={o}>{t(`${prefix}${o}`)}</option>)}
    </select>
  </label>
  return <section className={`${ui.card} p-4`} aria-labelledby="bp-policies">
    <CardTitle id="bp-policies" icon="shield">{t('bpPolicies')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('bpPoliciesIntro')}</p>
    <div className="mt-3 space-y-3">
      {select(t('bpPaperRule'), def.paper_rule, ['combined', 'each_paper'], v => change({ ...def, paper_rule: v as BlueprintDefinition['paper_rule'] }), 'bpPaper_')}
      {select(t('bpGpaRounding'), p.gpa_rounding, ['half_up', 'truncate'], v => change({ ...def, policy: { ...p, gpa_rounding: v as typeof p.gpa_rounding } }), 'bpGpa_')}
      {select(t('bpAbsent'), p.absent, ['zero', 'missing'], v => change({ ...def, policy: { ...p, absent: v as typeof p.absent } }), 'bpAbsent_')}
      {select(t('bpRanking'), p.ranking, ['competition', 'dense'], v => change({ ...def, policy: { ...p, ranking: v as typeof p.ranking } }), 'bpRank_')}
      <label className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1" disabled={!editable} checked={p.whole_marks} onChange={e => change({ ...def, policy: { ...p, whole_marks: e.target.checked } })} /><span>{t('bpWholeMarks')}</span></label>
      <label className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1" disabled={!editable} checked={p.round_contributions} onChange={e => change({ ...def, policy: { ...p, round_contributions: e.target.checked } })} /><span>{t('bpRoundContributions')}</span></label>
    </div>
    <h3 className="mt-5 text-sm font-bold text-slate-900">{t('bpAnnual')}</h3>
    <p className="mt-0.5 text-xs text-slate-600">{t('bpAnnualE6')}</p>
    <div className="mt-2 space-y-3">
      {select(t('bpAnnualMethod'), def.annual.method, ['final_only', 'equal', 'weighted'], v => change({ ...def, annual: { ...def.annual, method: v as BlueprintDefinition['annual']['method'], weights: v === 'weighted' ? Object.fromEntries(usedTerms.map(term => [term.id, def.annual.weights?.[term.id] ?? 0])) : undefined } }), 'bpAnnual_')}
      {def.annual.method === 'weighted' && <div className="grid grid-cols-2 gap-2">{usedTerms.map(term => <NumberField key={term.id} step="1" label={`${term.name} %`} disabled={!editable} value={def.annual.weights?.[term.id] ?? 0}
        onChange={n => change({ ...def, annual: { ...def.annual, weights: { ...def.annual.weights, [term.id]: Math.round(n) } } })} />)}</div>}
      <label className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1" disabled={!editable} checked={def.annual.final_must_pass} onChange={e => change({ ...def, annual: { ...def.annual, final_must_pass: e.target.checked } })} /><span>{t('bpFinalMustPass')}</span></label>
    </div>
  </section>
}

function Bands({ v }: { v: BlueprintVersion }) {
  const { t } = useTranslation()
  const sorted = useMemo(() => [...v.bands].sort((a, b) => b.min - a.min), [v.bands])
  return <section className={`${ui.card} p-4`} aria-labelledby="bp-bands">
    <CardTitle id="bp-bands" icon="grade">{t('bpBands')}</CardTitle>
    <p className="mt-1 text-xs text-slate-600">{t('bpBandsNote')}</p>
    <table className="mt-2 w-full text-sm"><thead><tr className="text-left text-xs uppercase tracking-wide text-slate-500"><th scope="col" className="py-1 font-semibold">{t('bpGrade')}</th><th scope="col" className="py-1 text-right font-semibold">%</th><th scope="col" className="py-1 text-right font-semibold">{t('bpPoint')}</th></tr></thead>
      <tbody>{sorted.map(b => <tr key={b.grade} className="border-t border-slate-100"><th scope="row" className="py-1 text-left font-semibold">{b.grade}</th><td className="py-1 text-right tabular-nums">{b.min}–{b.max}</td><td className="py-1 text-right tabular-nums">{b.gpa.toFixed(2)}</td></tr>)}</tbody>
    </table>
  </section>
}
