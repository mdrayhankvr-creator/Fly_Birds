import { bearerBlob, bearerRequest, json } from './client'

// Term exam routine planner (E8).

const id = (value: string) => encodeURIComponent(value)

export type RoomKind = 'classroom' | 'lab' | 'hall'
export type ExamRoom = { id: string; name: string; name_bn: string; kind: RoomKind; capacity: number | null; is_active: boolean }
export type ExamRoomInput = Omit<ExamRoom, 'id'>
export type RoutineStatus = 'draft' | 'submitted' | 'approved' | 'published' | 'superseded'
export type PaperKind = 'theory' | 'practical' | 'viva' | 'oral'
export type VersionSummary = {
  id: string; term_id: string; version: number; status: RoutineStatus; reason: string | null; returned_reason: string | null
  starts_on: string; ends_on: string; created_at: string; submitted_at: string | null; approved_at: string | null; approved_by_name: string | null
  published_at: string | null; superseded_at: string | null; content_sha256: string | null; papers: number; placed: number
}
export type RoutineTerm = { term_id: string; term_name: string; session_id: string; session_name: string; starts_on: string | null; ends_on: string | null; versions: VersionSummary[] }
export type RoutineSitting = { id: string; name: string; name_bn: string; start_time: string; end_time: string; minutes: number }
export type PaperRoom = { section_id: string; section_name: string; room_id: string; room_name: string; room_name_bn: string; students: number }
export type RoutinePaper = {
  id: string; class_id: string; class_name: string; subject_id: string; subject_code: string; subject_name: string; subject_name_bn: string
  kind: PaperKind; paper_no: number; duration_minutes: number; needs_lab: boolean; optional: boolean; pinned: boolean
  exam_date: string | null; sitting_id: string | null; rooms: PaperRoom[]; students: number
}
export type RoutineDuty = { id: string; exam_date: string; sitting_id: string; room_id: string; room_name: string; room_name_bn: string; employee_id: string; employee_name: string; employee_name_bn: string }
export type RoutineIssue = {
  code: string; hard: boolean; message: string; paper_ids?: string[]; class_id?: string; section_id?: string; date?: string
  sitting_id?: string; room_id?: string; teacher_id?: string; count?: number; limit?: number; holiday?: string
}
export type RoutineChange = {
  change: 'added' | 'removed' | 'moved' | 'rooms' | 'duration'; class_name: string; subject_name: string; subject_name_bn: string; kind: PaperKind; paper_no: number
  from_date?: string; from_sitting?: string; to_date?: string; to_sitting?: string; from_rooms?: string; to_rooms?: string; from_duration?: number; to_duration?: number
}
export type RoutineVersion = VersionSummary & {
  term_name: string; session_id: string; session_name: string; term_starts_on: string | null; term_ends_on: string | null; based_on: string | null
  instructions: string; from: string; to: string; sittings: RoutineSitting[]; papers: RoutinePaper[]; duties: RoutineDuty[]
  classes: { id: string; name: string; sections: { id: string; name: string; students: number }[] }[]
  rooms: ExamRoom[]; teachers: { id: string; name: string; name_bn: string }[]; weekly_off_days: number[]
  holidays: { date: string; title_en: string; title_bn: string }[]; dates: string[]; issues: RoutineIssue[]; changes: RoutineChange[]; previous_version: number | null
}
export type GenerateReport = { placed: number; kept: number; duties: number; unplaced: { paper_id: string; class_id: string; reason: string; message: string }[] }
export type Placement = { duration_minutes: number; needs_lab: boolean; exam_date: string; sitting_id: string; pinned?: boolean; rooms: { section_id: string; room_id: string }[] }
export type StudentExamPaper = {
  paper_id: string; exam_date: string; start_time: string; end_time: string; sitting_name: string; sitting_name_bn: string
  subject_code: string; subject_name: string; subject_name_bn: string; kind: PaperKind; paper_no: number; duration_minutes: number
  optional: boolean; room_name: string; room_name_bn: string; changed: boolean
}
export type StudentRoutine = { version_id: string; term_id: string; term_name: string; version: number; published_at: string; amended_on: string | null; reason: string | null; class_name: string; section_name: string; papers: StudentExamPaper[] }
export type TeacherDuty = { version_id: string; term_name: string; exam_date: string; start_time: string; end_time: string; sitting_name: string; sitting_name_bn: string; room_name: string; room_name_bn: string; papers: string[]; papers_bn: string[]; students: number }
export type RoutineDocKind = 'class' | 'room' | 'invigilation'

export const getExamRooms = (token: string, all = false) => bearerRequest<ExamRoom[]>(`/routines/rooms${all ? '?all=true' : ''}`, token)
export const saveExamRoom = (token: string, input: ExamRoomInput, roomId?: string) => bearerRequest<ExamRoom>(roomId ? `/routines/rooms/${id(roomId)}` : '/routines/rooms', token, json(roomId ? 'PATCH' : 'POST', input))
export const getRoutineTerms = (token: string) => bearerRequest<RoutineTerm[]>('/routines/terms', token)
export const createRoutineVersion = (token: string, input: { term_id: string; starts_on?: string; ends_on?: string; duration_minutes?: number }) => bearerRequest<RoutineVersion>('/routines/versions', token, json('POST', input))
export const getRoutineVersion = (token: string, versionId: string) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}`, token)
export const updateRoutineVersion = (token: string, versionId: string, input: { starts_on: string; ends_on: string; instructions: string }) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}`, token, json('PATCH', input))
export const deleteRoutineVersion = (token: string, versionId: string) => bearerRequest<{ deleted: boolean }>(`/routines/versions/${id(versionId)}`, token, { method: 'DELETE' })
export const saveRoutineSitting = (token: string, versionId: string, input: { name: string; name_bn: string; start_time: string; end_time: string }, sittingId?: string) =>
  bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/sittings${sittingId ? `/${id(sittingId)}` : ''}`, token, json(sittingId ? 'PATCH' : 'POST', input))
export const deleteRoutineSitting = (token: string, versionId: string, sittingId: string) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/sittings/${id(sittingId)}`, token, { method: 'DELETE' })
export const addRoutinePaper = (token: string, versionId: string, input: { class_id: string; subject_id: string; kind: PaperKind; paper_no: number; duration_minutes: number; needs_lab: boolean }) =>
  bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/papers`, token, json('POST', input))
export const placeRoutinePaper = (token: string, versionId: string, paperId: string, input: Placement) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/papers/${id(paperId)}`, token, json('PATCH', input))
export const deleteRoutinePaper = (token: string, versionId: string, paperId: string) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/papers/${id(paperId)}`, token, { method: 'DELETE' })
export const addRoutineDuty = (token: string, versionId: string, input: { exam_date: string; sitting_id: string; room_id: string; employee_id: string }) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/duties`, token, json('POST', input))
export const deleteRoutineDuty = (token: string, versionId: string, dutyId: string) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/duties/${id(dutyId)}`, token, { method: 'DELETE' })
export const generateRoutine = (token: string, versionId: string) => bearerRequest<{ report: GenerateReport; version: RoutineVersion }>(`/routines/versions/${id(versionId)}/generate`, token, json('POST', {}))
export type RoutineStep = 'submit' | 'approve' | 'publish' | 'return' | 'withdraw'
export const moveRoutine = (token: string, versionId: string, step: RoutineStep, reason = '') => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/${step}`, token, json('POST', { reason }))
export const amendRoutine = (token: string, versionId: string, reason: string) => bearerRequest<RoutineVersion>(`/routines/versions/${id(versionId)}/amend`, token, json('POST', { reason }))
export const downloadRoutinePdf = (token: string, versionId: string, kind: RoutineDocKind, lang: 'en' | 'bn', filter: { class_id?: string; room_id?: string } = {}) =>
  bearerBlob(`/routines/versions/${id(versionId)}/pdf?${new URLSearchParams({ kind, lang, ...filter })}`, token)
export const getMyTermRoutines = (token: string) => bearerRequest<StudentRoutine[]>('/routines/terms/me', token)
export const downloadMyRoutinePdf = (token: string, versionId: string, lang: 'en' | 'bn') => bearerBlob(`/routines/terms/me/${id(versionId)}/pdf?lang=${lang}`, token)
export const getMyDuties = (token: string) => bearerRequest<TeacherDuty[]>('/routines/duties/me', token)
