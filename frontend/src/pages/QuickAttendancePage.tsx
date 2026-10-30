import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError, getAttendanceSections, getAttendanceSessions, getStudentAttendanceRoster, saveStudentAttendanceRoster, type StudentAttendanceRecord, type StudentAttendanceRosterItem } from '../api/client'
import { Icon } from '../components/icons'
import { usePeopleApi } from './peopleShared'
import { EmptyState, Notice, PageHeader, Skeleton, tones, ui, useNumber } from './attendanceUi'

// Quick attendance (checkpoint 3), phone first: the section's roster with
// everyone present unless marked otherwise. A tap moves a student from
// present to absent to late and back; one Save records the class. Records
// already saved keep their versions, so another device's change is reported
// instead of overwritten (the roster's optimistic lock).

type Status = StudentAttendanceRecord['status']
const cycle: Record<Status, Status> = { present: 'absent', absent: 'late', late: 'present' }
const today = () => new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Dhaka', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())

export function QuickAttendancePage() {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const num = useNumber()
  const date = useMemo(today, [])
  const [sectionId, setSectionId] = useState(() => { try { return localStorage.getItem('flybirds.quickAttendance.section') ?? '' } catch { return '' } })
  const [marks, setMarks] = useState<Record<string, Status>>({})
  const [stale, setStale] = useState<string[]>([])
  const [message, setMessage] = useState<{ tone: 'success' | 'error' | 'warning'; text: string; reload?: boolean } | null>(null)
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)

  const sessions = useQuery({ queryKey: ['attendance', 'sessions'], queryFn: () => authorized(token => getAttendanceSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const session = sessions.data?.items.find(s => s.is_active) ?? sessions.data?.items.find(s => s.start_date <= date && date <= s.end_date)
  const sections = useQuery({ queryKey: ['attendance', 'sections', session?.id], enabled: !!session, queryFn: () => authorized(token => getAttendanceSections(token, session!.id)), retry: false })
  useEffect(() => { if (sections.data && !sections.data.some(s => s.id === sectionId)) setSectionId(sections.data[0]?.id ?? '') }, [sections.data, sectionId])
  const section = sections.data?.find(s => s.id === sectionId)
  const open = !!session && date >= session.start_date && date <= session.end_date
  const roster = useQuery({
    queryKey: ['attendance', 'student-roster', session?.id, sectionId, date], enabled: !!section && open, retry: false,
    queryFn: () => authorized(async token => {
      const items: StudentAttendanceRosterItem[] = []
      for (let page = 1; page <= 20; page++) {
        const result = await getStudentAttendanceRoster(token, new URLSearchParams({ session_id: session!.id, section_id: sectionId, date, page: String(page), page_size: '100' }))
        items.push(...result.items)
        if (items.length >= result.total || result.items.length === 0) break
      }
      return items
    }),
  })
  const items = useMemo(() => roster.data ?? [], [roster.data])
  // Unmarked students count as present until saved.
  const effective = (item: StudentAttendanceRosterItem): Status => marks[item.student_id] ?? item.status ?? 'present'
  const pending = items.filter(item => effective(item) !== item.status)
  const dirty = pending.length > 0
  const counts = { present: 0, absent: 0, late: 0 }
  for (const item of items) counts[effective(item)]++

  useEffect(() => {
    if (!dirty) return
    const unload = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', unload)
    return () => window.removeEventListener('beforeunload', unload)
  }, [dirty])

  function chooseSection(id: string) {
    if (Object.keys(marks).length && !window.confirm(t('attnDiscardConfirm'))) return
    setMarks({}); setStale([]); setMessage(null); setSectionId(id)
    try { localStorage.setItem('flybirds.quickAttendance.section', id) } catch { /* no memory in private mode */ }
  }
  function tap(item: StudentAttendanceRosterItem) {
    if (saving) return
    // From the latest marks, so quick double taps each count.
    setMarks(m => {
      const next = cycle[m[item.student_id] ?? item.status ?? 'present']
      const copy = { ...m }
      if (next === (item.status ?? 'present')) delete copy[item.student_id]; else copy[item.student_id] = next
      return copy
    })
    setMessage(null)
  }
  async function save() {
    if (savingRef.current || !dirty || !session) return
    savingRef.current = true; setSaving(true); setMessage(null)
    const entries = pending.map(item => ({ student_id: item.student_id, status: effective(item), note: item.note ?? '', record_id: item.record_id, version: item.version ?? '' }))
    try {
      const result = await authorized(token => saveStudentAttendanceRoster(token, { session_id: session.id, section_id: sectionId, attendance_date: date, entries }))
      setMarks({}); setStale([])
      setMessage({ tone: 'success', text: t('c3AttnSaved', { count: result.created + result.updated, time: new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { hour: 'numeric', minute: '2-digit', timeZone: 'Asia/Dhaka' }).format(new Date()) }) })
      await cache.invalidateQueries({ queryKey: ['attendance'] })
    } catch (cause) {
      if (cause instanceof ApiError && cause.code === 'attendance_changed') {
        const ids = Array.isArray(cause.details?.student_ids) ? (cause.details.student_ids as string[]).map(id => id.toLowerCase()) : []
        setStale(ids)
        setMessage({ tone: 'warning', text: t('attnStale', { count: ids.length }), reload: true })
      } else setMessage({ tone: 'error', text: t(cause instanceof ApiError && cause.code === 'forbidden' ? 'attnForbidden' : 'attendanceSaveError') })
    } finally { savingRef.current = false; setSaving(false) }
  }
  async function reload() {
    // Keep this device's marks except for the students changed elsewhere.
    setMarks(m => Object.fromEntries(Object.entries(m).filter(([id]) => !stale.includes(id.toLowerCase()))))
    setStale([]); setMessage(null)
    await roster.refetch()
  }

  const dateLabel = new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { weekday: 'long', day: 'numeric', month: 'long', timeZone: 'UTC' }).format(new Date(`${date}T00:00:00Z`))
  return <div className="space-y-4">
    <PageHeader eyebrow={t('attnEyebrow')} title={t('c3QuickTitle')} description={t('c3QuickIntro')} actions={<Link to="/admin/attendance/students" className={ui.secondary}>{t('c3DetailedView')}</Link>} />
    <section className={`${ui.card} p-4`} aria-label={t('attnContext')}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <label className={`${ui.label} min-w-0 sm:w-80`}>{t('attnClassSection')}<select className={`${ui.field} min-h-12 text-base`} value={sectionId} onChange={e => chooseSection(e.target.value)}>{sections.data?.map(s => <option key={s.id} value={s.id}>{s.class_name} · {s.name}</option>)}</select></label>
        <p className="text-sm font-semibold text-slate-700"><Icon name="calendar" className="mr-1.5 inline h-4 w-4 align-[-3px]" />{dateLabel}</p>
      </div>
      {(sessions.isError || sections.isError) && <p role="alert" className="mt-3 text-sm text-rose-700">{t('attendanceLoadError')}</p>}
      {sections.data?.length === 0 && <p className="mt-3 text-sm text-slate-600">{t('attnNoSections')}</p>}
      {session && !open && <p className="mt-3 text-sm text-amber-800">{t('attnInvalidDay')}</p>}
    </section>

    {message && <Notice tone={message.tone} action={message.reload ? <button type="button" className={ui.secondary} onClick={() => void reload()}>{t('attnReload')}</button> : undefined}>{message.text}</Notice>}

    <section className={ui.card} aria-labelledby="quick-roster-title">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-4 py-3">
        <h2 id="quick-roster-title" className="text-base font-bold text-slate-900">{section ? `${section.class_name} · ${section.name}` : t('attendanceDaily')}</h2>
        {items.length > 0 && <p className="flex flex-wrap gap-2 text-xs font-semibold" aria-live="polite">
          {(['present', 'absent', 'late'] as const).map(s => <span key={s} className={`rounded-full px-2.5 py-1 ring-1 ring-inset ${tones[s].soft}`}>{t(`c3Count_${s}`, { n: num(counts[s]) })}</span>)}
        </p>}
      </div>
      {items.length > 0 && <p className="border-b border-slate-100 bg-slate-50 px-4 py-2 text-xs text-slate-600">{t('c3QuickHint')}</p>}
      {!section ? <div className="p-4"><EmptyState title={t('attnChooseSection')} /></div> : !open ? null : roster.isPending ? <div className="p-4"><Skeleton rows={6} /></div> : roster.isError ? <p role="alert" className="p-4 text-rose-700">{t('attendanceLoadError')}</p> : items.length === 0 ? <div className="p-4"><EmptyState title={t('studentAttendanceEmpty')} /></div> :
        <ul className="divide-y divide-slate-100" aria-label={t('attnRosterList')}>{items.map(item => {
          const status = effective(item)
          const changed = status !== item.status
          const isStale = stale.includes(item.student_id.toLowerCase())
          const display = i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en
          return <li key={item.student_id}>
            <button type="button" disabled={saving} onClick={() => tap(item)} data-roll={item.roll_no ?? ''}
              aria-label={t('c3TapLabel', { roll: item.roll_no ?? '—', name: display, status: t(`c3Status_${status}`) })}
              className={`flex min-h-16 w-full items-center gap-3 px-4 py-2.5 text-left transition active:scale-[0.99] focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-[var(--brand-primary)] border-l-4 ${isStale ? 'border-l-amber-600 bg-amber-50' : status === 'absent' ? 'border-l-rose-500' : status === 'late' ? 'border-l-amber-400' : 'border-l-transparent'}`}>
              <span className="w-8 shrink-0 text-center font-mono text-lg font-bold tabular-nums text-slate-500">{item.roll_no == null ? '—' : num(item.roll_no)}</span>
              <span className="min-w-0 flex-1"><span className="block truncate font-semibold text-slate-900">{display}</span>
                <span className="block text-xs text-slate-500">{isStale ? t('attnChangedElsewhere') : changed ? (item.status ? t('c3Correction') : t('c3NotSaved')) : t('attnSaved')}</span></span>
              <span className={`inline-flex min-h-10 min-w-24 items-center justify-center gap-1.5 rounded-xl border px-3 text-sm font-bold ${tones[status].solid}`}>{t(`c3Status_${status}`)}</span>
            </button>
          </li>
        })}</ul>}
      {items.length > 0 && <div className="ds-action-bar flex items-center justify-between gap-3 rounded-b-2xl border-t border-slate-200 bg-white/95 px-4 py-3 backdrop-blur">
        <p className="min-w-0 text-sm" aria-live="polite">{dirty ? <strong className="text-[var(--brand-ink)]">{t('c3Unsaved', { n: num(pending.length) })}</strong> : <span className="text-emerald-800"><Icon name="check" className="mr-1 inline h-4 w-4 align-[-3px]" />{t('c3AllSaved')}</span>}</p>
        <button type="button" className={`${ui.primary} min-h-12 shrink-0 px-5 text-base`} disabled={!dirty || saving} onClick={() => void save()}>{saving ? t('attnSaving') : t('c3SaveN', { n: num(pending.length) })}</button>
      </div>}
    </section>
  </div>
}
