import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getAcademicSessions, type AcademicSession, type BlueprintVersionSummary, type ExamTerm } from '../../api/client'
import { ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'

// Shared pieces of the E5 exam setup screens: the session picker (kept in
// the URL), error wording and the term and version chips.

/** The sessions and the chosen one (?session=…, else the active session). */
export function useSessionChoice() {
  const authorized = usePeopleApi()
  const [params, setParams] = useSearchParams()
  const sessions = useQuery({
    queryKey: ['e5', 'sessions'],
    queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))),
    retry: false,
  })
  const items: AcademicSession[] = sessions.data?.items || []
  const chosen = params.get('session') || items.find(s => s.is_active)?.id || items[0]?.id || ''
  const choose = (id: string) => setParams(prev => { const next = new URLSearchParams(prev); next.set('session', id); return next }, { replace: true })
  return { sessions, items, chosen, choose }
}

export function SessionPicker({ items, value, onChange }: { items: AcademicSession[]; value: string; onChange: (id: string) => void }) {
  const { t } = useTranslation()
  return <label className={`${ui.label} w-full sm:w-56`}>{t('bpSession')}
    <select className={ui.field} value={value} onChange={e => onChange(e.target.value)}>
      {items.map(s => <option key={s.id} value={s.id}>{s.name}{s.is_active ? ` · ${t('bpActiveSession')}` : ''}</option>)}
    </select>
  </label>
}

/** A sentence for an API failure, by its code. */
export function useExplain() {
  const { t, i18n } = useTranslation()
  return (cause: unknown): string => {
    if (cause instanceof ApiError && cause.code && i18n.exists(`bpErr_${cause.code}`)) return t(`bpErr_${cause.code}`)
    if (cause instanceof ApiError && cause.code && i18n.exists(`e3Code_${cause.code}`)) return t(`e3Code_${cause.code}`)
    return t('bpErrGeneric')
  }
}

const termTone: Record<ExamTerm['status'], string> = {
  planning: 'border border-dashed border-slate-300 text-slate-600',
  running: 'bg-sky-50 text-sky-800 ring-1 ring-inset ring-sky-600/20',
  results: 'bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-600/20',
  closed: 'bg-slate-800 text-white',
}

export function TermChip({ status }: { status: ExamTerm['status'] }) {
  const { t } = useTranslation()
  return <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${termTone[status]}`}>{t(`bpTerm_${status}`)}</span>
}

const versionTone: Record<BlueprintVersionSummary['status'], string> = {
  draft: 'border border-dashed border-amber-400 bg-amber-50 text-amber-900',
  active: 'bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-600/20',
  retired: 'bg-slate-100 text-slate-600',
}

export function VersionChip({ version }: { version: Pick<BlueprintVersionSummary, 'status' | 'version' | 'legacy'> }) {
  const { t } = useTranslation()
  return <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${versionTone[version.status]}`}>
    {t(`bpVersion_${version.status}`)} · v{version.version}{version.legacy ? ` · ${t('bpLegacy')}` : ''}
  </span>
}

/** Subject name in the reader's language. */
export function useSubjectName() {
  const { i18n } = useTranslation()
  return (s: { name_en: string; name_bn: string } | undefined) => !s ? '' : i18n.language === 'bn' && s.name_bn ? s.name_bn : s.name_en
}
