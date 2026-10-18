import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, deleteStudent, getAcademicClasses, getAcademicSections, getAcademicSessions, getStudents, getUsers, importStudents, saveStudent, type Student, type StudentInput } from '../api/client'
import { peopleField, usePeopleApi } from './peopleShared'

const blank: StudentInput = { admission_no: '', name_en: '', name_bn: '', date_of_birth: '', status: 'active', session_id: '', class_id: '', section_id: '', roll_no: null, user_id: '' }

export function StudentsPage({ canManage }: { canManage: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('')
  const [sessionFilter, setSessionFilter] = useState('')
  const [classFilter, setClassFilter] = useState('')
  const [sectionFilter, setSectionFilter] = useState('')
  const [page, setPage] = useState(1)
  const [form, setForm] = useState<StudentInput>(blank)
  const [editing, setEditing] = useState('')
  const [accountSearch, setAccountSearch] = useState('')
  const [optionSearch, setOptionSearch] = useState('')
  const [csv, setCSV] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const params = new URLSearchParams({ page: String(page), page_size: '10', search })
  if (status) params.set('status', status)
  if (sessionFilter) params.set('session_id', sessionFilter)
  if (classFilter) params.set('class_id', classFilter)
  if (sectionFilter) params.set('section_id', sectionFilter)
  const listing = useQuery({ queryKey: ['people', 'students', params.toString()], queryFn: () => authorized(token => getStudents(token, params)), retry: false })
  const sessions = useQuery({ queryKey: ['people', 'session-options', optionSearch], queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100', search: optionSearch }))), retry: false })
  const classes = useQuery({ queryKey: ['people', 'class-options', optionSearch], queryFn: () => authorized(token => getAcademicClasses(token, new URLSearchParams({ page: '1', page_size: '100', search: optionSearch }))), retry: false })
  const sections = useQuery({ queryKey: ['people', 'section-options', form.session_id, form.class_id, optionSearch], queryFn: () => {
    const q = new URLSearchParams({ page: '1', page_size: '100', search: optionSearch })
    if (form.session_id) q.set('session_id', form.session_id)
    if (form.class_id) q.set('class_id', form.class_id)
    return authorized(token => getAcademicSections(token, q))
  }, retry: false })
  const filterSections = useQuery({ queryKey: ['people', 'filter-section-options', sessionFilter, classFilter], queryFn: () => {
    const q = new URLSearchParams({ page: '1', page_size: '100' })
    if (sessionFilter) q.set('session_id', sessionFilter)
    if (classFilter) q.set('class_id', classFilter)
    return authorized(token => getAcademicSections(token, q))
  }, retry: false })
  const accounts = useQuery({ queryKey: ['people', 'student-accounts', accountSearch], enabled: canManage, queryFn: () => authorized(token => getUsers(token, new URLSearchParams({ role: 'student', search: accountSearch, page_size: '100' }))), retry: false })
  const pages = Math.max(1, Math.ceil((listing.data?.total ?? 0) / 10))
  function edit(item: Student) { setEditing(item.id); setForm({ admission_no: item.admission_no, name_en: item.name_en, name_bn: item.name_bn, date_of_birth: item.date_of_birth, status: item.status, session_id: item.session_id, class_id: item.class_id, section_id: item.section_id, roll_no: item.roll_no, user_id: item.user_id ?? '' }); setError(''); setMessage('') }
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(''); setMessage('')
    try { await authorized(token => saveStudent(token, form, editing || undefined)); setEditing(''); setForm(blank); await cache.invalidateQueries({ queryKey: ['people', 'students'] }); setMessage(t('peopleSaved')) }
    catch (cause) { setError(cause instanceof ApiError && cause.status === 409 ? t('peopleDuplicate') : t('peopleSaveError')) }
    finally { setBusy(false) }
  }
  async function remove() {
    if (!editing || !(await confirmAction(t('deleteStudentConfirm', { name: form.name_en })))) return
    setBusy(true); setError(''); setMessage('')
    try { await authorized(token => deleteStudent(token, editing)); setEditing(''); setForm(blank); if (listing.data?.items.length === 1) setPage(Math.max(1, page - 1)); await cache.invalidateQueries({ queryKey: ['people', 'students'] }); setMessage(t('peopleDeleted')) }
    catch { setError(t('peopleSaveError')) }
    finally { setBusy(false) }
  }
  async function importCSV(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!csv) return; setBusy(true); setError(''); setMessage('')
    try {
      if (csv.size > 2097152) throw new Error('too large')
      const q = new URLSearchParams({ session_id: form.session_id, class_id: form.class_id, section_id: form.section_id })
      const text = await csv.text()
      const result = await authorized(token => importStudents(token, q, text))
      await cache.invalidateQueries({ queryKey: ['people', 'students'] }); setMessage(t('importedCount', { count: result.imported })); setCSV(null)
    } catch (cause) { setError(cause instanceof ApiError && cause.status === 409 ? t('peopleDuplicate') : t('peopleSaveError')) }
    finally { setBusy(false) }
  }
  return <div className="space-y-6">
    <header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm"><p className="text-sm font-bold uppercase tracking-widest text-[var(--brand-ink)]">{t('m12Group_people')}</p><h1 className="mt-2 text-3xl font-bold">{t('studentsHeading')}</h1><p className="mt-2 text-slate-600">{t('peopleDescription')}</p><Link to="/admin/guardians" className="mt-4 inline-block rounded-lg bg-[var(--brand-soft)] px-4 py-2 font-semibold text-[var(--brand-ink)]">{t('guardiansHeading')}</Link></header>
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}{message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-800">{message}</p>}
    <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(19rem,1fr)]">
      <section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="text-sm font-semibold">{t('search')}<input className={peopleField} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></label>
          <label className="text-sm font-semibold">{t('studentStatus')}<select className={peopleField} value={status} onChange={e => { setStatus(e.target.value); setPage(1) }}><option value="">{t('allStudentStatuses')}</option>{(['active','inactive','graduated','transferred'] as const).map(s => <option key={s} value={s}>{t(`${s}Student`)}</option>)}</select></label>
          <label className="text-sm font-semibold">{t('sessionLabel')}<select className={peopleField} value={sessionFilter} onChange={e => { setSessionFilter(e.target.value); setSectionFilter(''); setPage(1) }}><option value="">{t('allSessions')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <label className="text-sm font-semibold">{t('classLabel')}<select className={peopleField} value={classFilter} onChange={e => { setClassFilter(e.target.value); setSectionFilter(''); setPage(1) }}><option value="">{t('allClasses')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
          <label className="text-sm font-semibold">{t('sectionLabel')}<select className={peopleField} value={sectionFilter} onChange={e => { setSectionFilter(e.target.value); setPage(1) }}><option value="">{t('allSections')}</option>{filterSections.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        </div>
        {listing.isPending ? <p role="status" className="mt-4">{t('academicLoading')}</p> : listing.isError ? <p role="alert" className="mt-4 text-rose-700">{t('peopleLoadError')}</p> : listing.data.items.length === 0 ? <p className="mt-4 text-slate-600">{t('noStudents')}</p> : <ul className="mt-4 space-y-2">{listing.data.items.map(item => <li key={item.id} className="rounded-xl border border-slate-200 p-3"><div className="flex flex-wrap justify-between gap-2"><div className="min-w-0"><p className="font-semibold">{i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en}</p><p className="text-sm text-slate-600">{item.admission_no} · {item.class_name} / {item.section_name} · {t(`${item.status}Student`)}</p></div><div className="flex gap-2"><Link className="rounded-lg px-3 py-2 font-semibold text-[var(--brand-ink)]" to={`/admin/students/${item.id}`}>{t('viewProfile')}</Link>{canManage && <button className="rounded-lg px-3 py-2 font-semibold text-[var(--brand-ink)]" onClick={() => edit(item)}>{t('edit')}</button>}</div></div></li>)}</ul>}
        <div className="mt-5 flex items-center justify-between gap-2 text-sm"><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('previous')}</button><span>{t('pageOf', { page, total: pages })}</span><button className="rounded-lg border px-3 py-2 disabled:opacity-40" disabled={page >= pages} onClick={() => setPage(page + 1)}>{t('next')}</button></div>
      </section>
      {canManage && <div className="min-w-0 space-y-6"><section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-bold">{t(editing ? 'editStudent' : 'createStudent')}</h2><form className="mt-4 space-y-3" onSubmit={e => void save(e)}>
        <label className="block text-sm font-semibold">{t('admissionNo')}<input required maxLength={64} className={peopleField} value={form.admission_no} onChange={e => setForm({ ...form, admission_no: e.target.value })} /></label>
        <label className="block text-sm font-semibold">{t('studentNameEN')}<input required maxLength={160} className={peopleField} value={form.name_en} onChange={e => setForm({ ...form, name_en: e.target.value })} /></label>
        <label className="block text-sm font-semibold">{t('studentNameBN')}<input maxLength={160} className={peopleField} value={form.name_bn} onChange={e => setForm({ ...form, name_bn: e.target.value })} /></label>
        <label className="block text-sm font-semibold">{t('dateOfBirth')}<input type="date" className={peopleField} value={form.date_of_birth} onChange={e => setForm({ ...form, date_of_birth: e.target.value })} /></label>
        <label className="block text-sm font-semibold">{t('studentStatus')}<select className={peopleField} value={form.status} onChange={e => setForm({ ...form, status: e.target.value as StudentInput['status'] })}>{(['active','inactive','graduated','transferred'] as const).map(s => <option key={s} value={s}>{t(`${s}Student`)}</option>)}</select></label>
        <label className="block text-sm font-semibold">{t('search')}<input className={peopleField} value={optionSearch} onChange={e => setOptionSearch(e.target.value)} /></label>
        <label className="block text-sm font-semibold">{t('sessionLabel')}<select required className={peopleField} value={form.session_id} onChange={e => setForm({ ...form, session_id: e.target.value, section_id: '' })}><option value="">{t('selectSession')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className="block text-sm font-semibold">{t('classLabel')}<select required className={peopleField} value={form.class_id} onChange={e => setForm({ ...form, class_id: e.target.value, section_id: '' })}><option value="">{t('selectClass')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className="block text-sm font-semibold">{t('sectionLabel')}<select required className={peopleField} value={form.section_id} onChange={e => setForm({ ...form, section_id: e.target.value })}><option value="">{t('selectSection')}</option>{sections.data?.items.filter(s => s.session_id === form.session_id && s.class_id === form.class_id).map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className="block text-sm font-semibold">{t('rollNo')}<input type="number" min="1" max="999999" className={peopleField} value={form.roll_no ?? ''} onChange={e => setForm({ ...form, roll_no: e.target.value ? Number(e.target.value) : null })} /></label>
        <label className="block text-sm font-semibold">{t('studentAccount')}<input className={peopleField} aria-label={`${t('studentAccount')} · ${t('search')}`} placeholder={t('search')} value={accountSearch} onChange={e => setAccountSearch(e.target.value)} /><select className={peopleField} aria-label={t('studentAccount')} value={form.user_id ?? ''} onChange={e => setForm({ ...form, user_id: e.target.value })}><option value="">{t('optionalAccount')}</option>{accounts.data?.items.map(u => <option key={u.id} value={u.id}>{u.display_name || u.email} ({u.email})</option>)}{form.user_id && !accounts.data?.items.some(u => u.id === form.user_id) && <option value={form.user_id}>{form.user_id}</option>}</select></label>
        <div className="flex flex-wrap gap-2"><button disabled={busy} className="rounded-lg bg-[var(--brand-primary)] px-4 py-2 font-semibold text-[var(--brand-on)] disabled:opacity-50">{t('saveChanges')}</button>{editing && <><button type="button" className="rounded-lg border px-4 py-2" onClick={() => { setEditing(''); setForm(blank) }}>{t('cancel')}</button><button type="button" disabled={busy} className="rounded-lg border border-rose-300 px-4 py-2 font-semibold text-rose-700" onClick={() => void remove()}>{t('deleteStudent')}</button></>}</div>
      </form></section><section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-bold">{t('importCSV')}</h2><p className="mt-2 break-all text-sm text-slate-600">{t('importHelp')}</p><form className="mt-4 space-y-3" onSubmit={e => void importCSV(e)}><label className="block text-sm font-semibold">{t('chooseCSV')}<input required accept=".csv,text/csv" type="file" className={peopleField} onChange={e => setCSV(e.target.files?.[0] ?? null)} /></label><button disabled={busy || !form.session_id || !form.class_id || !form.section_id} className="rounded-lg bg-[var(--brand-primary)] px-4 py-2 font-semibold text-[var(--brand-on)] disabled:opacity-50">{t('importCSV')}</button></form></section></div>}
    </div>
  </div>
}
