import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, deleteClassRoutineSlot, deleteExamRoutine, deleteExamRoutineSlot, getAcademicClasses, getAcademicSections, getAcademicSessions, getAcademicSubjects, getClassRoutine, getEmployees, getExamRoutine, getExamRoutines, publishExamRoutine, saveClassRoutineSlot, saveExamRoutine, saveExamRoutineSlot, type ClassRoutineInput, type ClassRoutineSlot, type ExamRoutine, type ExamRoutineInput, type ExamRoutineSlotInput } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

type Mode = 'admin' | 'teacher' | 'student' | 'guardian'
const field = 'w-full min-w-0 rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:border-[var(--brand-primary)] focus:outline-none'
const button = 'rounded-lg bg-teal-700 px-4 py-2 font-semibold text-white disabled:opacity-50'
const blankClass: ClassRoutineInput = { session_id: '', class_id: '', section_id: '', subject_id: '', teacher_id: '', day_of_week: 0, start_time: '09:00', end_time: '09:45', room: '' }
const blankExam: ExamRoutineInput = { name: '', session_id: '', class_id: '', section_id: '' }
const blankSlot: ExamRoutineSlotInput = { subject_id: '', exam_date: '', start_time: '10:00', end_time: '12:00', room: '' }
const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

export function RoutinePage({ mode, canManage }: { mode: Mode; canManage: boolean }) {
  const { t, i18n } = useTranslation()
  const { id: childId } = useParams()
  const { accessToken, renew, expire } = useAuth()
  const client = useQueryClient()
  const [tab, setTab] = useState<'class' | 'exam'>('class')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [sessionId, setSessionId] = useState('')
  const [classId, setClassId] = useState('')
  const [sectionId, setSectionId] = useState('')
  const [classForm, setClassForm] = useState<ClassRoutineInput>(blankClass)
  const [classEdit, setClassEdit] = useState('')
  const [examForm, setExamForm] = useState<ExamRoutineInput>(blankExam)
  const [examEdit, setExamEdit] = useState('')
  const [selectedExam, setSelectedExam] = useState('')
  const [slotForm, setSlotForm] = useState<ExamRoutineSlotInput>(blankSlot)
  const [slotEdit, setSlotEdit] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  async function authorized<T>(call: (token: string) => Promise<T>): Promise<T> {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await call(accessToken) }
    catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await call(await renew()) }
        catch (retryCause) { if (retryCause instanceof ApiError && retryCause.status === 401) expire(); throw retryCause }
      }
      throw cause
    }
  }
  async function run(action: () => Promise<unknown>) {
    setBusy(true); setError(''); setMessage('')
    try { await action(); await client.invalidateQueries({ queryKey: ['routine'] }); setMessage(t('routineSaved')) }
    catch (cause) { setError(cause instanceof ApiError && cause.code === 'schedule_conflict' ? t('routineConflict') : cause instanceof ApiError && cause.code === 'invalid_request' ? t('routineInvalid') : t('routineSaveError')) }
    finally { setBusy(false) }
  }
  const classPath = mode === 'admin' ? '/routines/classes' : mode === 'guardian' ? `/routines/classes/children/${encodeURIComponent(childId || '')}` : '/routines/classes/me'
  const examPath = mode === 'admin' ? '/routines/exams' : mode === 'guardian' ? `/routines/exams/children/${encodeURIComponent(childId || '')}` : '/routines/exams/me'
  const params = new URLSearchParams({ page: String(page), page_size: tab === 'class' ? '100' : '10', search })
  if (sessionId) params.set('session_id', sessionId)
  if (classId) params.set('class_id', classId)
  if (sectionId) params.set('section_id', sectionId)
  const classes = useQuery({ queryKey: ['routine', 'classes', mode, childId, params.toString()], enabled: tab === 'class' && (mode !== 'guardian' || !!childId), queryFn: () => authorized(token => getClassRoutine(token, classPath, params)), retry: false })
  const exams = useQuery({ queryKey: ['routine', 'exams', mode, childId, params.toString()], enabled: tab === 'exam' && mode !== 'teacher' && (mode !== 'guardian' || !!childId), queryFn: () => authorized(token => getExamRoutines(token, examPath, params)), retry: false })
  const detail = useQuery({ queryKey: ['routine', 'exam', mode, childId, selectedExam], enabled: tab === 'exam' && !!selectedExam, queryFn: () => authorized(token => getExamRoutine(token, `${examPath}/${encodeURIComponent(selectedExam)}`)), retry: false })
  const options = new URLSearchParams({ page: '1', page_size: '100' })
  const sessions = useQuery({ queryKey: ['routine', 'sessions'], enabled: mode === 'admin', queryFn: () => authorized(token => getAcademicSessions(token, options)), retry: false })
  const academicClasses = useQuery({ queryKey: ['routine', 'class-options'], enabled: mode === 'admin', queryFn: () => authorized(token => getAcademicClasses(token, options)), retry: false })
  const sections = useQuery({ queryKey: ['routine', 'sections'], enabled: mode === 'admin', queryFn: () => authorized(token => getAcademicSections(token, options)), retry: false })
  const subjects = useQuery({ queryKey: ['routine', 'subjects'], enabled: mode === 'admin', queryFn: () => authorized(token => getAcademicSubjects(token, options)), retry: false })
  const teachers = useQuery({ queryKey: ['routine', 'teachers'], enabled: mode === 'admin', queryFn: () => authorized(token => getEmployees(token, new URLSearchParams({ page: '1', page_size: '100', kind: 'teacher', status: 'active' }))), retry: false })

  function selectClass(slot: ClassRoutineSlot) {
    setClassEdit(slot.id)
    setClassForm({ session_id: slot.session_id, class_id: slot.class_id, section_id: slot.section_id, subject_id: slot.subject_id, teacher_id: slot.teacher_id, day_of_week: slot.day_of_week, start_time: slot.start_time, end_time: slot.end_time, room: slot.room })
  }
  function selectExam(exam: ExamRoutine) { setSelectedExam(exam.id); setExamEdit(exam.id); setExamForm({ name: exam.name, session_id: exam.session_id, class_id: exam.class_id, section_id: exam.section_id }); setSlotEdit(''); setSlotForm(blankSlot) }
  function saveClass(event: FormEvent<HTMLFormElement>) { event.preventDefault(); void run(async () => { await authorized(token => saveClassRoutineSlot(token, classForm, classEdit || undefined)); setClassForm(blankClass); setClassEdit('') }) }
  function saveExam(event: FormEvent<HTMLFormElement>) { event.preventDefault(); void run(async () => { const value = await authorized(token => saveExamRoutine(token, examForm, examEdit || undefined)); setSelectedExam(value.id); setExamEdit(value.id); setExamForm({ name: value.name, session_id: value.session_id, class_id: value.class_id, section_id: value.section_id }) }) }
  function saveSlot(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (!selectedExam) return; void run(async () => { await authorized(token => saveExamRoutineSlot(token, selectedExam, slotForm, slotEdit || undefined)); setSlotForm(blankSlot); setSlotEdit('') }) }
  const classItems = classes.data?.items || []
  const examItems = exams.data?.items || []
  const activeExam = detail.data
  const pages = Math.max(1, Math.ceil((tab === 'class' ? classes.data?.total || 0 : exams.data?.total || 0) / (tab === 'class' ? 100 : 10)))
  const sessionSections = (target: { session_id: string; class_id: string }) => (sections.data?.items || []).filter(item => item.session_id === target.session_id && item.class_id === target.class_id && item.is_active)
  const name = (en: string, bn: string) => i18n.language === 'bn' && bn ? bn : en
  const msg = error || message

  return <div className="space-y-5">
    <header className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm print:border-0 print:shadow-none">
      <h1 className="text-2xl font-bold text-slate-900">{t('routineHeading')}</h1><p className="mt-1 text-slate-600">{t('routineDescription')}</p>
      <div className="mt-4 flex flex-wrap gap-2 print:hidden"><button className={tab === 'class' ? button : 'rounded-lg border px-4 py-2'} onClick={() => { setTab('class'); setPage(1) }}>{t('classRoutine')}</button>{mode !== 'teacher' && <button className={tab === 'exam' ? button : 'rounded-lg border px-4 py-2'} onClick={() => { setTab('exam'); setPage(1) }}>{t('examRoutine')}</button>}</div>
    </header>
    {msg && <p role={error ? 'alert' : 'status'} className={`rounded-lg p-3 ${error ? 'bg-rose-50 text-rose-800' : 'bg-emerald-50 text-emerald-800'}`}>{msg}</p>}
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm print:hidden">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"><label className="text-sm font-semibold">{t('search')}<input className={field} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></label>
      {mode === 'admin' && <><label className="text-sm font-semibold">{t('academicSessions')}<select className={field} value={sessionId} onChange={e => { setSessionId(e.target.value); setClassId(''); setSectionId(''); setPage(1) }}><option value="">{t('allSessions')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label><label className="text-sm font-semibold">{t('classesHeading')}<select className={field} value={classId} onChange={e => { setClassId(e.target.value); setSectionId(''); setPage(1) }}><option value="">{t('allClasses')}</option>{academicClasses.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label><label className="text-sm font-semibold">{t('sectionsHeading')}<select className={field} value={sectionId} onChange={e => { setSectionId(e.target.value); setPage(1) }}><option value="">{t('allSections')}</option>{sections.data?.items.filter(s => (!sessionId || s.session_id === sessionId) && (!classId || s.class_id === classId)).map(s => <option key={s.id} value={s.id}>{s.class_name} {s.name}</option>)}</select></label></>}
      </div>
    </section>
    {tab === 'class' ? <>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-xl font-bold">{t('weeklyTimetable')}</h2><button type="button" onClick={() => window.print()} className="rounded-lg border px-3 py-2 text-sm print:hidden">{t('printRoutine')}</button></div>
        {classes.isPending ? <p role="status" className="mt-4">{t('routineLoading')}</p> : classes.isError ? <p role="alert" className="mt-4 text-rose-700">{t('routineLoadError')}</p> : classItems.length === 0 ? <p className="mt-4 text-slate-600">{t('routineEmpty')}</p> : <div className="mt-4 grid gap-3 lg:grid-cols-7">{days.map((day, index) => <div key={day} className="min-w-0 rounded-xl bg-slate-50 p-3"><h3 className="font-semibold text-teal-800">{t(`routineDay${index}`)}</h3><div className="mt-3 space-y-2">{classItems.filter(item => item.day_of_week === index).map(item => <article key={item.id} className="rounded-lg border border-slate-200 bg-white p-3 text-sm"><p className="font-bold">{item.start_time}–{item.end_time}</p><p>{name(item.subject_name, item.subject_name_bn)}</p><p className="text-slate-600">{item.class_name} · {item.section_name}</p><p className="text-slate-600">{item.teacher_name} · {item.room}</p>{canManage && <div className="mt-2 flex gap-2 print:hidden"><button className="text-teal-800 underline" onClick={() => selectClass(item)}>{t('edit')}</button><button className="text-rose-700 underline" disabled={busy} onClick={() => void run(() => authorized(token => deleteClassRoutineSlot(token, item.id)))}>{t('delete')}</button></div>}</article>)}</div></div>)}</div>}
      </section>
      {canManage && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm print:hidden"><h2 className="text-xl font-bold">{t(classEdit ? 'editClassPeriod' : 'addClassPeriod')}</h2><form className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3" onSubmit={saveClass}>
        <label className="text-sm font-semibold">{t('academicSessions')}<select className={field} required value={classForm.session_id} onChange={e => setClassForm({ ...classForm, session_id: e.target.value, section_id: '' })}><option value="">{t('selectOption')}</option>{sessions.data?.items.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label>
        <label className="text-sm font-semibold">{t('classesHeading')}<select className={field} required value={classForm.class_id} onChange={e => setClassForm({ ...classForm, class_id: e.target.value, section_id: '' })}><option value="">{t('selectOption')}</option>{academicClasses.data?.items.filter(v => v.is_active).map(v => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label>
        <label className="text-sm font-semibold">{t('sectionsHeading')}<select className={field} required value={classForm.section_id} onChange={e => setClassForm({ ...classForm, section_id: e.target.value })}><option value="">{t('selectOption')}</option>{sessionSections(classForm).map(v => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label>
        <label className="text-sm font-semibold">{t('subjectsHeading')}<select className={field} required value={classForm.subject_id} onChange={e => setClassForm({ ...classForm, subject_id: e.target.value })}><option value="">{t('selectOption')}</option>{subjects.data?.items.filter(v => v.is_active).map(v => <option key={v.id} value={v.id}>{name(v.name_en, v.name_bn)}</option>)}</select></label>
        <label className="text-sm font-semibold">{t('routineTeacher')}<select className={field} required value={classForm.teacher_id} onChange={e => setClassForm({ ...classForm, teacher_id: e.target.value })}><option value="">{t('selectOption')}</option>{teachers.data?.items.filter(v => v.kind === 'teacher' && v.is_active).map(v => <option key={v.id} value={v.id}>{name(v.name_en, v.name_bn)}</option>)}</select></label>
        <label className="text-sm font-semibold">{t('routineDay')}<select className={field} value={classForm.day_of_week} onChange={e => setClassForm({ ...classForm, day_of_week: Number(e.target.value) })}>{days.map((v,i) => <option key={v} value={i}>{t(`routineDay${i}`)}</option>)}</select></label>
        <label className="text-sm font-semibold">{t('routineStart')}<input className={field} type="time" required value={classForm.start_time} onChange={e => setClassForm({ ...classForm, start_time: e.target.value })} /></label><label className="text-sm font-semibold">{t('routineEnd')}<input className={field} type="time" required min={classForm.start_time} value={classForm.end_time} onChange={e => setClassForm({ ...classForm, end_time: e.target.value })} /></label><label className="text-sm font-semibold">{t('routineRoom')}<input className={field} required maxLength={80} value={classForm.room} onChange={e => setClassForm({ ...classForm, room: e.target.value })} /></label>
        <div className="flex gap-2 sm:col-span-2 lg:col-span-3"><button className={button} disabled={busy}>{t('saveChanges')}</button>{classEdit && <button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setClassEdit(''); setClassForm(blankClass) }}>{t('cancel')}</button>}</div>
      </form></section>}
    </> : <>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm print:hidden"><h2 className="text-xl font-bold">{t('examRoutine')}</h2>{exams.isPending ? <p role="status" className="mt-4">{t('routineLoading')}</p> : exams.isError ? <p role="alert" className="mt-4 text-rose-700">{t('routineLoadError')}</p> : examItems.length === 0 ? <p className="mt-4 text-slate-600">{t('routineEmpty')}</p> : <ul className="mt-4 grid gap-2 sm:grid-cols-2">{examItems.map(item => <li key={item.id}><button className={`w-full rounded-xl border p-4 text-left hover:border-teal-500 ${selectedExam === item.id ? 'border-teal-600 bg-teal-50' : 'border-slate-200'}`} onClick={() => selectExam(item)}><span className="font-bold">{item.name}</span><span className="block text-sm text-slate-600">{item.class_name} · {item.section_name} · {t(item.is_published ? 'routinePublished' : 'routineDraft')}</span></button></li>)}</ul>}</section>
      {canManage && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm print:hidden"><h2 className="text-xl font-bold">{t(examEdit ? 'editExamRoutine' : 'addExamRoutine')}</h2><form className="mt-4 grid gap-3 sm:grid-cols-2" onSubmit={saveExam}><label className="text-sm font-semibold">{t('routineExamName')}<input required maxLength={120} className={field} value={examForm.name} onChange={e => setExamForm({ ...examForm, name: e.target.value })} /></label><label className="text-sm font-semibold">{t('academicSessions')}<select required className={field} value={examForm.session_id} onChange={e => setExamForm({ ...examForm, session_id: e.target.value, section_id: '' })}><option value="">{t('selectOption')}</option>{sessions.data?.items.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label><label className="text-sm font-semibold">{t('classesHeading')}<select required className={field} value={examForm.class_id} onChange={e => setExamForm({ ...examForm, class_id: e.target.value, section_id: '' })}><option value="">{t('selectOption')}</option>{academicClasses.data?.items.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label><label className="text-sm font-semibold">{t('sectionsHeading')}<select required className={field} value={examForm.section_id} onChange={e => setExamForm({ ...examForm, section_id: e.target.value })}><option value="">{t('selectOption')}</option>{sessionSections(examForm).map(v => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label><div className="flex gap-2 sm:col-span-2"><button disabled={busy || !!activeExam?.is_published} className={button}>{t('saveChanges')}</button><button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setExamEdit(''); setExamForm(blankExam); setSelectedExam('') }}>{t('newRoutine')}</button></div></form></section>}
      {selectedExam && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm print:border-0 print:shadow-none">{detail.isPending ? <p role="status">{t('routineLoading')}</p> : detail.isError ? <p role="alert" className="text-rose-700">{t('routineLoadError')}</p> : activeExam && <><div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-xl font-bold">{activeExam.name}</h2><p className="text-slate-600">{activeExam.class_name} · {activeExam.section_name} · {t(activeExam.is_published ? 'routinePublished' : 'routineDraft')}</p></div><div className="flex flex-wrap gap-2 print:hidden"><button className="rounded-lg border px-3 py-2" onClick={() => window.print()}>{t('printRoutine')}</button>{canManage && <><button disabled={busy} className={button} onClick={() => void run(() => authorized(token => publishExamRoutine(token, activeExam.id, !activeExam.is_published)))}>{t(activeExam.is_published ? 'unpublishRoutine' : 'publishRoutine')}</button>{!activeExam.is_published && <button disabled={busy} className="rounded-lg border border-rose-300 px-3 py-2 text-rose-700" onClick={() => void run(async () => { await authorized(token => deleteExamRoutine(token, activeExam.id)); setSelectedExam(''); setExamEdit(''); setExamForm(blankExam) })}>{t('delete')}</button>}</>}</div></div>
        {activeExam.slots.length === 0 ? <p className="mt-4 text-slate-600">{t('routineEmpty')}</p> : <div className="mt-4 overflow-x-auto"><table className="w-full min-w-[34rem] border-collapse text-left text-sm"><thead><tr className="border-b bg-slate-50"><th className="p-2">{t('routineDate')}</th><th className="p-2">{t('subjectsHeading')}</th><th className="p-2">{t('routineTime')}</th><th className="p-2">{t('routineRoom')}</th>{canManage && <th className="p-2 print:hidden">{t('actions')}</th>}</tr></thead><tbody>{activeExam.slots.map(slot => <tr key={slot.id} className="border-b"><td className="p-2">{slot.exam_date}</td><td className="p-2">{name(slot.subject_name, slot.subject_name_bn)}</td><td className="p-2">{slot.start_time}–{slot.end_time}</td><td className="p-2">{slot.room}</td>{canManage && <td className="p-2 print:hidden">{!activeExam.is_published && <><button className="mr-2 text-teal-800 underline" onClick={() => { setSlotEdit(slot.id); setSlotForm({ subject_id: slot.subject_id, exam_date: slot.exam_date, start_time: slot.start_time, end_time: slot.end_time, room: slot.room }) }}>{t('edit')}</button><button className="text-rose-700 underline" disabled={busy} onClick={() => void run(() => authorized(token => deleteExamRoutineSlot(token, activeExam.id, slot.id)))}>{t('delete')}</button></>}</td>}</tr>)}</tbody></table></div>}
        {canManage && !activeExam.is_published && <form className="mt-6 grid gap-3 border-t pt-5 sm:grid-cols-2 lg:grid-cols-5 print:hidden" onSubmit={saveSlot}><label className="text-sm font-semibold">{t('subjectsHeading')}<select required className={field} value={slotForm.subject_id} onChange={e => setSlotForm({ ...slotForm, subject_id: e.target.value })}><option value="">{t('selectOption')}</option>{subjects.data?.items.map(v => <option key={v.id} value={v.id}>{name(v.name_en, v.name_bn)}</option>)}</select></label><label className="text-sm font-semibold">{t('routineDate')}<input required type="date" className={field} value={slotForm.exam_date} onChange={e => setSlotForm({ ...slotForm, exam_date: e.target.value })} /></label><label className="text-sm font-semibold">{t('routineStart')}<input required type="time" className={field} value={slotForm.start_time} onChange={e => setSlotForm({ ...slotForm, start_time: e.target.value })} /></label><label className="text-sm font-semibold">{t('routineEnd')}<input required type="time" min={slotForm.start_time} className={field} value={slotForm.end_time} onChange={e => setSlotForm({ ...slotForm, end_time: e.target.value })} /></label><label className="text-sm font-semibold">{t('routineRoom')}<input required maxLength={80} className={field} value={slotForm.room} onChange={e => setSlotForm({ ...slotForm, room: e.target.value })} /></label><div className="flex gap-2 sm:col-span-2 lg:col-span-5"><button disabled={busy} className={button}>{t(slotEdit ? 'saveChanges' : 'addExamSlot')}</button>{slotEdit && <button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setSlotEdit(''); setSlotForm(blankSlot) }}>{t('cancel')}</button>}</div></form>}
      </>}</section>}
    </>}
    <div className="flex items-center justify-between gap-3 text-sm print:hidden"><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page <= 1} onClick={() => setPage(page-1)}>{t('previous')}</button><span>{t('pageOf', { page, total: pages })}</span><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page >= pages} onClick={() => setPage(page+1)}>{t('next')}</button></div>
  </div>
}
