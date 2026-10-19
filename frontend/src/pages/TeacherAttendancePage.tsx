import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, correctTeacherAttendance, getAttendanceSessions, getOwnTeacherAttendance, getOwnTeacherMonthly, getTeacherMonthlyReport, getTeacherRoster, recordTeacherAttendance, type AttendanceSession, type TeacherRosterItem } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { Avatar, EmptyState, Notice, PageHeader, Pager, SegmentedStatus, Skeleton, StatTile, StatusBadge, tones, ui, type SegmentOption } from './attendanceUi'

const today = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Dhaka', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
type Status = 'present' | 'absent' | 'leave'
type Draft = { status: Status | ''; note: string }
const sessionParams = new URLSearchParams({ page: '1', page_size: '100' })

export function TeacherAttendancePage({ permissions, own = false }: { permissions: string[]; own?: boolean }) {
  const { t, i18n } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const cache = useQueryClient()
  const [sessionId, setSessionId] = useState('')
  const [date, setDate] = useState(today)
  const [month, setMonth] = useState(today.slice(0, 7))
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [historyPage, setHistoryPage] = useState(1)
  const [reportPage, setReportPage] = useState(1)
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [busyId, setBusyId] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const canManage = !own && permissions.includes('attendance:teachers_manage')
  const canReport = !own && permissions.includes('attendance:teachers_reports')
  const options: SegmentOption<Status>[] = [
    { value: 'present', label: t('attendancePresent'), short: t('attnShortPresent'), tone: 'present', hotkey: 'p' },
    { value: 'absent', label: t('attendanceAbsent'), short: t('attnShortAbsent'), tone: 'absent', hotkey: 'a' },
    { value: 'leave', label: t('attendanceLeave'), short: t('attnShortLeave'), tone: 'leave', hotkey: 'l' },
  ]
  async function authorized<T>(fn: (token: string) => Promise<T>): Promise<T> {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await fn(accessToken) } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await fn(await renew()) } catch (retry) { if (retry instanceof ApiError && retry.status === 401) expire(); throw retry }
      }
      throw cause
    }
  }
  const sessions = useQuery({ queryKey: ['attendance', 'sessions'], queryFn: () => authorized(token => getAttendanceSessions(token, sessionParams)), retry: false })
  useEffect(() => {
    if (sessionId || !sessions.data?.items.length) return
    setSessionId((sessions.data.items.find(item => item.is_active) || sessions.data.items.find(item => item.start_date <= date && date <= item.end_date) || sessions.data.items[0]).id)
  }, [date, sessionId, sessions.data])
  const currentSession: AttendanceSession | undefined = sessions.data?.items.find(item => item.id === sessionId)
  const validDay = !!currentSession && date >= currentSession.start_date && date <= currentSession.end_date
  const validMonth = !!currentSession && `${month}-31` >= currentSession.start_date && `${month}-01` <= currentSession.end_date
  const rosterParams = new URLSearchParams({ session_id: sessionId, date, search, page: String(page), page_size: '10' })
  const roster = useQuery({ queryKey: ['attendance', 'roster', rosterParams.toString()], queryFn: () => authorized(token => getTeacherRoster(token, rosterParams)), enabled: !own && !!sessionId && validDay, retry: false })
  const historyParams = new URLSearchParams({ session_id: sessionId, page: String(historyPage), page_size: '10' })
  const history = useQuery({ queryKey: ['attendance', 'history', historyParams.toString()], queryFn: () => authorized(token => getOwnTeacherAttendance(token, historyParams)), enabled: own && !!sessionId, retry: false })
  const reportParams = new URLSearchParams({ session_id: sessionId, month, search, page: String(reportPage), page_size: '10' })
  const report = useQuery({ queryKey: ['attendance', 'monthly', reportParams.toString()], queryFn: () => authorized(token => getTeacherMonthlyReport(token, reportParams)), enabled: canReport && validMonth, retry: false })
  const ownMonthParams = new URLSearchParams({ session_id: sessionId, month })
  const ownMonth = useQuery({ queryKey: ['attendance', 'my-monthly', ownMonthParams.toString()], queryFn: () => authorized(token => getOwnTeacherMonthly(token, ownMonthParams)), enabled: own && validMonth, retry: false })
  const statusLabel = (status: Status | null) => t(status ? `attendance${status[0].toUpperCase()}${status.slice(1)}` : 'attendanceUnmarked')
  const name = (en: string, bn: string) => (i18n.language === 'bn' && bn ? bn : en)
  const updateDraft = (item: TeacherRosterItem, patch: Partial<Draft>) => setDrafts(previous => ({ ...previous, [item.employee_id]: { status: previous[item.employee_id]?.status ?? item.status ?? '', note: previous[item.employee_id]?.note ?? item.note ?? '', ...patch } }))
  async function save(item: TeacherRosterItem) {
    const draft = drafts[item.employee_id] || { status: item.status || '', note: item.note || '' }
    if (!draft.status) return
    setBusyId(item.employee_id); setError(''); setMessage('')
    try {
      if (item.record_id) await authorized(token => correctTeacherAttendance(token, item.record_id!, { status: draft.status as Status, note: draft.note }))
      else await authorized(token => recordTeacherAttendance(token, { session_id: sessionId, employee_id: item.employee_id, attendance_date: date, status: draft.status as Status, note: draft.note }))
      setMessage('attendanceSaved')
      setDrafts(previous => { const next = { ...previous }; delete next[item.employee_id]; return next })
      await cache.invalidateQueries({ queryKey: ['attendance'] })
    } catch (cause) {
      setError(cause instanceof ApiError && cause.code === 'duplicate_attendance' ? 'attendanceDuplicate' : cause instanceof ApiError && cause.code === 'attendance_conflict' ? 'attendanceConflict' : 'attendanceSaveError')
    } finally { setBusyId('') }
  }
  const pager = (current: number, total: number, set: (page: number) => void) => <Pager page={current} total={total} size={10} onPage={set} previous={t('attendancePrevious')} next={t('attendanceNext')} label={t('attendancePage')} />

  // Someone with the permission but no linked staff profile has no attendance
  // to show; explain that once instead of failing every panel.
  const noProfile = own && [history.error, ownMonth.error].some(e => e instanceof ApiError && e.status === 404)
  if (noProfile) return <div className="space-y-5">
    <PageHeader eyebrow={t('attnEyebrow')} title={t('attendanceMyHeading')} description={t('attendanceMyDescription')} />
    <EmptyState icon="user" title={t('staffProfileMissing')}>{t('uxStaffMissingHint')}</EmptyState>
  </div>
  return <div className="space-y-5">
    <PageHeader eyebrow={t('attnEyebrow')} title={t(own ? 'attendanceMyHeading' : 'attendanceHeading')} description={t(own ? 'attendanceMyDescription' : 'attendanceDescription')} />
    {message && <Notice tone="success">{t(message)}</Notice>}{error && <Notice tone="error">{t(error)}</Notice>}
    <section className={`${ui.card} p-4 sm:p-5`}>
      <div className={`grid gap-4 ${own ? 'sm:max-w-sm' : 'sm:grid-cols-3'}`}>
        <label className={ui.label}>{t('attendanceSession')}<select className={ui.field} value={sessionId} onChange={event => { setSessionId(event.target.value); setPage(1); setHistoryPage(1); setReportPage(1) }}><option value="">—</option>{sessions.data?.items.map(item => <option key={item.id} value={item.id}>{item.name}{item.is_active ? ` · ${t('attnActive')}` : ''}</option>)}</select></label>
        {!own && <label className={ui.label}>{t('attendanceDate')}<input className={ui.field} type="date" min={currentSession?.start_date} max={currentSession?.end_date} value={date} onChange={event => { setDate(event.target.value); setPage(1); setDrafts({}) }} /></label>}
        {!own && <label className={ui.label}>{t('attendanceSearch')}<input className={ui.field} type="search" value={search} onChange={event => { setSearch(event.target.value); setPage(1) }} /></label>}
      </div>
      {sessions.isError ? <p role="alert" className="mt-3 text-sm text-rose-700">{t('attendanceLoadError')}</p> : sessions.data && !sessions.data.items.length ? <p className="mt-3 text-sm text-slate-600">{t('attendanceNoSessions')}</p> : null}
    </section>

    {!own && sessionId && <section className={`${ui.card} p-2 sm:p-3`} aria-labelledby="teacher-roster-heading">
      <h2 id="teacher-roster-heading" className="px-2 pt-2 text-lg font-bold sm:px-3">{t('attendanceDaily')}</h2>
      {!validDay ? <div className="p-3"><Notice tone="warning">{t('attendanceInvalidDate')}</Notice></div> : roster.isPending ? <div className="px-3 py-2.5"><Skeleton /></div> : roster.isError ? <p role="alert" className="p-3 text-rose-700">{t('attendanceLoadError')}</p> : !roster.data?.items.length ? <div className="p-3"><EmptyState title={t('attendanceNoTeachers')} /></div> : <>
        <ul className="mt-2 divide-y divide-slate-100">{roster.data.items.map(item => {
          const draft = drafts[item.employee_id]
          const display = name(item.name_en, item.name_bn)
          const status = draft?.status ?? item.status ?? ''
          const dirty = !!draft && (draft.status !== (item.status ?? '') || draft.note !== (item.note ?? ''))
          return <li key={item.employee_id} className={`rounded-xl px-2 py-3 sm:px-3 ${dirty ? 'bg-[var(--brand-soft)]' : ''}`}>
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
              <div className="flex min-w-0 flex-1 items-center gap-3"><Avatar name={item.name_en} /><div className="min-w-0"><p className="truncate font-semibold text-slate-900">{display}</p>
                <p className="flex flex-wrap items-center gap-2 text-xs text-slate-500"><span>{item.employee_code}</span>{item.status ? <StatusBadge tone={item.status}>{t('attnSaved')} · {statusLabel(item.status)}</StatusBadge> : <StatusBadge tone="unmarked">{t('attendanceUnmarked')}</StatusBadge>}</p></div></div>
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <SegmentedStatus name={t('attnStatusFor', { name: display })} value={status} options={options} compact disabled={!canManage || busyId === item.employee_id} onChange={value => updateDraft(item, { status: value })} />
                <label className="sm:w-48"><span className="sr-only">{t('attnNoteFor', { name: display })}</span><input className={`${ui.field} mt-0`} placeholder={t('attendanceNote')} disabled={!canManage} maxLength={500} value={draft?.note ?? item.note ?? ''} onChange={event => updateDraft(item, { note: event.target.value })} /></label>
                {canManage && <button type="button" className={ui.primary} disabled={!dirty || !status || busyId === item.employee_id} onClick={() => void save(item)}>{busyId === item.employee_id ? t('attnSaving') : t('attendanceSave')}</button>}
              </div>
            </div>
          </li>
        })}</ul>
        <div className="px-2 pb-2 sm:px-3">{pager(page, roster.data.total, setPage)}</div>
      </>}
    </section>}

    {own && sessionId && <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="own-history-heading"><h2 id="own-history-heading" className="text-lg font-bold">{t('attendanceDaily')}</h2>
      {history.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : history.isError ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceLoadError')}</p> : history.data?.items.length ? <>
        <ul className="mt-3 divide-y divide-slate-100">{history.data.items.map(item => <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 py-2.5"><span className="text-sm text-slate-700">{item.attendance_date} · {item.session_name}</span><StatusBadge tone={item.status}>{statusLabel(item.status)}</StatusBadge></li>)}</ul>
        {pager(historyPage, history.data.total, setHistoryPage)}</> : <div className="mt-3"><EmptyState title={t('attendanceNoHistory')} /></div>}
    </section>}

    {sessionId && (own || canReport) && <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="teacher-monthly-heading">
      <div className="flex flex-wrap items-end justify-between gap-3"><h2 id="teacher-monthly-heading" className="text-lg font-bold">{t('attendanceMonthly')}</h2>
        <label className={ui.label}>{t('attendanceMonth')}<input className={ui.field} type="month" value={month} onChange={event => { setMonth(event.target.value); setReportPage(1) }} /></label></div>
      {!validMonth ? <div className="mt-3"><Notice tone="warning">{t('attendanceInvalidDate')}</Notice></div> : own ? ownMonth.isPending ? <div className="mt-3"><Skeleton rows={1} /></div> : ownMonth.isError ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceLoadError')}</p> :
        <dl className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4"><StatTile tone="present" label={t('attendancePresent')} value={ownMonth.data?.present ?? 0} /><StatTile tone="absent" label={t('attendanceAbsent')} value={ownMonth.data?.absent ?? 0} /><StatTile tone="leave" label={t('attendanceLeave')} value={ownMonth.data?.leave ?? 0} /><StatTile tone="unmarked" label={t('attendanceTotal')} value={ownMonth.data?.total ?? 0} /></dl>
        : report.isPending ? <div className="mt-3"><Skeleton /></div> : report.isError ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceLoadError')}</p> : report.data?.items.length ? <>
          <div className="mt-4 overflow-x-auto"><table className="w-full min-w-[32rem] text-left text-sm"><thead><tr className="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500"><th className="py-2 pr-3 font-semibold">{t('attendanceName')}</th><th className="px-2 text-right font-semibold">{t('attendancePresent')}</th><th className="px-2 text-right font-semibold">{t('attendanceAbsent')}</th><th className="px-2 text-right font-semibold">{t('attendanceLeave')}</th><th className="pl-2 text-right font-semibold">{t('attendanceTotal')}</th></tr></thead>
            <tbody>{report.data.items.map(item => <tr key={item.employee_id} className="border-b border-slate-100"><td className="py-2.5 pr-3"><p className="font-semibold text-slate-900">{name(item.name_en, item.name_bn)}</p><p className="text-xs text-slate-500">{item.employee_code}</p></td>
              <td className={`px-2 text-right font-semibold tabular-nums ${tones.present.text}`}>{item.present}</td><td className={`px-2 text-right font-semibold tabular-nums ${tones.absent.text}`}>{item.absent}</td><td className={`px-2 text-right font-semibold tabular-nums ${tones.leave.text}`}>{item.leave}</td><td className="pl-2 text-right font-semibold tabular-nums">{item.total}</td></tr>)}</tbody></table></div>
          {pager(reportPage, report.data.total, setReportPage)}</> : <div className="mt-3"><EmptyState title={t('attendanceNoReport')} /></div>}
    </section>}
  </div>
}
