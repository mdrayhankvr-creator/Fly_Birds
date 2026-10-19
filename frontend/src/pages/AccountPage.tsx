import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, getCurrentUser, type AuthUser } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { Icon } from '../components/icons'
import { StateScreen } from '../components/StateScreen'
import { InfoRow, PageHeader, SectionCard, ui } from './attendanceUi'

export function AccountPage() {
  const { t } = useTranslation()
  const { accessToken, renew, expire, signOut } = useAuth()
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

  if (error) return <StateScreen kind="error" message={t('accountError')} />
  if (!user) return <StateScreen kind="loading" message={t('loadingAccount')} />
  const name = user.display_name?.trim() || user.email
  const initials = name.split(/[\s@.]+/).filter(Boolean).slice(0, 2).map(part => [...part][0]).join('').toUpperCase()
  return <div className="space-y-6">
    <PageHeader eyebrow={t('uxAccountEyebrow')} title={t('accountHeading')} description={t('accountDescription')} />
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
      <SectionCard title={t('uxProfile')} id="account-profile">
        <div className="flex items-center gap-4">
          <span aria-hidden="true" className="grid h-14 w-14 shrink-0 place-items-center rounded-2xl bg-[var(--brand-soft)] text-lg font-bold text-[var(--brand-ink)]">{initials}</span>
          <div className="min-w-0"><p className="truncate text-lg font-bold text-slate-900">{name}</p>
            <span className="mt-1 inline-flex items-center gap-1.5 rounded-full bg-[var(--brand-soft)] px-2.5 py-0.5 text-xs font-semibold text-[var(--brand-ink)]"><Icon name="shield" className="h-3.5 w-3.5" />{t(`role_${user.role}`)}</span></div>
        </div>
        <dl className="mt-5 divide-y divide-slate-100 border-t border-slate-100">
          {user.display_name && <InfoRow label={t('uxDisplayName')}>{user.display_name}</InfoRow>}
          <InfoRow label={t('email')}><span className="break-all">{user.email}</span></InfoRow>
          <InfoRow label={t('role')}>{t(`role_${user.role}`)}</InfoRow>
        </dl>
      </SectionCard>
      <SectionCard title={t('uxSecurity')} id="account-security">
        <p className="flex gap-3 rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-900"><Icon name="lock" className="mt-0.5 h-5 w-5 shrink-0" />{t('uxPasswordHelp')}</p>
        <p className="mt-4 text-sm leading-6 text-slate-600">{t('uxSignOutHint')}</p>
        <button type="button" className={`${ui.secondary} mt-3`} onClick={() => void signOut()}><Icon name="logout" className="h-4 w-4" />{t('signOut')}</button>
      </SectionCard>
    </div>
  </div>
}
