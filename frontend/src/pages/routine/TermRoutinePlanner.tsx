import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { saveBlob } from '../../api/admin'
import {
  addRoutineDuty, addRoutinePaper, amendRoutine, createRoutineVersion, deleteRoutineDuty, deleteRoutinePaper, deleteRoutineSitting, deleteRoutineVersion,
  downloadRoutinePdf, generateRoutine, getExamRooms, getRoutineTerms, getRoutineVersion, moveRoutine, placeRoutinePaper, saveExamRoom, saveRoutineSitting, updateRoutineVersion,
  type ExamRoom, type ExamRoomInput, type GenerateReport, type PaperKind, type RoomKind, type RoutineDocKind, type RoutineIssue, type RoutinePaper, type RoutineStep, type RoutineVersion,
} from '../../api/routinePlanner'
import { confirmAction } from '../../components/ConfirmDialog'
import { Icon } from '../../components/icons'
import { usePeopleApi } from '../peopleShared'
import { CardTitle, EmptyState, Notice, ui } from '../attendanceUi'
import { StatusPill, routineErrorOf, useDay, useIssueText, useName, useNum, usePaperName } from './routineShared'

// E8: the term exam routine planner. Administrators plan (rooms, sittings,
// papers, invigilators, the generated draft) and publish; head teachers
// approve or return. Every change runs the same rule check as approval.

const card = `${ui.card} p-4 sm:p-6`
const field = ui.field

type Feedback = { message?: string; error?: string; issues?: RoutineIssue[] }

function useRunner(onDone: () => Promise<void>) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>({})
  async function run<T>(action: () => Promise<T>, done: string | ((value: T) => string)): Promise<T | undefined> {
    setBusy(true); setFeedback({})
    try {
      const value = await action()
      await onDone()
      setFeedback({ message: typeof done === 'function' ? done(value) : t(done) })
      return value
    } catch (cause) {
      const { key, issues } = routineErrorOf(cause)
      setFeedback({ error: t(key), issues })
      return undefined
    } finally { setBusy(false) }
  }
  return { busy, feedback, setFeedback, run }
}

function IssueList({ issues, version, limit = 8 }: { issues: RoutineIssue[]; version?: RoutineVersion; limit?: number }) {
  const { t } = useTranslation()
  const text = useIssueText(version)
  const num = useNum()
  if (!issues.length) return null
  return <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">{issues.slice(0, limit).map((i, n) => <li key={n}>{text(i)}</li>)}
    {issues.length > limit && <li className="list-none text-slate-500">{t('e8CheckMore', { n: num(issues.length - limit) })}</li>}</ul>
}

function FeedbackNotice({ feedback, version }: { feedback: Feedback; version?: RoutineVersion }) {
  if (feedback.error) return <div role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800"><p className="font-semibold">{feedback.error}</p><IssueList issues={feedback.issues ?? []} version={version} /></div>
  if (feedback.message) return <Notice tone="success">{feedback.message}</Notice>
  return null
}

// ---------------------------------------------------------------------------
// Rooms

const blankRoom: ExamRoomInput = { name: '', name_bn: '', kind: 'classroom', capacity: null, is_active: true }

function RoomsPanel({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const name = useName()
  const num = useNum()
  const rooms = useQuery({ queryKey: ['routine', 'planner', 'rooms'], queryFn: () => authorized(token => getExamRooms(token, true)), retry: false })
  const runner = useRunner(() => client.invalidateQueries({ queryKey: ['routine', 'planner'] }))
  const [form, setForm] = useState<ExamRoomInput>(blankRoom)
  const [editing, setEditing] = useState('')
  async function save(event: FormEvent) {
    event.preventDefault()
    const done = await runner.run(() => authorized(token => saveExamRoom(token, { ...form, name: form.name.trim(), name_bn: form.name_bn.trim() }, editing || undefined)), 'e8Saved')
    if (done) { setForm(blankRoom); setEditing('') }
  }
  function edit(r: ExamRoom) { setEditing(r.id); setForm({ name: r.name, name_bn: r.name_bn, kind: r.kind, capacity: r.capacity, is_active: r.is_active }) }
  return <details className={card}>
    <summary className="cursor-pointer list-none"><CardTitle icon="layers">{t('e8RoomsTitle')} {rooms.data && <span className="ml-1 text-sm font-normal text-slate-500">({num(rooms.data.filter(r => r.is_active).length)})</span>}</CardTitle></summary>
    <p className="mt-2 max-w-3xl text-sm text-slate-600">{t('e8RoomsHint')}</p>
    <div className="mt-3"><FeedbackNotice feedback={runner.feedback} /></div>
    {rooms.data && (rooms.data.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('e8NoRooms')}</p> :
      <div className="mt-3 overflow-x-auto"><table className={`${ui.table} min-w-[30rem]`}><thead><tr><th className={ui.th}>{t('e8RoomName')}</th><th className={ui.th}>{t('e8RoomKind')}</th><th className={ui.th}>{t('e8Capacity')}</th><th className={ui.th}>{t('e8RoomActive')}</th>{canManage && <th className={ui.th}><span className="sr-only">{t('e8EditRoom')}</span></th>}</tr></thead>
        <tbody>{rooms.data.map(r => <tr key={r.id} className={r.is_active ? '' : 'text-slate-500'}><td className={`${ui.td} font-semibold`}>{name(r.name, r.name_bn)}</td><td className={ui.td}>{t(`e8RoomKind_${r.kind}`)}</td><td className={`${ui.td} tabular-nums`}>{r.capacity ? num(r.capacity) : t('e8NotSet')}</td><td className={ui.td}>{r.is_active ? '✓' : '—'}</td>
          {canManage && <td className={ui.td}><button type="button" className={ui.ghost} onClick={() => edit(r)}>{t('e8EditRoom')}</button></td>}</tr>)}</tbody></table></div>)}
    {canManage && <form className="mt-4 grid gap-3 border-t border-slate-100 pt-4 sm:grid-cols-2 lg:grid-cols-5" onSubmit={save}>
      <label className={ui.label}>{t('e8RoomName')}<input className={field} required maxLength={80} value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /></label>
      <label className={ui.label}>{t('e8NameBn')}<input className={field} maxLength={80} value={form.name_bn} onChange={e => setForm({ ...form, name_bn: e.target.value })} /></label>
      <label className={ui.label}>{t('e8RoomKind')}<select className={field} value={form.kind} onChange={e => setForm({ ...form, kind: e.target.value as RoomKind })}>{(['classroom', 'lab', 'hall'] as const).map(k => <option key={k} value={k}>{t(`e8RoomKind_${k}`)}</option>)}</select></label>
      <label className={ui.label}>{t('e8Capacity')}<input className={field} type="number" min={1} max={5000} inputMode="numeric" value={form.capacity ?? ''} onChange={e => setForm({ ...form, capacity: e.target.value ? Number(e.target.value) : null })} /></label>
      <label className="flex items-center gap-2 self-end pb-3 text-sm font-medium text-slate-700"><input type="checkbox" checked={form.is_active} onChange={e => setForm({ ...form, is_active: e.target.checked })} />{t('e8RoomActive')}</label>
      <div className="flex gap-2 sm:col-span-2 lg:col-span-5"><button className={ui.primary} disabled={runner.busy}>{t(editing ? 'e8SaveRoom' : 'e8AddRoom')}</button>{editing && <button type="button" className={ui.secondary} onClick={() => { setEditing(''); setForm(blankRoom) }}>{t('cancel')}</button>}</div>
    </form>}
  </details>
}

// ---------------------------------------------------------------------------
// Planner

export function TermRoutinePlanner({ canManage, canApprove }: { canManage: boolean; canApprove: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const num = useNum()
  const [termId, setTermId] = useState('')
  const [versionId, setVersionId] = useState('')
  const [report, setReport] = useState<GenerateReport | null>(null)
  const [reason, setReason] = useState<{ step: RoutineStep | 'amend'; text: string } | null>(null)
  const [start, setStart] = useState({ from: '', to: '' })
  const terms = useQuery({ queryKey: ['routine', 'planner', 'terms'], queryFn: () => authorized(getRoutineTerms), retry: false })
  const term = terms.data?.find(x => x.term_id === termId)
  useEffect(() => {
    if (!termId && terms.data?.length) setTermId((terms.data.find(x => x.versions.length) ?? terms.data[0]).term_id)
  }, [terms.data, termId])
  useEffect(() => {
    if (!term) return
    const open = term.versions.find(v => ['draft', 'submitted', 'approved'].includes(v.status))
    const published = term.versions.find(v => v.status === 'published')
    if (!versionId || !term.versions.some(v => v.id === versionId)) setVersionId((open ?? published ?? term.versions[0])?.id ?? '')
    setStart({ from: term.starts_on ?? '', to: term.ends_on ?? '' })
  }, [term, versionId])
  const version = useQuery({ queryKey: ['routine', 'planner', 'version', versionId], enabled: !!versionId, queryFn: () => authorized(token => getRoutineVersion(token, versionId)), retry: false })
  const v = version.data
  const runner = useRunner(() => client.invalidateQueries({ queryKey: ['routine', 'planner'] }))
  const editable = canManage && v?.status === 'draft'
  const hard = v?.issues.filter(i => i.hard) ?? []
  const soft = v?.issues.filter(i => !i.hard) ?? []
  const hasOpen = !!term?.versions.some(x => ['draft', 'submitted', 'approved'].includes(x.status))

  async function startRoutine(event: FormEvent) {
    event.preventDefault()
    const made = await runner.run(() => authorized(token => createRoutineVersion(token, { term_id: termId, starts_on: start.from || undefined, ends_on: start.to || undefined })), 'e8Started')
    if (made) setVersionId(made.id)
  }
  async function generate() {
    if (!v || !(await confirmAction(t('e8GenerateConfirm'), { tone: 'default', confirmLabel: t('e8Generate') }))) return
    const out = await runner.run(() => authorized(token => generateRoutine(token, v.id)), 'e8Generated')
    if (out) setReport(out.report)
  }
  async function step(s: RoutineStep) {
    if (!v) return
    if (s === 'approve' && !(await confirmAction(t('e8ApproveConfirm', { n: num(v.version) }), { tone: 'default', confirmLabel: t('e8Approve') }))) return
    if (s === 'publish' && !(await confirmAction(v.previous_version ? t('e8PublishAmendConfirm', { n: num(v.version), old: num(v.previous_version) }) : t('e8PublishConfirm', { n: num(v.version) }), { tone: 'default', confirmLabel: t('e8Publish') }))) return
    const done = { submit: 'e8Submitted', approve: 'e8Approved', publish: 'e8Published', return: 'e8Returned', withdraw: 'e8Returned' }[s]
    await runner.run(() => authorized(token => moveRoutine(token, v.id, s)), done)
    setReport(null)
  }
  async function withReason(event: FormEvent) {
    event.preventDefault()
    if (!v || !reason) return
    if (reason.step === 'amend') {
      const made = await runner.run(() => authorized(token => amendRoutine(token, v.id, reason.text.trim())), out => t('e8Amended', { n: num(out.version) }))
      if (made) { setVersionId(made.id); setReason(null) }
      return
    }
    const done = await runner.run(() => authorized(token => moveRoutine(token, v.id, reason.step as RoutineStep, reason.text.trim())), 'e8Returned')
    if (done) setReason(null)
  }
  async function remove() {
    if (!v || !(await confirmAction(t('e8DeleteConfirm')))) return
    const done = await runner.run(() => authorized(token => deleteRoutineVersion(token, v.id)), 'e8Deleted')
    if (done) setVersionId('')
  }

  if (terms.isPending) return <section className={card}><p role="status">{t('routineLoading')}</p></section>
  if (terms.isError) return <section className={card}><p role="alert" className="text-rose-700">{t('routineLoadError')}</p></section>
  return <div className="space-y-5" data-testid="term-routine-planner">
    <section className={card} aria-labelledby="e8-planner">
      <CardTitle id="e8-planner" icon="calendar">{t('e8Tab')}</CardTitle>
      <p className="mt-1 max-w-3xl text-sm text-slate-600">{t('e8Intro')}</p>
      {terms.data.length === 0 ? <div className="mt-4"><EmptyState title={t('e8NoTerms')} icon="calendar">{t('e8NoTermsHint')}</EmptyState></div> : <>
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <label className={ui.label}>{t('e8Term')}<select className={field} value={termId} onChange={e => { setTermId(e.target.value); setVersionId(''); setReport(null); runner.setFeedback({}) }}>
            {terms.data.map(x => <option key={x.term_id} value={x.term_id}>{x.session_name} · {x.term_name}</option>)}</select></label>
          {term && term.versions.length > 0 && <div><p className={ui.label}>{t('e8Versions')}</p><div className="mt-1.5 flex flex-wrap gap-2">{term.versions.map(x =>
            <button key={x.id} type="button" aria-pressed={x.id === versionId} onClick={() => { setVersionId(x.id); setReport(null); runner.setFeedback({}) }}
              className={`inline-flex items-center gap-2 rounded-xl border px-3 py-2 text-sm font-semibold ${x.id === versionId ? 'border-[var(--brand-primary)] ds-softer' : 'border-slate-200 bg-white'}`}>{t('e8Version', { n: num(x.version) })}<StatusPill status={x.status} /></button>)}</div></div>}
        </div>
        {term && term.versions.length === 0 && (canManage ? <form className="mt-4 grid gap-3 rounded-xl border border-dashed border-slate-300 p-4 sm:grid-cols-3" onSubmit={startRoutine}>
          <p className="text-sm text-slate-700 sm:col-span-3">{t('e8NoRoutine')} {t('e8StartHint')}</p>
          <label className={ui.label}>{t('e8From')}<input type="date" className={field} required value={start.from} onChange={e => setStart({ ...start, from: e.target.value })} /></label>
          <label className={ui.label}>{t('e8To')}<input type="date" className={field} required min={start.from} value={start.to} onChange={e => setStart({ ...start, to: e.target.value })} /></label>
          <div className="self-end"><button className={ui.primary} disabled={runner.busy}>{t('e8StartRoutine')}</button></div>
        </form> : <p className="mt-4 text-sm text-slate-600">{t('e8NoRoutineHead')}</p>)}
      </>}
      <div className="mt-3"><FeedbackNotice feedback={runner.feedback} version={v} /></div>
    </section>
    <RoomsPanel canManage={canManage} />
    {versionId && (version.isPending ? <section className={card}><p role="status">{t('routineLoading')}</p></section> : version.isError ? <section className={card}><p role="alert" className="text-rose-700">{t('routineLoadError')}</p></section> : v && <>
      <section className={card} aria-labelledby="e8-version">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 id="e8-version" className="flex flex-wrap items-center gap-2 text-lg font-bold text-slate-900">{v.term_name} · {t('e8Version', { n: num(v.version) })} <StatusPill status={v.status} /></h2>
            <p className="mt-1 text-sm text-slate-600">{v.session_name} · {t('e8Placed', { placed: num(v.placed), total: num(v.papers.length) })}{v.approved_by_name ? ` · ${t('e8ApprovedBy', { name: v.approved_by_name })}` : ''}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {editable && <><button type="button" className={ui.secondary} disabled={runner.busy} onClick={() => void generate()}><Icon name="clipboard" className="h-4 w-4" />{t('e8Generate')}</button>
              <button type="button" className={ui.primary} disabled={runner.busy} onClick={() => void step('submit')}>{t('e8Submit')}</button></>}
            {v.status === 'submitted' && canApprove && <button type="button" className={ui.primary} disabled={runner.busy} onClick={() => void step('approve')}>{t('e8Approve')}</button>}
            {v.status === 'approved' && canManage && <button type="button" className={ui.primary} disabled={runner.busy} onClick={() => void step('publish')}>{t('e8Publish')}</button>}
            {(v.status === 'submitted' || v.status === 'approved') && canApprove && <button type="button" className={ui.secondary} disabled={runner.busy} onClick={() => setReason({ step: 'return', text: '' })}>{t('e8Return')}</button>}
            {(v.status === 'submitted' || v.status === 'approved') && canManage && <button type="button" className={ui.secondary} disabled={runner.busy} onClick={() => setReason({ step: 'withdraw', text: '' })}>{t('e8Withdraw')}</button>}
            {v.status === 'published' && canManage && !hasOpen && <button type="button" className={ui.secondary} disabled={runner.busy} onClick={() => setReason({ step: 'amend', text: '' })}>{t('e8Amend')}</button>}
            {editable && <button type="button" className={ui.danger} disabled={runner.busy} onClick={() => void remove()}>{t('e8DeleteDraft')}</button>}
          </div>
        </div>
        {editable && <p className="mt-2 max-w-3xl text-xs text-slate-500">{t('e8GenerateHint')}</p>}
        {v.returned_reason && v.status === 'draft' && <div className="mt-3"><Notice tone="warning">{t('e8ReturnedNotice', { reason: v.returned_reason })}</Notice></div>}
        {v.reason && v.previous_version && <div className="mt-3"><Notice tone="info">{t('e8AmendmentOf', { n: num(v.previous_version), reason: v.reason })}</Notice></div>}
        {reason && <form className="mt-4 grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4" onSubmit={withReason}>
          <label className={ui.label}>{t(reason.step === 'amend' ? 'e8AmendReason' : 'e8ReturnReason')}<textarea className={field} required maxLength={1000} rows={2} value={reason.text} onChange={e => setReason({ ...reason, text: e.target.value })} /></label>
          {reason.step === 'amend' && <p className="text-sm text-slate-600">{t('e8AmendHint')}</p>}
          <div className="flex gap-2"><button className={ui.primary} disabled={runner.busy || !reason.text.trim()}>{t(reason.step === 'amend' ? 'e8Amend' : reason.step === 'withdraw' ? 'e8Withdraw' : 'e8Return')}</button><button type="button" className={ui.secondary} onClick={() => setReason(null)}>{t('cancel')}</button></div>
        </form>}
      </section>
      {report && <ReportPanel report={report} version={v} />}
      <section className={card} aria-labelledby="e8-check">
        <CardTitle id="e8-check" icon="check">{t('e8Check')}</CardTitle>
        {hard.length === 0 ? <div className="mt-3"><Notice tone="success">{t('e8CheckReady')}</Notice></div> :
          <div className="mt-3 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-rose-800" data-testid="hard-issues"><p className="font-semibold">{t('e8CheckHard', { n: num(hard.length) })}</p><IssueList issues={hard} version={v} /></div>}
        {soft.length > 0 && <div className="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-amber-900"><p className="font-semibold">{t('e8CheckSoft', { n: num(soft.length) })} <span className="font-normal">· {t('e8CheckSoftHint')}</span></p><IssueList issues={soft} version={v} limit={5} /></div>}
      </section>
      {v.previous_version && <ChangesPanel version={v} />}
      {editable && <SettingsPanel version={v} />}
      <CalendarPanel version={v} />
      <PapersPanel version={v} editable={!!editable} />
      <DutiesPanel version={v} editable={!!editable} />
      <DocumentsPanel version={v} />
    </>)}
  </div>
}

function ReportPanel({ report, version }: { report: GenerateReport; version: RoutineVersion }) {
  const { t } = useTranslation()
  const num = useNum()
  const paperName = usePaperName()
  return <section className={card} aria-labelledby="e8-report" data-testid="generate-report">
    <CardTitle id="e8-report" icon="clipboard">{t('e8ReportTitle')}</CardTitle>
    <ul className="mt-3 flex flex-wrap gap-2 text-sm"><li className="rounded-full bg-emerald-50 px-3 py-1 font-semibold text-emerald-800">{t('e8ReportPlaced', { n: num(report.placed) })}</li><li className="rounded-full bg-slate-100 px-3 py-1 font-semibold text-slate-700">{t('e8ReportKept', { n: num(report.kept) })}</li><li className="rounded-full bg-sky-50 px-3 py-1 font-semibold text-sky-800">{t('e8ReportDuties', { n: num(report.duties) })}</li></ul>
    {report.unplaced.length > 0 && <div className="mt-3"><p className="font-semibold text-rose-800">{t('e8ReportUnplaced')}</p><ul className="mt-1 list-disc pl-5 text-sm text-slate-700">{report.unplaced.map(u => {
      const p = version.papers.find(x => x.id === u.paper_id)
      return <li key={u.paper_id}>{p ? `${p.class_name} ${paperName(p)}` : u.message}: {t(`e8Reason_${u.reason}`)}</li>
    })}</ul></div>}
    <p className="mt-3 text-xs text-slate-500">{t('e8ReportLimit')}</p>
  </section>
}

function ChangesPanel({ version }: { version: RoutineVersion }) {
  const { t } = useTranslation()
  const day = useDay()
  const num = useNum()
  const paperName = usePaperName()
  const when = (date?: string, sitting?: string) => [date ? day(date) : '', sitting ?? ''].filter(Boolean).join(' · ') || '—'
  return <section className={card} aria-labelledby="e8-changes" data-testid="routine-changes">
    <CardTitle id="e8-changes" icon="file">{t('e8Changes', { n: num(version.previous_version ?? 0) })}</CardTitle>
    {version.changes.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('e8NoChanges')}</p> :
      <div className="mt-3 overflow-x-auto"><table className={`${ui.table} min-w-[34rem]`}><thead><tr><th className={ui.th}>{t('e8Paper')}</th><th className={ui.th}>{t('e8Kind')}</th><th className={ui.th}>{t('e8From')}</th><th className={ui.th}>{t('e8To')}</th></tr></thead>
        <tbody>{version.changes.map((c, i) => <tr key={i} className="bg-amber-50/60"><td className={`${ui.td} font-semibold`}>{c.class_name} {paperName(c)}</td><td className={ui.td}>{t(`e8Change_${c.change}`)}</td>
          <td className={ui.td}>{c.change === 'rooms' ? c.from_rooms : c.change === 'duration' ? t('e8Minutes', { n: num(c.from_duration ?? 0) }) : when(c.from_date, c.from_sitting)}</td>
          <td className={`${ui.td} font-semibold`}>{c.change === 'rooms' ? c.to_rooms : c.change === 'duration' ? t('e8Minutes', { n: num(c.to_duration ?? 0) }) : c.change === 'removed' ? '—' : when(c.to_date, c.to_sitting)}{c.change === 'moved' && c.to_rooms ? <span className="block text-xs font-normal text-slate-600">{c.to_rooms}</span> : null}</td></tr>)}</tbody></table></div>}
  </section>
}

function SettingsPanel({ version }: { version: RoutineVersion }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const num = useNum()
  const name = useName()
  const runner = useRunner(() => client.invalidateQueries({ queryKey: ['routine', 'planner'] }))
  const [dates, setDates] = useState({ starts_on: version.starts_on, ends_on: version.ends_on, instructions: version.instructions })
  const [sitting, setSitting] = useState({ id: '', name: '', name_bn: '', start_time: '10:00', end_time: '13:00' })
  useEffect(() => { setDates({ starts_on: version.starts_on, ends_on: version.ends_on, instructions: version.instructions }) }, [version.starts_on, version.ends_on, version.instructions])
  async function saveSitting(event: FormEvent) {
    event.preventDefault()
    const done = await runner.run(() => authorized(token => saveRoutineSitting(token, version.id, { name: sitting.name, name_bn: sitting.name_bn, start_time: sitting.start_time, end_time: sitting.end_time }, sitting.id || undefined)), 'e8Saved')
    if (done) setSitting({ id: '', name: '', name_bn: '', start_time: '10:00', end_time: '13:00' })
  }
  return <section className={card} aria-labelledby="e8-settings">
    <CardTitle id="e8-settings" icon="settings">{t('e8Dates')} · {t('e8Sittings')}</CardTitle>
    <div className="mt-3"><FeedbackNotice feedback={runner.feedback} version={version} /></div>
    <form className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4" onSubmit={e => { e.preventDefault(); void runner.run(() => authorized(token => updateRoutineVersion(token, version.id, dates)), 'e8Saved') }}>
      <label className={ui.label}>{t('e8From')}<input type="date" className={field} required min={version.term_starts_on ?? undefined} max={version.term_ends_on ?? undefined} value={dates.starts_on} onChange={e => setDates({ ...dates, starts_on: e.target.value })} /></label>
      <label className={ui.label}>{t('e8To')}<input type="date" className={field} required min={dates.starts_on} max={version.term_ends_on ?? undefined} value={dates.ends_on} onChange={e => setDates({ ...dates, ends_on: e.target.value })} /></label>
      <label className={`${ui.label} sm:col-span-2`}>{t('e8Instructions')}<textarea className={field} rows={2} maxLength={4000} value={dates.instructions} onChange={e => setDates({ ...dates, instructions: e.target.value })} /></label>
      <div className="sm:col-span-2 lg:col-span-4"><button className={ui.secondary} disabled={runner.busy}>{t('e8SaveSettings')}</button></div>
    </form>
    <ul className="mt-5 grid gap-2 border-t border-slate-100 pt-4 sm:grid-cols-2">{version.sittings.map(s => <li key={s.id} className="flex items-center justify-between gap-2 rounded-xl border border-slate-200 px-3 py-2">
      <span><span className="font-semibold">{name(s.name, s.name_bn)}</span> <span className="tabular-nums text-sm text-slate-600">{num(s.start_time)}–{num(s.end_time)} · {t('e8Minutes', { n: num(s.minutes) })}</span></span>
      <span className="flex gap-1"><button type="button" className={ui.ghost} onClick={() => setSitting({ id: s.id, name: s.name, name_bn: s.name_bn, start_time: s.start_time, end_time: s.end_time })}>{t('edit')}</button>
        <button type="button" className={`${ui.ghost} text-rose-700`} aria-label={`${t('e8RemoveSitting')} ${s.name}`} disabled={runner.busy} onClick={() => void runner.run(() => authorized(token => deleteRoutineSitting(token, version.id, s.id)), 'e8Saved')}>{t('delete')}</button></span></li>)}</ul>
    <form className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-5" onSubmit={saveSitting}>
      <label className={ui.label}>{t('e8SittingName')}<input className={field} required maxLength={60} value={sitting.name} onChange={e => setSitting({ ...sitting, name: e.target.value })} /></label>
      <label className={ui.label}>{t('e8NameBn')}<input className={field} maxLength={60} value={sitting.name_bn} onChange={e => setSitting({ ...sitting, name_bn: e.target.value })} /></label>
      <label className={ui.label}>{t('e8SittingStart')}<input type="time" className={field} required value={sitting.start_time} onChange={e => setSitting({ ...sitting, start_time: e.target.value })} /></label>
      <label className={ui.label}>{t('e8SittingEnd')}<input type="time" className={field} required value={sitting.end_time} onChange={e => setSitting({ ...sitting, end_time: e.target.value })} /></label>
      <div className="flex gap-2 self-end"><button className={ui.secondary} disabled={runner.busy}>{t(sitting.id ? 'saveChanges' : 'e8AddSitting')}</button>{sitting.id && <button type="button" className={ui.quiet} onClick={() => setSitting({ id: '', name: '', name_bn: '', start_time: '10:00', end_time: '13:00' })}>{t('cancel')}</button>}</div>
    </form>
  </section>
}

function CalendarPanel({ version }: { version: RoutineVersion }) {
  const { t } = useTranslation()
  const day = useDay()
  const name = useName()
  const paperName = usePaperName()
  const offDays = version.weekly_off_days.map(d => t(`afWeekday_${d}`)).join(', ') || '—'
  const cells = useMemo(() => {
    const map = new Map<string, RoutinePaper[]>()
    for (const p of version.papers) if (p.exam_date && p.sitting_id) map.set(`${p.exam_date}|${p.sitting_id}`, [...(map.get(`${p.exam_date}|${p.sitting_id}`) ?? []), p])
    return map
  }, [version.papers])
  return <section className={card} aria-labelledby="e8-calendar">
    <CardTitle id="e8-calendar" icon="calendar">{t('e8Calendar')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('e8CalendarHint', { days: offDays })}</p>
    {version.holidays.length > 0 && <p className="mt-1 text-sm text-slate-600">{t('e8Holidays', { list: version.holidays.map(h => `${day(h.date)} (${name(h.title_en, h.title_bn)})`).join('; ') })}</p>}
    <div className="mt-3 overflow-x-auto"><table className={`${ui.table} min-w-[36rem]`} data-testid="routine-calendar">
      <thead><tr><th className={ui.th}>{t('e8Date')}</th>{version.sittings.map(s => <th key={s.id} className={ui.th}>{name(s.name, s.name_bn)} <span className="font-normal normal-case">{s.start_time}–{s.end_time}</span></th>)}</tr></thead>
      <tbody>{version.dates.map(d => <tr key={d}><td className={`${ui.td} whitespace-nowrap font-semibold`}>{day(d)}</td>
        {version.sittings.map(s => { const list = cells.get(`${d}|${s.id}`) ?? []; return <td key={s.id} className={`${ui.td} align-top`}>{list.length === 0 ? <span className="text-slate-400">{t('e8Empty')}</span> :
          <ul className="flex flex-wrap gap-1">{list.map(p => <li key={p.id} className="rounded-lg bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-800">{p.class_name} · {paperName(p)}</li>)}</ul>}</td> })}</tr>)}</tbody>
    </table></div>
  </section>
}

type Draft = { duration: number; needs_lab: boolean; date: string; sitting: string; pinned: boolean; rooms: Record<string, string> }

function PapersPanel({ version, editable }: { version: RoutineVersion; editable: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const day = useDay()
  const num = useNum()
  const name = useName()
  const paperName = usePaperName()
  const runner = useRunner(() => client.invalidateQueries({ queryKey: ['routine', 'planner'] }))
  const [open, setOpen] = useState('')
  const [draft, setDraft] = useState<Draft | null>(null)
  const [adding, setAdding] = useState({ class_id: '', subject_id: '', kind: 'practical' as PaperKind, paper_no: 1, duration_minutes: 120, needs_lab: true })
  const sitting = (id: string | null) => version.sittings.find(s => s.id === id)
  const activeRooms = version.rooms.filter(r => r.is_active)
  const numbered = (p: RoutinePaper) => version.papers.some(x => x.id !== p.id && x.class_id === p.class_id && x.subject_id === p.subject_id && x.kind === p.kind)
  function edit(p: RoutinePaper) {
    setOpen(p.id); runner.setFeedback({})
    setDraft({ duration: p.duration_minutes, needs_lab: p.needs_lab, date: p.exam_date ?? '', sitting: p.sitting_id ?? version.sittings[0]?.id ?? '', pinned: p.exam_date ? p.pinned : true, rooms: Object.fromEntries(p.rooms.map(r => [r.section_id, r.room_id])) })
  }
  async function save(p: RoutinePaper, event: FormEvent) {
    event.preventDefault()
    if (!draft) return
    const placed = !!draft.date
    const done = await runner.run(() => authorized(token => placeRoutinePaper(token, version.id, p.id, {
      duration_minutes: draft.duration, needs_lab: draft.needs_lab, exam_date: draft.date, sitting_id: placed ? draft.sitting : '', pinned: placed && draft.pinned,
      rooms: placed ? Object.entries(draft.rooms).filter(([, room]) => room).map(([section_id, room_id]) => ({ section_id, room_id })) : [],
    })), 'e8Saved')
    if (done) { setOpen(''); setDraft(null) }
  }
  const subjectsOf = (classId: string) => [...new Map(version.papers.filter(p => p.class_id === classId).map(p => [p.subject_id, p])).values()]
  return <section className={card} aria-labelledby="e8-papers">
    <CardTitle id="e8-papers" icon="grade">{t('e8Papers')}</CardTitle>
    <div className="mt-3"><FeedbackNotice feedback={runner.feedback} version={version} /></div>
    {version.classes.map(c => <div key={c.id} className="mt-4">
      <h3 className="text-sm font-bold text-slate-900">{c.name} <span className="font-normal text-slate-500">· {c.sections.map(s => `${s.name} (${num(s.students)})`).join(', ')}</span></h3>
      <div className="mt-2 overflow-x-auto"><table className={`${ui.table} min-w-[40rem]`}><thead><tr><th className={ui.th}>{t('e8Paper')}</th><th className={ui.th}>{t('e8Date')}</th><th className={ui.th}>{t('e8Rooms')}</th>{editable && <th className={ui.th}><span className="sr-only">{t('e8Place')}</span></th>}</tr></thead>
        <tbody>{version.papers.filter(p => p.class_id === c.id).map(p => <PaperRows key={p.id}>
          <tr data-testid={`paper-${p.subject_code}-${p.kind}-${p.paper_no}`}>
            <td className={ui.td}><span className="font-semibold text-slate-900">{paperName(p, numbered(p))}</span>{p.optional && <span className="ml-1 text-xs text-slate-500">({t('e8Optional')})</span>}
              <span className="block text-xs text-slate-500">{p.subject_code} · {t('e8Minutes', { n: num(p.duration_minutes) })}{p.needs_lab ? ` · ${t('e8NeedsLab')}` : ''} · {t('e8Students', { n: num(p.students) })}</span></td>
            <td className={ui.td}>{p.exam_date ? <><span className="font-semibold">{day(p.exam_date)}</span><span className="block text-xs text-slate-500">{sitting(p.sitting_id) ? name(sitting(p.sitting_id)!.name, sitting(p.sitting_id)!.name_bn) : ''}{p.pinned ? ` · ${t('e8Pinned')}` : ''}</span></> : <span className="text-slate-500">{t('e8NotPlaced')}</span>}</td>
            <td className={`${ui.td} text-sm`}>{p.rooms.length ? p.rooms.map(r => `${r.section_name}: ${name(r.room_name, r.room_name_bn)}`).join(', ') : '—'}</td>
            {editable && <td className={`${ui.td} whitespace-nowrap text-right`}><button type="button" className={ui.ghost} aria-expanded={open === p.id} onClick={() => open === p.id ? setOpen('') : edit(p)}>{t('e8Place')}</button></td>}
          </tr>
          {editable && open === p.id && draft && <tr><td colSpan={4} className="border-b border-slate-200 bg-slate-50 px-3 py-4">
            <form className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4" onSubmit={e => void save(p, e)} aria-label={t('e8PlaceTitle', { paper: `${c.name} ${paperName(p)}` })}>
              <label className={ui.label}>{t('e8Date')}<select className={field} value={draft.date} onChange={e => setDraft({ ...draft, date: e.target.value })}><option value="">{t('e8NotPlaced')}</option>
                {[...new Set([...(p.exam_date ? [p.exam_date] : []), ...version.dates])].sort().map(d => <option key={d} value={d}>{day(d)}</option>)}</select></label>
              <label className={ui.label}>{t('e8Sitting')}<select className={field} disabled={!draft.date} value={draft.sitting} onChange={e => setDraft({ ...draft, sitting: e.target.value })}>{version.sittings.map(s => <option key={s.id} value={s.id}>{name(s.name, s.name_bn)} {s.start_time}–{s.end_time}</option>)}</select></label>
              <label className={ui.label}>{t('e8Duration')}<input type="number" min={15} max={360} className={field} value={draft.duration} onChange={e => setDraft({ ...draft, duration: Number(e.target.value) })} /></label>
              <div className="flex flex-col justify-end gap-2 pb-1 text-sm font-medium text-slate-700">
                <label className="flex items-center gap-2"><input type="checkbox" checked={draft.needs_lab} onChange={e => setDraft({ ...draft, needs_lab: e.target.checked })} />{t('e8NeedsLab')}</label>
                <label className="flex items-center gap-2" title={t('e8PinnedHint')}><input type="checkbox" disabled={!draft.date} checked={draft.pinned} onChange={e => setDraft({ ...draft, pinned: e.target.checked })} />{t('e8Pinned')}</label>
              </div>
              {draft.date && c.sections.map(s => <label key={s.id} className={ui.label}>{t('e8SectionRoom', { section: s.name, n: num(s.students) })}
                <select className={field} value={draft.rooms[s.id] ?? ''} onChange={e => setDraft({ ...draft, rooms: { ...draft.rooms, [s.id]: e.target.value } })}><option value="">{t('e8ChooseRoom')}</option>
                  {activeRooms.filter(r => !draft.needs_lab || r.kind === 'lab').map(r => <option key={r.id} value={r.id}>{name(r.name, r.name_bn)} · {t(`e8RoomKind_${r.kind}`)}{r.capacity ? ` · ${num(r.capacity)}` : ''}</option>)}</select></label>)}
              <div className="flex flex-wrap gap-2 sm:col-span-2 lg:col-span-4">
                <button className={ui.primary} disabled={runner.busy}>{t('e8SavePaper')}</button>
                <button type="button" className={ui.secondary} onClick={() => { setOpen(''); setDraft(null) }}>{t('cancel')}</button>
                <button type="button" className={`${ui.danger} ml-auto`} disabled={runner.busy} onClick={async () => {
                  if (!(await confirmAction(t('e8RemovePaperConfirm', { paper: `${c.name} ${paperName(p)}` })))) return
                  await runner.run(() => authorized(token => deleteRoutinePaper(token, version.id, p.id)), 'e8Saved')
                }}>{t('e8RemovePaper')}</button>
              </div>
            </form></td></tr>}
        </PaperRows>)}</tbody></table></div>
    </div>)}
    {editable && <form className="mt-5 grid gap-3 border-t border-slate-100 pt-4 sm:grid-cols-3 lg:grid-cols-6" onSubmit={e => { e.preventDefault(); void runner.run(() => authorized(token => addRoutinePaper(token, version.id, adding)), 'e8Saved') }}>
      <p className="text-sm font-semibold text-slate-800 sm:col-span-3 lg:col-span-6">{t('e8AddPaper')} <span className="font-normal text-slate-500">· {t('e8AddPaperHint')}</span></p>
      <label className={ui.label}>{t('e8Class')}<select className={field} required value={adding.class_id} onChange={e => setAdding({ ...adding, class_id: e.target.value, subject_id: '' })}><option value="">{t('selectOption')}</option>{version.classes.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
      <label className={ui.label}>{t('e8Subject')}<select className={field} required value={adding.subject_id} onChange={e => setAdding({ ...adding, subject_id: e.target.value })}><option value="">{t('selectOption')}</option>{subjectsOf(adding.class_id).map(p => <option key={p.subject_id} value={p.subject_id}>{name(p.subject_name, p.subject_name_bn)}</option>)}</select></label>
      <label className={ui.label}>{t('e8Kind')}<select className={field} value={adding.kind} onChange={e => setAdding({ ...adding, kind: e.target.value as PaperKind, needs_lab: e.target.value === 'practical' })}>{(['theory', 'practical', 'viva', 'oral'] as const).map(k => <option key={k} value={k}>{t(`e8Kind_${k}`)}</option>)}</select></label>
      <label className={ui.label}>{t('e8PaperNo')}<select className={field} value={adding.paper_no} onChange={e => setAdding({ ...adding, paper_no: Number(e.target.value) })}>{[1, 2, 3].map(n => <option key={n} value={n}>{t(`e8Nth_${n}`)}</option>)}</select></label>
      <label className={ui.label}>{t('e8Duration')}<input type="number" min={15} max={360} className={field} value={adding.duration_minutes} onChange={e => setAdding({ ...adding, duration_minutes: Number(e.target.value) })} /></label>
      <div className="flex flex-col justify-end gap-2"><label className="flex items-center gap-2 text-sm font-medium text-slate-700"><input type="checkbox" checked={adding.needs_lab} onChange={e => setAdding({ ...adding, needs_lab: e.target.checked })} />{t('e8NeedsLab')}</label><button className={ui.secondary} disabled={runner.busy}>{t('e8AddPaper')}</button></div>
    </form>}
  </section>
}

function PaperRows({ children }: { children: ReactNode }) { return <>{children}</> }

function DutiesPanel({ version, editable }: { version: RoutineVersion; editable: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const day = useDay()
  const name = useName()
  const runner = useRunner(() => client.invalidateQueries({ queryKey: ['routine', 'planner'] }))
  const [form, setForm] = useState({ exam_date: '', sitting_id: '', room_id: '', employee_id: '' })
  const sittingName = (id: string) => { const s = version.sittings.find(x => x.id === id); return s ? `${name(s.name, s.name_bn)} ${s.start_time}` : '' }
  const usedDates = [...new Set(version.papers.flatMap(p => p.exam_date ? [p.exam_date] : []))].sort()
  return <section className={card} aria-labelledby="e8-duties">
    <CardTitle id="e8-duties" icon="teacher">{t('e8Duties')}</CardTitle>
    <p className="mt-1 max-w-3xl text-sm text-slate-600">{t('e8DutiesHint')}</p>
    <div className="mt-3"><FeedbackNotice feedback={runner.feedback} version={version} /></div>
    {version.duties.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('e8NoDuties')}</p> :
      <div className="mt-3 max-h-96 overflow-auto"><table className={`${ui.table} min-w-[34rem]`}><thead><tr><th className={ui.th}>{t('e8Date')}</th><th className={ui.th}>{t('e8Sitting')}</th><th className={ui.th}>{t('e8Room')}</th><th className={ui.th}>{t('e8Teacher')}</th>{editable && <th className={ui.th}><span className="sr-only">{t('e8RemoveDuty')}</span></th>}</tr></thead>
        <tbody>{version.duties.map(d => <tr key={d.id}><td className={ui.td}>{day(d.exam_date)}</td><td className={ui.td}>{sittingName(d.sitting_id)}</td><td className={ui.td}>{name(d.room_name, d.room_name_bn)}</td><td className={`${ui.td} font-semibold`}>{name(d.employee_name, d.employee_name_bn)}</td>
          {editable && <td className={ui.td}><button type="button" className={`${ui.ghost} text-rose-700`} aria-label={`${t('e8RemoveDuty')}: ${d.employee_name}`} disabled={runner.busy} onClick={() => void runner.run(() => authorized(token => deleteRoutineDuty(token, version.id, d.id)), 'e8Saved')}>{t('delete')}</button></td>}</tr>)}</tbody></table></div>}
    {editable && <form className="mt-4 grid gap-3 border-t border-slate-100 pt-4 sm:grid-cols-2 lg:grid-cols-5" onSubmit={e => { e.preventDefault(); void runner.run(() => authorized(token => addRoutineDuty(token, version.id, form)), 'e8Saved') }}>
      <label className={ui.label}>{t('e8Date')}<select className={field} required value={form.exam_date} onChange={e => setForm({ ...form, exam_date: e.target.value })}><option value="">{t('selectOption')}</option>{usedDates.map(d => <option key={d} value={d}>{day(d)}</option>)}</select></label>
      <label className={ui.label}>{t('e8Sitting')}<select className={field} required value={form.sitting_id} onChange={e => setForm({ ...form, sitting_id: e.target.value })}><option value="">{t('selectOption')}</option>{version.sittings.map(s => <option key={s.id} value={s.id}>{sittingName(s.id)}</option>)}</select></label>
      <label className={ui.label}>{t('e8Room')}<select className={field} required value={form.room_id} onChange={e => setForm({ ...form, room_id: e.target.value })}><option value="">{t('selectOption')}</option>{version.rooms.filter(r => r.is_active).map(r => <option key={r.id} value={r.id}>{name(r.name, r.name_bn)}</option>)}</select></label>
      <label className={ui.label}>{t('e8Teacher')}<select className={field} required value={form.employee_id} onChange={e => setForm({ ...form, employee_id: e.target.value })}><option value="">{t('selectOption')}</option>{version.teachers.map(x => <option key={x.id} value={x.id}>{name(x.name, x.name_bn)}</option>)}</select></label>
      <div className="self-end"><button className={ui.secondary} disabled={runner.busy}>{t('e8AddDuty')}</button></div>
    </form>}
  </section>
}

function DocumentsPanel({ version }: { version: RoutineVersion }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  async function download(kind: RoutineDocKind, lang: 'en' | 'bn') {
    setBusy(kind + lang); setError('')
    try { saveBlob(await authorized(token => downloadRoutinePdf(token, version.id, kind, lang)), `exam-routine-v${version.version}-${kind}-${lang}.pdf`) }
    catch (cause) { setError(t(routineErrorOf(cause).key === 'e8DocUnavailable' ? 'e8DocUnavailable' : 'e8DocError')) }
    finally { setBusy('') }
  }
  return <section className={card} aria-labelledby="e8-docs">
    <CardTitle id="e8-docs" icon="download">{t('e8Documents')}</CardTitle>
    <p className="mt-1 max-w-3xl text-sm text-slate-600">{t('e8DocumentsHint')}</p>
    {error && <div className="mt-3"><Notice tone="error">{error}</Notice></div>}
    <div className="mt-3 grid gap-3 sm:grid-cols-3">{(['class', 'room', 'invigilation'] as const).map(kind => <div key={kind} className="rounded-xl border border-slate-200 p-3">
      <p className="font-semibold text-slate-900">{t(`e8Doc_${kind}`)}</p>
      <div className="mt-2 flex flex-wrap gap-2">{(['en', 'bn'] as const).map(lang => <button key={lang} type="button" className={ui.secondary} disabled={!!busy} onClick={() => void download(kind, lang)}>
        <Icon name="download" className="h-4 w-4" />{t(lang === 'en' ? 'e8English' : 'e8Bangla')}</button>)}</div>
    </div>)}</div>
  </section>
}
