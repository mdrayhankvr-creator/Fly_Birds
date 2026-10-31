import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useAuth } from '../auth/AuthProvider'

// Whether the signed-in user may open student profiles: the same rule as the
// student pages (administrators and the head teacher with students:read).
export function useCanOpenStudents(): boolean {
  const { user } = useAuth()
  const permissions = useQueryClient().getQueryData<string[]>(['my-permissions', user?.id]) ?? []
  return (user?.role === 'admin' || user?.role === 'head_teacher') && permissions.includes('students:read')
}

// StudentName names a student and, for viewers who may open profiles, links
// the name to the student's profile.
export function StudentName({ id, children, className = '' }: { id: string; children: ReactNode; className?: string }) {
  const can = useCanOpenStudents()
  if (!can) return <>{children}</>
  return <Link to={`/admin/students/${encodeURIComponent(id)}`} className={`underline decoration-[color-mix(in_srgb,currentColor_40%,transparent)] decoration-2 underline-offset-4 hover:decoration-current focus-visible:rounded focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] ${className}`}>{children}</Link>
}
