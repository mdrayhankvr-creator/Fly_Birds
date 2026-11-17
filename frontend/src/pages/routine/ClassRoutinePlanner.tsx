import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, getClassPlanDraft, previewClassPlan, publishClassPlan, saveClassPlanDraft, type ClassPlan, type ClassPlanPeriod, type ClassPlanPreview, type ClassPlanVersion, type ClassRoutineInput } from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { usePeopleApi } from '../peopleShared'
import { Notice, ui } from '../attendanceUi'

type Session = { id: string; name: string; is_active: boolean }
const empty = (sessionId: string): ClassPlan => ({ session_id: sessionId, config: { start_time: '', end_time: '', lunch_start: '', lunch_minutes: 45, period_minutes: 0, transition_minutes: 0, days: [], availability_confirmed: false, frequencies: [], unavailable: [] }, slots: [] })
const blankSlot = (sessionId: string): ClassRoutineInput => ({ session_id: sessionId, class_id: '', section_id: '', subject_id: '', teacher_id: '', day_of_week: 0, start_time: '', end_time: '', room: '' })
const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const periodKey = (p: ClassPlanPeriod) => `${p.day_of_week}|${p.start_time}|${p.end_time}`

export function ClassRoutinePlanner({ sessions }: { sessions: Session[] }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const [requestedSession, setRequestedSession] = useState('')
  const sessionId = requestedSession || sessions.find(s => s.is_active)?.id || ''
  const [plan, setPlan] = useState<ClassPlan>(() => empty(''))
  const [preview, setPreview] = useState<ClassPlanPreview | null>(null)
  const [catalogue, setCatalogue] = useState<ClassPlanPreview['catalogue'] | null>(null)
  const [grid, setGrid] = useState<ClassPlanPeriod[]>([])
  const [draft, setDraft] = useState<ClassPlanVersion | null>(null)
  const [draftSaved, setDraftSaved] = useState(false)
  const [slot, setSlot] = useState<ClassRoutineInput>(() => blankSlot(''))
  const [editIndex, setEditIndex] = useState<number | null>(null)
  const [unavailableTeacher, setUnavailableTeacher] = useState('')
  const [unavailableDay, setUnavailableDay] = useState(0)
  const [unavailableStart, setUnavailableStart] = useState('')
  const [unavailableEnd, setUnavailableEnd] = useState('')
  const [viewSection, setViewSection] = useState('')
  const [viewDay, setViewDay] = useState<number | null>(null)
  const [viewTeacher, setViewTeacher] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!sessionId) return
    let alive = true
    setPlan(empty(sessionId)); setPreview(null); setCatalogue(null); setGrid([]); setDraft(null); setDraftSaved(false); setSlot(blankSlot(sessionId)); setError('')
    void authorized(token => getClassPlanDraft(token, sessionId)).then(value => {
      if (!alive) return
      setPlan(value.plan); setDraft(value); setDraftSaved(true)
    }).catch(cause => { if (alive && !(cause instanceof ApiError && cause.status === 404)) setError(t('crpLoadError')) })
    return () => { alive = false }
  }, [sessionId, authorized])

  function change(update: (current: ClassPlan) => ClassPlan, configChanged = false) {
    setPlan(update); setPreview(null); setDraftSaved(false); setMessage('')
    if (configChanged) setGrid([])
  }
  function config<K extends keyof ClassPlan['config']>(key: K, value: ClassPlan['config'][K], gridChanged = false) {
    change(current => ({ ...current, config: { ...current.config, [key]: value } }), gridChanged)
  }
  async function run(action: () => Promise<void>) {
    setBusy(true); setError(''); setMessage('')
    try { await action() }
    catch (cause) { setError(cause instanceof ApiError && cause.status === 409 ? t('crpStale') : t('crpSaveError')) }
    finally { setBusy(false) }
  }
  const teachers = Array.from(new Map((catalogue?.assignments ?? []).map(a => [a.teacher_id, a.teacher_name])).entries())
  const activeSection = catalogue?.sections.find(s => s.id === slot.section_id)
  const subjects = catalogue?.subjects.filter(s => s.class_id === activeSection?.class_id) ?? []
  const assignments = catalogue?.assignments.filter(a => a.class_id === activeSection?.class_id && a.subject_id === slot.subject_id) ?? []
  const candidatePeriods = grid.filter(p => p.kind === 'lesson' && plan.config.days.includes(p.day_of_week))
  const filteredSlots = plan.slots.map((value, index) => ({ value, index })).filter(({ value }) => (!viewSection || value.section_id === viewSection) && (viewDay === null || value.day_of_week === viewDay) && (!viewTeacher || value.teacher_id === viewTeacher))
  const sectionPeriods = viewSection ? grid.filter(p => viewDay === null || p.day_of_week === viewDay) : []

  return <div className="space-y-4">
    <section className={`${ui.card} space-y-4 p-4 sm:p-6`}>
      <div><h2 className="text-xl font-bold text-slate-900">{t('crpTitle')}</h2><p className="text-sm text-slate-600">{t('crpIntro')}</p></div>
      <label className={`${ui.label} block max-w-sm`}>{t('academicSessions')}<select className={ui.field} value={sessionId} onChange={event => setRequestedSession(event.target.value)}>{sessions.filter(s => s.is_active).map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
      {!sessionId && <Notice tone="info">{t('crpNoSession')}</Notice>}
      {error && <Notice tone="error">{error}</Notice>}{message && <Notice tone="success">{message}</Notice>}
      {!!sessionId && <>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {([['start_time', 'crpStart'], ['end_time', 'crpEnd'], ['lunch_start', 'crpLunchStart']] as const).map(([key, label]) => <label key={key} className={ui.label}>{t(label)}<input className={ui.field} type="time" value={plan.config[key]} onChange={e => config(key, e.target.value, true)} /></label>)}
          {([['period_minutes', 'crpPeriod'], ['transition_minutes', 'crpTransition']] as const).map(([key, label]) => <label key={key} className={ui.label}>{t(label)}<input className={ui.field} type="number" min={key === 'period_minutes' ? 20 : 0} max={key === 'period_minutes' ? 120 : 30} value={plan.config[key] || ''} onChange={e => config(key, Number(e.target.value), true)} /></label>)}
          <div className={`${ui.label} rounded-lg border border-amber-200 bg-amber-50 p-3`}>{t('crpLunchLength')}<strong className="block text-lg">45 {t('crpMinutes')}</strong></div>
        </div>
        <fieldset className="space-y-2"><legend className="text-sm font-semibold text-slate-900">{t('crpTeachingDays')}</legend><div className="flex flex-wrap gap-2">{dayNames.map((day, index) => <label key={day} className={`rounded-lg border px-3 py-2 text-sm ${catalogue?.off_days.includes(index) ? 'opacity-50' : ''}`}><input type="checkbox" className="mr-2" disabled={catalogue?.off_days.includes(index)} checked={plan.config.days.includes(index)} onChange={e => config('days', e.target.checked ? [...plan.config.days, index].sort() : plan.config.days.filter(d => d !== index), true)} />{t(`routineDay${index}`)}</label>)}</div>{catalogue && <p className="text-xs text-slate-500">{t('crpOffDays')}: {catalogue.off_days.map(d => t(`routineDay${d}`)).join(', ')}</p>}</fieldset>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={plan.config.availability_confirmed} onChange={e => config('availability_confirmed', e.target.checked)} />{t('crpAvailabilityConfirmed')}</label>
      </>}
    </section>
    {!!sessionId && <section className={`${ui.card} space-y-4 p-4 sm:p-6`}>
      <div><h3 className="text-lg font-semibold">{t('crpRequirements')}</h3><p className="text-sm text-slate-600">{t('crpRequirementsHelp')}</p></div>
      {!catalogue ? <p className="text-sm text-slate-600">{t('crpPreviewFirst')}</p> : <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{catalogue.subjects.map(subject => {
        const value = plan.config.frequencies.find(f => f.class_id === subject.class_id && f.subject_id === subject.id)?.periods_per_week
        const className = catalogue.sections.find(sec => sec.class_id === subject.class_id)?.class_name ?? ''
        return <label key={`${subject.class_id}|${subject.id}`} className={`${ui.label} rounded-lg border border-slate-200 p-3`}>{className} · {subject.name}<input className={ui.field} type="number" min={0} max={30} placeholder={t('crpFrequencyMissing')} value={value ?? ''} onChange={e => change(current => ({ ...current, config: { ...current.config, frequencies: [...current.config.frequencies.filter(f => f.class_id !== subject.class_id || f.subject_id !== subject.id), ...(e.target.value === '' ? [] : [{ class_id: subject.class_id, subject_id: subject.id, periods_per_week: Number(e.target.value) }])] } }))} /></label>
      })}</div>}
    </section>}
    {!!sessionId && <section className={`${ui.card} space-y-3 p-4 sm:p-6`}>
      <h3 className="text-lg font-semibold">{t('crpUnavailable')}</h3><p className="text-sm text-slate-600">{t('crpUnavailableHelp')}</p>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5"><select aria-label={t('routineTeacher')} className={ui.field} value={unavailableTeacher} onChange={e => setUnavailableTeacher(e.target.value)}><option value="">{t('routineTeacher')}</option>{teachers.map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select><select aria-label={t('routineDay')} className={ui.field} value={unavailableDay} onChange={e => setUnavailableDay(Number(e.target.value))}>{dayNames.map((_, i) => <option key={i} value={i}>{t(`routineDay${i}`)}</option>)}</select><input aria-label={t('routineStart')} className={ui.field} type="time" value={unavailableStart} onChange={e => setUnavailableStart(e.target.value)} /><input aria-label={t('routineEnd')} className={ui.field} type="time" value={unavailableEnd} onChange={e => setUnavailableEnd(e.target.value)} /><button type="button" className={ui.secondary} disabled={!unavailableTeacher || !unavailableStart || !unavailableEnd} onClick={() => { config('unavailable', [...plan.config.unavailable, { teacher_id: unavailableTeacher, day_of_week: unavailableDay, start_time: unavailableStart, end_time: unavailableEnd }]); setUnavailableStart(''); setUnavailableEnd('') }}>{t('crpAddBlock')}</button></div>
      <ul className="space-y-1">{plan.config.unavailable.map((block, i) => <li key={`${block.teacher_id}|${block.day_of_week}|${i}`} className="flex flex-wrap items-center gap-2 text-sm"><span>{teachers.find(([id]) => id === block.teacher_id)?.[1] ?? block.teacher_id} · {t(`routineDay${block.day_of_week}`)} · {block.start_time}–{block.end_time}</span><button className={ui.ghost} type="button" onClick={() => config('unavailable', plan.config.unavailable.filter((_, n) => n !== i))}>{t('delete')}</button></li>)}</ul>
    </section>}
    {!!sessionId && <section className={`${ui.card} space-y-4 p-4 sm:p-6`}>
      <h3 className="text-lg font-semibold">{t('crpManual')}</h3><p className="text-sm text-slate-600">{t('crpManualHelp')}</p>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3"><select aria-label={t('sectionsHeading')} className={ui.field} value={slot.section_id} onChange={e => { const sec = catalogue?.sections.find(s => s.id === e.target.value); setSlot({ ...slot, section_id: e.target.value, class_id: sec?.class_id ?? '', subject_id: '', teacher_id: '' }) }}><option value="">{t('sectionsHeading')}</option>{catalogue?.sections.map(s => <option key={s.id} value={s.id}>{s.class_name} · {s.name}</option>)}</select><select aria-label={t('subjectsHeading')} className={ui.field} value={slot.subject_id} onChange={e => setSlot({ ...slot, subject_id: e.target.value, teacher_id: '' })}><option value="">{t('subjectsHeading')}</option>{subjects.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select><select aria-label={t('routineTeacher')} className={ui.field} value={slot.teacher_id} onChange={e => setSlot({ ...slot, teacher_id: e.target.value })}><option value="">{t('routineTeacher')}</option>{assignments.map(a => <option key={a.teacher_id} value={a.teacher_id}>{a.teacher_name}</option>)}</select><select aria-label={t('routineTime')} className={ui.field} value={`${slot.day_of_week}|${slot.start_time}|${slot.end_time}`} onChange={e => { const p = candidatePeriods.find(value => periodKey(value) === e.target.value); if (p) setSlot({ ...slot, day_of_week: p.day_of_week, start_time: p.start_time, end_time: p.end_time }) }}><option value="0||">{t('crpChoosePeriod')}</option>{candidatePeriods.map(p => <option key={periodKey(p)} value={periodKey(p)}>{t(`routineDay${p.day_of_week}`)} · {p.start_time}–{p.end_time}</option>)}</select><select aria-label={t('routineRoom')} className={ui.field} value={slot.room} onChange={e => setSlot({ ...slot, room: e.target.value })}><option value="">{t('routineRoom')}</option>{catalogue?.rooms.map(room => <option key={room} value={room}>{room}</option>)}</select><button type="button" className={ui.secondary} disabled={!slot.section_id || !slot.subject_id || !slot.teacher_id || !slot.start_time || !slot.room} onClick={() => { change(current => ({ ...current, slots: editIndex === null ? [...current.slots, slot] : current.slots.map((value, index) => index === editIndex ? slot : value) })); setSlot(blankSlot(sessionId)); setEditIndex(null) }}>{t(editIndex === null ? 'crpAddLesson' : 'saveChanges')}</button></div>
      <div className="flex flex-wrap gap-2"><label className={ui.label}>{t('sectionsHeading')}<select className={ui.field} value={viewSection} onChange={e => setViewSection(e.target.value)}><option value="">{t('allSections')}</option>{catalogue?.sections.map(s => <option key={s.id} value={s.id}>{s.class_name} · {s.name}</option>)}</select></label><label className={ui.label}>{t('routineDay')}<select className={ui.field} value={viewDay ?? ''} onChange={e => setViewDay(e.target.value === '' ? null : Number(e.target.value))}><option value="">{t('crpAllDays')}</option>{dayNames.map((_, i) => <option key={i} value={i}>{t(`routineDay${i}`)}</option>)}</select></label><label className={ui.label}>{t('crpTeacherView')}<select className={ui.field} value={viewTeacher} onChange={e => setViewTeacher(e.target.value)}><option value="">{t('crpAllTeachers')}</option>{teachers.map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></label></div>
      {filteredSlots.length === 0 ? <Notice tone="info">{t('crpEmpty')}</Notice> : <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{filteredSlots.map(({ value, index }) => <article key={index} className="rounded-xl border border-slate-200 p-3 text-sm"><strong>{t(`routineDay${value.day_of_week}`)} · {value.start_time}–{value.end_time}</strong><p>{catalogue?.sections.find(s => s.id === value.section_id)?.class_name} · {catalogue?.sections.find(s => s.id === value.section_id)?.name}</p><p>{catalogue?.subjects.find(s => s.id === value.subject_id)?.name} · {teachers.find(([id]) => id === value.teacher_id)?.[1]} · {value.room}</p><div className="mt-2 flex gap-2"><button type="button" className={ui.ghost} onClick={() => { setSlot(value); setEditIndex(index) }}>{t('edit')}</button><button type="button" className={ui.ghost} onClick={() => change(current => ({ ...current, slots: current.slots.filter((_, i) => i !== index) }))}>{t('delete')}</button></div></article>)}</div>}
      {sectionPeriods.length > 0 && <div><h4 className="mb-2 font-semibold">{t('crpSectionGrid')}</h4><div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{sectionPeriods.map(period => {
        const lesson = plan.slots.find(value => value.section_id === viewSection && value.day_of_week === period.day_of_week && value.start_time === period.start_time && value.end_time === period.end_time)
        return <div key={periodKey(period)} className={`rounded-lg border p-3 text-sm ${period.kind === 'lunch' ? 'border-amber-300 bg-amber-50' : lesson ? 'border-blue-200 bg-blue-50' : 'border-slate-200'}`}><strong>{t(`routineDay${period.day_of_week}`)} · {period.start_time}–{period.end_time}</strong><span className="block">{period.kind === 'lunch' ? t('crpLunch') : lesson ? `${catalogue?.subjects.find(subject => subject.id === lesson.subject_id)?.name ?? ''} · ${teachers.find(([id]) => id === lesson.teacher_id)?.[1] ?? ''} · ${lesson.room}` : t('crpFree')}</span></div>
      })}</div></div>}
    </section>}
    {!!sessionId && <section className={`${ui.card} space-y-4 p-4 sm:p-6`}>
      <div className="flex flex-wrap gap-2"><button type="button" className={ui.secondary} disabled={busy} onClick={() => void run(async () => { const result = await authorized(token => previewClassPlan(token, plan)); setPreview(result); setCatalogue(result.catalogue); setGrid(result.periods); setMessage(t('crpPreviewReady')) })}>{t('crpPreview')}</button><button type="button" className={ui.secondary} disabled={busy || !preview} onClick={() => void run(async () => { const value = await authorized(token => saveClassPlanDraft(token, plan, preview!.digest)); setDraft(value); setDraftSaved(true); setMessage(t('crpDraftSaved')) })}>{t('crpSaveDraft')}</button><button type="button" className={ui.primary} disabled={busy || !preview?.can_publish || !draft || !draftSaved} onClick={() => void run(async () => { if (!await confirmAction(t('crpPublishConfirm', { n: preview?.existing.length ?? 0, version: draft?.version ?? 0 }))) return; const result = await authorized(token => publishClassPlan(token, sessionId, draft!.id, preview!.digest)); setDraft(null); setDraftSaved(false); setPreview(null); await client.invalidateQueries({ queryKey: ['routine'] }); setMessage(t('crpPublished', { version: result.version })) })}>{t('crpPublish')}</button></div>
      {draft && <p className="text-sm text-slate-600">{t('crpDraftVersion', { version: draft.version })}</p>}
      {preview && <><p className="text-sm font-medium">{t('crpExistingCount', { n: preview.existing.length, version: preview.published_version })}</p><div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{preview.periods.filter(p => p.kind === 'lunch').map(p => <div key={periodKey(p)} className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm"><strong>{t(`routineDay${p.day_of_week}`)} · {t('crpLunch')}</strong><span className="block">{p.start_time}–{p.end_time} · 45 {t('crpMinutes')}</span></div>)}</div><p className="text-sm">{t('crpFreeCount', { n: preview.free.length })}</p>{preview.issues.length === 0 ? <Notice tone="success">{t('crpNoIssues')}</Notice> : <div role="alert" className="space-y-1">{preview.issues.map((issue, i) => <p key={i} className="rounded-lg border border-rose-200 bg-rose-50 p-2 text-sm text-rose-800">{t(`crpIssue_${issue.code}`)}{issue.slot ? ` #${issue.slot}` : ''}<span className="block text-xs text-rose-700">{issue.message}</span></p>)}</div>}</>}
    </section>}
  </div>
}
