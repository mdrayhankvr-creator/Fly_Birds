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
