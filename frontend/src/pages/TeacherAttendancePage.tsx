import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, correctTeacherAttendance, getAttendanceSessions, getOwnTeacherAttendance, getOwnTeacherMonthly, getTeacherMonthlyReport, getTeacherRoster, recordTeacherAttendance, type AttendanceSession, type TeacherRosterItem } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

const fieldClass = 'mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:border-[var(--brand-primary)] focus:outline-none'
const card = 'rounded-2xl border border-slate-200 bg-white p-5 shadow-sm'
const button = 'rounded-lg bg-[var(--brand-primary)] px-4 py-2 font-semibold text-white disabled:opacity-50'
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
    setSessionId(sessions.data.items.find(item => item.start_date <= date && date <= item.end_date)?.id || sessions.data.items[0].id)
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
  return <div className="space-y-6">
    <header><h1 className="text-3xl font-bold text-slate-900">{t(own ? 'attendanceMyHeading' : 'attendanceHeading')}</h1><p className="mt-2 text-slate-600">{t(own ? 'attendanceMyDescription' : 'attendanceDescription')}</p></header>
    {message && <p role="status" className="rounded-lg bg-emerald-50 p-3 text-emerald-800">{t(message)}</p>}{error && <p role="alert" className="rounded-lg bg-rose-50 p-3 text-rose-800">{t(error)}</p>}
    <section className={card}><label className="block max-w-sm text-sm font-semibold text-slate-700">{t('attendanceSession')}<select className={fieldClass} value={sessionId} onChange={event => { setSessionId(event.target.value); setPage(1); setHistoryPage(1); setReportPage(1) }}><option value="">—</option>{sessions.data?.items.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>{sessions.isPending ? <p role="status" className="mt-3">{t('attendanceLoading')}</p> : sessions.isError ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceLoadError')}</p> : !sessions.data?.items.length ? <p className="mt-3 text-slate-600">{t('attendanceNoSessions')}</p> : null}</section>
    {!own && sessionId && <section className={card}><h2 className="text-xl font-semibold">{t('attendanceDaily')}</h2><div className="mt-3 grid gap-3 sm:grid-cols-2"> <label className="text-sm font-semibold text-slate-700">{t('attendanceDate')}<input className={fieldClass} type="date" min={currentSession?.start_date} max={currentSession?.end_date} value={date} onChange={event => { setDate(event.target.value); setPage(1); setDrafts({}) }} /></label><label className="text-sm font-semibold text-slate-700">{t('attendanceSearch')}<input className={fieldClass} value={search} onChange={event => { setSearch(event.target.value); setPage(1) }} /></label></div>
      {!validDay ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceInvalidDate')}</p> : roster.isPending ? <p role="status" className="mt-3">{t('attendanceLoading')}</p> : roster.isError ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceLoadError')}</p> : roster.data?.items.length ? <><div className="mt-4 space-y-3">{roster.data.items.map(item => { const draft = drafts[item.employee_id]; return <article key={item.employee_id} className="grid gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 sm:grid-cols-[minmax(0,1.5fr)_minmax(8rem,1fr)_minmax(8rem,1fr)_auto] sm:items-end"><div><h3 className="font-semibold text-slate-900">{i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en}</h3><p className="text-sm text-slate-600">{item.employee_code} · {statusLabel(item.status)}</p></div><label className="text-sm font-semibold">{t('attendanceStatus')}<select className={fieldClass} disabled={!canManage} value={draft?.status ?? item.status ?? ''} onChange={event => updateDraft(item, { status: event.target.value as Status | '' })}><option value="">{t('attendanceUnmarked')}</option>{(['present', 'absent', 'leave'] as const).map(status => <option key={status} value={status}>{statusLabel(status)}</option>)}</select></label><label className="text-sm font-semibold">{t('attendanceNote')}<input className={fieldClass} disabled={!canManage} maxLength={500} value={draft?.note ?? item.note ?? ''} onChange={event => updateDraft(item, { note: event.target.value })} /></label>{canManage && <button className={button} disabled={busyId === item.employee_id || !(draft?.status ?? item.status)} onClick={() => void save(item)}>{t('attendanceSave')}</button>}</article> })}</div><div className="mt-4 flex gap-3"><button disabled={page <= 1} onClick={() => setPage(page - 1)}>{t('attendancePrevious')}</button><span>{t('attendancePage')} {page}</span><button disabled={page * 10 >= (roster.data?.total || 0)} onClick={() => setPage(page + 1)}>{t('attendanceNext')}</button></div></> : <p className="mt-3 text-slate-600">{t('attendanceNoTeachers')}</p>}</section>}
    {own && sessionId && <section className={card}><h2 className="text-xl font-semibold">{t('attendanceDaily')}</h2>{history.isPending ? <p role="status">{t('attendanceLoading')}</p> : history.isError ? <p role="alert">{t('attendanceLoadError')}</p> : history.data?.items.length ? <><ul className="mt-3 space-y-2">{history.data.items.map(item => <li key={item.id} className="flex flex-wrap justify-between gap-2 rounded-lg bg-slate-50 p-3"><span>{item.attendance_date} · {item.session_name}</span><strong>{statusLabel(item.status)}</strong></li>)}</ul><div className="mt-4 flex gap-3"><button disabled={historyPage <= 1} onClick={() => setHistoryPage(historyPage - 1)}>{t('attendancePrevious')}</button><span>{t('attendancePage')} {historyPage}</span><button disabled={historyPage * 10 >= (history.data?.total || 0)} onClick={() => setHistoryPage(historyPage + 1)}>{t('attendanceNext')}</button></div></> : <p className="mt-3 text-slate-600">{t('attendanceNoHistory')}</p>}</section>}
    {sessionId && (own || canReport) && <section className={card}><h2 className="text-xl font-semibold">{t('attendanceMonthly')}</h2><label className="mt-3 block max-w-xs text-sm font-semibold text-slate-700">{t('attendanceMonth')}<input className={fieldClass} type="month" value={month} onChange={event => { setMonth(event.target.value); setReportPage(1) }} /></label>{!validMonth ? <p role="alert" className="mt-3 text-rose-700">{t('attendanceInvalidDate')}</p> : own ? ownMonth.isPending ? <p role="status" className="mt-3">{t('attendanceLoading')}</p> : ownMonth.isError ? <p role="alert" className="mt-3">{t('attendanceLoadError')}</p> : <p className="mt-4 flex flex-wrap gap-4 text-slate-700">{t('attendancePresent')}: {ownMonth.data?.present} · {t('attendanceAbsent')}: {ownMonth.data?.absent} · {t('attendanceLeave')}: {ownMonth.data?.leave} · {t('attendanceTotal')}: {ownMonth.data?.total}</p> : report.isPending ? <p role="status" className="mt-3">{t('attendanceLoading')}</p> : report.isError ? <p role="alert" className="mt-3">{t('attendanceLoadError')}</p> : report.data?.items.length ? <><ul className="mt-4 space-y-2">{report.data.items.map(item => <li key={item.employee_id} className="rounded-lg bg-slate-50 p-3"><strong>{i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en}</strong><p className="text-sm text-slate-600">{item.employee_code} · {t('attendancePresent')}: {item.present} · {t('attendanceAbsent')}: {item.absent} · {t('attendanceLeave')}: {item.leave} · {t('attendanceTotal')}: {item.total}</p></li>)}</ul><div className="mt-4 flex gap-3"><button disabled={reportPage <= 1} onClick={() => setReportPage(reportPage - 1)}>{t('attendancePrevious')}</button><span>{t('attendancePage')} {reportPage}</span><button disabled={reportPage * 10 >= (report.data?.total || 0)} onClick={() => setReportPage(reportPage + 1)}>{t('attendanceNext')}</button></div></> : <p className="mt-3 text-slate-600">{t('attendanceNoReport')}</p>}</section>}
  </div>
}
