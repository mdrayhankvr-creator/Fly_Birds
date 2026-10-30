import { bearerBlob, bearerRequest, json, type EmployeeLeave } from './client'

// Staff profiles (checkpoint 4). The server leaves out every field the
// viewer may not see, so an absent field means "not for you".
export type Audience = { staff: boolean; families: boolean }
export type StaffVisibility = { phone: Audience; email: Audience; qualifications: Audience }
export type StaffCounts = { present: number; absent: number; leave: number }
export type StaffProfile = {
  id: string; kind: 'teacher' | 'non_teaching'; name_en: string; name_bn: string; designation: string; is_active: boolean; photo_version: string
  teaching: { subject_en: string; subject_bn: string; class_name: string }[]
  today: { start_minute: number; end_minute: number; subject_en: string; subject_bn: string; class_name: string; section_name: string; room: string }[]
  phone?: string; email?: string; qualifications?: string[]; joined_on?: string; mpo_index?: string; nid_masked?: string
  attendance?: { month: StaffCounts; session: StaffCounts }; leaves?: EmployeeLeave[]; visibility?: StaffVisibility
  self: boolean; can_edit_own: boolean; can_edit_official: boolean; can_reveal_nid: boolean; can_see_salary: boolean
}
export type OwnProfileInput = { qualifications: string[]; visibility: StaffVisibility }
export type OfficialInput = { email?: string; mpo_index?: string; nid?: string }

const base = (id: string) => `/staff/profiles/${encodeURIComponent(id)}`
export const getStaffProfile = (token: string, id: string) => bearerRequest<StaffProfile>(id === 'me' ? '/staff/me/profile' : base(id), token)
export const getStaffPhoto = (token: string, id: string, version: string) => bearerBlob(`${base(id)}/photo?v=${encodeURIComponent(version)}`, token)
export const saveOwnStaffProfile = (token: string, input: OwnProfileInput) => bearerRequest<StaffProfile>('/staff/me/profile', token, json('PATCH', input))
export const saveStaffOfficial = (token: string, id: string, input: OfficialInput) => bearerRequest<StaffProfile>(`/staff/employees/${encodeURIComponent(id)}/official`, token, json('PATCH', input))
export const revealStaffNID = (token: string, id: string) => bearerRequest<{ nid: string }>(`${base(id)}/nid-reveal`, token, { method: 'POST' })
// The employee's own photo goes to /staff/me/photo; an administrator's change to the employee's record.
export const saveStaffPhoto = (token: string, file: File, id?: string) => bearerRequest<StaffProfile>(id ? `/staff/employees/${encodeURIComponent(id)}/photo` : '/staff/me/photo', token, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file })
export const deleteStaffPhoto = (token: string, id?: string) => bearerRequest<{ deleted: boolean }>(id ? `/staff/employees/${encodeURIComponent(id)}/photo` : '/staff/me/photo', token, { method: 'DELETE' })
