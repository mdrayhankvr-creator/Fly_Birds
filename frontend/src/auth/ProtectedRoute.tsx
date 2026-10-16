import { Navigate, useLocation } from 'react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from './AuthProvider'

export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { ready, user } = useAuth()
  const location = useLocation()
  const { t } = useTranslation()

  if (!ready) {
    return <p role="status" className="text-slate-600">{t('restoringSession')}</p>
  }
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return children
}
