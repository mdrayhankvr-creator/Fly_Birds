import { createPortal } from 'react-dom'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getSchoolBranding } from '../api/client'
import type { StudentLogin } from '../api/studentLogins'
import { Icon } from '../components/icons'
import { ui } from './attendanceUi'
import { slipDate } from './slipDate'

// Login slips (checkpoint 5): the sign-in ID and the temporary password a
// student needs for the first sign-in. Labels are in English and Bangla on
// every slip, so one print serves every family. Passwords are shown only on
// the screen that created them; nothing here stores them.

function useSchoolNames() {
  const branding = useQuery({ queryKey: ['school', 'branding'], queryFn: ({ signal }) => getSchoolBranding(signal), retry: false, staleTime: 60_000 })
  return { en: branding.data?.name_en ?? '', bn: branding.data?.name_bn ?? '' }
}

function LoginSlip({ login, school }: { login: StudentLogin; school: { en: string; bn: string } }) {
  const { i18n } = useTranslation()
  const en = i18n.getFixedT('en'), bn = i18n.getFixedT('bn')
  const both = (key: string, options?: Record<string, unknown>) => <>{en(key, options)} <span lang="bn" className="font-normal">· {bn(key, options)}</span></>
  const site = window.location.host
  const label = 'text-[11px] font-semibold uppercase tracking-wide text-slate-500'
  const place = [login.class_name, login.section_name, login.roll_no == null ? '' : `${en('rollNo')} ${login.roll_no}`].filter(Boolean).join(' · ')
  return <article className="login-slip min-w-0 rounded-xl border-2 border-dashed border-slate-400 bg-white p-4 text-slate-900" aria-label={en('c5SlipFor', { name: login.student_name })}>
    <header className="border-b border-slate-200 pb-2">
      <p className="text-sm font-bold" style={{ overflowWrap: 'anywhere' }}>{school.en}{school.bn && <span lang="bn" className="font-semibold"> · {school.bn}</span>}</p>
      <p className="text-xs font-semibold text-slate-600">{both('c5SlipTitle')}</p>
    </header>
    <p className="mt-2 font-bold leading-tight" style={{ overflowWrap: 'anywhere' }}>{login.student_name}{login.name_bn && <span lang="bn" className="block text-sm font-semibold text-slate-700">{login.name_bn}</span>}</p>
    <p className="text-xs text-slate-600">{place}</p>
    <dl className="mt-3 grid grid-cols-2 gap-3">
      <div className="min-w-0"><dt className={label}>{both('c5SignInId')}</dt><dd className="mt-0.5 font-mono text-base font-bold" style={{ overflowWrap: 'anywhere' }} data-login-id>{login.login_id}</dd></div>
      <div className="min-w-0"><dt className={label}>{both('c4TemporaryPassword')}</dt><dd className="mt-0.5 font-mono text-base font-bold tracking-wider" data-temporary-password>{login.initial_password}</dd></div>
      {login.password_expires_at && <div className="col-span-2 min-w-0"><dt className={label}>{both('c5PasswordUseBy')}</dt><dd className="mt-0.5 text-sm font-semibold" data-password-expires>{slipDate(login.password_expires_at, 'en')} <span lang="bn" className="font-normal">· {slipDate(login.password_expires_at, 'bn')}</span></dd></div>}
    </dl>
    <p className="mt-3 text-xs leading-5 text-slate-700">{en('c5SlipSteps', { site })}</p>
    <p lang="bn" className="text-xs leading-5 text-slate-700">{bn('c5SlipSteps', { site })}</p>
    {login.password_expires_at && <p className="mt-1 text-xs leading-5 text-slate-700">{en('c5PasswordExpiryHelp')} <span lang="bn">{bn('c5PasswordExpiryHelp')}</span></p>}
  </article>
}

// LoginSlipSheet shows the slips with a print button. On paper they are laid
// out two across, four to an A4 page.
export function LoginSlipSheet({ logins }: { logins: StudentLogin[] }) {
  const { t } = useTranslation()
  const school = useSchoolNames()
  if (!logins.length) return null
  return <section aria-labelledby="login-slips-title" className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h3 id="login-slips-title" className="text-base font-bold text-slate-900">{t('c5SlipsReady', { count: logins.length })}</h3>
      <button type="button" className={ui.primary} onClick={() => window.print()}><Icon name="print" className="h-4 w-4" />{t(logins.length === 1 ? 'c5PrintSlip' : 'c5PrintSlips')}</button>
    </div>
    <p className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm font-semibold text-amber-900">{t('c5PasswordsOnce')}</p>
    <div className="grid gap-3 md:grid-cols-2">{logins.map(l => <LoginSlip key={l.user_id} login={l} school={school} />)}</div>
    {createPortal(<div className="login-slips-print" aria-hidden="true">{logins.map(l => <LoginSlip key={l.user_id} login={l} school={school} />)}</div>, document.body)}
  </section>
}
