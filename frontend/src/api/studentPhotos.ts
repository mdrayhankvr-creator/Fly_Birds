import { bearerBlob, bearerRequest, json } from './client'

// Students' own photos, waiting for the class teacher or the administrator
// (migration 1760), and the uploaded school logo (migration 1761).
export type PhotoStatus = '' | 'pending' | 'approved' | 'rejected'
export type PhotoSubmission = { enabled: boolean; status: PhotoStatus; reason: string; submitted_at: string | null; reviewed_at: string | null; has_photo: boolean; student_id: string }
export type PhotoReview = { student_id: string; name_en: string; name_bn: string; class_name: string; section_name: string; roll_no: number | null; submitted_at: string; has_photo: boolean }

export const getMyPhotoSubmission = (token: string) => bearerRequest<PhotoSubmission>('/students/me/photo-submission', token)
export const getMyPhotoSubmissionImage = (token: string) => bearerBlob('/students/me/photo-submission/image', token)
export const submitMyPhoto = (token: string, image: Blob) => bearerRequest<PhotoSubmission>('/students/me/photo-submission', token, { method: 'PUT', headers: { 'Content-Type': image.type }, body: image })
export const getPhotoReviews = (token: string) => bearerRequest<{ items: PhotoReview[]; total: number }>('/students/photo-reviews', token)
export const getReviewImage = (token: string, studentId: string) => bearerBlob(`/students/${encodeURIComponent(studentId)}/photo-submission/image`, token)
export const approvePhoto = (token: string, studentId: string) => bearerRequest<{ status: string }>(`/students/${encodeURIComponent(studentId)}/photo-submission/approve`, token, { method: 'POST' })
export const rejectPhoto = (token: string, studentId: string, reason: string) => bearerRequest<{ status: string }>(`/students/${encodeURIComponent(studentId)}/photo-submission/reject`, token, json('POST', { reason }))

export const uploadSchoolLogo = (token: string, image: Blob) => bearerRequest<{ logo_url: string }>('/school/logo', token, { method: 'PUT', headers: { 'Content-Type': image.type }, body: image })
export const deleteSchoolLogo = (token: string) => bearerRequest<{ logo_url: string }>('/school/logo', token, { method: 'DELETE' })
