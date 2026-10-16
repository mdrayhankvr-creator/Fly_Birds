import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getAcademicClasses, getAcademicSections, getAcademicSessions, saveAcademicSection, type AcademicSection, type AcademicSectionInput } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

const field = 'mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:border-teal-700 focus:outline-none'
const emptyForm: AcademicSectionInput = { session_id: '', class_id: '', name: '', code: '', display_order: 0, is_active: true }

export function SectionsPage({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const client = useQueryClient()
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [sessionFilter, setSessionFilter] = useState('')
  const [classFilter, setClassFilter] = useState('')
  const [sessionSearch, setSessionSearch] = useState('')
  const [classSearch, setClassSearch] = useState('')
  const [form, setForm] = useState<AcademicSectionInput>(emptyForm)
  const [editingId, setEditingId] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

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
  if (status) params.set('status', status)
  if (sessionFilter) params.set('session_id', sessionFilter)
  if (classFilter) params.set('class_id', classFilter)
  const listing = useQuery({ queryKey: ['academic', 'sections', params.toString()], queryFn: () => authorized((token) => getAcademicSections(token, params)), retry: false })
  const sessions = useQuery({ queryKey: ['academic', 'section-sessions', sessionSearch], queryFn: () => authorized((token) => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100', search: sessionSearch }))), retry: false })
  const classes = useQuery({ queryKey: ['academic', 'section-classes', classSearch], queryFn: () => authorized((token) => getAcademicClasses(token, new URLSearchParams({ page: '1', page_size: '100', search: classSearch }))), retry: false })
  const pages = Math.max(1, Math.ceil((listing.data?.total ?? 0) / 10))
  function edit(section: AcademicSection) {
    setEditingId(section.id)
    setForm({ session_id: section.session_id, class_id: section.class_id, name: section.name, code: section.code, display_order: section.display_order, is_active: section.is_active })
    setError(''); setMessage('')
  }
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(''); setMessage('')
    try {
      await authorized((token) => saveAcademicSection(token, form, editingId || undefined))
      setEditingId(''); setForm(emptyForm)
      await client.invalidateQueries({ queryKey: ['academic', 'sections'] })
      setMessage(t('academicSaved'))
    } catch (cause) {
      setError(cause instanceof ApiError && cause.code === 'duplicate_academic_record' ? t('sectionDuplicate') : t('sectionSaveError'))
    } finally { setBusy(false) }
  }

  return <div className="space-y-6">
    <header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
      <p className="text-sm font-bold uppercase tracking-widest text-teal-700">{t('academicStructure')}</p>
      <h1 className="mt-2 text-3xl font-bold text-slate-900">{t('sectionsHeading')}</h1>
      <p className="mt-2 text-slate-600">{t('sectionsDescription')}</p>
      <nav className="mt-5 flex flex-wrap gap-2" aria-label={t('academicStructure')}>
        <Link to="/admin/academic/sessions" className="rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800">{t('academicSessions')}</Link>
        <Link to="/admin/academic/classes" className="rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800">{t('classesHeading')}</Link>
        <Link to="/admin/academic/subjects" className="rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800">{t('subjectsHeading')}</Link>
      </nav>
    </header>
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}
    {message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-800">{message}</p>}
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(17rem,1fr)]">
      <section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" aria-label={t('academicDirectory')}>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="text-sm font-semibold">{t('search')}<input className={field} value={search} onChange={(e) => { setSearch(e.target.value); setPage(1) }} /></label>
          <label className="text-sm font-semibold">{t('status')}<select className={field} value={status} onChange={(e) => { setStatus(e.target.value); setPage(1) }}><option value="">{t('allStatuses')}</option><option value="active">{t('active')}</option><option value="inactive">{t('inactive')}</option></select></label>
          <label className="text-sm font-semibold">{t('sectionSession')}<select className={field} value={sessionFilter} onChange={(e) => { setSessionFilter(e.target.value); setPage(1) }}><option value="">{t('sectionAllSessions')}</option>{sessions.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
          <label className="text-sm font-semibold">{t('sectionClass')}<select className={field} value={classFilter} onChange={(e) => { setClassFilter(e.target.value); setPage(1) }}><option value="">{t('sectionAllClasses')}</option>{classes.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
        </div>
        {listing.isPending ? <p role="status" className="mt-4">{t('academicLoading')}</p> : listing.isError ? <p role="alert" className="mt-4 text-rose-700">{t('academicLoadError')}</p> : listing.data.items.length === 0 ? <p className="mt-4 text-slate-600">{t('academicEmpty')}</p> : <ul className="mt-4 space-y-2">{listing.data.items.map((item) => <li key={item.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 p-3"><div className="min-w-0"><p className="font-semibold text-slate-900">{item.name} <span className="font-normal text-slate-500">({item.code})</span></p><p className="text-sm text-slate-600">{item.session_name} · {item.class_name} · {t('displayOrder')}: {item.display_order} · {t(item.is_active ? 'active' : 'inactive')}</p></div>{canManage && <button type="button" className="rounded-lg px-3 py-2 font-semibold text-teal-800 hover:bg-teal-50" onClick={() => edit(item)}>{t('edit')}</button>}</li>)}</ul>}
        <div className="mt-5 flex items-center justify-between gap-3 text-sm"><button type="button" className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('previous')}</button><span>{t('pageOf', { page, total: pages })}</span><button type="button" className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page >= pages} onClick={() => setPage(page + 1)}>{t('next')}</button></div>
      </section>
      {canManage && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" aria-label={t('academicEditor')}>
        <h2 className="text-xl font-bold">{t(editingId ? 'edit' : 'sectionCreate')}</h2>
        <form className="mt-4 space-y-4" onSubmit={(event) => void save(event)}>
          <label className="block text-sm font-semibold">{t('sectionSession')}{!editingId && <input className={field} placeholder={t('search')} value={sessionSearch} onChange={(e) => setSessionSearch(e.target.value)} />}<select required disabled={!!editingId} className={field} value={form.session_id} onChange={(e) => setForm({ ...form, session_id: e.target.value })}><option value="">{t('sectionChooseSession')}</option>{sessions.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}{editingId && form.session_id && !sessions.data?.items.some((item) => item.id === form.session_id) && <option value={form.session_id}>{form.session_id}</option>}</select></label>
          <label className="block text-sm font-semibold">{t('sectionClass')}{!editingId && <input className={field} placeholder={t('search')} value={classSearch} onChange={(e) => setClassSearch(e.target.value)} />}<select required disabled={!!editingId} className={field} value={form.class_id} onChange={(e) => setForm({ ...form, class_id: e.target.value })}><option value="">{t('sectionChooseClass')}</option>{classes.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}{editingId && form.class_id && !classes.data?.items.some((item) => item.id === form.class_id) && <option value={form.class_id}>{form.class_id}</option>}</select></label>
          {editingId && <p className="text-sm text-slate-600">{t('sectionContextFixed')}</p>}
          <label className="block text-sm font-semibold">{t('sectionName')}<input required maxLength={120} className={field} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
          <label className="block text-sm font-semibold">{t('sectionCode')}<input required maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9_-]*" className={field} value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} /></label>
          <label className="block text-sm font-semibold">{t('displayOrder')}<input required type="number" min="0" max="100000" className={field} value={form.display_order} onChange={(e) => setForm({ ...form, display_order: Number(e.target.value) })} /></label>
          <label className="flex items-center gap-2 text-sm font-semibold"><input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} />{t('active')}</label>
          <div className="flex flex-wrap gap-2"><button disabled={busy} className="rounded-lg bg-teal-700 px-4 py-2 font-semibold text-white disabled:opacity-50">{t('saveChanges')}</button>{editingId && <button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setEditingId(''); setForm(emptyForm) }}>{t('cancel')}</button>}</div>
        </form>
        {(sessions.data && sessions.data.total > 100 || classes.data && classes.data.total > 100) && <p className="mt-3 text-sm text-amber-700">{t('sectionOptionsLimit')}</p>}
      </section>}
    </div>
  </div>
}
