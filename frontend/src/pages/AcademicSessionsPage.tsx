import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ApiError, createAcademicSession, getAcademicSessions, getCurrentAcademicSession,
  setAcademicSessionActive, updateAcademicSession,
  type AcademicSession, type AcademicSessionInput,
} from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { ui, PageHeader, LinkTabs } from './attendanceUi'

const emptyForm: AcademicSessionInput = { name: '', start_date: '', end_date: '' }
const fieldClass = ui.field

function validDate(value: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const date = new Date(`${value}T00:00:00Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
}
function validForm(input: AcademicSessionInput) {
  const name = input.name.trim()
  const match = /^(\d{4})(?:[-–](\d{4}))?$/.exec(name)
  if (!match || Number(match[1]) < 1900 || Number(match[1]) > 2200 || (match[2] && Number(match[2]) !== Number(match[1]) + 1)) return false
  return validDate(input.start_date) && validDate(input.end_date) && input.end_date > input.start_date
}

export function AcademicSessionsPage({ permissions }: { permissions: string[] }) {
  const { t } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<AcademicSession | null>(null)
  const [form, setForm] = useState<AcademicSessionInput>(emptyForm)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const canCreate = permissions.includes('academic_sessions:create')
  const canUpdate = permissions.includes('academic_sessions:update')
  const canActivate = permissions.includes('academic_sessions:activate')

  async function authorized<T>(request: (token: string) => Promise<T>): Promise<T> {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await request(accessToken) }
    catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await request(await renew()) }
        catch (retryCause) { if (retryCause instanceof ApiError && retryCause.status === 401) expire(); throw retryCause }
      }
      throw cause
    }
  }
  const params = new URLSearchParams({ page: String(page), page_size: '10', search })
  const sessions = useQuery({ queryKey: ['academic-sessions', params.toString()], queryFn: () => authorized((token) => getAcademicSessions(token, params)), retry: false })
  const current = useQuery({
    queryKey: ['academic-session-current'], retry: false,
    queryFn: () => authorized((token) => getCurrentAcademicSession(token)).catch((cause) => {
      if (cause instanceof ApiError && cause.status === 404) return null
      throw cause
    }),
  })

  async function run(action: () => Promise<unknown>) {
    setBusy(true); setError(''); setMessage('')
    try {
      await action()
      setMessage(t('sessionSaved'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['academic-sessions'] }),
        queryClient.invalidateQueries({ queryKey: ['academic-session-current'] }),
      ])
    } catch (cause) {
      setError(cause instanceof ApiError && cause.code === 'duplicate_session_name' ? t('duplicateSession') :
        cause instanceof ApiError && cause.code === 'module_disabled' ? t('academicModuleDisabled') : t('sessionSaveError'))
    } finally { setBusy(false) }
  }
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!validForm(form)) { setError(t('invalidSession')); return }
    const input = { ...form, name: form.name.trim() }
    void run(async () => {
      if (editing) await authorized((token) => updateAcademicSession(token, editing.id, input))
      else await authorized((token) => createAcademicSession(token, input))
      setEditing(null); setForm(emptyForm)
    })
  }
  function edit(session: AcademicSession) {
    setEditing(session)
    setForm({ name: session.name, start_date: session.start_date, end_date: session.end_date })
    setError(''); setMessage('')
  }
  async function activation(session: AcademicSession) {
    if (!(await confirmAction(t(session.is_active ? 'confirmSessionDeactivate' : 'confirmSessionActivate', { name: session.name })))) return
    void run(() => authorized((token) => setAcademicSessionActive(token, session.id, !session.is_active)))
  }

  return <div className="space-y-8">
    <PageHeader eyebrow={t('academicStructure')} title={t('academicSessions')} description={t('academicSessionsDescription')} />
    <LinkTabs label={t('academicStructure')} items={[{ to: '/admin/academic/sessions', label: t('academicSessions') }, { to: '/admin/academic/classes', label: t('classesHeading') }, { to: '/admin/academic/sections', label: t('sectionsHeading') }, { to: '/admin/academic/subjects', label: t('subjectsHeading') }]} />
    {message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-900">{message}</p>}
    {error && <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900">{error}</p>}
    <section className={`${ui.card} p-4 sm:p-6`} aria-label={t('currentSession')}>
      <h2 className="text-lg font-semibold text-slate-900">{t('currentSession')}</h2>
      {current.isPending ? <p role="status" className="mt-2 text-slate-600">{t('loadingSessions')}</p> :
        current.isError ? <p role="alert" className="mt-2 text-rose-700">{current.error instanceof ApiError && current.error.code === 'module_disabled' ? t('academicModuleDisabled') : t('loadSessionsError')}</p> :
          <p className="mt-2 text-slate-700">{current.data?.name ?? t('noCurrentSession')}</p>}
    </section>
    {(canCreate || (canUpdate && editing)) && <form onSubmit={submit} className="grid gap-4 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:grid-cols-3">
      <h2 className="text-xl font-semibold text-slate-900 sm:col-span-3">{t(editing ? 'editSession' : 'createSession')}</h2>
      <label className={ui.label}>{t('sessionName')}<input className={fieldClass} required maxLength={9} placeholder="2026–2027" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
      <label className={ui.label}>{t('startDate')}<input className={fieldClass} type="date" required value={form.start_date} onChange={(e) => setForm({ ...form, start_date: e.target.value })} /></label>
      <label className={ui.label}>{t('endDate')}<input className={fieldClass} type="date" required min={form.start_date} value={form.end_date} onChange={(e) => setForm({ ...form, end_date: e.target.value })} /></label>
      <p className="text-sm text-slate-600 sm:col-span-3">{t('sessionNameHint')}</p>
      <div className="flex flex-wrap gap-2 sm:col-span-3"><button disabled={busy} className="brand-bg rounded-lg px-4 py-2 font-semibold text-white disabled:opacity-50">{t(editing ? 'saveChanges' : 'createSession')}</button>{editing && <button type="button" onClick={() => { setEditing(null); setForm(emptyForm) }} className="rounded-lg border px-4 py-2 font-semibold">{t('cancel')}</button>}</div>
    </form>}
    <section className={`${ui.card} p-4 sm:p-6`}>
      <h2 className={ui.sectionTitle}>{t('sessionList')}</h2>
      <label className="mt-4 block max-w-sm text-sm font-semibold text-slate-700">{t('search')}<input className={fieldClass} value={search} maxLength={100} onChange={(e) => { setSearch(e.target.value); setPage(1) }} /></label>
      {sessions.isPending ? <p role="status" className="mt-5">{t('loadingSessions')}</p> :
        sessions.isError ? <p role="alert" className="mt-5 text-rose-700">{sessions.error instanceof ApiError && sessions.error.code === 'module_disabled' ? t('academicModuleDisabled') : t('loadSessionsError')}</p> :
          sessions.data.items.length === 0 ? <p className="mt-5 text-slate-600">{t('noSessions')}</p> :
            <div className="mt-5 grid gap-3 md:grid-cols-2">{sessions.data.items.map((session) => <article key={session.id} className="min-w-0 rounded-xl border border-slate-200 p-4">
              <div className="flex flex-wrap items-center gap-2"><h3 className="text-lg font-bold text-slate-900">{session.name}</h3>{session.is_active && <span className="rounded-full bg-emerald-100 px-2 py-1 text-xs font-semibold text-emerald-800">{t('active')}</span>}</div>
              <p className="mt-2 text-sm text-slate-600">{session.start_date} → {session.end_date}</p>
              <div className="mt-4 flex flex-wrap gap-2">{canUpdate && <button type="button" onClick={() => edit(session)} className="rounded-lg border px-3 py-2 text-sm font-semibold">{t('editSession')}</button>}{canActivate && <button type="button" disabled={busy} onClick={() => activation(session)} className="rounded-lg border px-3 py-2 text-sm font-semibold disabled:opacity-50">{t(session.is_active ? 'deactivate' : 'activate')}</button>}</div>
            </article>)}</div>}
      {(sessions.data?.total || 0) > 10 && <div className="mt-5 border-t border-slate-100 pt-4 flex flex-wrap items-center justify-between gap-2 text-sm"><button type="button" disabled={page <= 1 || sessions.isPending} onClick={() => setPage(page - 1)} className={ui.secondary}>{t('previous')}</button><span>{t('pageOf', { page, total: Math.max(1, Math.ceil((sessions.data?.total || 0) / 10)) })}</span><button type="button" disabled={sessions.isPending || !sessions.data || page * 10 >= sessions.data.total} onClick={() => setPage(page + 1)} className={ui.secondary}>{t('next')}</button></div>}
    </section>
  </div>
}
