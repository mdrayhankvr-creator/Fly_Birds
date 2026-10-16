import { useQuery } from '@tanstack/react-query'
import { Link, Navigate, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getDashboard } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

const roleKeys = ['admin', 'head_teacher', 'teacher', 'student', 'guardian', 'accountant']

export function DashboardPage() {
  const { t, i18n } = useTranslation()
  const { role } = useParams()
  const { user, accessToken, renew, expire } = useAuth()
  const dashboard = useQuery({
    queryKey: ['dashboard', user?.id], enabled: !!accessToken, retry: false, refetchInterval: 15_000,
    queryFn: async () => {
      if (!accessToken) throw new ApiError(401, 'unauthorized')
      try { return await getDashboard(accessToken) }
      catch (cause) {
        if (cause instanceof ApiError && cause.status === 401) {
          try { return await getDashboard(await renew()) }
          catch (retryCause) { if (retryCause instanceof ApiError && retryCause.status === 401) expire(); throw retryCause }
        }
        throw cause
      }
    },
  })
  if (dashboard.isPending) return <p role="status">{t('loadingDashboard')}</p>
  if (dashboard.isError) return <p role="alert" className="text-rose-700">{t('dashboardError')}</p>
  const data = dashboard.data
  if (!role || role !== data.user.role) return <Navigate to={`/dashboard/${data.user.role}`} replace />
  const label = roleKeys.includes(role) ? t(`role_${role}`) : role
  return <div className="grid gap-6 lg:grid-cols-[14rem_minmax(0,1fr)]">
    <aside className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <h2 className="brand-text text-lg font-bold">{i18n.language === 'bn' ? data.branding.name_bn : data.branding.name_en}</h2>
      <p className="mt-2 text-sm text-slate-600">{label}</p>
      <nav className="mt-5 flex flex-wrap gap-2 lg:flex-col" aria-label={t('dashboardNavigation')}>
        {data.navigation.map((item) => item.coming_soon ? <span key={item.key} className="rounded-lg bg-slate-100 px-3 py-2 text-sm text-slate-600">{t(`module_${item.key}`, { defaultValue: item.name })} · {t('comingSoon')}</span> :
          <Link key={item.key} to={item.path} className="rounded-lg px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-100">{t(`nav_${item.key}`, { defaultValue: item.name })}</Link>)}
      </nav>
    </aside>
    <div className="space-y-6 min-w-0">
      <header className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm"><p className="text-sm font-semibold uppercase tracking-widest text-teal-700">{label}</p><h1 className="mt-2 text-3xl font-bold text-slate-900">{t('dashboardHeading')}</h1><p className="mt-3 break-words text-slate-600">{data.user.display_name || data.user.email} · {data.user.email}</p></header>
      {data.summary && <section aria-label={t('overview')} className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {([['total_users', data.summary.total_users], ['active_users', data.summary.active_users], ['inactive_users', data.summary.inactive_users], ['enabled_modules', data.summary.enabled_modules]] as const).map(([key, value]) => <div key={key} className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><p className="text-sm text-slate-600">{t(key)}</p><p className="mt-2 text-3xl font-bold text-slate-900">{value}</p></div>)}
        <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><p className="text-sm text-slate-600">{t('schoolCompleteness')}</p><p className="mt-2 text-3xl font-bold text-slate-900">{data.summary.school_fields_complete}/{data.summary.school_fields_total}</p></div>
        <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm"><p className="text-sm text-slate-600">{t('usersByRole')}</p><ul className="mt-2 space-y-1 text-sm">{data.summary.by_role.map((entry) => <li key={entry.role}>{t(`role_${entry.role}`)}: {entry.count}</li>)}</ul></div>
      </section>}
      <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm"><h2 className="text-xl font-semibold text-slate-900">{t('yourWorkspace')}</h2><p className="mt-2 text-slate-600">{t('futureModulesNote')}</p><div className="mt-4 flex flex-wrap gap-2">{data.navigation.filter((item) => item.coming_soon).map((item) => <span key={item.key} className="rounded-full bg-slate-100 px-3 py-1 text-sm text-slate-700">{t(`module_${item.key}`, { defaultValue: item.name })} · {t('comingSoon')}</span>)}</div></section>
    </div>
  </div>
}
