import { useTranslation } from 'react-i18next'
import { ApiError } from '../../api/client'
import type { PaperKind, RoutineIssue, RoutineVersion } from '../../api/routinePlanner'

// Shared wording for the term exam routine (E8).

const DHAKA = 'Asia/Dhaka'

export function todayDhaka() {
  return new Intl.DateTimeFormat('en-CA', { timeZone: DHAKA, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
}

export function daysBetween(from: string, to: string) {
  return Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / 86_400_000)
}

// useDay formats a calendar date ("Wed, 2 Jun 2027") in the page's language.
export function useDay() {
  const { i18n } = useTranslation()
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  return (iso: string, long = false) => iso ? new Intl.DateTimeFormat(locale, long ? { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' } : { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${iso}T00:00:00Z`)) : ''
}

export function useNum() {
  const { i18n } = useTranslation()
  return (value: number | string) => i18n.language === 'bn' ? String(value).replace(/[0-9]/g, d => '০১২৩৪৫৬৭৮৯'[Number(d)]) : String(value)
}

export function useName() {
  const { i18n } = useTranslation()
  return (en: string, bn?: string | null) => i18n.language === 'bn' && bn ? bn : en
}

// usePaperName names a paper: "English 2nd paper", "Physics practical".
export function usePaperName() {
  const { t } = useTranslation()
  const name = useName()
  return (p: { subject_name: string; subject_name_bn: string; kind: PaperKind; paper_no: number }, numbered = false) => {
    const parts = [name(p.subject_name, p.subject_name_bn)]
    if (p.paper_no > 1 || numbered) parts.push(t(`e8Nth_${p.paper_no}`))
    if (p.kind !== 'theory') parts.push(t(`e8Kind_${p.kind}`).toLowerCase())
    return parts.join(' ')
  }
}

// useIssueText words a rule check finding from its code and the version's
// names, in the page's language.
export function useIssueText(version: RoutineVersion | undefined) {
  const { t } = useTranslation()
  const day = useDay()
  const num = useNum()
  const name = useName()
  const paperName = usePaperName()
  return (issue: RoutineIssue) => {
    if (!version) return issue.message
    const paper = (id: string) => {
      const p = version.papers.find(x => x.id === id)
      return p ? `${p.class_name} ${paperName(p)}` : ''
    }
    const sitting = version.sittings.find(s => s.id === issue.sitting_id)
    const room = version.rooms.find(r => r.id === issue.room_id)
    const teacher = version.teachers.find(x => x.id === issue.teacher_id)
    const section = version.classes.flatMap(c => c.sections).find(s => s.id === issue.section_id)
    const cls = version.classes.find(c => c.id === issue.class_id) ?? version.classes.find(c => version.papers.some(p => p.class_id === c.id && issue.paper_ids?.includes(p.id)))
    const ids = issue.paper_ids ?? []
    const values = {
      paper: ids.length ? paper(ids[0]) : '', papers: ids.map(id => { const p = version.papers.find(x => x.id === id); return p ? paperName(p) : '' }).filter(Boolean).join(', '),
      class: cls?.name ?? '', section: section?.name ?? '', room: room ? name(room.name, room.name_bn) : '', teacher: teacher ? name(teacher.name, teacher.name_bn) : '',
      sitting: sitting ? name(sitting.name, sitting.name_bn) : '', date: issue.date ? day(issue.date) : '', holiday: issue.holiday ?? '',
      when: [issue.date ? day(issue.date) : '', sitting ? name(sitting.name, sitting.name_bn) : ''].filter(Boolean).join(', '),
      count: num(issue.count ?? 0), limit: num(issue.limit ?? 0),
    }
    const key = `e8Issue_${issue.code}`
    const text = t(key, values)
    return text === key ? issue.message : text
  }
}

// routineError words a failed planner request; issues come back on a
// conflict.
export function routineErrorOf(cause: unknown): { key: string; issues: RoutineIssue[] } {
  if (cause instanceof ApiError) {
    const issues = Array.isArray(cause.details?.issues) ? cause.details.issues as RoutineIssue[] : []
    switch (cause.code) {
      case 'routine_conflict': return { key: 'e8ErrorConflict', issues }
      case 'routine_not_ready': return { key: 'e8ErrorNotReady', issues }
      case 'routine_state': return { key: 'e8ErrorState', issues }
      case 'invalid_request': return { key: 'e8ErrorInvalid', issues }
      case 'schedule_conflict': return { key: 'e8ErrorExists', issues }
      case 'documents_unavailable': return { key: 'e8DocUnavailable', issues }
    }
  }
  return { key: 'e8ErrorGeneric', issues: [] }
}

export const statusTone: Record<string, string> = {
  draft: 'bg-slate-100 text-slate-700 ring-slate-500/20',
  submitted: 'bg-amber-50 text-amber-900 ring-amber-600/25',
  approved: 'bg-sky-50 text-sky-800 ring-sky-600/20',
  published: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20',
  superseded: 'bg-slate-100 text-slate-600 ring-slate-500/20',
}

export function StatusPill({ status }: { status: string }) {
  const { t } = useTranslation()
  return <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ring-1 ring-inset ${statusTone[status] ?? statusTone.draft}`}>{t(`e8Status_${status}`)}</span>
}
