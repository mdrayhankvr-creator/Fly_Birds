import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { getMyChildren, getMyGuardian } from '../api/client'
import { Icon, type IconName } from '../components/icons'
import { StateScreen } from '../components/StateScreen'
import { Avatar, EmptyState, PageHeader, ui, useNumber } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

// GuardianChildrenPage is the guardian's hub: one card per linked child with
// direct links to that child's records. Links appear only for modules the
// guardian may open.
export function GuardianChildrenPage({ showAttendance = false, showRoutine = false, showResults = false, showFees = false, showDiary = false }: { showAttendance?: boolean; showRoutine?: boolean; showResults?: boolean; showFees?: boolean; showDiary?: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const num = useNumber()
  const guardian = useQuery({ queryKey: ['people', 'my-guardian'], queryFn: () => authorized(token => getMyGuardian(token)), retry: false })
  const children = useQuery({ queryKey: ['people', 'my-children'], queryFn: () => authorized(token => getMyChildren(token)), retry: false })
  const local = (en: string, bn?: string) => (i18n.language === 'bn' && bn ? bn : en)
  if (children.isPending) return <StateScreen kind="loading" />
  if (children.isError) return <StateScreen kind="error" message={t('peopleLoadError')} />
  const links: { show: boolean; path: string; label: string; icon: IconName }[] = [
    { show: true, path: '', label: 'uxProfile', icon: 'user' },
    { show: showAttendance, path: '/attendance', label: 'uxAttendance', icon: 'check' },
    { show: showRoutine, path: '/routine', label: 'uxRoutine', icon: 'clock' },
    { show: showResults, path: '/results', label: 'uxResults', icon: 'grade' },
    { show: showFees, path: '/fees', label: 'uxFees', icon: 'receipt' },
    { show: showDiary, path: '/diary', label: 'uxDiary', icon: 'notebook' },
  ]
  return <div className="space-y-6">
    <PageHeader eyebrow={t('myGuardianProfile')} title={t('guardianChildren')} description={t('uxMyChildrenIntro')} />
    {guardian.data && <section className={`${ui.card} flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3 text-sm sm:px-6`} aria-label={t('uxYourContact')}>
      <span className="font-semibold text-slate-900">{local(guardian.data.name_en, guardian.data.name_bn)}</span>
      {guardian.data.phone && <span className="flex items-center gap-1.5 text-slate-600"><Icon name="user" className="h-4 w-4 text-slate-400" />{guardian.data.phone}</span>}
      {guardian.data.email && <span className="flex min-w-0 items-center gap-1.5 break-all text-slate-600"><Icon name="bell" className="h-4 w-4 shrink-0 text-slate-400" />{guardian.data.email}</span>}
    </section>}
    {children.data.length === 0 ? <EmptyState icon="users" title={t('noStudents')}>{t('linkedChildrenOnly')}</EmptyState> :
      <ul className="grid gap-4 md:grid-cols-2">{children.data.map(child => {
        const name = local(child.name_en, child.name_bn)
        return <li key={child.id} className={`${ui.card} p-4 sm:p-5`}>
          <div className="flex items-center gap-3"><Avatar name={child.name_en} />
            <div className="min-w-0"><h2 className="truncate text-lg font-bold text-slate-900">{name}</h2>
              <p className="text-sm text-slate-600">{child.class_name} · {child.section_name}{child.roll_no != null ? ` · ${t('uxRoll', { n: num(child.roll_no) })}` : ''}</p>
              <p className="text-xs text-slate-500">{t('admissionNo')}: {child.admission_no}</p></div></div>
          <nav aria-label={t('uxChildLinks', { name })} className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3">
            {links.filter(l => l.show).map(l => <Link key={l.label} to={`/guardian/children/${child.id}${l.path}`} className="flex min-h-11 items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition hover:border-[var(--brand-primary)] hover:bg-[var(--brand-soft)] hover:text-[var(--brand-ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)]">
              <Icon name={l.icon} className="h-4 w-4 shrink-0" />{t(l.label)}</Link>)}
          </nav>
        </li>
      })}</ul>}
  </div>
}
