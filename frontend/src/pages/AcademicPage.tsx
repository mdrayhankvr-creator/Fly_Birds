import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, addClassSubject, getAcademicClasses, getAcademicSubjects, getClassSubjects, removeClassSubject, saveAcademicClass, saveAcademicSubject, updateClassSubject, type AcademicClass, type AcademicClassInput, type AcademicSubject, type AcademicSubjectInput, type Page } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

const field = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:border-[var(--brand-primary)] focus:outline-none'
const emptyClass: AcademicClassInput = { name: '', display_order: 0, is_active: true }
const emptySubject: AcademicSubjectInput = { name_en: '', name_bn: '', code: '', is_active: true }

export function AcademicPage({ kind, canManage }: { kind: 'classes' | 'subjects'; canManage: boolean }) {
  const { t, i18n } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const client = useQueryClient()
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [editingId, setEditingId] = useState('')
  const [classForm, setClassForm] = useState<AcademicClassInput>(emptyClass)
  const [subjectForm, setSubjectForm] = useState<AcademicSubjectInput>(emptySubject)
  const [selectedClass, setSelectedClass] = useState<AcademicClass | null>(null)
  const [subjectId, setSubjectId] = useState('')
  const [subjectSearch, setSubjectSearch] = useState('')
  const [fourth, setFourth] = useState(false)
  const [mappingPage, setMappingPage] = useState(1)
  const [mappingSearch, setMappingSearch] = useState('')
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
  const listing = useQuery<Page<AcademicClass> | Page<AcademicSubject>>({ queryKey: ['academic', kind, params.toString()], queryFn: () => kind === 'classes' ? authorized((token) => getAcademicClasses(token, params)) : authorized((token) => getAcademicSubjects(token, params)), retry: false })
  const allSubjects = useQuery({ queryKey: ['academic', 'subject-options', subjectSearch], enabled: kind === 'classes' && !!selectedClass, queryFn: () => authorized((token) => getAcademicSubjects(token, new URLSearchParams({ page: '1', page_size: '100', status: 'active', search: subjectSearch }))), retry: false })
  const mappingParams = new URLSearchParams({ page: String(mappingPage), page_size: '10', search: mappingSearch })
  const mappings = useQuery({ queryKey: ['academic', 'mappings', selectedClass?.id, mappingParams.toString()], enabled: kind === 'classes' && !!selectedClass, queryFn: () => authorized((token) => getClassSubjects(token, selectedClass!.id, mappingParams)), retry: false })

  async function run(action: () => Promise<unknown>) {
    setBusy(true); setError(''); setMessage('')
    try {
      await action()
      await client.invalidateQueries({ queryKey: ['academic'] })
      setMessage(t('academicSaved'))
    } catch (cause) {
      setError(cause instanceof ApiError && cause.code === 'duplicate_academic_record' ? t('academicDuplicate') : cause instanceof ApiError && cause.code === 'inactive_academic_record' ? t('academicInactive') : t('academicSaveError'))
    } finally { setBusy(false) }
  }
  function selectClass(item: AcademicClass) { setSelectedClass(item); setMappingPage(1); setMappingSearch(''); setSubjectId('') }
  function editClass(item: AcademicClass) { setEditingId(item.id); setClassForm({ name: item.name, display_order: item.display_order, is_active: item.is_active }); selectClass(item) }
  function editSubject(item: AcademicSubject) { setEditingId(item.id); setSubjectForm({ name_en: item.name_en, name_bn: item.name_bn, code: item.code, is_active: item.is_active }) }
  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (kind === 'classes') void run(async () => { await authorized((token) => saveAcademicClass(token, classForm, editingId || undefined)); setEditingId(''); setClassForm(emptyClass) })
    else void run(async () => { await authorized((token) => saveAcademicSubject(token, subjectForm, editingId || undefined)); setEditingId(''); setSubjectForm(emptySubject) })
  }
  function addMapping(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedClass || !subjectId) return
    void run(async () => { await authorized((token) => addClassSubject(token, selectedClass.id, subjectId, fourth)); setSubjectId('') })
  }
  const items = listing.data?.items ?? []
  const pages = Math.max(1, Math.ceil((listing.data?.total ?? 0) / 10))
  const mappingPages = Math.max(1, Math.ceil((mappings.data?.total ?? 0) / 10))
  return <div className="space-y-6">
    <header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
      <p className="text-sm font-bold uppercase tracking-widest text-teal-700">{t('academicStructure')}</p>
      <h1 className="mt-2 text-3xl font-bold text-slate-900">{t(kind === 'classes' ? 'classesHeading' : 'subjectsHeading')}</h1>
      <p className="mt-2 text-slate-600">{t(kind === 'classes' ? 'classesDescription' : 'subjectsDescription')}</p>
      <nav className="mt-5 flex flex-wrap gap-2" aria-label={t('academicStructure')}>
        <Link to="/admin/academic/classes" className="rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800 hover:bg-teal-100">{t('classesHeading')}</Link>
        <Link to="/admin/academic/sections" className="rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800 hover:bg-teal-100">{t('sectionsHeading')}</Link>
        <Link to="/admin/academic/subjects" className="rounded-lg bg-teal-50 px-4 py-2 font-semibold text-teal-800 hover:bg-teal-100">{t('subjectsHeading')}</Link>
      </nav>
    </header>
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}
    {message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-800">{message}</p>}
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(17rem,1fr)]">
      <section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" aria-label={t('academicDirectory')}>
        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <label className="text-sm font-semibold text-slate-700">{t('search')}<input className={field} value={search} onChange={(e) => { setSearch(e.target.value); setPage(1) }} /></label>
          <label className="text-sm font-semibold text-slate-700">{t('status')}<select className={field} value={status} onChange={(e) => { setStatus(e.target.value); setPage(1) }}><option value="">{t('allStatuses')}</option><option value="active">{t('active')}</option><option value="inactive">{t('inactive')}</option></select></label>
        </div>
        {listing.isPending ? <p role="status" className="mt-4">{t('academicLoading')}</p> : listing.isError ? <p role="alert" className="mt-4 text-rose-700">{t('academicLoadError')}</p> : items.length === 0 ? <p className="mt-4 text-slate-600">{t('academicEmpty')}</p> : <ul className="mt-4 space-y-2">
          {items.map((item) => kind === 'classes' ? (() => { const value = item as AcademicClass; return <li key={value.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 p-3"><div className="min-w-0"><p className="font-semibold text-slate-900">{value.name}</p><p className="text-sm text-slate-600">{t('displayOrder')}: {value.display_order} · {t(value.is_active ? 'active' : 'inactive')}</p></div><div className="flex gap-2"><button className="rounded-lg px-3 py-2 text-sm font-semibold text-teal-800 hover:bg-teal-50" onClick={() => selectClass(value)}>{t('assignSubjects')}</button>{canManage && <button className="rounded-lg px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-100" onClick={() => editClass(value)}>{t('edit')}</button>}</div></li> })() : (() => { const value = item as AcademicSubject; return <li key={value.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 p-3"><div className="min-w-0"><p className="font-semibold text-slate-900">{i18n.language === 'bn' ? value.name_bn : value.name_en} <span className="text-sm font-normal text-slate-500">({value.code})</span></p><p className="text-sm text-slate-600">{t(value.is_active ? 'active' : 'inactive')}</p></div>{canManage && <button className="rounded-lg px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-100" onClick={() => editSubject(value)}>{t('edit')}</button>}</li> })())}
        </ul>}
        <div className="mt-5 flex items-center justify-between gap-3 text-sm"><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page <= 1} onClick={() => setPage(page-1)}>{t('previous')}</button><span>{t('pageOf', { page, total: pages })}</span><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page >= pages} onClick={() => setPage(page+1)}>{t('next')}</button></div>
      </section>
      {canManage && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" aria-label={t('academicEditor')}>
        <h2 className="text-xl font-bold text-slate-900">{t(editingId ? 'edit' : kind === 'classes' ? 'createClass' : 'createSubject')}</h2>
        <form className="mt-4 space-y-4" onSubmit={save}>
          {kind === 'classes' ? <><label className="block text-sm font-semibold">{t('className')}<input required maxLength={120} className={field} value={classForm.name} onChange={(e) => setClassForm({ ...classForm, name: e.target.value })} /></label><label className="block text-sm font-semibold">{t('displayOrder')}<input required type="number" min="0" max="100000" className={field} value={classForm.display_order} onChange={(e) => setClassForm({ ...classForm, display_order: Number(e.target.value) })} /></label></> : <><label className="block text-sm font-semibold">{t('subjectNameEN')}<input required maxLength={160} className={field} value={subjectForm.name_en} onChange={(e) => setSubjectForm({ ...subjectForm, name_en: e.target.value })} /></label><label className="block text-sm font-semibold">{t('subjectNameBN')}<input required maxLength={160} className={field} value={subjectForm.name_bn} onChange={(e) => setSubjectForm({ ...subjectForm, name_bn: e.target.value })} /></label><label className="block text-sm font-semibold">{t('subjectCode')}<input required maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9_-]*" className={field} value={subjectForm.code} onChange={(e) => setSubjectForm({ ...subjectForm, code: e.target.value })} /></label></>}
          <label className="flex items-center gap-2 text-sm font-semibold"><input type="checkbox" checked={kind === 'classes' ? classForm.is_active : subjectForm.is_active} onChange={(e) => kind === 'classes' ? setClassForm({ ...classForm, is_active: e.target.checked }) : setSubjectForm({ ...subjectForm, is_active: e.target.checked })} />{t('active')}</label>
          <div className="flex flex-wrap gap-2"><button disabled={busy} className="rounded-lg bg-teal-700 px-4 py-2 font-semibold text-white disabled:opacity-50">{t('saveChanges')}</button>{editingId && <button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setEditingId(''); setClassForm(emptyClass); setSubjectForm(emptySubject) }}>{t('cancel')}</button>}</div>
        </form>
      </section>}
    </div>
    {kind === 'classes' && selectedClass && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" aria-label={t('classSubjects')}>
      <h2 className="text-xl font-bold text-slate-900">{t('classSubjects')}: {selectedClass.name}</h2>
      <label className="mt-4 block max-w-sm text-sm font-semibold">{t('search')}<input className={field} value={mappingSearch} onChange={(e) => { setMappingSearch(e.target.value); setMappingPage(1) }} /></label>
      {mappings.isPending ? <p className="mt-4" role="status">{t('academicLoading')}</p> : mappings.isError ? <p className="mt-4 text-rose-700" role="alert">{t('academicLoadError')}</p> : mappings.data?.items.length === 0 ? <p className="mt-4 text-slate-600">{t('noClassSubjects')}</p> : <ul className="mt-4 grid gap-2 sm:grid-cols-2">{mappings.data?.items.map((mapping) => <li key={mapping.id} className="rounded-xl border border-slate-200 p-3"><p className="font-semibold">{i18n.language === 'bn' ? mapping.subject_name_bn : mapping.subject_name_en} ({mapping.subject_code})</p><p className="mt-1 text-sm text-slate-600">{t(mapping.is_required ? 'requiredSubject' : 'fourthSubject')}</p>{canManage && <div className="mt-3 flex flex-wrap gap-2 text-sm"><button disabled={busy} className="rounded-lg border px-3 py-2" onClick={() => void run(() => authorized((token) => updateClassSubject(token, selectedClass.id, mapping.subject_id, mapping.is_required)))}>{t('switchSubjectType')}</button><button disabled={busy} className="rounded-lg border border-rose-200 px-3 py-2 text-rose-700" onClick={() => { if (window.confirm(t('confirmRemoveSubject'))) void run(() => authorized((token) => removeClassSubject(token, selectedClass.id, mapping.subject_id))) }}>{t('remove')}</button></div>}</li>)}</ul>}
      <div className="mt-4 flex items-center justify-between gap-3 text-sm"><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={mappingPage <= 1} onClick={() => setMappingPage(mappingPage-1)}>{t('previous')}</button><span>{t('pageOf', { page: mappingPage, total: mappingPages })}</span><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={mappingPage >= mappingPages} onClick={() => setMappingPage(mappingPage+1)}>{t('next')}</button></div>
      {canManage && <form className="mt-6 grid gap-3 border-t border-slate-200 pt-5 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-end" onSubmit={addMapping}><div className="space-y-2"><label className="block text-sm font-semibold">{t('search')}<input className={field} value={subjectSearch} onChange={(e) => { setSubjectSearch(e.target.value); setSubjectId('') }} /></label><label className="block text-sm font-semibold">{t('subject')}<select required className={field} value={subjectId} onChange={(e) => setSubjectId(e.target.value)}><option value="">{t('chooseSubject')}</option>{allSubjects.data?.items.map((item) => <option key={item.id} value={item.id}>{i18n.language === 'bn' ? item.name_bn : item.name_en} ({item.code})</option>)}</select></label></div><label className="text-sm font-semibold">{t('subjectType')}<select className={field} value={fourth ? 'fourth' : 'required'} onChange={(e) => setFourth(e.target.value === 'fourth')}><option value="required">{t('requiredSubject')}</option><option value="fourth">{t('fourthSubject')}</option></select></label><button disabled={busy || !selectedClass.is_active} className="rounded-lg bg-teal-700 px-4 py-2 font-semibold text-white disabled:opacity-50">{t('assignSubjects')}</button></form>}
      {allSubjects.data && allSubjects.data.total > 100 && <p className="mt-2 text-sm text-amber-700">{t('subjectOptionsLimit')}</p>}
    </section>}
  </div>
}
