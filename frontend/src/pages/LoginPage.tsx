import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useLocation, useNavigate } from 'react-router'
import { ApiError, type SchoolBranding } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

export function LoginPage({ branding }: { branding?: SchoolBranding }) {
  const { t, i18n } = useTranslation()
  const { user, ready, expired, signIn } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

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

  return (
    <div className="mx-auto grid max-w-5xl gap-8 lg:grid-cols-[minmax(0,1fr)_minmax(22rem,0.9fr)] lg:items-center">
      <section className="max-w-xl">
        <p className="text-sm font-bold uppercase tracking-[0.2em] text-teal-700">{t('welcomeBack')}</p>
        {branding && <p className="brand-text mt-3 text-lg font-semibold">{i18n.language === 'bn' ? branding.name_bn : branding.name_en}</p>}
        <h1 className="mt-4 text-4xl font-bold leading-[1.25] tracking-tight text-slate-900 sm:text-5xl">{t('loginHeading')}</h1>
        <p className="mt-5 text-lg leading-8 text-slate-600">{t('loginDescription')}</p>
      </section>
      <form onSubmit={(event) => void submit(event)} className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm sm:p-8">
        <h2 className="text-2xl font-bold text-slate-900">{t('signIn')}</h2>
        {expired && <p role="status" className="mt-4 rounded-lg bg-amber-50 p-3 text-sm text-amber-900">{t('sessionExpired')}</p>}
        {error && <p role="alert" className="mt-4 rounded-lg bg-rose-50 p-3 text-sm text-rose-800">{error}</p>}
        <label htmlFor="login-email" className="mt-6 block text-sm font-semibold text-slate-700">{t('email')}</label>
        <input id="login-email" name="email" type="email" autoComplete="username" required maxLength={254} value={email}
          onChange={(event) => setEmail(event.target.value)} disabled={pending}
          className="mt-2 w-full rounded-lg border border-slate-300 px-4 py-3 text-slate-900 outline-none focus:border-teal-700 focus:ring-2 focus:ring-teal-100" />
        <label htmlFor="login-password" className="mt-5 block text-sm font-semibold text-slate-700">{t('password')}</label>
        <input id="login-password" name="password" type="password" autoComplete="current-password" required value={password}
          onChange={(event) => setPassword(event.target.value)} disabled={pending}
          className="mt-2 w-full rounded-lg border border-slate-300 px-4 py-3 text-slate-900 outline-none focus:border-teal-700 focus:ring-2 focus:ring-teal-100" />
        <button type="submit" disabled={pending} className="mt-7 w-full rounded-lg bg-teal-700 px-4 py-3 font-semibold text-white hover:bg-teal-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-wait disabled:opacity-60">
          {pending ? t('signingIn') : t('signIn')}
        </button>
      </form>
    </div>
  )
}
