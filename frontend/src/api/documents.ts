import { ApiError, bearerBlob, bearerRequest, json } from './client'

// Result documents (E7): PDF marksheets and class documents.

export type DocumentKind = 'marksheet' | 'marksheets' | 'tabulation' | 'merit_list' | 'failed_list' | 'summary'
export type DocumentLanguage = 'en' | 'bn'

export type DocumentRequest = {
  kind: DocumentKind
  scope: 'term' | 'annual'
  scope_id: string
  class_id: string
  section_id?: string
  student_id?: string
  language: DocumentLanguage
  provisional?: boolean
}

export type ResultDocument = {
  id: string; kind: DocumentKind; scope: 'term' | 'annual'; scope_id: string; session_id: string; class_id: string
  section_id: string | null; student_id: string | null; language: DocumentLanguage; provisional: boolean
  status: 'queued' | 'running' | 'done' | 'failed'; progress_done: number; progress_total: number
  pages: number | null; bytes: number | null; sha256: string | null; error: string; created_at: string; finished_at: string | null; file_name: string
}

export type DocumentClass = {
  scope: 'term' | 'annual'; scope_id: string; session_id: string; session_name: string; class_id: string; class_name: string
  title: string; revision: number; published_at: string; students: number; sections: { id: string; name: string }[]
}

export type ResultVerification = {
  state: 'current' | 'superseded' | 'withdrawn'; code: string; scope: 'term' | 'annual'; title: string; session_name: string
  class_name: string; version: number; published_at: string; school_name_en: string; school_name_bn: string; logo_url: string
}

const id = encodeURIComponent
export const getDocumentClasses = (token: string) => bearerRequest<DocumentClass[]>('/exams/documents/classes', token)
export const requestDocument = (token: string, input: DocumentRequest) => bearerRequest<ResultDocument>('/exams/documents', token, json('POST', input))
export const getDocument = (token: string, docId: string) => bearerRequest<ResultDocument>(`/exams/documents/${id(docId)}`, token)
export const downloadDocument = (token: string, docId: string) => bearerBlob(`/exams/documents/${id(docId)}/file`, token)
export const downloadMyMarksheet = (token: string, versionId: string, language: DocumentLanguage) => bearerBlob(`/exams/me/documents/${id(versionId)}?lang=${language}`, token)

// The public check behind a marksheet's QR code (no sign-in).
export async function verifyResult(token: string): Promise<ResultVerification> {
  const base = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
  const response = await fetch(`${base}/public/results/${id(token)}`, { headers: { Accept: 'application/json' } })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new ApiError(response.status, body?.error?.code || 'request_failed')
  return body.data as ResultVerification
}
