import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { cancelResultCorrection, getResultCorrections, openResultCorrection, saveExamMarkCells, type CorrectionCell, type ExamMarkCell, type ExamMarkStatus, type ResultCorrection, type ResultExam } from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { Notice, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { useResultError, useWhen } from './resultUi'

// Corrections of a published (or locked) exam (E6, calculation rules §7):
// an administrator opens one per student with a reason, changes that
// student's marks here, then the change is reviewed, approved and
// published as a new version. Families keep seeing the published result
// until then; cancelling puts the published marks back.
const statuses: ExamMarkStatus[] = ['present', 'absent', 'exempt', 'incomplete', 'withheld']
const carries = (s: ExamMarkStatus | '') => s === 'present' || s === 'withheld'
const needsReason = (s: ExamMarkStatus | '') => s === 'exempt' || s === 'withheld'

export function ExamCorrections({ exam, canManage, students }: { exam: ResultExam; canManage: boolean; students: { id: string; name: string; admission_no: string }[] }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const explain = useResultError()
  const when = useWhen()
  const list = useQuery({ queryKey: ['e6', 'corrections', exam.id], queryFn: () => authorized(token => getResultCorrections(token, exam.id)), retry: false })
  const [student, setStudent] = useState('')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  async function run(task: () => Promise<unknown>, done: string) {
    setBusy(true); setError(''); setMessage('')
    try { await task(); await client.invalidateQueries({ queryKey: ['e6', 'corrections', exam.id] }); setMessage(done) } catch (cause) { setError(explain(cause)) } finally { setBusy(false) }
  }
  const open = (list.data || []).filter(c => c.status === 'open')
  const closed = (list.data || []).filter(c => c.status !== 'open')
  const review = `/admin/exams/results/review?scope=assessment&scope_id=${exam.id}&session=${exam.session_id}`
  return <section aria-labelledby="rv-corrections-title" className="mt-6 rounded-2xl border border-slate-200 p-4">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div><h3 id="rv-corrections-title" className="font-bold text-slate-900">{t('rvCorrectionsTitle')}</h3><p className="mt-0.5 text-sm text-slate-600">{t(exam.status === 'locked' ? 'rvCorrectionsLocked' : 'rvCorrectionsIntro')}</p></div>
      <Link to={review} className={ui.secondary}>{t('rvReviewCorrections')}</Link>
    </div>
    {(error || message) && <div className="mt-3"><Notice tone={error ? 'error' : 'success'}>{error || message}</Notice></div>}
    {canManage && <form className="mt-3 grid gap-2 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_auto] sm:items-end" onSubmit={e => { e.preventDefault(); void run(async () => { await authorized(token => openResultCorrection(token, exam.id, student, reason.trim())); setStudent(''); setReason('') }, t('rvCorrectionOpened')) }}>
      <label className={ui.label}>{t('rvStudent')}<select required className={ui.field} value={student} onChange={e => setStudent(e.target.value)}>
        <option value="">{t('rvChooseStudent')}</option>{students.filter(s => !open.some(c => c.student_id === s.id)).map(s => <option key={s.id} value={s.id}>{s.name} · {s.admission_no}</option>)}
      </select></label>
      <label className={ui.label}>{t('rvCorrectionReason')}<input required minLength={3} maxLength={500} className={ui.field} value={reason} onChange={e => setReason(e.target.value)} placeholder={t('rvCorrectionReasonHint')} /></label>
      <button disabled={busy || !student || reason.trim().length < 3} className={ui.secondary}>{t('rvOpenCorrection')}</button>
    </form>}
    {list.isPending ? <div className="mt-3"><Skeleton rows={2} /></div> : list.isError ? <div className="mt-3"><Notice tone="error">{explain(list.error)}</Notice></div> : <>
      {open.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('rvNoOpenCorrections')}</p> : <ul className="mt-3 space-y-3">{open.map(c => <CorrectionEditor key={c.id} exam={exam} c={c} canManage={canManage} busy={busy}
        onSaved={() => void client.invalidateQueries({ queryKey: ['e6', 'corrections', exam.id] })}
        onCancel={async () => { if (await confirmAction(t('rvConfirmCancelCorrection', { name: c.student_name }))) void run(() => authorized(token => cancelResultCorrection(token, exam.id, c.id)), t('rvCorrectionCancelled')) }} />)}</ul>}
      {closed.length > 0 && <details className="mt-3"><summary className="cursor-pointer text-sm font-semibold text-slate-700">{t('rvPastCorrections', { n: closed.length })}</summary>
        <ul className="mt-2 divide-y divide-slate-100 text-sm">{closed.map(c => <li key={c.id} className="py-2"><span className="font-medium text-slate-900">{c.student_name}</span> · {t(`rvCorrection_${c.status}`)} · {when(c.closed_at)}<span className="block text-slate-600">{c.reason}</span></li>)}</ul>
      </details>}
    </>}
  </section>
}

function CorrectionEditor({ exam, c, canManage, busy, onSaved, onCancel }: { exam: ResultExam; c: ResultCorrection; canManage: boolean; busy: boolean; onSaved: () => void; onCancel: () => void }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useResultError()
  const when = useWhen()
  const [draft, setDraft] = useState<Record<string, { status: ExamMarkStatus; marks: string; reason: string; components: Record<string, string> }>>({})
  const [saving, setSaving] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [note, setNote] = useState('')
  const value = (cell: CorrectionCell) => draft[cell.exam_subject_id] ?? { status: (cell.status || 'present') as ExamMarkStatus, marks: cell.marks == null ? '' : String(cell.marks), reason: cell.reason, components: Object.fromEntries(cell.components.map(x => [x.key, x.marks == null ? '' : String(x.marks)])) }
  const set = (cell: CorrectionCell, patch: Partial<ReturnType<typeof value>>) => setDraft({ ...draft, [cell.exam_subject_id]: { ...value(cell), ...patch } })
  async function save() {
    const entries: ExamMarkCell[] = Object.entries(draft).map(([sheet, d]) => {
      const cell = c.cells.find(x => x.exam_subject_id === sheet)!
      const comps = cell.components.length > 0 && d.status === 'present' ? Object.fromEntries(Object.entries(d.components).map(([k, v]) => [k, Number(v)])) : undefined
      return { exam_subject_id: sheet, student_id: c.student_id, status: d.status, marks: carries(d.status) && !comps ? (d.marks === '' ? null : Number(d.marks)) : null, reason: needsReason(d.status) ? d.reason : '', row_version: cell.row_version, ...(comps ? { components: comps } : {}) }
    })
    if (!entries.length) return
    setSaving(true); setNote(''); setErrors({})
    try {
      const result = await authorized(token => saveExamMarkCells(token, exam.id, entries))
      const failed: Record<string, string> = {}
      for (const e of result.errors) failed[e.exam_subject_id] = i18n.exists(`e3Err_${e.code}`) ? t(`e3Err_${e.code}`) : i18n.exists(`rvErr_${e.code}`) ? t(`rvErr_${e.code}`) : e.message
      setErrors(failed)
      setDraft(Object.fromEntries(Object.entries(draft).filter(([k]) => failed[k])))
      setNote(result.errors.length ? t('rvCellsNotSaved', { n: result.errors.length }) : t('rvCorrectionSaved'))
      onSaved()
    } catch (cause) { setNote(explain(cause)) } finally { setSaving(false) }
  }
  return <li className="rounded-xl border border-violet-300 p-3">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div><p className="font-semibold text-slate-900">{c.student_name} <span className="text-xs font-normal text-slate-500">{c.admission_no}</span></p><p className="text-sm text-slate-700">{c.reason}</p><p className="text-xs text-slate-500">{t('rvOpenedBy', { name: c.opened_by, when: when(c.opened_at) })}</p></div>
      {canManage && <button type="button" disabled={busy || saving} className={ui.ghost} onClick={onCancel}>{t('rvCancelCorrection')}</button>}
    </div>
    <div className="mt-2 overflow-x-auto"><table className="ds-cards w-full min-w-[34rem] text-left text-sm">
      <thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th scope="col" className="px-2 py-2">{t('rvSubject')}</th><th scope="col" className="px-2 py-2">{t('rvPublishedValue')}</th><th scope="col" className="px-2 py-2">{t('rvStatus')}</th><th scope="col" className="px-2 py-2">{t('rvMarks')}</th></tr></thead>
      <tbody>{c.cells.map(cell => {
        const v = value(cell); const err = errors[cell.exam_subject_id]
        const subject = i18n.language === 'bn' && cell.subject_name_bn ? cell.subject_name_bn : cell.subject_name
        return <tr key={cell.exam_subject_id} className={`border-b border-slate-100 align-top ${err ? 'bg-rose-50' : ''}`}>
          <th scope="row" className="px-2 py-2 font-medium">{subject}<span className="block text-xs font-normal text-slate-500">{t('rvOutOf', { full: cell.full_marks })}</span></th>
          <td className="px-2 py-2 tabular-nums text-slate-600">{cell.published_status && cell.published_status !== 'present' ? t(`e3Status_${cell.published_status}`) : (cell.published_marks ?? '—')}{cell.published_grade ? ` · ${cell.published_grade}` : ''}</td>
          <td className="px-2 py-2"><select aria-label={t('rvStatusFor', { subject })} disabled={!canManage} className="min-h-10 rounded-lg border border-slate-300 bg-white px-2 text-sm" value={v.status} onChange={e => set(cell, { status: e.target.value as ExamMarkStatus })}>{statuses.map(s => <option key={s} value={s}>{t(`e3Status_${s}`)}</option>)}</select>
            {needsReason(v.status) && <input aria-label={t('rvReasonFor', { subject })} maxLength={240} placeholder={t('e3StatusReason')} disabled={!canManage} className="mt-1.5 block min-h-9 w-44 rounded-lg border border-slate-300 bg-white px-2 text-sm" value={v.reason} onChange={e => set(cell, { reason: e.target.value })} />}</td>
          <td className="px-2 py-2">{cell.components.length > 0 && v.status === 'present' ? <div className="flex flex-wrap gap-2">{cell.components.map(x => <label key={x.key} className="text-xs font-semibold text-slate-600">{x.key} <span className="font-normal">/ {x.full}</span>
            <input type="number" min="0" max={x.full} step="0.01" disabled={!canManage} aria-label={t('rvComponentFor', { subject, component: x.key })} className="mt-0.5 block min-h-10 w-20 rounded-lg border border-slate-300 bg-white px-2 text-sm tabular-nums" value={v.components[x.key] ?? ''} onChange={e => set(cell, { components: { ...v.components, [x.key]: e.target.value } })} /></label>)}</div>
            : <input type="number" min="0" max={cell.full_marks} step="0.01" disabled={!canManage || !carries(v.status)} aria-label={t('rvMarksFor', { subject })} aria-invalid={!!err} aria-describedby={err ? `rv-err-${cell.exam_subject_id}` : undefined} className="min-h-10 w-24 rounded-lg border border-slate-300 bg-white px-2 text-sm tabular-nums disabled:bg-slate-50" value={carries(v.status) ? v.marks : ''} onChange={e => set(cell, { marks: e.target.value })} />}
            {err && <p id={`rv-err-${cell.exam_subject_id}`} role="alert" className="mt-1 text-xs font-medium text-rose-700">{err}</p>}</td>
        </tr>
      })}</tbody>
    </table></div>
    {canManage && <div className="mt-2 flex flex-wrap items-center gap-3"><button type="button" disabled={saving || Object.keys(draft).length === 0} className={ui.secondary} onClick={() => void save()}>{t('rvSaveCorrection')}</button>{note && <span role="status" className="text-sm text-slate-700">{note}</span>}</div>}
  </li>
}
