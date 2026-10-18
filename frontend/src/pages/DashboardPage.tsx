import { useQuery } from '@tanstack/react-query'
import { Navigate, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getDashboard } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { RoleDashboard } from './RoleDashboards'

export function DashboardPage() {
  const { t } = useTranslation()
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
  if (dashboard.isPending) return <div className="space-y-4" role="status" aria-label={t('loadingDashboard')}><div className="h-16 w-2/3 animate-pulse rounded-xl bg-slate-200/70 motion-reduce:animate-none" /><div className="grid grid-cols-2 gap-3 lg:grid-cols-4">{Array.from({ length: 4 }, (_, i) => <div key={i} className="h-28 animate-pulse rounded-2xl bg-slate-200/60 motion-reduce:animate-none" />)}</div></div>
  if (dashboard.isError) return <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-rose-800">{t('dashboardError')}</p>
  const data = dashboard.data
  if (!role || role !== data.user.role) return <Navigate to={`/dashboard/${data.user.role}`} replace />
  return <RoleDashboard user={data.user} branding={data.branding} navigation={data.navigation.filter(item => !item.coming_soon).map(item => item.key)} />
}
