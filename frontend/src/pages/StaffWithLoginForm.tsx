import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, getAcademicClasses, getAcademicSessions, getClassSubjects } from '../api/client'
import { createStaffWithLogin, type CreatedStaffWithLogin, type CreateStaffWithLoginInput } from '../api/staffProfile'
import { confirmAction } from '../components/ConfirmDialog'
import { Icon } from '../components/icons'
import { usePeopleApi } from './peopleShared'
import { PasswordSlip } from './PasswordSlip'
import { CardTitle, Notice, ui } from './attendanceUi'

const all = new URLSearchParams({ page: '1', page_size: '100' })

export function StaffWithLoginForm({ onCreated, onClose }: { onCreated: (id: string) => void; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [form, setForm] = useState({ employee_code: '', confirm_employee_code: '', role: 'teacher' as 'teacher' | 'head_teacher', email: '', name_en: '', name_bn: '', designation: '', joined_on: '' })
  const [qualifications, setQualifications] = useState('')
  const [sessionId, setSessionId] = useState('')
  const [classId, setClassId] = useState('')
  const [subjectId, setSubjectId] = useState('')
  const [photo, setPhoto] = useState<File | null>(null)
  const [preview, setPreview] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [created, setCreated] = useState<CreatedStaffWithLogin | null>(null)
  const sessions = useQuery({ queryKey: ['staff-with-login', 'sessions'], queryFn: () => authorized(token => getAcademicSessions(token, all)), retry: false })
  const classes = useQuery({ queryKey: ['staff-with-login', 'classes'], queryFn: () => authorized(token => getAcademicClasses(token, all)), retry: false })
  const subjects = useQuery({ queryKey: ['staff-with-login', 'subjects', classId], enabled: !!classId, queryFn: () => authorized(token => getClassSubjects(token, classId, all)), retry: false })

  useEffect(() => {
    if (!photo) { setPreview(''); return }
    const url = URL.createObjectURL(photo)
    setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [photo])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    setError('')
    const code = form.employee_code.trim().toUpperCase()
    const confirmation = form.confirm_employee_code.trim().toUpperCase()
    const items = qualifications.split(/\r?\n/).map(value => value.trim()).filter(Boolean)
    if (!/^[A-Z0-9][A-Z0-9_-]{1,31}$/.test(code) || code !== confirmation || items.length > 20 || items.some(value => new TextEncoder().encode(value).length > 160) ||
      (photo && (!['image/jpeg', 'image/png'].includes(photo.type) || photo.size > 6 * 1024 * 1024)) ||
      (!!(sessionId || classId || subjectId) && !(sessionId && classId && subjectId))) {
      setError(t('staffWithLoginInvalid')); return
    }
    const input: CreateStaffWithLoginInput = { ...form, employee_code: code, confirm_employee_code: confirmation, email: form.email.trim(), qualifications: items }
    if (sessionId && classId && subjectId) input.assignment = { session_id: sessionId, class_id: classId, subject_id: subjectId }
    const confirmed = await confirmAction(t('staffWithLoginConfirm', { name: form.name_en.trim(), code, email: input.email, role: t(`role_${form.role}`) }), { tone: 'default', confirmLabel: t('staffWithLoginCreate') })
    if (!confirmed) return
    setBusy(true)
    try {
      const result = await authorized(token => createStaffWithLogin(token, input, photo ?? undefined))
      setCreated(result)
      await cache.invalidateQueries({ queryKey: ['staff'] })
      await cache.invalidateQueries({ queryKey: ['users'] })
      onCreated(result.employee.id)
    } catch (cause) {
      setError(t(cause instanceof ApiError && cause.status === 409 ? 'staffWithLoginDuplicate' : cause instanceof ApiError && cause.status === 400 ? 'staffWithLoginInvalid' : cause instanceof ApiError && cause.status === 403 ? 'permissionDenied' : 'staffWithLoginFailed'))
    } finally { setBusy(false) }
  }

  if (created) return <section className={`${ui.card} min-w-0 space-y-4 p-4 sm:p-6`} aria-labelledby="staff-created-title">
    <CardTitle id="staff-created-title" icon="check">{t('staffWithLoginSuccess')}</CardTitle>
    <Notice tone="success">{t('staffWithLoginOnce')}</Notice>
    <PasswordSlip name={created.employee.name_en} email={created.email} password={created.initial_password} expiresAt={created.expires_at} />
    <div className="flex flex-wrap gap-2 print:hidden"><button type="button" className={ui.secondary} onClick={() => window.print()}><Icon name="print" className="h-4 w-4" />{t('c4PrintSlip')}</button><button type="button" className={ui.primary} onClick={onClose}>{t('staffWithLoginDone')}</button></div>
  </section>

  return <section className={`${ui.card} min-w-0 p-4 sm:p-6`} aria-labelledby="staff-with-login-title">
    <div className="flex flex-wrap items-start justify-between gap-2"><CardTitle id="staff-with-login-title" icon="plus">{t('staffWithLoginTitle')}</CardTitle><button type="button" className={ui.secondary} disabled={busy} onClick={onClose}>{t('staffCancel')}</button></div>
    <p className="mt-2 text-sm leading-6 text-slate-600">{t('staffWithLoginIntro')}</p>
    {error && <div className="mt-3"><Notice tone="error">{error}</Notice></div>}
    <form className="mt-4 grid min-w-0 gap-4 sm:grid-cols-2" onSubmit={event => void submit(event)}>
      <label className={ui.label}>{t('staffNameEN')}<input name="name_en" className={ui.field} required maxLength={160} value={form.name_en} onChange={event => setForm({ ...form, name_en: event.target.value })} /></label>
      <label className={ui.label}>{t('staffNameBN')}<input name="name_bn" className={ui.field} maxLength={160} value={form.name_bn} onChange={event => setForm({ ...form, name_bn: event.target.value })} /></label>
      <label className={ui.label}>{t('staffCode')}<input name="employee_code" className={ui.field} required maxLength={32} value={form.employee_code} onChange={event => setForm({ ...form, employee_code: event.target.value })} /></label>
      <label className={ui.label}>{t('staffAccountCodeConfirm')}<input name="confirm_employee_code" className={ui.field} required maxLength={32} value={form.confirm_employee_code} onChange={event => setForm({ ...form, confirm_employee_code: event.target.value })} /></label>
      <label className={ui.label}>{t('staffDesignation')}<input name="designation" className={ui.field} required maxLength={120} value={form.designation} onChange={event => setForm({ ...form, designation: event.target.value })} /></label>
      <label className={ui.label}>{t('staffJoined')}<input name="joined_on" className={ui.field} type="date" required value={form.joined_on} onChange={event => setForm({ ...form, joined_on: event.target.value })} /></label>
      <label className={ui.label}>{t('staffAccountRole')}<select name="role" className={ui.field} value={form.role} onChange={event => setForm({ ...form, role: event.target.value as typeof form.role })}><option value="teacher">{t('role_teacher')}</option><option value="head_teacher">{t('role_head_teacher')}</option></select></label>
      <label className={ui.label}>{t('staffAccountEmail')}<input name="email" className={ui.field} type="email" required maxLength={254} autoComplete="off" value={form.email} onChange={event => setForm({ ...form, email: event.target.value })} /></label>
      <label className={`${ui.label} sm:col-span-2`}>{t('staffWithLoginQualifications')}<textarea className={`${ui.field} min-h-20`} maxLength={3200} value={qualifications} onChange={event => setQualifications(event.target.value)} placeholder={t('staffWithLoginQualificationsHint')} /></label>
      <fieldset className="min-w-0 rounded-xl border border-slate-200 p-3 sm:col-span-2"><legend className="px-1 text-sm font-bold text-slate-800">{t('staffWithLoginAssignment')}</legend><p className="mb-3 text-xs text-slate-600">{t('staffWithLoginAssignmentHint')}</p>
        <div className="grid gap-3 sm:grid-cols-3"><label className={ui.label}>{t('staffSession')}<select name="session_id" className={ui.field} value={sessionId} onChange={event => setSessionId(event.target.value)}><option value="">{t('staffWithLoginOptional')}</option>{sessions.data?.items.filter(item => item.is_active).map(item => <option value={item.id} key={item.id}>{item.name}</option>)}</select></label>
          <label className={ui.label}>{t('staffClass')}<select name="class_id" className={ui.field} value={classId} onChange={event => { setClassId(event.target.value); setSubjectId('') }}><option value="">{t('staffWithLoginOptional')}</option>{classes.data?.items.filter(item => item.is_active).map(item => <option value={item.id} key={item.id}>{item.name}</option>)}</select></label>
          <label className={ui.label}>{t('staffSubject')}<select name="subject_id" className={ui.field} value={subjectId} disabled={!classId || subjects.isPending} onChange={event => setSubjectId(event.target.value)}><option value="">{t('staffWithLoginOptional')}</option>{subjects.data?.items.map(item => <option value={item.subject_id} key={item.subject_id}>{i18n.language === 'bn' && item.subject_name_bn ? item.subject_name_bn : item.subject_name_en}</option>)}</select></label></div>
        {(sessions.isError || classes.isError || subjects.isError) && <p role="alert" className="mt-2 text-sm text-rose-700">{t('staffLoadError')}</p>}
      </fieldset>
      <label className={`${ui.label} sm:col-span-2`}>{t('staffWithLoginPhoto')}<input className={ui.field} type="file" accept="image/jpeg,image/png" onChange={event => setPhoto(event.target.files?.[0] ?? null)} /><span className="mt-1 block text-xs font-normal text-slate-600">{t('staffWithLoginPhotoHint')}</span></label>
      {preview && <img src={preview} alt={t('staffWithLoginPhotoPreview')} className="h-24 w-24 rounded-xl object-cover sm:col-span-2" />}
      <button type="submit" className={`${ui.primary} w-fit sm:col-span-2`} disabled={busy}>{busy ? t('staffWithLoginWorking') : t('staffWithLoginCreate')}</button>
    </form>
  </section>
}
