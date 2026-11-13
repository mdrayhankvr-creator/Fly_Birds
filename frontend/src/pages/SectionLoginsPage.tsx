import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getAcademicClasses, getAcademicSections, getAcademicSessions, getStudents } from '../api/client'
import { createSectionLogins, type StudentLogin } from '../api/studentLogins'
import { confirmAction } from '../components/ConfirmDialog'
import { Icon } from '../components/icons'
import { usePeopleApi } from './peopleShared'
import { LoginSlipSheet } from './LoginSlips'
import { EmptyState, Notice, PageHeader, SectionCard, Skeleton, ui, useNumber } from './attendanceUi'

// Give every student in a section who has no login one, in one step, and
// print their slips (checkpoint 5).
export function SectionLoginsPage() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const num = useNumber()
  const [sessionId, setSessionId] = useState('')
  const [classId, setClassId] = useState('')
  const [sectionId, setSectionId] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [made, setMade] = useState<StudentLogin[]>([])
  const page = (extra: Record<string, string> = {}) => new URLSearchParams({ page: '1', page_size: '100', ...extra })
  const sessions = useQuery({ queryKey: ['logins', 'sessions'], queryFn: () => authorized(token => getAcademicSessions(token, page())), retry: false })
  const classes = useQuery({ queryKey: ['logins', 'classes'], queryFn: () => authorized(token => getAcademicClasses(token, page())), retry: false })
  const sections = useQuery({ queryKey: ['logins', 'sections', sessionId, classId], enabled: !!sessionId && !!classId, queryFn: () => authorized(token => getAcademicSections(token, page({ session_id: sessionId, class_id: classId }))), retry: false })
  const without = useQuery({ queryKey: ['logins', 'without', sectionId], enabled: !!sectionId, queryFn: () => authorized(token => getStudents(token, page({ section_id: sectionId, status: 'active', without_login: 'true' }))), retry: false })
  const sectionName = sections.data?.items.find(s => s.id === sectionId)?.name ?? ''
  const className = classes.data?.items.find(c => c.id === classId)?.name ?? ''
  const count = without.data?.total ?? 0

  async function give() {
    const section = [className, sectionName].join(' ')
    if (!sectionId || !(await confirmAction(t('c5GiveConfirm', { count, n: num(count), section }), { tone: 'default', confirmLabel: t('c5GiveLogins') }))) return
    setBusy(true); setError(''); setMade([])
    try {
      const result = await authorized(token => createSectionLogins(token, sectionId))
      setMade(result.logins)
      await cache.invalidateQueries({ queryKey: ['logins', 'without'] })
      await cache.invalidateQueries({ queryKey: ['people'] })
    } catch (cause) {
      setError(t(cause instanceof ApiError && cause.code === 'no_students_without_login' ? 'c5NobodyWithout' : cause instanceof ApiError && cause.code === 'no_login_id' ? 'c5NoLoginId' : cause instanceof ApiError && cause.status === 403 ? 'permissionDenied' : 'c5LoginsFailed'))
    } finally { setBusy(false) }
  }

  return <div className="space-y-6">
    <PageHeader eyebrow={t('m12Group_people')} title={t('c5LoginsTitle')} description={t('c5LoginsIntro')}
      actions={<Link to="/admin/students" className={ui.secondary}><Icon name="chevronLeft" className="h-4 w-4" />{t('studentsHeading')}</Link>} />
    <SectionCard title={t('c5ChooseSection')} id="logins-section" icon="users">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <label className={ui.label}>{t('sessionLabel')}<select className={ui.field} value={sessionId} onChange={e => { setSessionId(e.target.value); setSectionId(''); setMade([]) }}><option value="">{t('selectOption')}</option>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('classLabel')}<select className={ui.field} value={classId} onChange={e => { setClassId(e.target.value); setSectionId(''); setMade([]) }}><option value="">{t('selectOption')}</option>{classes.data?.items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
        <label className={ui.label}>{t('sectionLabel')}<select className={ui.field} value={sectionId} disabled={!sessionId || !classId} onChange={e => { setSectionId(e.target.value); setMade([]) }}><option value="">{t('selectOption')}</option>{sections.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
      </div>
      {sectionId && <div className="mt-5">
        {without.isPending ? <Skeleton rows={2} /> : without.isError ? <Notice tone="error">{t('peopleLoadError')}</Notice> : count === 0 && !made.length
          ? <EmptyState icon="check" title={t('c5EveryoneHasLogin')} />
          : count > 0 && <div className="flex flex-col gap-3 rounded-2xl border border-slate-200 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div><p className="text-lg font-bold text-slate-900" data-without-count={count}>{t('c5WithoutCount', { count, n: num(count) })}</p>
              <p className="text-sm text-slate-600">{without.data.items.slice(0, 6).map(s => s.name_en).join(', ')}{count > 6 ? '…' : ''}</p></div>
            <button type="button" className={ui.primary} disabled={busy} onClick={() => void give()}><Icon name="lock" className="h-4 w-4" />{busy ? t('c3Saving') : t('c5GiveLogins')}</button>
          </div>}
      </div>}
      {error && <div className="mt-4"><Notice tone="error">{error}</Notice></div>}
    </SectionCard>
    {made.length > 0 && <SectionCard title={t('c5Slips')} id="logins-slips" icon="print"><LoginSlipSheet logins={made} /></SectionCard>}
  </div>
}
