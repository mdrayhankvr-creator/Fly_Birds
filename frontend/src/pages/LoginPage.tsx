import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useLocation, useNavigate } from 'react-router'
import { ApiError, type SchoolBranding } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { SchoolMark } from '../components/AppShell'
import { Icon } from '../components/icons'
import { ui } from './attendanceUi'

export function LoginPage({ branding }: { branding?: SchoolBranding }) {
  const { t, i18n } = useTranslation()
  const { user, ready, expired, signIn } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [showPassword, setShowPassword] = useState(false)

  if (!ready) return <p role="status" className="text-slate-600">{t('restoringSession')}</p>
  if (user) return <Navigate to="/dashboard" replace />

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
      const requested = location.state?.from
      const destination = typeof requested === 'string' && requested.startsWith('/') && !requested.startsWith('//') ? requested : '/dashboard'
      navigate(destination, { replace: true })
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) setError(t('invalidCredentials'))
      else if (cause instanceof ApiError && cause.status === 503) setError(t('authUnavailable'))
      else setError(t('loginError'))
    } finally {
      setPending(false)
    }
  }

  const schoolName = branding ? (i18n.language === 'bn' ? branding.name_bn : branding.name_en) : t('appName')
  return (
    <div className="mx-auto grid max-w-5xl overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-[0_24px_60px_-30px_rgba(15,23,42,0.35)] lg:grid-cols-[minmax(0,1fr)_minmax(24rem,0.95fr)]">
      <section className="login-panel relative hidden flex-col justify-between p-10 lg:flex">
        <div className="flex items-center gap-3"><SchoolMark branding={branding} size="h-11 w-11" /><p className="text-lg font-bold">{schoolName}</p></div>
        <div>
          <h1 className="text-3xl font-bold leading-snug tracking-tight">{t('loginHeading')}</h1>
          <p className="mt-3 max-w-sm text-[15px] leading-7 opacity-90">{t('loginDescription')}</p>
          <ul className="mt-8 space-y-3 text-sm">
            {(['m12LoginPoint1', 'm12LoginPoint2', 'm12LoginPoint3'] as const).map(key => <li key={key} className="flex items-start gap-3"><span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-white/15"><Icon name="check" className="h-4 w-4" /></span><span className="leading-6">{t(key)}</span></li>)}
          </ul>
        </div>
        <p className="text-xs opacity-75">{t('m12PoweredBy')}</p>
      </section>
      <form onSubmit={(event) => void submit(event)} className="p-6 sm:p-10" noValidate={false}>
        <div className="mb-8 flex items-center gap-3 lg:hidden"><SchoolMark branding={branding} /><p className="font-bold text-slate-900">{schoolName}</p></div>
        <p className="text-sm font-semibold text-[var(--brand-ink)]">{t('welcomeBack')}</p>
        <h2 className="mt-1 text-2xl font-bold tracking-tight text-slate-900">{t('signIn')}</h2>
        <p className="mt-1.5 text-sm text-slate-600 lg:hidden">{t('loginDescription')}</p>
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
        <p className="mt-6 text-xs leading-5 text-slate-500">{t('m12LoginHelp')}</p>
      </form>
    </div>
  )
}
