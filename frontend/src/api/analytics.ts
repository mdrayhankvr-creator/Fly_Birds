import { bearerBlob, bearerRequest, json } from './client'

export type Filter = { session_id?: string; class_id?: string; section_id?: string; exam_id?: string; exam_type?: string; from?: string; to?: string; group?: string }
export type Session = { id: string; name: string; start_date: string; end_date: string; is_active: boolean }
export type AlertSettings = {
  attendance_threshold: number; attendance_window_days: number; min_marked_days: number; gpa_threshold: number
  consecutive_absences: number; inactivity_days: number; updated_at?: string
}
export type Options = {
  sessions: Session[]; session: Session | null; classes: { id: string; name: string }[]; sections: { id: string; class_id: string; name: string }[]
  exams: { id: string; name: string; exam_type: string; class_id: string; section_id: string }[]; modules: Record<string, boolean>; today: string; settings: AlertSettings
}
export type Counts = { marked: number; present: number; late: number; absent: number; rate: number | null }
export type TeacherCounts = { marked: number; present: number; absent: number; leave: number; rate: number | null }
export type Attendance = {
  filter: Filter; summary: Counts; days: number; students: number; trend: (Counts & { period: string })[]; group_by: 'class' | 'section'
  groups: (Counts & { id: string; name: string })[]; teachers?: TeacherCounts; teacher_trend?: (TeacherCounts & { period: string })[]
}
export type ResultSummary = { results: number; passed: number; failed: number; pass_rate: number | null; avg_gpa: number | null; gpa5: number }
export type Exams = {
  filter: Filter; summary: ResultSummary
  exams: { exam_id: string; name: string; exam_type: string; class_name: string; section_name: string; published_on: string; results: number; passed: number; pass_rate: number | null; avg_gpa: number | null }[]
  distribution: { grade: string; gpa: number; count: number }[]
  subjects: { subject_id: string; name_en: string; name_bn: string; code: string; entries: number; passed: number; pass_rate: number | null; avg_percent: number | null }[]
  group_by: 'class' | 'section'; groups: { id: string; name: string; results: number; passed: number; pass_rate: number | null; avg_gpa: number | null }[]
}
export type FeeSummary = {
  billed: number; discounts: number; net_billed: number; paid_against_billed: number; collection_rate: number | null; collected: number
  outstanding: number; overdue: number; overdue_students: number; income: number; expense: number; net: number
}
export type Fees = {
  filter: Filter; summary: FeeSummary; months: { month: string; billed: number; collected: number; income: number; expense: number }[]
  methods: { method: string; amount: number; receipts: number }[]; classes: { id: string; name: string; net_billed: number; paid: number; outstanding: number; rate: number | null }[]
  aging: { bucket: string; amount: number; students: number }[]
  top_overdue: { student_id: string; name_en: string; name_bn: string; admission_no: string; class_name: string; section_name: string; overdue: number; oldest_due: string }[]
  ledger: { entry_type: 'income' | 'expense'; category: string; amount: number }[]
}
export type SessionRow = Session & {
  active_students: number; attendance_marked: number; attendance_rate: number | null; exams: number; results: number; pass_rate: number | null; avg_gpa: number | null
  net_billed?: number; collected?: number; collection_rate?: number | null
}
export type RiskSummary = { at_risk: number; dropout: number; students: number; reasons: Record<string, number> }
export type Risk = {
  student_id: string; name_en: string; name_bn: string; admission_no: string; roll_no: number | null; class_name: string; section_name: string
  level: 'dropout' | 'at_risk'; reasons: string[]; marked: number; attendance_rate: number | null; absent_streak: number; last_present: string | null
  latest_exam: string | null; latest_gpa: number | null; latest_passed: boolean | null
}
export type RiskPage = { filter: Filter; summary: RiskSummary; settings: AlertSettings; items: Risk[]; total: number; page: number; page_size: number }
export type Overview = {
  session: Session | null; active_students: number; students_by_status: Record<string, number>; by_class: { id: string; name: string; students: number }[]
  teachers: number; non_teaching_staff: number; guardians: number; today: string; today_students: Counts | null; today_teachers: TeacherCounts | null
  last_30_days: Counts | null; results: ResultSummary | null; risks: RiskSummary | null; fees?: FeeSummary
}
export type StudentResult = {
  exam_id: string; name: string; exam_type: string; published_on: string; total_marks: number; full_marks: number; percent: number | null
  gpa: number; grade: string; passed: boolean; class_position: number; section_position: number
}
export type StudentSummary = {
  student: { id: string; name_en: string; name_bn: string; admission_no: string; roll_no: number | null; status: string; session_name: string; class_name: string; section_name: string }
  today: string
  attendance: { session: Counts; last_30_days: Counts; months: (Counts & { period: string })[]; recent: { date: string; status: 'present' | 'late' | 'absent' }[] } | null
  results: StudentResult[]
  latest_subjects: { name_en: string; name_bn: string; marks: number; full_marks: number; percent: number | null; grade: string | null; grade_point: number | null; passed: boolean | null }[]
  fees: { outstanding: number; overdue: number; paid_session: number; next_due: string | null } | null
  alert: Risk | null; settings: AlertSettings
}
export type ReportKey = 'student_attendance' | 'teacher_attendance' | 'exam_results' | 'fee_balances' | 'risks'
export type ReportDef = { key: ReportKey; access: 'academic' | 'school' | 'finance'; module: string; ranged: boolean; columns: string[] }
export type ReportPage = { report: ReportKey; filter: Filter; columns: string[]; rows: string[][]; total: number; page: number; page_size: number }

const id = encodeURIComponent
export function query(filter: Filter, extra: Record<string, string | number> = {}) {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries({ ...filter, ...extra })) if (v !== undefined && v !== '') params.set(k, String(v))
  return params.toString()
}

export const getOptions = (token: string, session = '') => bearerRequest<Options>(`/analytics/options?${query({ session_id: session })}`, token)
export const getOverview = (token: string, session = '') => bearerRequest<Overview>(`/analytics/overview?${query({ session_id: session })}`, token)
export const getAttendance = (token: string, f: Filter) => bearerRequest<Attendance>(`/analytics/attendance?${query(f)}`, token)
export const getExams = (token: string, f: Filter) => bearerRequest<Exams>(`/analytics/exams?${query(f)}`, token)
export const getFees = (token: string, f: Filter) => bearerRequest<Fees>(`/analytics/fees?${query(f)}`, token)
export const getSessions = (token: string) => bearerRequest<{ items: SessionRow[] }>('/analytics/sessions', token)
export const getRisks = (token: string, f: Filter, level: string, reason: string, page: number, size = 25) =>
  bearerRequest<RiskPage>(`/analytics/risks?${query(f, { level, reason, page, page_size: size })}`, token)
export const saveAlertSettings = (token: string, s: AlertSettings) => bearerRequest<AlertSettings>('/analytics/settings', token, json('PUT', s))
export const getStudentSummary = (token: string, student: string) => bearerRequest<StudentSummary>(`/analytics/students/${id(student)}`, token)
export const getMySummary = (token: string) => bearerRequest<StudentSummary>('/analytics/me', token)
export const getChildren = (token: string) => bearerRequest<{ items: StudentSummary[] }>('/analytics/children', token)
export const getReports = (token: string) => bearerRequest<{ items: ReportDef[]; max_rows: number }>('/analytics/reports', token)
export const getReport = (token: string, key: ReportKey, f: Filter, page: number, size = 50) =>
  bearerRequest<ReportPage>(`/analytics/reports/${id(key)}?${query(f, { page, page_size: size })}`, token)
export const downloadReport = (token: string, key: ReportKey, f: Filter) => bearerBlob(`/analytics/reports/${id(key)}/csv?${query(f)}`, token)
