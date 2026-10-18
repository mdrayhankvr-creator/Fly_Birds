import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Route, Routes } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getDashboard, getHealth, getMyPermissions, getSchoolBranding, getSchoolModules, type SchoolBranding } from './api/client'
import { useAuth } from './auth/AuthProvider'
import { ProtectedRoute } from './auth/ProtectedRoute'
import { LoginPage } from './pages/LoginPage'
import { AccountPage } from './pages/AccountPage'
import { PermissionPage } from './pages/PermissionPage'
import { SchoolSettingsPage } from './pages/SchoolSettingsPage'
import { UsersPage } from './pages/UsersPage'
import { DashboardPage } from './pages/DashboardPage'
import { AcademicPage } from './pages/AcademicPage'
import { AcademicSessionsPage } from './pages/AcademicSessionsPage'
import { StaffPage } from './pages/StaffPage'
import { TeacherAttendancePage } from './pages/TeacherAttendancePage'
import { StudentAttendancePage, StudentAttendanceReportPage } from './pages/StudentAttendancePage'
import { SectionsPage } from './pages/SectionsPage'
import { StudentsPage } from './pages/StudentsPage'
import { GuardiansPage } from './pages/GuardiansPage'
import { StudentProfilePage } from './pages/StudentProfilePage'
import { GuardianChildrenPage } from './pages/GuardianChildrenPage'
import { RoutinePage } from './pages/RoutinePage'
import { ExamsPage } from './pages/ExamsPage'
import { FeesPage } from './pages/FeesPage'
import { MyFeesPage } from './pages/MyFeesPage'
import { NoticesPage } from './pages/NoticesPage'
import { EventsPage } from './pages/EventsPage'
import { AdminNotificationsPage, NotificationBell, NotificationsPage } from './pages/NotificationsPage'
import { AdminDiaryPage, DiaryFeedPage, TeacherDiaryPage } from './pages/DiaryPages'
import { AdminOverviewPage, AuditLogPage, BackupsPage, DataExportPage, ModulesPage } from './pages/AdminPages'
import { AnalyticsPage, ChildrenProgressPage, MyProgressPage, ReportsPage, StaffStudentPage } from './pages/AnalyticsPages'

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

// AdminGate guards the M10 control-center pages: administrators only, with
// the page's own permission; the server enforces the same rules.
function AdminGate({ kind, permissions, role, loading, error }: { kind: 'overview' | 'audit' | 'exports' | 'backups' | 'modules'; permissions: string[] | undefined; role: string | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  const required = { overview: 'system:read', audit: 'audit:read', exports: 'data:export', backups: 'backups:manage', modules: 'modules:read' }[kind]
  if (role !== 'admin' || !permissions?.includes(required)) return <p role="alert">{t('permissionDenied')}</p>
  switch (kind) {
    case 'overview': return <AdminOverviewPage />
    case 'audit': return <AuditLogPage />
    case 'exports': return <DataExportPage />
    case 'backups': return <BackupsPage />
    default: return <ModulesPage canManage={permissions.includes('modules:manage')} />
  }
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

function AcademicGate({ kind, permissions, role, enabled, loading, error }: { kind: 'classes' | 'subjects' | 'sections'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!permissions?.includes('academics:read')) return <p role="alert">{t('permissionDenied')}</p>
  if (!enabled) return <p role="alert">{t('academicModuleDisabled')}</p>
  const canManage = role === 'admin' && permissions.includes('academics:manage')
  return kind === 'sections' ? <SectionsPage canManage={canManage} /> : <AcademicPage kind={kind} canManage={canManage} />
}

function AcademicSessionsGate({ permissions, loading, error }: { permissions: string[] | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!permissions?.includes('academic_sessions:read')) return <p role="alert">{t('permissionDenied')}</p>
  return <AcademicSessionsPage permissions={permissions} />
}

function StaffGate({ permissions, own, enabled, loading, error }: { permissions: string[] | undefined; own: boolean; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!permissions?.includes(own ? 'staff:read_self' : 'staff:read')) return <p role="alert">{t('permissionDenied')}</p>
  if (!enabled) return <p role="alert">{t('staffModuleDisabled')}</p>
  return <StaffPage permissions={permissions} own={own} />
}

function AttendanceGate({ permissions, own, enabled, loading, error }: { permissions: string[] | undefined; own: boolean; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!permissions?.includes(own ? 'attendance:teachers_read_self' : 'attendance:teachers_read')) return <p role="alert">{t('permissionDenied')}</p>
  if (!enabled) return <p role="alert">{t('attendanceModuleDisabled')}</p>
  return <TeacherAttendancePage permissions={permissions} own={own} />
}

function StudentAttendanceGate({ kind, permissions, enabled, loading, error }: { kind: 'manage' | 'self' | 'guardian'; permissions: string[] | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  const required = kind === 'manage' ? 'attendance:students_read' : kind === 'self' ? 'attendance:students_read_self' : 'attendance:guardians_read_children'
  if (!permissions?.includes(required)) return <p role="alert">{t('permissionDenied')}</p>
  if (!enabled) return <p role="alert">{t('attendanceModuleDisabled')}</p>
  return kind === 'manage' ? <StudentAttendancePage permissions={permissions} /> : <StudentAttendanceReportPage guardian={kind === 'guardian'} />
}

function PeopleGate({ kind, permissions, role, enabled, loading, error, routineEnabled = false, examsEnabled = false, feesEnabled = false, diaryEnabled = false }: { kind: 'students' | 'guardians' | 'profile' | 'self' | 'children' | 'child'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean; routineEnabled?: boolean; examsEnabled?: boolean; feesEnabled?: boolean; diaryEnabled?: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('loadingPermissions')}</p>
  if (error) return <p role="alert">{t('loadPermissionsError')}</p>
  if (!enabled) return <p role="alert">{t('peopleModuleDisabled')}</p>
  const canStudents = !!permissions?.includes('students:read') && (role === 'admin' || role === 'head_teacher')
  const canGuardians = !!permissions?.includes('guardians:read') && (role === 'admin' || role === 'head_teacher')
  const canManageStudents = role === 'admin' && !!permissions?.includes('students:manage')
  const canManageGuardians = role === 'admin' && !!permissions?.includes('guardians:manage')
  if (kind === 'students' && canStudents) return <StudentsPage canManage={canManageStudents} />
  if (kind === 'guardians' && canGuardians) return <GuardiansPage canManage={canManageGuardians} />
  if (kind === 'profile' && canStudents) return <StudentProfilePage mode="staff" canManagePhoto={canManageStudents} canManageLinks={canManageGuardians} canReadGuardians={canGuardians} />
  if (kind === 'self' && role === 'student' && permissions?.includes('students:read_self')) return <StudentProfilePage mode="self" canManagePhoto={false} canManageLinks={false} canReadGuardians={false} />
  if (kind === 'children' && role === 'guardian' && permissions?.includes('guardians:read_children')) return <GuardianChildrenPage showAttendance={!!permissions?.includes('attendance:guardians_read_children')} showRoutine={routineEnabled && !!permissions?.includes('routine:read_children')} showResults={examsEnabled && !!permissions?.includes('exams:read_children')} showFees={feesEnabled && !!permissions?.includes('fees:read_children')} showDiary={diaryEnabled && !!permissions?.includes('diary:read_children')} />
  if (kind === 'child' && role === 'guardian' && permissions?.includes('guardians:read_children')) return <StudentProfilePage mode="child" canManagePhoto={false} canManageLinks={false} canReadGuardians={false} />
  return <p role="alert">{t('permissionDenied')}</p>
}

function RoutineGate({ mode, permissions, role, enabled, loading, error }: { mode: 'admin' | 'student' | 'teacher' | 'guardian'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('routineLoading')}</p>
  if (error) return <p role="alert">{t('routineLoadError')}</p>
  const required = mode === 'admin' ? 'routine:read' : mode === 'guardian' ? 'routine:read_children' : 'routine:read_self'
  if (!enabled || !permissions?.includes(required) || (mode === 'student' && role !== 'student') || (mode === 'teacher' && role !== 'teacher') || (mode === 'guardian' && role !== 'guardian')) return <p role="alert">{t('permissionDenied')}</p>
  return <RoutinePage mode={mode} canManage={mode === 'admin' && role === 'admin' && permissions.includes('routine:manage')} />
}

function ExamsGate({ mode, permissions, role, enabled, loading, error }: { mode: 'admin' | 'student' | 'teacher' | 'guardian'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('examLoading')}</p>
  if (error) return <p role="alert">{t('examError')}</p>
  const required = mode === 'admin' ? 'exams:read' : mode === 'teacher' ? 'exams:enter_marks' : mode === 'guardian' ? 'exams:read_children' : 'exams:read_self'
  if (!enabled || !permissions?.includes(required) || mode !== 'admin' && role !== mode) return <p role="alert">{t('permissionDenied')}</p>
  return <ExamsPage mode={mode} canManage={mode === 'admin' && role === 'admin' && permissions.includes('exams:manage')} />
}

function CommGate({ kind, permissions, role, enabled, loading, error }: { kind: 'notices' | 'events' | 'notifications' | 'delivery' | 'teacher-diary' | 'student-diary' | 'guardian-diary' | 'admin-diary'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('m9Loading')}</p>
  if (error) return <p role="alert">{t('m9ErrGeneric')}</p>
  const has = (p: string) => !!permissions?.includes(p)
  const allowed = enabled && ({
    notices: has('notices:read'), events: has('events:read'), notifications: has('notifications:read_self'), delivery: has('notifications:manage') && role === 'admin',
    'teacher-diary': has('diary:write') && (role === 'teacher' || role === 'head_teacher'), 'student-diary': has('diary:read_self') && role === 'student',
    'guardian-diary': has('diary:read_children') && role === 'guardian', 'admin-diary': has('diary:read'),
  } as Record<string, boolean>)[kind]
  if (!allowed) return <p role="alert">{t(enabled ? 'permissionDenied' : 'm9ErrModule')}</p>
  switch (kind) {
    case 'notices': return <NoticesPage canManage={has('notices:manage') && (role === 'admin' || role === 'head_teacher')} />
    case 'events': return <EventsPage canManage={has('events:manage') && (role === 'admin' || role === 'head_teacher')} />
    case 'notifications': return <NotificationsPage />
    case 'delivery': return <AdminNotificationsPage />
    case 'teacher-diary': return <TeacherDiaryPage />
    case 'student-diary': return <DiaryFeedPage mode="student" />
    case 'guardian-diary': return <DiaryFeedPage mode="guardian" />
    default: return <AdminDiaryPage canManage={has('diary:manage') && role === 'admin'} />
  }
}

function FeesGate({ mode, permissions, role, enabled, loading, error }: { mode: 'staff' | 'student' | 'guardian'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('feeLoading')}</p>
  if (error) return <p role="alert">{t('feeErrGeneric')}</p>
  const required = mode === 'staff' ? 'fees:read' : mode === 'guardian' ? 'fees:read_children' : 'fees:read_self'
  if (!enabled || !permissions?.includes(required) || (mode !== 'staff' && role !== mode)) return <p role="alert">{t('permissionDenied')}</p>
  if (mode !== 'staff') return <MyFeesPage mode={mode} canPay={permissions.includes('fees:pay_online')} />
  const writer = role === 'admin' || role === 'accountant'
  return <FeesPage access={{ role: role ?? '', canManage: writer && permissions.includes('fees:manage'), canCollect: writer && permissions.includes('fees:collect'), canReadAccounts: permissions.includes('accounts:read'), canManageAccounts: writer && permissions.includes('accounts:manage') }} />
}

// AnalyticsGate guards the M11 pages. The server enforces the same rules and
// narrows teacher, student and guardian data to their own records.
function AnalyticsGate({ kind, permissions, role, enabled, loading, error }: { kind: 'school' | 'teacher' | 'reports' | 'student' | 'guardian' | 'staff-student'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <p role="status">{t('m11Eyebrow')}…</p>
  if (error) return <p role="alert">{t('m11ErrGeneric')}</p>
  const has = (p: string) => !!permissions?.includes(p)
  const access = { school: has('analytics:read'), teacher: has('analytics:read_assigned'), finance: has('analytics:finance'), manage: has('analytics:manage') && role === 'admin' }
  const allowed = enabled && ({
    school: access.school || access.finance, teacher: access.teacher && !access.school, reports: access.school || access.teacher || access.finance,
    student: has('analytics:read_self') && role === 'student', guardian: has('analytics:read_children') && role === 'guardian', 'staff-student': access.school || access.teacher,
  } as Record<string, boolean>)[kind]
  if (!allowed) return <p role="alert">{t(enabled ? 'permissionDenied' : 'm11ErrModule')}</p>
  switch (kind) {
    case 'reports': return <ReportsPage access={access} />
    case 'student': return <MyProgressPage />
    case 'guardian': return <ChildrenProgressPage />
    case 'staff-student': return <StaffStudentPage />
    default: return <AnalyticsPage access={kind === 'teacher' ? { ...access, school: false, finance: false } : access} />
  }
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
  const academicNavigation = useQuery({
    queryKey: ['dashboard', user?.id],
    enabled: !!accessToken && !!myPermissions.data?.some((permission) => ['academics:read', 'academic_sessions:read', 'students:read', 'guardians:read', 'students:read_self', 'guardians:read_children', 'staff:read', 'staff:read_self', 'attendance:teachers_read', 'attendance:teachers_read_self', 'attendance:students_read', 'attendance:students_read_self', 'attendance:guardians_read_children', 'routine:read', 'routine:read_self', 'routine:read_children', 'fees:read', 'fees:read_self', 'fees:read_children', 'notices:read', 'events:read', 'notifications:read_self', 'diary:write', 'diary:read', 'diary:read_self', 'diary:read_children', 'analytics:read', 'analytics:read_assigned', 'analytics:finance', 'analytics:read_self', 'analytics:read_children'].includes(permission)) && !myPermissions.data?.includes('modules:read'),
    retry: false,
    refetchInterval: 15_000,
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
  useEffect(() => {
    applyBranding(branding.data)
    if (branding.data?.preferred_language && !localStorage.getItem('flybirds.language')) {
      void i18n.changeLanguage(branding.data.preferred_language)
      document.documentElement.lang = branding.data.preferred_language
    }
  }, [branding.data, i18n])
  const moduleEnabled = (key: string) => myModules.data?.items.find((item) => item.key === key)?.enabled ?? true
  const academicEnabled = myModules.data
    ? moduleEnabled('academics')
    : !!academicNavigation.data?.navigation.some((item) => item.path.startsWith('/admin/academic/'))
  const peopleEnabled = myModules.data ? moduleEnabled('students') : !!academicNavigation.data?.navigation.some((item) => ['/admin/students', '/admin/guardians', '/student/me', '/guardian/children'].some(path => item.path.startsWith(path)))
  const hasModuleNavigation = !!myPermissions.data?.some(permission => ['academics:read', 'academic_sessions:read', 'students:read', 'guardians:read', 'students:read_self', 'guardians:read_children'].includes(permission))
  const academicLoading = myPermissions.isPending || (hasModuleNavigation && (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending))
  const academicError = myPermissions.isError || myModules.isError || academicNavigation.isError
  const staffEnabled = myModules.data ? moduleEnabled('staff') : !!academicNavigation.data?.navigation.some((item) => item.path === '/admin/staff' || item.path === '/staff/me')
  const hasStaffRead = !!myPermissions.data?.some((permission) => permission === 'staff:read' || permission === 'staff:read_self')
  const staffLoading = myPermissions.isPending || (hasStaffRead && (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending))
  const staffError = myPermissions.isError || myModules.isError || academicNavigation.isError
  const attendanceEnabled = myModules.data ? moduleEnabled('attendance') : !!academicNavigation.data?.navigation.some((item) => item.path === '/admin/attendance/teachers' || item.path === '/attendance/me' || item.path === '/admin/attendance/students' || item.path === '/student/attendance' || item.key === 'guardian_attendance')
  const hasAttendanceRead = !!myPermissions.data?.some((permission) => permission === 'attendance:teachers_read' || permission === 'attendance:teachers_read_self' || permission === 'attendance:students_read' || permission === 'attendance:students_read_self' || permission === 'attendance:guardians_read_children')
  const attendanceLoading = myPermissions.isPending || (hasAttendanceRead && (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending))
  const attendanceError = myPermissions.isError || myModules.isError || academicNavigation.isError
  const routineEnabled = myModules.data ? moduleEnabled('routine') : !!academicNavigation.data?.navigation.some(item => item.key.startsWith('routine_'))
  const hasRoutineRead = !!myPermissions.data?.some(permission => ['routine:read', 'routine:read_self', 'routine:read_children'].includes(permission))
  const routineLoading = myPermissions.isPending || (hasRoutineRead && (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending))
  const routineError = myPermissions.isError || myModules.isError || academicNavigation.isError
  const examsEnabled = myModules.data ? moduleEnabled('examinations') : !!academicNavigation.data?.navigation.some(item => item.key.startsWith('exam_'))
  const hasExamRead = !!myPermissions.data?.some(permission => ['exams:read', 'exams:enter_marks', 'exams:read_self', 'exams:read_children'].includes(permission))
  const examsLoading = myPermissions.isPending || (hasExamRead && (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending))
  const examsError = myPermissions.isError || myModules.isError || academicNavigation.isError
  const feesEnabled = myModules.data ? moduleEnabled('fees') : !!academicNavigation.data?.navigation.some(item => item.key.startsWith('fees_'))
  const hasFeeRead = !!myPermissions.data?.some(permission => ['fees:read', 'fees:read_self', 'fees:read_children'].includes(permission))
  const feesLoading = myPermissions.isPending || (hasFeeRead && (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending))
  const feesError = myPermissions.isError || myModules.isError || academicNavigation.isError
  const noticesEnabled = myModules.data ? moduleEnabled('notices') : !!academicNavigation.data?.navigation.some(item => item.key === 'notices' || item.key === 'events')
  const diaryEnabled = myModules.data ? moduleEnabled('diary') : !!academicNavigation.data?.navigation.some(item => item.key.startsWith('diary_'))
  const analyticsEnabled = myModules.data ? moduleEnabled('analytics') : !!academicNavigation.data?.navigation.some(item => item.key.startsWith('analytics') || item.key === 'reports')
  const analyticsLoading = myPermissions.isPending || (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending && academicNavigation.fetchStatus !== 'idle')
  const commLoading = myPermissions.isPending || (myPermissions.data?.includes('modules:read') ? myModules.isPending : academicNavigation.isPending && academicNavigation.fetchStatus !== 'idle')
  const commError = myPermissions.isError || myModules.isError || academicNavigation.isError

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
                    {user.role === 'admin' && myPermissions.data?.includes('system:read') && <Link to="/admin/control" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m10ControlCenter')}</Link>}
                    {user.role === 'admin' && myPermissions.data?.includes('users:read') && moduleEnabled('permissions') && <Link to="/admin/users" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_users')}</Link>}
                    {myPermissions.data?.includes('permissions:read') && moduleEnabled('permissions') && <Link to="/admin/permissions" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('permissionEditor')}</Link>}
                    {myPermissions.data?.includes('school_settings:read') && moduleEnabled('school_settings') && <Link to="/admin/school-settings" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('schoolSettings')}</Link>}
                    {myPermissions.data?.includes('academics:read') && academicEnabled && <><Link to="/admin/academic/classes" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('classesHeading')}</Link><Link to="/admin/academic/sections" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('sectionsHeading')}</Link><Link to="/admin/academic/subjects" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('subjectsHeading')}</Link></>}
                    {myPermissions.data?.includes('academic_sessions:read') && academicEnabled && <Link to="/admin/academic/sessions" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('academicSessions')}</Link>}
                    {myPermissions.data?.includes('staff:read') && staffEnabled && <Link to="/admin/staff" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('staffHeading')}</Link>}
                    {myPermissions.data?.includes('staff:read_self') && staffEnabled && <Link to="/staff/me" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('staffMyProfile')}</Link>}
                    {myPermissions.data?.includes('attendance:students_read') && attendanceEnabled && <Link to="/admin/attendance/students" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('studentAttendanceHeading')}</Link>}
                    {myPermissions.data?.includes('attendance:students_read_self') && attendanceEnabled && <Link to="/student/attendance" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('studentAttendanceReport')}</Link>}
                    {myPermissions.data?.includes('attendance:teachers_read') && attendanceEnabled && <Link to="/admin/attendance/teachers" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('attendanceHeading')}</Link>}
                    {user.role !== 'admin' && myPermissions.data?.includes('attendance:teachers_read_self') && attendanceEnabled && <Link to="/attendance/me" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('attendanceMyHeading')}</Link>}
                    {myPermissions.data?.includes('students:read') && peopleEnabled && <Link to="/admin/students" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('studentsHeading')}</Link>}
                    {myPermissions.data?.includes('guardians:read') && peopleEnabled && <Link to="/admin/guardians" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('guardiansHeading')}</Link>}
                    {myPermissions.data?.includes('students:read_self') && peopleEnabled && <Link to="/student/me" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('myStudentProfile')}</Link>}
                    {myPermissions.data?.includes('guardians:read_children') && peopleEnabled && <Link to="/guardian/children" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('guardianChildren')}</Link>}
                    {myPermissions.data?.includes('routine:read') && routineEnabled && <Link to="/admin/routines" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('routineHeading')}</Link>}
                    {user.role === 'teacher' && myPermissions.data?.includes('routine:read_self') && routineEnabled && <Link to="/teacher/routine" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('classRoutine')}</Link>}
                    {user.role === 'student' && myPermissions.data?.includes('routine:read_self') && routineEnabled && <Link to="/student/routine" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('routineHeading')}</Link>}
                    {myPermissions.data?.includes('exams:read') && examsEnabled && <Link to="/admin/exams" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('examHeading')}</Link>}
                    {user.role === 'teacher' && myPermissions.data?.includes('exams:enter_marks') && examsEnabled && <Link to="/teacher/exams" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('examMyMarks')}</Link>}
                    {user.role === 'student' && myPermissions.data?.includes('exams:read_self') && examsEnabled && <Link to="/student/results" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('examMyResults')}</Link>}
                    {myPermissions.data?.includes('fees:read') && feesEnabled && <Link to="/accounts/fees" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('feeHeading')}</Link>}
                    {user.role === 'student' && myPermissions.data?.includes('fees:read_self') && feesEnabled && <Link to="/student/fees" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('feeMyFees')}</Link>}
                    {myPermissions.data?.includes('notices:read') && noticesEnabled && <Link to="/notices" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m9Notices')}</Link>}
                    {myPermissions.data?.includes('events:read') && noticesEnabled && <Link to="/events" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m9Events')}</Link>}
                    {(user.role === 'teacher' || user.role === 'head_teacher') && myPermissions.data?.includes('diary:write') && diaryEnabled && <Link to="/teacher/diary" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m9TeacherDiary')}</Link>}
                    {myPermissions.data?.includes('diary:read') && diaryEnabled && <Link to="/admin/diary" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m9DiaryOversight')}</Link>}
                    {user.role === 'student' && myPermissions.data?.includes('diary:read_self') && diaryEnabled && <Link to="/student/diary" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m9MyDiary')}</Link>}
                    {user.role === 'admin' && myPermissions.data?.includes('notifications:manage') && noticesEnabled && <Link to="/admin/notifications" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('m9Delivery')}</Link>}
                    {analyticsEnabled && (myPermissions.data?.includes('analytics:read') || myPermissions.data?.includes('analytics:finance')) && <Link to="/analytics" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_analytics')}</Link>}
                    {analyticsEnabled && !myPermissions.data?.includes('analytics:read') && myPermissions.data?.includes('analytics:read_assigned') && <Link to="/teacher/analytics" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_analytics_teacher')}</Link>}
                    {analyticsEnabled && myPermissions.data?.some(p => p === 'analytics:read' || p === 'analytics:read_assigned' || p === 'analytics:finance') && <Link to="/reports" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_reports')}</Link>}
                    {analyticsEnabled && user.role === 'student' && myPermissions.data?.includes('analytics:read_self') && <Link to="/student/progress" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_analytics_student')}</Link>}
                    {analyticsEnabled && user.role === 'guardian' && myPermissions.data?.includes('analytics:read_children') && <Link to="/guardian/progress" className="text-sm font-semibold text-slate-700 hover:text-teal-700">{t('nav_analytics_guardian')}</Link>}
                    {myPermissions.data?.includes('notifications:read_self') && noticesEnabled && <NotificationBell />}
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
          <Route path="/admin/control" element={<ProtectedRoute><AdminGate kind="overview" permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/audit" element={<ProtectedRoute><AdminGate kind="audit" permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/exports" element={<ProtectedRoute><AdminGate kind="exports" permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/backups" element={<ProtectedRoute><AdminGate kind="backups" permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/modules" element={<ProtectedRoute><AdminGate kind="modules" permissions={myPermissions.data} role={user?.role} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/school-settings" element={<ProtectedRoute><SchoolGate permissions={myPermissions.data} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/academic/classes" element={<ProtectedRoute><AcademicGate kind="classes" permissions={myPermissions.data} role={user?.role} enabled={academicEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/admin/academic/subjects" element={<ProtectedRoute><AcademicGate kind="subjects" permissions={myPermissions.data} role={user?.role} enabled={academicEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/admin/academic/sections" element={<ProtectedRoute><AcademicGate kind="sections" permissions={myPermissions.data} role={user?.role} enabled={academicEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/admin/academic/sessions" element={<ProtectedRoute><AcademicSessionsGate permissions={myPermissions.data} loading={myPermissions.isPending} error={myPermissions.isError} /></ProtectedRoute>} />
          <Route path="/admin/staff" element={<ProtectedRoute><StaffGate permissions={myPermissions.data} own={false} enabled={staffEnabled} loading={staffLoading} error={staffError} /></ProtectedRoute>} />
          <Route path="/staff/me" element={<ProtectedRoute><StaffGate permissions={myPermissions.data} own enabled={staffEnabled} loading={staffLoading} error={staffError} /></ProtectedRoute>} />
          <Route path="/admin/attendance/students" element={<ProtectedRoute><StudentAttendanceGate kind="manage" permissions={myPermissions.data} enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
          <Route path="/student/attendance" element={<ProtectedRoute><StudentAttendanceGate kind="self" permissions={myPermissions.data} enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
          <Route path="/guardian/children/:id/attendance" element={<ProtectedRoute><StudentAttendanceGate kind="guardian" permissions={myPermissions.data} enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
          <Route path="/admin/attendance/teachers" element={<ProtectedRoute><AttendanceGate permissions={myPermissions.data} own={false} enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
          <Route path="/attendance/me" element={<ProtectedRoute><AttendanceGate permissions={myPermissions.data} own enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
          <Route path="/admin/students" element={<ProtectedRoute><PeopleGate kind="students" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/admin/students/:id" element={<ProtectedRoute><PeopleGate kind="profile" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/admin/guardians" element={<ProtectedRoute><PeopleGate kind="guardians" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/student/me" element={<ProtectedRoute><PeopleGate kind="self" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/guardian/children" element={<ProtectedRoute><PeopleGate kind="children" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} routineEnabled={routineEnabled} examsEnabled={examsEnabled} feesEnabled={feesEnabled} diaryEnabled={diaryEnabled} /></ProtectedRoute>} />
          <Route path="/guardian/children/:id" element={<ProtectedRoute><PeopleGate kind="child" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
          <Route path="/admin/routines" element={<ProtectedRoute><RoutineGate mode="admin" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
          <Route path="/teacher/routine" element={<ProtectedRoute><RoutineGate mode="teacher" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
          <Route path="/student/routine" element={<ProtectedRoute><RoutineGate mode="student" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
          <Route path="/guardian/children/:id/routine" element={<ProtectedRoute><RoutineGate mode="guardian" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
          <Route path="/admin/exams" element={<ProtectedRoute><ExamsGate mode="admin" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
          <Route path="/teacher/exams" element={<ProtectedRoute><ExamsGate mode="teacher" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
          <Route path="/student/results" element={<ProtectedRoute><ExamsGate mode="student" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
          <Route path="/guardian/children/:id/results" element={<ProtectedRoute><ExamsGate mode="guardian" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
          <Route path="/notices" element={<ProtectedRoute><CommGate kind="notices" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/events" element={<ProtectedRoute><CommGate kind="events" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/notifications" element={<ProtectedRoute><CommGate kind="notifications" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/admin/notifications" element={<ProtectedRoute><CommGate kind="delivery" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/teacher/diary" element={<ProtectedRoute><CommGate kind="teacher-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/student/diary" element={<ProtectedRoute><CommGate kind="student-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/guardian/children/:id/diary" element={<ProtectedRoute><CommGate kind="guardian-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/admin/diary" element={<ProtectedRoute><CommGate kind="admin-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
          <Route path="/accounts/fees" element={<ProtectedRoute><FeesGate mode="staff" permissions={myPermissions.data} role={user?.role} enabled={feesEnabled} loading={feesLoading} error={feesError} /></ProtectedRoute>} />
          <Route path="/student/fees" element={<ProtectedRoute><FeesGate mode="student" permissions={myPermissions.data} role={user?.role} enabled={feesEnabled} loading={feesLoading} error={feesError} /></ProtectedRoute>} />
          <Route path="/guardian/children/:id/fees" element={<ProtectedRoute><FeesGate mode="guardian" permissions={myPermissions.data} role={user?.role} enabled={feesEnabled} loading={feesLoading} error={feesError} /></ProtectedRoute>} />
          <Route path="/analytics" element={<ProtectedRoute><AnalyticsGate kind="school" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
          <Route path="/teacher/analytics" element={<ProtectedRoute><AnalyticsGate kind="teacher" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
          <Route path="/reports" element={<ProtectedRoute><AnalyticsGate kind="reports" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
          <Route path="/student/progress" element={<ProtectedRoute><AnalyticsGate kind="student" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
          <Route path="/guardian/progress" element={<ProtectedRoute><AnalyticsGate kind="guardian" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
          <Route path="/analytics/students/:id" element={<ProtectedRoute><AnalyticsGate kind="staff-student" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
    </div>
  )
}
