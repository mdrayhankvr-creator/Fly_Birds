import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getGuardians, getMyStudent, getSchoolBranding, getStudent, getStudentCard, getStudentGuardians, getStudentPhoto, linkStudentGuardian, saveStudentPhoto, unlinkStudentGuardian } from '../api/client'
import { usePeopleApi } from './peopleShared'
import { Icon } from '../components/icons'
import { StateScreen } from '../components/StateScreen'
import { Avatar, EmptyState, InfoRow, Notice as Banner, PageHeader, SectionCard, Skeleton, StatusBadge, ui, useNumber } from './attendanceUi'

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
  const student = useQuery({ queryKey: ['people', 'profile', mode, id], queryFn: () => authorized(token => mode === 'self' ? getMyStudent(token) : getStudent(token, id!)), enabled: mode === 'self' || !!id, retry: false })
  const studentID = student.data?.id
  const card = useQuery({ queryKey: ['people', 'card', studentID], queryFn: () => authorized(token => getStudentCard(token, studentID!)), enabled: !!studentID, retry: false })
  const photo = useQuery({ queryKey: ['people', 'photo', studentID], queryFn: () => authorized(token => getStudentPhoto(token, studentID!)), enabled: !!studentID && !!student.data?.has_photo, retry: false })
  const guardianLinks = useQuery({ queryKey: ['people', 'links', studentID], queryFn: () => authorized(token => getStudentGuardians(token, studentID!)), enabled: !!studentID && canReadGuardians, retry: false })
  const guardians = useQuery({ queryKey: ['people', 'guardian-options', guardianSearch], queryFn: () => authorized(token => getGuardians(token, new URLSearchParams({ search: guardianSearch, page_size: '100' }))), enabled: canManageLinks, retry: false })
  const branding = useQuery({ queryKey: ['school', 'branding'], queryFn: () => getSchoolBranding(), retry: false })
  useEffect(() => { if (!photo.data) { setPhotoUrl(''); return }; const url = URL.createObjectURL(photo.data); setPhotoUrl(url); return () => URL.revokeObjectURL(url) }, [photo.data])
  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!photoFile || !studentID) return; setBusy(true); setError(''); setMessage('')
    try { if (photoFile.size > 2097152) throw new Error('too large'); await authorized(token => saveStudentPhoto(token, studentID, photoFile)); await cache.invalidateQueries({ queryKey: ['people', 'photo', studentID] }); await cache.invalidateQueries({ queryKey: ['people', 'profile'] }); setMessage(t('peopleSaved')); setPhotoFile(null) }
    catch { setError(t('peopleSaveError')) } finally { setBusy(false) }
  }
  async function link(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!studentID) return; setBusy(true); setError(''); setMessage('')
    try { await authorized(token => linkStudentGuardian(token, studentID, guardianID, relationship, primary)); await cache.invalidateQueries({ queryKey: ['people', 'links', studentID] }); setGuardianID(''); setRelationship(''); setPrimary(false); setMessage(t('peopleSaved')) }
    catch (cause) { setError(cause instanceof ApiError && cause.status === 409 ? t('peopleDuplicate') : t('peopleSaveError')) } finally { setBusy(false) }
  }
  async function unlink(guardianId: string) {
    if (!studentID) return; setBusy(true); setError(''); setMessage('')
    try { await authorized(token => unlinkStudentGuardian(token, studentID, guardianId)); await cache.invalidateQueries({ queryKey: ['people', 'links', studentID] }); setMessage(t('peopleSaved')) }
    catch { setError(t('peopleSaveError')) } finally { setBusy(false) }
  }
  if (student.isPending) return <StateScreen kind="loading" message={t('academicLoading')} />
  if (student.isError) return <StateScreen kind="error" message={t('peopleLoadError')} />
  const value = student.data
  const name = i18n.language === 'bn' && value.name_bn ? value.name_bn : value.name_en
  const statusTone = value.status === 'active' ? 'present' : value.status === 'inactive' ? 'unmarked' : 'leave'
  return <div className="space-y-6">
    <PageHeader eyebrow={t(mode === 'self' ? 'myStudentProfile' : 'profileHeading')} title={name}
      description={`${t('admissionNo')}: ${value.admission_no} · ${value.class_name} · ${value.section_name}`}
      actions={mode === 'staff' ? <Link to="/admin/students" className={ui.secondary}><Icon name="chevronLeft" className="h-4 w-4" />{t('studentsHeading')}</Link> : undefined} />
    {error && <Banner tone="error">{error}</Banner>}{message && <Banner tone="success">{message}</Banner>}
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
      <SectionCard title={t('profileHeading')} id="student-profile">
        <div className="flex items-center gap-4">
          {photoUrl ? <img src={photoUrl} alt={name} className="h-20 w-20 shrink-0 rounded-2xl object-cover ring-1 ring-slate-200" /> : <span aria-hidden="true" className="grid h-20 w-20 shrink-0 place-items-center rounded-2xl bg-[var(--brand-soft)] text-2xl font-bold text-[var(--brand-ink)]">{[...value.name_en][0]}</span>}
          <div className="min-w-0"><p className="truncate text-lg font-bold text-slate-900">{name}</p>
            <div className="mt-1 flex flex-wrap items-center gap-2"><StatusBadge tone={statusTone}>{t(`${value.status}Student`)}</StatusBadge>{!photoUrl && <span className="text-xs text-slate-500">{t('noPhoto')}</span>}</div></div>
        </div>
        <dl className="mt-5 divide-y divide-slate-100 border-t border-slate-100">
          <InfoRow label={t('studentNameEN')}>{value.name_en}</InfoRow>
          <InfoRow label={t('studentNameBN')}>{value.name_bn || '—'}</InfoRow>
          <InfoRow label={t('dateOfBirth')}>{value.date_of_birth || '—'}</InfoRow>
          <InfoRow label={t('sessionLabel')}>{value.session_name}</InfoRow>
          <InfoRow label={t('classLabel')}>{value.class_name}</InfoRow>
          <InfoRow label={t('sectionLabel')}>{value.section_name}</InfoRow>
          <InfoRow label={t('rollNo')}>{value.roll_no == null ? '—' : num(value.roll_no)}</InfoRow>
        </dl>
        {canManagePhoto && <form className="mt-5 flex flex-col gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 sm:flex-row sm:items-end" onSubmit={e => void upload(e)}>
          <label className={`${ui.label} flex-1`}>{t('selectPhoto')}<input type="file" required accept="image/jpeg,image/png" className={ui.field} onChange={e => setPhotoFile(e.target.files?.[0] ?? null)} /></label>
          <button disabled={busy || !photoFile} className={ui.primary}>{t('uploadPhoto')}</button></form>}
      </SectionCard>
      <SectionCard title={t('studentCard')} id="student-card" actions={card.data ? <button type="button" className={ui.secondary} onClick={() => window.print()}>{t('printCard')}</button> : undefined}>
        {card.isPending ? <Skeleton rows={3} /> : card.isError ? <Banner tone="error">{t('peopleLoadError')}</Banner> :
          <div className="student-card mx-auto max-w-sm overflow-hidden rounded-2xl border border-slate-200 bg-white text-slate-900 shadow-sm">
            <div className="bg-[var(--brand-primary)] px-5 py-3 text-sm font-bold uppercase tracking-wide text-[var(--brand-on)]">{i18n.language === 'bn' ? branding.data?.name_bn : branding.data?.name_en}</div>
            <div className="flex items-start gap-4 p-5">
              <div className="min-w-0 flex-1"><h3 className="text-xl font-bold">{name}</h3><p className="mt-1 text-sm text-slate-600">{value.admission_no}</p><p className="text-sm text-slate-600">{value.class_name} · {value.section_name}</p>
                {photoUrl && <img src={photoUrl} alt="" className="mt-3 h-20 w-20 rounded-lg object-cover" />}</div>
              <img src={card.data.qr_data_url} alt={t('uxQrAlt')} className="h-32 w-32 shrink-0" /></div>
            <p className="border-t border-slate-100 px-5 py-2 text-[0.65rem] text-slate-500">{value.id}</p>
          </div>}
      </SectionCard>
    </div>
    {canReadGuardians && <SectionCard title={t('guardiansForStudent')} description={t('linkHelp')} id="student-guardians">
      {guardianLinks.isPending ? <Skeleton rows={2} /> : guardianLinks.isError ? <Banner tone="error">{t('peopleLoadError')}</Banner> : guardianLinks.data.length === 0 ? <EmptyState icon="users" title={t('noGuardians')} /> :
        <ul className="divide-y divide-slate-100 rounded-xl border border-slate-200">{guardianLinks.data.map(link => <li key={link.guardian_id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <div className="flex min-w-0 flex-wrap items-center gap-3"><Avatar name={link.guardian_name} /><div className="min-w-0"><p className="font-semibold text-slate-900">{link.guardian_name}</p><p className="text-sm text-slate-600">{link.relationship}</p></div>
            {link.is_primary && <StatusBadge tone="present">{t('primaryGuardian')}</StatusBadge>}</div>
          {canManageLinks && <button type="button" disabled={busy} className={ui.danger} onClick={() => void unlink(link.guardian_id)}>{t('unlinkGuardian')}</button>}</li>)}</ul>}
      {canManageLinks && <form className="mt-5 grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 sm:grid-cols-2" onSubmit={e => void link(e)}>
        <label className={ui.label}>{t('search')}<input className={ui.field} placeholder={t('search')} value={guardianSearch} onChange={e => setGuardianSearch(e.target.value)} /></label>
        <label className={ui.label}>{t('chooseGuardian')}<select required className={ui.field} value={guardianID} onChange={e => setGuardianID(e.target.value)}><option value="">{t('chooseGuardian')}</option>{guardians.data?.items.map(g => <option key={g.id} value={g.id}>{g.name_en} · {g.phone}</option>)}</select></label>
        <label className={ui.label}>{t('relationshipLabel')}<input required maxLength={60} className={ui.field} value={relationship} onChange={e => setRelationship(e.target.value)} /></label>
        <label className="flex min-h-11 items-center gap-2 self-end text-sm font-semibold text-slate-700"><input type="checkbox" className="h-4 w-4 accent-[var(--brand-primary)]" checked={primary} onChange={e => setPrimary(e.target.checked)} />{t('primaryGuardian')}</label>
        <div className="sm:col-span-2"><button disabled={busy} className={ui.primary}>{t('linkGuardian')}</button></div></form>}
    </SectionCard>}
  </div>
}
