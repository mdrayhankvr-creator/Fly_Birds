import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getStudents, type ClassPlan, type ClassPlanPeriod, type ClassPlanPreview, type CombinedClassInput, type Student } from '../../api/client'
import { Notice, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'

const blank = (): CombinedClassInput => ({ class_id: '', subject_id: '', teacher_id: '', day_of_week: 0, start_time: '', end_time: '', room: '', student_ids: [] })
const key = (period: ClassPlanPeriod) => `${period.day_of_week}|${period.start_time}|${period.end_time}`

export function CombinedLessonsPanel({ sessionId, plan, catalogue, periods, viewDay, viewTeacher, viewSection, onChange }: {
  sessionId: string
  plan: ClassPlan
  catalogue: ClassPlanPreview['catalogue'] | null
  periods: ClassPlanPeriod[]
  viewDay: number | null
  viewTeacher: string
  viewSection: string
  onChange: (update: (current: ClassPlan) => ClassPlan) => void
}) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const [entry, setEntry] = useState<CombinedClassInput>(blank)
  const [editing, setEditing] = useState<number | null>(null)
  useEffect(() => { setEntry(blank()); setEditing(null) }, [sessionId])
  const students = useQuery({ queryKey: ['cohort', 'students', sessionId], enabled: !!sessionId, retry: false, queryFn: () => authorized(async token => {
    const all: Student[] = []
    for (let page = 1; page <= 5; page++) {
      const result = await getStudents(token, new URLSearchParams({ session_id: sessionId, status: 'active', page: String(page), page_size: '100' }))
      all.push(...result.items)
      if (all.length >= result.total) return all
    }
    throw new Error('cohort roster exceeds 500 students')
  }) })
  const classOptions = Array.from(new Map((catalogue?.sections ?? []).map(section => [section.class_id, section.class_name])).entries())
  const subjects = catalogue?.subjects.filter(subject => subject.class_id === entry.class_id) ?? []
  const teachers = catalogue?.assignments.filter(assignment => assignment.class_id === entry.class_id && assignment.subject_id === entry.subject_id) ?? []
  const candidates = students.data?.filter(student => student.class_id === entry.class_id) ?? []
  const memberSection = new Map(students.data?.map(student => [student.id, student.section_id]) ?? [])
  const visible = (plan.combined_slots ?? []).map((value, index) => ({ value, index })).filter(({ value }) =>
    (viewDay === null || value.day_of_week === viewDay) && (!viewTeacher || value.teacher_id === viewTeacher) &&
    (!viewSection || value.student_ids.some(id => memberSection.get(id) === viewSection)))
  const valid = !!entry.class_id && !!entry.subject_id && !!entry.teacher_id && !!entry.start_time && !!entry.room && entry.student_ids.length > 0

  function save() {
    if (!valid) return
    onChange(current => ({ ...current, combined_slots: editing === null ? [...(current.combined_slots ?? []), entry]
      : (current.combined_slots ?? []).map((value, index) => index === editing ? entry : value) }))
    setEntry(blank()); setEditing(null)
  }

  return <section className={`${ui.card} space-y-4 p-4 sm:p-6`} data-testid="combined-lessons">
    <div><h3 className="text-lg font-semibold">{t('cohortTitle')}</h3><p className="text-sm text-slate-600">{t('cohortIntro')}</p></div>
    {!catalogue && <Notice tone="info">{t('crpPreviewFirst')}</Notice>}
    {catalogue && <>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        <label className={ui.label}>{t('cohortClass')}<select className={ui.field} value={entry.class_id} onChange={e => setEntry({ ...blank(), class_id: e.target.value })}><option value="">{t('cohortClass')}</option>{classOptions.map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></label>
        <label className={ui.label}>{t('subjectsHeading')}<select className={ui.field} value={entry.subject_id} onChange={e => setEntry({ ...entry, subject_id: e.target.value, teacher_id: '' })}><option value="">{t('subjectsHeading')}</option>{subjects.map(subject => <option key={subject.id} value={subject.id}>{i18n.language === 'bn' ? subject.name_bn : subject.name}</option>)}</select></label>
        <label className={ui.label}>{t('routineTeacher')}<select className={ui.field} value={entry.teacher_id} onChange={e => setEntry({ ...entry, teacher_id: e.target.value })}><option value="">{t('routineTeacher')}</option>{teachers.map(teacher => <option key={teacher.teacher_id} value={teacher.teacher_id}>{teacher.teacher_name}</option>)}</select></label>
        <label className={ui.label}>{t('routineTime')}<select className={ui.field} value={`${entry.day_of_week}|${entry.start_time}|${entry.end_time}`} onChange={e => { const period = periods.find(p => key(p) === e.target.value); if (period) setEntry({ ...entry, day_of_week: period.day_of_week, start_time: period.start_time, end_time: period.end_time }) }}><option value="0||">{t('crpChoosePeriod')}</option>{periods.filter(p => p.kind === 'lesson').map(p => <option key={key(p)} value={key(p)}>{t(`routineDay${p.day_of_week}`)} · {p.start_time}–{p.end_time}</option>)}</select></label>
        <label className={ui.label}>{t('routineRoom')}<select className={ui.field} value={entry.room} onChange={e => setEntry({ ...entry, room: e.target.value })}><option value="">{t('routineRoom')}</option>{catalogue.rooms.map(room => <option key={room} value={room}>{room}</option>)}</select></label>
      </div>
      <fieldset className="rounded-xl border border-slate-200 p-3"><legend className="font-medium">{t('cohortStudents')} · {t('cohortCount', { count: entry.student_ids.length })}</legend>
        {students.isPending ? <p className="text-sm">{t('cohortRosterLoading')}</p> : students.isError ? <Notice tone="error">{t('cohortRosterError')}</Notice> : candidates.length === 0 ? <p className="text-sm text-slate-600">{t('religionNoStudents')}</p>
          : <div className="grid max-h-60 gap-1 overflow-auto sm:grid-cols-2 xl:grid-cols-3">{candidates.map(student => <label key={student.id} className="flex items-center gap-2 rounded-lg px-2 py-1 text-sm hover:bg-slate-50"><input type="checkbox" checked={entry.student_ids.includes(student.id)} onChange={e => setEntry(current => ({ ...current, student_ids: e.target.checked ? [...current.student_ids, student.id] : current.student_ids.filter(id => id !== student.id) }))} /><span>{student.name_en}<small className="block text-slate-500">{student.section_name} · {student.admission_no}</small></span></label>)}</div>}
      </fieldset>
      <button type="button" className={ui.secondary} disabled={!valid || students.isError} onClick={save}>{t(editing === null ? 'cohortAdd' : 'saveChanges')}</button>
      {!valid && <p className="text-xs text-slate-600">{t('cohortNoChoice')}</p>}
      <p className="text-xs text-slate-600">{t('cohortPreviewHint')}</p>
    </>}
    {visible.length === 0 ? <Notice tone="info">{t('cohortEmpty')}</Notice> : <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{visible.map(({ value, index }) => <article key={index} className="rounded-xl border border-slate-200 p-3 text-sm">
      <strong>{t(`routineDay${value.day_of_week}`)} · {value.start_time}–{value.end_time}</strong>
      <p>{classOptions.find(([id]) => id === value.class_id)?.[1]} · {catalogue?.subjects.find(s => s.id === value.subject_id)?.name}</p>
      <p>{catalogue?.assignments.find(a => a.teacher_id === value.teacher_id)?.teacher_name} · {value.room} · {t('cohortCount', { count: value.student_ids.length })}</p>
      <div className="mt-2 flex gap-2"><button type="button" className={ui.ghost} onClick={() => { setEntry(value); setEditing(index) }}>{t('edit')}</button><button type="button" className={ui.ghost} onClick={() => onChange(current => ({ ...current, combined_slots: (current.combined_slots ?? []).filter((_, i) => i !== index) }))}>{t('cohortRemove')}</button></div>
    </article>)}</div>}
  </section>
}
