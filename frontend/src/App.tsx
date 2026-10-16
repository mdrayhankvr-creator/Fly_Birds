import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Route, Routes } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getHealth, getMyPermissions, getSchoolBranding, getSchoolModules, type SchoolBranding } from './api/client'
import { useAuth } from './auth/AuthProvider'
import { ProtectedRoute } from './auth/ProtectedRoute'
import { LoginPage } from './pages/LoginPage'
import { AccountPage } from './pages/AccountPage'
import { PermissionPage } from './pages/PermissionPage'
import { SchoolSettingsPage } from './pages/SchoolSettingsPage'
import { UsersPage } from './pages/UsersPage'
import { DashboardPage } from './pages/DashboardPage'
import { AcademicSessionsPage } from './pages/AcademicSessionsPage'

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

function SchoolGate({ permissions, loading, error }: { permissions: string[] | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!permissions?.includes('school_settings:read')) return <p role="alert">{t('permissionDenied')}</p>
  return <SchoolSettingsPage permissions={permissions} />
}

function UsersGate({ permissions, role, loading, error }: { permissions: string[] | undefined; role: string | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (role !== 'admin' || !permissions?.includes('users:read')) return <p role="alert">{t('permissionDenied')}</p>
  return <UsersPage permissions={permissions} />
}

function AcademicSessionsGate({ permissions, loading, error }: { permissions: string[] | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!permissions?.includes('academic_sessions:read')) return <p role="alert">{t('permissionDenied')}</p>
  return <AcademicSessionsPage permissions={permissions} />
}

function applyBranding(branding: SchoolBranding | undefined) {
  const root = document.documentElement
  const validColor = (value: string | undefined) => value && /^#[0-9a-fA-F]{6}$/.test(value) ? value : null
  root.style.setProperty('--brand-primary', validColor(branding?.primary_color) || '#0F766E')
  root.style.setProperty('--brand-secondary', validColor(branding?.secondary_color) || '#334155')
  root.dataset.theme = branding?.dashboard_theme === 'dark' ? 'dark' : 'light'
}

export default function App() {
  const { t, i18n } = useTranslation()
  const { user, accessToken, ready, signOut, renew, expire } = useAuth()
  const [logoutError, setLogoutError] = useState(false)
  const branding = useQuery({ queryKey: ['school', 'branding'], queryFn: ({ signal }) => getSchoolBranding(signal), retry: false, staleTime: 60_000 })
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
  const myModules = useQuery({
    queryKey: ['school', 'modules', 'navigation', user?.id], enabled: !!accessToken && !!myPermissions.data?.includes('modules:read'), retry: false,
    queryFn: async () => {
      if (!accessToken) return null
      try { return await getSchoolModules(accessToken) }
      catch (cause) {
        if (cause instanceof ApiError && cause.status === 401) return getSchoolModules(await renew())
        throw cause
      }
    },
  })
  useEffect(() => {
    applyBranding(branding.data)
    if (branding.data?.preferred_language && !localStorage.getItem('flybirds.language')) {
      void i18n.changeLanguage(branding.data.preferred_language)
      document.documentElement.lang = branding.data.preferred_language
    }
  }, [branding.data, i18n])
  const moduleEnabled = (key: string) => myModules.data?.items.find((item) => item.key === key)?.enabled ?? true

  async function setLanguage(language: 'en' | 'bn') {
    localStorage.setItem('flybirds.language', language)
    document.documentElement.lang = language
    await i18n.changeLanguage(language)
  }

  async function handleLogout() {
    setLogoutError(false)
    try { await signOut() } catch { setLogoutError(true) }
  }

  return (
    <div className="min-h-screen bg-slate-50">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-5 py-5 sm:px-8">
          <Link to="/" className="brand-text flex min-w-0 items-center gap-3 text-xl font-extrabold tracking-tight">
            {branding.data?.logo_url && <img src={branding.data.logo_url} alt="" referrerPolicy="no-referrer" className="h-10 w-10 rounded object-contain" />}
            <span className="truncate">{branding.data ? (i18n.language === 'bn' ? branding.data.name_bn : branding.data.name_en) : branding.isPending ? t('loadingBranding') : t('appName')}</span>
          </Link>
          <div className="flex w-full flex-wrap items-center justify-between gap-3 sm:w-auto sm:gap-5">
            <nav aria-label="Main navigation" className="min-w-0">
              <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                <Link to="/" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('home')}</Link>
                {ready && (user
                  ? <><Link to="/account" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('account')}</Link>
                    <Link to="/dashboard" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('dashboardHeading')}</Link>
                    {user.role === 'admin' && myPermissions.data?.includes('users:read') && moduleEnabled('permissions') && <Link to="/admin/users" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_users')}</Link>}
                    {myPermissions.data?.includes('permissions:read') && moduleEnabled('permissions') && <Link to="/admin/permissions" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('permissionEditor')}</Link>}
                    {myPermissions.data?.includes('school_settings:read') && moduleEnabled('school_settings') && <Link to="/admin/school-settings" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('schoolSettings')}</Link>}
                    {myPermissions.data?.includes('academic_sessions:read') && myModules.data?.items.some((item) => item.key === 'academics' && item.enabled) && <Link to="/admin/academic/sessions" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('academicSessions')}</Link>}
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
          <Route path="/login" element={<LoginPage branding={branding.data} />} />
          <Route path="/account" element={<ProtectedRoute><AccountPage /></ProtectedRoute>} />
          <Route path="/dashboard" element={<ProtectedRoute><DashboardPage /></ProtectedRoute>} />
          <Route path="/dashboard/:role" element={<ProtectedRoute><DashboardPage /></ProtectedRoute>} />
          <Route path="/admin/users" element={<ProtectedRoute><UsersGate permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/permissions" element={<ProtectedRoute><PermissionGate permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/school-settings" element={<ProtectedRoute><SchoolGate permissions={myPermissions.data} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/academic/sessions" element={<ProtectedRoute><AcademicSessionsGate permissions={myPermissions.data} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
    </div>
  )
}
