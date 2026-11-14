import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getOwnStudentAttendanceReport, getStaffStudentAttendanceReport } from '../api/client'
import { SectionCard, Skeleton } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

function currentMonth() {
  const parts = new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Dhaka', year: 'numeric', month: '2-digit' }).formatToParts(new Date())
  return `${parts.find(p => p.type === 'year')?.value}-${parts.find(p => p.type === 'month')?.value}`
}

export function ProfileAttendanceCalendar({ studentId, self }: { studentId: string; self: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const [month, setMonth] = useState(currentMonth)
  const report = useQuery({ queryKey: ['profile-calendar', self ? 'self' : studentId, month],
    queryFn: () => authorized(token => self ? getOwnStudentAttendanceReport(token, month) : getStaffStudentAttendanceReport(token, studentId, month)), retry: false })
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  const [year, monthNumber] = month.split('-').map(Number)
  const first = new Date(Date.UTC(year, monthNumber - 1, 1))
  const offset = (first.getUTCDay() + 6) % 7
  const days = new Date(Date.UTC(year, monthNumber, 0)).getUTCDate()
  const byDay = new Map(report.data?.items.map(item => [Number(item.attendance_date.slice(8)), item.status]) ?? [])
  const weekdays = Array.from({ length: 7 }, (_, i) => new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' }).format(new Date(Date.UTC(2024, 0, 1 + i))))
  const number = new Intl.NumberFormat(locale)
  const statusText = (status: 'present' | 'late' | 'absent') => t(`studentAttendance${status[0].toUpperCase()}${status.slice(1)}`)
  return <SectionCard title={t('prMonthCalendar')} id="profile-attendance-calendar" icon="calendar"
    actions={<label className="text-sm font-semibold text-slate-700">{t('attendanceMonth')}<input type="month" value={month} onChange={e => e.target.value && setMonth(e.target.value)} className="mt-1 block min-h-10 rounded-xl border border-slate-300 bg-white px-3" /></label>}>
    {report.isPending ? <Skeleton rows={3} /> : report.isError ? <p role="alert" className="text-sm text-rose-700">{t('attendanceLoadError')}</p> : <>
      <div className="grid max-w-[32rem] grid-cols-7 gap-1.5" role="grid" aria-label={t('attnCalendar')}>
        {weekdays.map((day, i) => <div key={i} role="columnheader" className="pb-1 text-center text-xs font-semibold text-slate-500">{day}</div>)}
        {Array.from({ length: offset }, (_, i) => <div key={`blank-${i}`} role="gridcell" aria-hidden="true" />)}
        {Array.from({ length: days }, (_, i) => {
          const status = byDay.get(i + 1)
          const date = new Date(Date.UTC(year, monthNumber - 1, i + 1))
          const label = `${new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', timeZone: 'UTC' }).format(date)}: ${status ? statusText(status) : t('attnNoRecord')}`
          return <div key={i} role="gridcell" aria-label={label} title={label} data-attendance-day={status || 'none'}
            className={`pr-calendar-day ${status || 'none'}`}>{number.format(i + 1)}</div>
        })}
      </div>
      <div className="mt-4 flex flex-wrap gap-3 text-xs text-slate-600">{(['present', 'late', 'absent'] as const).map(status => <span key={status} className="flex items-center gap-1.5"><span className={`pr-calendar-swatch ${status}`} />{statusText(status)}</span>)}</div>
    </>}
  </SectionCard>
}
