import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { verifyAdmitCard } from '../api/marksheetAdmit'
import { Icon } from '../components/icons'

// Public page opened from an admit card's QR code: whether the card was
// printed from the routine now in force. The server returns no name or
// identifier.
export function VerifyAdmitPage() {
  const { token = '' } = useParams()
  const { t, i18n } = useTranslation()
  const check = useQuery({ queryKey: ['verify-admit', token], queryFn: () => verifyAdmitCard(token), retry: false })
  const bn = i18n.language === 'bn'
  const notFound = check.error instanceof ApiError && check.error.status === 404
  const v = check.data
  const tone = v?.status === 'current' ? 'border-emerald-300 bg-emerald-50 text-emerald-900' : v ? 'border-amber-300 bg-amber-50 text-amber-900' : 'border-rose-300 bg-rose-50 text-rose-900'
  const date = (iso: string) => iso ? new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date(iso)) : ''
  const num = (n: number | string) => bn ? String(n).replace(/[0-9]/g, d => '০১২৩৪৫৬৭৮৯'[Number(d)]) : String(n)
  return <div className="mx-auto max-w-md py-8">
    <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm" aria-labelledby="verify-admit-heading">
      <div className="h-2 bg-[var(--brand-primary)]" aria-hidden="true" />
      <div className="p-6 text-center">
        {v && <p className="text-lg font-bold text-slate-900">{bn && v.school_name_bn ? v.school_name_bn : v.school_name}</p>}
        <h1 id="verify-admit-heading" className="mt-1 text-sm font-semibold uppercase tracking-wide text-slate-600">{t('msVerifyTitle')}</h1>
        <div role="status" aria-live="polite" className="mt-5">
          {check.isPending ? <p className="text-slate-600">{t('msVerifyChecking')}</p>
            : <div className={`flex items-start gap-3 rounded-xl border p-4 text-left ${tone}`}>
              <Icon name={v?.status === 'current' ? 'shield' : 'alert'} className="mt-0.5 h-5 w-5 shrink-0" />
              <div className="min-w-0">
                <p className="font-bold">{v ? t(v.status === 'current' ? 'msVerifyCurrent' : 'msVerifySuperseded') : notFound ? t('msVerifyNotFound') : t('msLoadError')}</p>
                {v && <p className="mt-0.5 text-sm">{t(v.status === 'current' ? 'msVerifyCurrentHint' : 'msVerifySupersededHint')}</p>}
                {v && <dl className="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-sm">
                  <dt className="opacity-80">{t('msVerifyTerm')}</dt><dd className="font-semibold">{v.term} · {num(v.session)}</dd>
                  <dt className="opacity-80">{t('msVerifyClass')}</dt><dd>{v.class} · {v.section}</dd>
                  <dt className="opacity-80">{t('msVerifyVersion')}</dt><dd>{num(v.version)} · {date(v.published_at)}</dd>
                  <dt className="opacity-80">{t('msVerifyIssued')}</dt><dd>{date(v.issued_at)}</dd>
                </dl>}
              </div>
            </div>}
        </div>
        <p className="mt-5 text-xs text-slate-600">{t('msVerifyPrivacy')}</p>
      </div>
    </section>
  </div>
}
