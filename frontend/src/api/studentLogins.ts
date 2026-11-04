import { bearerRequest, json } from './client'

// Student logins (checkpoint 5). Each response carries temporary passwords
// once; they are not stored or logged anywhere.
export type StudentLogin = {
  student_id: string; student_name: string; name_bn: string; admission_no: string
  class_name: string; section_name: string; roll_no: number | null
  user_id: string; login_id: string; email?: string; initial_password: string
  /** When the temporary password stops working if the student has not set their own. */
  password_expires_at: string
}
export const createStudentLogin = (token: string, studentId: string, email = '') =>
  bearerRequest<StudentLogin>(`/students/${encodeURIComponent(studentId)}/login`, token, json('POST', { email }))
export const createSectionLogins = (token: string, sectionId: string) =>
  bearerRequest<{ logins: StudentLogin[] }>(`/students/logins?section_id=${encodeURIComponent(sectionId)}`, token, { method: 'POST' })
