import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
import { ApiError, getSalaryRecords } from '../api/client'
import { deleteStaffPhoto, getStaffPhoto, getStaffProfile, revealStaffNID, saveOwnStaffProfile, saveStaffOfficial, saveStaffPhoto, type Audience, type StaffProfile, type StaffVisibility } from '../api/staffProfile'
import { Icon } from '../components/icons'
import { usePeopleApi } from './peopleShared'
import { CardTitle, EmptyState, Notice, Skeleton, StatusBadge, ui, useNumber } from './attendanceUi'
import { Chip, Fact, ProfilePhoto, clock } from './profileUi'

// Staff profile (checkpoint 4). The server sends only what this viewer may
// see; a section appears when its data does. Teachers edit their own photo,
// qualifications and who sees their contact details; administrators edit the
// official details.

type Field = keyof StaffVisibility
const fields: Field[] = ['phone', 'email', 'qualifications']

export function StaffProfilePage() {
  const { t, i18n } = useTranslation()
  const { id = 'me' } = useParams()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const num = useNumber()
  const bn = i18n.language === 'bn'
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const profile = useQuery({ queryKey: ['staff-profile', id], queryFn: () => authorized(token => getStaffProfile(token, id)), retry: false })
  const p = profile.data
  const photo = useQuery({ queryKey: ['staff-profile', 'photo', p?.id, p?.photo_version], enabled: !!p?.photo_version, queryFn: () => authorized(token => getStaffPhoto(token, p!.id, p!.photo_version)), retry: false, staleTime: Infinity })
  const [photoUrl, setPhotoUrl] = useState('')
  useEffect(() => { if (!photo.data || !p?.photo_version) { setPhotoUrl(''); return }; const url = URL.createObjectURL(photo.data); setPhotoUrl(url); return () => URL.revokeObjectURL(url) }, [photo.data, p?.photo_version])
  const salaries = useQuery({ queryKey: ['staff-profile', 'salaries', p?.id], enabled: !!p?.can_see_salary, queryFn: () => authorized(token => getSalaryRecords(token, p!.id, new URLSearchParams({ page: '1', page_size: '3' }))), retry: false })

  async function run(task: () => Promise<StaffProfile | unknown>, done: string) {
    setBusy(true); setMessage(null)
    try { await task(); await cache.invalidateQueries({ queryKey: ['staff-profile'] }); setMessage({ tone: 'success', text: done }) }
    catch (cause) { setMessage({ tone: 'error', text: t(cause instanceof ApiError && cause.status === 400 ? 'c4ProfileInvalid' : cause instanceof ApiError && cause.status === 403 ? 'permissionDenied' : 'c4ProfileFailed') }) }
    finally { setBusy(false) }
  }

  if (profile.isPending) return <div className={`${ui.card} p-6`} role="status"><span className="sr-only">{t('academicLoading')}</span><Skeleton rows={6} /></div>
  if (profile.isError || !p) return <div className={`${ui.card} p-6`}><EmptyState icon="user" title={t(profile.error instanceof ApiError && profile.error.code === 'module_disabled' ? 'staffModuleDisabled' : profile.error instanceof ApiError && profile.error.status === 404 ? 'c4ProfileMissing' : 'c4ProfileLoadError')} /></div>
  const name = bn && p.name_bn ? p.name_bn : p.name_en
  const local = (en: string, b: string) => (bn && b ? b : en)
  const editPhotoFor = p.self ? undefined : p.id
  const canPhoto = p.self || p.can_edit_official

  return <div className="space-y-5">
    {message && <Notice tone={message.tone}>{message.text}</Notice>}
    <section className="ds-page-head ds-rise p-5 sm:p-7" aria-labelledby="staff-name">
      <div className="flex flex-col gap-5 sm:flex-row sm:items-center">
        <ProfilePhoto url={photoUrl} name={p.name_en} />
        <div className="min-w-0 flex-1">
          <p className="ds-eyebrow ds-ink">{t(p.kind === 'teacher' ? 'c4KindTeacher' : 'c4KindStaff')}</p>
          <h1 id="staff-name" className="mt-1 text-2xl font-extrabold tracking-tight text-slate-900 sm:text-3xl" style={{ overflowWrap: 'anywhere' }}>{name}</h1>
          {p.name_bn && !bn && <p lang="bn" className="text-lg font-semibold text-slate-700">{p.name_bn}</p>}
          {bn && p.name_en !== name && <p className="text-lg font-semibold text-slate-700">{p.name_en}</p>}
          <p className="mt-1 text-base font-semibold text-slate-700">{p.designation}</p>
          <div className="mt-3 flex flex-wrap gap-1.5">
            {!p.is_active && <StatusBadge tone="unmarked">{t('inactive')}</StatusBadge>}
            {p.teaching.map(item => <Chip key={`${item.subject_en}-${item.class_name}`}>{local(item.subject_en, item.subject_bn)} · {item.class_name}</Chip>)}
          </div>
        </div>
        {canPhoto && <PhotoActions hasPhoto={!!p.photo_version} busy={busy} onUpload={file => void run(() => authorized(token => saveStaffPhoto(token, file, editPhotoFor)), t('c4PhotoSaved'))} onRemove={() => void run(() => authorized(token => deleteStaffPhoto(token, editPhotoFor)), t('c4PhotoRemoved'))} />}
      </div>
    </section>

    <div className="grid min-w-0 gap-5 lg:grid-cols-2">
      <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-today">
        <CardTitle id="staff-today" icon="clock">{t('c4TodayRoutine')}</CardTitle>
        {p.today.length === 0 ? <div className="mt-3"><EmptyState icon="calendar" title={t('c4NoPeriodsToday')} /></div> :
          <ol className="mt-4 space-y-2">{p.today.map((period, i) => <li key={i} className="flex items-center gap-3 rounded-xl border border-slate-200 px-3 py-2.5">
            <span className="shrink-0 whitespace-nowrap font-mono text-xs font-bold tabular-nums text-slate-700 sm:text-sm">{clock(period.start_minute, bn)}–{clock(period.end_minute, bn)}</span>
            <span className="min-w-0 flex-1"><strong className="block text-slate-900">{local(period.subject_en, period.subject_bn)}</strong><span className="block text-xs text-slate-600">{period.class_name} · {period.section_name} · {period.room}</span></span>
          </li>)}</ol>}
      </section>

      {(p.phone !== undefined || p.email !== undefined || p.qualifications !== undefined) && <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-contact">
        <CardTitle id="staff-contact" icon="user">{t('c4ContactAndQualifications')}</CardTitle>
        <dl className="mt-4 grid gap-4 sm:grid-cols-2">
          {p.phone !== undefined && <Fact icon="phone" label={t('phoneLabel')}>{p.phone ? <a className={ui.link} href={`tel:${p.phone.replace(/[^+0-9]/g, '')}`}>{p.phone}</a> : <span className="font-normal text-slate-500">{t('profileNotProvided')}</span>}</Fact>}
          {p.email !== undefined && <Fact icon="mail" label={t('email')}>{p.email ? <a className={ui.link} href={`mailto:${p.email}`}>{p.email}</a> : <span className="font-normal text-slate-500">{t('profileNotProvided')}</span>}</Fact>}
        </dl>
        {p.qualifications !== undefined && <div className="mt-4"><p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('c4Qualifications')}</p>
          {p.qualifications.length ? <ul className="mt-2 flex flex-wrap gap-1.5">{p.qualifications.map(q => <li key={q}><Chip>{q}</Chip></li>)}</ul> : <p className="mt-1 text-sm text-slate-500">{t('profileNotProvided')}</p>}</div>}
      </section>}

      {(p.joined_on !== undefined || p.mpo_index !== undefined || p.nid_masked !== undefined) && <OfficialCard profile={p} busy={busy} onSave={input => run(() => authorized(token => saveStaffOfficial(token, p.id, input)), t('c4OfficialSaved'))} />}

      {p.attendance && <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-attendance">
        <CardTitle id="staff-attendance" icon="check">{t('c4AttendanceAndLeave')}</CardTitle>
        <dl className="mt-4 grid grid-cols-3 gap-2">
          {(['present', 'absent', 'leave'] as const).map(k => <div key={k} className="ds-stat px-3 py-2.5"><dt className="text-xs font-semibold text-slate-600">{t(`c4Att_${k}`)}</dt><dd className="mt-0.5 text-xl font-extrabold tabular-nums text-slate-900">{num(p.attendance!.month[k])}</dd><dd className="text-xs text-slate-500">{t('c4ThisYearN', { n: num(p.attendance!.session[k]) })}</dd></div>)}
        </dl>
        <p className="mt-1 text-xs text-slate-500">{t('c4ThisMonth')}</p>
        {p.leaves && p.leaves.length > 0 && <ul className="mt-4 divide-y divide-slate-100 rounded-xl border border-slate-200">{p.leaves.map(l => <li key={l.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-sm"><span>{l.starts_on} → {l.ends_on} · {t(`leave_${l.leave_type}`, { defaultValue: l.leave_type })}</span><StatusBadge tone={l.status === 'approved' ? 'present' : l.status === 'rejected' ? 'absent' : 'unmarked'}>{t(`leaveStatus_${l.status}`, { defaultValue: l.status })}</StatusBadge></li>)}</ul>}
      </section>}

      {p.can_see_salary && <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-salary">
        <CardTitle id="staff-salary" icon="money">{t('c4Salary')}</CardTitle>
        <p className="mt-1 text-xs text-slate-500">{t('c4SalaryPrivate')}</p>
        {salaries.isPending ? <Skeleton rows={2} /> : !salaries.data?.items.length ? <div className="mt-3"><EmptyState icon="money" title={t('c4NoSalary')} /></div> :
          <ul className="mt-3 divide-y divide-slate-100 rounded-xl border border-slate-200">{salaries.data.items.map(s => <li key={s.id} className="flex flex-wrap justify-between gap-2 px-3 py-2 text-sm"><span>{t('c4SalaryFrom', { date: s.effective_on })}</span><span className="font-semibold tabular-nums">৳ {(Number(s.basic_bdt) + Number(s.allowance_bdt) - Number(s.deduction_bdt)).toFixed(2)}</span></li>)}</ul>}
        <Link to="/admin/staff" className={`${ui.link} mt-3 inline-block text-sm`}>{t('c4ManageInStaff')}</Link>
      </section>}
    </div>

    {p.self && p.visibility && <OwnSettings key={JSON.stringify([p.qualifications, p.visibility])} qualifications={p.qualifications ?? []} visibility={p.visibility} busy={busy} onSave={input => run(() => authorized(token => saveOwnStaffProfile(token, input)), t('c4OwnSaved'))} />}
  </div>
}

function PhotoActions({ hasPhoto, busy, onUpload, onRemove }: { hasPhoto: boolean; busy: boolean; onUpload: (file: File) => void; onRemove: () => void }) {
  const { t } = useTranslation()
  return <div className="flex flex-wrap gap-2 sm:flex-col sm:items-stretch">
    <label className={`${ui.secondary} ${busy ? 'pointer-events-none opacity-50' : ''}`}><Icon name="camera" className="h-4 w-4" />{t(hasPhoto ? 'c4ChangePhoto' : 'c4AddPhoto')}
      <input type="file" accept="image/jpeg,image/png" className="sr-only" disabled={busy} onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) onUpload(f) }} /></label>
    {hasPhoto && <button type="button" className={ui.quiet} disabled={busy} onClick={onRemove}>{t('c4RemovePhoto')}</button>}
  </div>
}

function OfficialCard({ profile: p, busy, onSave }: { profile: StaffProfile; busy: boolean; onSave: (input: { email?: string; mpo_index?: string; nid?: string }) => Promise<unknown> }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const [shown, setShown] = useState('')
  const [revealing, setRevealing] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ email: p.email ?? '', mpo_index: p.mpo_index ?? '', nid: '' })
  // A revealed NID hides itself again after a short while.
  useEffect(() => { if (!shown) return; const timer = window.setTimeout(() => setShown(''), 30_000); return () => window.clearTimeout(timer) }, [shown])
  async function reveal() {
    setRevealing(true)
    try { setShown((await authorized(token => revealStaffNID(token, p.id))).nid) } catch { setShown('') } finally { setRevealing(false) }
  }
  async function save(event: FormEvent) {
    event.preventDefault()
    const input: { email?: string; mpo_index?: string; nid?: string } = { email: form.email.trim(), mpo_index: form.mpo_index.trim() }
    if (form.nid.trim()) input.nid = form.nid.replace(/\s/g, '')
    await onSave(input); setEditing(false); setForm(f => ({ ...f, nid: '' })); setShown('')
  }
  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-official">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <CardTitle id="staff-official" icon="shield">{t('c4Official')}</CardTitle>
      {p.can_edit_official && !editing && <button type="button" className={`${ui.secondary} btn-sm`} onClick={() => setEditing(true)}>{t('edit')}</button>}
    </div>
    {editing ? <form className="mt-4 grid gap-3 sm:grid-cols-2" onSubmit={e => void save(e)}>
      <label className={`${ui.label} sm:col-span-2`}>{t('email')}<input type="email" maxLength={254} className={ui.field} value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} /></label>
      <label className={ui.label}>{t('c4MPO')}<input maxLength={40} className={ui.field} value={form.mpo_index} onChange={e => setForm({ ...form, mpo_index: e.target.value })} /></label>
      <label className={ui.label}>{t('c4NID')}<input inputMode="numeric" autoComplete="off" pattern="[0-9 ]{10,20}" className={`${ui.field} font-mono`} placeholder={t('c4NIDKeep')} value={form.nid} onChange={e => setForm({ ...form, nid: e.target.value })} /></label>
      <p className="text-xs text-slate-500 sm:col-span-2">{t('c4NIDRule')}</p>
      <div className="flex flex-wrap gap-2 sm:col-span-2"><button disabled={busy} className={ui.primary}>{t('saveChanges')}</button><button type="button" className={ui.secondary} onClick={() => setEditing(false)}>{t('cancel')}</button></div>
    </form> : <dl className="mt-4 grid gap-4 sm:grid-cols-2">
      {p.joined_on !== undefined && <Fact icon="calendar" label={t('c4JoinedOn')}>{p.joined_on}</Fact>}
      {p.mpo_index !== undefined && <Fact label={t('c4MPO')}>{p.mpo_index || <span className="font-normal text-slate-500">{t('profileNotProvided')}</span>}</Fact>}
      {p.nid_masked !== undefined && <div className="min-w-0 sm:col-span-2">
        <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('c4NID')}</dt>
        <dd className="mt-1 flex flex-wrap items-center gap-2">
          <span className="font-mono text-lg font-bold tracking-wider text-slate-900" data-nid>{shown || p.nid_masked || <span className="font-sans text-sm font-normal text-slate-500">{t('profileNotProvided')}</span>}</span>
          {p.can_reveal_nid && (shown ? <button type="button" className={`${ui.quiet} btn-sm`} onClick={() => setShown('')}><Icon name="eye" className="h-4 w-4" />{t('c4HideNID')}</button>
            : <button type="button" className={`${ui.secondary} btn-sm`} disabled={revealing} onClick={() => void reveal()}><Icon name="eye" className="h-4 w-4" />{t('c4ShowNID')}</button>)}
        </dd>
        <p className="mt-1 text-xs text-slate-500">{t('c4NIDPrivate')}</p>
      </div>}
    </dl>}
  </section>
}

function OwnSettings({ qualifications, visibility, busy, onSave }: { qualifications: string[]; visibility: StaffVisibility; busy: boolean; onSave: (input: { qualifications: string[]; visibility: StaffVisibility }) => Promise<unknown> }) {
  const { t } = useTranslation()
  const [list, setList] = useState(qualifications.length ? qualifications : [''])
  const [share, setShare] = useState<StaffVisibility>(visibility)
  const set = (field: Field, audience: keyof Audience, value: boolean) => setShare(v => ({ ...v, [field]: { ...v[field], [audience]: value } }))
  return <form className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-own" onSubmit={e => { e.preventDefault(); void onSave({ qualifications: list.map(q => q.trim()).filter(Boolean), visibility: share }) }}>
    <CardTitle id="staff-own" icon="settings">{t('c4OwnSettings')}</CardTitle>
    <div className="mt-4 grid gap-6 lg:grid-cols-2">
      <fieldset className="min-w-0">
        <legend className="text-sm font-bold text-slate-900">{t('c4Qualifications')}</legend>
        <ul className="mt-2 space-y-2">{list.map((q, i) => <li key={i} className="flex gap-2">
          <label className="min-w-0 flex-1"><span className="sr-only">{t('c4QualificationN', { n: i + 1 })}</span><input maxLength={160} className={`${ui.field} mt-0`} placeholder={t('c4QualificationHint')} value={q} onChange={e => setList(l => l.map((v, j) => j === i ? e.target.value : v))} /></label>
          <button type="button" className={`${ui.quiet} shrink-0 text-rose-700`} aria-label={t('c4RemoveQualification', { n: i + 1 })} onClick={() => setList(l => l.length > 1 ? l.filter((_, j) => j !== i) : [''])}><Icon name="close" className="h-4 w-4" /></button>
        </li>)}</ul>
        {list.length < 20 && <button type="button" className={`${ui.secondary} btn-sm mt-2`} onClick={() => setList(l => [...l, ''])}><Icon name="plus" className="h-4 w-4" />{t('c4AddQualification')}</button>}
      </fieldset>
      <fieldset className="min-w-0">
        <legend className="text-sm font-bold text-slate-900">{t('c4WhoSees')}</legend>
        <p className="mt-1 text-xs text-slate-500">{t('c4LeadersAlways')}</p>
        <table className="mt-2 w-full text-sm">
          <thead><tr className="text-left text-xs uppercase tracking-wide text-slate-500"><th className="py-2 font-semibold"><span className="sr-only">{t('c4Detail')}</span></th><th className="py-2 text-center font-semibold">{t('c4AudienceStaff')}</th><th className="py-2 text-center font-semibold">{t('c4AudienceFamilies')}</th></tr></thead>
          <tbody>{fields.map(f => <tr key={f} className="border-t border-slate-100"><th scope="row" className="py-2.5 text-left font-semibold text-slate-800">{t(`c4Field_${f}`)}</th>
            {(['staff', 'families'] as const).map(a => <td key={a} className="py-1 text-center"><label className="inline-grid h-11 w-11 place-items-center rounded-lg hover:bg-slate-100"><span className="sr-only">{t(`c4Field_${f}`)} · {t(a === 'staff' ? 'c4AudienceStaff' : 'c4AudienceFamilies')}</span><input type="checkbox" className="h-5 w-5 accent-[var(--brand-primary)]" checked={share[f][a]} onChange={e => set(f, a, e.target.checked)} /></label></td>)}</tr>)}
            <tr className="border-t border-slate-100"><th scope="row" className="py-2.5 text-left font-semibold text-slate-800">{t('c4MPO')} · {t('c4NID')}</th><td colSpan={2} className="py-2.5 text-center text-xs text-slate-500">{t('c4NeverShared')}</td></tr>
          </tbody>
        </table>
      </fieldset>
    </div>
    <button disabled={busy} className={`${ui.primary} mt-5`}><Icon name="check" className="h-4 w-4" />{t('saveChanges')}</button>
  </form>
}
