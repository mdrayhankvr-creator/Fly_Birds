import { bearerRequest, json, type Guardian, type GuardianInput, type Student, type StudentGuardian, type StudentInput } from './client'

// One-page admission (checkpoint 3): the student, an optional login and up to
// two guardians are created in one server transaction.
export type AdmissionGuardian = { guardian_id?: string; new?: Omit<GuardianInput, 'user_id'> & { user_id: '' }; relationship: string; is_primary: boolean; confirm_new?: boolean }
export type AdmissionInput = { student: Omit<StudentInput, 'user_id'> & { user_id: '' }; account: { create: boolean; email: string }; guardians: AdmissionGuardian[] }
export type AdmissionLogin = { user_id: string; email: string; initial_password: string }
export type AdmissionResult = { student: Student; guardians: StudentGuardian[]; login?: AdmissionLogin }
export type GuardianMatch = Guardian & { matched_on: 'phone' | 'email'; children: string[] }
export type AdmissionDefaults = { next_roll: number; admission_no: string; login_domain: string }

export const admitStudent = (token: string, input: AdmissionInput) => bearerRequest<AdmissionResult>('/students/admission', token, json('POST', input))
export const getAdmissionDefaults = (token: string, sectionId: string) => bearerRequest<AdmissionDefaults>(`/students/admission/defaults?${new URLSearchParams(sectionId ? { section_id: sectionId } : {})}`, token)
export const getGuardianMatches = (token: string, phone: string, email: string) => bearerRequest<GuardianMatch[]>(`/guardians/matches?${new URLSearchParams({ phone, email })}`, token)
