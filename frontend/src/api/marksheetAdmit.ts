import { ApiError, bearerBlob, bearerRequest, json } from './client'

// Marksheet v2 and admit cards: class teachers, their comments, staff
// signatures and admit cards from published term routines.

const id = (value: string) => encodeURIComponent(value)

export type SectionTeacher = {
  section_id: string; section_name: string; class_id: string; class_name: string; session_id: string
  employee_id: string | null; teacher_name: string; teacher_name_bn: string; has_signature: boolean
}
export type CommentPreset = { id: string; text_en: string; text_bn: string; display_order: number; is_active: boolean }
export type PresetInput = { text_en: string; text_bn: string; display_order: number; is_active: boolean }
export type ResultScope = { scope: 'term' | 'annual'; scope_id: string; name: string }
export type CommentSection = {
  section_id: string; section_name: string; class_id: string; class_name: string; session_id: string; session_name: string
  teacher_name: string; scopes: ResultScope[]
}
export type StudentComment = {
  student_id: string; name_en: string; name_bn: string; admission_no: string; roll_no: number | null
  preset_id: string | null; custom_text: string; status: '' | 'written' | 'reviewed'
  written_by: string; written_at: string | null; reviewed_by: string; reviewed_at: string | null; gpa: string | null; outcome: string
}
export type CommentSheet = {
  scope: 'term' | 'annual'; scope_id: string; section_id: string; section_name: string; class_name: string
  can_write: boolean; can_review: boolean; presets: CommentPreset[]; students: StudentComment[]
}
export type CommentInput = { scope: 'term' | 'annual'; scope_id: string; student_id: string; preset_id: string | null; custom_text: string }

export const getClassTeachers = (token: string, sessionId: string) => bearerRequest<SectionTeacher[]>(`/academic/class-teachers?session_id=${id(sessionId)}`, token)
export const assignClassTeacher = (token: string, sectionId: string, employeeId: string) =>
  bearerRequest<SectionTeacher>(`/academic/sections/${id(sectionId)}/class-teacher`, token, json('PUT', { employee_id: employeeId }))
export const getPresets = (token: string, all = false) => bearerRequest<CommentPreset[]>(`/exams/comments/presets${all ? '?all=true' : ''}`, token)
export const savePreset = (token: string, input: PresetInput, presetId?: string) =>
  bearerRequest<CommentPreset>(presetId ? `/exams/comments/presets/${id(presetId)}` : '/exams/comments/presets', token, json(presetId ? 'PUT' : 'POST', input))
export const getCommentSections = (token: string) => bearerRequest<CommentSection[]>('/exams/comments/sections', token)
export const getCommentSheet = (token: string, scope: string, scopeId: string, sectionId: string) =>
  bearerRequest<CommentSheet>(`/exams/comments?${new URLSearchParams({ scope, scope_id: scopeId, section_id: sectionId })}`, token)
export const saveComment = (token: string, input: CommentInput) => bearerRequest<StudentComment>('/exams/comments', token, json('PUT', input))
export const reviewComments = (token: string, input: { scope: string; scope_id: string; section_id: string; student_ids?: string[] }) =>
  bearerRequest<{ reviewed: number }>('/exams/comments/review', token, json('POST', input))

// Signatures.
export async function getOwnSignature(token: string): Promise<Blob | null> {
  try {
    return await bearerBlob('/staff/me/signature', token)
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 404) return null
    throw cause
  }
}
export const saveOwnSignature = (token: string, file: File) =>
  bearerRequest<{ version: string }>('/staff/me/signature', token, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file })
export const deleteOwnSignature = (token: string) => bearerRequest<{ deleted: boolean }>('/staff/me/signature', token, { method: 'DELETE' })

// Admit cards.
export type AdmitSettings = { rules_en: string; rules_bn: string; withhold_for_dues: boolean; dues_cutoff: string | null; default_rules_en: string; default_rules_bn: string }
export type AdmitSection = { section_id: string; section_name: string; class_id: string; class_name: string; students: number; held: number }
export type AdmitTerm = { term_id: string; term_name: string; session_id: string; session_name: string; version_id: string; version: number; published_at: string; sections: AdmitSection[] }
export type AdmitStudent = {
  student_id: string; name_en: string; name_bn: string; admission_no: string; roll_no: number | null; class_id: string; class_name: string
  section_id: string; section_name: string; due: string; oldest_due: string | null; held: boolean; released: boolean; release_note: string
}
export type MyAdmitCard = { term_id: string; term_name: string; version_id: string; version: number; published_at: string; held: boolean }
export type AdmitVerification = {
  status: 'current' | 'superseded'; school_name: string; school_name_bn: string; term: string; session: string; class: string; section: string
  version: number; published_at: string; issued_at: string
}

export const getAdmitSettings = (token: string) => bearerRequest<AdmitSettings>('/routines/admit-cards/settings', token)
export const saveAdmitSettings = (token: string, input: Pick<AdmitSettings, 'rules_en' | 'rules_bn' | 'withhold_for_dues' | 'dues_cutoff'>) =>
  bearerRequest<AdmitSettings>('/routines/admit-cards/settings', token, json('PUT', input))
export const getAdmitTerms = (token: string) => bearerRequest<AdmitTerm[]>('/routines/admit-cards/terms', token)
export const getHeldStudents = (token: string, termId: string) => bearerRequest<AdmitStudent[]>(`/routines/admit-cards/withheld?term_id=${id(termId)}`, token)
export const releaseAdmitCard = (token: string, termId: string, studentId: string, note: string) =>
  bearerRequest<{ released: boolean }>(`/routines/admit-cards/terms/${id(termId)}/releases/${id(studentId)}`, token, json('PUT', { note }))
export const unreleaseAdmitCard = (token: string, termId: string, studentId: string) =>
  bearerRequest<{ released: boolean }>(`/routines/admit-cards/terms/${id(termId)}/releases/${id(studentId)}`, token, { method: 'DELETE' })
export const printAdmitCards = (token: string, termId: string, target: { class_id?: string; section_id?: string }, language: 'en' | 'bn') =>
  bearerBlob(`/routines/admit-cards/terms/${id(termId)}/pdf?${new URLSearchParams({ ...target, lang: language })}`, token)
export const getMyAdmitCards = (token: string) => bearerRequest<MyAdmitCard[]>('/routines/admit-cards/me', token)
export const downloadMyAdmitCard = (token: string, termId: string, language: 'en' | 'bn') => bearerBlob(`/routines/admit-cards/me/${id(termId)}/pdf?lang=${language}`, token)

// The public check behind an admit card's QR code (no sign-in).
export async function verifyAdmitCard(token: string): Promise<AdmitVerification> {
  const base = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
  const response = await fetch(`${base}/public/admit-cards/${id(token)}`, { headers: { Accept: 'application/json' } })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new ApiError(response.status, body?.error?.code || 'request_failed')
  return body.data as AdmitVerification
}
