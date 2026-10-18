import { bearerBlob, bearerRequest, bearerUpload, json, type Page } from './client'

export type AudienceRole = 'head_teacher' | 'teacher' | 'student' | 'guardian' | 'accountant'
export const audienceRoles: AudienceRole[] = ['student', 'guardian', 'teacher', 'head_teacher', 'accountant']
export type Target = { class_id: string; section_id: string | null; class_name?: string; section_name?: string }
export type Attachment = { id: string; name: string; content_type: string; size: number }
export type Priority = 'normal' | 'important' | 'urgent'
export type Notice = { id: string; title_en: string; title_bn: string; body_en: string; body_bn: string; priority: Priority; audience_roles: AudienceRole[]; status: 'draft' | 'published' | 'archived'; publish_at: string | null; expires_at: string | null; live: boolean; author_name: string; updated_at: string; targets: Target[]; attachments: Attachment[] }
export type NoticeInput = { title_en: string; title_bn: string; body_en: string; body_bn: string; priority: Priority; audience_roles: AudienceRole[]; targets: Target[]; expires_at: string | null; file_ids: string[] }
export type EventKind = 'holiday' | 'exam' | 'meeting' | 'event' | 'other'
export type SchoolEvent = { id: string; title_en: string; title_bn: string; description_en: string; description_bn: string; kind: EventKind; starts_on: string; ends_on: string; start_time: string | null; end_time: string | null; location: string; audience_roles: AudienceRole[]; status: 'draft' | 'published' | 'cancelled'; targets: Target[] }
export type EventInput = Omit<SchoolEvent, 'id' | 'status' | 'targets'> & { targets: Target[] }
export type NotificationCategory = 'notice' | 'absence' | 'result' | 'fee' | 'diary'
export type AppNotification = { id: string; category: NotificationCategory; title_en: string; title_bn: string; body_en: string; body_bn: string; link: string; urgent: boolean; read: boolean; created_at: string }
export type Inbox = Page<AppNotification> & { unread: number }
export type Preference = { category: NotificationCategory; in_app: boolean; sms: boolean }
export type MockSMS = { id: string; recipient_name: string; phone: string; category: NotificationCategory; body: string; status: 'queued' | 'sent_mock' | 'failed' | 'cancelled'; provider: string; provider_ref: string; error: string; attempts: number; created_at: string; processed_at: string | null }
export type DispatchResult = { skipped: boolean; disabled: boolean; created: Record<string, number>; withdrawn: number; sms_queued: number; sms_sent_mock: number; sms_failed: number; sms_retrying: number }
export type DiaryAssignment = { session_id: string; session_name: string; class_id: string; class_name: string; section_id: string; section_name: string; subject_id: string; subject_name_en: string; subject_name_bn: string; diary_id: string | null; diary_status: 'draft' | 'published' | null }
export type Diary = { id: string; session_id: string; class_id: string; section_id: string; subject_id: string; class_name: string; section_name: string; subject_name_en: string; subject_name_bn: string; teacher_name: string; diary_date: string; lesson_topics: string; class_notes: string; homework: string; instructions: string; status: 'draft' | 'published'; published_at: string | null; submission_enabled: boolean; submission_due_at: string | null; submission_open: boolean; submission_count: number; my_submission_status: 'submitted' | 'reviewed' | null; attachments: Attachment[] }
export type DiaryInput = { section_id?: string; subject_id?: string; diary_date?: string; lesson_topics: string; class_notes: string; homework: string; instructions: string; submission_enabled: boolean; submission_due_at: string | null; file_ids: string[] }
export type Submission = { id: string; diary_id: string; student_id: string; student_name: string; student_name_bn: string; admission_no: string; roll_no: number | null; answer_text: string; status: 'missing' | 'submitted' | 'reviewed'; review_note: string; submitted_at: string | null; reviewed_at: string | null; files: Attachment[] }
export type DiarySettings = { daily_required: boolean; reminder_time: string }
export type ComplianceRow = { class_name: string; section_name: string; subject_name_en: string; subject_name_bn: string; teachers: string; diary_status: 'draft' | 'published' | null }

const id = (v: string) => encodeURIComponent(v)

export const getNoticeFeed = (token: string, params: URLSearchParams) => bearerRequest<Page<Notice>>(`/notices?${params}`, token)
export const getManagedNotices = (token: string, params: URLSearchParams) => bearerRequest<Page<Notice>>(`/notices/manage?${params}`, token)
export const saveNotice = (token: string, input: NoticeInput, noticeId?: string) => bearerRequest<Notice>(noticeId ? `/notices/${id(noticeId)}` : '/notices', token, json(noticeId ? 'PATCH' : 'POST', input))
export const publishNotice = (token: string, noticeId: string, publishAt: string | null) => bearerRequest<Notice>(`/notices/${id(noticeId)}/publish`, token, json('POST', { publish_at: publishAt }))
export const archiveNotice = (token: string, noticeId: string) => bearerRequest<Notice>(`/notices/${id(noticeId)}/archive`, token, { method: 'POST' })
export const deleteNotice = (token: string, noticeId: string) => bearerRequest<{ deleted: boolean }>(`/notices/${id(noticeId)}`, token, { method: 'DELETE' })
export const uploadFile = (token: string, path: '/notices/files' | '/diary/files' | '/diary/homework/files', file: File) => bearerUpload<Attachment>(path, token, file)
export const fetchFile = (token: string, path: string) => bearerBlob(path, token)

export const getEvents = (token: string, params: URLSearchParams) => bearerRequest<SchoolEvent[]>(`/events?${params}`, token)
export const saveEvent = (token: string, input: EventInput, eventId?: string) => bearerRequest<SchoolEvent>(eventId ? `/events/${id(eventId)}` : '/events', token, json(eventId ? 'PATCH' : 'POST', input))
export const setEventStatus = (token: string, eventId: string, action: 'publish' | 'cancel') => bearerRequest<SchoolEvent>(`/events/${id(eventId)}/${action}`, token, { method: 'POST' })
export const deleteEvent = (token: string, eventId: string) => bearerRequest<{ deleted: boolean }>(`/events/${id(eventId)}`, token, { method: 'DELETE' })

export const getInbox = (token: string, params: URLSearchParams) => bearerRequest<Inbox>(`/notifications?${params}`, token)
export const markNotificationRead = (token: string, notificationId: string) => bearerRequest<{ read: boolean }>(`/notifications/${id(notificationId)}/read`, token, { method: 'POST' })
export const markAllNotificationsRead = (token: string) => bearerRequest<{ read: number }>('/notifications/read-all', token, { method: 'POST' })
export const getPreferences = (token: string) => bearerRequest<Preference[]>('/notifications/preferences', token)
export const savePreferences = (token: string, items: Preference[]) => bearerRequest<Preference[]>('/notifications/preferences', token, json('PUT', { items }))
export const runDispatch = (token: string) => bearerRequest<DispatchResult>('/notifications/dispatch', token, { method: 'POST' })
export const getSMSLog = (token: string, params: URLSearchParams) => bearerRequest<Page<MockSMS> & { counts: Record<string, number> }>(`/notifications/sms?${params}`, token)

export const getDiaryAssignments = (token: string, date: string) => bearerRequest<DiaryAssignment[]>(`/diary/assignments?date=${id(date)}`, token)
export const getTeachingDiaries = (token: string, params: URLSearchParams) => bearerRequest<Page<Diary>>(`/diary/teaching?${params}`, token)
export const getDiaryFeed = (token: string, studentId: string | null, params: URLSearchParams) => bearerRequest<Page<Diary>>(`${studentId ? `/diary/children/${id(studentId)}` : '/diary/me'}?${params}`, token)
export const getAllDiaries = (token: string, params: URLSearchParams) => bearerRequest<Page<Diary>>(`/diary/all?${params}`, token)
export const getDiary = (token: string, diaryId: string, studentId?: string) => bearerRequest<Diary>(`/diary/${id(diaryId)}${studentId ? `?student_id=${id(studentId)}` : ''}`, token)
export const saveDiary = (token: string, input: DiaryInput, diaryId?: string) => bearerRequest<Diary>(diaryId ? `/diary/${id(diaryId)}` : '/diary', token, json(diaryId ? 'PATCH' : 'POST', input))
export const publishDiary = (token: string, diaryId: string) => bearerRequest<Diary>(`/diary/${id(diaryId)}/publish`, token, { method: 'POST' })
export const deleteDiary = (token: string, diaryId: string) => bearerRequest<{ deleted: boolean }>(`/diary/${id(diaryId)}`, token, { method: 'DELETE' })
export const getSubmissions = (token: string, diaryId: string) => bearerRequest<Submission[]>(`/diary/${id(diaryId)}/submissions`, token)
export const reviewSubmission = (token: string, submissionId: string, note: string) => bearerRequest<Submission>(`/diary/submissions/${id(submissionId)}/review`, token, json('PATCH', { note }))
export const getMySubmission = (token: string, diaryId: string) => bearerRequest<Submission>(`/diary/${id(diaryId)}/submission`, token)
export const submitHomework = (token: string, diaryId: string, input: { answer_text: string; file_ids: string[] }) => bearerRequest<Submission>(`/diary/${id(diaryId)}/submission`, token, json('PUT', input))
export const getDiarySettings = (token: string) => bearerRequest<DiarySettings>('/diary/settings', token)
export const saveDiarySettings = (token: string, input: DiarySettings) => bearerRequest<DiarySettings>('/diary/settings', token, json('PUT', input))
export const getDiaryCompliance = (token: string, date: string) => bearerRequest<ComplianceRow[]>(`/diary/compliance?date=${id(date)}`, token)
