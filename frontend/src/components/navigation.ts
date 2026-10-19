import type { IconName } from './icons'

// Navigation is derived from the signed-in user's live permissions, role and
// enabled modules, using the same rules as the route gates, so a link is only
// offered when its page will open. The server still enforces every rule.
export type NavItem = { key: string; label: string; path: string; icon: IconName; exact?: boolean }
export type NavGroup = { key: string; label: string; items: NavItem[] }
export type NavContext = { role: string; has: (permission: string) => boolean; on: (module: string) => boolean }

export function buildNavigation({ role, has, on }: NavContext): NavGroup[] {
  const admin = role === 'admin'
  const groups: NavGroup[] = [
    {
      key: 'overview', label: 'm12Group_overview', items: [
        { key: 'dashboard', label: 'm12Nav_dashboard', path: '/dashboard', icon: 'home' },
        ...(on('analytics') && (has('analytics:read') || has('analytics:finance')) ? [{ key: 'analytics', label: 'm12Nav_analytics', path: '/analytics', icon: 'chart' as const, exact: true }] : []),
        ...(on('analytics') && !has('analytics:read') && has('analytics:read_assigned') ? [{ key: 'analytics_teacher', label: 'm12Nav_classAnalytics', path: '/teacher/analytics', icon: 'chart' as const }] : []),
        ...(on('analytics') && role === 'student' && has('analytics:read_self') ? [{ key: 'progress', label: 'm12Nav_myProgress', path: '/student/progress', icon: 'trend' as const }] : []),
        ...(on('analytics') && role === 'guardian' && has('analytics:read_children') ? [{ key: 'progress', label: 'm12Nav_childrenProgress', path: '/guardian/progress', icon: 'trend' as const }] : []),
        ...(on('analytics') && (has('analytics:read') || has('analytics:read_assigned') || has('analytics:finance')) ? [{ key: 'reports', label: 'm12Nav_reports', path: '/reports', icon: 'report' as const }] : []),
      ],
    },
    {
      key: 'people', label: 'm12Group_people', items: [
        ...(on('students') && has('students:read') && (admin || role === 'head_teacher') ? [{ key: 'students', label: 'm12Nav_students', path: '/admin/students', icon: 'users' as const }] : []),
        ...(on('students') && has('guardians:read') && (admin || role === 'head_teacher') ? [{ key: 'guardians', label: 'm12Nav_guardians', path: '/admin/guardians', icon: 'user' as const }] : []),
        ...(on('staff') && has('staff:read') ? [{ key: 'staff', label: 'm12Nav_staff', path: '/admin/staff', icon: 'teacher' as const }] : []),
        ...(on('students') && has('students:read_self') && role === 'student' ? [{ key: 'me', label: 'm12Nav_myProfile', path: '/student/me', icon: 'user' as const }] : []),
        ...(on('students') && has('guardians:read_children') && role === 'guardian' ? [{ key: 'children', label: 'm12Nav_myChildren', path: '/guardian/children', icon: 'users' as const }] : []),
        ...(on('staff') && has('staff:read_self') && !admin ? [{ key: 'staff_me', label: 'm12Nav_myStaffProfile', path: '/staff/me', icon: 'user' as const }] : []),
      ],
    },
    {
      key: 'academics', label: 'm12Group_academics', items: [
        ...(on('academics') && has('academics:read') ? [
          { key: 'classes', label: 'm12Nav_classes', path: '/admin/academic/classes', icon: 'layers' as const },
          { key: 'sections', label: 'm12Nav_sections', path: '/admin/academic/sections', icon: 'layers' as const },
          { key: 'subjects', label: 'm12Nav_subjects', path: '/admin/academic/subjects', icon: 'book' as const },
        ] : []),
        ...(on('academics') && has('academic_sessions:read') ? [{ key: 'sessions', label: 'm12Nav_sessions', path: '/admin/academic/sessions', icon: 'calendar' as const }] : []),
        ...(on('routine') && has('routine:read') ? [{ key: 'routines', label: 'm12Nav_routines', path: '/admin/routines', icon: 'clock' as const }] : []),
        ...(on('routine') && has('routine:read_self') && role === 'teacher' ? [{ key: 'routine_me', label: 'm12Nav_myRoutine', path: '/teacher/routine', icon: 'clock' as const }] : []),
        ...(on('routine') && has('routine:read_self') && role === 'student' ? [{ key: 'routine_me', label: 'm12Nav_classRoutine', path: '/student/routine', icon: 'clock' as const }] : []),
        ...(on('examinations') && has('exams:read') ? [{ key: 'exams', label: 'm12Nav_exams', path: '/admin/exams', icon: 'grade' as const }] : []),
        ...(on('examinations') && has('exams:enter_marks') && role === 'teacher' ? [{ key: 'exams_me', label: 'm12Nav_marksEntry', path: '/teacher/exams', icon: 'grade' as const }] : []),
        ...(on('examinations') && has('exams:read_self') && role === 'student' ? [{ key: 'results', label: 'm12Nav_myResults', path: '/student/results', icon: 'grade' as const }] : []),
        ...(on('diary') && has('diary:write') && (role === 'teacher' || role === 'head_teacher') ? [{ key: 'diary', label: 'm12Nav_classDiary', path: '/teacher/diary', icon: 'notebook' as const }] : []),
        ...(on('diary') && has('diary:read_self') && role === 'student' ? [{ key: 'diary_me', label: 'm12Nav_classDiary', path: '/student/diary', icon: 'notebook' as const }] : []),
        ...(on('diary') && has('diary:read') ? [{ key: 'diary_admin', label: 'm12Nav_diaryOversight', path: '/admin/diary', icon: 'notebook' as const }] : []),
      ],
    },
    {
      key: 'attendance', label: 'm12Group_attendance', items: [
        ...(on('attendance') && has('attendance:students_read') ? [{ key: 'student_attendance', label: 'm12Nav_studentAttendance', path: '/admin/attendance/students', icon: 'check' as const }] : []),
        ...(on('attendance') && has('attendance:teachers_read') ? [{ key: 'teacher_attendance', label: 'm12Nav_teacherAttendance', path: '/admin/attendance/teachers', icon: 'clipboard' as const }] : []),
        ...(on('attendance') && !admin && has('attendance:teachers_read_self') ? [{ key: 'attendance_me', label: 'm12Nav_myAttendance', path: '/attendance/me', icon: 'clipboard' as const }] : []),
        ...(on('attendance') && has('attendance:students_read_self') && role === 'student' ? [{ key: 'attendance_self', label: 'm12Nav_myAttendance', path: '/student/attendance', icon: 'check' as const }] : []),
      ],
    },
    {
      key: 'finance', label: 'm12Group_finance', items: [
        ...(on('fees') && has('fees:read') ? [{ key: 'fees', label: 'm12Nav_fees', path: '/accounts/fees', icon: 'money' as const }] : []),
        ...(on('fees') && has('fees:read_self') && role === 'student' ? [{ key: 'fees_me', label: 'm12Nav_myFees', path: '/student/fees', icon: 'receipt' as const }] : []),
      ],
    },
    {
      key: 'communication', label: 'm12Group_communication', items: [
        ...(on('notices') && has('notices:read') ? [{ key: 'notices', label: 'm12Nav_notices', path: '/notices', icon: 'megaphone' as const }] : []),
        ...(on('notices') && has('events:read') ? [{ key: 'events', label: 'm12Nav_events', path: '/events', icon: 'calendar' as const }] : []),
        ...(on('notices') && has('notifications:read_self') ? [{ key: 'notifications', label: 'm12Nav_notifications', path: '/notifications', icon: 'bell' as const }] : []),
        ...(on('notices') && admin && has('notifications:manage') ? [{ key: 'delivery', label: 'm12Nav_delivery', path: '/admin/notifications', icon: 'megaphone' as const }] : []),
      ],
    },
    {
      key: 'administration', label: 'm12Group_administration', items: [
        ...(admin && has('system:read') ? [{ key: 'control', label: 'm12Nav_controlCenter', path: '/admin/control', icon: 'shield' as const }] : []),
        ...(admin && has('users:read') && on('permissions') ? [{ key: 'users', label: 'm12Nav_users', path: '/admin/users', icon: 'users' as const }] : []),
        ...(has('permissions:read') && on('permissions') ? [{ key: 'permissions', label: 'm12Nav_permissions', path: '/admin/permissions', icon: 'shield' as const }] : []),
        ...(admin && has('modules:read') ? [{ key: 'modules', label: 'm12Nav_modules', path: '/admin/modules', icon: 'layers' as const }] : []),
        ...(admin && has('audit:read') ? [{ key: 'audit', label: 'm12Nav_audit', path: '/admin/audit', icon: 'report' as const }] : []),
        ...(admin && has('data:export') ? [{ key: 'exports', label: 'm12Nav_exports', path: '/admin/exports', icon: 'database' as const }] : []),
        ...(admin && has('backups:manage') ? [{ key: 'backups', label: 'm12Nav_backups', path: '/admin/backups', icon: 'database' as const }] : []),
        ...(has('school_settings:read') && on('school_settings') ? [{ key: 'settings', label: 'm12Nav_schoolSettings', path: '/admin/school-settings', icon: 'settings' as const }] : []),
      ],
    },
  ]
  return groups.filter(g => g.items.length > 0)
}

// activeItem finds the navigation entry for the current path: an exact match
// first, then the longest prefix match (so /admin/students/123 highlights
// Students).
export function activeItem(groups: NavGroup[], pathname: string): NavItem | undefined {
  const items = groups.flatMap(g => g.items)
  const exact = items.find(i => i.path === pathname)
  if (exact) return exact
  return items.filter(i => !i.exact && pathname.startsWith(i.path + '/')).sort((a, b) => b.path.length - a.path.length)[0]
}
