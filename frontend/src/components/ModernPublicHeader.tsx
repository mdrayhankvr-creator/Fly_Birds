import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SchoolBranding } from '../api/client'
import { LanguageSwitch, SchoolMark, ThemeSwitch } from './AppShell'
import { Icon } from './icons'

const items = [
  ['home', 'public-home', 'pdNavHome'],
  ['notices', 'public-notices', 'pdNavNotices'],
  ['results', 'public-results', 'pdNavResults'],
  ['gallery', 'public-gallery', 'pdNavGallery'],
  ['about', 'public-about', 'pdNavAbout'],
  ['contact', 'public-contact', 'pdNavContact'],
] as const

export function ModernPublicHeader({ branding, onLanguage }: { branding?: SchoolBranding; onLanguage: (language: 'en' | 'bn') => void }) {
  const { t, i18n } = useTranslation()
  const [active, setActive] = useState<string>('home')
  const [open, setOpen] = useState(false)
  const name = branding ? (i18n.language === 'bn' ? branding.name_bn || branding.name_en : branding.name_en || branding.name_bn) : ''
  useEffect(() => {
    const update = () => {
      let current = 'home'
      let nearest = Number.NEGATIVE_INFINITY
      for (const [key, id] of items) {
        const node = document.getElementById(id)
        const top = node?.getBoundingClientRect().top ?? Number.POSITIVE_INFINITY
        if (top <= 150 && top > nearest) { current = key; nearest = top }
      }
      setActive(current)
    }
    update()
    window.addEventListener('scroll', update, { passive: true })
    return () => window.removeEventListener('scroll', update)
  }, [])
  useEffect(() => {
    if (!open) return
    const close = (event: KeyboardEvent) => { if (event.key === 'Escape') setOpen(false) }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [open])
  const nav = (mobile: boolean) => items.map(([key, id, label]) => <a key={key} href={`/#${id}`} onClick={() => { setActive(key); setOpen(false) }} aria-current={active === key ? 'page' : undefined}
    className={`pd-nav-link ${mobile ? 'pd-nav-mobile' : ''} ${active === key ? 'pd-nav-active' : ''}`}>{t(label)}</a>)
  return <header className="pd-header sticky top-0 z-30" data-public-header>
    <div className="mx-auto flex h-16 max-w-7xl items-center gap-3 px-4 sm:px-6">
      <a href="/#public-home" className="flex min-w-0 shrink items-center gap-2 rounded-lg focus-visible:outline-2 focus-visible:outline-[var(--public-primary)]" aria-label={name || t('pdNavHome')}>
        <SchoolMark branding={branding} size="h-10 w-10" />
        <span data-public-school-name className="block min-w-0 max-h-12 overflow-hidden text-xs font-extrabold leading-4 text-slate-900 [overflow-wrap:anywhere] sm:max-h-10 sm:max-w-40 sm:text-sm sm:leading-5 xl:max-w-56">{name}</span>
      </a>
      <nav aria-label={t('pdNavigation')} className="hidden min-w-0 flex-1 items-center justify-center gap-1 lg:flex">{nav(false)}</nav>
      <div className="ml-auto flex shrink-0 items-center gap-1 lg:ml-0">
        <div className="hidden lg:block"><LanguageSwitch onChange={onLanguage} /></div>
        <ThemeSwitch />
        <a href="/#public-sign-in" className="pd-login hidden min-h-10 items-center justify-center rounded-full px-5 text-sm font-bold lg:inline-flex">{t('pdLogin')}</a>
        <button type="button" className="pd-menu-button grid h-10 w-10 place-items-center rounded-full lg:hidden" onClick={() => setOpen(v => !v)} aria-expanded={open} aria-controls="pd-mobile-menu" aria-label={t(open ? 'pdCloseMenu' : 'pdMenu')}><Icon name={open ? 'close' : 'menu'} className="h-5 w-5" /></button>
      </div>
    </div>
    {open && <nav id="pd-mobile-menu" aria-label={t('pdNavigation')} className="pd-mobile-menu border-t px-4 pb-5 pt-3 lg:hidden">
      <div className="grid grid-cols-2 gap-1">{nav(true)}</div>
      <div className="mt-4 flex items-center justify-between gap-3"><LanguageSwitch onChange={onLanguage} /><a href="/#public-sign-in" onClick={() => setOpen(false)} className="pd-login inline-flex min-h-10 items-center justify-center rounded-full px-5 text-sm font-bold">{t('pdLogin')}</a></div>
    </nav>}
  </header>
}
