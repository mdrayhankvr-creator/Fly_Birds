import { bearerBlob, bearerRequest, json, type Page } from './client'

export type RoleCount = { role: string; total: number; active: number }
export type RecentEvent = { id: string; created_at: string; actor_name: string; action: string; outcome: string }
export type BackupRun = {
  id: string; status: 'running' | 'completed' | 'failed'; trigger: 'manual' | 'scheduled'; file_name: string | null; size_bytes: number | null
  sha256: string | null; table_count: number | null; row_count: number | null; file_count: number | null; schema_version: number | null
  error: string; started_by_name: string; started_at: string; finished_at: string | null; deleted: boolean
}
export type AdminOverview = {
  schema_version: number; database_bytes: number; users: RoleCount[]; locked_users: number; active_students: number; guardians: number
  active_staff: number; modules_enabled: number; modules_total: number; fees_outstanding: number; fees_overdue: number
  exams_published: number; exams_draft: number; sms_queued: number; sms_failed: number; attachments: number; attachment_bytes: number
  audit_last_day: number; denied_last_day: number; last_backup: BackupRun | null; recent_events: RecentEvent[]
}
export type AuditItem = {
  id: string; created_at: string; actor_user_id: string | null; action: string; target_type: string; target_id: string; outcome: string
  metadata: unknown; actor_name: string; actor_email: string; actor_role: string
}
export type ActionCount = { action: string; count: number }
export type ExportDataset = { key: string; ranged: boolean; rows: number }
export type BackupPage = Page<BackupRun> & { retention: number }
export type Verification = { ok: boolean; entries: number; problem?: string }
export type MatrixRole = { id: string; name: string; active_users: number }
export type MatrixPermission = { id: string; description: string; group: string }
export type PermissionMatrix = { roles: MatrixRole[]; permissions: MatrixPermission[]; grants: Record<string, string[]>; protected: Record<string, string[]> }
export type PermissionHistory = { id: string; created_at: string; actor_name: string; added: string[]; removed: string[] }
export type ManagedModule = { key: string; name: string; is_core: boolean; enabled: boolean; depends_on: string[]; dependents: string[]; updated_at: string; changed?: string[] }

const id = encodeURIComponent
export const getAdminOverview = (token: string) => bearerRequest<AdminOverview>('/admin/overview', token)
export const getAuditEvents = (token: string, params: URLSearchParams) => bearerRequest<Page<AuditItem>>(`/audit/events?${params}`, token)
export const getAuditActions = (token: string) => bearerRequest<ActionCount[]>('/audit/actions', token)
export const getExportDatasets = (token: string, from: string, to: string) => bearerRequest<{ items: ExportDataset[]; max_days: number }>(`/admin/exports?${new URLSearchParams({ from, to })}`, token)
export const downloadExport = (token: string, key: string, from: string, to: string) => bearerBlob(`/admin/exports/${id(key)}?${new URLSearchParams({ from, to })}`, token)
export const getBackups = (token: string, page: number) => bearerRequest<BackupPage>(`/admin/backups?page=${page}&page_size=10`, token)
export const createBackup = (token: string) => bearerRequest<BackupRun>('/admin/backups', token, { method: 'POST' })
export const verifyBackup = (token: string, backup: string) => bearerRequest<Verification>(`/admin/backups/${id(backup)}/verify`, token, { method: 'POST' })
export const downloadBackup = (token: string, backup: string) => bearerBlob(`/admin/backups/${id(backup)}/download`, token)
export const deleteBackup = (token: string, backup: string) => bearerRequest<{ deleted: boolean }>(`/admin/backups/${id(backup)}`, token, { method: 'DELETE' })
export const getPermissionMatrix = (token: string) => bearerRequest<PermissionMatrix>('/permissions/matrix', token)
export const getPermissionHistory = (token: string, role: string, page: number) => bearerRequest<Page<PermissionHistory>>(`/roles/${id(role)}/permissions/history?page=${page}&page_size=10`, token)
export const saveRolePermissions = (token: string, role: string, ids: string[], expected: string[]) => bearerRequest<string[]>(`/roles/${id(role)}/permissions`, token, json('PUT', { ids, expected_ids: expected }))
export const getManagedModules = (token: string) => bearerRequest<Page<ManagedModule>>('/school/modules?page_size=100', token)
export const setManagedModule = (token: string, key: string, enabled: boolean, cascade: boolean) => bearerRequest<ManagedModule>(`/school/modules/${id(key)}`, token, json('PUT', { enabled, cascade }))

// saveBlob offers a fetched file to the browser under the given name.
export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = name; document.body.appendChild(a); a.click(); a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

export function formatBytes(size: number, locale: string) {
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size, unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++ }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: unit === 0 ? 0 : 1 }).format(value)} ${units[unit]}`
}
