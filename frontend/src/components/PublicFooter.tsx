import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { SchoolBranding } from '../api/client'
import { getPublicPage } from '../api/publicPage'
import { SchoolMark } from './AppShell'
import { Icon } from './icons'

function useIdle() {
  const [ready, setReady] = useState(false)
  useEffect(() => {
    const idle = (window as Window & { requestIdleCallback?: (cb: () => void, opts?: { timeout: number }) => number }).requestIdleCallback
    if (idle) { idle(() => setReady(true), { timeout: 1500 }); return }
    const timer = window.setTimeout(() => setReady(true), 300)
    return () => window.clearTimeout(timer)
  }, [])
  return ready
}

export function PublicFooter({ branding }: { branding?: SchoolBranding }) {
  const { t, i18n } = useTranslation()
  const bn = i18n.language === 'bn'
  const ready = useIdle()
  const { data: v } = useQuery({ queryKey: ['school', 'public-page'], queryFn: ({ signal }) => getPublicPage(signal), enabled: ready, staleTime: 60_000, retry: 1 })
  const name = branding ? (bn ? branding.name_bn || branding.name_en : branding.name_en || branding.name_bn) : ''
  const tagline = branding ? (bn ? branding.tagline_bn || branding.tagline_en : branding.tagline_en || branding.tagline_bn) : ''
  const description = v ? (bn ? v.footer_description_bn || v.footer_description_en : v.footer_description_en || v.footer_description_bn) : ''
  const address = v?.contact ? (bn ? v.contact.address_bn || v.contact.address_en : v.contact.address_en || v.contact.address_bn) : ''
  const phone = v?.contact?.phone?.trim() || ''
  const email = v?.contact?.email?.trim() || ''
  const modern = !v || v.design_style === 'modern_bold'
  const year = new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US', { useGrouping: false }).format(Number(new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Dhaka', year: 'numeric' }).format(new Date())))
  const quickLinks = [
    { label: t('pdNavHome'), href: '/' },
    { label: t('pdNavAbout'), href: '/#public-about', show: !!v?.about || (modern && !!(v?.activities_en || v?.activities_bn)) },
    { label: t('pdNavNotices'), href: '/#public-notices', show: !!v?.notices.length },
    { label: t('pdNavResults'), href: '/#public-results', show: !!v && modern },
    { label: t('pdNavGallery'), href: '/#public-gallery', show: !!v?.gallery.length },
    { label: t('pdNavContact'), href: '/#public-contact', show: !!(address || email || v?.contact?.map_url) },
  ].filter(item => item.show !== false)
  const resources = [
    { label: t('ppFooterSignIn'), href: '/login', show: true },
    { label: t('pdPrivacy'), href: '/privacy-policy', show: !!(v?.privacy_policy_en || v?.privacy_policy_bn) },
    { label: t('pdTerms'), href: '/terms', show: !!(v?.terms_en || v?.terms_bn) },
  ].filter(item => item.show)
  const socials = v ? [
    { name: 'Facebook', symbol: 'f', url: v.social_facebook },
    { name: 'YouTube', symbol: '▶', url: v.social_youtube },
    { name: 'WhatsApp', symbol: 'W', url: v.social_whatsapp },
    { name: 'Messenger', symbol: 'M', url: v.social_messenger },
  ].filter(item => item.url) : []

  return <footer className="public-footer school-footer" data-public-footer data-public-style={v?.design_style || 'modern_bold'}>
    <div className="school-footer-main mx-auto grid max-w-6xl gap-x-10 gap-y-10 px-4 py-14 sm:grid-cols-2 sm:px-6 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,.8fr)_minmax(0,.8fr)_minmax(0,1.2fr)] lg:py-16">
      <section className="min-w-0" aria-label={t('ppFooter')}>
        <div className="flex min-w-0 items-center gap-3"><SchoolMark branding={branding} size="h-12 w-12" />{name && <h2 className="school-footer-name min-w-0 text-lg font-extrabold leading-snug">{name}</h2>}</div>
        <p className="school-footer-tagline mt-5 text-sm font-semibold">{tagline || t('pfTagline')}</p>
        <p className="school-footer-description mt-2 text-sm leading-7">{description || t('pfDescription')}</p>
        {v?.about?.eiin && <p className="mt-3 text-xs">{t('ppFooterEiin', { eiin: v.about.eiin })}</p>}
        {socials.length > 0 && <ul className="school-footer-socials mt-5 flex flex-wrap gap-2">{socials.map(social => <li key={social.name}><a href={social.url} target="_blank" rel="noopener noreferrer" aria-label={social.name} title={social.name}>{social.symbol}</a></li>)}</ul>}
      </section>
      <nav aria-label={t('pdQuickLinks')}><h2 className="school-footer-heading">{t('pdQuickLinks')}</h2><ul className="school-footer-links">{quickLinks.map(item => <li key={item.href}><a href={item.href}>{item.label}</a></li>)}</ul></nav>
      <nav aria-label={t('pfResources')}><h2 className="school-footer-heading">{t('pfResources')}</h2><ul className="school-footer-links">{resources.map(item => <li key={item.href}><a href={item.href}>{item.label}</a></li>)}</ul></nav>
      {(email || phone || address || v?.contact?.map_url) && <section className="min-w-0" aria-label={t('ppFooterContact')}><h2 className="school-footer-heading">{t('ppFooterContact')}</h2>
        <ul className="school-footer-contact">
          {email && <li><Icon name="mail" aria-hidden="true" /><a href={`mailto:${email}`}>{email}</a></li>}
          {phone && <li><Icon name="phone" aria-hidden="true" /><a href={`tel:${phone.replace(/[^+\d]/g, '')}`}>{phone}</a></li>}
          {address && <li><Icon name="home" aria-hidden="true" /><span className="whitespace-pre-line">{address}</span></li>}
          {v?.contact?.map_url && <li><Icon name="globe" aria-hidden="true" /><a href={v.contact.map_url} target="_blank" rel="noopener noreferrer">{t('c5OpenMap')}</a></li>}
        </ul>
      </section>}
    </div>
    <div className="school-footer-bottom"><div className="mx-auto flex max-w-6xl flex-col gap-2 px-4 py-5 text-xs sm:flex-row sm:items-center sm:justify-between sm:px-6">
      {name && <p>© {year} {name}. {t('pdRights')}</p>}
      <p>{t('pfCredit')}</p>
    </div></div>
  </footer>
}
