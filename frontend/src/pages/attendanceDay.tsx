import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, getAttendanceDay, type AttendanceDay } from '../api/client'

// Weekly off days and published holidays take no new attendance (lifecycle
// audit QA-12). Attendance already recorded on such a day stays and can be
// corrected.

export function useAttendanceDay(date: string, enabled: boolean, authorized: <T>(fn: (token: string) => Promise<T>) => Promise<T>) {
  const query = useQuery({ queryKey: ['attendance', 'day', date], enabled: enabled && !!date, queryFn: () => authorized(token => getAttendanceDay(token, date)), retry: false, staleTime: 60_000 })
  return { day: query.data, closed: !!query.data && !query.data.open }
}

export function useDayOffText() {
  const { t, i18n } = useTranslation()
  return (day: AttendanceDay) => {
    const date = new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${day.date}T00:00:00Z`))
    if (day.reason === 'holiday') return t('afDayOffHoliday', { date, holiday: i18n.language === 'bn' && day.holiday_title_bn ? day.holiday_title_bn : day.holiday_title_en })
    return t('afDayOffWeekly', { date, weekday: t(`afWeekday_${day.weekday}`) })
  }
}

// dayOffFromError returns the closed day of an attendance_day_off refusal.
export function dayOffFromError(cause: unknown): AttendanceDay | null {
  if (cause instanceof ApiError && cause.code === 'attendance_day_off' && cause.details && typeof cause.details.day === 'object' && cause.details.day) return cause.details.day as AttendanceDay
  return null
}

export function DayOffNotice({ day, existing = true }: { day: AttendanceDay; existing?: boolean }) {
  const { t } = useTranslation()
  const text = useDayOffText()
  return <div role="status" className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
    <p className="font-semibold">{text(day)}</p>
    {existing && <p className="mt-1">{t('afDayOffExisting')}</p>}
    <p className="mt-1 text-xs text-amber-900">{t('afDayOffWhere')}</p>
  </div>
}
