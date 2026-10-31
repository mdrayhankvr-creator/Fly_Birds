import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getSchoolBranding } from '../api/client'

// PasswordSlip is the printable note of a temporary password set by an
// administrator. The password is shown only here, once.
export function PasswordSlip({ name, email, password }: { name: string; email: string; password: string }) {
  const { t, i18n } = useTranslation()
  const branding = useQuery({ queryKey: ['school', 'branding'], queryFn: ({ signal }) => getSchoolBranding(signal), retry: false, staleTime: 60_000 })
  const bn = i18n.language === 'bn'
  const school = branding.data ? (bn && branding.data.name_bn ? branding.data.name_bn : branding.data.name_en) : ''
  const date = new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date())
  const label = 'text-xs font-semibold uppercase tracking-wide text-slate-500'
  return <article className="print-slip rounded-2xl border-2 border-dashed border-slate-400 bg-white p-5 sm:p-6" lang={i18n.language} aria-labelledby="password-slip-title">
    <header className="flex flex-wrap items-start justify-between gap-3 border-b border-slate-200 pb-3">
      <div><p className="font-bold text-slate-900">{school}</p><h3 id="password-slip-title" className="text-base font-semibold text-slate-800">{t('c4SlipTitle')}</h3></div>
      <p className="text-sm text-slate-600">{date}</p>
    </header>
    <dl className="mt-4 grid gap-4 sm:grid-cols-2">
      <div className="min-w-0"><dt className={label}>{t('displayName')}</dt><dd className="mt-0.5 font-semibold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{name || '—'}</dd></div>
      <div className="min-w-0"><dt className={label}>{t('c3LoginEmail')}</dt><dd className="mt-0.5 font-mono font-semibold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{email}</dd></div>
      <div className="min-w-0 sm:col-span-2"><dt className={label}>{t('c4TemporaryPassword')}</dt><dd className="mt-0.5 font-mono text-xl font-bold tracking-wider text-slate-900" data-temporary-password>{password}</dd></div>
    </dl>
    <p className="mt-4 text-sm text-slate-700">{t('c4SlipHelp')}</p>
    <p className="mt-1 text-xs font-semibold text-amber-800 print:hidden">{t('c3PasswordOnce')}</p>
  </article>
}
