const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')

export type AuthUser = {
  id: string
  email: string
  role: string
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
