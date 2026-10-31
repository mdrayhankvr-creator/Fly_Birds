import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { verifyResult } from '../api/documents'
import { Icon } from '../components/icons'

// Public page opened from a marksheet's QR code. It shows whether the result
// is genuine and current, and the document code to compare with the paper;
// the server returns no name, mark or grade.
export function VerifyResultPage() {
  const { token = '' } = useParams()
  const { t, i18n } = useTranslation()
  const check = useQuery({ queryKey: ['verify-result', token], queryFn: () => verifyResult(token), retry: false })
  const bn = i18n.language === 'bn'
  const notFound = check.error instanceof ApiError && check.error.status === 404
  const v = check.data
  const tone = v?.state === 'current' ? 'border-emerald-300 bg-emerald-50 text-emerald-900' : v ? 'border-amber-300 bg-amber-50 text-amber-900' : 'border-rose-300 bg-rose-50 text-rose-900'
  const date = v?.published_at ? new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date(v.published_at)) : ''
  const num = (n: number) => new Intl.NumberFormat(bn ? 'bn-BD' : 'en-GB').format(n)
  return <div className="mx-auto max-w-md py-8">
    <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm" aria-labelledby="verify-result-heading">
      <div className="h-2 bg-[var(--brand-primary)]" aria-hidden="true" />
      <div className="p-6 text-center">
        {v?.logo_url && <img src={v.logo_url} alt="" referrerPolicy="no-referrer" className="mx-auto mb-3 h-14 w-14 object-contain" />}
        {v && <p className="text-lg font-bold text-slate-900">{bn && v.school_name_bn ? v.school_name_bn : v.school_name_en}</p>}
        <h1 id="verify-result-heading" className="mt-1 text-sm font-semibold uppercase tracking-wide text-slate-500">{t('e7VerifyHeading')}</h1>
        <div role="status" aria-live="polite" className="mt-5">
          {check.isPending ? <p className="text-slate-600">{t('e7VerifyChecking')}</p>
            : <div className={`flex items-start gap-3 rounded-xl border p-4 text-left ${tone}`}>
              <Icon name={v?.state === 'current' ? 'shield' : 'alert'} className="mt-0.5 h-5 w-5 shrink-0" />
              <div className="min-w-0">
                <p className="font-bold">{t(v ? `e7Verify_${v.state}` : notFound ? 'e7VerifyNotFound' : 'e7VerifyError')}</p>
                {v && <dl className="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-sm">
                  <dt className="opacity-80">{t('e7VerifyResult')}</dt><dd className="font-semibold">{v.scope === 'annual' ? t('e7AnnualOf', { session: v.session_name }) : v.title}</dd>
                  <dt className="opacity-80">{t('e7VerifyClass')}</dt><dd>{v.class_name} · {v.session_name}</dd>
                  <dt className="opacity-80">{t('e7VerifyVersion')}</dt><dd>{num(v.version)} · {date}</dd>
                  <dt className="opacity-80">{t('e7VerifyCode')}</dt><dd className="font-mono font-semibold">{v.code}</dd>
                </dl>}
              </div>
            </div>}
        </div>
        <p className="mt-5 text-xs text-slate-500">{t('e7VerifyPrivacy')}</p>
      </div>
    </section>
  </div>
}
