import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, getCurrentUser, type AuthUser } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

export function AccountPage() {
  const { t } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const [user, setUser] = useState<AuthUser | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    if (!accessToken) return
    let active = true
    void getCurrentUser(accessToken).catch(async (cause) => {
      if (cause instanceof ApiError && cause.status === 401) {
        const replacement = await renew()
        return getCurrentUser(replacement)
      }
      throw cause
    }).then((current) => {
      if (active) { setUser(current); setError(false) }
    }).catch((cause) => {
      if (!active) return
      if (cause instanceof ApiError && cause.status === 401) expire()
      else setError(true)
    })
    return () => { active = false }
  }, [accessToken, renew, expire])

  return (
    <section className="max-w-2xl rounded-2xl border border-slate-200 bg-white p-6 shadow-sm sm:p-8">
      <h1 className="text-3xl font-bold text-slate-900">{t('accountHeading')}</h1>
      <p className="mt-3 text-slate-600">{t('accountDescription')}</p>
      {!user && !error && <p role="status" className="mt-6 text-slate-600">{t('loadingAccount')}</p>}
      {error && <p role="alert" className="mt-6 text-rose-700">{t('accountError')}</p>}
      {user && !error && <dl className="mt-6 grid gap-4 sm:grid-cols-2">
        <div><dt className="text-sm font-semibold text-slate-500">{t('email')}</dt><dd className="mt-1 break-all text-slate-900">{user.email}</dd></div>
        <div><dt className="text-sm font-semibold text-slate-500">{t('role')}</dt><dd className="mt-1 text-slate-900">{user.role}</dd></div>
      </dl>}
    </section>
  )
}
