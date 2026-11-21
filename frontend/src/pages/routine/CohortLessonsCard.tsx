import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getCohortRoster, getOwnCohortLessons, saveCohortAttendance, type CohortRoster } from '../../api/client'
import { confirmAction } from '../../components/ConfirmDialog'
import { Notice, SectionCard, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'

type Status = 'present'|'absent'|'late'

export function CohortLessonsCard({ mode }: { mode: 'teacher'|'student' }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [selected, setSelected] = useState('')
  const [date, setDate] = useState(() => new Date().toLocaleDateString('sv-SE', { timeZone: 'Asia/Dhaka' }))
  const [edits, setEdits] = useState<Record<string, Status>>({})
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const lessons = useQuery({ queryKey: ['routine', 'cohorts', mode], queryFn: () => authorized(getOwnCohortLessons), retry: false })
  const roster = useQuery({ queryKey: ['routine', 'cohort-roster', selected, date], enabled: mode === 'teacher' && !!selected && !!date, queryFn: () => authorized(token => getCohortRoster(token, selected, date)), retry: false })
  const members = roster.data?.members ?? []
  const statuses = members.map(member => ({ student_id: member.student_id, status: edits[member.student_id] ?? member.status }))
  async function save() {
    if (!selected || !date || !members.length || statuses.some(entry => !entry.status) || !await confirmAction(t('cohortAttendanceConfirm'))) return
    setBusy(true); setMessage('')
    try {
      await authorized(token => saveCohortAttendance(token, selected, date, statuses as {student_id:string;status:Status}[]))
      setEdits({})
      await cache.invalidateQueries({ queryKey: ['routine', 'cohort-roster', selected, date] })
      setMessage(t('cohortAttendanceSaved'))
    } catch { setMessage(t('cohortAttendanceError')) }
    finally { setBusy(false) }
  }
  return <SectionCard title={t('cohortMyLessons')} id="cohort-own-lessons" icon="book">
    {lessons.isPending ? <Skeleton rows={2} /> : lessons.isError ? <Notice tone="error">{t('cohortLessonsError')}</Notice> : lessons.data.length === 0 ? <p className="text-sm text-slate-600">{t('cohortNoOwnLessons')}</p> : <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
      {lessons.data.map(lesson => <article key={lesson.id} className="rounded-xl border border-slate-200 p-3 text-sm">
        <strong>{t(`routineDay${lesson.day_of_week}`)} · {lesson.start_time}–{lesson.end_time}</strong>
        <p>{i18n.language === 'bn' && lesson.subject_name_bn ? lesson.subject_name_bn : lesson.subject_name} · {lesson.room}</p><p className="text-slate-600">{t('cohortCount', { count: lesson.member_count })}</p>
        {mode === 'teacher' && <button type="button" className={`${ui.secondary} mt-2`} onClick={() => { setSelected(lesson.id); setEdits({}); setMessage('') }}>{t('cohortAttendanceOpen')}</button>}
      </article>)}
    </div>}
    {mode === 'teacher' && selected && <div className="mt-4 border-t border-slate-200 pt-4">
      <label className={`${ui.label} block max-w-xs`}>{t('cohortAttendanceDate')}<input type="date" className={ui.field} value={date} onChange={e => { setDate(e.target.value); setEdits({}); setMessage('') }} /></label>
      {roster.isPending ? <Skeleton rows={2} /> : roster.isError ? <Notice tone="error">{t('cohortAttendanceDayError')}</Notice> : <CohortAttendanceRoster roster={roster.data} edits={edits} disabled={busy} onChange={(id, status) => setEdits(current => ({ ...current, [id]: status }))} />}
      {message && <p role="status" className="mt-2 text-sm">{message}</p>}
      {roster.data && <button type="button" className={`${ui.primary} mt-3`} disabled={busy || !members.length || statuses.some(entry => !entry.status)} onClick={() => void save()}>{t('cohortAttendanceSave')}</button>}
    </div>}
  </SectionCard>
}

function CohortAttendanceRoster({ roster, edits, disabled, onChange }: { roster: CohortRoster; edits: Record<string, Status>; disabled: boolean; onChange: (id: string, status: Status) => void }) {
  const { t } = useTranslation()
  return <div className="mt-3 max-h-96 space-y-2 overflow-auto">{roster.members.map(member => <label key={member.student_id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 p-2 text-sm">
    <span>{member.name} · {member.section_name}<span className="block text-xs text-slate-500">{member.admission_no}</span></span>
    <select className={`${ui.field} max-w-40`} aria-label={t('cohortAttendanceFor', { name: member.name })} disabled={disabled} value={edits[member.student_id] ?? member.status} onChange={e => onChange(member.student_id, e.target.value as Status)}>
      <option value="">{t('cohortAttendanceChoose')}</option><option value="present">{t('attendancePresent')}</option><option value="absent">{t('attendanceAbsent')}</option><option value="late">{t('cohortAttendanceLate')}</option>
    </select>
  </label>)}</div>
}
