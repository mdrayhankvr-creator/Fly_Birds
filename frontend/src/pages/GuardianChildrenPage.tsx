import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { getMyChildren, getMyGuardian } from '../api/client'
import { usePeopleApi } from './peopleShared'

export function GuardianChildrenPage({ showAttendance = false, showRoutine = false, showResults = false }: { showAttendance?: boolean; showRoutine?: boolean; showResults?: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const guardian = useQuery({ queryKey: ['people', 'my-guardian'], queryFn: () => authorized(token => getMyGuardian(token)), retry: false })
  const children = useQuery({ queryKey: ['people', 'my-children'], queryFn: () => authorized(token => getMyChildren(token)), retry: false })
  return <div className="space-y-6"><header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm"><p className="text-sm font-bold uppercase tracking-widest text-teal-700">{t('myGuardianProfile')}</p><h1 className="mt-2 text-3xl font-bold">{t('guardianChildren')}</h1><p className="mt-2 text-slate-600">{t('linkedChildrenOnly')}</p></header>
    {guardian.data && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="text-lg font-bold">{i18n.language === 'bn' && guardian.data.name_bn ? guardian.data.name_bn : guardian.data.name_en}</h2><p className="mt-2 text-sm text-slate-600">{guardian.data.phone} · {guardian.data.email}</p></section>}
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">{children.isPending ? <p role="status">{t('academicLoading')}</p> : children.isError ? <p role="alert" className="text-rose-700">{t('peopleLoadError')}</p> : children.data.length === 0 ? <p>{t('noStudents')}</p> : <ul className="space-y-3">{children.data.map(child => <li key={child.id} className="rounded-xl border p-4"><Link to={`/guardian/children/${child.id}`} className="font-semibold text-teal-800 underline">{i18n.language === 'bn' && child.name_bn ? child.name_bn : child.name_en}</Link><p className="mt-1 text-sm text-slate-600">{child.admission_no} · {child.class_name} / {child.section_name}</p>{showAttendance && <Link to={`/guardian/children/${child.id}/attendance`} className="mt-2 inline-block text-sm font-semibold text-teal-800 underline">{t('studentAttendanceReport')}</Link>}</li>)}</ul>}</section>
    {showRoutine && children.data && children.data.length > 0 && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="font-bold">{t('routineHeading')}</h2><ul className="mt-3 space-y-2">{children.data.map(child => <li key={child.id}><Link className="font-semibold text-teal-800 underline" to={`/guardian/children/${child.id}/routine`}>{i18n.language === 'bn' && child.name_bn ? child.name_bn : child.name_en}</Link></li>)}</ul></section>}
    {showResults && children.data && children.data.length > 0 && <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><h2 className="font-bold">{t('examChildResults')}</h2><ul className="mt-3 space-y-2">{children.data.map(child => <li key={child.id}><Link className="font-semibold text-teal-800 underline" to={`/guardian/children/${child.id}/results`}>{i18n.language === 'bn' && child.name_bn ? child.name_bn : child.name_en}</Link></li>)}</ul></section>}
  </div>
}
