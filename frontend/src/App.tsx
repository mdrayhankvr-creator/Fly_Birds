import { lazy, Suspense, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Navigate, Route, Routes, useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getDashboard, getMyPermissions, getSchoolBranding, getSchoolModules, type SchoolBranding } from './api/client'
import { useAuth } from './auth/AuthProvider'
import { AppShell, LanguageSwitch, SchoolMark, ThemeSwitch } from './components/AppShell'
import { applyTheme } from './components/theme'
import { buildNavigation } from './components/navigation'
import { ConfirmHost } from './components/ConfirmDialog'
import { ProtectedRoute } from './auth/ProtectedRoute'
import { LandingPage } from './pages/LandingPage'
import { StateScreen } from './components/StateScreen'

// Each page loads on first visit, so phones download only the screens they open.
const AccountPage = lazy(() => import('./pages/AccountPage').then(m => ({ default: m.AccountPage })))
const PermissionPage = lazy(() => import('./pages/PermissionPage').then(m => ({ default: m.PermissionPage })))
const SchoolSettingsPage = lazy(() => import('./pages/SchoolSettingsPage').then(m => ({ default: m.SchoolSettingsPage })))
const UsersPage = lazy(() => import('./pages/UsersPage').then(m => ({ default: m.UsersPage })))
const DashboardPage = lazy(() => import('./pages/DashboardPage').then(m => ({ default: m.DashboardPage })))
const AcademicPage = lazy(() => import('./pages/AcademicPage').then(m => ({ default: m.AcademicPage })))
const AcademicSessionsPage = lazy(() => import('./pages/AcademicSessionsPage').then(m => ({ default: m.AcademicSessionsPage })))
const StaffPage = lazy(() => import('./pages/StaffPage').then(m => ({ default: m.StaffPage })))
const TeacherAttendancePage = lazy(() => import('./pages/TeacherAttendancePage').then(m => ({ default: m.TeacherAttendancePage })))
const StudentAttendancePage = lazy(() => import('./pages/StudentAttendancePage').then(m => ({ default: m.StudentAttendancePage })))
const StudentAttendanceReportPage = lazy(() => import('./pages/StudentAttendancePage').then(m => ({ default: m.StudentAttendanceReportPage })))
const SectionsPage = lazy(() => import('./pages/SectionsPage').then(m => ({ default: m.SectionsPage })))
const StudentsPage = lazy(() => import('./pages/StudentsPage').then(m => ({ default: m.StudentsPage })))
const StudentProfilePage = lazy(() => import('./pages/StudentProfilePage').then(m => ({ default: m.StudentProfilePage })))
const IdCardPrintPage = lazy(() => import('./pages/IdCardPrintPage').then(m => ({ default: m.IdCardPrintPage })))
const VerifyCardPage = lazy(() => import('./pages/VerifyCardPage').then(m => ({ default: m.VerifyCardPage })))
const RoutinePage = lazy(() => import('./pages/RoutinePage').then(m => ({ default: m.RoutinePage })))
const ExamsPage = lazy(() => import('./pages/ExamsPage').then(m => ({ default: m.ExamsPage })))
const MarksQueue = lazy(() => import('./pages/marks/MarksQueue').then(m => ({ default: m.MarksQueue })))
const MarksWorkspace = lazy(() => import('./pages/marks/MarksWorkspace').then(m => ({ default: m.MarksWorkspace })))
const ExamSetup = lazy(() => import('./pages/exams/ExamSetup').then(m => ({ default: m.ExamSetup })))
const BlueprintBuilder = lazy(() => import('./pages/exams/BlueprintBuilder').then(m => ({ default: m.BlueprintBuilder })))
const TermWizard = lazy(() => import('./pages/exams/TermWizard').then(m => ({ default: m.TermWizard })))
const TermResults = lazy(() => import('./pages/exams/TermResults').then(m => ({ default: m.TermResults })))
const ResultsCentre = lazy(() => import('./pages/exams/ResultsCentre').then(m => ({ default: m.ResultsCentre })))
const ResultReview = lazy(() => import('./pages/exams/ResultReview').then(m => ({ default: m.ResultReview })))
const FeesPage = lazy(() => import('./pages/FeesPage').then(m => ({ default: m.FeesPage })))
const MyFeesPage = lazy(() => import('./pages/MyFeesPage').then(m => ({ default: m.MyFeesPage })))
const NoticesPage = lazy(() => import('./pages/NoticesPage').then(m => ({ default: m.NoticesPage })))
const EventsPage = lazy(() => import('./pages/EventsPage').then(m => ({ default: m.EventsPage })))
const AdminNotificationsPage = lazy(() => import('./pages/NotificationsPage').then(m => ({ default: m.AdminNotificationsPage })))
const NotificationBell = lazy(() => import('./pages/NotificationsPage').then(m => ({ default: m.NotificationBell })))
const NotificationsPage = lazy(() => import('./pages/NotificationsPage').then(m => ({ default: m.NotificationsPage })))
const AdminDiaryPage = lazy(() => import('./pages/DiaryPages').then(m => ({ default: m.AdminDiaryPage })))
const DiaryFeedPage = lazy(() => import('./pages/DiaryPages').then(m => ({ default: m.DiaryFeedPage })))
const TeacherDiaryPage = lazy(() => import('./pages/DiaryPages').then(m => ({ default: m.TeacherDiaryPage })))
const AdminOverviewPage = lazy(() => import('./pages/AdminPages').then(m => ({ default: m.AdminOverviewPage })))
const AuditLogPage = lazy(() => import('./pages/AdminPages').then(m => ({ default: m.AuditLogPage })))
const BackupsPage = lazy(() => import('./pages/AdminPages').then(m => ({ default: m.BackupsPage })))
const DataExportPage = lazy(() => import('./pages/AdminPages').then(m => ({ default: m.DataExportPage })))
const ModulesPage = lazy(() => import('./pages/AdminPages').then(m => ({ default: m.ModulesPage })))
const AnalyticsPage = lazy(() => import('./pages/AnalyticsPages').then(m => ({ default: m.AnalyticsPage })))
const ChildrenProgressPage = lazy(() => import('./pages/AnalyticsPages').then(m => ({ default: m.ChildrenProgressPage })))
const MyProgressPage = lazy(() => import('./pages/AnalyticsPages').then(m => ({ default: m.MyProgressPage })))
const ReportsPage = lazy(() => import('./pages/AnalyticsPages').then(m => ({ default: m.ReportsPage })))
const StaffStudentPage = lazy(() => import('./pages/AnalyticsPages').then(m => ({ default: m.StaffStudentPage })))

function NotFound() {
  return <StateScreen kind="notFound" />
}

function PermissionGate({ permissions, role, loading, error }: { permissions: string[] | undefined; role: string | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (!permissions?.includes('permissions:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  return <PermissionPage canManage={role === 'admin' && permissions.includes('permissions:manage')} />
}

// AdminGate guards the M10 control-center pages: administrators only, with
// the page's own permission; the server enforces the same rules.
function AdminGate({ kind, permissions, role, loading, error }: { kind: 'overview' | 'audit' | 'exports' | 'backups' | 'modules'; permissions: string[] | undefined; role: string | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  const required = { overview: 'system:read', audit: 'audit:read', exports: 'data:export', backups: 'backups:manage', modules: 'modules:read' }[kind]
  if (role !== 'admin' || !permissions?.includes(required)) return <StateScreen kind="denied" message={t('permissionDenied')} />
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
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (!permissions?.includes('school_settings:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  return <SchoolSettingsPage permissions={permissions} />
}

function UsersGate({ permissions, role, loading, error }: { permissions: string[] | undefined; role: string | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (role !== 'admin' || !permissions?.includes('users:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  return <UsersPage permissions={permissions} />
}

function AcademicGate({ kind, permissions, role, enabled, loading, error }: { kind: 'classes' | 'subjects' | 'sections'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (!permissions?.includes('academics:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" message={t('academicModuleDisabled')} />
  const canManage = role === 'admin' && permissions.includes('academics:manage')
  return kind === 'sections' ? <SectionsPage canManage={canManage} /> : <AcademicPage kind={kind} canManage={canManage} />
}

function AcademicSessionsGate({ permissions, loading, error }: { permissions: string[] | undefined; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (!permissions?.includes('academic_sessions:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  return <AcademicSessionsPage permissions={permissions} />
}

function StaffGate({ permissions, own, enabled, loading, error }: { permissions: string[] | undefined; own: boolean; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (!permissions?.includes(own ? 'staff:read_self' : 'staff:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" message={t('staffModuleDisabled')} />
  return <StaffPage permissions={permissions} own={own} />
}

function AttendanceGate({ permissions, own, enabled, loading, error }: { permissions: string[] | undefined; own: boolean; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  if (!permissions?.includes(own ? 'attendance:teachers_read_self' : 'attendance:teachers_read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" message={t('attendanceModuleDisabled')} />
  return <TeacherAttendancePage permissions={permissions} own={own} />
}

function StudentAttendanceGate({ kind, permissions, enabled, loading, error }: { kind: 'manage' | 'self' | 'guardian'; permissions: string[] | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  const required = kind === 'manage' ? 'attendance:students_read' : kind === 'self' ? 'attendance:students_read_self' : 'attendance:guardians_read_children'
  if (!permissions?.includes(required)) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" message={t('attendanceModuleDisabled')} />
  return kind === 'manage' ? <StudentAttendancePage permissions={permissions} /> : <StudentAttendanceReportPage guardian={kind === 'guardian'} />
}

function PeopleGate({ kind, permissions, role, enabled, loading, error }: { kind: 'students' | 'profile' | 'self' | 'cards' | 'card'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('loadingPermissions')} />
  if (error) return <StateScreen kind="error" message={t('loadPermissionsError')} />
  const canStudents = !!permissions?.includes('students:read') && (role === 'admin' || role === 'head_teacher')
  const canGuardians = !!permissions?.includes('guardians:read') && (role === 'admin' || role === 'head_teacher')
  const canManageStudents = role === 'admin' && !!permissions?.includes('students:manage')
  const canManageGuardians = role === 'admin' && !!permissions?.includes('guardians:manage')
  const permitted = kind === 'cards' || kind === 'card' ? canManageStudents : kind === 'students' || kind === 'profile' ? canStudents : role === 'student' && !!permissions?.includes('students:read_self')
  // Permission first: someone without access learns that, not the module state.
  if (!permitted) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" message={t('peopleModuleDisabled')} />
  if (kind === 'students' && canStudents) return <StudentsPage canManage={canManageStudents} />
  if ((kind === 'cards' || kind === 'card') && canManageStudents) return <IdCardPrintPage mode={kind === 'card' ? 'single' : 'bulk'} />
  if (kind === 'profile' && canStudents) return <StudentProfilePage mode="staff" canManagePhoto={canManageStudents} canManageLinks={canManageGuardians} canReadGuardians={canGuardians} />
  if (kind === 'self' && role === 'student' && permissions?.includes('students:read_self')) return <StudentProfilePage mode="self" canManagePhoto={false} canManageLinks={false} canReadGuardians={false} />
  return <StateScreen kind="denied" message={t('permissionDenied')} />
}

function RoutineGate({ mode, permissions, role, enabled, loading, error }: { mode: 'admin' | 'student' | 'teacher' | 'guardian'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('routineLoading')} />
  if (error) return <StateScreen kind="error" message={t('routineLoadError')} />
  const required = mode === 'admin' ? 'routine:read' : mode === 'guardian' ? 'routine:read_children' : 'routine:read_self'
  if (!permissions?.includes(required) || (mode === 'student' && role !== 'student') || (mode === 'teacher' && role !== 'teacher') || (mode === 'guardian' && role !== 'guardian')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" />
  return <RoutinePage mode={mode} canManage={mode === 'admin' && role === 'admin' && permissions.includes('routine:manage')} />
}

// SheetGate opens one mark sheet: teachers through their own routes,
// administrators and head teachers (exams:read) through the staff routes.
function SheetGate({ scope, permissions, role, enabled, loading, error }: { scope: 'teacher' | 'staff'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('mwLoading')} />
  if (error) return <StateScreen kind="error" message={t('examError')} />
  const allowed = scope === 'teacher' ? role === 'teacher' && !!permissions?.includes('exams:enter_marks') : role !== 'teacher' && !!permissions?.includes('exams:read')
  if (!allowed) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" />
  return <MarksWorkspace scope={scope} />
}

// SetupGate guards the E5 exam setup screens: staff with exams:read may look
// (the head teacher), administrators with exams:manage may change them.
function SetupGate({ page, permissions, role, enabled, loading, error }: { page: 'setup' | 'builder' | 'wizard' | 'results' | 'centre' | 'review'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('examLoading')} />
  if (error) return <StateScreen kind="error" message={t('examError')} />
  if (role === 'teacher' || role === 'student' || role === 'guardian' || !permissions?.includes('exams:read')) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" />
  const canManage = role === 'admin' && permissions.includes('exams:manage')
  switch (page) {
    case 'setup': return <ExamSetup canManage={canManage} />
    case 'builder': return <BlueprintBuilder canManage={canManage} />
    case 'wizard': return <TermWizard canManage={canManage} />
    // E6: the head teacher approves (exams:review); administrators publish.
    case 'centre': return <ResultsCentre />
    case 'review': return <ResultReview canApprove={(role === 'admin' || role === 'head_teacher') && permissions.includes('exams:review')} canPublish={canManage} />
    default: return <TermResults />
  }
}

function ExamsGate({ mode, permissions, role, enabled, loading, error }: { mode: 'admin' | 'student' | 'teacher' | 'guardian'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('examLoading')} />
  if (error) return <StateScreen kind="error" message={t('examError')} />
  const required = mode === 'admin' ? 'exams:read' : mode === 'teacher' ? 'exams:enter_marks' : mode === 'guardian' ? 'exams:read_children' : 'exams:read_self'
  if (!permissions?.includes(required) || mode !== 'admin' && role !== mode) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" />
  if (mode === 'teacher') return <MarksQueue />
  return <ExamsPage mode={mode} canManage={mode === 'admin' && role === 'admin' && permissions.includes('exams:manage')} canReview={mode === 'admin' && (role === 'admin' || role === 'head_teacher') && permissions.includes('exams:review')} />
}

function CommGate({ kind, permissions, role, enabled, loading, error }: { kind: 'notices' | 'events' | 'notifications' | 'delivery' | 'teacher-diary' | 'student-diary' | 'guardian-diary' | 'admin-diary'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('m9Loading')} />
  if (error) return <StateScreen kind="error" message={t('m9ErrGeneric')} />
  const has = (p: string) => !!permissions?.includes(p)
  const allowed = enabled && ({
    notices: has('notices:read'), events: has('events:read'), notifications: has('notifications:read_self'), delivery: has('notifications:manage') && role === 'admin',
    'teacher-diary': has('diary:write') && (role === 'teacher' || role === 'head_teacher'), 'student-diary': has('diary:read_self') && role === 'student',
    'guardian-diary': has('diary:read_children') && role === 'guardian', 'admin-diary': has('diary:read'),
  } as Record<string, boolean>)[kind]
  if (!allowed) return <StateScreen kind={enabled ? 'denied' : 'disabled'} message={t(enabled ? 'permissionDenied' : 'm9ErrModule')} />
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
  if (loading) return <StateScreen kind="loading" message={t('feeLoading')} />
  if (error) return <StateScreen kind="error" message={t('feeErrGeneric')} />
  const required = mode === 'staff' ? 'fees:read' : mode === 'guardian' ? 'fees:read_children' : 'fees:read_self'
  if (!permissions?.includes(required) || (mode !== 'staff' && role !== mode)) return <StateScreen kind="denied" message={t('permissionDenied')} />
  if (!enabled) return <StateScreen kind="disabled" />
  if (mode !== 'staff') return <MyFeesPage mode={mode} canPay={permissions.includes('fees:pay_online')} />
  const writer = role === 'admin' || role === 'accountant'
  return <FeesPage access={{ role: role ?? '', canManage: writer && permissions.includes('fees:manage'), canCollect: writer && permissions.includes('fees:collect'), canReadAccounts: permissions.includes('accounts:read'), canManageAccounts: writer && permissions.includes('accounts:manage') }} />
}

// AnalyticsGate guards the M11 pages. The server enforces the same rules and
// narrows teacher, student and guardian data to their own records.
function AnalyticsGate({ kind, permissions, role, enabled, loading, error }: { kind: 'school' | 'teacher' | 'reports' | 'student' | 'guardian' | 'staff-student'; permissions: string[] | undefined; role: string | undefined; enabled: boolean; loading: boolean; error: boolean }) {
  const { t } = useTranslation()
  if (loading) return <StateScreen kind="loading" message={t('m11Eyebrow')} />
  if (error) return <StateScreen kind="error" message={t('m11ErrGeneric')} />
  const has = (p: string) => !!permissions?.includes(p)
  const access = { school: has('analytics:read'), teacher: has('analytics:read_assigned'), finance: has('analytics:finance'), manage: has('analytics:manage') && role === 'admin' }
  const allowed = enabled && ({
    school: access.school || access.finance, teacher: access.teacher && !access.school, reports: access.school || access.teacher || access.finance,
    student: has('analytics:read_self') && role === 'student', guardian: has('analytics:read_children') && role === 'guardian', 'staff-student': access.school || access.teacher,
  } as Record<string, boolean>)[kind]
  if (!allowed) return <StateScreen kind={enabled ? 'denied' : 'disabled'} message={t(enabled ? 'permissionDenied' : 'm11ErrModule')} />
  switch (kind) {
    case 'reports': return <ReportsPage access={access} />
    case 'student': return <MyProgressPage />
    case 'guardian': return <ChildrenProgressPage />
    case 'staff-student': return <StaffStudentPage />
    default: return <AnalyticsPage access={kind === 'teacher' ? { ...access, school: false, finance: false } : access} />
  }
}

// Relative luminance (WCAG) of a #rrggbb colour.
function luminance(hex: string) {
  const [r, g, b] = [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16) / 255).map(c => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
const contrast = (a: number, b: number) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)

// mixToward blends a colour toward black or white by a fraction.
function mixToward(hex: string, target: number, amount: number) {
  return '#' + [1, 3, 5].map(i => Math.round(parseInt(hex.slice(i, i + 2), 16) * (1 - amount) + target * amount).toString(16).padStart(2, '0')).join('')
}

// applyBranding sets the school's colours and derives contrast-safe
// companions, so any white-label colour stays readable: --brand-ink is the
// brand darkened until it reads at 4.5:1 or better on its own light tint, and so on white (for text and icons), and
// --brand-on is white or near-black, whichever reads on the brand colour.
function applyBranding(branding: SchoolBranding | undefined) {
  const root = document.documentElement
  const validColor = (value: string | undefined) => value && /^#[0-9a-fA-F]{6}$/.test(value) ? value : null
  const primary = validColor(branding?.primary_color) || '#1D4ED8'
  let ink = primary
  // Brand text must stay readable on white and on the 9% brand tint used
  // for selected items (--brand-soft), so measure against the tint.
  const tint = luminance(mixToward(primary, 255, 0.91))
  for (let step = 1; step <= 14 && contrast(luminance(ink), tint) < 4.6; step++) ink = mixToward(primary, 0, step * 0.05)
  const onBrand = contrast(luminance(primary), 1) >= contrast(luminance(primary), luminance('#0f172a')) ? '#ffffff' : '#0f172a'
  root.style.setProperty('--brand-primary', primary)
  root.style.setProperty('--brand-ink', ink)
  root.style.setProperty('--brand-on', onBrand)
  root.style.setProperty('--brand-secondary', validColor(branding?.secondary_color) || '#0F172A')
  applyTheme(branding?.dashboard_theme === 'dark' ? 'dark' : 'light')
}

export default function App() {
  const { t, i18n } = useTranslation()
  const { user, accessToken, ready, signOut, renew, expire } = useAuth()
  const [logoutError, setLogoutError] = useState(false)
  const { pathname } = useLocation()
  const landing = pathname === '/' || pathname === '/login'
  // These pages show the school's logo and name themselves; the header does not repeat them.
  const pageShowsSchool = landing || pathname.startsWith('/verify/')
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

  const navigation = user ? buildNavigation({
    role: user.role,
    has: permission => !!myPermissions.data?.includes(permission),
    on: key => ({ academics: academicEnabled, students: peopleEnabled, staff: staffEnabled, attendance: attendanceEnabled, routine: routineEnabled, examinations: examsEnabled, fees: feesEnabled, notices: noticesEnabled, diary: diaryEnabled, analytics: analyticsEnabled } as Record<string, boolean>)[key] ?? moduleEnabled(key),
  }) : []
  const routes = (
    <Suspense fallback={<StateScreen kind="loading" />}>
      <Routes>
        <Route path="/" element={user ? <Navigate to="/dashboard" replace /> : <LandingPage branding={branding.data} />} />
        <Route path="/login" element={<LandingPage branding={branding.data} />} />
        <Route path="/verify/:token" element={<VerifyCardPage />} />
        <Route path="/account" element={<ProtectedRoute><AccountPage permissions={myPermissions.data} /></ProtectedRoute>} />
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
        {/* Families use the student's account; old guardian links go home. */}
        <Route path="/guardian/*" element={<Navigate to="/dashboard" replace />} />
        <Route path="/admin/attendance/teachers" element={<ProtectedRoute><AttendanceGate permissions={myPermissions.data} own={false} enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
        <Route path="/attendance/me" element={<ProtectedRoute><AttendanceGate permissions={myPermissions.data} own enabled={attendanceEnabled} loading={attendanceLoading} error={attendanceError} /></ProtectedRoute>} />
        <Route path="/admin/students" element={<ProtectedRoute><PeopleGate kind="students" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
        <Route path="/admin/students/:id" element={<ProtectedRoute><PeopleGate kind="profile" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
        <Route path="/admin/students/cards" element={<ProtectedRoute><PeopleGate kind="cards" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
        <Route path="/admin/students/:id/card" element={<ProtectedRoute><PeopleGate kind="card" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
        <Route path="/admin/guardians" element={<Navigate to="/admin/students" replace />} />
        <Route path="/student/me" element={<ProtectedRoute><PeopleGate kind="self" permissions={myPermissions.data} role={user?.role} enabled={peopleEnabled} loading={academicLoading} error={academicError} /></ProtectedRoute>} />
        <Route path="/admin/routines" element={<ProtectedRoute><RoutineGate mode="admin" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
        <Route path="/teacher/routine" element={<ProtectedRoute><RoutineGate mode="teacher" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
        <Route path="/student/routine" element={<ProtectedRoute><RoutineGate mode="student" permissions={myPermissions.data} role={user?.role} enabled={routineEnabled} loading={routineLoading} error={routineError} /></ProtectedRoute>} />
        <Route path="/admin/exams" element={<ProtectedRoute><ExamsGate mode="admin" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/teacher/exams" element={<ProtectedRoute><ExamsGate mode="teacher" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/teacher/exams/:examId/sheets/:sheetId" element={<ProtectedRoute><SheetGate scope="teacher" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/setup" element={<ProtectedRoute><SetupGate page="setup" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/blueprints/:versionId" element={<ProtectedRoute><SetupGate page="builder" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/terms/:termId/setup" element={<ProtectedRoute><SetupGate page="wizard" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/terms/:termId/results" element={<ProtectedRoute><SetupGate page="results" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/results" element={<ProtectedRoute><SetupGate page="centre" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/results/review" element={<ProtectedRoute><SetupGate page="review" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/admin/exams/:examId/sheets/:sheetId" element={<ProtectedRoute><SheetGate scope="staff" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/student/results" element={<ProtectedRoute><ExamsGate mode="student" permissions={myPermissions.data} role={user?.role} enabled={examsEnabled} loading={examsLoading} error={examsError} /></ProtectedRoute>} />
        <Route path="/notices" element={<ProtectedRoute><CommGate kind="notices" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/events" element={<ProtectedRoute><CommGate kind="events" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/notifications" element={<ProtectedRoute><CommGate kind="notifications" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/admin/notifications" element={<ProtectedRoute><CommGate kind="delivery" permissions={myPermissions.data} role={user?.role} enabled={noticesEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/teacher/diary" element={<ProtectedRoute><CommGate kind="teacher-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/student/diary" element={<ProtectedRoute><CommGate kind="student-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/admin/diary" element={<ProtectedRoute><CommGate kind="admin-diary" permissions={myPermissions.data} role={user?.role} enabled={diaryEnabled} loading={commLoading} error={commError} /></ProtectedRoute>} />
        <Route path="/accounts/fees" element={<ProtectedRoute><FeesGate mode="staff" permissions={myPermissions.data} role={user?.role} enabled={feesEnabled} loading={feesLoading} error={feesError} /></ProtectedRoute>} />
        <Route path="/student/fees" element={<ProtectedRoute><FeesGate mode="student" permissions={myPermissions.data} role={user?.role} enabled={feesEnabled} loading={feesLoading} error={feesError} /></ProtectedRoute>} />
        <Route path="/analytics" element={<ProtectedRoute><AnalyticsGate kind="school" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
        <Route path="/teacher/analytics" element={<ProtectedRoute><AnalyticsGate kind="teacher" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
        <Route path="/reports" element={<ProtectedRoute><AnalyticsGate kind="reports" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
        <Route path="/student/progress" element={<ProtectedRoute><AnalyticsGate kind="student" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
        <Route path="/analytics/students/:id" element={<ProtectedRoute><AnalyticsGate kind="staff-student" permissions={myPermissions.data} role={user?.role} enabled={analyticsEnabled} loading={analyticsLoading} error={myPermissions.isError || myModules.isError || academicNavigation.isError} /></ProtectedRoute>} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </Suspense>
  )

  if (ready && user) {
    return <AppShell branding={branding.data} user={user} groups={navigation} onLanguage={language => void setLanguage(language)} onSignOut={() => void handleLogout()}
      bell={myPermissions.data?.includes('notifications:read_self') && noticesEnabled && user?.role !== 'guardian' ? <Suspense fallback={null}><NotificationBell /></Suspense> : undefined}>
      {logoutError && <p role="alert" className="mb-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800">{t('logoutError')}</p>}
      {routes}
      <ConfirmHost />
    </AppShell>
  }
  return (
    <div className="shell-root min-h-screen">
      <header className="shell-topbar border-b border-slate-200">
        <div className={`mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6 ${pageShowsSchool ? 'justify-end' : 'justify-between'}`}>
          {!pageShowsSchool && <Link to="/" className="flex min-w-0 items-center gap-3 rounded-lg focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
            <SchoolMark branding={branding.data} />
            <span className="truncate text-base font-bold text-slate-900">{branding.data ? (i18n.language === 'bn' ? branding.data.name_bn : branding.data.name_en) : branding.isPending ? t('loadingBranding') : t('appName')}</span>
          </Link>}
          <div className="flex shrink-0 items-center gap-2 sm:gap-3">
            <LanguageSwitch onChange={language => void setLanguage(language)} />
            <ThemeSwitch />
            {ready && !landing && <Link to="/login" className="hidden rounded-lg px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-100 sm:inline-block">{t('signIn')}</Link>}
          </div>
        </div>
      </header>
      {/* The landing page runs edge to edge; other public pages keep the reading width. */}
      <main id="main" className={landing ? '' : 'mx-auto max-w-6xl px-4 py-10 sm:px-6 sm:py-14'}>{routes}</main>
      <ConfirmHost />
    </div>
  )
}
