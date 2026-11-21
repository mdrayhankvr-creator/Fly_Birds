import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getClassBlueprints, getClassSubjects, getReligionChoices, getReligionCorrectionRequests, resolveReligionCorrection, saveReligionChoices, saveReligionOptions } from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { Notice, SectionCard, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'

// This roster is Admin-only. It is an academic course choice, never a field
// for a person's faith. There is deliberately no "select all" default.
export function ReligionChoices({ sessionId }: { sessionId: string }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const classes = useQuery({ queryKey: ['religion', 'classes', sessionId], enabled: !!sessionId, queryFn: () => authorized(token => getClassBlueprints(token, sessionId)), retry: false })
  const [requestedClass, setRequestedClass] = useState('')
  const classId = requestedClass || classes.data?.[0]?.class_id || ''
  const view = useQuery({ queryKey: ['religion', 'choices', sessionId, classId], enabled: !!classId, queryFn: () => authorized(token => getReligionChoices(token, sessionId, classId)), retry: false })
  const mappings = useQuery({ queryKey: ['religion', 'mappings', classId], enabled: !!classId, queryFn: () => authorized(token => getClassSubjects(token, classId, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const requests = useQuery({ queryKey: ['religion', 'requests'], queryFn: () => authorized(getReligionCorrectionRequests), retry: false })
  const [optionEdits, setOptionEdits] = useState<string[] | null>(null)
  const [choiceEdits, setChoiceEdits] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const chosenOptions = optionEdits ?? view.data?.options.map(o => o.id) ?? []
  const changed = Object.keys(choiceEdits).length

  async function saveOptions() {
    if (!view.data || !optionEdits || !await confirmAction(t('religionConfirmOptions'))) return
    setBusy(true); setError(''); setMessage('')
    try {
      await authorized(token => saveReligionOptions(token, sessionId, classId, optionEdits, view.data!.digest))
      setOptionEdits(null); setChoiceEdits({}); await client.invalidateQueries({ queryKey: ['religion', 'choices', sessionId, classId] })
      setMessage(t('religionOptionsSaved'))
    } catch { setError(t('religionStaleOrInvalid')) }
    finally { setBusy(false) }
  }

  async function saveChoices() {
    if (!view.data || changed === 0 || !await confirmAction(t('religionConfirmRoster', { count: changed }))) return
    setBusy(true); setError(''); setMessage('')
    try {
      await authorized(token => saveReligionChoices(token, sessionId, classId, Object.entries(choiceEdits).map(([student_id, subject_id]) => ({ student_id, subject_id })), view.data!.digest))
      setChoiceEdits({}); await client.invalidateQueries({ queryKey: ['religion', 'choices', sessionId, classId] })
      setMessage(t('religionChoicesSaved', { count: changed }))
    } catch { setError(t('religionStaleOrInvalid')) }
    finally { setBusy(false) }
  }

  async function reviewRequest(id: string, decision: 'resolved'|'rejected') {
    if (!await confirmAction(t(decision === 'resolved' ? 'religionRequestResolveConfirm' : 'religionRequestRejectConfirm'))) return
    setBusy(true); setError('')
    try { await authorized(token => resolveReligionCorrection(token, id, decision)); await client.invalidateQueries({ queryKey: ['religion', 'requests'] }) }
    catch { setError(t('religionStaleOrInvalid')) }
    finally { setBusy(false) }
  }

  return <SectionCard id="religion-choices" title={t('religionTitle')} description={t('religionIntro')}>
    <label className={`${ui.label} block max-w-xs`}>{t('bpClass')}
      <select className={ui.field} value={classId} onChange={e => { setRequestedClass(e.target.value); setOptionEdits(null); setChoiceEdits({}); setError(''); setMessage('') }}>
        {classes.data?.map(c => <option key={c.class_id} value={c.class_id}>{c.class_name}</option>)}
      </select>
    </label>
    {error && <div className="mt-3"><Notice tone="error">{error}</Notice></div>}
    {message && <div className="mt-3"><Notice tone="success">{message}</Notice></div>}
    {!classId || view.isPending ? <div className="mt-4"><Skeleton rows={3} /></div> : view.isError ? <div className="mt-4"><Notice tone="error">{t('religionLoadError')}</Notice></div> : <div className="mt-4 space-y-4">
      {view.data.warnings.map(w => <Notice key={w} tone="info">{t(`religionWarning_${w}`)}</Notice>)}
      <section className="rounded-xl border border-slate-200 p-4">
        <h3 className="font-semibold">{t('religionApprovedOptions')}</h3>
        <p className="mt-1 text-sm text-slate-600">{t('religionOptionsHelp')}</p>
        {mappings.isPending ? <Skeleton rows={2} /> : mappings.isError ? <Notice tone="error">{t('religionLoadError')}</Notice> : <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {mappings.data.items.filter(m => m.is_required).map(m => <label key={m.subject_id} className="flex items-center gap-2 rounded-lg border border-slate-200 p-2 text-sm">
            <input type="checkbox" checked={chosenOptions.includes(m.subject_id)} disabled={busy || changed > 0} onChange={e => setOptionEdits(current => e.target.checked ? [...(current ?? chosenOptions), m.subject_id] : (current ?? chosenOptions).filter(id => id !== m.subject_id))} />
            {i18n.language === 'bn' ? m.subject_name_bn : m.subject_name_en} ({m.subject_code})
          </label>)}
        </div>}
        {optionEdits && <button type="button" className={`${ui.secondary} mt-3`} disabled={busy} onClick={() => void saveOptions()}>{t('religionSaveOptions')}</button>}
      </section>
      <section className="rounded-xl border border-slate-200 p-4">
        <h3 className="font-semibold">{t('religionRequestQueue')}</h3>
        <p className="mt-1 text-sm text-slate-600">{t('religionRequestReviewHelp')}</p>
        {requests.isError ? <Notice tone="error">{t('religionLoadError')}</Notice> : !requests.data?.some(r => view.data.students.some(st => st.student_id === r.student_id)) ? <p className="mt-2 text-sm text-slate-600">{t('religionRequestNone')}</p> : <div className="mt-3 space-y-2">{requests.data.filter(r => view.data.students.some(st => st.student_id === r.student_id)).map(r => {
          const student = view.data.students.find(st => st.student_id === r.student_id)
          const option = view.data.options.find(o => o.id === r.proposed_subject_id)
          return <div key={r.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 p-3 text-sm"><span>{student?.name} · {option ? (i18n.language === 'bn' ? option.name_bn : option.name_en) : t('religionCourse')}</span><div className="flex gap-2"><button type="button" className={ui.secondary} disabled={busy} onClick={() => void reviewRequest(r.id, 'resolved')}>{t('religionRequestResolve')}</button><button type="button" className={ui.secondary} disabled={busy} onClick={() => void reviewRequest(r.id, 'rejected')}>{t('religionRequestReject')}</button></div></div>
        })}</div>}
      </section>
      <section className="rounded-xl border border-slate-200 p-4">
        <div className="flex flex-wrap items-center justify-between gap-2"><div><h3 className="font-semibold">{t('religionRoster')}</h3><p className="text-sm text-slate-600">{t('religionUnresolved', { count: view.data.unresolved })}</p></div>
          {changed > 0 && <button type="button" className={ui.primary} disabled={busy || !!optionEdits} onClick={() => void saveChoices()}>{t('religionSaveChoices', { count: changed })}</button>}</div>
        {view.data.students.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('religionNoStudents')}</p> : <div className="mt-3 max-h-[34rem] overflow-auto rounded-lg border border-slate-200">
          <table className="w-full min-w-[34rem] text-left text-sm"><thead className="sticky top-0 bg-slate-50"><tr><th className={ui.th}>{t('bpStudent')}</th><th className={ui.th}>{t('bpSection')}</th><th className={ui.th}>{t('religionCourse')}</th></tr></thead>
            <tbody>{view.data.students.map(st => <tr key={st.student_id} className={choiceEdits[st.student_id] !== undefined ? 'bg-amber-50' : ''}>
              <th scope="row" className={ui.td}>{st.name}<span className="block text-xs font-normal text-slate-500">{st.admission_no}</span></th>
              <td className={ui.td}>{st.section_name}</td><td className={ui.td}><select className={ui.field} aria-label={t('religionChooseFor', { name: st.name })} value={choiceEdits[st.student_id] ?? st.subject_id} disabled={busy || !!optionEdits} onChange={e => setChoiceEdits(current => {
                const next = { ...current }; if (e.target.value === st.subject_id) delete next[st.student_id]; else next[st.student_id] = e.target.value; return next
              })}><option value="">{t('religionUnselected')}</option>{view.data.options.filter(o => st.eligible_subject_ids.includes(o.id)).map(o => <option key={o.id} value={o.id}>{i18n.language === 'bn' ? o.name_bn : o.name_en} ({o.code})</option>)}</select></td>
            </tr>)}</tbody></table>
        </div>}
      </section>
    </div>}
  </SectionCard>
}
