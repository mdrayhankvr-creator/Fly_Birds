import { bearerRequest } from './client'
import type { StudentLogin } from './studentLogins'

export type SetupSection = { class_id: string; class: string; section_id: string; section: string; target: number; existing: number; to_add: number }
export type PlannedStudent = { session_id: string; session: string; class_id: string; class: string; section_id: string; section: string; admission_no: string; label: string; test_marker: 'TEST'; name_en: string; roll_no: number; login_id: string }
export type StudentSetupPlan = { session_id: string; session: string; existing: number; to_add: number; sections: SetupSection[]; students: PlannedStudent[]; fingerprint: string; incomplete: string[] }
export type StudentSetupResult = { admitted: number; replayed: boolean; logins: StudentLogin[] }

export const previewStudentSetup = (token: string, sessionId: string) => bearerRequest<StudentSetupPlan>(`/students/synthetic-setup/preview?session_id=${encodeURIComponent(sessionId)}`, token)
export const applyStudentSetup = (token: string, input: { session_id: string; fingerprint: string; idempotency_key: string; confirm: true }) => bearerRequest<StudentSetupResult>('/students/synthetic-setup', token, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
