import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getGuardians, getMyStudent, getSchoolBranding, getStudent, getStudentCard, getStudentGuardians, getStudentPhoto, linkStudentGuardian, saveStudentPhoto, unlinkStudentGuardian } from '../api/client'
import { peopleField, usePeopleApi } from './peopleShared'

export function StudentProfilePage({ mode, canManagePhoto, canManageLinks, canReadGuardians }: { mode: 'staff' | 'self' | 'child'; canManagePhoto: boolean; canManageLinks: boolean; canReadGuardians: boolean }) {
  const { id } = useParams()
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
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
  if (student.isPending) return <p role="status">{t('academicLoading')}</p>
  if (student.isError) return <p role="alert" className="text-rose-700">{t('peopleLoadError')}</p>
  const value = student.data
  const name = i18n.language === 'bn' && value.name_bn ? value.name_bn : value.name_en
  return <div className="space-y-6"><header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm"><p className="text-sm font-bold uppercase tracking-widest text-[var(--brand-ink)]">{t(mode === 'self' ? 'myStudentProfile' : 'profileHeading')}</p><h1 className="mt-2 text-3xl font-bold">{name}</h1><p className="mt-2 text-slate-600">{t('admissionNo')}: {value.admission_no}</p>{mode === 'staff' && <Link to="/admin/students" className="mt-4 inline-block font-semibold text-[var(--brand-ink)] underline">{t('studentsHeading')}</Link>}</header>
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{error}</p>}{message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-800">{message}</p>}
    <div className="grid gap-6 lg:grid-cols-2"><section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-bold">{t('profileHeading')}</h2><dl className="mt-4 grid grid-cols-[max-content_1fr] gap-x-5 gap-y-2 text-sm"><dt className="font-semibold">{t('studentNameEN')}</dt><dd>{value.name_en}</dd><dt className="font-semibold">{t('studentNameBN')}</dt><dd>{value.name_bn || '—'}</dd><dt className="font-semibold">{t('dateOfBirth')}</dt><dd>{value.date_of_birth || '—'}</dd><dt className="font-semibold">{t('studentStatus')}</dt><dd>{t(`${value.status}Student`)}</dd><dt className="font-semibold">{t('sessionLabel')}</dt><dd>{value.session_name}</dd><dt className="font-semibold">{t('classLabel')}</dt><dd>{value.class_name}</dd><dt className="font-semibold">{t('sectionLabel')}</dt><dd>{value.section_name}</dd><dt className="font-semibold">{t('rollNo')}</dt><dd>{value.roll_no ?? '—'}</dd></dl>
      <h3 className="mt-6 font-semibold">{t('photoLabel')}</h3>{photoUrl ? <img src={photoUrl} alt={name} className="mt-2 h-32 w-32 rounded-lg object-cover" /> : <p className="mt-2 text-sm text-slate-600">{t('noPhoto')}</p>}
      {canManagePhoto && <form className="mt-4 space-y-3" onSubmit={e => void upload(e)}><label className="block text-sm font-semibold">{t('selectPhoto')}<input type="file" required accept="image/jpeg,image/png" className={peopleField} onChange={e => setPhotoFile(e.target.files?.[0] ?? null)} /></label><button disabled={busy || !photoFile} className="rounded-lg bg-[var(--brand-primary)] px-4 py-2 font-semibold text-[var(--brand-on)] disabled:opacity-50">{t('uploadPhoto')}</button></form>}
    </section><section className="min-w-0 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-bold">{t('studentCard')}</h2>{card.isPending ? <p role="status">{t('academicLoading')}</p> : card.isError ? <p role="alert">{t('peopleLoadError')}</p> : <><div className="student-card mt-4 max-w-sm rounded-xl border-2 border-[var(--brand-primary)] bg-white p-5 text-slate-900"><p className="text-sm font-bold uppercase text-[var(--brand-ink)]">{i18n.language === 'bn' ? branding.data?.name_bn : branding.data?.name_en}</p><h3 className="mt-3 text-xl font-bold">{name}</h3><p className="text-sm">{value.admission_no} · {value.class_name} / {value.section_name}</p>{photoUrl && <img src={photoUrl} alt="" className="mt-3 h-20 w-20 rounded object-cover" />}<img src={card.data.qr_data_url} alt="Student ID QR code" className="mt-4 h-36 w-36" /><p className="mt-1 text-xs text-slate-500">{value.id}</p></div><button className="mt-4 rounded-lg border px-4 py-2 font-semibold text-[var(--brand-ink)]" onClick={() => window.print()}>{t('printCard')}</button></>}
    </section></div>
    {canReadGuardians && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-xl font-bold">{t('guardiansForStudent')}</h2><p className="mt-1 text-sm text-slate-600">{t('linkHelp')}</p>{guardianLinks.isPending ? <p role="status">{t('academicLoading')}</p> : guardianLinks.isError ? <p role="alert">{t('peopleLoadError')}</p> : guardianLinks.data.length === 0 ? <p className="mt-3 text-slate-600">{t('noGuardians')}</p> : <ul className="mt-3 space-y-2">{guardianLinks.data.map(link => <li key={link.guardian_id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3"><span>{link.guardian_name} · {link.relationship}{link.is_primary ? ` · ${t('primaryGuardian')}` : ''}</span>{canManageLinks && <button disabled={busy} className="font-semibold text-rose-700" onClick={() => void unlink(link.guardian_id)}>{t('unlinkGuardian')}</button>}</li>)}</ul>}
      {canManageLinks && <form className="mt-5 grid gap-3 sm:grid-cols-2" onSubmit={e => void link(e)}><label className="text-sm font-semibold">{t('chooseGuardian')}<input className={peopleField} placeholder={t('search')} value={guardianSearch} onChange={e => setGuardianSearch(e.target.value)} /><select required className={peopleField} value={guardianID} onChange={e => setGuardianID(e.target.value)}><option value="">{t('chooseGuardian')}</option>{guardians.data?.items.map(g => <option key={g.id} value={g.id}>{g.name_en} · {g.phone}</option>)}</select></label><label className="text-sm font-semibold">{t('relationshipLabel')}<input required maxLength={60} className={peopleField} value={relationship} onChange={e => setRelationship(e.target.value)} /></label><label className="flex items-center gap-2 text-sm font-semibold"><input type="checkbox" checked={primary} onChange={e => setPrimary(e.target.checked)} />{t('primaryGuardian')}</label><button disabled={busy} className="rounded-lg bg-[var(--brand-primary)] px-4 py-2 font-semibold text-[var(--brand-on)] disabled:opacity-50">{t('linkGuardian')}</button></form>}
    </section>}
  </div>
}
