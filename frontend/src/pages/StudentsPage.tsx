import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, bloodGroups, deleteStudent, getAcademicClasses, getAcademicSections, getAcademicSessions, getStudents, getUsers, importStudents, saveStudent, type Gender, type Student, type StudentInput, type StudentProfileDetails } from '../api/client'
import { peopleField, usePeopleApi } from './peopleShared'
import { ui, PageHeader, LinkTabs } from './attendanceUi'

const blankProfile: StudentProfileDetails = { student_code: '', gender: '', blood_group: '', father_name_en: '', father_name_bn: '', mother_name_en: '', mother_name_bn: '', present_address: '', permanent_address: '', admission_date: '' }
type StudentForm = StudentInput & StudentProfileDetails
const blank: StudentForm = { admission_no: '', name_en: '', name_bn: '', date_of_birth: '', status: 'active', session_id: '', class_id: '', section_id: '', roll_no: null, user_id: '', ...blankProfile }

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
  const [form, setForm] = useState<StudentForm>(blank)
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
  function edit(item: Student) { setEditing(item.id); setForm({ admission_no: item.admission_no, name_en: item.name_en, name_bn: item.name_bn, date_of_birth: item.date_of_birth, status: item.status, session_id: item.session_id, class_id: item.class_id, section_id: item.section_id, roll_no: item.roll_no, user_id: item.user_id ?? '', ...Object.fromEntries(Object.keys(blankProfile).map(key => [key, item[key as keyof StudentProfileDetails] ?? ''])) as StudentProfileDetails }); setError(''); setMessage('') }
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
    <PageHeader eyebrow={t('m12Group_people')} title={t('studentsHeading')} description={t('peopleDescription')} actions={canManage ? <Link to="/admin/students/cards" className={ui.secondary}>{t('idCardPrintBulk')}</Link> : undefined} />
    <LinkTabs label={t('m12Group_people')} items={[{ to: '/admin/students', label: t('studentsHeading') }, { to: '/admin/guardians', label: t('guardiansHeading') }]} />
    {error && <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900">{error}</p>}{message && <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-900">{message}</p>}
    <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(19rem,1fr)]">
      <section className={`${ui.card} min-w-0 p-4 sm:p-6`}>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className={ui.label}>{t('search')}<input className={peopleField} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></label>
          <label className={ui.label}>{t('studentStatus')}<select className={peopleField} value={status} onChange={e => { setStatus(e.target.value); setPage(1) }}><option value="">{t('allStudentStatuses')}</option>{(['active','inactive','graduated','transferred'] as const).map(s => <option key={s} value={s}>{t(`${s}Student`)}</option>)}</select></label>
          <label className={ui.label}>{t('sessionLabel')}<select className={peopleField} value={sessionFilter} onChange={e => { setSessionFilter(e.target.value); setSectionFilter(''); setPage(1) }}><option value="">{t('allSessions')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <label className={ui.label}>{t('classLabel')}<select className={peopleField} value={classFilter} onChange={e => { setClassFilter(e.target.value); setSectionFilter(''); setPage(1) }}><option value="">{t('allClasses')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
          <label className={ui.label}>{t('sectionLabel')}<select className={peopleField} value={sectionFilter} onChange={e => { setSectionFilter(e.target.value); setPage(1) }}><option value="">{t('allSections')}</option>{filterSections.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        </div>
        {listing.isPending ? <p role="status" className="mt-4">{t('academicLoading')}</p> : listing.isError ? <p role="alert" className="mt-4 text-rose-700">{t('peopleLoadError')}</p> : listing.data.items.length === 0 ? <p className="mt-4 text-slate-600">{t('noStudents')}</p> : <ul className="mt-4 space-y-2">{listing.data.items.map(item => <li key={item.id} className="rounded-xl border border-slate-200 p-3"><div className="flex flex-wrap justify-between gap-2"><div className="min-w-0"><p className="font-semibold">{i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en}</p><p className="text-sm text-slate-600">{item.admission_no} · {item.class_name} / {item.section_name} · {t(`${item.status}Student`)}</p></div><div className="flex gap-2"><Link className="inline-flex min-h-10 items-center rounded-lg px-3 text-sm font-semibold text-[var(--brand-ink)] hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]" to={`/admin/students/${item.id}`}>{t('viewProfile')}</Link>{canManage && <button className="inline-flex min-h-10 items-center rounded-lg px-3 text-sm font-semibold text-[var(--brand-ink)] hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]" onClick={() => edit(item)}>{t('edit')}</button>}</div></div></li>)}</ul>}
        {pages > 1 && <div className="border-t border-slate-100 pt-4 mt-5 flex items-center justify-between gap-2 text-sm"><button className={ui.secondary} disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('previous')}</button><span>{t('pageOf', { page, total: pages })}</span><button className={ui.secondary} disabled={page >= pages} onClick={() => setPage(page + 1)}>{t('next')}</button></div>}
      </section>
      {canManage && <div className="min-w-0 space-y-6"><section className={`${ui.card} min-w-0 p-4 sm:p-6`}><h2 className={ui.sectionTitle}>{t(editing ? 'editStudent' : 'createStudent')}</h2><form className="mt-4 space-y-3" onSubmit={e => void save(e)}>
        <label className={ui.label}>{t('admissionNo')}<input required maxLength={64} className={peopleField} value={form.admission_no} onChange={e => setForm({ ...form, admission_no: e.target.value })} /></label>
        <label className={ui.label}>{t('studentNameEN')}<input required maxLength={160} className={peopleField} value={form.name_en} onChange={e => setForm({ ...form, name_en: e.target.value })} /></label>
        <label className={ui.label}>{t('studentNameBN')}<input maxLength={160} className={peopleField} value={form.name_bn} onChange={e => setForm({ ...form, name_bn: e.target.value })} /></label>
        <label className={ui.label}>{t('dateOfBirth')}<input type="date" className={peopleField} value={form.date_of_birth} onChange={e => setForm({ ...form, date_of_birth: e.target.value })} /></label>
        <label className={ui.label}>{t('studentStatus')}<select className={peopleField} value={form.status} onChange={e => setForm({ ...form, status: e.target.value as StudentInput['status'] })}>{(['active','inactive','graduated','transferred'] as const).map(s => <option key={s} value={s}>{t(`${s}Student`)}</option>)}</select></label>
        <label className={ui.label}>{t('search')}<input className={peopleField} value={optionSearch} onChange={e => setOptionSearch(e.target.value)} /></label>
        <label className={ui.label}>{t('sessionLabel')}<select required className={peopleField} value={form.session_id} onChange={e => setForm({ ...form, session_id: e.target.value, section_id: '' })}><option value="">{t('selectSession')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('classLabel')}<select required className={peopleField} value={form.class_id} onChange={e => setForm({ ...form, class_id: e.target.value, section_id: '' })}><option value="">{t('selectClass')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('sectionLabel')}<select required className={peopleField} value={form.section_id} onChange={e => setForm({ ...form, section_id: e.target.value })}><option value="">{t('selectSection')}</option>{sections.data?.items.filter(s => s.session_id === form.session_id && s.class_id === form.class_id).map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('rollNo')}<input type="number" min="1" max="999999" className={peopleField} value={form.roll_no ?? ''} onChange={e => setForm({ ...form, roll_no: e.target.value ? Number(e.target.value) : null })} /></label>
        <label className={ui.label}>{t('studentAccount')}<input className={peopleField} aria-label={`${t('studentAccount')} · ${t('search')}`} placeholder={t('search')} value={accountSearch} onChange={e => setAccountSearch(e.target.value)} /><select className={peopleField} aria-label={t('studentAccount')} value={form.user_id ?? ''} onChange={e => setForm({ ...form, user_id: e.target.value })}><option value="">{t('optionalAccount')}</option>{accounts.data?.items.map(u => <option key={u.id} value={u.id}>{u.display_name || u.email} ({u.email})</option>)}{form.user_id && !accounts.data?.items.some(u => u.id === form.user_id) && <option value={form.user_id}>{form.user_id}</option>}</select></label>
        <fieldset className="space-y-3 rounded-xl border border-slate-200 p-3">
          <legend className="px-1 text-sm font-semibold text-slate-900">{t('profileDetails')}</legend>
          <p className="text-xs text-slate-500">{t('profileDetailsHelp')}</p>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className={ui.label}>{t('idCardStudentId')}<input maxLength={64} className={peopleField} value={form.student_code} onChange={e => setForm({ ...form, student_code: e.target.value })} /></label>
            <label className={ui.label}>{t('profileAdmissionDate')}<input type="date" className={peopleField} value={form.admission_date} onChange={e => setForm({ ...form, admission_date: e.target.value })} /></label>
            <label className={ui.label}>{t('profileGender')}<select className={peopleField} value={form.gender} onChange={e => setForm({ ...form, gender: e.target.value as Gender })}><option value="">{t('profileNotProvided')}</option>{(['female', 'male', 'other'] as const).map(g => <option key={g} value={g}>{t(`gender_${g}`)}</option>)}</select></label>
            <label className={ui.label}>{t('profileBloodGroup')}<select className={peopleField} value={form.blood_group} onChange={e => setForm({ ...form, blood_group: e.target.value })}><option value="">{t('profileNotProvided')}</option>{bloodGroups.map(g => <option key={g} value={g}>{g}</option>)}</select></label>
            <label className={ui.label}>{t('profileFatherEN')}<input maxLength={160} className={peopleField} value={form.father_name_en} onChange={e => setForm({ ...form, father_name_en: e.target.value })} /></label>
            <label className={ui.label}>{t('profileFatherBN')}<input maxLength={160} lang="bn" className={peopleField} value={form.father_name_bn} onChange={e => setForm({ ...form, father_name_bn: e.target.value })} /></label>
            <label className={ui.label}>{t('profileMotherEN')}<input maxLength={160} className={peopleField} value={form.mother_name_en} onChange={e => setForm({ ...form, mother_name_en: e.target.value })} /></label>
            <label className={ui.label}>{t('profileMotherBN')}<input maxLength={160} lang="bn" className={peopleField} value={form.mother_name_bn} onChange={e => setForm({ ...form, mother_name_bn: e.target.value })} /></label>
          </div>
          <label className={ui.label}>{t('profilePresentAddress')}<textarea rows={2} maxLength={500} className={peopleField} value={form.present_address} onChange={e => setForm({ ...form, present_address: e.target.value })} /></label>
          <label className={ui.label}>{t('profilePermanentAddress')}<textarea rows={2} maxLength={500} className={peopleField} value={form.permanent_address} onChange={e => setForm({ ...form, permanent_address: e.target.value })} /></label>
          <button type="button" className={ui.secondary} disabled={!form.present_address} onClick={() => setForm({ ...form, permanent_address: form.present_address })}>{t('profileSameAddress')}</button>
          <p className="text-xs text-slate-500">{t('profileGuardianNote')}</p>
        </fieldset>
        <div className="flex flex-wrap gap-2"><button disabled={busy} className={ui.primary}>{t('saveChanges')}</button>{editing && <><button type="button" className={ui.secondary} onClick={() => { setEditing(''); setForm(blank) }}>{t('cancel')}</button><button type="button" disabled={busy} className={ui.danger} onClick={() => void remove()}>{t('deleteStudent')}</button></>}</div>
      </form></section><section className={`${ui.card} min-w-0 p-4 sm:p-6`}><h2 className={ui.sectionTitle}>{t('importCSV')}</h2><p className="mt-2 break-all text-sm text-slate-600">{t('importHelp')}</p><form className="mt-4 space-y-3" onSubmit={e => void importCSV(e)}><label className={ui.label}>{t('chooseCSV')}<input required accept=".csv,text/csv" type="file" className={peopleField} onChange={e => setCSV(e.target.files?.[0] ?? null)} /></label><button disabled={busy || !form.session_id || !form.class_id || !form.section_id} className={ui.primary}>{t('importCSV')}</button></form></section></div>}
    </div>
  </div>
}
