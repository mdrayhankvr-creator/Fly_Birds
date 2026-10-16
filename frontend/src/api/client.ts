const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')

export type AuthUser = {
  id: string
  email: string
  role: string
  display_name?: string
}

export type AuthSession = {
  user: AuthUser
  access_token: string
}

export class ApiError extends Error {
  constructor(public status: number, public code: string) {
    super(code)
  }
}

function csrfToken(): string | null {
  const cookie = document.cookie.split('; ').find((entry) => entry.startsWith('flybirds_csrf='))
  return cookie ? decodeURIComponent(cookie.slice('flybirds_csrf='.length)) : null
}

export function hasRefreshSession(): boolean {
  return csrfToken() !== null
}

async function authRequest(path: string, options: RequestInit): Promise<Response> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    credentials: 'same-origin',
    cache: 'no-store',
    ...options,
  })
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new ApiError(response.status, body?.error?.code || 'request_failed')
  }
  return response
}

export async function login(email: string, password: string): Promise<AuthSession> {
  const response = await authRequest('/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  const body = await response.json()
  return body.data as AuthSession
}

let refreshInFlight: Promise<AuthSession> | null = null

export function refreshSession(): Promise<AuthSession> {
  if (refreshInFlight) return refreshInFlight
  const csrf = csrfToken()
  if (!csrf) return Promise.reject(new ApiError(401, 'invalid_session'))
  refreshInFlight = authRequest('/auth/refresh', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrf },
  }).then(async (response) => {
    const body = await response.json()
    return body.data as AuthSession
  }).finally(() => { refreshInFlight = null })
  return refreshInFlight
}

export async function logout(): Promise<void> {
  const csrf = csrfToken()
  if (!csrf) throw new ApiError(401, 'invalid_session')
  await authRequest('/auth/logout', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrf },
  })
}

export async function getCurrentUser(accessToken: string): Promise<AuthUser> {
  const response = await authRequest('/auth/me', {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await response.json()
  return body.data as AuthUser
}

export type Role = { id: string; name: string }
export type Permission = { id: string; description: string }
export type Page<T> = { items: T[]; total: number; page: number; page_size: number }

async function bearerRequest<T>(path: string, accessToken: string, options: RequestInit = {}): Promise<T> {
  const response = await authRequest(path, {
    ...options,
    headers: { Authorization: `Bearer ${accessToken}`, ...options.headers },
  })
  const body = await response.json()
  return body.data as T
}

export const getMyPermissions = (token: string) => bearerRequest<string[]>('/auth/permissions', token)
export const getRoles = (token: string) => bearerRequest<Page<Role>>('/roles?page_size=100', token)
export const getPermissions = (token: string) => bearerRequest<Page<Permission>>('/permissions?page_size=100', token)
export const getRolePermissions = (token: string, role: string) => bearerRequest<string[]>(`/roles/${encodeURIComponent(role)}/permissions`, token)
export const putRolePermissions = (token: string, role: string, ids: string[]) => bearerRequest<string[]>(
  `/roles/${encodeURIComponent(role)}/permissions`, token,
  { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids }) },
)

export type SchoolBranding = {
  name_en: string; name_bn: string; logo_url: string; preferred_language: 'en' | 'bn'
  primary_color: string; secondary_color: string; dashboard_theme: 'light' | 'dark'
}
export type SchoolSettings = SchoolBranding & {
  address: string; phone: string; email: string; grading_scheme_id: string; updated_at: string
}
export type SchoolSettingsInput = Omit<SchoolSettings, 'updated_at'>
export type GradeBand = { grade: string; min_marks: number; max_marks: number; gpa: number }
export type SchoolModule = { key: string; name: string; is_core: boolean; enabled: boolean }

export async function getSchoolBranding(signal?: AbortSignal): Promise<SchoolBranding> {
  const response = await authRequest('/school/branding', { signal })
  return (await response.json()).data as SchoolBranding
}
export const getSchoolSettings = (token: string) => bearerRequest<SchoolSettings>('/school/settings', token)
export const putSchoolSettings = (token: string, data: SchoolSettingsInput) => bearerRequest<SchoolSettings>('/school/settings', token, {
  method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data),
})
export const getSchoolGrading = (token: string) => bearerRequest<GradeBand[]>('/school/grading', token)
export const putSchoolGrading = (token: string, bands: GradeBand[]) => bearerRequest<GradeBand[]>('/school/grading', token, {
  method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ bands }),
})
export const getSchoolModules = (token: string) => bearerRequest<Page<SchoolModule>>('/school/modules?page_size=100', token)
export const putSchoolModule = (token: string, key: string, enabled: boolean) => bearerRequest<SchoolModule>(
  `/school/modules/${encodeURIComponent(key)}`, token,
  { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled }) },
)

export type ManagedUser = { id: string; email: string; display_name: string; role: string; is_active: boolean }
export type NewUser = { email: string; display_name: string; role: string; password: string }
export type DashboardNavigation = { key: string; name: string; path: string; coming_soon: boolean }
export type DashboardSummary = {
  total_users: number; active_users: number; inactive_users: number
  by_role: { role: string; count: number }[]; enabled_modules: number
  school_fields_complete: number; school_fields_total: number
}
export type DashboardData = { user: AuthUser; branding: SchoolBranding; navigation: DashboardNavigation[]; summary?: DashboardSummary }

export const getUsers = (token: string, params: URLSearchParams) => bearerRequest<Page<ManagedUser>>(`/users?${params}`, token)
export const getUser = (token: string, id: string) => bearerRequest<ManagedUser>(`/users/${encodeURIComponent(id)}`, token)
export const createUser = (token: string, input: NewUser) => bearerRequest<ManagedUser>('/users', token, {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
})
export const updateUser = (token: string, id: string, input: { email: string; display_name: string }) => bearerRequest<ManagedUser>(`/users/${encodeURIComponent(id)}`, token, {
  method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
})
export const setUserStatus = (token: string, id: string, is_active: boolean) => bearerRequest<ManagedUser>(`/users/${encodeURIComponent(id)}/status`, token, {
  method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ is_active }),
})
export const setUserRole = (token: string, id: string, role: string) => bearerRequest<string[]>(`/users/${encodeURIComponent(id)}/roles`, token, {
  method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids: [role] }),
})
export const getDashboard = (token: string) => bearerRequest<DashboardData>('/dashboard', token)

export type AcademicClass = { id: string; name: string; display_order: number; is_active: boolean }
export type AcademicSubject = { id: string; name_en: string; name_bn: string; code: string; is_active: boolean }
export type ClassSubject = { id: string; class_id: string; subject_id: string; is_required: boolean; is_fourth_subject: boolean; subject_name_en: string; subject_name_bn: string; subject_code: string }
export type AcademicClassInput = Omit<AcademicClass, 'id'>
export type AcademicSubjectInput = Omit<AcademicSubject, 'id'>
export const getAcademicClasses = (token: string, params: URLSearchParams) => bearerRequest<Page<AcademicClass>>(`/academic/classes?${params}`, token)
export const getAcademicSubjects = (token: string, params: URLSearchParams) => bearerRequest<Page<AcademicSubject>>(`/academic/subjects?${params}`, token)
export const saveAcademicClass = (token: string, input: AcademicClassInput, id?: string) => bearerRequest<AcademicClass>(id ? `/academic/classes/${encodeURIComponent(id)}` : '/academic/classes', token, { method: id ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
export const saveAcademicSubject = (token: string, input: AcademicSubjectInput, id?: string) => bearerRequest<AcademicSubject>(id ? `/academic/subjects/${encodeURIComponent(id)}` : '/academic/subjects', token, { method: id ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
export const getClassSubjects = (token: string, classId: string, params: URLSearchParams) => bearerRequest<Page<ClassSubject>>(`/academic/classes/${encodeURIComponent(classId)}/subjects?${params}`, token)
export const addClassSubject = (token: string, classId: string, subjectId: string, fourth: boolean) => bearerRequest<ClassSubject>(`/academic/classes/${encodeURIComponent(classId)}/subjects`, token, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ subject_id: subjectId, is_required: !fourth, is_fourth_subject: fourth }) })
export const updateClassSubject = (token: string, classId: string, subjectId: string, fourth: boolean) => bearerRequest<ClassSubject>(`/academic/classes/${encodeURIComponent(classId)}/subjects/${encodeURIComponent(subjectId)}`, token, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ is_required: !fourth, is_fourth_subject: fourth }) })
export const removeClassSubject = (token: string, classId: string, subjectId: string) => bearerRequest<{ deleted: boolean }>(`/academic/classes/${encodeURIComponent(classId)}/subjects/${encodeURIComponent(subjectId)}`, token, { method: 'DELETE' })

export type AcademicSession = {
  id: string; name: string; start_date: string; end_date: string
  is_active: boolean; created_at: string; updated_at: string
}
export type AcademicSessionInput = Pick<AcademicSession, 'name' | 'start_date' | 'end_date'>
export const getAcademicSessions = (token: string, params: URLSearchParams) => bearerRequest<Page<AcademicSession>>(`/academic/sessions?${params}`, token)
export const getAcademicSession = (token: string, id: string) => bearerRequest<AcademicSession>(`/academic/sessions/${encodeURIComponent(id)}`, token)
export const getCurrentAcademicSession = (token: string) => bearerRequest<AcademicSession>('/academic/sessions/current', token)
export const createAcademicSession = (token: string, input: AcademicSessionInput) => bearerRequest<AcademicSession>('/academic/sessions', token, {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
})
export const updateAcademicSession = (token: string, id: string, input: AcademicSessionInput) => bearerRequest<AcademicSession>(`/academic/sessions/${encodeURIComponent(id)}`, token, {
  method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
})
export const setAcademicSessionActive = (token: string, id: string, is_active: boolean) => bearerRequest<AcademicSession>(`/academic/sessions/${encodeURIComponent(id)}/activation`, token, {
  method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ is_active }),
})

export type Employee = { id: string; employee_code: string; user_id: string | null; kind: 'teacher' | 'non_teaching'; name_en: string; name_bn: string; designation: string; phone: string; joined_on: string; left_on: string | null; is_active: boolean }
export type EmployeeInput = Omit<Employee, 'id'>
export type TeacherAssignment = { id: string; employee_id: string; session_id: string; class_id: string; subject_id: string; session_name: string; class_name: string; subject_name_en: string; subject_name_bn: string }
export type EmployeeLeave = { id: string; employee_id: string; starts_on: string; ends_on: string; leave_type: string; reason: string; status: string; reviewed_by: string | null }
export type LeaveInput = Pick<EmployeeLeave, 'starts_on' | 'ends_on' | 'leave_type' | 'reason'>
export type SalaryRecord = { id: string; employee_id: string; effective_on: string; basic_bdt: string; allowance_bdt: string; deduction_bdt: string; note: string }
export type SalaryInput = Omit<SalaryRecord, 'id' | 'employee_id'>
const json = (method: string, body: unknown): RequestInit => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
export const getEmployees = (token: string, params: URLSearchParams) => bearerRequest<Page<Employee>>(`/staff/employees?${params}`, token)
export const getEmployee = (token: string, id: string) => bearerRequest<Employee>(`/staff/employees/${encodeURIComponent(id)}`, token)
export const getOwnEmployee = (token: string) => bearerRequest<Employee>('/staff/me', token)
export const saveEmployee = (token: string, input: EmployeeInput, id?: string) => bearerRequest<Employee>(id ? `/staff/employees/${encodeURIComponent(id)}` : '/staff/employees', token, json(id ? 'PATCH' : 'POST', input))
export const getTeacherAssignments = (token: string, id: string, params: URLSearchParams) => bearerRequest<Page<TeacherAssignment>>(`/staff/employees/${encodeURIComponent(id)}/assignments?${params}`, token)
export const getOwnAssignments = (token: string, params: URLSearchParams) => bearerRequest<Page<TeacherAssignment>>(`/staff/me/assignments?${params}`, token)
export const addTeacherAssignment = (token: string, id: string, input: { session_id: string; class_id: string; subject_id: string }) => bearerRequest<TeacherAssignment>(`/staff/employees/${encodeURIComponent(id)}/assignments`, token, json('POST', input))
export const deleteTeacherAssignment = (token: string, id: string, assignmentId: string) => bearerRequest<{ deleted: boolean }>(`/staff/employees/${encodeURIComponent(id)}/assignments/${encodeURIComponent(assignmentId)}`, token, { method: 'DELETE' })
export const getEmployeeLeaves = (token: string, id: string, params: URLSearchParams) => bearerRequest<Page<EmployeeLeave>>(`/staff/employees/${encodeURIComponent(id)}/leaves?${params}`, token)
export const getOwnLeaves = (token: string, params: URLSearchParams) => bearerRequest<Page<EmployeeLeave>>(`/staff/me/leaves?${params}`, token)
export const requestLeave = (token: string, input: LeaveInput, id?: string) => bearerRequest<EmployeeLeave>(id ? `/staff/employees/${encodeURIComponent(id)}/leaves` : '/staff/me/leaves', token, json('POST', input))
export const decideLeave = (token: string, id: string, status: 'approved' | 'rejected') => bearerRequest<EmployeeLeave>(`/staff/leaves/${encodeURIComponent(id)}/decision`, token, json('PATCH', { status }))
export const getSalaryRecords = (token: string, id: string, params: URLSearchParams) => bearerRequest<Page<SalaryRecord>>(`/staff/employees/${encodeURIComponent(id)}/salaries?${params}`, token)
export const addSalaryRecord = (token: string, id: string, input: SalaryInput) => bearerRequest<SalaryRecord>(`/staff/employees/${encodeURIComponent(id)}/salaries`, token, json('POST', input))

export type AttendanceSession = { id: string; name: string; start_date: string; end_date: string }
export type TeacherRosterItem = { employee_id: string; employee_code: string; name_en: string; name_bn: string; record_id: string | null; status: 'present' | 'absent' | 'leave' | null; note: string | null }
export type TeacherAttendanceRecord = { id: string; session_id: string; employee_id: string; attendance_date: string; status: 'present' | 'absent' | 'leave'; note: string; recorded_by: string; updated_by: string; created_at: string; updated_at: string; session_name: string; employee_code: string; employee_name_en: string; employee_name_bn: string }
export type TeacherMonthlySummary = { employee_id: string; employee_code: string; name_en: string; name_bn: string; present: number; absent: number; leave: number; total: number }
export type TeacherAttendanceInput = Pick<TeacherAttendanceRecord, 'session_id' | 'employee_id' | 'attendance_date' | 'status' | 'note'>
export const getAttendanceSessions = (token: string, params: URLSearchParams) => bearerRequest<Page<AttendanceSession>>(`/attendance/sessions?${params}`, token)
export const getTeacherRoster = (token: string, params: URLSearchParams) => bearerRequest<Page<TeacherRosterItem>>(`/attendance/teachers/roster?${params}`, token)
export const recordTeacherAttendance = (token: string, input: TeacherAttendanceInput) => bearerRequest<TeacherAttendanceRecord>('/attendance/teachers', token, json('POST', input))
export const correctTeacherAttendance = (token: string, id: string, input: Pick<TeacherAttendanceRecord, 'status' | 'note'>) => bearerRequest<TeacherAttendanceRecord>(`/attendance/teachers/${encodeURIComponent(id)}`, token, json('PATCH', input))
export const getOwnTeacherAttendance = (token: string, params: URLSearchParams) => bearerRequest<Page<TeacherAttendanceRecord>>(`/attendance/teachers/me?${params}`, token)
export const getTeacherMonthlyReport = (token: string, params: URLSearchParams) => bearerRequest<Page<TeacherMonthlySummary>>(`/attendance/teachers/monthly?${params}`, token)
export const getOwnTeacherMonthly = (token: string, params: URLSearchParams) => bearerRequest<TeacherMonthlySummary>(`/attendance/teachers/me/monthly?${params}`, token)

type HealthResponse = {
  data: {
    status: string
  }
}

export async function getHealth(signal?: AbortSignal): Promise<string> {
  const response = await fetch(`${apiBaseUrl}/healthz`, { signal })
  if (!response.ok) {
    throw new Error(`Health request failed (${response.status})`)
  }

  const body: HealthResponse = await response.json()
  if (body?.data?.status !== 'ok') {
    throw new Error('Unexpected health response')
  }
  return body.data.status
}
