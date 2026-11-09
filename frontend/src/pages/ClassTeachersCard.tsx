import { useEffect, useId, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getAcademicSessions, getEmployees } from '../api/client'
import { assignClassTeacher, getClassTeachers } from '../api/marksheetAdmit'
import { CardTitle, Notice, Skeleton, ui } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

// Class teachers (marksheet v2): one per section, set by the administrator.
// Others see who they are.
export function ClassTeachersCard({ canManage }: { canManage: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const uid = useId()
  const sessions = useQuery({ queryKey: ['ms', 'sessions'], queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const [sessionId, setSessionId] = useState('')
  useEffect(() => {
    if (!sessionId && sessions.data?.items.length) setSessionId((sessions.data.items.find(s => s.is_active) ?? sessions.data.items[0]).id)
  }, [sessions.data, sessionId])
  const rows = useQuery({ queryKey: ['ms', 'class-teachers', sessionId], enabled: !!sessionId, queryFn: () => authorized(token => getClassTeachers(token, sessionId)), retry: false })
  const teachers = useQuery({ queryKey: ['ms', 'teachers'], enabled: canManage, queryFn: () => authorized(token => getEmployees(token, new URLSearchParams({ page: '1', page_size: '100', kind: 'teacher', status: 'active' }))), retry: false })
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [busy, setBusy] = useState('')
  const bn = i18n.language === 'bn'
  async function assign(sectionId: string, employeeId: string) {
    setBusy(sectionId); setMessage(null)
    try {
      await authorized(token => assignClassTeacher(token, sectionId, employeeId))
      await client.invalidateQueries({ queryKey: ['ms', 'class-teachers'] })
      setMessage({ tone: 'success', text: t('msAssigned') })
    } catch { setMessage({ tone: 'error', text: t('msErrGeneric') }) } finally { setBusy('') }
  }
  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby={`${uid}-title`} data-testid="class-teachers">
    <CardTitle id={`${uid}-title`} icon="teacher">{t('msClassTeachersTitle')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('msClassTeachersIntro')}</p>
    <label className={`${ui.label} mt-4 block max-w-xs`} htmlFor={`${uid}-session`}>{t('msSession')}
      <select id={`${uid}-session`} className={ui.field} value={sessionId} onChange={e => setSessionId(e.target.value)}>
        {sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
      </select>
    </label>
    {message && <div className="mt-3"><Notice tone={message.tone}>{message.text}</Notice></div>}
    {rows.isPending ? <div className="mt-4"><Skeleton rows={3} /></div> : rows.isError ? <div className="mt-4"><Notice tone="error">{t('msLoadError')}</Notice></div> :
      <ul className="mt-4 divide-y divide-slate-100">{rows.data.map(r => <li key={r.section_id} className="grid gap-2 py-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,18rem)] sm:items-center">
        <div className="min-w-0">
          <p className="font-semibold text-slate-900">{r.class_name} · {r.section_name}</p>
          {r.employee_id && <p className="text-xs text-slate-600">{r.has_signature ? t('msSignatureReady') : t('msSignatureMissing')}</p>}
        </div>
        {canManage ? <select aria-label={t('msTeacherFor', { section: `${r.class_name} ${r.section_name}` })} className={ui.field} disabled={busy === r.section_id}
          value={r.employee_id ?? ''} onChange={e => void assign(r.section_id, e.target.value)}>
          <option value="">{t('msNoTeacher')}</option>
          {teachers.data?.items.map(e => <option key={e.id} value={e.id}>{bn && e.name_bn ? e.name_bn : e.name_en}</option>)}
          {r.employee_id && !teachers.data?.items.some(e => e.id === r.employee_id) && <option value={r.employee_id}>{r.teacher_name}</option>}
        </select> : <p className="text-sm text-slate-800">{r.employee_id ? (bn && r.teacher_name_bn ? r.teacher_name_bn : r.teacher_name) : t('msNoTeacher')}</p>}
      </li>)}</ul>}
  </section>
}
