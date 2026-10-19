import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { useAuth } from '../auth/AuthProvider'
import { ApiError, getAttendanceSessions, getAttendanceSections, getStudentAttendanceRoster, getStudentAttendanceMonthly, getOwnStudentAttendanceReport, getGuardianStudentAttendanceReport, saveStudentAttendanceRoster, scanStudentAttendance, getAbsenceEvents, type StudentAttendanceRosterItem, type StudentAttendanceRecord } from '../api/client'
import { Avatar, EmptyState, Notice, PageHeader, Pager, SegmentedStatus, Skeleton, StatTile, StatusBadge, Tabs, tones, ui, type SegmentOption } from './attendanceUi'

const today = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Dhaka', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
type Status = StudentAttendanceRecord['status']
type Draft = { status: Status | ''; note: string }
type Filter = 'all' | 'unmarked' | 'changed' | Status
type Tab = 'daily' | 'monthly' | 'alerts'
type Detector = { detect: (source: HTMLVideoElement) => Promise<Array<{ rawValue: string }>> }
type DetectorConstructor = new (options: { formats: string[] }) => Detector
type Message = { tone: 'success' | 'error' | 'info' | 'warning'; text: string; reload?: boolean }

function shiftDay(date: string, days: number) {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

function useAuthorized() {
  const { accessToken, renew, expire } = useAuth()
  return async function authorized<T>(fn: (token: string) => Promise<T>): Promise<T> {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await fn(accessToken) } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await fn(await renew()) } catch (retry) { if (retry instanceof ApiError && retry.status === 401) expire(); throw retry }
      }
      throw cause
    }
  }
}

function useStatusOptions(): SegmentOption<Status>[] {
  const { t } = useTranslation()
  return [
    { value: 'present', label: t('studentAttendancePresent'), short: t('attnShortPresent'), tone: 'present', hotkey: 'p' },
    { value: 'absent', label: t('studentAttendanceAbsent'), short: t('attnShortAbsent'), tone: 'absent', hotkey: 'a' },
    { value: 'late', label: t('studentAttendanceLate'), short: t('attnShortLate'), tone: 'late', hotkey: 'l' },
  ]
}

export function StudentAttendancePage({ permissions }: { permissions: string[] }) {
  const { t, i18n } = useTranslation()
  const authorized = useAuthorized()
  const cache = useQueryClient()
  const options = useStatusOptions()
  const canManage = permissions.includes('attendance:students_manage')
  const canAlerts = permissions.includes('attendance:alerts_read')
  const [tab, setTab] = useState<Tab>('daily')
  const [sessionId, setSessionId] = useState('')
  const [sectionId, setSectionId] = useState('')
  const [date, setDate] = useState(today)
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [notesOpen, setNotesOpen] = useState<Record<string, boolean>>({})
  const [stale, setStale] = useState<string[]>([])
  const [message, setMessage] = useState<Message | null>(null)
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)
  const [qrOpen, setQrOpen] = useState(false)
  const name = (en: string, bn: string) => (i18n.language === 'bn' && bn ? bn : en)
  const num = (n: number) => new Intl.NumberFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-US').format(n)

  const sessions = useQuery({ queryKey: ['attendance', 'sessions'], queryFn: () => authorized(token => getAttendanceSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  useEffect(() => {
    if (!sessionId && sessions.data?.items.length) setSessionId((sessions.data.items.find(item => item.is_active) || sessions.data.items.find(item => item.start_date <= today && today <= item.end_date) || sessions.data.items[0]).id)
  }, [sessionId, sessions.data])
  const session = sessions.data?.items.find(item => item.id === sessionId)
  const sections = useQuery({ queryKey: ['attendance', 'sections', sessionId], queryFn: () => authorized(token => getAttendanceSections(token, sessionId)), enabled: !!sessionId, retry: false })
  useEffect(() => { if (sections.data && !sections.data.some(item => item.id === sectionId)) setSectionId(sections.data[0]?.id || '') }, [sections.data, sectionId])
  const section = sections.data?.find(item => item.id === sectionId)
  const maxDate = session && session.end_date < today ? session.end_date : today
  const dateValid = !!session && date >= session.start_date && date <= maxDate

  // Load the whole section so the class can be saved in one action.
  const roster = useQuery({
    queryKey: ['attendance', 'student-roster', sessionId, sectionId, date],
    enabled: !!sectionId && dateValid,
    retry: false,
    queryFn: () => authorized(async token => {
      const items: StudentAttendanceRosterItem[] = []
      for (let page = 1; page <= 20; page++) {
        const result = await getStudentAttendanceRoster(token, new URLSearchParams({ session_id: sessionId, section_id: sectionId, date, page: String(page), page_size: '100' }))
        items.push(...result.items)
        if (items.length >= result.total || result.items.length === 0) break
      }
      return items
    }),
  })
  const items = useMemo(() => roster.data ?? [], [roster.data])
  const effective = (item: StudentAttendanceRosterItem) => drafts[item.student_id]?.status ?? item.status ?? ''
  const changed = items.filter(item => drafts[item.student_id])
  const dirty = changed.length > 0
  const counts = useMemo(() => {
    const c = { present: 0, absent: 0, late: 0, unmarked: 0 }
    for (const item of items) { const s = drafts[item.student_id]?.status ?? item.status ?? ''; if (s) c[s]++; else c.unmarked++ }
    return c
  }, [items, drafts])
  const marked = items.length - counts.unmarked

  // Warn before losing unsaved marks: browser reloads/closes and in-app links.
  useEffect(() => {
    if (!dirty) return
    const unload = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    const links = (event: MouseEvent) => {
      const link = (event.target as HTMLElement | null)?.closest?.('a[href]') as HTMLAnchorElement | null
      if (link && link.origin === location.origin && !window.confirm(t('attnLeaveConfirm'))) { event.preventDefault(); event.stopPropagation() }
    }
    window.addEventListener('beforeunload', unload)
    document.addEventListener('click', links, true)
    return () => { window.removeEventListener('beforeunload', unload); document.removeEventListener('click', links, true) }
  }, [dirty, t])

  function guarded(change: () => void) {
    if (dirty && !window.confirm(t('attnDiscardConfirm'))) return
    setDrafts({}); setNotesOpen({}); setStale([]); setMessage(null); change()
  }
  function setDraft(item: StudentAttendanceRosterItem, patch: Partial<Draft>) {
    setDrafts(previous => {
      const current = previous[item.student_id] ?? { status: item.status ?? '', note: item.note ?? '' }
      const next = { ...current, ...patch }
      const copy = { ...previous }
      if (next.status === (item.status ?? '') && next.note === (item.note ?? '')) delete copy[item.student_id]
      else copy[item.student_id] = next
      return copy
    })
  }
  function markAllPresent() {
    setDrafts(previous => {
      const copy = { ...previous }
      for (const item of items) if (!(copy[item.student_id]?.status ?? item.status)) copy[item.student_id] = { status: 'present', note: copy[item.student_id]?.note ?? item.note ?? '' }
      return copy
    })
  }
  async function save() {
    if (savingRef.current || !dirty) return
    const entries = changed.filter(item => drafts[item.student_id].status).map(item => ({ student_id: item.student_id, status: drafts[item.student_id].status as Status, note: drafts[item.student_id].note.trim(), record_id: item.record_id, version: item.version ?? '' }))
    if (!entries.length) return
    savingRef.current = true; setSaving(true); setMessage(null)
    try {
      const result = await authorized(token => saveStudentAttendanceRoster(token, { session_id: sessionId, section_id: sectionId, attendance_date: date, entries }))
      setDrafts({}); setStale([]); setNotesOpen({})
      setMessage({ tone: 'success', text: t('attnSavedSummary', { created: result.created, updated: result.updated, unchanged: result.unchanged }) })
      await cache.invalidateQueries({ queryKey: ['attendance'] })
    } catch (cause) {
      if (cause instanceof ApiError && cause.code === 'attendance_changed') {
        const ids = Array.isArray(cause.details?.student_ids) ? (cause.details.student_ids as string[]) : []
        setStale(ids.map(id => id.toLowerCase()))
        setMessage({ tone: 'warning', text: t('attnStale', { count: ids.length }), reload: true })
      } else setMessage({ tone: 'error', text: t(cause instanceof ApiError ? ({ forbidden: 'attnForbidden', attendance_conflict: 'attnConflict', duplicate_attendance: 'attnDuplicate', invalid_request: 'attnInvalid' } as Record<string, string>)[cause.code] ?? 'attendanceSaveError' : 'attendanceSaveError'), reload: cause instanceof ApiError && cause.code === 'duplicate_attendance' })
    } finally { savingRef.current = false; setSaving(false) }
  }
  async function reloadRoster() {
    setDrafts(previous => { const copy = { ...previous }; for (const id of stale) for (const key of Object.keys(copy)) if (key.toLowerCase() === id) delete copy[key]; return copy })
    setStale([]); setMessage(null)
    await roster.refetch()
  }

  const query = search.trim().toLowerCase()
  const visible = items.filter(item => {
    const status = effective(item)
    if (filter === 'unmarked' && status) return false
    if (filter === 'changed' && !drafts[item.student_id]) return false
    if (filter !== 'all' && filter !== 'unmarked' && filter !== 'changed' && status !== filter) return false
    return !query || item.name_en.toLowerCase().includes(query) || item.name_bn.includes(search.trim()) || item.admission_no.toLowerCase().includes(query) || String(item.roll_no ?? '') === query
  })
  const correcting = date !== today
  const filters: { value: Filter; label: string; count: number }[] = [
    { value: 'all', label: t('attnFilterAll'), count: items.length },
    { value: 'unmarked', label: t('attendanceUnmarked'), count: counts.unmarked },
    { value: 'present', label: t('studentAttendancePresent'), count: counts.present },
    { value: 'absent', label: t('studentAttendanceAbsent'), count: counts.absent },
    { value: 'late', label: t('studentAttendanceLate'), count: counts.late },
    ...(dirty ? [{ value: 'changed' as Filter, label: t('attnFilterChanged'), count: changed.length }] : []),
  ]

  return <div className="space-y-5 pb-4">
    <PageHeader eyebrow={t('attnEyebrow')} title={t('studentAttendanceHeading')} description={t('attnDescription')} />

    <section className={`${ui.card} p-4 sm:p-5`} aria-label={t('attnContext')}>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-[1fr_1.2fr_1.6fr]">
        <label className={ui.label}>{t('attendanceSession')}<select className={ui.field} value={sessionId} onChange={event => { const v = event.target.value; guarded(() => { setSessionId(v); setSectionId('') }) }}><option value="">—</option>{sessions.data?.items.map(item => <option key={item.id} value={item.id}>{item.name}{item.is_active ? ` · ${t('attnActive')}` : ''}</option>)}</select></label>
        <label className={ui.label}>{t('attnClassSection')}<select className={ui.field} value={sectionId} onChange={event => { const v = event.target.value; guarded(() => setSectionId(v)) }}><option value="">—</option>{sections.data?.map(item => <option key={item.id} value={item.id}>{item.class_name} · {item.name}</option>)}</select></label>
        <div className={`${ui.label} sm:col-span-2 lg:col-span-1`}><label htmlFor="attn-date">{t('attendanceDate')}</label>
          <div className="mt-1.5 flex gap-2">
            <button type="button" className={ui.secondary} aria-label={t('attnPreviousDay')} disabled={!session || date <= session.start_date} onClick={() => guarded(() => setDate(shiftDay(date, -1)))}>‹</button>
            <input id="attn-date" className={`${ui.field} mt-0`} type="date" min={session?.start_date} max={maxDate} value={date} onChange={event => { const v = event.target.value; if (v) guarded(() => setDate(v)) }} />
            <button type="button" className={ui.secondary} aria-label={t('attnNextDay')} disabled={date >= maxDate} onClick={() => guarded(() => setDate(shiftDay(date, 1)))}>›</button>
            <button type="button" className={ui.secondary} disabled={date === today || today > maxDate} onClick={() => guarded(() => setDate(today))}>{t('attnToday')}</button>
          </div></div>
      </div>
      {(sessions.isError || sections.isError) && <p role="alert" className="mt-3 text-sm text-rose-700">{t('attendanceLoadError')}</p>}
      {sections.data && sections.data.length === 0 && <p className="mt-3 text-sm text-slate-600">{t('attnNoSections')}</p>}
    </section>

    <Tabs label={t('studentAttendanceHeading')} value={tab} onChange={value => setTab(value)} items={[
      { value: 'daily', label: t('attendanceDaily'), badge: dirty ? <span className="rounded-full bg-[var(--brand-primary)] px-1.5 text-xs text-[var(--brand-on)]">{changed.length}</span> : undefined },
      { value: 'monthly', label: t('attendanceMonthly') },
      ...(canAlerts ? [{ value: 'alerts' as Tab, label: t('studentAttendanceAlerts') }] : []),
    ]} />

    {tab === 'daily' && <>
      {message && <Notice tone={message.tone} action={message.reload ? <button type="button" className={ui.secondary} onClick={() => void reloadRoster()}>{t('attnReload')}</button> : undefined}>{message.text}</Notice>}
      {correcting && dateValid && canManage && <Notice tone="info">{t('attnCorrectionMode', { date })}</Notice>}
      {!dateValid && session && <Notice tone="warning">{t('attnInvalidDay')}</Notice>}

      {canManage && <QrPanel open={qrOpen} onToggle={() => setQrOpen(v => !v)} date={date} disabled={!dateValid} onScanned={(studentId, ok, text) => {
        if (ok) setDrafts(previous => { const copy = { ...previous }; for (const key of Object.keys(copy)) if (key.toLowerCase() === studentId.toLowerCase()) delete copy[key]; return copy })
        setMessage({ tone: ok ? 'success' : 'error', text })
        if (ok) void cache.invalidateQueries({ queryKey: ['attendance'] })
      }} authorized={authorized} />}

      <section className={`${ui.card} overflow-hidden`} aria-labelledby="roster-heading">
        <div className="border-b border-slate-100 p-4 sm:p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div><h2 id="roster-heading" className="text-lg font-bold text-slate-900">{section ? `${section.class_name} · ${section.name}` : t('attendanceDaily')}</h2>
              <p className="text-sm text-slate-600">{new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${date}T00:00:00Z`))}</p></div>
            {canManage && items.length > 0 && <div className="flex flex-wrap gap-2">
              <button type="button" className={ui.secondary} disabled={counts.unmarked === 0} onClick={markAllPresent}><span aria-hidden="true" className={`h-2 w-2 rounded-full ${tones.present.dot}`} />{t('attnMarkAllPresent')}</button>
              {dirty && <button type="button" className={ui.secondary} onClick={() => { setDrafts({}); setStale([]) }}>{t('attnDiscard')}</button>}
            </div>}
          </div>
          {items.length > 0 && <>
            <dl className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
              <StatTile tone="present" label={t('studentAttendancePresent')} value={num(counts.present)} />
              <StatTile tone="absent" label={t('studentAttendanceAbsent')} value={num(counts.absent)} />
              <StatTile tone="late" label={t('studentAttendanceLate')} value={num(counts.late)} />
              <StatTile tone="unmarked" label={t('attendanceUnmarked')} value={num(counts.unmarked)} />
            </dl>
            <div className="mt-3"><div className="flex justify-between text-xs font-medium text-slate-500"><span>{t('attnProgress', { marked, total: items.length })}</span><span className="tabular-nums">{num(Math.round((marked / items.length) * 100))}%</span></div>
              <div className="mt-1 flex h-2 overflow-hidden rounded-full bg-slate-100" role="img" aria-label={t('attnProgress', { marked, total: items.length })}>
                {(['present', 'late', 'absent'] as const).map(s => <div key={s} className={tones[s].dot} style={{ width: `${(counts[s] / items.length) * 100}%` }} />)}</div></div>
            <div className="mt-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex gap-1.5 overflow-x-auto pb-1" role="group" aria-label={t('attnFilter')}>{filters.map(f => <button key={f.value} type="button" aria-pressed={filter === f.value} onClick={() => setFilter(f.value)} className={`shrink-0 rounded-full border px-3 py-1.5 text-xs font-semibold transition ${filter === f.value ? 'border-[var(--brand-primary)] bg-[var(--brand-primary)] text-[var(--brand-on)]' : 'border-slate-200 bg-white text-slate-600 hover:border-slate-300'}`}>{f.label} <span className="tabular-nums opacity-80">{f.count}</span></button>)}</div>
              <label className="sr-only" htmlFor="attn-search">{t('studentAttendanceSearch')}</label>
              <input id="attn-search" type="search" className={`${ui.field} mt-0 lg:max-w-xs`} placeholder={t('attnSearchPlaceholder')} value={search} onChange={event => setSearch(event.target.value)} />
            </div>
          </>}
        </div>

        <div className="p-2 sm:p-3">
          {!sectionId ? <EmptyState title={t('attnChooseSection')} /> : !dateValid ? null : roster.isPending ? <Skeleton rows={6} /> : roster.isError ? <p role="alert" className="p-3 text-rose-700">{t('attendanceLoadError')}</p> : items.length === 0 ? <EmptyState title={t('studentAttendanceEmpty')} /> : visible.length === 0 ? <EmptyState title={t('attnNoMatches')} /> :
            <ul className="divide-y divide-slate-100" aria-label={t('attnRosterList')}>{visible.map(item => {
              const draft = drafts[item.student_id]
              const status = effective(item)
              const isStale = stale.includes(item.student_id.toLowerCase())
              const display = name(item.name_en, item.name_bn)
              const noteValue = draft?.note ?? item.note ?? ''
              return <li key={item.student_id} className={`relative rounded-xl px-2 py-3 sm:px-3 ${isStale ? 'bg-amber-50' : draft ? 'bg-[color-mix(in_srgb,var(--brand-primary)_6%,white)]' : ''}`}>
                {draft && <span aria-hidden="true" className="absolute inset-y-2 left-0 w-1 rounded-full bg-[var(--brand-primary)]" />}
                <div className="flex flex-col gap-3 md:flex-row md:items-center">
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    <span className="w-9 shrink-0 text-center font-mono text-lg font-bold tabular-nums text-slate-500" aria-label={t('attnRoll', { roll: item.roll_no ?? '—' })}>{item.roll_no == null ? '—' : num(item.roll_no)}</span>
                    <Avatar name={item.name_en} />
                    <div className="min-w-0"><p className="truncate font-semibold text-slate-900">{display}</p>
                      <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-slate-500"><span>{item.admission_no}</span>
                        {item.status && !draft && <StatusBadge tone={item.status}>{t('attnSaved')}{item.source === 'qr' ? ' · QR' : ''}</StatusBadge>}
                        {draft && <span className="font-semibold text-[var(--brand-ink)]">{item.record_id ? t('attnCorrection') : t('attnUnsaved')}</span>}
                        {isStale && <StatusBadge tone="late">{t('attnChangedElsewhere')}</StatusBadge>}</p></div>
                  </div>
                  <div className="flex items-center gap-2 md:shrink-0">
                    <SegmentedStatus name={t('attnStatusFor', { name: display })} value={status} options={options} disabled={!canManage || saving} compact onChange={value => setDraft(item, { status: value })} />
                    {canManage && <button type="button" className={`${ui.ghost} shrink-0`} aria-expanded={!!notesOpen[item.student_id] || !!noteValue} aria-label={t('attnNoteFor', { name: display })} onClick={() => setNotesOpen(v => ({ ...v, [item.student_id]: !v[item.student_id] }))}>
                      <svg aria-hidden="true" viewBox="0 0 20 20" className="h-4 w-4 fill-current"><path d="M4 3h9l3 3v11H4zm2 5v1.5h8V8zm0 3v1.5h8V11zm0 3v1.5h5V14z" /></svg><span className="hidden lg:inline">{t('attendanceNote')}</span></button>}
                  </div>
                </div>
                {(notesOpen[item.student_id] || noteValue) && <label className="mt-2 block md:ml-[5.5rem]"><span className="sr-only">{t('attnNoteFor', { name: display })}</span>
                  <input className={`${ui.field} mt-0`} maxLength={500} disabled={!canManage || saving} placeholder={t('attnNotePlaceholder')} value={noteValue} onChange={event => setDraft(item, { note: event.target.value })} /></label>}
              </li>
            })}</ul>}
        </div>

        {canManage && items.length > 0 && <div className="sticky bottom-0 z-10 flex flex-col gap-3 border-t border-slate-200 bg-white/95 p-4 backdrop-blur sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-slate-600" aria-live="polite">{dirty ? t('attnPending', { count: changed.length }) : t('attnNoPending')}{counts.unmarked > 0 && <span className="block text-xs text-slate-500">{t('attnUnmarkedStay', { count: counts.unmarked })}</span>}</p>
          <button type="button" className={`${ui.primary} sm:min-w-56`} disabled={!dirty || saving || changed.every(item => !drafts[item.student_id].status)} onClick={() => void save()}>{saving ? t('attnSaving') : dirty ? t('attnSaveRoster', { count: changed.length }) : t('attnSaveIdle')}</button>
        </div>}
      </section>
    </>}

    {tab === 'monthly' && <MonthlyPanel sessionId={sessionId} sectionId={sectionId} authorized={authorized} />}
    {tab === 'alerts' && canAlerts && <AlertsPanel authorized={authorized} />}
  </div>
}

type Authorized = <T>(fn: (token: string) => Promise<T>) => Promise<T>

function QrPanel({ open, onToggle, date, disabled, onScanned, authorized }: { open: boolean; onToggle: () => void; date: string; disabled: boolean; onScanned: (studentId: string, ok: boolean, text: string) => void; authorized: Authorized }) {
  const { t } = useTranslation()
  const [qr, setQr] = useState('')
  const [cameraOn, setCameraOn] = useState(false)
  const [cameraError, setCameraError] = useState('')
  const video = useRef<HTMLVideoElement>(null)
  async function scan(payload: string) {
    const value = payload.trim()
    if (!value) return
    try {
      await authorized(token => scanStudentAttendance(token, { payload: value, attendance_date: date }))
      setQr(''); onScanned(value.replace(/^FB-STUDENT:/, ''), true, t('studentAttendanceScanned'))
    } catch (cause) { onScanned('', false, t(cause instanceof ApiError && cause.code === 'duplicate_attendance' ? 'studentAttendanceDuplicate' : 'studentAttendanceScanError')) }
  }
  useEffect(() => {
    if (!cameraOn) return
    let cancelled = false, frame = 0, stream: MediaStream | undefined
    const ctor = (window as Window & { BarcodeDetector?: DetectorConstructor }).BarcodeDetector
    if (!ctor) { setCameraError(t('studentAttendanceCameraUnsupported')); setCameraOn(false); return }
    const detector = new ctor({ formats: ['qr_code'] })
    async function start() {
      try {
        stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } })
        if (!video.current || cancelled) { stream.getTracks().forEach(track => track.stop()); return }
        video.current.srcObject = stream; await video.current.play()
        async function loop() { if (cancelled || !video.current) return; try { const codes = await detector.detect(video.current); if (codes.length) { setCameraOn(false); void scan(codes[0].rawValue); return } } catch { setCameraError(t('studentAttendanceCameraError')) } frame = requestAnimationFrame(() => void loop()) }
        void loop()
      } catch { setCameraError(t('studentAttendanceCameraError')); setCameraOn(false) }
    }
    void start()
    return () => { cancelled = true; cancelAnimationFrame(frame); stream?.getTracks().forEach(track => track.stop()) }
  }, [cameraOn, date])
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="qr-heading">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-3"><span aria-hidden="true" className="flex h-10 w-10 items-center justify-center rounded-xl bg-slate-900 text-white"><svg viewBox="0 0 20 20" className="h-5 w-5 fill-current"><path d="M3 3h6v6H3zm2 2v2h2V5zm6-2h6v6h-6zm2 2v2h2V5zM3 11h6v6H3zm2 2v2h2v-2zm6-2h2v2h-2zm4 0h2v4h-2zm-4 4h4v2h-4z" /></svg></span>
        <div><h2 id="qr-heading" className="font-bold text-slate-900">{t('studentAttendanceQR')}</h2><p className="text-sm text-slate-600">{t('attnQrShort')}</p></div></div>
      <button type="button" className={ui.secondary} aria-expanded={open} aria-controls="qr-body" onClick={onToggle}>{open ? t('attnHideScanner') : t('attnShowScanner')}</button>
    </div>
    {open && <div id="qr-body" className="mt-4 space-y-3 border-t border-slate-100 pt-4">
      <p className="text-sm text-slate-600">{t('studentAttendanceQRHelp')}</p>
      <form className="flex flex-col gap-2 sm:flex-row sm:items-end" onSubmit={(event: FormEvent) => { event.preventDefault(); void scan(qr) }}>
        <label className={`${ui.label} flex-1`}>{t('studentAttendanceQRValue')}<input className={ui.field} value={qr} disabled={disabled} onChange={event => setQr(event.target.value)} autoComplete="off" placeholder="FB-STUDENT:…" /></label>
        <button className={ui.primary} type="submit" disabled={disabled || !qr.trim()}>{t('studentAttendanceScan')}</button>
        <button className={ui.secondary} type="button" disabled={disabled} onClick={() => { setCameraError(''); setCameraOn(v => !v) }}>{t(cameraOn ? 'studentAttendanceStopCamera' : 'studentAttendanceStartCamera')}</button>
      </form>
      {cameraError && <p role="alert" className="text-sm text-rose-700">{cameraError}</p>}
      {cameraOn && <video ref={video} playsInline muted className="max-h-64 w-full rounded-xl bg-black object-contain" />}
    </div>}
  </section>
}

function MonthlyPanel({ sessionId, sectionId, authorized }: { sessionId: string; sectionId: string; authorized: Authorized }) {
  const { t, i18n } = useTranslation()
  const [month, setMonth] = useState(today.slice(0, 7))
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const params = new URLSearchParams({ session_id: sessionId, section_id: sectionId, month, search, page: String(page), page_size: '20' })
  const monthly = useQuery({ queryKey: ['attendance', 'student-monthly', params.toString()], queryFn: () => authorized(token => getStudentAttendanceMonthly(token, params)), enabled: !!sectionId, retry: false })
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="monthly-heading">
    <div className="flex flex-wrap items-end justify-between gap-3"><h2 id="monthly-heading" className="text-lg font-bold">{t('attendanceMonthly')}</h2>
      <div className="grid w-full gap-3 sm:w-auto sm:grid-cols-2">
        <label className={ui.label}>{t('attendanceMonth')}<input className={ui.field} type="month" value={month} onChange={event => { setMonth(event.target.value); setPage(1) }} /></label>
        <label className={ui.label}>{t('studentAttendanceSearch')}<input className={ui.field} type="search" value={search} onChange={event => { setSearch(event.target.value); setPage(1) }} /></label>
      </div></div>
    {!sectionId ? <div className="mt-4"><EmptyState title={t('attnChooseSection')} /></div> : monthly.isPending ? <div className="mt-4"><Skeleton /></div> : monthly.isError ? <p role="alert" className="mt-4 text-rose-700">{t('attendanceLoadError')}</p> : !monthly.data.items.length ? <div className="mt-4"><EmptyState title={t('attendanceNoReport')} /></div> : <>
      <div className="mt-4 overflow-x-auto"><table className="w-full min-w-[34rem] text-left text-sm">
        <thead><tr className="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500"><th className="py-2 pr-3 font-semibold">{t('attnStudent')}</th><th className="px-2 py-2 text-right font-semibold">{t('studentAttendancePresent')}</th><th className="px-2 py-2 text-right font-semibold">{t('studentAttendanceAbsent')}</th><th className="px-2 py-2 text-right font-semibold">{t('studentAttendanceLate')}</th><th className="py-2 pl-3 font-semibold">{t('attnRate')}</th></tr></thead>
        <tbody>{monthly.data.items.map(item => {
          const rate = item.total ? Math.round(((item.present + item.late) / item.total) * 100) : null
          return <tr key={item.student_id} className="border-b border-slate-100">
            <td className="py-2.5 pr-3"><p className="font-semibold text-slate-900">{i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en}</p><p className="text-xs text-slate-500">{item.admission_no}</p></td>
            <td className={`px-2 text-right font-semibold tabular-nums ${tones.present.text}`}>{item.present}</td><td className={`px-2 text-right font-semibold tabular-nums ${tones.absent.text}`}>{item.absent}</td><td className={`px-2 text-right font-semibold tabular-nums ${tones.late.text}`}>{item.late}</td>
            <td className="py-2.5 pl-3">{rate === null ? <span className="text-xs text-slate-500">{t('attnNoDays')}</span> : <div className="flex items-center gap-2"><div className="h-1.5 w-24 overflow-hidden rounded-full bg-slate-100"><div className={`h-1.5 ${rate >= 75 ? tones.present.dot : rate >= 50 ? tones.late.dot : tones.absent.dot}`} style={{ width: `${rate}%` }} /></div><span className="text-xs font-semibold tabular-nums text-slate-700">{rate}%</span></div>}</td>
          </tr>
        })}</tbody></table></div>
      <p className="mt-2 text-xs text-slate-500">{t('attnRateHint')}</p>
      <Pager page={page} total={monthly.data.total} size={20} onPage={setPage} previous={t('attendancePrevious')} next={t('attendanceNext')} label={t('attendanceMonthly')} />
    </>}
  </section>
}

function AlertsPanel({ authorized }: { authorized: Authorized }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const alerts = useQuery({ queryKey: ['attendance', 'alerts', page], queryFn: () => authorized(token => getAbsenceEvents(token, new URLSearchParams({ page: String(page), page_size: '20' }))), retry: false })
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="alerts-heading">
    <h2 id="alerts-heading" className="text-lg font-bold">{t('studentAttendanceAlerts')}</h2><p className="text-sm text-slate-600">{t('attnAlertsHint')}</p>
    {alerts.isPending ? <div className="mt-4"><Skeleton rows={3} /></div> : alerts.isError ? <p role="alert" className="mt-4 text-rose-700">{t('attendanceLoadError')}</p> : !alerts.data.items.length ? <div className="mt-4"><EmptyState title={t('attnNoAlerts')} /></div> : <>
      <ul className="mt-4 divide-y divide-slate-100">{alerts.data.items.map(item => <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
        <div className="flex items-center gap-3"><Avatar name={item.name_en} /><div><p className="font-semibold text-slate-900">{item.name_en}</p><p className="text-xs text-slate-500">{item.attendance_date}</p></div></div>
        <div className="flex items-center gap-2"><StatusBadge tone="absent">{t('studentAttendanceAbsent')}</StatusBadge><span className="text-xs text-slate-600">{t('studentAttendanceLinkedGuardians')}: {item.guardian_count}</span></div>
      </li>)}</ul>
      <Pager page={page} total={alerts.data.total} size={20} onPage={setPage} previous={t('attendancePrevious')} next={t('attendanceNext')} label={t('studentAttendanceAlerts')} />
    </>}
  </section>
}

export function StudentAttendanceReportPage({ guardian }: { guardian: boolean }) {
  const { id } = useParams()
  const { t, i18n } = useTranslation()
  const authorized = useAuthorized()
  const [month, setMonth] = useState(today.slice(0, 7))
  const report = useQuery({ queryKey: ['attendance', 'student-report', guardian, id, month], queryFn: () => authorized(token => guardian ? getGuardianStudentAttendanceReport(token, id!, month) : getOwnStudentAttendanceReport(token, month)), enabled: !guardian || !!id, retry: false })
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  const [year, monthIndex] = month.split('-').map(Number)
  const days = new Date(Date.UTC(year, monthIndex, 0)).getUTCDate()
  const offset = (new Date(Date.UTC(year, monthIndex - 1, 1)).getUTCDay() + 6) % 7
  const byDay = new Map(report.data?.items.map(item => [Number(item.attendance_date.slice(8)), item]) ?? [])
  const weekdays = Array.from({ length: 7 }, (_, i) => new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' }).format(new Date(Date.UTC(2024, 0, 1 + i))))
  const summary = report.data?.student
  const rate = summary && summary.total ? Math.round(((summary.present + summary.late) / summary.total) * 100) : null
  const num = (n: number) => new Intl.NumberFormat(locale).format(n)
  const statusText = (s: Status) => t(`studentAttendance${s[0].toUpperCase()}${s.slice(1)}`)
  return <div className="space-y-5">
    <PageHeader eyebrow={t(guardian ? 'attnGuardianEyebrow' : 'attnStudentEyebrow')} title={summary ? (i18n.language === 'bn' && summary.name_bn ? summary.name_bn : summary.name_en) : t('studentAttendanceReport')} description={t('studentAttendanceReport')}
      actions={<label className={ui.label}>{t('attendanceMonth')}<input className={ui.field} type="month" value={month} onChange={event => event.target.value && setMonth(event.target.value)} /></label>} />
    {report.isPending ? <Skeleton rows={4} /> : report.isError ? <Notice tone="error">{t('attendanceLoadError')}</Notice> : <>
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile tone="present" label={t('studentAttendancePresent')} value={num(summary!.present)} />
        <StatTile tone="absent" label={t('studentAttendanceAbsent')} value={num(summary!.absent)} />
        <StatTile tone="late" label={t('studentAttendanceLate')} value={num(summary!.late)} />
        <StatTile tone="unmarked" label={t('attnRate')} value={rate === null ? '—' : `${num(rate)}%`} hint={t('attnRecordedDays', { count: summary!.total })} />
      </dl>
      <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="calendar-heading">
        <h2 id="calendar-heading" className="text-lg font-bold">{new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(year, monthIndex - 1, 1)))}</h2>
        <div className="mt-4 grid grid-cols-7 gap-1.5 text-center" role="grid" aria-label={t('attnCalendar')}>
          {weekdays.map(day => <div key={day} role="columnheader" className="pb-1 text-xs font-semibold text-slate-500">{day}</div>)}
          {Array.from({ length: offset }, (_, i) => <div key={`blank-${i}`} aria-hidden="true" />)}
          {Array.from({ length: days }, (_, i) => {
            const record = byDay.get(i + 1)
            const label = `${new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(year, monthIndex - 1, i + 1)))}: ${record ? statusText(record.status) : t('attnNoRecord')}`
            return <div key={i} role="gridcell" aria-label={label} title={record?.note || label} className={`flex aspect-square flex-col items-center justify-center rounded-lg text-sm font-semibold ${record ? tones[record.status].solid : 'bg-slate-50 text-slate-500'}`}>
              <span>{new Intl.NumberFormat(locale).format(i + 1)}</span>{record && <span className="text-[0.6rem] font-semibold">{t(`attnShort${record.status[0].toUpperCase()}${record.status.slice(1)}`)}</span>}</div>
          })}
        </div>
        <div className="mt-4 flex flex-wrap gap-3 text-xs text-slate-600">{(['present', 'absent', 'late'] as const).map(s => <span key={s} className="flex items-center gap-1.5"><span aria-hidden="true" className={`h-2.5 w-2.5 rounded ${tones[s].dot}`} />{statusText(s)}</span>)}</div>
      </section>
      {report.data.items.length > 0 && <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="records-heading"><h2 id="records-heading" className="text-lg font-bold">{t('attnRecords')}</h2>
        <ul className="mt-3 divide-y divide-slate-100">{report.data.items.map(item => <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 py-2.5"><span className="text-sm text-slate-700">{new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'short', timeZone: 'UTC' }).format(new Date(`${item.attendance_date}T00:00:00Z`))}{item.note && <span className="block text-xs text-slate-500">{item.note}</span>}</span><StatusBadge tone={item.status}>{statusText(item.status)}</StatusBadge></li>)}</ul></section>}
    </>}
  </div>
}
