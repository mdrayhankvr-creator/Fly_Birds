import { bearerRequest, json } from './client'

// M15 year-end promotion (stage B1 API). The server enforces every rule;
// these types mirror docs/openapi.yaml.
export type Decision = 'promote' | 'repeat' | 'graduate' | 'withdraw' | 'transfer'
export type Recommendation = Decision | 'blocked'
export type BatchStatus = 'draft' | 'approved' | 'applied' | 'reverted' | 'discarded'
export type RollPolicy = 'keep' | 'merit' | 'name' | 'manual'

export type PromotionIssue = { code: string; student_id?: string; detail?: string }
export type PromotionItem = {
  student_id: string; student_name: string; admission_no: string; enrolment_id: string
  source_section_id: string; source_section_name: string; source_roll_no: number | null
  annual_version_id: string | null; annual_outcome: string | null; annual_class_position: number | null
  recommendation: Recommendation; decision: Decision | null; reason: string; decided_by: string | null
  target_class_id: string | null; target_class_name: string | null
  target_section_id: string | null; target_section_name: string | null; target_roll_no: number | null
  applied_enrolment_id: string | null
}
export type PromotionBatch = {
  id: string; from_session_id: string; from_session_name: string; to_session_id: string; to_session_name: string
  class_id: string; class_name: string; next_class_id: string | null; next_class_name: string | null; class_is_final: boolean
  status: BatchStatus; roll_policy: RollPolicy
  approved_sha256: string | null; approved_by: string | null; approved_at: string | null
  applied_by: string | null; applied_at: string | null
  reverted_by: string | null; reverted_at: string | null; revert_reason: string
  created_by: string; created_at: string
  inputs_sha256: string; counts: Record<string, number>
  blockers: PromotionIssue[]; warnings: PromotionIssue[]; revert_blockers: PromotionIssue[]
  items: PromotionItem[]
}
export type ClassProgression = {
  class_id: string; class_name: string; display_order: number; is_active: boolean
  setting: 'next' | 'final' | 'unset'; next_class_id: string | null; next_class_name: string | null; is_final: boolean
  suggested_next_class_id: string | null; suggested_next_class_name: string | null
}
export type ConcessionReview = {
  id: string; student_id: string; session_id: string; session_name: string; category_id: string; percent: number; reason: string
  student_name: string; admission_no: string; category_name_en: string; category_name_bn: string
  to_session_id: string; to_class_name: string; to_section_name: string; to_percent: number | null
}
export type ItemInput = { decision?: Decision; reason?: string; target_section_id?: string | null; target_roll_no?: number | null }

const id = (value: string) => encodeURIComponent(value)

export const getProgression = (token: string) => bearerRequest<ClassProgression[]>('/promotions/progression', token)
export const setProgression = (token: string, classId: string, input: { next_class_id: string | null } | { final: true }) =>
  bearerRequest<ClassProgression[]>(`/promotions/progression/${id(classId)}`, token, json('PUT', input))
export const getBatches = (token: string) => bearerRequest<PromotionBatch[]>('/promotions', token)
export const getBatch = (token: string, batchId: string) => bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}`, token)
export const createBatch = (token: string, input: { from_session_id: string; to_session_id: string; class_id: string }) =>
  bearerRequest<PromotionBatch>('/promotions', token, json('POST', input))
export const updateItem = (token: string, batchId: string, studentId: string, input: ItemInput) =>
  bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/items/${id(studentId)}`, token, json('PATCH', input))
export const arrangeRolls = (token: string, batchId: string, roll_policy: RollPolicy) =>
  bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/arrange`, token, json('POST', { roll_policy }))
export const refreshBatch = (token: string, batchId: string) => bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/refresh`, token, { method: 'POST' })
export const discardBatch = (token: string, batchId: string) => bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/discard`, token, { method: 'POST' })
export const approveBatch = (token: string, batchId: string, inputs_sha256: string) =>
  bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/approve`, token, json('POST', { inputs_sha256 }))
export const applyBatch = (token: string, batchId: string) => bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/apply`, token, { method: 'POST' })
export const revertBatch = (token: string, batchId: string, reason: string) =>
  bearerRequest<PromotionBatch>(`/promotions/${id(batchId)}/revert`, token, json('POST', { reason }))
export const getConcessionReview = (token: string, from: string, to: string) =>
  bearerRequest<ConcessionReview[]>(`/fees/concessions/review?from_session_id=${id(from)}&to_session_id=${id(to)}`, token)
