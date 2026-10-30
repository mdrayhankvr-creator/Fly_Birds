import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError, bloodGroups, getAcademicClasses, getAcademicSections, getAcademicSessions, type Gender } from '../api/client'
import { admitStudent, getAdmissionDefaults, getGuardianMatches, type AdmissionGuardian, type AdmissionResult, type GuardianMatch } from '../api/admission'
import { Icon } from '../components/icons'
import { peopleField, usePeopleApi } from './peopleShared'
import { CardTitle, Notice, PageHeader, ui } from './attendanceUi'
import { AdmissionSlip } from './AdmissionSlip'
import { admissionLoginLocalPart, newAdmissionKey, readStoredPlacement, storePlacement, todayInDhaka, type Placement } from './admissionModel'

// One-page admission (checkpoint 3): placement, the student, up to two
// guardians and an optional login on one screen, saved in one server
// transaction. Smart defaults keep the required typing to the minimum.

type Relation = 'Father' | 'Mother' | 'other'
type GuardianDraft = { relation: Relation; other: string; name_en: string; name_bn: string; phone: string; email: string; primary: boolean; existing: GuardianMatch | null; matches: GuardianMatch[]; confirmNew: boolean }
type Details = { gender: Gender; blood_group: string; student_code: string; admission_date: string; father_name_en: string; father_name_bn: string; mother_name_en: string; mother_name_bn: string; present_address: string; permanent_address: string }
type FieldErrors = Partial<Record<'admission' | 'roll' | 'login' | 'guardian0' | 'guardian1' | 'form', string>>

const blankGuardian = (relation: Relation, primary: boolean): GuardianDraft => ({ relation, other: '', name_en: '', name_bn: '', phone: '', email: '', primary, existing: null, matches: [], confirmNew: false })
const blankDetails = (): Details => ({ gender: '', blood_group: '', student_code: '', admission_date: todayInDhaka(), father_name_en: '', father_name_bn: '', mother_name_en: '', mother_name_bn: '', present_address: '', permanent_address: '' })
const digits = (v: string) => v.replace(/[০-৯]/g, d => String('০১২৩৪৫৬৭৮৯'.indexOf(d))).replace(/\D/g, '')

export function AdmissionPage({ canManageGuardians, canCreateLogin }: { canManageGuardians: boolean; canCreateLogin: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const stored = useRef<Placement | null>(readStoredPlacement())
  const [placement, setPlacement] = useState<Placement>(stored.current ?? { session_id: '', class_id: '', section_id: '' })
  const [roll, setRoll] = useState({ value: '', touched: false })
  const [admissionNo, setAdmissionNo] = useState({ value: '', touched: false })
  const [nameEN, setNameEN] = useState('')
  const [nameBN, setNameBN] = useState('')
  const [dob, setDob] = useState('')
  const [details, setDetails] = useState<Details>(blankDetails)
  const [guardians, setGuardians] = useState<GuardianDraft[]>(() => canManageGuardians ? [blankGuardian('Father', true)] : [])
  const [login, setLogin] = useState({ create: canCreateLogin, email: '', touched: false })
  const [errors, setErrors] = useState<FieldErrors>({})
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<AdmissionResult | null>(null)
  // One key per admission: kept while the same form is retried, renewed for the next student.
  const [attempt, setAttempt] = useState(newAdmissionKey)
  const top = useRef<HTMLDivElement>(null)

  const sessions = useQuery({ queryKey: ['people', 'admission', 'sessions'], queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const classes = useQuery({ queryKey: ['people', 'admission', 'classes'], queryFn: () => authorized(token => getAcademicClasses(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const sections = useQuery({ queryKey: ['people', 'admission', 'sections', placement.session_id], enabled: !!placement.session_id, queryFn: () => authorized(token => getAcademicSections(token, new URLSearchParams({ page: '1', page_size: '100', session_id: placement.session_id }))), retry: false })
  const defaults = useQuery({ queryKey: ['people', 'admission', 'defaults', placement.section_id], queryFn: () => authorized(token => getAdmissionDefaults(token, placement.section_id)), retry: false })
  const classSections = (sections.data?.items ?? []).filter(s => s.class_id === placement.class_id && s.is_active !== false)

  // The session used last, else the active one.
  useEffect(() => {
    const items = sessions.data?.items
    if (!items?.length || items.some(s => s.id === placement.session_id)) return
    const next = items.find(s => s.is_active) ?? items[0]
    // Keep a class picked while the sessions were loading.
    setPlacement(p => ({ ...p, session_id: next.id, section_id: '' }))
  }, [sessions.data, placement.session_id])
  // A class with one section picks it.
  useEffect(() => {
    if (!sections.data) return
    if (placement.section_id && !classSections.some(s => s.id === placement.section_id)) setPlacement(p => ({ ...p, section_id: '' }))
    else if (!placement.section_id && classSections.length === 1) setPlacement(p => ({ ...p, section_id: classSections[0].id }))
  }, [sections.data, placement.class_id, placement.section_id]) // eslint-disable-line react-hooks/exhaustive-deps
  // Suggestions fill fields the user has not typed in.
  useEffect(() => {
    if (!defaults.data) return
    if (!roll.touched) setRoll({ value: placement.section_id ? String(defaults.data.next_roll) : '', touched: false })
    if (!admissionNo.touched) setAdmissionNo({ value: defaults.data.admission_no, touched: false })
  }, [defaults.data]) // eslint-disable-line react-hooks/exhaustive-deps
  const domain = defaults.data?.login_domain ?? ''
  const suggestedLogin = admissionNo.value && domain ? `${admissionLoginLocalPart(admissionNo.value)}@${domain}` : ''
  const loginEmail = login.touched ? login.email : suggestedLogin

  function setGuardian(index: number, patch: Partial<GuardianDraft>) {
    setGuardians(list => list.map((g, i) => i === index ? { ...g, ...patch } : patch.primary ? { ...g, primary: false } : g))
    setErrors(e => ({ ...e, [`guardian${index}`]: undefined }))
  }
  function relationship(g: GuardianDraft) { return g.relation === 'other' ? g.other.trim() : g.relation }

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy) return
    setErrors({})
    const list: AdmissionGuardian[] = guardians.filter(g => g.existing || g.name_en.trim()).map(g => g.existing
      ? { guardian_id: g.existing.id, relationship: relationship(g), is_primary: g.primary }
      : { new: { name_en: g.name_en.trim(), name_bn: g.name_bn.trim(), phone: g.phone.trim(), email: g.email.trim(), address: '', user_id: '' }, relationship: relationship(g), is_primary: g.primary, confirm_new: g.confirmNew })
    // Parents' names on the profile come from the guardians when left empty.
    const father = guardians.find(g => g.relation === 'Father' && (g.existing || g.name_en.trim()))
    const mother = guardians.find(g => g.relation === 'Mother' && (g.existing || g.name_en.trim()))
    const parent = (g: GuardianDraft | undefined, lang: 'en' | 'bn') => !g ? '' : g.existing ? (lang === 'en' ? g.existing.name_en : g.existing.name_bn) : (lang === 'en' ? g.name_en.trim() : g.name_bn.trim())
    setBusy(true)
    try {
      const admitted = await authorized(token => admitStudent(token, {
        idempotency_key: attempt,
        student: {
          admission_no: admissionNo.value.trim(), name_en: nameEN.trim(), name_bn: nameBN.trim(), date_of_birth: dob, status: 'active',
          session_id: placement.session_id, class_id: placement.class_id, section_id: placement.section_id, roll_no: roll.value ? Number(digits(roll.value)) : null, user_id: '',
          ...details,
          father_name_en: details.father_name_en.trim() || parent(father, 'en'), father_name_bn: details.father_name_bn.trim() || parent(father, 'bn'),
          mother_name_en: details.mother_name_en.trim() || parent(mother, 'en'), mother_name_bn: details.mother_name_bn.trim() || parent(mother, 'bn'),
        },
        account: { create: canCreateLogin && login.create, email: loginEmail.trim() },
        guardians: list,
      }))
      storePlacement(placement)
      setResult(admitted)
      await cache.invalidateQueries({ queryKey: ['people'] })
      top.current?.scrollIntoView({ block: 'start' })
    } catch (cause) {
      const code = cause instanceof ApiError ? cause.code : ''
      if (code === 'guardian_exists') {
        const index = Number(cause instanceof ApiError ? cause.details?.index : -1)
        const matches = (cause instanceof ApiError ? cause.details?.matches : []) as GuardianMatch[]
        const drafts = guardians.map((g, i) => ({ g, i })).filter(({ g }) => g.existing || g.name_en.trim())
        const target = drafts[index]?.i ?? 0
        setGuardians(gs => gs.map((g, i) => i === target ? { ...g, matches } : g))
        setErrors({ [`guardian${target}`]: t('c3GuardianExists') })
      } else if (code === 'idempotency_key_reused') { setAttempt(newAdmissionKey()); setErrors({ form: t('c3ErrKeyReused') }) }
      else if (code === 'login_email_taken') setErrors({ login: t('c3ErrLoginTaken') })
      else if (code === 'admission_no_taken') { setErrors({ admission: t('c3ErrAdmissionTaken') }); void defaults.refetch() }
      else if (code === 'roll_taken') { setErrors({ roll: t('c3ErrRollTaken') }); setRoll(r => ({ ...r, touched: false })); void defaults.refetch() }
      else setErrors({ form: t(code === 'forbidden' ? 'c3ErrForbidden' : code === 'invalid_request' ? 'c3ErrInvalid' : 'peopleSaveError') })
    } finally { setBusy(false) }
  }
  function another() {
    setResult(null); setAttempt(newAdmissionKey()); setNameEN(''); setNameBN(''); setDob(''); setDetails(blankDetails()); setErrors({})
    setGuardians(canManageGuardians ? [blankGuardian('Father', true)] : []); setLogin({ create: canCreateLogin, email: '', touched: false })
    // Start from the current suggestions; the refetch moves them on if they changed.
    setRoll({ value: placement.section_id && defaults.data ? String(defaults.data.next_roll) : '', touched: false })
    setAdmissionNo({ value: defaults.data?.admission_no ?? '', touched: false })
    void defaults.refetch()
    top.current?.scrollIntoView({ block: 'start' })
  }

  const header = <PageHeader eyebrow={t('m12Group_people')} title={t('c3AdmitTitle')} description={t('c3AdmitIntro')} actions={<Link to="/admin/students" className={ui.secondary}><Icon name="chevronLeft" className="h-4 w-4" />{t('c3BackToStudents')}</Link>} />
  if (result) return <div ref={top} className="space-y-5">
    {header}
    <Notice tone={result.replayed ? 'info' : 'success'}>{t(result.replayed ? 'c3AlreadyAdmitted' : 'c3Admitted', { name: result.student.name_en })}</Notice>
    <AdmissionSlip result={result} />
    <div className="flex flex-wrap gap-2 print:hidden">
      <button type="button" className={ui.primary} onClick={() => window.print()}><Icon name="print" className="h-4 w-4" />{t('c3PrintSlip')}</button>
      <button type="button" className={ui.secondary} onClick={another}><Icon name="plus" className="h-4 w-4" />{t('c3AdmitAnother')}</button>
      <Link className={ui.secondary} to={`/admin/students/${result.student.id}`}>{t('viewProfile')}</Link>
    </div>
  </div>

  const loading = sessions.isPending || classes.isPending
  return <div ref={top} className="space-y-5">
    {header}
    {(sessions.isError || classes.isError) && <Notice tone="error">{t('peopleLoadError')}</Notice>}
    {errors.form && <Notice tone="error">{errors.form}</Notice>}
    <form onSubmit={e => void submit(e)} className="space-y-5" aria-busy={busy} noValidate={false}>
      <Step n={1} icon="school" title={t('c3StepPlacement')}>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <label className={ui.label}>{t('sessionLabel')}<select required className={peopleField} value={placement.session_id} disabled={loading} onChange={e => setPlacement({ session_id: e.target.value, class_id: '', section_id: '' })}>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}{s.is_active ? ` · ${t('attnActive')}` : ''}</option>)}</select></label>
          <label className={ui.label}>{t('classLabel')}<select required className={peopleField} value={placement.class_id} onChange={e => { setPlacement({ ...placement, class_id: e.target.value, section_id: '' }); setRoll({ value: '', touched: false }) }}><option value="">{t('selectClass')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
          <label className={ui.label}>{t('sectionLabel')}<select required className={peopleField} value={placement.section_id} disabled={!placement.class_id} onChange={e => { setPlacement({ ...placement, section_id: e.target.value }); setRoll({ value: '', touched: false }) }}><option value="">{t('selectSection')}</option>{classSections.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <Field label={t('rollNo')} error={errors.roll} hint={!roll.touched && roll.value ? t('c3Suggested') : undefined}>
            <input id="admit-roll" inputMode="numeric" className={peopleField} value={roll.value} aria-invalid={!!errors.roll} onChange={e => { setRoll({ value: e.target.value, touched: true }); setErrors(x => ({ ...x, roll: undefined })) }} />
          </Field>
        </div>
        {placement.class_id && sections.data && classSections.length === 0 && <p className="mt-3 text-sm text-amber-800">{t('c3NoSections')}</p>}
      </Step>

      <Step n={2} icon="user" title={t('c3StepStudent')}>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Field label={t('admissionNo')} error={errors.admission} hint={!admissionNo.touched && admissionNo.value ? t('c3Suggested') : undefined}>
            <input id="admit-admission" required maxLength={64} className={peopleField} value={admissionNo.value} aria-invalid={!!errors.admission} onChange={e => { setAdmissionNo({ value: e.target.value, touched: true }); setErrors(x => ({ ...x, admission: undefined })) }} />
          </Field>
          <label className={ui.label}>{t('studentNameEN')}<input required maxLength={160} autoComplete="off" className={peopleField} value={nameEN} onChange={e => setNameEN(e.target.value)} /></label>
          <label className={ui.label}>{t('studentNameBN')}<input maxLength={160} lang="bn" autoComplete="off" className={peopleField} value={nameBN} onChange={e => setNameBN(e.target.value)} /></label>
          <label className={ui.label}>{t('dateOfBirth')}<input type="date" max={todayInDhaka()} className={peopleField} value={dob} onChange={e => setDob(e.target.value)} /></label>
        </div>
        <details className="group mt-4 rounded-xl border border-slate-200 bg-slate-50">
          <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-2 px-3 text-sm font-semibold text-slate-700 [&::-webkit-details-marker]:hidden">{t('c3MoreDetails')}<Icon name="chevronDown" className="h-4 w-4 transition group-open:rotate-180" /></summary>
          <div className="grid gap-3 border-t border-slate-200 p-3 sm:grid-cols-2 lg:grid-cols-4">
            <label className={ui.label}>{t('profileGender')}<select className={peopleField} value={details.gender} onChange={e => setDetails({ ...details, gender: e.target.value as Gender })}><option value="">{t('profileNotProvided')}</option>{(['female', 'male', 'other'] as const).map(g => <option key={g} value={g}>{t(`gender_${g}`)}</option>)}</select></label>
            <label className={ui.label}>{t('profileBloodGroup')}<select className={peopleField} value={details.blood_group} onChange={e => setDetails({ ...details, blood_group: e.target.value })}><option value="">{t('profileNotProvided')}</option>{bloodGroups.map(g => <option key={g} value={g}>{g}</option>)}</select></label>
            <label className={ui.label}>{t('idCardStudentId')}<input maxLength={64} className={peopleField} value={details.student_code} onChange={e => setDetails({ ...details, student_code: e.target.value })} /></label>
            <label className={ui.label}>{t('profileAdmissionDate')}<input type="date" className={peopleField} value={details.admission_date} onChange={e => setDetails({ ...details, admission_date: e.target.value })} /></label>
            <label className={ui.label}>{t('profileFatherEN')}<input maxLength={160} className={peopleField} value={details.father_name_en} onChange={e => setDetails({ ...details, father_name_en: e.target.value })} /></label>
            <label className={ui.label}>{t('profileFatherBN')}<input maxLength={160} lang="bn" className={peopleField} value={details.father_name_bn} onChange={e => setDetails({ ...details, father_name_bn: e.target.value })} /></label>
            <label className={ui.label}>{t('profileMotherEN')}<input maxLength={160} className={peopleField} value={details.mother_name_en} onChange={e => setDetails({ ...details, mother_name_en: e.target.value })} /></label>
            <label className={ui.label}>{t('profileMotherBN')}<input maxLength={160} lang="bn" className={peopleField} value={details.mother_name_bn} onChange={e => setDetails({ ...details, mother_name_bn: e.target.value })} /></label>
            <label className={`${ui.label} sm:col-span-2`}>{t('profilePresentAddress')}<textarea rows={2} maxLength={500} className={peopleField} value={details.present_address} onChange={e => setDetails({ ...details, present_address: e.target.value })} /></label>
            <label className={`${ui.label} sm:col-span-2`}>{t('profilePermanentAddress')}<textarea rows={2} maxLength={500} className={peopleField} value={details.permanent_address} onChange={e => setDetails({ ...details, permanent_address: e.target.value })} /></label>
            <p className="text-xs text-slate-500 sm:col-span-2 lg:col-span-4">{t('c3ParentsFromGuardians')}</p>
          </div>
        </details>
      </Step>

      {canManageGuardians && <Step n={3} icon="users" title={t('c3StepGuardians')} help={t('c3GuardiansHelp')}>
        <div className="grid gap-4 xl:grid-cols-2">
          {guardians.map((g, i) => <GuardianCard key={i} index={i} draft={g} error={errors[`guardian${i}` as 'guardian0']} onChange={patch => setGuardian(i, patch)} onRemove={i > 0 ? () => setGuardians(list => list.filter((_, j) => j !== i).map((x, j) => j === 0 && !list.some((y, k) => k !== i && y.primary) ? { ...x, primary: true } : x)) : undefined} />)}
        </div>
        {guardians.length < 2 && <button type="button" className={`${ui.secondary} mt-4`} onClick={() => setGuardians(list => [...list, blankGuardian(list[0]?.relation === 'Mother' ? 'Father' : 'Mother', list.length === 0)])}><Icon name="plus" className="h-4 w-4" />{t(guardians.length ? 'c3AddSecondGuardian' : 'c3AddGuardian')}</button>}
      </Step>}

      {canCreateLogin && <Step n={canManageGuardians ? 4 : 3} icon="lock" title={t('c3StepLogin')}>
        <label className="flex min-h-11 items-center gap-3 text-sm font-semibold text-slate-800"><input type="checkbox" className="h-5 w-5 accent-[var(--brand-primary)]" checked={login.create} onChange={e => setLogin({ ...login, create: e.target.checked })} />{t('c3CreateLogin')}</label>
        {login.create && <div className="mt-2 grid gap-3 sm:grid-cols-2">
          <Field label={t('c3LoginEmail')} error={errors.login} hint={!login.touched && loginEmail ? t('c3Suggested') : undefined}>
            <input id="admit-login" type="email" required maxLength={254} autoComplete="off" className={peopleField} value={loginEmail} aria-invalid={!!errors.login} onChange={e => { setLogin({ ...login, email: e.target.value, touched: true }); setErrors(x => ({ ...x, login: undefined })) }} />
          </Field>
          <p className="ds-sunken self-end px-3 py-2.5 text-sm text-slate-700"><Icon name="info" className="mr-1.5 inline h-4 w-4 align-[-3px]" />{t('c3PasswordNote')}</p>
        </div>}
      </Step>}

      <div className="ds-action-bar flex flex-col gap-3 rounded-2xl border border-slate-200 bg-white/95 p-3 shadow-lg backdrop-blur sm:flex-row sm:items-center sm:justify-between sm:p-4">
        <p className="text-sm text-slate-600">{t('c3SaveNote')}</p>
        <button disabled={busy || !placement.section_id} className={`${ui.primary} min-h-12 px-6 text-base sm:min-w-56`}><Icon name="check" className="h-5 w-5" />{busy ? t('c3Saving') : t('c3Admit')}</button>
      </div>
    </form>
  </div>
}

function Step({ n, icon, title, help, children }: { n: number; icon: 'school' | 'user' | 'users' | 'lock'; title: string; help?: string; children: ReactNode }) {
  const { t } = useTranslation()
  const id = `admit-step-${n}`
  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby={id}>
    <div className="flex flex-wrap items-baseline justify-between gap-2"><CardTitle id={id} icon={icon}><span className="sr-only">{t('c3StepN', { n })} </span>{title}</CardTitle>{help && <p className="text-sm text-slate-600">{help}</p>}</div>
    <div className="mt-4">{children}</div>
  </section>
}

function Field({ label, error, hint, className = '', children }: { label: string; error?: string; hint?: string; className?: string; children: ReactNode }) {
  return <label className={`${ui.label} ${className}`}>
    <span className="flex items-center justify-between gap-2">{label}{hint && <span className="rounded-full bg-[var(--brand-soft)] px-2 py-0.5 text-[10px] font-bold normal-case tracking-normal text-[var(--brand-ink)]">{hint}</span>}</span>
    {children}
    {error && <span role="alert" className="mt-1 block text-xs font-semibold normal-case tracking-normal text-rose-700">{error}</span>}
  </label>
}

// GuardianCard: one guardian, new or already on record. Typing a phone or
// email looks for guardians on record with it, so a family is linked once.
function GuardianCard({ index, draft, error, onChange, onRemove }: { index: number; draft: GuardianDraft; error?: string; onChange: (patch: Partial<GuardianDraft>) => void; onRemove?: () => void }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const phone = digits(draft.phone)
  const email = /^\S+@\S+\.\S+$/.test(draft.email.trim()) ? draft.email.trim() : ''
  const [probe, setProbe] = useState({ phone: '', email: '' })
  useEffect(() => {
    const timer = window.setTimeout(() => setProbe({ phone: phone.length >= 10 ? phone : '', email }), 400)
    return () => window.clearTimeout(timer)
  }, [phone, email])
  const lookup = useQuery({ queryKey: ['people', 'guardian-matches', probe.phone, probe.email], enabled: !draft.existing && !draft.confirmNew && !!(probe.phone || probe.email), queryFn: () => authorized(token => getGuardianMatches(token, probe.phone, probe.email)), retry: false, staleTime: 30_000 })
  const matches = draft.matches.length ? draft.matches : lookup.data ?? []
  const relations: Relation[] = ['Father', 'Mother', 'other']
  const title = t(index === 0 ? 'c3GuardianOne' : 'c3GuardianTwo')
  return <fieldset className={`min-w-0 rounded-xl border p-3 sm:p-4 ${error ? 'border-amber-300 bg-amber-50' : 'border-slate-200'}`}>
    <legend className="px-1 text-sm font-bold text-slate-900">{title}</legend>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div role="radiogroup" aria-label={`${t('relationshipLabel')} · ${title}`} className="inline-flex flex-wrap gap-1.5">
        {relations.map(r => <button key={r} type="button" role="radio" aria-checked={draft.relation === r} onClick={() => onChange({ relation: r })} className={`min-h-10 rounded-full border px-3.5 text-sm font-semibold transition ${draft.relation === r ? 'border-[var(--brand-primary)] bg-[var(--brand-primary)] text-[var(--brand-on)]' : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50'}`}>{t(`c3Rel_${r}`)}</button>)}
      </div>
      <label className="flex min-h-10 items-center gap-2 text-sm font-semibold text-slate-700"><input type="radio" name="primary-guardian" className="h-4 w-4 accent-[var(--brand-primary)]" checked={draft.primary} onChange={() => onChange({ primary: true })} />{t('primaryGuardian')}</label>
    </div>
    {draft.relation === 'other' && <label className={`${ui.label} mt-3`}>{t('relationshipLabel')}<input required maxLength={60} className={peopleField} placeholder={t('c3RelOtherHint')} value={draft.other} onChange={e => onChange({ other: e.target.value })} /></label>}
    {draft.existing ? <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-sm text-emerald-900">
      <span className="min-w-0"><strong>{draft.existing.name_en}</strong>{draft.existing.phone && <> · {draft.existing.phone}</>}<span className="block text-xs">{t('c3LinkingExisting')}</span></span>
      <button type="button" className={ui.secondary} onClick={() => onChange({ existing: null, matches: [] })}>{t('c3Change')}</button>
    </div> : <div className="mt-3 grid gap-3 sm:grid-cols-2">
      <label className={ui.label}>{t('guardianNameEN')}<input maxLength={160} autoComplete="off" className={peopleField} value={draft.name_en} required={!!(draft.phone || draft.email || draft.name_bn)} onChange={e => onChange({ name_en: e.target.value })} /></label>
      <label className={ui.label}>{t('guardianNameBN')}<input maxLength={160} lang="bn" autoComplete="off" className={peopleField} value={draft.name_bn} onChange={e => onChange({ name_bn: e.target.value })} /></label>
      <label className={ui.label}>{t('phoneLabel')}<input type="tel" inputMode="tel" maxLength={32} autoComplete="off" className={peopleField} value={draft.phone} onChange={e => onChange({ phone: e.target.value, matches: [], confirmNew: false })} /></label>
      <label className={ui.label}>{t('email')}<input type="email" maxLength={254} autoComplete="off" className={peopleField} value={draft.email} onChange={e => onChange({ email: e.target.value, matches: [], confirmNew: false })} /></label>
    </div>}
    {!draft.existing && !draft.confirmNew && matches.length > 0 && <div role={error ? 'alert' : 'status'} className="mt-3 rounded-xl border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
      <p className="font-semibold">{error ?? t('c3MatchFound')}</p>
      <ul className="mt-2 space-y-2">{matches.map(m => <li key={m.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-white px-3 py-2">
        <span className="min-w-0"><strong>{m.name_en}</strong>{m.name_bn && <> · <span lang="bn">{m.name_bn}</span></>}<span className="block text-xs text-slate-600">{[m.phone, m.email].filter(Boolean).join(' · ')}{m.children.length > 0 && <> · {t('c3GuardianOf', { names: m.children.join(', ') })}</>}</span></span>
        <button type="button" className={ui.primary} onClick={() => onChange({ existing: m, matches: [] })}>{t('c3UseGuardian')}</button>
      </li>)}</ul>
      <button type="button" className={`${ui.ghost} mt-2`} onClick={() => onChange({ confirmNew: true, matches: [] })}>{t('c3NewAnyway')}</button>
    </div>}
    {error && !matches.length && <p role="alert" className="mt-2 text-sm text-rose-700">{error}</p>}
    {onRemove && <button type="button" className={`${ui.ghost} mt-3 text-rose-700`} onClick={onRemove}>{t('c3RemoveGuardian')}</button>}
  </fieldset>
}
