import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getGuardians, getMyStudent, getStudent, getStudentCard, getStudentGuardians, getStudentPhoto, linkStudentGuardian, saveStudentPhoto, unlinkStudentGuardian, type Student } from '../api/client'
import { usePeopleApi } from './peopleShared'
import { CardBack, CardFront, type CardLanguage } from '../components/IdCard'
import { Icon } from '../components/icons'
import { StateScreen } from '../components/StateScreen'
import { Avatar, EmptyState, Notice as Banner, PageHeader, SectionCard, Skeleton, StatusBadge, ui, useNumber } from './attendanceUi'

function formatDate(value: string, lang: string) {
  if (!value) return ''
  const [y, m, d] = value.split('-').map(Number)
  return new Intl.DateTimeFormat(lang === 'bn' ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}

// A label/value row that shows a quiet placeholder for details not yet recorded.
function Field({ label, children, wide = false }: { label: string; children: ReactNode; wide?: boolean }) {
  const { t } = useTranslation()
  const empty = children === '' || children === null || children === undefined
  return <div className={wide ? 'sm:col-span-2' : ''}>
    <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt>
    <dd className={`mt-1 text-sm ${empty ? 'italic text-slate-500' : 'font-medium text-slate-900'}`} style={{ overflowWrap: 'anywhere' }}>{empty ? t('profileNotProvided') : children}</dd>
  </div>
}

export function StudentProfilePage({ mode, canManagePhoto, canManageLinks, canReadGuardians }: { mode: 'staff' | 'self' | 'child'; canManagePhoto: boolean; canManageLinks: boolean; canReadGuardians: boolean }) {
  const { id } = useParams()
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const num = useNumber()
  const cache = useQueryClient()
  const [photoUrl, setPhotoUrl] = useState('')
  const [photoFile, setPhotoFile] = useState<File | null>(null)
  const [guardianID, setGuardianID] = useState('')
  const [guardianSearch, setGuardianSearch] = useState('')
  const [relationship, setRelationship] = useState('')
  const [primary, setPrimary] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [cardLang, setCardLang] = useState<CardLanguage>(i18n.language === 'bn' ? 'bn' : 'en')
  const student = useQuery({ queryKey: ['people', 'profile', mode, id], queryFn: () => authorized(token => mode === 'self' ? getMyStudent(token) : getStudent(token, id!)), enabled: mode === 'self' || !!id, retry: false })
  const studentID = student.data?.id
  const card = useQuery({ queryKey: ['people', 'card', studentID], queryFn: () => authorized(token => getStudentCard(token, studentID!)), enabled: !!studentID, retry: false })
  const photo = useQuery({ queryKey: ['people', 'photo', studentID], queryFn: () => authorized(token => getStudentPhoto(token, studentID!)), enabled: !!studentID && !!student.data?.has_photo, retry: false })
  const guardianLinks = useQuery({ queryKey: ['people', 'links', studentID], queryFn: () => authorized(token => getStudentGuardians(token, studentID!)), enabled: !!studentID && canReadGuardians, retry: false })
  const guardians = useQuery({ queryKey: ['people', 'guardian-options', guardianSearch], queryFn: () => authorized(token => getGuardians(token, new URLSearchParams({ search: guardianSearch, page_size: '100' }))), enabled: canManageLinks, retry: false })
  useEffect(() => { if (!photo.data) { setPhotoUrl(''); return }; const url = URL.createObjectURL(photo.data); setPhotoUrl(url); return () => URL.revokeObjectURL(url) }, [photo.data])
  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!photoFile || !studentID) return; setBusy(true); setError(''); setMessage('')
    try { if (photoFile.size > 2097152) throw new Error('too large'); await authorized(token => saveStudentPhoto(token, studentID, photoFile)); await cache.invalidateQueries({ queryKey: ['people', 'photo', studentID] }); await cache.invalidateQueries({ queryKey: ['people', 'profile'] }); setMessage(t('peopleSaved')); setPhotoFile(null) }
    catch { setError(t('peopleSaveError')) } finally { setBusy(false) }
  }
  async function link(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!studentID) return; setBusy(true); setError(''); setMessage('')
    try { await authorized(token => linkStudentGuardian(token, studentID, guardianID, relationship, primary)); await cache.invalidateQueries({ queryKey: ['people', 'links', studentID] }); await cache.invalidateQueries({ queryKey: ['people', 'profile'] }); setGuardianID(''); setRelationship(''); setPrimary(false); setMessage(t('peopleSaved')) }
    catch (cause) { setError(cause instanceof ApiError && cause.status === 409 ? t('peopleDuplicate') : t('peopleSaveError')) } finally { setBusy(false) }
  }
  async function unlink(guardianId: string) {
    if (!studentID) return; setBusy(true); setError(''); setMessage('')
    try { await authorized(token => unlinkStudentGuardian(token, studentID, guardianId)); await cache.invalidateQueries({ queryKey: ['people', 'links', studentID] }); await cache.invalidateQueries({ queryKey: ['people', 'profile'] }); setMessage(t('peopleSaved')) }
    catch { setError(t('peopleSaveError')) } finally { setBusy(false) }
  }
  if (student.isPending) return <StateScreen kind="loading" message={t('academicLoading')} />
  if (student.isError) return <StateScreen kind="error" message={t('peopleLoadError')} />
  const value: Student = student.data
  const bn = i18n.language === 'bn'
  const pick = (en: string, other: string) => bn && other ? other : en
  const name = pick(value.name_en, value.name_bn)
  const statusTone = value.status === 'active' ? 'present' : value.status === 'inactive' ? 'unmarked' : 'leave'
  const guardian = value.primary_guardian
  const alternate = (en: string, other: string) => en && other ? (bn ? en : other) : ''
  return <div className="space-y-6">
    <PageHeader eyebrow={t(mode === 'self' ? 'myStudentProfile' : 'profileHeading')} title={name}
      description={`${t('admissionNo')}: ${value.admission_no} · ${value.class_name} · ${value.section_name}`}
      actions={mode === 'staff' ? <div className="flex flex-wrap gap-2">
        {canManagePhoto && <Link to={`/admin/students/${value.id}/card`} className={ui.primary}><Icon name="print" className="h-4 w-4" />{t('idCardPrintOne')}</Link>}
        <Link to="/admin/students" className={ui.secondary}><Icon name="chevronLeft" className="h-4 w-4" />{t('studentsHeading')}</Link></div> : undefined} />
    {error && <Banner tone="error">{error}</Banner>}{message && <Banner tone="success">{message}</Banner>}

    <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm" aria-labelledby="profile-identity">
      <div className="h-2 bg-[var(--brand-primary)]" aria-hidden="true" />
      <div className="flex flex-col gap-5 p-5 sm:flex-row sm:items-center sm:p-6">
        {photoUrl
          ? <img src={photoUrl} alt={t('idCardPhotoAlt', { name })} className="h-28 w-24 shrink-0 rounded-xl object-cover ring-1 ring-slate-200" />
          : <span role="img" aria-label={t('noPhoto')} className="grid h-28 w-24 shrink-0 place-items-center rounded-xl bg-[var(--brand-soft)] text-3xl font-bold text-[var(--brand-ink)]">{[...name][0]}</span>}
        <div className="min-w-0 flex-1">
          <h2 id="profile-identity" className="text-xl font-bold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{name}</h2>
          {alternate(value.name_en, value.name_bn) && <p className="text-sm text-slate-600">{alternate(value.name_en, value.name_bn)}</p>}
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <StatusBadge tone={statusTone}>{t(`${value.status}Student`)}</StatusBadge>
            {value.blood_group && <span className="inline-flex items-center rounded-full bg-rose-50 px-2.5 py-0.5 text-xs font-bold text-rose-800">{t('profileBloodGroup')}: {value.blood_group}</span>}
            <span className="inline-flex items-center rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-semibold text-slate-700">{t('idCardStudentId')}: {value.student_code || value.admission_no}</span>
            {!photoUrl && <span className="text-xs text-slate-500">{t('noPhoto')}</span>}
          </div>
        </div>
      </div>
      {canManagePhoto && <form className="flex flex-col gap-3 border-t border-slate-100 bg-slate-50 p-4 sm:flex-row sm:items-end sm:px-6" onSubmit={e => void upload(e)}>
        <label className={`${ui.label} flex-1`}>{t('selectPhoto')}<input type="file" required accept="image/jpeg,image/png" className={ui.field} onChange={e => setPhotoFile(e.target.files?.[0] ?? null)} /></label>
        <button disabled={busy || !photoFile} className={ui.primary}>{t('uploadPhoto')}</button></form>}
    </section>

    <div className="grid gap-6 lg:grid-cols-2">
      <SectionCard title={t('profilePersonal')} id="profile-personal">
        <dl className="grid gap-4 sm:grid-cols-2">
          <Field label={t('studentNameEN')}>{value.name_en}</Field>
          <Field label={t('studentNameBN')}>{value.name_bn}</Field>
          <Field label={t('dateOfBirth')}>{formatDate(value.date_of_birth, i18n.language)}</Field>
          <Field label={t('profileGender')}>{value.gender ? t(`gender_${value.gender}`) : ''}</Field>
          <Field label={t('profileBloodGroup')}>{value.blood_group}</Field>
        </dl>
      </SectionCard>
      <SectionCard title={t('profileFamily')} id="profile-family">
        <dl className="grid gap-4 sm:grid-cols-2">
          <Field label={t('profileFather')}>{pick(value.father_name_en, value.father_name_bn)}</Field>
          <Field label={t('profileMother')}>{pick(value.mother_name_en, value.mother_name_bn)}</Field>
          <Field label={t('profileGuardian')}>{guardian ? pick(guardian.name_en, guardian.name_bn) : ''}</Field>
          <Field label={t('relationshipLabel')}>{guardian?.relationship ?? ''}</Field>
          <Field label={t('profileGuardianPhone')}>{guardian?.phone ? <a className="text-[var(--brand-ink)] underline-offset-2 hover:underline" href={`tel:${guardian.phone.replace(/[^+0-9]/g, '')}`}>{guardian.phone}</a> : ''}</Field>
        </dl>
      </SectionCard>
      <SectionCard title={t('profileAddress')} id="profile-address">
        <dl className="grid gap-4">
          <Field label={t('profilePresentAddress')}>{value.present_address}</Field>
          <Field label={t('profilePermanentAddress')}>{value.permanent_address}</Field>
        </dl>
      </SectionCard>
      <SectionCard title={t('profileAcademic')} id="profile-academic">
        <dl className="grid gap-4 sm:grid-cols-2">
          <Field label={t('idCardStudentId')}>{value.student_code}</Field>
          <Field label={t('admissionNo')}>{value.admission_no}</Field>
          <Field label={t('classLabel')}>{value.class_name}</Field>
          <Field label={t('sectionLabel')}>{value.section_name}</Field>
          <Field label={t('rollNo')}>{value.roll_no == null ? '' : num(value.roll_no)}</Field>
          <Field label={t('sessionLabel')}>{value.session_name}</Field>
          <Field label={t('profileAdmissionDate')}>{formatDate(value.admission_date, i18n.language)}</Field>
        </dl>
      </SectionCard>
    </div>

    <SectionCard title={t('studentCard')} description={t('idCardPreviewHelp')} id="student-card"
      actions={<label className="flex items-center gap-2 text-sm font-semibold text-slate-700">{t('idCardLanguage')}<select className="rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm" value={cardLang} onChange={e => setCardLang(e.target.value as CardLanguage)}><option value="en">English</option><option value="bn">বাংলা</option></select></label>}>
      {card.isPending ? <Skeleton rows={3} /> : card.isError ? <Banner tone="error">{t('peopleLoadError')}</Banner> :
        // Focusable so keyboard users can scroll the preview on narrow screens.
        <div className="overflow-x-auto pb-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)]" tabIndex={0} role="region" aria-label={t('idCardPreview')}>
          <div className="flex w-max gap-6" style={{ zoom: 1.25 }}>
            <figure className="m-0"><div className="rounded-[3mm] shadow-[0_8px_24px_rgb(15_23_42/0.18)]"><CardFront card={card.data} school={card.data.school} settings={card.data.settings} lang={cardLang} photoUrl={photoUrl} /></div><figcaption className="mt-1 text-center text-xs text-slate-500">{t('idCardFront')}</figcaption></figure>
            <figure className="m-0"><div className="rounded-[3mm] shadow-[0_8px_24px_rgb(15_23_42/0.18)]"><CardBack card={card.data} school={card.data.school} settings={card.data.settings} lang={cardLang} /></div><figcaption className="mt-1 text-center text-xs text-slate-500">{t('idCardBack')}</figcaption></figure>
          </div>
        </div>}
    </SectionCard>

    {canReadGuardians && <SectionCard title={t('guardiansForStudent')} description={t('linkHelp')} id="student-guardians">
      {guardianLinks.isPending ? <Skeleton rows={2} /> : guardianLinks.isError ? <Banner tone="error">{t('peopleLoadError')}</Banner> : guardianLinks.data.length === 0 ? <EmptyState icon="users" title={t('noGuardians')} /> :
        <ul className="divide-y divide-slate-100 rounded-xl border border-slate-200">{guardianLinks.data.map(item => <li key={item.guardian_id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <div className="flex min-w-0 flex-wrap items-center gap-3"><Avatar name={item.guardian_name} /><div className="min-w-0"><p className="font-semibold text-slate-900">{item.guardian_name}</p><p className="text-sm text-slate-600">{item.relationship}</p></div>
            {item.is_primary && <StatusBadge tone="present">{t('primaryGuardian')}</StatusBadge>}</div>
          {canManageLinks && <button type="button" disabled={busy} className={ui.danger} onClick={() => void unlink(item.guardian_id)}>{t('unlinkGuardian')}</button>}</li>)}</ul>}
      {canManageLinks && <form className="mt-5 grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 sm:grid-cols-2" onSubmit={e => void link(e)}>
        <label className={ui.label}>{t('search')}<input className={ui.field} placeholder={t('search')} value={guardianSearch} onChange={e => setGuardianSearch(e.target.value)} /></label>
        <label className={ui.label}>{t('chooseGuardian')}<select required className={ui.field} value={guardianID} onChange={e => setGuardianID(e.target.value)}><option value="">{t('chooseGuardian')}</option>{guardians.data?.items.map(g => <option key={g.id} value={g.id}>{g.name_en} · {g.phone}</option>)}</select></label>
        <label className={ui.label}>{t('relationshipLabel')}<input required maxLength={60} className={ui.field} value={relationship} onChange={e => setRelationship(e.target.value)} /></label>
        <label className="flex min-h-11 items-center gap-2 self-end text-sm font-semibold text-slate-700"><input type="checkbox" className="h-4 w-4 accent-[var(--brand-primary)]" checked={primary} onChange={e => setPrimary(e.target.checked)} />{t('primaryGuardian')}</label>
        <div className="sm:col-span-2"><button disabled={busy} className={ui.primary}>{t('linkGuardian')}</button></div></form>}
    </SectionCard>}
  </div>
}
