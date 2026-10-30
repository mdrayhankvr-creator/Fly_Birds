import { bearerRequest, type ManagedUser } from './client'

// Checkpoint 4 account safety: an administrator's reset of another account.
// The temporary password is in this response only; the account must set its
// own password at the next sign-in.
export type PasswordReset = { user: ManagedUser; temporary_password: string; sessions_revoked: number }
export const resetUserPassword = (token: string, id: string) => bearerRequest<PasswordReset>(`/users/${encodeURIComponent(id)}/password-reset`, token, { method: 'POST' })
