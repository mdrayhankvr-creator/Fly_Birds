import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, changeOwnPassword } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { Icon } from '../components/icons'
import { Notice, ui } from './attendanceUi'

const errorKeys: Record<string, string> = { invalid_current_password: 'c4WrongTemporary', account_locked: 'c4Locked', weak_password: 'c4PasswordLength', same_password: 'c4PasswordSame' }

// ForcedPasswordPage stands in for the whole app while the account's password
// was set by someone else (admission or an administrator's reset). The server
// refuses everything else until the password is changed.
export function ForcedPasswordPage() {
  const { t } = useTranslation()
  const { user, accessToken, renew, signOut } = useAuth()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const bytes = new TextEncoder().encode(next).length
  const mismatch = repeat !== '' && repeat !== next
  async function submit(event: FormEvent) {
    event.preventDefault()
    if (busy || !accessToken) return
    if (bytes < 12 || bytes > 72) { setError(t('c4PasswordLength')); return }
    if (next !== repeat) { setError(t('c4PasswordMismatch')); return }
    if (next === current) { setError(t('c4PasswordSame')); return }
    setBusy(true); setError('')
    try {
      try { await changeOwnPassword(accessToken, current, next) }
      catch (cause) { if (cause instanceof ApiError && cause.status === 401) await changeOwnPassword(await renew(), current, next); else throw cause }
      // A fresh session reports the change; the app then opens as usual.
      await renew()
    } catch (cause) {
      setError(t(cause instanceof ApiError && errorKeys[cause.code] ? errorKeys[cause.code] : 'c4ChangeFailed'))
    } finally { setBusy(false) }
  }
  return <div className="mx-auto max-w-lg">
    <section className={`${ui.card} p-5 sm:p-8`} aria-labelledby="forced-title">
      <span aria-hidden="true" className="grid h-12 w-12 place-items-center rounded-2xl bg-[var(--brand-primary)] text-[var(--brand-on)]"><Icon name="lock" className="h-6 w-6" /></span>
      <h1 id="forced-title" className="mt-4 text-2xl font-extrabold tracking-tight text-slate-900">{t('c4ForcedTitle')}</h1>
      <p className="mt-2 text-sm leading-6 text-slate-600">{t('c4ForcedIntro', { email: user?.email ?? '' })}</p>
      {error && <div className="mt-4"><Notice tone="error">{error}</Notice></div>}
      <form className="mt-5 space-y-4" onSubmit={e => void submit(e)}>
        <label className={ui.label}>{t('c4TemporaryPassword')}<input className={ui.field} type="password" required autoComplete="current-password" value={current} onChange={e => setCurrent(e.target.value)} /></label>
        <label className={ui.label}>{t('c4NewPassword')}<input className={ui.field} type="password" required minLength={12} autoComplete="new-password" aria-describedby="forced-rule" value={next} onChange={e => setNext(e.target.value)} /></label>
        <p id="forced-rule" className={`text-xs ${next && (bytes < 12 || bytes > 72) ? 'font-semibold text-rose-700' : 'text-slate-500'}`}>{t('c4PasswordRule')}</p>
        <label className={ui.label}>{t('c4RepeatPassword')}<input className={ui.field} type="password" required autoComplete="new-password" aria-invalid={mismatch} value={repeat} onChange={e => setRepeat(e.target.value)} />
          {mismatch && <span role="alert" className="mt-1 block text-xs font-semibold normal-case tracking-normal text-rose-700">{t('c4PasswordMismatch')}</span>}</label>
        <button disabled={busy} className={`${ui.primary} w-full`}><Icon name="check" className="h-5 w-5" />{busy ? t('c3Saving') : t('c4SetPassword')}</button>
      </form>
      <button type="button" className={`${ui.quiet} mt-3 w-full`} onClick={() => void signOut()}>{t('c4SignOutInstead')}</button>
    </section>
  </div>
}
