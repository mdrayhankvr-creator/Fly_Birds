import { useEffect, useState, type MouseEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate } from 'react-router'
import type { SchoolBranding } from '../api/client'
import { getPublicPage } from '../api/publicPage'
import { SchoolMark } from './AppShell'
import { Icon } from './icons'

// The footer of every public page: who the school is, how to reach it, a few
// links, the copyright and a small credit. Anything the school has not filled
// in is left out. The public page is read after the first paint, so the
// footer never slows the sign-in form; it shares its cache with the sections
// on the landing page.

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

const link = 'rounded font-semibold text-[var(--brand-ink)] underline decoration-1 underline-offset-4 hover:decoration-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)]'

export function PublicFooter({ branding }: { branding?: SchoolBranding }) {
  const { t, i18n } = useTranslation()
  const bn = i18n.language === 'bn'
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const ready = useIdle()
  const page = useQuery({ queryKey: ['school', 'public-page'], queryFn: ({ signal }) => getPublicPage(signal), enabled: ready, staleTime: 60_000, retry: 1 })
  const v = page.data
  const name = branding ? (bn ? branding.name_bn || branding.name_en : branding.name_en || branding.name_bn) : ''
  const address = v?.contact ? (bn ? v.contact.address_bn || v.contact.address_en : v.contact.address_en || v.contact.address_bn) : ''
  const phone = v?.contact?.phone ?? ''
  const email = v?.contact?.email ?? ''
  const eiin = v?.about?.eiin ?? ''
  const year = new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US', { useGrouping: false }).format(Number(new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Dhaka', year: 'numeric' }).format(new Date())))
  const landing = pathname === '/' || pathname === '/login'

  // On the landing page the sign-in form and the sections are already there.
  function signIn(event: MouseEvent) {
    const field = document.getElementById('login-email') as HTMLInputElement | null
    if (!field) return
    event.preventDefault()
    window.scrollTo({ top: 0, behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
    field.focus({ preventScroll: true })
  }
  function section(id: string) {
    return (event: MouseEvent) => {
      event.preventDefault()
      const target = landing ? document.getElementById(id) : null
      if (target) {
        target.scrollIntoView({ behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth', block: 'start' })
        window.history.replaceState(window.history.state, '', `#${id}`)
        return
      }
      navigate({ pathname: '/', hash: `#${id}` })
    }
  }

  const links: { key: string; label: string; to: string; onClick: (e: MouseEvent) => void }[] = [
    { key: 'signin', label: t('ppFooterSignIn'), to: '/login', onClick: signIn },
  ]
  if (v?.notices.length) links.push({ key: 'notices', label: t('ppFooterNotices'), to: '/#public-notices', onClick: section('public-notices') })
  if (v?.contact) links.push({ key: 'contact', label: t('ppFooterContactLink'), to: '/#public-contact', onClick: section('public-contact') })

  return <footer className="public-footer border-t border-slate-200 bg-white" data-public-footer>
    <div className="mx-auto grid max-w-6xl gap-8 px-4 py-10 sm:px-6 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,0.8fr)]">
      <section aria-label={t('ppFooter')} className="min-w-0">
        {name && <div className="flex min-w-0 items-center gap-3">
          <SchoolMark branding={branding} size="h-11 w-11" />
          <p className="min-w-0 text-base font-bold leading-snug text-slate-900" style={{ overflowWrap: 'anywhere' }}>{name}</p>
        </div>}
        {(address || eiin) && <div className="mt-3 space-y-1 text-sm leading-6 text-slate-700">
          {address && <p className="whitespace-pre-line" style={{ overflowWrap: 'anywhere' }}>{address}</p>}
          {eiin && <p className="tabular-nums">{t('ppFooterEiin', { eiin })}</p>}
        </div>}
      </section>

      {(phone || email) && <section aria-labelledby="footer-contact" className="min-w-0">
        <h2 id="footer-contact" className="text-xs font-bold uppercase tracking-wide text-slate-600">{t('ppFooterContact')}</h2>
        <ul className="mt-3 space-y-2 text-sm">
          {phone && <li className="flex min-w-0 items-center gap-2"><Icon name="phone" className="h-4 w-4 shrink-0 text-slate-500" /><a className={link} href={`tel:${phone.replace(/[^+0-9]/g, '')}`}>{phone}</a></li>}
          {email && <li className="flex min-w-0 items-center gap-2"><Icon name="mail" className="h-4 w-4 shrink-0 text-slate-500" /><a className={`${link} min-w-0`} style={{ overflowWrap: 'anywhere' }} href={`mailto:${email}`}>{email}</a></li>}
        </ul>
      </section>}

      <nav aria-labelledby="footer-links" className="min-w-0">
        <h2 id="footer-links" className="text-xs font-bold uppercase tracking-wide text-slate-600">{t('ppFooterLinks')}</h2>
        <ul className="mt-3 space-y-2 text-sm">
          {links.map(l => <li key={l.key}><Link className={`${link} inline-flex min-h-[24px] items-center`} to={l.to} onClick={l.onClick}>{l.label}</Link></li>)}
        </ul>
      </nav>
    </div>
    <div className="border-t border-slate-200">
      <div className="mx-auto flex max-w-6xl flex-col gap-1 px-4 py-4 text-xs text-slate-600 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        {name && <p>{t('ppFooterCopyright', { year, school: name })}</p>}
        <p className="text-slate-500">{t('ppFooterPowered')}</p>
      </div>
    </div>
  </footer>
}
