import { useEffect, useState, type CSSProperties, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ApiError, getSchoolGrading, getSchoolModules, getSchoolSettings, putSchoolGrading,
  putSchoolSettings, type GradeBand, type SchoolSettingsInput,
} from '../api/client'
import { Link } from 'react-router'
import { useAuth } from '../auth/AuthProvider'

const colorPattern = /^#[0-9a-fA-F]{6}$/
function readableOnWhite(value: string): boolean {
  if (!colorPattern.test(value)) return false
  const channel = (part: string) => {
    const value = parseInt(part, 16) / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }
  const luminance = 0.2126 * channel(value.slice(1, 3)) + 0.7152 * channel(value.slice(3, 5)) + 0.0722 * channel(value.slice(5, 7))
  return 1.05 / (luminance + 0.05) >= 4.5
}
const inputClass = 'mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 outline-none focus:border-[var(--brand-primary)]'
const labelClass = 'block text-sm font-semibold text-slate-700'

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className={labelClass}>{label}{children}</label>
}

export function SchoolSettingsPage({ permissions }: { permissions: string[] }) {
  const { t, i18n } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<SchoolSettingsInput | null>(null)
  const [bands, setBands] = useState<GradeBand[] | null>(null)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const canEditSettings = permissions.includes('school_settings:update')
  const canReadGrading = permissions.includes('grading:read')
  const canEditGrading = permissions.includes('grading:update')
  const canReadModules = permissions.includes('modules:read')

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

  const settings = useQuery({ queryKey: ['school', 'settings'], queryFn: () => authorized(getSchoolSettings), retry: false })
  const grading = useQuery({ queryKey: ['school', 'grading'], queryFn: () => authorized(getSchoolGrading), enabled: canReadGrading, retry: false })
  const modules = useQuery({ queryKey: ['school', 'modules'], queryFn: () => authorized(getSchoolModules), enabled: canReadModules, retry: false })

  useEffect(() => {
    if (settings.data) {
      const { updated_at: _updatedAt, ...fields } = settings.data
      setDraft(fields)
    }
  }, [settings.data])
  useEffect(() => { if (grading.data) setBands(grading.data.map((band) => ({ ...band }))) }, [grading.data])

  const saveSettings = useMutation({
    mutationFn: (value: SchoolSettingsInput) => authorized((token) => putSchoolSettings(token, value)),
    onSuccess: async () => {
      setError(''); setNotice(t('schoolSaved'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['school', 'settings'] }),
        queryClient.invalidateQueries({ queryKey: ['school', 'branding'] }),
      ])
    },
    onError: () => { setNotice(''); setError(t('schoolSaveError')) },
  })
  const saveGrading = useMutation({
    mutationFn: (value: GradeBand[]) => authorized((token) => putSchoolGrading(token, value)),
    onSuccess: async () => { setError(''); setNotice(t('gradingSaved')); await queryClient.invalidateQueries({ queryKey: ['school', 'grading'] }) },
    onError: () => { setNotice(''); setError(t('schoolSaveError')) },
  })
  function setField<Key extends keyof SchoolSettingsInput>(key: Key, value: SchoolSettingsInput[Key]) {
    setDraft((current) => current ? { ...current, [key]: value } : current)
  }
  function resetDrafts() {
    if (settings.data) {
      const { updated_at: _updatedAt, ...fields } = settings.data
      setDraft(fields)
    }
    if (grading.data) setBands(grading.data.map((band) => ({ ...band })))
    setError(''); setNotice('')
  }
  function submitSettings(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft) return
    if (!readableOnWhite(draft.primary_color) || !readableOnWhite(draft.secondary_color)) {
      setError(t('schoolValidationError')); return
    }
    setError(''); setNotice(''); saveSettings.mutate(draft)
  }
  function submitGrading(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!bands) return
    const sorted = [...bands].sort((a, b) => a.min_marks - b.min_marks)
    if (sorted.length === 0 || sorted[0].min_marks !== 0 || sorted[sorted.length - 1].max_marks !== 100 ||
      sorted.some((band, index) => !band.grade.trim() || !Number.isInteger(band.min_marks) || !Number.isInteger(band.max_marks) ||
        band.min_marks > band.max_marks || band.gpa < 0 || band.gpa > 5 || !Number.isFinite(band.gpa) ||
        (index > 0 && band.min_marks !== sorted[index - 1].max_marks + 1))) {
      setError(t('gradingValidationError')); return
    }
    setError(''); setNotice(''); saveGrading.mutate(bands)
  }

  const previewStyle = draft ? {
    '--brand-primary': colorPattern.test(draft.primary_color) ? draft.primary_color : '#0F766E',
    '--brand-secondary': colorPattern.test(draft.secondary_color) ? draft.secondary_color : '#334155',
  } as CSSProperties : undefined
  const previewLogo = draft?.logo_url && /^https:\/\/[^\s]+\.(png|jpe?g|webp)$/i.test(draft.logo_url)
  const loading = settings.isPending || (canReadGrading && grading.isPending) || (canReadModules && modules.isPending)
  const loadError = settings.isError || grading.isError || modules.isError

  return <section className="space-y-8">
    <div>
      <p className="brand-text text-sm font-bold uppercase tracking-[0.2em]">{t('administration')}</p>
      <h1 className="mt-2 text-3xl font-bold text-slate-900">{t('schoolSettings')}</h1>
      <p className="mt-2 text-slate-600">{t('schoolSettingsDescription')}</p>
    </div>
    {loading && <p role="status">{t('loadingSettings')}</p>}
    {loadError && <p role="alert" className="rounded-lg bg-rose-50 p-4 text-rose-800">{t('schoolLoadError')} <button type="button" className="underline" onClick={() => { void settings.refetch(); if (canReadGrading) void grading.refetch(); if (canReadModules) void modules.refetch() }}>{t('reload')}</button></p>}
    {notice && <p role="status" className="rounded-lg bg-emerald-50 p-4 text-emerald-800">{notice}</p>}
    {error && <p role="alert" className="rounded-lg bg-rose-50 p-4 text-rose-800">{error}</p>}
    {!loading && !loadError && draft && <>
      {!canEditSettings && <p className="rounded-lg border border-slate-200 p-3 text-sm text-slate-600">{t('settingsReadOnly')}</p>}
      <form onSubmit={submitSettings} className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
        <h2 className="text-xl font-bold text-slate-900">{t('schoolProfile')}</h2>
        <div className="mt-5 grid gap-5 sm:grid-cols-2">
          <FormField label={t('schoolNameEN')}><input className={inputClass} required maxLength={120} value={draft.name_en} disabled={!canEditSettings} onChange={(event) => setField('name_en', event.target.value)} /></FormField>
          <FormField label={t('schoolNameBN')}><input className={inputClass} required maxLength={120} value={draft.name_bn} disabled={!canEditSettings} onChange={(event) => setField('name_bn', event.target.value)} /></FormField>
          <FormField label={t('schoolLogo')}><input className={inputClass} type="url" maxLength={500} value={draft.logo_url} disabled={!canEditSettings} onChange={(event) => setField('logo_url', event.target.value)} /></FormField>
          <FormField label={t('schoolAddress')}><input className={inputClass} maxLength={500} value={draft.address} disabled={!canEditSettings} onChange={(event) => setField('address', event.target.value)} /></FormField>
          <FormField label={t('schoolPhone')}><input className={inputClass} type="tel" maxLength={32} value={draft.phone} disabled={!canEditSettings} onChange={(event) => setField('phone', event.target.value)} /></FormField>
          <FormField label={t('schoolEmail')}><input className={inputClass} type="email" maxLength={254} value={draft.email} disabled={!canEditSettings} onChange={(event) => setField('email', event.target.value)} /></FormField>
          <FormField label={t('preferredLanguage')}><select className={inputClass} value={draft.preferred_language} disabled={!canEditSettings} onChange={(event) => setField('preferred_language', event.target.value as 'en' | 'bn')}><option value="en">{t('languageEnglish')}</option><option value="bn">{t('languageBangla')}</option></select></FormField>
          <FormField label={t('dashboardTheme')}><select className={inputClass} value={draft.dashboard_theme} disabled={!canEditSettings} onChange={(event) => setField('dashboard_theme', event.target.value as 'light' | 'dark')}><option value="light">{t('lightTheme')}</option><option value="dark">{t('darkTheme')}</option></select></FormField>
          <FormField label={t('primaryColor')}><input className={inputClass} type="text" pattern="#[0-9a-fA-F]{6}" maxLength={7} value={draft.primary_color} disabled={!canEditSettings} onChange={(event) => setField('primary_color', event.target.value)} /></FormField>
          <FormField label={t('secondaryColor')}><input className={inputClass} type="text" pattern="#[0-9a-fA-F]{6}" maxLength={7} value={draft.secondary_color} disabled={!canEditSettings} onChange={(event) => setField('secondary_color', event.target.value)} /></FormField>
        </div>
        <div className="mt-6 rounded-xl border border-slate-200 p-5" data-theme-preview={draft.dashboard_theme} style={previewStyle}>
          <h3 className="text-lg font-semibold">{t('preview')}</h3>
          <div className="mt-3 flex flex-wrap items-center gap-4">
            {previewLogo ? <img src={draft.logo_url} alt="" referrerPolicy="no-referrer" className="h-14 w-14 rounded-lg object-contain" /> : <span className="text-sm opacity-70">{t('noLogo')}</span>}
            <span className="font-bold">{i18n.language === 'bn' ? draft.name_bn : draft.name_en}</span>
          </div>
          <p className="mt-3 text-sm">{t('previewText')}</p>
          <span className="brand-button mt-3 inline-block rounded-lg px-4 py-2 text-sm font-semibold">{t('previewAction')}</span>
        </div>
        {canEditSettings && <div className="mt-6 flex flex-wrap gap-3"><button className="brand-button rounded-lg px-5 py-2.5 font-semibold disabled:opacity-50" type="submit" disabled={saveSettings.isPending}>{saveSettings.isPending ? t('saving') : t('saveProfile')}</button><button type="button" className="rounded-lg border border-slate-300 px-5 py-2.5 font-semibold" onClick={resetDrafts}>{t('discardChanges')}</button></div>}
      </form>
      {canReadGrading && bands && <form onSubmit={submitGrading} className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
        <h2 className="text-xl font-bold text-slate-900">{t('gradingScheme')}</h2>
        <div className="mt-5 overflow-x-auto"><table className="w-full min-w-[28rem] text-left text-sm"><thead><tr className="border-b border-slate-200"><th className="p-2">{t('grade')}</th><th className="p-2">{t('minMarks')}</th><th className="p-2">{t('maxMarks')}</th><th className="p-2">{t('gpa')}</th></tr></thead><tbody>{bands.map((band, index) => <tr key={index} className="border-b border-slate-100">
          {(['grade', 'min_marks', 'max_marks', 'gpa'] as const).map((field) => <td className="p-2" key={field}><input aria-label={`${t(field === 'min_marks' ? 'minMarks' : field === 'max_marks' ? 'maxMarks' : field)} ${band.grade}`} className={inputClass} type={field === 'grade' ? 'text' : 'number'} min={field === 'gpa' ? 0 : field === 'grade' ? undefined : 0} max={field === 'gpa' ? 5 : field === 'grade' ? undefined : 100} step={field === 'gpa' ? '0.01' : '1'} required disabled={!canEditGrading} value={band[field]} onChange={(event) => setBands((current) => current?.map((row, rowIndex) => rowIndex === index ? { ...row, [field]: field === 'grade' ? event.target.value : Number(event.target.value) } : row) ?? null)} /></td>)}
        </tr>)}</tbody></table></div>
        {canEditGrading && <div className="mt-5 flex flex-wrap gap-3"><button type="submit" disabled={saveGrading.isPending} className="brand-button rounded-lg px-5 py-2.5 font-semibold disabled:opacity-50">{saveGrading.isPending ? t('saving') : t('saveGrading')}</button><button type="button" onClick={resetDrafts} className="rounded-lg border border-slate-300 px-5 py-2.5 font-semibold">{t('discardChanges')}</button></div>}
      </form>}
      {canReadModules && modules.data && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
        <h2 className="text-xl font-bold text-slate-900">{t('schoolModules')}</h2><p className="mt-2 text-sm text-slate-600">{t('m10SettingsModulesMoved')}</p>
        <ul className="mt-4 flex flex-wrap gap-2">{modules.data.items.map((module) => <li key={module.key} className={`rounded-full px-3 py-1 text-sm font-medium ${module.enabled ? 'bg-emerald-50 text-emerald-800' : 'bg-slate-100 text-slate-600'}`}>{t(`m10Module_${module.key}`, { defaultValue: module.name })} · {module.enabled ? t('enabled') : t('disabled')}</li>)}</ul>
        <Link to="/admin/modules" className="brand-button mt-5 inline-block rounded-lg px-5 py-2.5 font-semibold">{t('m10OpenModules')}</Link>
      </section>}
    </>}
  </section>
}
