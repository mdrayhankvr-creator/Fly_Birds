import { useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, getAcademicClasses, getAcademicSections } from '../api/client'
import { audienceRoles, fetchFile, uploadFile, type Attachment, type AudienceRole, type Priority, type Target } from '../api/communication'
import { usePeopleApi } from './peopleShared'
import { ui } from './attendanceUi'
import { Icon } from '../components/icons'

export const useApi = usePeopleApi

export function useLocal() {
  const { i18n } = useTranslation()
  return (en: string, bn?: string | null) => (i18n.language === 'bn' && bn ? bn : en)
}

export function useDhakaFormat() {
  const { i18n } = useTranslation()
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  return {
    dateTime: (iso: string | null) => iso ? new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Dhaka' }).format(new Date(iso)) : '—',
    date: (day: string) => new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${day}T00:00:00Z`)),
    number: (n: number) => new Intl.NumberFormat(locale).format(n),
  }
}

export const todayDhaka = () => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date())
export function shiftDay(day: string, days: number) {
  const d = new Date(`${day}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}
// datetime-local inputs are interpreted as Bangladesh time.
export const localToIso = (value: string) => (value ? `${value}:00+06:00` : null)
export function isoToLocal(iso: string | null) {
  if (!iso) return ''
  const d = new Date(new Date(iso).getTime() + 6 * 3600e3)
  return d.toISOString().slice(0, 16)
}

const errorKeys: Record<string, string> = {
  forbidden: 'm9ErrForbidden', not_found: 'm9ErrNotFound', conflict: 'm9ErrConflict', invalid_request: 'm9ErrInvalid', unsupported_file: 'm9ErrFileType',
  file_too_large: 'm9ErrFileSize', submission_closed: 'm9ErrSubmissionClosed', module_disabled: 'm9ErrModule',
}
export function useErrorText() {
  const { t } = useTranslation()
  return (cause: unknown) => t(cause instanceof ApiError && errorKeys[cause.code] ? errorKeys[cause.code] : 'm9ErrGeneric')
}

export function PriorityBadge({ priority }: { priority: Priority }) {
  const { t } = useTranslation()
  if (priority === 'normal') return null
  const tone = priority === 'urgent' ? 'bg-rose-600 text-white' : 'bg-amber-100 text-amber-900 ring-1 ring-amber-600/30'
  return <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-bold uppercase tracking-wide ${tone}`}>{priority === 'urgent' && <span aria-hidden="true">!</span>}{t(`m9Priority_${priority}`)}</span>
}

function formatSize(size: number) {
  return size >= 1 << 20 ? `${(size / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(size / 1024))} KB`
}

const fileKind = (file: Attachment) => file.content_type === 'application/pdf' ? 'PDF' : file.content_type === 'image/png' ? 'PNG' : file.content_type === 'image/jpeg' ? 'JPG' : 'IMG'
const fileAction = 'inline-flex min-h-10 items-center justify-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-[var(--brand-ink)] transition hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-wait disabled:opacity-60'

// AttachmentList fetches files with the session token, then opens or saves
// them from a short-lived object URL. The "cards" variant shows each file as a
// row with its type, size and labelled Preview and Download buttons.
export function AttachmentList({ items, pathFor, onRemove, variant = 'chips' }: { items: Attachment[]; pathFor: (file: Attachment) => string; onRemove?: (id: string) => void; variant?: 'chips' | 'cards' }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  async function open(file: Attachment, download: boolean) {
    setBusy(file.id); setError('')
    try {
      const blob = await authorized(token => fetchFile(token, pathFor(file) + (download ? '?download=1' : '')))
      const url = URL.createObjectURL(blob)
      if (download) {
        const a = document.createElement('a')
        a.href = url; a.download = file.name; document.body.appendChild(a); a.click(); a.remove()
      } else window.open(url, '_blank', 'noopener')
      setTimeout(() => URL.revokeObjectURL(url), 60_000)
    } catch { setError(t('m9ErrDownload')) } finally { setBusy('') }
  }
  if (items.length === 0) return null
  if (variant === 'cards') return <div><ul className="space-y-2">{items.map(file => {
    const pdf = file.content_type === 'application/pdf'
    return <li key={file.id} className="flex flex-wrap items-center gap-3 rounded-xl border border-slate-200 bg-white p-3">
      <span aria-hidden="true" className={`grid h-11 w-11 shrink-0 place-items-center rounded-lg ${pdf ? 'bg-rose-50 text-rose-700' : 'bg-sky-50 text-sky-800'}`}><Icon name={pdf ? 'file' : 'image'} className="h-5 w-5" /></span>
      <span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold text-slate-900" title={file.name}>{file.name}</span>
        <span className="mt-0.5 block text-xs text-slate-500">{fileKind(file)} · {formatSize(file.size)}</span></span>
      {pathFor(file) && <span className="flex w-full gap-2 sm:w-auto">
        <button type="button" className={`${fileAction} flex-1 sm:flex-none`} disabled={busy === file.id} onClick={() => void open(file, false)} aria-label={`${t('m9Preview')} ${file.name}`}><Icon name="eye" className="h-4 w-4" />{t('m9Preview')}</button>
        <button type="button" className={`${fileAction} flex-1 sm:flex-none`} disabled={busy === file.id} onClick={() => void open(file, true)} aria-label={`${t('m9Download')} ${file.name}`}><Icon name="download" className="h-4 w-4" />{t('m9Download')}</button></span>}
      {onRemove && <button type="button" className={`${fileAction} text-rose-700`} onClick={() => onRemove(file.id)} aria-label={`${t('m9Remove')} ${file.name}`}>{t('m9Remove')}</button>}
    </li>
  })}</ul>{error && <p role="alert" className="mt-1 text-sm text-rose-700">{error}</p>}</div>
  return <div className="mt-3"><ul className="flex flex-wrap gap-2">{items.map(file => <li key={file.id} className="flex max-w-full items-center gap-2 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm">
    <span aria-hidden="true" className={`rounded px-1.5 py-0.5 text-[0.65rem] font-bold ${file.content_type === 'application/pdf' ? 'bg-rose-100 text-rose-800' : 'bg-sky-100 text-sky-800'}`}>{file.content_type === 'application/pdf' ? 'PDF' : 'IMG'}</span>
    <span className="min-w-0 truncate font-medium text-slate-800">{file.name}</span><span className="shrink-0 text-xs text-slate-500">{formatSize(file.size)}</span>
    {pathFor(file) && <><button type="button" className={ui.ghost} disabled={busy === file.id} onClick={() => void open(file, false)} aria-label={`${t('m9Open')} ${file.name}`}>{t('m9Open')}</button>
      <button type="button" className={ui.ghost} disabled={busy === file.id} onClick={() => void open(file, true)} aria-label={`${t('m9Download')} ${file.name}`}>{t('m9Download')}</button></>}
    {onRemove && <button type="button" className="text-rose-700 hover:underline" onClick={() => onRemove(file.id)} aria-label={`${t('m9Remove')} ${file.name}`}>{t('m9Remove')}</button>}
  </li>)}</ul>{error && <p role="alert" className="mt-1 text-sm text-rose-700">{error}</p>}</div>
}

export function FileUploader({ path, onUploaded, onBusy, disabled, max }: { path: '/notices/files' | '/diary/files' | '/diary/homework/files'; onUploaded: (file: Attachment) => void; onBusy?: (busy: boolean) => void; disabled?: boolean; max: number }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const errorText = useErrorText()
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  async function pick(list: FileList | null) {
    if (!list) return
    setBusy(true); onBusy?.(true); setError('')
    for (const file of Array.from(list).slice(0, max)) {
      try { onUploaded(await authorized(token => uploadFile(token, path, file))) } catch (cause) { setError(`${file.name}: ${errorText(cause)}`) }
    }
    setBusy(false); onBusy?.(false)
    if (input.current) input.current.value = ''
  }
  return <div>
    <label className={`${ui.secondary} cursor-pointer ${disabled || busy || max <= 0 ? 'pointer-events-none opacity-50' : ''}`}>
      <svg aria-hidden="true" viewBox="0 0 20 20" className="h-4 w-4 fill-current"><path d="M9 3h2v8h3l-4 4-4-4h3zM4 16h12v2H4z" transform="rotate(180 10 10.5)" /></svg>
      {busy ? t('m9Uploading') : t('m9AttachFiles')}
      <input ref={input} type="file" className="sr-only" multiple accept=".pdf,.jpg,.jpeg,.png,application/pdf,image/jpeg,image/png" disabled={disabled || busy || max <= 0} onChange={e => void pick(e.target.files)} />
    </label>
    <p className="mt-1 text-xs text-slate-500">{t('m9FileHint')}</p>
    {error && <p role="alert" className="mt-1 text-sm text-rose-700">{error}</p>}
  </div>
}

export function RolePicker({ value, onChange }: { value: AudienceRole[]; onChange: (roles: AudienceRole[]) => void }) {
  const { t } = useTranslation()
  return <fieldset><legend className={ui.label}>{t('m9Audience')}</legend>
    <div className="mt-2 flex flex-wrap gap-2">
      <button type="button" className={ui.ghost} onClick={() => onChange(value.length === audienceRoles.length ? [] : [...audienceRoles])}>{t(value.length === audienceRoles.length ? 'm9ClearAll' : 'm9Everyone')}</button>
      {audienceRoles.map(role => <label key={role} className={`flex cursor-pointer items-center gap-2 rounded-full border px-3 py-1.5 text-sm ${value.includes(role) ? 'border-[var(--brand-primary)] bg-[var(--brand-soft)] font-semibold' : 'border-slate-200'}`}>
        <input type="checkbox" className="accent-[var(--brand-primary)]" checked={value.includes(role)} onChange={e => onChange(e.target.checked ? [...value, role] : value.filter(r => r !== role))} />{t(`role_${role}`)}</label>)}
    </div></fieldset>
}

// TargetPicker narrows an audience to classes or sections; leaving it empty
// keeps the notice school-wide for the chosen roles.
export function TargetPicker({ value, onChange }: { value: Target[]; onChange: (targets: Target[]) => void }) {
  const { t } = useTranslation()
  const authorized = useApi()
  const [classId, setClassId] = useState('')
  const [sectionId, setSectionId] = useState('')
  const params = new URLSearchParams({ page: '1', page_size: '100' })
  const classes = useQuery({ queryKey: ['m9', 'classes'], queryFn: () => authorized(token => getAcademicClasses(token, params)), retry: false })
  const sections = useQuery({ queryKey: ['m9', 'sections', classId], enabled: !!classId, queryFn: () => authorized(token => getAcademicSections(token, new URLSearchParams({ page: '1', page_size: '100', class_id: classId }))), retry: false })
  const className = (id: string) => classes.data?.items.find(c => c.id === id)?.name ?? ''
  function add() {
    if (!classId) return
    const section = sections.data?.items.find(s => s.id === sectionId)
    const next: Target = { class_id: classId, section_id: sectionId || null, class_name: className(classId), section_name: section ? `${section.name} (${section.session_name})` : '' }
    if (!value.some(v => v.class_id === next.class_id && v.section_id === next.section_id)) onChange([...value, next])
    setSectionId('')
  }
  return <fieldset><legend className={ui.label}>{t('m9Targets')}</legend><p className="mt-1 text-xs text-slate-500">{t('m9TargetsHint')}</p>
    <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:items-end">
      <label className={`${ui.label} flex-1`}>{t('m9Class')}<select className={ui.field} value={classId} onChange={e => { setClassId(e.target.value); setSectionId('') }}><option value="">—</option>{classes.data?.items.filter(c => c.is_active).map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
      <label className={`${ui.label} flex-1`}>{t('m9Section')}<select className={ui.field} value={sectionId} disabled={!classId} onChange={e => setSectionId(e.target.value)}><option value="">{t('m9AllSections')}</option>{sections.data?.items.filter(s => s.is_active).map(s => <option key={s.id} value={s.id}>{s.name} · {s.session_name}</option>)}</select></label>
      <button type="button" className={ui.secondary} disabled={!classId} onClick={add}>{t('m9AddTarget')}</button>
    </div>
    {classes.isError && <p className="mt-1 text-xs text-amber-800">{t('m9TargetsUnavailable')}</p>}
    {value.length > 0 && <ul className="mt-2 flex flex-wrap gap-2">{value.map((v, i) => <li key={`${v.class_id}-${v.section_id}`} className="flex items-center gap-2 rounded-full bg-slate-100 px-3 py-1 text-sm">
      {v.class_name || className(v.class_id)}{v.section_id ? ` · ${v.section_name}` : ` · ${t('m9AllSections')}`}
      <button type="button" className="text-slate-500 hover:text-rose-700" aria-label={t('m9Remove')} onClick={() => onChange(value.filter((_, j) => j !== i))}>×</button></li>)}</ul>}
  </fieldset>
}

export function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return <label className={`${ui.label} ${className}`}>{label}{children}</label>
}
