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
  constructor(public status: number, public code: string, public details?: Record<string, unknown>) {
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
    throw new ApiError(response.status, body?.error?.code || 'request_failed', body?.error)
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

export async function bearerRequest<T>(path: string, accessToken: string, options: RequestInit = {}): Promise<T> {
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

export type AcademicSection = { id: string; session_id: string; class_id: string; name: string; code: string; display_order: number; is_active: boolean; session_name: string; class_name: string }
export type AcademicSectionInput = Omit<AcademicSection, 'id' | 'session_name' | 'class_name'>
export const getAcademicSections = (token: string, params: URLSearchParams) => bearerRequest<Page<AcademicSection>>(`/academic/sections?${params}`, token)
export const getAcademicSection = (token: string, id: string) => bearerRequest<AcademicSection>(`/academic/sections/${encodeURIComponent(id)}`, token)
export const saveAcademicSection = (token: string, input: AcademicSectionInput, id?: string) => bearerRequest<AcademicSection>(id ? `/academic/sections/${encodeURIComponent(id)}` : '/academic/sections', token, { method: id ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })

export type Gender = '' | 'male' | 'female' | 'other'
export const bloodGroups = ['A+', 'A-', 'B+', 'B-', 'AB+', 'AB-', 'O+', 'O-'] as const
export type StudentProfileDetails = { student_code: string; gender: Gender; blood_group: string; father_name_en: string; father_name_bn: string; mother_name_en: string; mother_name_bn: string; present_address: string; permanent_address: string; admission_date: string }
export type GuardianSummary = { name_en: string; name_bn: string; relationship: string; phone: string }
export type Student = { id: string; admission_no: string; name_en: string; name_bn: string; date_of_birth: string; status: 'active' | 'inactive' | 'graduated' | 'transferred'; session_id: string; class_id: string; section_id: string; roll_no: number | null; user_id: string | null; session_name: string; class_name: string; section_name: string; has_photo: boolean; primary_guardian?: GuardianSummary } & StudentProfileDetails
// Profile details are optional on input: an omitted detail keeps its stored value.
export type StudentInput = Omit<Student, 'id' | 'session_name' | 'class_name' | 'section_name' | 'has_photo' | 'primary_guardian' | keyof StudentProfileDetails> & Partial<StudentProfileDetails>
export type Guardian = { id: string; name_en: string; name_bn: string; phone: string; email: string; address: string; user_id: string | null }
export type GuardianInput = Omit<Guardian, 'id'>
export type StudentGuardian = { student_id: string; guardian_id: string; guardian_name: string; guardian_name_bn?: string; phone?: string; relationship: string; is_primary: boolean }
export type CardSchool = { name_en: string; name_bn: string; logo_url: string; address: string; phone: string; email: string; primary_color: string; secondary_color: string }
export type CardSettingsInput = { valid_until: string; signatory_title_en: string; signatory_title_bn: string; emergency_phone: string; usage_note_en: string; usage_note_bn: string; return_note_en: string; return_note_bn: string; show_qr: boolean }
export type CardSettings = CardSettingsInput & { has_signature: boolean }
export type CardData = { student: Student; valid_until: string; qr_payload: string; qr_data_url: string }
export type StudentCard = CardData & { school: CardSchool; settings: CardSettings }
export type SectionCards = { cards: CardData[]; school: CardSchool; settings: CardSettings }
export type CardVerification = { state: 'valid' | 'expired' | 'inactive'; valid_until: string; school_name_en: string; school_name_bn: string; logo_url: string }
// The page's origin goes into the card QR's verification link.
const cardOrigin = () => encodeURIComponent(window.location.origin)
export const getSectionCards = (token: string, sectionId: string) => bearerRequest<SectionCards>(`/students/cards?section_id=${encodeURIComponent(sectionId)}&origin=${cardOrigin()}`, token)
export const getCardSettings = (token: string) => bearerRequest<CardSettings>('/students/card-settings', token)
export const saveCardSettings = (token: string, input: CardSettingsInput) => bearerRequest<CardSettings>('/students/card-settings', token, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
export const saveCardSignature = (token: string, file: File) => bearerRequest<{ saved: boolean }>('/students/card-settings/signature', token, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file })
export const deleteCardSignature = (token: string) => bearerRequest<{ removed: boolean }>('/students/card-settings/signature', token, { method: 'DELETE' })
export async function getCardSignature(token: string): Promise<Blob> {
  const response = await authRequest('/students/card-settings/signature', { headers: { Authorization: `Bearer ${token}` } })
  return response.blob()
}
export const reissueStudentCard = (token: string, id: string) => bearerRequest<{ reissued: boolean }>(`/students/${encodeURIComponent(id)}/card/reissue`, token, { method: 'POST' })
export async function verifyStudentCard(cardToken: string): Promise<CardVerification> {
  const response = await authRequest(`/public/id-cards/${encodeURIComponent(cardToken)}`, {})
  return (await response.json()).data as CardVerification
}
export const getStudents = (token: string, params: URLSearchParams) => bearerRequest<Page<Student>>(`/students?${params}`, token)
export const getStudent = (token: string, id: string) => bearerRequest<Student>(`/students/${encodeURIComponent(id)}`, token)
export const getMyStudent = (token: string) => bearerRequest<Student>('/students/me', token)
export const saveStudent = (token: string, input: StudentInput, id?: string) => bearerRequest<Student>(id ? `/students/${encodeURIComponent(id)}` : '/students', token, { method: id ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
export const deleteStudent = (token: string, id: string) => bearerRequest<{ deleted: boolean }>(`/students/${encodeURIComponent(id)}`, token, { method: 'DELETE' })
export const importStudents = (token: string, params: URLSearchParams, csv: string) => bearerRequest<{ imported: number }>(`/students/import?${params}`, token, { method: 'POST', headers: { 'Content-Type': 'text/csv' }, body: csv })
export const getStudentCard = (token: string, id: string) => bearerRequest<StudentCard>(`/students/${encodeURIComponent(id)}/card?origin=${cardOrigin()}`, token)
export const saveStudentPhoto = (token: string, id: string, file: File) => bearerRequest<{ saved: boolean }>(`/students/${encodeURIComponent(id)}/photo`, token, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file })
export async function getStudentPhoto(token: string, id: string): Promise<Blob> {
  const response = await authRequest(`/students/${encodeURIComponent(id)}/photo`, { headers: { Authorization: `Bearer ${token}` } })
  return response.blob()
}
export const getGuardians = (token: string, params: URLSearchParams) => bearerRequest<Page<Guardian>>(`/guardians?${params}`, token)
export const getGuardian = (token: string, id: string) => bearerRequest<Guardian>(`/guardians/${encodeURIComponent(id)}`, token)
export const getMyGuardian = (token: string) => bearerRequest<Guardian>('/guardians/me', token)
export const saveGuardian = (token: string, input: GuardianInput, id?: string) => bearerRequest<Guardian>(id ? `/guardians/${encodeURIComponent(id)}` : '/guardians', token, { method: id ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
export const deleteGuardian = (token: string, id: string) => bearerRequest<{ deleted: boolean }>(`/guardians/${encodeURIComponent(id)}`, token, { method: 'DELETE' })
export const getMyStudentGuardians = (token: string) => bearerRequest<StudentGuardian[]>('/students/me/guardians', token)
export const getStudentGuardians = (token: string, id: string) => bearerRequest<StudentGuardian[]>(`/students/${encodeURIComponent(id)}/guardians`, token)
export const linkStudentGuardian = (token: string, id: string, guardian_id: string, relationship: string, is_primary: boolean) => bearerRequest<StudentGuardian>(`/students/${encodeURIComponent(id)}/guardians`, token, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ guardian_id, relationship, is_primary }) })
export const unlinkStudentGuardian = (token: string, id: string, guardianId: string) => bearerRequest<{ deleted: boolean }>(`/students/${encodeURIComponent(id)}/guardians/${encodeURIComponent(guardianId)}`, token, { method: 'DELETE' })
export const getMyChildren = (token: string) => bearerRequest<Student[]>('/guardians/me/children', token)
export const getGuardianStudents = (token: string, id: string) => bearerRequest<Student[]>(`/guardians/${encodeURIComponent(id)}/students`, token)

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
export const json = (method: string, body: unknown): RequestInit => ({ method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
export const changeOwnPassword = (token: string, current_password: string, new_password: string) => bearerRequest<{ sessions_revoked: number }>('/auth/password', token, json('POST', { current_password, new_password }))
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

export type ClassRoutineSlot = { id: string; session_id: string; class_id: string; section_id: string; subject_id: string; teacher_id: string; day_of_week: number; start_time: string; end_time: string; room: string; class_name: string; section_name: string; subject_name: string; subject_name_bn: string; teacher_name: string }
export type ClassRoutineInput = Pick<ClassRoutineSlot, 'session_id' | 'class_id' | 'section_id' | 'subject_id' | 'teacher_id' | 'day_of_week' | 'start_time' | 'end_time' | 'room'>
export type ExamRoutineSlot = { id: string; routine_id: string; subject_id: string; subject_name: string; subject_name_bn: string; exam_date: string; start_time: string; end_time: string; room: string }
export type ExamRoutineSlotInput = Pick<ExamRoutineSlot, 'subject_id' | 'exam_date' | 'start_time' | 'end_time' | 'room'>
export type ExamRoutine = { id: string; name: string; session_id: string; class_id: string; section_id: string; class_name: string; section_name: string; is_published: boolean; slots: ExamRoutineSlot[] }
export type ExamRoutineInput = Pick<ExamRoutine, 'name' | 'session_id' | 'class_id' | 'section_id'>
export const getClassRoutine = (token: string, path: string, params: URLSearchParams) => bearerRequest<Page<ClassRoutineSlot>>(`${path}?${params}`, token)
export const saveClassRoutineSlot = (token: string, input: ClassRoutineInput, id?: string) => bearerRequest<ClassRoutineSlot>(id ? `/routines/classes/${encodeURIComponent(id)}` : '/routines/classes', token, json(id ? 'PATCH' : 'POST', input))
export const deleteClassRoutineSlot = (token: string, id: string) => bearerRequest<{ deleted: boolean }>(`/routines/classes/${encodeURIComponent(id)}`, token, { method: 'DELETE' })
export const getExamRoutines = (token: string, path: string, params: URLSearchParams) => bearerRequest<Page<ExamRoutine>>(`${path}?${params}`, token)
export const getExamRoutine = (token: string, path: string) => bearerRequest<ExamRoutine>(path, token)
export const saveExamRoutine = (token: string, input: ExamRoutineInput, id?: string) => bearerRequest<ExamRoutine>(id ? `/routines/exams/${encodeURIComponent(id)}` : '/routines/exams', token, json(id ? 'PATCH' : 'POST', input))
export const deleteExamRoutine = (token: string, id: string) => bearerRequest<{ deleted: boolean }>(`/routines/exams/${encodeURIComponent(id)}`, token, { method: 'DELETE' })
export const publishExamRoutine = (token: string, id: string, published: boolean) => bearerRequest<ExamRoutine>(`/routines/exams/${encodeURIComponent(id)}/publication`, token, json('PATCH', { published }))
export const saveExamRoutineSlot = (token: string, id: string, input: ExamRoutineSlotInput, slotId?: string) => bearerRequest<ExamRoutine>(slotId ? `/routines/exams/${encodeURIComponent(id)}/slots/${encodeURIComponent(slotId)}` : `/routines/exams/${encodeURIComponent(id)}/slots`, token, json(slotId ? 'PATCH' : 'POST', input))
export const deleteExamRoutineSlot = (token: string, id: string, slotId: string) => bearerRequest<{ deleted: boolean }>(`/routines/exams/${encodeURIComponent(id)}/slots/${encodeURIComponent(slotId)}`, token, { method: 'DELETE' })

export type AttendanceSession = { id: string; name: string; start_date: string; end_date: string; is_active: boolean }
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
export type StudentAttendanceRecord = { id: string; student_id: string; session_id: string; class_id: string; section_id: string; attendance_date: string; status: 'present' | 'absent' | 'late'; source: 'manual' | 'qr'; note: string; name_en: string; name_bn: string; admission_no: string }
export type StudentAttendanceRosterItem = { student_id: string; admission_no: string; name_en: string; name_bn: string; roll_no: number | null; record_id: string | null; status: StudentAttendanceRecord['status'] | null; source: StudentAttendanceRecord['source'] | null; note: string | null; version: string | null }
export type StudentAttendanceSummary = { student_id: string; admission_no: string; name_en: string; name_bn: string; present: number; absent: number; late: number; total: number }
export type StudentAttendanceReport = { student: StudentAttendanceSummary; items: StudentAttendanceRecord[] }
export type AbsenceEvent = { id: string; student_id: string; attendance_date: string; state: 'pending' | 'cancelled'; name_en: string; guardian_count: number }
export const getStudentAttendanceRoster = (token: string, params: URLSearchParams) => bearerRequest<Page<StudentAttendanceRosterItem>>(`/attendance/students/roster?${params}`, token)
export const recordStudentAttendance = (token: string, input: { student_id: string; attendance_date: string; status: StudentAttendanceRecord['status']; note: string }) => bearerRequest<StudentAttendanceRecord>('/attendance/students', token, json('POST', input))
export type StudentRosterEntry = { student_id: string; status: StudentAttendanceRecord['status']; note: string; record_id: string | null; version: string }
export const saveStudentAttendanceRoster = (token: string, input: { session_id: string; section_id: string; attendance_date: string; entries: StudentRosterEntry[] }) => bearerRequest<{ created: number; updated: number; unchanged: number }>('/attendance/students/roster', token, json('PUT', input))
export const scanStudentAttendance = (token: string, input: { payload: string; attendance_date: string }) => bearerRequest<StudentAttendanceRecord>('/attendance/students/qr', token, json('POST', input))
export const correctStudentAttendance = (token: string, id: string, input: { status: StudentAttendanceRecord['status']; note: string }) => bearerRequest<StudentAttendanceRecord>(`/attendance/students/${encodeURIComponent(id)}`, token, json('PATCH', input))
export const getStudentAttendanceMonthly = (token: string, params: URLSearchParams) => bearerRequest<Page<StudentAttendanceSummary>>(`/attendance/students/monthly?${params}`, token)
export const getOwnStudentAttendanceReport = (token: string, month: string) => bearerRequest<StudentAttendanceReport>(`/attendance/students/me/report?month=${encodeURIComponent(month)}`, token)
export const getGuardianStudentAttendanceReport = (token: string, id: string, month: string) => bearerRequest<StudentAttendanceReport>(`/attendance/guardians/children/${encodeURIComponent(id)}/report?month=${encodeURIComponent(month)}`, token)
export const getAbsenceEvents = (token: string, params: URLSearchParams) => bearerRequest<Page<AbsenceEvent>>(`/attendance/alerts?${params}`, token)

export type AttendanceSection = { id: string; name: string; class_name: string; session_id: string }
export const getAttendanceSections = (token: string, sessionId: string) => bearerRequest<AttendanceSection[]>(`/attendance/students/sections?session_id=${encodeURIComponent(sessionId)}`, token)

export type ResultExam = { id: string; name: string; exam_type: 'class_test'|'half_yearly'|'annual'|'other'; session_id: string; class_id: string; section_id: string; routine_id: string|null; status: 'draft'|'published'|'locked'; session_name: string; class_name: string; section_name: string; subjects: ExamSubject[]; term_id?: string|null; term_name?: string|null; assessment_key?: string|null; blueprint_version_id?: string|null; marks_due_on?: string|null }
export type ResultExamInput = Pick<ResultExam,'name'|'exam_type'|'session_id'|'class_id'|'section_id'|'routine_id'>
export type ExamSubject = { id: string; exam_id: string; subject_id: string; name_en: string; name_bn: string; full_marks: number; pass_marks: number; is_fourth_subject: boolean }
export type ExamMarkRow = { exam_subject_id: string; student_id: string; student_name: string; admission_no: string; roll_no: number|null; subject_name: string; subject_name_bn: string; marks: number|null; grade: string|null; grade_point: number|null; passed: boolean|null; status: ExamMarkStatus|''; status_reason: string; row_version: number; components?: Record<string, number> }
export type ExamMarkStatus = 'present'|'absent'|'exempt'|'incomplete'|'withheld'
export type ExamResult = { exam_id: string; student_id: string; student_name: string; student_name_bn: string; admission_no: string; roll_no: number|null; total_marks: number; full_marks: number; gpa: number; passed: boolean; class_position: number; section_position: number }
export type ReportCard = { withheld: boolean; version_id?: string; version?: number; revised?: boolean; published_at?: string; exam: ResultExam; result: ExamResult; subjects: ExamMarkRow[]; school_name_en: string; school_name_bn: string; logo_url: string }
export const getResultExams = (token:string,path:string,params:URLSearchParams) => bearerRequest<Page<ResultExam>>(`${path}?${params}`,token)
export const getResultExam = (token:string,path:string) => bearerRequest<ResultExam>(path,token)
export const saveResultExam = (token:string,input:ResultExamInput,id?:string) => bearerRequest<ResultExam>(id?`/exams/${encodeURIComponent(id)}`:'/exams',token,json(id?'PATCH':'POST',input))
export const deleteResultExam = (token:string,id:string) => bearerRequest<{deleted:boolean}>(`/exams/${encodeURIComponent(id)}`,token,{method:'DELETE'})
export const saveResultSubject = (token:string,examId:string,input:{subject_id:string;full_marks:number;pass_marks:number}) => bearerRequest<ResultExam>(`/exams/${encodeURIComponent(examId)}/subjects`,token,json('POST',input))
export const deleteResultSubject = (token:string,examId:string,subjectId:string) => bearerRequest<{deleted:boolean}>(`/exams/${encodeURIComponent(examId)}/subjects/${encodeURIComponent(subjectId)}`,token,{method:'DELETE'})
export const getExamMarksRoster = (token:string,examId:string,teacher:boolean) => bearerRequest<ExamMarkRow[]>(`/exams/${encodeURIComponent(examId)}/${teacher?'assigned-roster':'roster'}`,token)
export const saveExamMarks = (token:string,examId:string,entries:{exam_subject_id:string;student_id:string;marks:number}[]) => bearerRequest<{saved:number}>(`/exams/${encodeURIComponent(examId)}/marks`,token,json('PUT',{entries}))
export type ExamSheetStatus = 'not_started'|'in_progress'|'submitted'|'returned'|'approved'
export type ExamSheet = { id: string; subject_id: string; name_en: string; name_bn: string; full_marks: number; pass_marks: number; is_fourth_subject: boolean; status: ExamSheetStatus; students: number; entered: number; missing: number; absent: number; exempt: number; incomplete: number; withheld: number; submitted_by: string; submitted_at: string|null; reviewed_by: string; reviewed_at: string|null; return_reason: string }
export type ExamBlocker = { code: 'no_subjects'|'no_students'|'marks_missing'|'marks_incomplete'|'awaiting_review'|'returned'; sheet_id?: string; subject?: string; count?: number }
export type ExamSheets = { exam_id: string; exam_status: ResultExam['status']; sheets: ExamSheet[]; readiness: { ready: boolean; blockers: ExamBlocker[] } }
export type ExamMarkCell = { exam_subject_id: string; student_id: string; status: ExamMarkStatus; marks: number|null; reason: string; row_version: number; components?: Record<string, number> }
export type ExamCellError = { exam_subject_id: string; student_id: string; code: string; message: string; current?: ExamMarkCell }
export const getExamSheets = (token:string,examId:string,teacher:boolean) => bearerRequest<ExamSheets>(teacher?`/exams/assigned/${encodeURIComponent(examId)}/sheets`:`/exams/${encodeURIComponent(examId)}/sheets`,token)
export const saveExamMarkCells = (token:string,examId:string,entries:ExamMarkCell[]) => bearerRequest<{saved:ExamMarkCell[];errors:ExamCellError[]}>(`/exams/${encodeURIComponent(examId)}/marks`,token,json('PATCH',{entries}))
export const examSheetAction = (token:string,examId:string,sheetId:string,action:'submit'|'approve'|'return',reason?:string) => bearerRequest<ExamSheet>(`/exams/${encodeURIComponent(examId)}/sheets/${encodeURIComponent(sheetId)}/${action}`,token,json('POST',action==='return'?{reason}:{}))
export type ExamSheetQueueItem = { exam_id: string; exam_name: string; exam_type: ResultExam['exam_type']; exam_status: ResultExam['status']; session_name: string; class_name: string; section_name: string; sheet_id: string; subject_id: string; subject_name_en: string; subject_name_bn: string; full_marks: number; status: ExamSheetStatus; students: number; entered: number; missing: number; return_reason: string; updated_at: string|null; due_on?: string|null }
export type ExamSheetPermissions = { can_edit: boolean; can_submit: boolean; can_review: boolean; statuses: ExamMarkStatus[]; read_only_reason?: 'exam_published'|'exam_locked'|'sheet_submitted'|'sheet_approved'|'review_only' }
export type ExamSheetComponent = { key: string; full: number; pass: number|null }
export type ExamSheetWorkspace = { exam: ResultExam; sheet: ExamSheet; components?: ExamSheetComponent[]; rows: ExamMarkRow[]; permissions: ExamSheetPermissions }
export type ExamSheetHistoryEntry = { at: string; action: 'exam.mark_saved'|'exam.sheet_submitted'|'exam.sheet_returned'|'exam.sheet_approved'; actor: string; student_name: string; admission_no: string; metadata: { old_marks?: number|string|null; new_marks?: number|string|null; old_status?: string|null; new_status?: string|null; reason?: string; from?: string } }
export const getMyExamSheets = (token:string) => bearerRequest<ExamSheetQueueItem[]>('/exams/my-sheets',token)
const sheetPath = (examId:string,sheetId:string,teacher:boolean) => `/exams/${teacher?'assigned/':''}${encodeURIComponent(examId)}/sheets/${encodeURIComponent(sheetId)}`
export const getExamSheetWorkspace = (token:string,examId:string,sheetId:string,teacher:boolean) => bearerRequest<ExamSheetWorkspace>(sheetPath(examId,sheetId,teacher),token)
export const getExamSheetHistory = (token:string,examId:string,sheetId:string,teacher:boolean) => bearerRequest<ExamSheetHistoryEntry[]>(`${sheetPath(examId,sheetId,teacher)}/history`,token)
export const setResultExamState = (token:string,examId:string,status:ResultExam['status']) => bearerRequest<ResultExam>(`/exams/${encodeURIComponent(examId)}/state`,token,json('PATCH',{status}))
export const getExamTabulation = (token:string,examId:string) => bearerRequest<ExamResult[]>(`/exams/${encodeURIComponent(examId)}/tabulation`,token)
export const getReportCards = (token:string,path:string) => bearerRequest<ReportCard[]>(path,token)

// E5: exam terms, assessment blueprints, the setup wizard, optional subject
// choices and the provisional term results.
export type ExamTermKind = 'term'|'annual'
export type ExamTerm = { id: string; session_id: string; name: string; sequence: number; kind: ExamTermKind; starts_on: string|null; ends_on: string|null; status: 'planning'|'running'|'results'|'closed'; legacy: boolean; exams: number; setup: TermSetup }
export type ExamTermInput = { session_id: string; name: string; kind: ExamTermKind; starts_on: string; ends_on: string }
export type AssessmentKind = 'class_test'|'continuous'|'performance'|'term_exam'
export type StudentVisibility = 'hidden'|'after_term_publication'|'with_assessment_publication'
export type BlueprintAssessment = { key: string; name: string; term_id: string; kind: AssessmentKind; visibility: StudentVisibility }
export type BlueprintComponent = { key: string; full: number; pass?: number|null }
export type BlueprintSheet = { assessment: string; pass: number; components: BlueprintComponent[] }
export type CombineMethod = 'single'|'sum'|'average'|'best'|'latest'
export type BlueprintContribution = { from: string[]; combine: CombineMethod; best?: number; worth: number }
export type BlueprintTermRule = { term_id: string; full: number; pass: number; contributions: BlueprintContribution[] }
export type SubjectRole = 'required'|'optional'|'non_graded'
export type BlueprintSubject = { subject_id: string; role: SubjectRole; combined_key?: string; sheets: BlueprintSheet[]; terms: BlueprintTermRule[] }
export type BlueprintPolicy = { round_contributions: boolean; whole_marks: boolean; gpa_rounding: 'half_up'|'truncate'; absent: 'zero'|'missing'; ranking: 'competition'|'dense' }
export type BlueprintAnnual = { method: 'final_only'|'equal'|'weighted'; weights?: Record<string, number>; final_must_pass: boolean }
export type BlueprintDefinition = { assessments: BlueprintAssessment[]; subjects: BlueprintSubject[]; paper_rule: 'combined'|'each_paper'; annual: BlueprintAnnual; policy: BlueprintPolicy }
export type BlueprintIssue = { code: string; path: string; params?: Record<string, string>; message: string }
export type BlueprintVersionSummary = { id: string; version: number; status: 'draft'|'active'|'retired'; legacy: boolean; activated_at: string|null; created_at: string; exams: number }
export type ClassBlueprint = { class_id: string; class_name: string; blueprint_id: string|null; name: string; active: BlueprintVersionSummary|null; draft: BlueprintVersionSummary|null; versions: BlueprintVersionSummary[] }
export type BlueprintTermRef = { id: string; name: string; sequence: number; kind: ExamTermKind; status: ExamTerm['status']; legacy: boolean }
export type BlueprintSubjectRef = { id: string; code: string; name_en: string; name_bn: string; fourth: boolean }
export type BlueprintBand = { grade: string; min: number; max: number; gpa: number }
export type BlueprintVersion = { id: string; blueprint_id: string; blueprint_name: string; session_id: string; session_name: string; class_id: string; class_name: string; version: number; status: BlueprintVersionSummary['status']; legacy: boolean; activated_at: string|null; sha256: string; exams: number; definition: BlueprintDefinition; terms: BlueprintTermRef[]; subjects: BlueprintSubjectRef[]; bands: BlueprintBand[]; issues: BlueprintIssue[]; versions: BlueprintVersionSummary[] }
export type SampleMark = { status?: ExamMarkStatus; marks?: number|null; components?: Record<string, number> }
export type TermReason = { code: 'component_below_pass'|'paper_below_pass'; subject_id: string; assessment?: string; component?: string }
export type TermContribution = { from: string[]; combine: CombineMethod; worth: string; value: string; exempt: boolean }
export type TermSubjectResult = { subject_ids: string[]; combined_key?: string; optional: boolean; status: 'graded'|'exempt'|'incomplete'|'withheld'; marks: string; full: string; grade: string; point: string; passed: boolean; reasons: TermReason[]; contributions: TermContribution[] }
export type TermStudentResult = { student_id: string; section_id: string; outcome: 'passed'|'failed'|'incomplete'|'withheld'; gpa: string; total: string; full: string; section_position: number; class_position: number; subjects: TermSubjectResult[] }
export type TermSetup = { class_ids: string[]; due: Record<string, string>; assignees: Record<string, string> }
export type SetupClass = { class_id: string; class_name: string; selected: boolean; blueprint_id: string|null; version_id: string|null; version: number; has_draft: boolean; state: 'ready'|'needs_blueprint'|'no_assessments'; sections: number; assessments: string[]; sheets: number; existing: number; students: number }
export type SetupPlan = { term: ExamTerm; setup: TermSetup; classes: SetupClass[]; assessments: { key: string; name: string; kind: AssessmentKind; classes: number; due: string }[]; gaps: { class_id: string; class_name: string; subject_id: string; subject_name: string; sheets: number; assignee: string|null }[]; teachers: { employee_id: string; name: string }[]; conflicts: { class_name: string; section_name: string; exam_name: string }[]; sheets: number; to_create: number; exams: number; new_exams: number }
export type SetupResult = { plan: SetupPlan; exams_created: number; sheets_created: number; assigned: number }
export type SubjectChoices = { optional: BlueprintSubjectRef[]; students: { student_id: string; name: string; admission_no: string; roll_no: number|null; section_name: string; subject_id: string|null }[]; in_use: boolean; counts: Record<string, number> }
export type TermPreview = { term: ExamTerm; class_id: string; class_name: string; state: 'ready'|'pending'|'no_exams'|'mixed_versions'; version_id: string; version: number; definition: BlueprintDefinition|null; subjects: BlueprintSubjectRef[]; students: { student_id: string; name: string; admission_no: string; roll_no: number|null; section_id: string; section_name: string; result: TermStudentResult }[]; sheets: number; approved: number; pending: { exam_id: string; sheet_id: string; section_name: string; assessment: string; subject_name: string; status: ExamSheetStatus; missing: number }[]; outcomes: Record<string, number>; provisional: boolean }
const enc = encodeURIComponent
export const getExamTerms = (token:string,sessionId:string) => bearerRequest<ExamTerm[]>(`/exams/terms?session_id=${enc(sessionId)}`,token)
export const saveExamTerm = (token:string,input:ExamTermInput,id?:string) => bearerRequest<ExamTerm>(id?`/exams/terms/${enc(id)}`:'/exams/terms',token,json(id?'PUT':'POST',input))
export const deleteExamTerm = (token:string,id:string) => bearerRequest<{deleted:boolean}>(`/exams/terms/${enc(id)}`,token,{method:'DELETE'})
export const getClassBlueprints = (token:string,sessionId:string) => bearerRequest<ClassBlueprint[]>(`/exams/blueprints?session_id=${enc(sessionId)}`,token)
export const createBlueprint = (token:string,sessionId:string,classId:string,copyFrom?:string) => bearerRequest<BlueprintVersion>('/exams/blueprints',token,json('POST',{session_id:sessionId,class_id:classId,...(copyFrom?{copy_from:copyFrom}:{})}))
export const newBlueprintVersion = (token:string,blueprintId:string,fromVersionId:string) => bearerRequest<BlueprintVersion>(`/exams/blueprints/${enc(blueprintId)}/versions`,token,json('POST',{from_version_id:fromVersionId}))
export const getBlueprintVersion = (token:string,id:string) => bearerRequest<BlueprintVersion>(`/exams/blueprint-versions/${enc(id)}`,token)
export const saveBlueprintDraft = (token:string,id:string,definition:BlueprintDefinition) => bearerRequest<BlueprintVersion>(`/exams/blueprint-versions/${enc(id)}`,token,json('PUT',{definition}))
export const deleteBlueprintDraft = (token:string,id:string) => bearerRequest<{deleted:boolean}>(`/exams/blueprint-versions/${enc(id)}`,token,{method:'DELETE'})
export const activateBlueprint = (token:string,id:string) => bearerRequest<BlueprintVersion>(`/exams/blueprint-versions/${enc(id)}/activate`,token,json('POST',{}))
export const setAssessmentVisibility = (token:string,id:string,key:string,visibility:StudentVisibility) => bearerRequest<BlueprintVersion>(`/exams/blueprint-versions/${enc(id)}/visibility`,token,json('PUT',{key,visibility}))
export const sampleBlueprintSubject = (token:string,id:string,input:{subject_id:string;term_id:string;marks:Record<string,SampleMark>;definition?:BlueprintDefinition}) => bearerRequest<TermSubjectResult>(`/exams/blueprint-versions/${enc(id)}/sample`,token,json('POST',input))
export const getTermSetup = (token:string,termId:string) => bearerRequest<SetupPlan>(`/exams/terms/${enc(termId)}/setup`,token)
export const saveTermSetup = (token:string,termId:string,setup:TermSetup) => bearerRequest<SetupPlan>(`/exams/terms/${enc(termId)}/setup`,token,json('PUT',setup))
export const createTermExams = (token:string,termId:string) => bearerRequest<SetupResult>(`/exams/terms/${enc(termId)}/create`,token,json('POST',{}))
export const getSubjectChoices = (token:string,sessionId:string,classId:string) => bearerRequest<SubjectChoices>(`/exams/subject-choices?session_id=${enc(sessionId)}&class_id=${enc(classId)}`,token)
export const saveSubjectChoices = (token:string,sessionId:string,classId:string,choices:{student_id:string;subject_id:string}[]) => bearerRequest<SubjectChoices>('/exams/subject-choices',token,json('PUT',{session_id:sessionId,class_id:classId,choices}))
export const getTermPreview = (token:string,termId:string,classId:string) => bearerRequest<TermPreview>(`/exams/terms/${enc(termId)}/results?class_id=${enc(classId)}`,token)
export const getReportCard = (token:string,path:string) => bearerRequest<ReportCard>(path,token)

// E6: result versions, review, approval, publication and corrections.
export type ResultScope = 'assessment'|'term'|'annual'
export type ResultReason = { code: string; subject_id: string; assessment?: string; component?: string }
export type ResultLine = { key: string; sheet_id?: string; subject_ids: string[]; code: string; name_en: string; name_bn: string; optional: boolean; combined: string; status: string; marks: string|null; full: string; pass: string; grade: string|null; point: string|null; passed: boolean|null; reasons: ResultReason[]; components: { key: string; marks: string|null; full: string; pass: string|null }[]; contributions?: TermContribution[]; terms?: { term_id: string; status: string; marks: string; full: string; weight: number }[] }
export type ResultOutputs = { outcome: 'passed'|'failed'|'withheld'|'incomplete'; total: string|null; full: string|null; gpa: string|null; passed: boolean; class_position: number|null; section_position: number|null; subjects: ResultLine[] }
export type ResultVersionBrief = { id: string; version: number; status: 'published'|'superseded'|'withdrawn'; outcome: string; gpa: string|null; total: string|null; full: string|null; class_position: number|null; section_position: number|null; published_at: string; reason: string }
export type ResultPublication = { id: string; revision: number|null; kind: 'publication'|'correction'|'position_update'; status: 'approved'|'published'|'discarded'; source: 'engine'|'migration'; reason: string; students: number; changed: number; inputs_sha256: string; rules_sha256: string; approved_by: string|null; approved_at: string|null; published_by: string|null; published_at: string|null; stale: boolean }
export type ResultBlocker = { code: string; detail?: string; count?: number; exam_id?: string; sheet_id?: string }
export type ReviewStudent = { student_id: string; name: string; name_bn: string; admission_no: string; roll_no: number|null; section_name: string; blocker: string; outputs: ResultOutputs; current: ResultVersionBrief|null; previous?: ResultOutputs; change: 'new'|'changed'|'same'; fields: string[] }
export type CorrectionCell = { exam_subject_id: string; subject_id: string; subject_name: string; subject_name_bn: string; full_marks: number; status: ExamMarkStatus|''; marks: number|null; reason: string; row_version: number; components: { key: string; full: number; marks: number|null }[]; published_status: string; published_marks: string|null; published_grade: string|null }
export type ResultCorrection = { id: string; exam_id: string; student_id: string; student_name: string; admission_no: string; roll_no: number|null; reason: string; status: 'open'|'applied'|'cancelled'; opened_by: string; opened_at: string; closed_at: string|null; publication_id: string|null; cells: CorrectionCell[] }
export type ResultRules = { bands?: BlueprintBand[]; bands_source?: string; annual?: { method: string; final_must_pass: boolean; terms: { id: string; name: string; weight: number }[] }; version?: number; term_name?: string } & Record<string, unknown>
export type ResultReview = { scope: ResultScope; scope_id: string; class_id: string; class_name: string; session_id: string; title: string; locked: boolean; state: 'blocked'|'ready'|'approved'|'changes'|'published'; blockers: ResultBlocker[]; inputs_sha256: string; rules: ResultRules; subjects: { key: string; code: string; name_en: string; name_bn: string }[]; students: ReviewStudent[]; published: ResultPublication|null; approval: ResultPublication|null; changed: number; outcomes: Record<string, number>; corrections: ResultCorrection[]; other_position_changes: number; can_approve: boolean; provisional: boolean }
export type ResultOverviewCell = { class_id: string; class_name: string; exams: number; sheets: number; approved: number; state: 'no_exams'|'sheets_pending'|'ready'|'approved'|'published'|'terms_pending'|'no_blueprint'; published: ResultPublication|null; approval: ResultPublication|null; corrections: number; missing?: string[] }
export type ResultOverview = { session_id: string; session_name: string; terms: { id: string; name: string; sequence: number; status: ExamTerm['status']; classes: ResultOverviewCell[] }[]; annual: ResultOverviewCell[] }
export type ResultVersionView = { id: string; publication_id: string; scope: ResultScope; scope_id: string; student_id: string; version: number; status: ResultVersionBrief['status']; kind: ResultPublication['kind']; revision: number|null; source: 'engine'|'migration'; outcome: string; gpa: string|null; total: string|null; full: string|null; class_position: number|null; section_position: number|null; reason: string; published_at: string; published_by: string; closed_at: string|null; closed_by: string; close_reason: string; inputs_sha256: string; supersedes_id: string|null }
export type ResultVersionDetail = { version: ResultVersionView; snapshot: { source: string; student: { name_en: string; name_bn: string; admission_no: string; section_name: string; class_name: string; session_name: string }; context: Record<string, unknown>; inputs: unknown; outputs: ResultOutputs }; rules: ResultRules; rules_sha256: string; verification: { hash_matches: boolean; reproduced: boolean; differences: string[]; note?: string } }
export type PublishedResultLine = { key: string; name_en: string; name_bn: string; optional: boolean; status: string; marks: string|null; full: string; grade: string|null; point: string|null; passed: boolean|null; reasons: string[]; terms?: { term_id: string; term_name: string; status: string; marks: string; full: string; weight: number }[] }
export type PublishedResult = { id: string; scope: 'term'|'annual'; version: number; revised: boolean; published_at: string; session_name: string; class_name: string; section_name: string; term_id: string|null; term_name: string; term_sequence: number; method: string; method_terms: { id: string; name: string; weight: number }[]; withheld: boolean; outcome: string; gpa: string|null; total: string|null; full: string|null; class_position: number|null; section_position: number|null; student: { name_en: string; name_bn: string; admission_no: string; roll_no: number|null }; subjects: PublishedResultLine[] }
export type ResultScopeRef = { scope: ResultScope; scope_id: string; class_id?: string }
const scopeQuery = (ref:ResultScopeRef) => `scope=${enc(ref.scope)}&scope_id=${enc(ref.scope_id)}${ref.class_id?`&class_id=${enc(ref.class_id)}`:''}`
export const getResultOverview = (token:string,sessionId:string) => bearerRequest<ResultOverview>(`/exams/results/overview?session_id=${enc(sessionId)}`,token)
export const getResultReview = (token:string,ref:ResultScopeRef) => bearerRequest<ResultReview>(`/exams/results/review?${scopeQuery(ref)}`,token)
export const approveResults = (token:string,ref:ResultScopeRef,inputsSha256:string,reason:string) => bearerRequest<ResultReview>('/exams/results/approve',token,json('POST',{...ref,inputs_sha256:inputsSha256,reason}))
export const publishResults = (token:string,ref:ResultScopeRef,publicationId:string) => bearerRequest<ResultReview>('/exams/results/publish',token,json('POST',{...ref,publication_id:publicationId}))
export const getResultHistory = (token:string,scope:ResultScope,scopeId:string,studentId:string) => bearerRequest<ResultVersionView[]>(`/exams/results/history?scope=${enc(scope)}&scope_id=${enc(scopeId)}&student_id=${enc(studentId)}`,token)
export const getResultVersion = (token:string,id:string) => bearerRequest<ResultVersionDetail>(`/exams/results/versions/${enc(id)}`,token)
export const getResultCorrections = (token:string,examId:string) => bearerRequest<ResultCorrection[]>(`/exams/${enc(examId)}/corrections`,token)
export const openResultCorrection = (token:string,examId:string,studentId:string,reason:string) => bearerRequest<ResultCorrection>(`/exams/${enc(examId)}/corrections`,token,json('POST',{student_id:studentId,reason}))
export const cancelResultCorrection = (token:string,examId:string,correctionId:string) => bearerRequest<{cancelled:boolean}>(`/exams/${enc(examId)}/corrections/${enc(correctionId)}/cancel`,token,json('POST',{}))
export const withdrawResultExam = (token:string,examId:string,reason:string) => bearerRequest<ResultExam>(`/exams/${enc(examId)}/state`,token,json('PATCH',{status:'draft',reason}))
export const getPublishedResults = (token:string,childId?:string) => bearerRequest<PublishedResult[]>(childId?`/exams/children/${enc(childId)}/term-results`:'/exams/me/term-results',token)

// Attachments require the bearer token, so they are fetched as blobs rather
// than linked directly.
export async function bearerBlob(path: string, accessToken: string): Promise<Blob> {
  const response = await authRequest(path, { headers: { Authorization: `Bearer ${accessToken}` } })
  return response.blob()
}

export async function bearerUpload<T>(path: string, accessToken: string, file: File): Promise<T> {
  const form = new FormData()
  form.append('file', file)
  const response = await authRequest(path, { method: 'POST', body: form, headers: { Authorization: `Bearer ${accessToken}` } })
  const body = await response.json()
  return body.data as T
}
