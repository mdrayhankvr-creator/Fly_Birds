import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, verifyStudentCard } from '../api/client'
import { Icon } from '../components/icons'

// Public page opened from a card's QR code. It shows only whether the card is
// genuine and current; the server returns no personal data.
export function VerifyCardPage() {
  const { token = '' } = useParams()
  const { t, i18n } = useTranslation()
  const check = useQuery({ queryKey: ['verify-card', token], queryFn: () => verifyStudentCard(token), retry: false })
  const bn = i18n.language === 'bn'
  const notFound = check.error instanceof ApiError && check.error.status === 404
  const state = check.data?.state
  const tone = state === 'valid' ? 'border-emerald-300 bg-emerald-50 text-emerald-900' : state ? 'border-amber-300 bg-amber-50 text-amber-900' : 'border-rose-300 bg-rose-50 text-rose-900'
  const date = check.data?.valid_until ? new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${check.data.valid_until}T00:00:00Z`)) : ''
  return <div className="mx-auto max-w-md py-8">
    <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm" aria-labelledby="verify-heading">
      <div className="h-2 bg-[var(--brand-primary)]" aria-hidden="true" />
      <div className="p-6 text-center">
        {check.data?.logo_url && <img src={check.data.logo_url} alt="" referrerPolicy="no-referrer" className="mx-auto mb-3 h-14 w-14 object-contain" />}
        {check.data && <p className="text-lg font-bold text-slate-900">{bn && check.data.school_name_bn ? check.data.school_name_bn : check.data.school_name_en}</p>}
        <h1 id="verify-heading" className="mt-1 text-sm font-semibold uppercase tracking-wide text-slate-500">{t('verifyHeading')}</h1>
        <div role="status" aria-live="polite" className="mt-5">
          {check.isPending ? <p className="text-slate-600">{t('verifyChecking')}</p>
            : <div className={`flex items-start gap-3 rounded-xl border p-4 text-left ${tone}`}>
                <Icon name={state === 'valid' ? 'shield' : 'lock'} className="mt-0.5 h-5 w-5 shrink-0" />
                <div>
                  <p className="font-bold">{t(state === 'valid' ? 'verifyValid' : state === 'expired' ? 'verifyExpired' : state === 'inactive' ? 'verifyInactive' : notFound ? 'verifyNotFound' : 'verifyError')}</p>
                  {date && <p className="mt-1 text-sm">{t('idCardValidUntil')}: {date}</p>}
                </div>
              </div>}
        </div>
        <p className="mt-5 text-xs text-slate-500">{t('verifyPrivacy')}</p>
      </div>
    </section>
  </div>
}
