import { bearerRequest } from './client'

export type ExistingTeacher = { id: string; employee_code: string; name_en: string; active: boolean; linked_account: boolean; linked_role: string; linked_account_active: boolean }
export type PlannedTeacher = { employee_code: string; name_en: string; name_bn: string; role: 'teacher'; login_id: string; test_marker: 'TEST' }
export type TeacherPlan = { target: number; existing: number; to_add: number; teachers: ExistingTeacher[]; candidates: PlannedTeacher[]; fingerprint: string; incomplete: string[] }
export type TeacherCredential = { name_en: string; name_bn: string; employee_code: string; role: 'teacher'; login_id: string; initial_password: string; password_expires_at: string }
export type TeacherApplyResult = { created: number; replayed: boolean; credentials: TeacherCredential[] }

export const previewTeacherSetup = (token: string) => bearerRequest<TeacherPlan>('/staff/synthetic-setup/preview', token)
export const applyTeacherSetup = (token: string, input: { fingerprint: string; idempotency_key: string; confirm: true }) => bearerRequest<TeacherApplyResult>('/staff/synthetic-setup', token, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
