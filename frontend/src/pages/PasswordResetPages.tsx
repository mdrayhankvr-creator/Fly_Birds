import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'
import { ApiError, checkPasswordResetToken, completePasswordReset, getPasswordResetAvailable, requestPasswordReset } from '../api/client'
import { ui } from './attendanceUi'

// Both pages are public and deliberately plain: someone locked out of the
// school's system reads them, often on a phone, sometimes in Bangla.
function Shell({ title, children }: { title: string; children: React.ReactNode }) {
  const { t } = useTranslation()
  return <main className="mx-auto w-full max-w-md px-4 py-10 sm:py-16">
    <div className={`${ui.card} p-6 sm:p-8`}>
      <h1 className="text-2xl font-bold tracking-tight text-slate-900">{title}</h1>
      {children}
      <p className="mt-6 text-sm">
        <Link to="/login" className={ui.link}>{t('fpBackToSignIn')}</Link>
      </p>
    </div>
  </main>
}

// The byte length is what the server checks, so count the same way: a Bangla
// password is longer in bytes than in characters.
function passwordBytes(value: string): number {
  return new TextEncoder().encode(value).length
}

// ForgotPasswordPage asks for the address and reports the same thing however
// the request turned out, apart from being rate limited.
export function ForgotPasswordPage() {
  const { t, i18n } = useTranslation()
  const available = useQuery({ queryKey: ['auth', 'password-reset-available'], queryFn: ({ signal }) => getPasswordResetAvailable(signal), staleTime: Infinity })
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    if (!event.currentTarget.reportValidity()) return
    setPending(true)
    try {
      await requestPasswordReset(email.trim(), i18n.language)
      setSent(true)
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 429) setError(t('fpRateLimited'))
      else if (cause instanceof ApiError && cause.status === 503) setError(t('fpUnavailable'))
      else setError(t('fpError'))
    } finally {
      setPending(false)
    }
  }

  if (available.isFetched && !available.data) {
    return <Shell title={t('fpTitle')}>
      <p className="mt-4 text-sm leading-6 text-slate-700">{t('fpUnavailable')}</p>
    </Shell>
  }
  if (sent) {
    return <Shell title={t('fpTitle')}>
      <p role="status" className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm leading-6 text-emerald-900">{t('fpSent')}</p>
    </Shell>
  }
  return <Shell title={t('fpTitle')}>
    <form onSubmit={(event) => void submit(event)} noValidate={false}>
      <p className="mt-3 text-sm leading-6 text-slate-700">{t('fpIntro')}</p>
      {error && <p role="alert" className="mt-4 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">{error}</p>}
      <label htmlFor="forgot-email" className="mt-5 block text-sm font-semibold text-slate-700">{t('fpEmail')}</label>
      <input id="forgot-email" name="email" type="email" autoComplete="username" autoCapitalize="none" spellCheck={false} required maxLength={254}
        value={email} onChange={(event) => setEmail(event.target.value)} disabled={pending} className={`${ui.field} py-3`} />
      <button type="submit" disabled={pending} className={`${ui.primary} mt-6 w-full py-3`}>{pending ? t('fpSending') : t('fpSend')}</button>
      <p className="mt-4 text-xs leading-5 text-slate-500">{t('fpPrivacyNote')}</p>
      <p className="mt-2 text-xs leading-5 text-slate-500">{t('fpStudentNote')}</p>
    </form>
  </Shell>
}

// ResetPasswordPage is where the emailed link lands. The link is checked
// before the form is shown, so a dead link says so straight away instead of
// after someone has typed a new password twice.
export function ResetPasswordPage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const target = useQuery({
    queryKey: ['auth', 'password-reset', token],
    queryFn: ({ signal }) => checkPasswordResetToken(token, signal),
    enabled: token !== '', retry: false, staleTime: Infinity,
  })
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [show, setShow] = useState(false)
  const [error, setError] = useState('')
  const save = useMutation({
    mutationFn: () => completePasswordReset(token, password),
    onError: (cause) => {
      if (cause instanceof ApiError && cause.code === 'invalid_reset_token') setError(t('fpInvalid'))
      else if (cause instanceof ApiError && cause.code === 'weak_password') setError(t('fpTooShort'))
      else setError(t('fpError'))
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    if (passwordBytes(password) < 12) { setError(t('fpTooShort')); return }
    if (password !== again) { setError(t('fpMismatch')); return }
    save.mutate()
  }

  if (token === '') {
    return <Shell title={t('fpNewTitle')}>
      <p className="mt-4 text-sm leading-6 text-slate-700">{t('fpNoToken')}</p>
      <p className="mt-4 text-sm"><Link to="/forgot-password" className={ui.link}>{t('fpAskAgain')}</Link></p>
    </Shell>
  }
  if (save.isSuccess) {
    return <Shell title={t('fpNewTitle')}>
      <p role="status" className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm leading-6 text-emerald-900">{t('fpDone')}</p>
      <p className="mt-4 text-sm"><Link to="/login" className={ui.link}>{t('fpGoSignIn')}</Link></p>
    </Shell>
  }
  if (target.isPending) {
    return <Shell title={t('fpNewTitle')}><p className="mt-4 text-sm text-slate-700">{t('fpChecking')}</p></Shell>
  }
  if (target.isError) {
    const unavailable = target.error instanceof ApiError && target.error.status === 503
    return <Shell title={t('fpNewTitle')}>
      <p role="alert" className="mt-4 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm leading-6 text-rose-800">{unavailable ? t('fpUnavailable') : t('fpInvalid')}</p>
      {!unavailable && <p className="mt-4 text-sm"><Link to="/forgot-password" className={ui.link}>{t('fpAskAgain')}</Link></p>}
    </Shell>
  }
  return <Shell title={t('fpNewTitle')}>
    <form onSubmit={submit}>
      {target.data?.display_name && <p className="mt-2 text-sm font-semibold text-slate-700">{t('fpNewFor', { name: target.data.display_name })}</p>}
      {error && <p role="alert" className="mt-4 rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">{error}</p>}
      <label htmlFor="reset-password" className="mt-5 block text-sm font-semibold text-slate-700">{t('fpNewPassword')}</label>
      <div className="relative">
        <input id="reset-password" name="new-password" type={show ? 'text' : 'password'} autoComplete="new-password" required minLength={12} maxLength={72}
          value={password} onChange={(event) => setPassword(event.target.value)} disabled={save.isPending} className={`${ui.field} py-3 pr-24`} />
        <button type="button" onClick={() => setShow(v => !v)} aria-pressed={show} aria-controls="reset-password"
          className="absolute inset-y-0 right-1.5 my-auto h-9 rounded-lg px-3 text-xs font-semibold text-slate-600 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">{t(show ? 'm12HidePassword' : 'm12ShowPassword')}</button>
      </div>
      <p className="mt-1.5 text-xs text-slate-500">{t('fpRules')}</p>
      <label htmlFor="reset-password-again" className="mt-5 block text-sm font-semibold text-slate-700">{t('fpConfirmPassword')}</label>
      <input id="reset-password-again" name="new-password-again" type={show ? 'text' : 'password'} autoComplete="new-password" required minLength={12} maxLength={72}
        value={again} onChange={(event) => setAgain(event.target.value)} disabled={save.isPending} className={`${ui.field} py-3`} />
      <button type="submit" disabled={save.isPending} className={`${ui.primary} mt-6 w-full py-3`}>{save.isPending ? t('fpSaving') : t('fpSave')}</button>
    </form>
  </Shell>
}
