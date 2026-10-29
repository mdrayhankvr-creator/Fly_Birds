import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { ui } from './attendanceUi'

// After sign-in, return to the page that asked for it, if it was ours.
function useAfterSignIn() {
  const navigate = useNavigate()
  const location = useLocation()
  return () => {
    const requested = location.state?.from
    const destination = typeof requested === 'string' && requested.startsWith('/') && !requested.startsWith('//') ? requested : '/dashboard'
    navigate(destination, { replace: true })
  }
}

// The sign-in form, shown on the school's landing page.
export function SignInForm() {
  const { t } = useTranslation()
  const { expired, signIn } = useAuth()
  const done = useAfterSignIn()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [showPassword, setShowPassword] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    if (!event.currentTarget.reportValidity() || new TextEncoder().encode(password).length > 72) {
      setError(t('invalidLoginForm'))
      return
    }
    setPending(true)
    try {
      await signIn(email.trim(), password)
      done()
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) setError(t('invalidCredentials'))
      else if (cause instanceof ApiError && cause.status === 503) setError(t('authUnavailable'))
      else setError(t('loginError'))
    } finally {
      setPending(false)
    }
  }

  return <div className="landing-card rounded-2xl border border-slate-200 bg-white p-6 shadow-[0_24px_60px_-28px_rgba(15,23,42,0.45)] sm:p-8">
    <form onSubmit={(event) => void submit(event)} aria-labelledby="sign-in-title">
      <p className="text-sm font-semibold text-[var(--brand-ink)]">{t('welcomeBack')}</p>
      <h2 id="sign-in-title" className="mt-1 text-2xl font-bold tracking-tight text-slate-900">{t('signIn')}</h2>
      <p className="mt-1.5 text-sm text-slate-600">{t('loginDescription')}</p>
      {expired && <p role="status" className="mt-5 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">{t('sessionExpired')}</p>}
      {error && <p role="alert" className="mt-5 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">{error}</p>}
      <label htmlFor="login-email" className="mt-6 block text-sm font-semibold text-slate-700">{t('email')}</label>
      <input id="login-email" name="email" type="email" autoComplete="username" required maxLength={254} value={email}
        onChange={(event) => setEmail(event.target.value)} disabled={pending} className={`${ui.field} py-3`} />
      <label htmlFor="login-password" className="mt-5 block text-sm font-semibold text-slate-700">{t('password')}</label>
      <div className="relative">
        <input id="login-password" name="password" type={showPassword ? 'text' : 'password'} autoComplete="current-password" required value={password}
          onChange={(event) => setPassword(event.target.value)} disabled={pending} className={`${ui.field} py-3 pr-24`} />
        <button type="button" onClick={() => setShowPassword(v => !v)} aria-pressed={showPassword} aria-controls="login-password"
          className="absolute inset-y-0 right-1.5 my-auto h-9 rounded-lg px-3 text-xs font-semibold text-slate-600 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">{t(showPassword ? 'm12HidePassword' : 'm12ShowPassword')}</button>
      </div>
      <button type="submit" disabled={pending} className={`${ui.primary} mt-7 w-full py-3 text-[15px]`}>
        {pending ? t('signingIn') : t('signIn')}
      </button>
      <p className="mt-5 text-xs leading-5 text-slate-500">{t('m12LoginHelp')}</p>
    </form>
  </div>
}
