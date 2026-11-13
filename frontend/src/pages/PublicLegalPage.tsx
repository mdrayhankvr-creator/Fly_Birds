import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { getPublicPage } from '../api/publicPage'

export function PublicLegalPage({ kind }: { kind: 'privacy' | 'terms' }) {
  const { t, i18n } = useTranslation()
  const page = useQuery({ queryKey: ['school', 'public-page'], queryFn: ({ signal }) => getPublicPage(signal), staleTime: 60_000 })
  const v = page.data
  const bn = i18n.language === 'bn'
  const text = kind === 'privacy' ? (bn ? v?.privacy_policy_bn || v?.privacy_policy_en : v?.privacy_policy_en || v?.privacy_policy_bn)
    : (bn ? v?.terms_bn || v?.terms_en : v?.terms_en || v?.terms_bn)
  return <article className="pd-legal mx-auto max-w-3xl px-4 py-12 sm:px-6 sm:py-20">
    <Link to="/" className="text-sm font-bold text-[var(--public-primary)] hover:underline">← {t('pdNavHome')}</Link>
    <h1 className="mt-6 text-3xl font-extrabold text-slate-900 sm:text-5xl">{t(kind === 'privacy' ? 'pdPrivacy' : 'pdTerms')}</h1>
    {text ? <div className="mt-8 space-y-4">{text.split(/\n{2,}|\n/).filter(Boolean).map((paragraph, i) => <p key={i}>{paragraph}</p>)}</div>
      : page.isPending ? <p className="mt-8">{t('loadingBranding')}</p> : <p className="mt-8">{t('pdLegalEmpty')}</p>}
  </article>
}
