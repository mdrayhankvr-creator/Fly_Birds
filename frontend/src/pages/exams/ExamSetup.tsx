import { useMemo, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import {
  createBlueprint, deleteExamTerm, getClassBlueprints, getExamTerms, getSubjectChoices, newBlueprintVersion, saveExamTerm, saveSubjectChoices,
  type AcademicSession, type ClassBlueprint, type ExamTerm, type ExamTermInput,
} from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { EmptyState, Notice, PageHeader, SectionCard, Skeleton, Tabs, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { SessionPicker, TermChip, VersionChip, useExplain, useSessionChoice, useSubjectName } from './setupShared'

type Tab = 'terms' | 'blueprints' | 'choices'

// ExamSetup is where a school's examinations are designed before marks are
// entered: the session's terms, each class's assessment blueprint and the
// students' optional subjects. Administrators change them; head teachers
// (exams:read) see them.
export function ExamSetup({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const { sessions, items, chosen, choose } = useSessionChoice()
  const [tab, setTab] = useState<Tab>('terms')
  const previous = useMemo(() => {
    const sorted = [...items].sort((a, b) => a.start_date.localeCompare(b.start_date))
    const i = sorted.findIndex(s => s.id === chosen)
    return i > 0 ? sorted[i - 1] : undefined
  }, [items, chosen])

  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_academics')} title={t('bpSetupTitle')} description={t('bpSetupIntro')}
      actions={<Link to="/admin/exams" className={ui.secondary}>{t('bpBackToExams')}</Link>} />
    {sessions.isPending ? <Skeleton rows={3} /> : sessions.isError ? <Notice tone="error">{t('bpErrGeneric')}</Notice> : items.length === 0
      ? <EmptyState icon="calendar" title={t('bpNoSessions')} />
      : <>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <SessionPicker items={items} value={chosen} onChange={choose} />
          <Tabs label={t('bpSetupTitle')} value={tab} onChange={setTab} items={[
            { value: 'terms', label: t('bpTabTerms') }, { value: 'blueprints', label: t('bpTabBlueprints') }, { value: 'choices', label: t('bpTabChoices') },
          ]} />
        </div>
        {tab === 'terms' && <TermsPanel sessionId={chosen} canManage={canManage} />}
        {tab === 'blueprints' && <BlueprintsPanel sessionId={chosen} previous={previous} canManage={canManage} />}
        {tab === 'choices' && <ChoicesPanel sessionId={chosen} canManage={canManage} />}
      </>}
  </div>
}

const blankTerm = (sessionId: string): ExamTermInput => ({ session_id: sessionId, name: '', kind: 'term', starts_on: '', ends_on: '' })

function TermsPanel({ sessionId, canManage }: { sessionId: string; canManage: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const client = useQueryClient()
  const terms = useQuery({ queryKey: ['e5', 'terms', sessionId], enabled: !!sessionId, queryFn: () => authorized(token => getExamTerms(token, sessionId)), retry: false })
  const [form, setForm] = useState<ExamTermInput>(blankTerm(sessionId))
  const [editing, setEditing] = useState('')
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)

  async function run(task: () => Promise<unknown>, done: string) {
    setBusy(true); setNotice(null)
    try { await task(); await client.invalidateQueries({ queryKey: ['e5'] }); setNotice({ tone: 'success', text: done }) }
    catch (cause) { setNotice({ tone: 'error', text: explain(cause) }) }
    finally { setBusy(false) }
  }
  function submit(event: FormEvent) {
    event.preventDefault()
    void run(async () => { await authorized(token => saveExamTerm(token, { ...form, session_id: sessionId }, editing || undefined)); setForm(blankTerm(sessionId)); setEditing('') }, t('bpTermSaved'))
  }
  // One click for the usual year: First Term, Half-Yearly and Annual.
  function addUsual() {
    void run(async () => {
      for (const [name, kind] of [[t('bpUsualFirst'), 'term'], [t('bpUsualHalf'), 'term'], [t('bpUsualAnnual'), 'annual']] as const) {
        await authorized(token => saveExamTerm(token, { session_id: sessionId, name, kind, starts_on: '', ends_on: '' }))
      }
    }, t('bpTermsAdded'))
  }
  const edit = (term: ExamTerm) => { setEditing(term.id); setForm({ session_id: sessionId, name: term.name, kind: term.kind, starts_on: term.starts_on || '', ends_on: term.ends_on || '' }) }

  return <SectionCard id="bp-terms" title={t('bpTermsTitle')} description={t('bpTermsIntro')}>
    {notice && <div className="mb-4"><Notice tone={notice.tone}>{notice.text}</Notice></div>}
    {terms.isPending ? <Skeleton rows={3} /> : terms.isError ? <Notice tone="error">{t('bpErrGeneric')}</Notice> : terms.data.length === 0
      ? <EmptyState icon="calendar" title={t('bpNoTerms')} action={canManage ? <button type="button" disabled={busy} className={ui.primary} onClick={addUsual}>{t('bpAddUsual')}</button> : undefined}>{t('bpNoTermsHint')}</EmptyState>
      : <ol className="divide-y divide-slate-100 rounded-xl border border-slate-200">
        {terms.data.map(term => <li key={term.id} className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center sm:justify-between sm:p-4">
          <div className="flex min-w-0 items-start gap-3">
            <span aria-hidden="true" className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-slate-100 text-sm font-bold tabular-nums text-slate-700">{term.sequence}</span>
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-2"><span className="font-semibold text-slate-900">{term.name}</span><TermChip status={term.status} />{term.kind === 'annual' && <span className="text-xs font-semibold text-slate-500">{t('bpKindAnnual')}</span>}{term.legacy && <span className="text-xs text-slate-500">{t('bpLegacy')}</span>}</p>
              <p className="mt-0.5 text-sm text-slate-600">{term.starts_on ? `${term.starts_on} – ${term.ends_on || '…'} · ` : ''}{t('bpExamCount', { count: term.exams })}</p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2 sm:justify-end">
            {canManage && !term.legacy && term.status !== 'closed' && <Link to={`/admin/exams/terms/${term.id}/setup?session=${sessionId}`} className={term.exams === 0 ? ui.primary : ui.secondary}>{t(term.exams === 0 ? 'bpSetUpExams' : 'bpReviewSetup')}</Link>}
            {term.exams > 0 && <Link to={`/admin/exams/terms/${term.id}/results?session=${sessionId}`} className={ui.secondary}>{t('bpTermResults')}</Link>}
            {canManage && <button type="button" className={ui.ghost} onClick={() => edit(term)}>{t('bpEdit')}</button>}
            {canManage && term.exams === 0 && !term.legacy && <button type="button" disabled={busy} className={`${ui.ghost} text-rose-700`}
              onClick={() => void (async () => { if (await confirmAction(t('bpDeleteTermConfirm', { name: term.name }))) await run(() => authorized(token => deleteExamTerm(token, term.id)), t('bpTermDeleted')) })()}>{t('bpDelete')}</button>}
          </div>
        </li>)}
      </ol>}
    {canManage && <form onSubmit={submit} className="mt-5 grid gap-3 border-t border-slate-100 pt-5 sm:grid-cols-2 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end">
      <label className={ui.label}>{t('bpTermName')}<input required maxLength={120} className={ui.field} value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /></label>
      <label className={ui.label}>{t('bpTermKind')}<select className={ui.field} value={form.kind} onChange={e => setForm({ ...form, kind: e.target.value as ExamTerm['kind'] })}><option value="term">{t('bpKindTerm')}</option><option value="annual">{t('bpKindAnnual')}</option></select></label>
      <label className={ui.label}>{t('bpStarts')}<input type="date" className={ui.field} value={form.starts_on} onChange={e => setForm({ ...form, starts_on: e.target.value })} /></label>
      <label className={ui.label}>{t('bpEnds')}<input type="date" className={ui.field} value={form.ends_on} onChange={e => setForm({ ...form, ends_on: e.target.value })} /></label>
      <div className="flex gap-2"><button disabled={busy} className={ui.primary}>{t(editing ? 'bpSaveTerm' : 'bpAddTerm')}</button>{editing && <button type="button" className={ui.ghost} onClick={() => { setEditing(''); setForm(blankTerm(sessionId)) }}>{t('bpCancel')}</button>}</div>
    </form>}
  </SectionCard>
}

function BlueprintsPanel({ sessionId, previous, canManage }: { sessionId: string; previous?: AcademicSession; canManage: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const navigate = useNavigate()
  const list = useQuery({ queryKey: ['e5', 'blueprints', sessionId], enabled: !!sessionId, queryFn: () => authorized(token => getClassBlueprints(token, sessionId)), retry: false })
  const last = useQuery({ queryKey: ['e5', 'blueprints', previous?.id], enabled: !!previous && canManage, queryFn: () => authorized(token => getClassBlueprints(token, previous!.id)), retry: false })
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const lastActive = (classId: string) => last.data?.find(c => c.class_id === classId)?.active

  async function open(classBlueprint: ClassBlueprint, task: () => Promise<{ id: string }>) {
    setBusy(classBlueprint.class_id); setError('')
    try { const v = await task(); navigate(`/admin/exams/blueprints/${v.id}?session=${sessionId}`) }
    catch (cause) { setError(explain(cause)) }
    finally { setBusy('') }
  }

  return <SectionCard id="bp-blueprints" title={t('bpBlueprintsTitle')} description={t('bpBlueprintsIntro')}>
    {error && <div className="mb-4"><Notice tone="error">{error}</Notice></div>}
    {list.isPending ? <Skeleton rows={4} /> : list.isError ? <Notice tone="error">{t('bpErrGeneric')}</Notice> : list.data.length === 0
      ? <EmptyState icon="grade" title={t('bpNoClasses')} />
      : <ul className="divide-y divide-slate-100 rounded-xl border border-slate-200">
        {list.data.map(c => {
          const copy = lastActive(c.class_id)
          return <li key={c.class_id} className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center sm:justify-between sm:p-4">
            <div className="min-w-0">
              <p className="font-semibold text-slate-900">{c.class_name}</p>
              <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-slate-600">
                {c.active && <VersionChip version={c.active} />}{c.draft && <VersionChip version={c.draft} />}
                {!c.active && !c.draft && <span>{t('bpNeedsBlueprint')}</span>}
                {c.active && <span className="text-xs">{t('bpExamCount', { count: c.active.exams })}</span>}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              {c.draft && <Link to={`/admin/exams/blueprints/${c.draft.id}?session=${sessionId}`} className={ui.primary}>{t(canManage ? 'bpContinueDraft' : 'bpOpen')}</Link>}
              {c.active && !c.draft && <Link to={`/admin/exams/blueprints/${c.active.id}?session=${sessionId}`} className={ui.secondary}>{t('bpOpen')}</Link>}
              {canManage && c.active && !c.draft && c.blueprint_id && <button type="button" disabled={!!busy} className={ui.ghost}
                onClick={() => void open(c, () => authorized(token => newBlueprintVersion(token, c.blueprint_id!, c.active!.id)))}>{t('bpNewVersion')}</button>}
              {canManage && !c.active && !c.draft && <>
                {copy && <button type="button" disabled={!!busy} className={ui.primary} onClick={() => void open(c, () => authorized(token => createBlueprint(token, sessionId, c.class_id, copy.id)))}>{t('bpCopyLast', { session: previous?.name })}</button>}
                <button type="button" disabled={!!busy} className={copy ? ui.secondary : ui.primary} onClick={() => void open(c, () => authorized(token => createBlueprint(token, sessionId, c.class_id)))}>{t('bpStartBlueprint')}</button>
              </>}
            </div>
          </li>
        })}
      </ul>}
  </SectionCard>
}

function ChoicesPanel({ sessionId, canManage }: { sessionId: string; canManage: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useExplain()
  const client = useQueryClient()
  const subjectName = useSubjectName()
  const classes = useQuery({ queryKey: ['e5', 'blueprints', sessionId], enabled: !!sessionId, queryFn: () => authorized(token => getClassBlueprints(token, sessionId)), retry: false })
  const [classId, setClassId] = useState('')
  const chosenClass = classId || classes.data?.[0]?.class_id || ''
  const choices = useQuery({ queryKey: ['e5', 'choices', sessionId, chosenClass], enabled: !!chosenClass, queryFn: () => authorized(token => getSubjectChoices(token, sessionId, chosenClass)), retry: false })
  const [edits, setEdits] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const changed = Object.keys(edits).length

  async function save() {
    setBusy(true); setNotice(null)
    try {
      await authorized(token => saveSubjectChoices(token, sessionId, chosenClass, Object.entries(edits).map(([student_id, subject_id]) => ({ student_id, subject_id }))))
      setEdits({}); await client.invalidateQueries({ queryKey: ['e5', 'choices'] }); setNotice({ tone: 'success', text: t('bpChoicesSaved', { count: changed }) })
    } catch (cause) { setNotice({ tone: 'error', text: explain(cause) }) }
    finally { setBusy(false) }
  }

  return <SectionCard id="bp-choices" title={t('bpChoicesTitle')} description={t('bpChoicesIntro')}
    actions={canManage && changed > 0 ? <button type="button" disabled={busy} className={ui.primary} onClick={() => void save()}>{t('bpSaveChoices', { count: changed })}</button> : undefined}>
    <label className={`${ui.label} max-w-xs`}>{t('bpClass')}
      <select className={ui.field} value={chosenClass} onChange={e => { setClassId(e.target.value); setEdits({}) }}>
        {classes.data?.map(c => <option key={c.class_id} value={c.class_id}>{c.class_name}</option>)}
      </select>
    </label>
    {notice && <div className="mt-4"><Notice tone={notice.tone}>{notice.text}</Notice></div>}
    <div className="mt-4">
      {choices.isPending ? <Skeleton rows={4} /> : choices.isError ? <Notice tone="error">{t('bpErrGeneric')}</Notice> : choices.data.optional.length === 0
        ? <EmptyState icon="grade" title={t('bpNoOptional')}>{t('bpNoOptionalHint')}</EmptyState>
        : <>
          <Notice tone="info">{t(choices.data.in_use ? 'bpChoicesInUse' : 'bpChoicesNotInUse')}</Notice>
          <p className="mt-3 flex flex-wrap gap-3 text-sm text-slate-600">{choices.data.optional.map(s => <span key={s.id}><strong className="tabular-nums text-slate-900">{choices.data.counts[s.id] || 0}</strong> · {subjectName(s)}</span>)}</p>
          <div className="mt-3 max-h-[32rem] overflow-auto rounded-xl border border-slate-200">
            <table className="w-full text-left text-sm">
              <thead className="sticky top-0 bg-slate-50"><tr><th scope="col" className={ui.th}>{t('bpRoll')}</th><th scope="col" className={ui.th}>{t('bpStudent')}</th><th scope="col" className={ui.th}>{t('bpSection')}</th><th scope="col" className={ui.th}>{t('bpOptionalSubject')}</th></tr></thead>
              <tbody>{choices.data.students.map(s => {
                const value = edits[s.student_id] ?? s.subject_id ?? ''
                return <tr key={s.student_id} className={edits[s.student_id] !== undefined ? 'bg-[var(--brand-soft)]' : ''}>
                  <td className={`${ui.td} tabular-nums text-slate-600`}>{s.roll_no ?? '—'}</td>
                  <th scope="row" className={`${ui.td} font-medium text-slate-900`}>{s.name}<span className="block text-xs font-normal text-slate-500">{s.admission_no}</span></th>
                  <td className={`${ui.td} text-slate-600`}>{s.section_name}</td>
                  <td className={ui.td}>
                    <select aria-label={t('bpOptionalFor', { name: s.name })} disabled={!canManage} className="min-h-10 w-full max-w-xs rounded-lg border border-slate-300 bg-white px-2 text-sm disabled:bg-slate-50" value={value}
                      onChange={e => { const next = e.target.value; setEdits(prev => { const copy = { ...prev }; if (next === (s.subject_id ?? '')) delete copy[s.student_id]; else copy[s.student_id] = next; return copy }) }}>
                      <option value="">{t('bpNoChoice')}</option>
                      {choices.data.optional.map(o => <option key={o.id} value={o.id}>{subjectName(o)}</option>)}
                    </select>
                  </td>
                </tr>
              })}</tbody>
            </table>
          </div>
        </>}
    </div>
  </SectionCard>
}
