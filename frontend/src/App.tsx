import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Route, Routes } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getHealth, getMyPermissions } from './api/client'
import { useAuth } from './auth/AuthProvider'
import { ProtectedRoute } from './auth/ProtectedRoute'
import { LoginPage } from './pages/LoginPage'
import { AccountPage } from './pages/AccountPage'
import { PermissionPage } from './pages/PermissionPage'

function HealthCard() {
  const { t } = useTranslation()
  const health = useQuery({
    queryKey: ['health'],
    queryFn: ({ signal }) => getHealth(signal),
    refetchInterval: 30_000,
    retry: 1,
  })

  const label = health.isPending ? t('checking') : health.isError ? t('offline') : t('online')
  const color = health.isPending ? 'bg-amber-400' : health.isError ? 'bg-rose-500' : 'bg-emerald-500'

  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm" aria-live="polite">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-semibold text-slate-900">{t('apiStatus')}</h2>
        <span className="flex items-center gap-2 text-sm font-medium text-slate-700">
          <span className={`h-2.5 w-2.5 rounded-full ${color}`} aria-hidden="true" />
          {label}
        </span>
      </div>
      <p className="mt-3 text-sm leading-6 text-slate-600">{t('healthNote')}</p>
      <button
        type="button"
        onClick={() => void health.refetch()}
        className="mt-5 rounded-lg bg-teal-700 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
      >
        {t('reload')}
      </button>
    </section>
  )
}

function Home() {
  const { t } = useTranslation()
  return (
    <div className="grid gap-8 lg:grid-cols-[minmax(0,1.4fr)_minmax(18rem,1fr)] lg:items-start">
      <section>
        <p className="text-sm font-bold uppercase tracking-[0.2em] text-teal-700">M0 · {t('foundation')}</p>
        <h1 className="mt-4 max-w-2xl text-4xl font-bold tracking-tight text-slate-900 sm:text-5xl">{t('welcome')}</h1>
        <p className="mt-5 max-w-xl text-lg leading-8 text-slate-600">{t('description')}</p>
      </section>
      <HealthCard />
    </div>
  )
}

function NotFound() {
  const { t } = useTranslation()
  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-8 shadow-sm">
      <h1 className="text-2xl font-bold text-slate-900">{t('notFound')}</h1>
      <Link to="/" className="mt-4 inline-block font-semibold text-teal-700 underline-offset-4 hover:underline">
        {t('backHome')}
      </Link>
    </div>
  )
}

function PermissionGate({ permissions, role, loading, error }: { permissions: string[] | undefined; role: string | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status" className="text-slate-600">{t('loadingPermissions')}</p>
  if (error) return <p role="alert" className="text-rose-700">{t('loadPermissionsError')}</p>
  if (!permissions?.includes('permissions:read')) return <p role="alert" className="text-rose-700">{t('permissionDenied')}</p>
  return <PermissionPage canManage={role === 'admin' && permissions.includes('permissions:manage')} />
}

export default function App() {
  const { t, i18n } = useTranslation()
  const { user, accessToken, ready, signOut, renew, expire } = useAuth()
  const [logoutError, setLogoutError] = useState(false)
  const myPermissions = useQuery({
    queryKey: ['my-permissions', user?.id],
    enabled: !!accessToken,
    retry: false,
    refetchInterval: 15_000,
    queryFn: async () => {
      if (!accessToken) return []
      try { return await getMyPermissions(accessToken) }
      catch (cause) {
        if (cause instanceof ApiError && cause.status === 401) {
          try { return await getMyPermissions(await renew()) }
          catch (retryCause) { if (retryCause instanceof ApiError && retryCause.status === 401) expire(); throw retryCause }
        }
        throw cause
      }
    },
  })

  async function setLanguage(language: 'en' | 'bn') {
    await i18n.changeLanguage(language)
    localStorage.setItem('flybirds.language', language)
    document.documentElement.lang = language
  }

  async function handleLogout() {
    setLogoutError(false)
    try { await signOut() } catch { setLogoutError(true) }
  }

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-5 py-5 sm:px-8">
          <Link to="/" className="text-xl font-extrabold tracking-tight text-teal-800">{t('appName')}</Link>
          <div className="flex items-center gap-5">
            <nav aria-label="Main navigation">
              <div className="flex items-center gap-4">
                <Link to="/" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('home')}</Link>
                {ready && (user
                  ? <><Link to="/account" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('account')}</Link>
                    {myPermissions.data?.includes('permissions:read') && <Link to="/admin/permissions" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('permissionEditor')}</Link>}
                    <button type="button" onClick={() => void handleLogout()} className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('signOut')}</button></>
                  : <Link to="/login" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('signIn')}</Link>)}
              </div>
            </nav>
            <div className="flex items-center gap-2 text-sm" aria-label={t('language')}>
              <button type="button" onClick={() => void setLanguage('en')} aria-pressed={i18n.language === 'en'} className="rounded px-2 py-1 font-medium text-slate-700 hover:bg-slate-100 aria-pressed:bg-teal-100">EN</button>
              <button type="button" onClick={() => void setLanguage('bn')} aria-pressed={i18n.language === 'bn'} className="rounded px-2 py-1 font-medium text-slate-700 hover:bg-slate-100 aria-pressed:bg-teal-100">বাংলা</button>
            </div>
          </div>
        </div>
      </header>
      {logoutError && <p role="alert" className="mx-auto max-w-6xl px-5 pt-4 text-sm text-rose-700 sm:px-8">{t('logoutError')}</p>}
      <main className="mx-auto max-w-6xl px-5 py-14 sm:px-8">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/account" element={<ProtectedRoute><AccountPage /></ProtectedRoute>} />
          <Route path="/admin/permissions" element={<ProtectedRoute><PermissionGate permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
    </div>
  )
}
