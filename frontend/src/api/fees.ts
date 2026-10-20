import { bearerRequest, json, type Page } from './client'

export type FeeCategory = { id: string; code: string; name_en: string; name_bn: string; frequency: 'monthly' | 'one_time'; is_active: boolean }
export type FeeCategoryInput = Omit<FeeCategory, 'id'>
export type FeeStructure = { id: string; session_id: string; class_id: string; category_id: string; amount: number; due_day: number; is_active: boolean; session_name: string; class_name: string; category_code: string; category_name_en: string; category_name_bn: string; frequency: FeeCategory['frequency'] }
export type FeeStructureInput = { session_id: string; class_id: string; category_id: string; amount: number; due_day: number; is_active: boolean }
export type FeeConcession = { id: string; student_id: string; category_id: string; percent: number; reason: string; student_name: string; admission_no: string; category_name_en: string; category_name_bn: string }
export type FeeDueStatus = 'unpaid' | 'partial' | 'paid' | 'waived'
export type FeeDue = { id: string; student_id: string; structure_id: string; session_id: string; class_id: string; period: string | null; due_date: string; amount: number; discount_amount: number; paid_amount: number; balance: number; status: FeeDueStatus; overdue: boolean; category_id: string; category_code: string; category_name_en: string; category_name_bn: string; student_name: string; student_name_bn: string; admission_no: string; session_name: string; class_name: string }
export type FeeAdjustment = { id: string; student_fee_id: string; kind: 'discount' | 'waiver'; amount: number; reason: string; created_at: string }
export type FeeMethod = 'cash' | 'bank' | 'bkash' | 'nagad'
export type FeePayment = { id: string; receipt_no: string; student_id: string; amount: number; method: FeeMethod; source: 'counter' | 'online'; simulated: boolean; voided_at: string | null; void_reason: string; reference: string; paid_on: string; note: string; received_by_name: string; created_at: string; student_name: string; student_name_bn: string; admission_no: string; class_name: string; section_name: string }
export type FeePaymentInput = { student_id: string; amount: number; method: FeeMethod; reference: string; paid_on: string; note: string; idempotency_key: string; fee_ids: string[] }
export type FeeAllocation = { student_fee_id: string; amount: number; category_name_en: string; category_name_bn: string; period: string | null; due_date: string }
export type FeeReceipt = { payment: FeePayment; allocations: FeeAllocation[]; balance_after: number; school_name_en: string; school_name_bn: string; logo_url: string; address: string; phone: string; replayed: boolean }
export type FeeStudent = { id: string; admission_no: string; name_en: string; name_bn: string; roll_no: number | null; status: string; session_id: string; class_id: string; section_id: string; session_name: string; class_name: string; section_name: string }
export type FeeTotals = { charged: number; discount: number; paid: number; balance: number; overdue: number; simulated_unreviewed: number }
export type FeeStatement = { student: FeeStudent; dues: FeeDue[]; payments: FeePayment[]; totals: FeeTotals }
export type FeeOptions = { sessions: { id: string; name: string; is_active: boolean }[]; classes: { id: string; name: string; is_active: boolean }[]; sections: { id: string; name: string; session_id: string; class_id: string }[]; categories: FeeCategory[] }
export type FeeIntent = { id: string; provider: 'bkash' | 'nagad'; provider_ref: string; student_id: string; amount: number; status: 'pending' | 'completed' | 'failed' | 'simulated'; expires_at: string; expired: boolean; payment_id: string | null }
export type FeeBucket = { key: string; name_en: string; name_bn: string; total: number; count: number }
export type FeeCollectionReport = { from: string; to: string; total: number; count: number; simulated_total: number; simulated_count: number; by_method: FeeBucket[]; by_class: FeeBucket[]; by_category: FeeBucket[]; by_day: FeeBucket[] }
export type FeeDueReportRow = { student_id: string; admission_no: string; name_en: string; name_bn: string; class_name: string; section_name: string; charged: number; discount: number; paid: number; balance: number; overdue: number; oldest_due: string | null }
export type FeeDueReport = Page<FeeDueReportRow> & { total_balance: number; total_overdue: number }
export type LedgerEntry = { id: string; entry_type: 'income' | 'expense'; category: string; amount: number; entry_date: string; description: string; reference: string; payment_id: string | null; created_by_name: string; simulated: boolean }
export type LedgerEntryInput = Omit<LedgerEntry, 'id' | 'payment_id' | 'created_by_name' | 'simulated'>
export type AccountingSummary = { from: string; to: string; income: number; expense: number; net: number; income_by_category: FeeBucket[]; expense_by_category: FeeBucket[]; outstanding: number; arrears: number }

export const ledgerCategories = {
  income: ['donation', 'grant', 'other_income'],
  expense: ['salary', 'utilities', 'supplies', 'maintenance', 'transport', 'events', 'other_expense'],
} as const

const id = (value: string) => encodeURIComponent(value)

export const getFeeOptions = (token: string) => bearerRequest<FeeOptions>('/fees/options', token)
export const searchFeeStudents = (token: string, params: URLSearchParams) => bearerRequest<Page<FeeStudent>>(`/fees/students?${params}`, token)
export const getFeeCategories = (token: string, params: URLSearchParams) => bearerRequest<Page<FeeCategory>>(`/fees/categories?${params}`, token)
export const saveFeeCategory = (token: string, input: FeeCategoryInput, categoryId?: string) => bearerRequest<FeeCategory>(categoryId ? `/fees/categories/${id(categoryId)}` : '/fees/categories', token, json(categoryId ? 'PATCH' : 'POST', input))
export const getFeeStructures = (token: string, params: URLSearchParams) => bearerRequest<Page<FeeStructure>>(`/fees/structures?${params}`, token)
export const saveFeeStructure = (token: string, input: FeeStructureInput, structureId?: string) => bearerRequest<FeeStructure>(structureId ? `/fees/structures/${id(structureId)}` : '/fees/structures', token, json(structureId ? 'PATCH' : 'POST', input))
export const getFeeConcessions = (token: string, params: URLSearchParams) => bearerRequest<Page<FeeConcession>>(`/fees/concessions?${params}`, token)
export const saveFeeConcession = (token: string, input: { student_id: string; category_id: string; percent: number; reason: string }) => bearerRequest<FeeConcession>('/fees/concessions', token, json('POST', input))
export const deleteFeeConcession = (token: string, concessionId: string) => bearerRequest<{ deleted: boolean }>(`/fees/concessions/${id(concessionId)}`, token, { method: 'DELETE' })
export const getFeeDues = (token: string, params: URLSearchParams) => bearerRequest<Page<FeeDue>>(`/fees/dues?${params}`, token)
export const assignFeeDue = (token: string, input: { student_id: string; structure_id: string; period: string }) => bearerRequest<FeeDue>('/fees/dues', token, json('POST', input))
export const generateFeeDues = (token: string, input: { session_id: string; class_id: string; period: string; include_one_time: boolean }) => bearerRequest<{ created: number; skipped: number }>('/fees/dues/generate', token, json('POST', input))
export const deleteFeeDue = (token: string, dueId: string) => bearerRequest<{ deleted: boolean }>(`/fees/dues/${id(dueId)}`, token, { method: 'DELETE' })
export const getFeeAdjustments = (token: string, dueId: string) => bearerRequest<FeeAdjustment[]>(`/fees/dues/${id(dueId)}/adjustments`, token)
export const adjustFeeDue = (token: string, dueId: string, input: { kind: FeeAdjustment['kind']; amount: number; reason: string }) => bearerRequest<FeeDue>(`/fees/dues/${id(dueId)}/adjustments`, token, json('POST', input))
export const getStudentFeeStatement = (token: string, studentId: string) => bearerRequest<FeeStatement>(`/fees/students/${id(studentId)}/statement`, token)
export const getFeePayments = (token: string, params: URLSearchParams) => bearerRequest<Page<FeePayment>>(`/fees/payments?${params}`, token)
export const recordFeePayment = (token: string, input: FeePaymentInput) => bearerRequest<FeeReceipt>('/fees/payments', token, json('POST', input))
export const getFeeReceipt = (token: string, path: string) => bearerRequest<FeeReceipt>(path, token)
export const getOwnFeeStatement = (token: string) => bearerRequest<FeeStatement>('/fees/me', token)
export const getChildFeeStatement = (token: string, studentId: string) => bearerRequest<FeeStatement>(`/fees/children/${id(studentId)}`, token)
export const startFeeIntent = (token: string, input: { student_id?: string; provider: FeeIntent['provider']; amount: number }) => bearerRequest<FeeIntent>('/fees/online/intents', token, json('POST', input))
// Online payment is simulation only: there is no live bKash or Nagad integration.
export type FeeOnlineStatus = { mode: 'off' | 'simulation'; simulation_allowed: boolean; simulation_enabled: boolean; updated_at?: string }
export type FeeSimulation = { simulated: true; intent: FeeIntent; balance: number; message: string; replayed: boolean }
export const getOnlinePaymentStatus = (token: string) => bearerRequest<FeeOnlineStatus>('/fees/online/status', token)
export const saveOnlinePaymentSettings = (token: string, simulationEnabled: boolean) => bearerRequest<FeeOnlineStatus>('/fees/online/settings', token, json('PUT', { simulation_enabled: simulationEnabled }))
export const voidSimulatedPayment = (token: string, paymentId: string, reason: string) => bearerRequest<FeePayment>(`/fees/payments/${id(paymentId)}/void`, token, json('POST', { reason }))
export const confirmFeeIntent = (token: string, intentId: string, otp: string) => bearerRequest<FeeSimulation>(`/fees/online/intents/${id(intentId)}/confirm`, token, json('POST', { otp }))
export const getFeeCollections = (token: string, params: URLSearchParams) => bearerRequest<FeeCollectionReport>(`/fees/reports/collections?${params}`, token)
export const getFeeDueReport = (token: string, params: URLSearchParams) => bearerRequest<FeeDueReport>(`/fees/reports/dues?${params}`, token)
export const getLedgerEntries = (token: string, params: URLSearchParams) => bearerRequest<Page<LedgerEntry>>(`/accounts/entries?${params}`, token)
export const saveLedgerEntry = (token: string, input: LedgerEntryInput) => bearerRequest<LedgerEntry>('/accounts/entries', token, json('POST', input))
export const deleteLedgerEntry = (token: string, entryId: string) => bearerRequest<{ deleted: boolean }>(`/accounts/entries/${id(entryId)}`, token, { method: 'DELETE' })
export const getAccountingSummary = (token: string, params: URLSearchParams) => bearerRequest<AccountingSummary>(`/accounts/summary?${params}`, token)
