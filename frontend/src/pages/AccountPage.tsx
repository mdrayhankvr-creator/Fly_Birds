import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, changeOwnPassword, getCurrentUser, type AuthUser } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { Icon } from '../components/icons'
import { StateScreen } from '../components/StateScreen'
import { InfoRow, PageHeader, SectionCard, ui } from './attendanceUi'

export function AccountPage({ permissions }: { permissions?: string[] }) {
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
        {permissions?.includes('auth:password_change_self')
          ? <PasswordChangeForm />
          : <p className="flex gap-3 rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-900"><Icon name="lock" className="mt-0.5 h-5 w-5 shrink-0" />{t('uxPasswordHelp')}</p>}
        <p className="mt-4 text-sm leading-6 text-slate-600">{t('uxSignOutHint')}</p>
        <button type="button" className={`${ui.secondary} mt-3`} onClick={() => void signOut()}><Icon name="logout" className="h-4 w-4" />{t('signOut')}</button>
      </SectionCard>
    </div>
  </div>
}

// The server counts UTF-8 bytes, so a Bangla password is measured the same way here.
const utf8Length = (value: string) => new TextEncoder().encode(value).length
const passwordErrors: Record<string, string> = {
  weak_password: 'passwordErrorWeak',
  same_password: 'passwordErrorSame',
  invalid_current_password: 'passwordErrorCurrent',
  account_locked: 'passwordErrorLocked',
}

function PasswordChangeForm() {
  const { t } = useTranslation()
  const { accessToken, renew, expire } = useAuth()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [revoked, setRevoked] = useState<number | null>(null)

  const nextLength = utf8Length(next)
  const tooShort = next !== '' && nextLength < 12
  const tooLong = nextLength > 72
  const mismatch = confirm !== '' && confirm !== next

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setRevoked(null)
    if (nextLength < 12 || tooLong) return setError('passwordErrorWeak')
    if (next !== confirm) return setError('passwordErrorMismatch')
    if (next === current) return setError('passwordErrorSame')
    if (!accessToken) return expire()
    setBusy(true)
    try {
      let result
      try {
        result = await changeOwnPassword(accessToken, current, next)
      } catch (cause) {
        if (!(cause instanceof ApiError) || cause.status !== 401) throw cause
        result = await changeOwnPassword(await renew(), current, next)
      }
      setRevoked(result.sessions_revoked)
      setCurrent('')
      setNext('')
      setConfirm('')
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) return expire()
      setError(cause instanceof ApiError ? passwordErrors[cause.code] ?? 'passwordErrorGeneric' : 'passwordErrorGeneric')
    } finally {
      setBusy(false)
    }
  }

  return <form className="space-y-4" onSubmit={submit} noValidate>
    <p className="text-sm leading-6 text-slate-600">{t('passwordChangeIntro')}</p>
    {/* Hints sit outside the labels so they are announced once, as descriptions. */}
    <div><label htmlFor="password-current" className={ui.label}>{t('passwordCurrent')}</label>
      <input id="password-current" className={ui.field} type="password" autoComplete="current-password" required value={current} onChange={event => setCurrent(event.target.value)} /></div>
    <div><label htmlFor="password-new" className={ui.label}>{t('passwordNew')}</label>
      <input id="password-new" className={ui.field} type="password" autoComplete="new-password" required aria-describedby="password-new-hint" aria-invalid={tooShort || tooLong} value={next} onChange={event => setNext(event.target.value)} />
      <p id="password-new-hint" className={`mt-1 text-xs ${tooShort || tooLong ? 'text-rose-700' : 'text-slate-500'}`}>{t('passwordRule')}</p></div>
    <div><label htmlFor="password-confirm" className={ui.label}>{t('passwordConfirm')}</label>
      <input id="password-confirm" className={ui.field} type="password" autoComplete="new-password" required aria-describedby={mismatch ? 'password-confirm-error' : undefined} aria-invalid={mismatch} value={confirm} onChange={event => setConfirm(event.target.value)} />
      {mismatch && <p id="password-confirm-error" className="mt-1 text-xs text-rose-700">{t('passwordErrorMismatch')}</p>}</div>
    {error && <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">{t(error)}</p>}
    {revoked !== null && <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800">{t('passwordChanged', { count: revoked })}</p>}
    <button type="submit" className={`${ui.primary} w-full sm:w-auto`} disabled={busy || !current || !next || !confirm}><Icon name="lock" className="h-4 w-4" />{t(busy ? 'passwordSaving' : 'passwordSubmit')}</button>
  </form>
}
