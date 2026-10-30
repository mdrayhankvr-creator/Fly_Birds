import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getSchoolBranding } from '../api/client'
import type { AdmissionResult } from '../api/admission'
import { useNumber } from './attendanceUi'

// AdmissionSlip is the printable record of an admission. When a login was
// created it carries the initial password, which is shown only here, once.
export function AdmissionSlip({ result }: { result: AdmissionResult }) {
  const { t, i18n } = useTranslation()
  const num = useNumber()
  const branding = useQuery({ queryKey: ['school', 'branding'], queryFn: ({ signal }) => getSchoolBranding(signal), retry: false, staleTime: 60_000 })
  const s = result.student
  const bn = i18n.language === 'bn'
  const school = branding.data ? (bn && branding.data.name_bn ? branding.data.name_bn : branding.data.name_en) : ''
  const date = new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date())
  const row = (label: string, value: ReactNode) => <div className="min-w-0"><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt><dd className="mt-0.5 font-semibold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{value || '—'}</dd></div>
  return <article className="admission-slip relative overflow-hidden rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8" lang={i18n.language} aria-labelledby="admission-slip-title">
    <header className="flex flex-wrap items-center gap-4 border-b border-slate-200 pb-4">
      {branding.data?.logo_url && <img src={branding.data.logo_url} alt="" className="h-14 w-14 object-contain" />}
      <div className="min-w-0 flex-1"><p className="text-base font-bold text-slate-900">{school}</p><h2 id="admission-slip-title" className="text-lg font-semibold text-slate-800">{t('c3SlipTitle')}</h2></div>
      <div className="text-right text-sm"><p className="font-mono text-base font-bold">{s.admission_no}</p><p className="text-slate-600">{date}</p></div>
    </header>
    <dl className="mt-5 grid gap-4 sm:grid-cols-2">
      {row(t('studentNameEN'), s.name_en)}
      {row(t('studentNameBN'), s.name_bn && <span lang="bn">{s.name_bn}</span>)}
      {row(t('sessionLabel'), s.session_name)}
      {row(t('c3ClassSectionRoll'), `${s.class_name} · ${s.section_name}${s.roll_no != null ? ` · ${t('p2HistoryRoll', { roll: num(s.roll_no) })}` : ''}`)}
      {s.date_of_birth && row(t('dateOfBirth'), s.date_of_birth)}
    </dl>
    {result.guardians.length > 0 && <section className="mt-5"><h3 className="text-sm font-bold text-slate-900">{t('guardiansForStudent')}</h3>
      <ul className="mt-2 divide-y divide-slate-100 rounded-xl border border-slate-200">{result.guardians.map(g => <li key={g.guardian_id} className="flex flex-wrap justify-between gap-2 px-3 py-2 text-sm">
        <span><strong>{bn && g.guardian_name_bn ? g.guardian_name_bn : g.guardian_name}</strong> · {g.relationship}{g.is_primary && <> · {t('primaryGuardian')}</>}</span><span className="tabular-nums">{g.phone}</span></li>)}</ul>
    </section>}
    {result.login && <section className="mt-5 rounded-xl border-2 border-dashed border-slate-400 p-4" aria-labelledby="admission-slip-login">
      <h3 id="admission-slip-login" className="text-sm font-bold text-slate-900">{t('c3SlipLogin')}</h3>
      <dl className="mt-3 grid gap-4 sm:grid-cols-2">
        {row(t('c3LoginEmail'), <span className="font-mono">{result.login.email}</span>)}
        {result.login.initial_password ? row(t('c3InitialPassword'), <span className="font-mono text-lg tracking-wider" data-initial-password>{result.login.initial_password}</span>)
          : row(t('c3InitialPassword'), <span className="font-normal text-slate-700">{t('c3PasswordNotAgain')}</span>)}
      </dl>
      {result.login.initial_password ? <>
        <p className="mt-3 text-sm text-slate-700">{t('c3SlipLoginHelp')}</p>
        <p className="mt-1 text-xs font-semibold text-amber-800 print:hidden">{t('c3PasswordOnce')}</p>
      </> : <p className="mt-3 text-sm print:hidden"><Link className="font-semibold text-[var(--brand-ink)] underline underline-offset-4" to={`/admin/users?search=${encodeURIComponent(result.login.email)}`}>{t('c3OpenLoginInUsers')}</Link></p>}
    </section>}
    <footer className="mt-8 grid grid-cols-2 gap-8 text-xs text-slate-500"><p className="border-t border-slate-300 pt-2">{t('c3SlipGuardianSign')}</p><p className="border-t border-slate-300 pt-2 text-right">{t('c3SlipOfficeSign')}</p></footer>
  </article>
}
