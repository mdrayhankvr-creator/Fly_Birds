import { bearerBlob, bearerRequest } from './client'
import type { StudentLogin } from './studentLogins'

// Bulk admission from a spreadsheet (checkpoint 5).
export type BulkGuardian = { name: string; relationship: string; action: 'new' | 'existing' | 'same_as_row'; existing_name?: string; same_as_line?: number }
export type BulkRow = {
  line: number; admission_no: string; name_en: string; name_bn: string; class_name: string; section_name: string; roll_no: number | null
  status: 'ready' | 'skip' | 'error'; errors: string[]; guardian?: BulkGuardian; login_email?: string
}
export type BulkPreview = { rows: BulkRow[]; ready: number; skipped: number; errors: number; missing_columns: string[]; unknown_columns: string[]; file_sha256: string }
export type BulkResult = { admitted: number; skipped: number; logins: StudentLogin[]; replayed: boolean }

const XLSX = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
const kind = (file: File) => /\.xlsx$/i.test(file.name) || file.type === XLSX ? XLSX : 'text/csv'
const upload = (file: File): RequestInit => ({ method: 'POST', headers: { 'Content-Type': kind(file) }, body: file })

export const getBulkTemplate = (token: string, format: 'xlsx' | 'csv') => bearerBlob(`/students/bulk-admission/template?format=${format}`, token)
export const previewBulk = (token: string, sessionId: string, file: File) =>
  bearerRequest<BulkPreview>(`/students/bulk-admission/preview?session_id=${encodeURIComponent(sessionId)}`, token, upload(file))
export const admitBulk = (token: string, sessionId: string, key: string, createLogins: boolean, file: File) =>
  bearerRequest<BulkResult>(`/students/bulk-admission?${new URLSearchParams({ session_id: sessionId, idempotency_key: key, create_logins: String(createLogins) })}`, token, upload(file))
