import { lazy, Suspense, useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Navigate, useLocation, useNavigate } from 'react-router'
import { getSchoolSlides, slideImageUrl, type SchoolBranding, type SchoolSlide } from '../api/client'
import { getPublicPage } from '../api/publicPage'
import { useAuth } from '../auth/AuthProvider'
import { Icon } from '../components/icons'
import { SignInForm } from './LoginPage'

const SLIDE_MS = 7000
// The public school sections below the hero load as their own chunk.
const PublicSections = lazy(() => import('./PublicSections').then(m => ({ default: m.PublicSections })))

// True once the browser is idle after the first paint, so the sections below
// the fold never compete with the first screen on a slow connection.
function useAfterFirstPaint() {
  const [ready, setReady] = useState(false)
  useEffect(() => {
    const idle = (window as Window & { requestIdleCallback?: (cb: () => void, opts?: { timeout: number }) => number }).requestIdleCallback
    if (idle) { idle(() => setReady(true), { timeout: 1500 }); return }
    const timer = window.setTimeout(() => setReady(true), 300)
    return () => window.clearTimeout(timer)
  }, [])
  return ready
}

function useReducedMotion() {
  const query = '(prefers-reduced-motion: reduce)'
  const [reduced, setReduced] = useState(() => typeof window !== 'undefined' && window.matchMedia?.(query).matches)
  useEffect(() => {
    const media = window.matchMedia?.(query)
    if (!media) return
    const change = () => setReduced(media.matches)
    media.addEventListener('change', change)
    return () => media.removeEventListener('change', change)
  }, [])
  return reduced
}

// The school's photos cross-fade behind its identity.
// Only the first photo loads up front; each later one loads just before it
// is shown, so a slow connection gets a picture quickly.
function Slideshow({ slides, index, reduced }: { slides: SchoolSlide[]; index: number; reduced: boolean }) {
  const [wanted, setWanted] = useState<Set<number>>(() => new Set([0]))
  const next = (index + 1) % slides.length
  useEffect(() => { setWanted(w => w.has(index) && w.has(next) ? w : new Set([...w, index, next])) }, [index, next])
  return <div className="absolute inset-0" aria-hidden="true">
    {slides.map((s, i) => wanted.has(i) && <img key={s.id} src={slideImageUrl(s)} alt="" width={s.width} height={s.height}
      loading={i === 0 ? 'eager' : 'lazy'} decoding="async" {...(i === 0 ? { fetchPriority: 'high' } : {})}
      className={`absolute inset-0 h-full w-full object-cover ${reduced ? '' : 'transition-opacity duration-[1400ms] ease-in-out'} ${i === index ? 'opacity-100' : 'opacity-0'}`} />)}
  </div>
}

type ControlsProps = {
  slides: SchoolSlide[]; index: number; onIndex: (i: number) => void; playing: boolean; onPlaying: (p: boolean) => void
  reduced: boolean; onHold: (hold: boolean) => void; className: string; showCaption?: boolean; placement: 'phone' | 'desktop'
}

// Previous/next, one dot per photo and pause. Arrow keys move between photos
// while the group has focus; hovering or focusing it pauses the slideshow.
function SlideControls({ slides, index, onIndex, playing, onPlaying, reduced, onHold, className, showCaption = false, placement }: ControlsProps) {
  const { t, i18n } = useTranslation()
  const go = (i: number) => onIndex((i + slides.length) % slides.length)
  function key(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'ArrowRight') { event.preventDefault(); go(index + 1) }
    else if (event.key === 'ArrowLeft') { event.preventDefault(); go(index - 1) }
  }
  const current = slides[index]
  const caption = i18n.language === 'bn' ? current.caption_bn || current.caption_en : current.caption_en || current.caption_bn
  const button = 'grid h-8 w-8 place-items-center rounded-full text-white hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-white'
  return <div data-slide-controls={placement} className={`${className} z-10 items-center gap-3`} onMouseEnter={() => onHold(true)} onMouseLeave={() => onHold(false)}
    onFocus={() => onHold(true)} onBlur={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) onHold(false) }}>
    <div role="group" aria-roledescription={t('p3Carousel')} aria-label={t('p3Slides')} tabIndex={0} onKeyDown={key}
      className="flex shrink-0 items-center gap-1 rounded-full bg-slate-950/45 px-1.5 py-1 backdrop-blur-sm focus-visible:outline-2 focus-visible:outline-white">
      <button type="button" onClick={() => go(index - 1)} aria-label={t('p3SlidePrev')} className={button}><Icon name="chevronLeft" className="h-4 w-4" /></button>
      {slides.map((s, i) => <button key={s.id} type="button" onClick={() => onIndex(i)} aria-label={t('p3SlideGo', { n: i + 1, total: slides.length })} aria-current={i === index ? 'true' : undefined}
        className="grid h-8 w-6 place-items-center rounded-full focus-visible:outline-2 focus-visible:outline-white">
        <span className={`block h-2 rounded-full transition-all ${i === index ? 'w-5 bg-[#ffffff]' : 'w-2 bg-[#ffffff]/55'}`} /></button>)}
      <button type="button" onClick={() => go(index + 1)} aria-label={t('p3SlideNext')} className={button}><Icon name="chevronRight" className="h-4 w-4" /></button>
      {!reduced && <button type="button" onClick={() => onPlaying(!playing)} aria-label={t(playing ? 'p3SlidePause' : 'p3SlidePlay')} aria-pressed={!playing} className={button}>
        {playing ? <span aria-hidden="true" className="flex gap-[3px]"><span className="h-3 w-[3px] rounded bg-[#ffffff]" /><span className="h-3 w-[3px] rounded bg-[#ffffff]" /></span>
          : <span aria-hidden="true" className="ml-0.5 h-0 w-0 border-y-[6px] border-l-[9px] border-y-transparent border-l-white" />}</button>}
    </div>
    {showCaption && caption && <p className="min-w-0 truncate text-sm font-medium text-white/90">{caption}</p>}
  </div>
}

function SlideDots({ slides, index, onIndex }: { slides: SchoolSlide[]; index: number; onIndex: (index: number) => void }) {
  const { t } = useTranslation()
  return <div data-slide-dots role="group" aria-label={t('p3Slides')} className="absolute inset-x-0 bottom-5 z-10 flex justify-center gap-1.5 sm:bottom-8">
    {slides.map((slide, i) => <button key={slide.id} type="button" onClick={() => onIndex(i)}
      aria-label={t('p3SlideGo', { n: i + 1, total: slides.length })} aria-current={i === index ? 'true' : undefined}
      className="grid h-9 w-9 place-items-center rounded-full focus-visible:outline-2 focus-visible:outline-white">
      <span className={`block h-2 rounded-full bg-white shadow-sm ${i === index ? 'w-5' : 'w-2 opacity-60'}`} />
    </button>)}
  </div>
}

function SignInOverlay() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const dialog = useRef<HTMLDivElement>(null)
  const returnTo = location.state?.modalReturnTo
  const close = useCallback(() => {
    if (typeof returnTo === 'string' && returnTo.startsWith('/') && !returnTo.startsWith('//')) navigate(-1)
    else navigate('/', { replace: true })
  }, [navigate, returnTo])

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const oldOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    dialog.current?.querySelector<HTMLInputElement>('#login-email')?.focus()
    return () => { document.body.style.overflow = oldOverflow; previous?.focus() }
  }, [])

  function key(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') { event.preventDefault(); close(); return }
    if (event.key !== 'Tab') return
    const focusable = [...(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), a[href], [tabindex]:not([tabindex="-1"])') ?? [])]
      .filter(element => element.getClientRects().length > 0)
    if (!focusable.length) return
    const first = focusable[0], last = focusable[focusable.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
  }

  return <div data-sign-in-overlay className="fixed inset-0 z-50 flex items-end justify-center sm:items-center sm:p-4" onKeyDown={key}>
    <button type="button" aria-label={t('pdCloseSignIn')} className="absolute inset-0 cursor-default bg-slate-950/60 backdrop-blur-[3px]" onClick={close} tabIndex={-1} />
    <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby="sign-in-title"
      className="pd-sign-in-sheet relative z-10 w-full max-w-md overflow-y-auto rounded-t-3xl bg-white p-3 shadow-2xl sm:rounded-2xl">
      <button type="button" onClick={close} aria-label={t('pdCloseSignIn')}
        className="absolute right-5 top-5 z-10 grid h-9 w-9 place-items-center rounded-full bg-slate-100 text-slate-700 hover:bg-slate-200 focus-visible:outline-2 focus-visible:outline-[var(--public-primary)]">
        <Icon name="close" className="h-5 w-5" />
      </button>
      <SignInForm />
    </div>
  </div>
}

export function LandingPage({ branding }: { branding?: SchoolBranding }) {
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const { user, ready } = useAuth()
  const slides = useQuery({ queryKey: ['school', 'slides'], queryFn: ({ signal }) => getSchoolSlides(signal), staleTime: 60_000, retry: 1 })
  const publicPage = useQuery({ queryKey: ['school', 'public-page'], queryFn: ({ signal }) => getPublicPage(signal), staleTime: 60_000, retry: 1 })
  const reduced = useReducedMotion()
  const showPublic = useAfterFirstPaint()
  const [index, setIndex] = useState(0)
  const [playing, setPlaying] = useState(true)
  const [hold, setHold] = useState(false) // hover or keyboard focus inside the slideshow
  const touchStart = useRef<{ x: number; y: number } | null>(null)
  const list = slides.data ?? []
  const count = list.length
  useEffect(() => { if (index >= count && count > 0) setIndex(0) }, [count, index])
  const advance = useCallback(() => setIndex(i => (count ? (i + 1) % count : 0)), [count])
  useEffect(() => {
    if (count < 2 || reduced || !playing || hold) return
    const timer = window.setTimeout(advance, SLIDE_MS)
    return () => window.clearTimeout(timer)
  }, [count, reduced, playing, hold, index, advance])

  if (ready && user) return <Navigate to="/dashboard" replace />
  const bn = i18n.language === 'bn'
  const name = branding ? (bn ? branding.name_bn : branding.name_en) : t('appName')
  const otherName = branding ? (bn ? branding.name_en : branding.name_bn) : ''
  const tagline = branding ? (bn ? branding.tagline_bn || branding.tagline_en : branding.tagline_en || branding.tagline_bn) ?? '' : ''
  const hasPhotos = count > 0
  const current = Math.min(index, Math.max(0, count - 1))
  const controls = { slides: list, index: current, onIndex: setIndex, playing, onPlaying: setPlaying, reduced, onHold: setHold }
  const design = publicPage.data?.design_style || 'modern_bold'
  const modern = design === 'modern_bold'
  const exploreTarget = publicPage.data?.about ? '#public-about' : publicPage.data?.gallery.length ? '#public-gallery' : '#public-sign-in'

  return <><div id="public-home" className="landing relative isolate" data-public-style={design}>
    {/* Banner: the photos on phones, the whole first screen on large screens. */}
    <section aria-label={t('p3Slides')} onMouseEnter={() => setHold(true)} onMouseLeave={() => setHold(false)}
      onFocus={() => setHold(true)} onBlur={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setHold(false) }}
      onTouchStart={modern ? e => { touchStart.current = { x: e.touches[0].clientX, y: e.touches[0].clientY } } : undefined}
      onTouchEnd={modern ? e => { const start = touchStart.current; touchStart.current = null; if (!start || count < 2) return; const dx = e.changedTouches[0].clientX - start.x; const dy = e.changedTouches[0].clientY - start.y; if (Math.abs(dx) > 45 && Math.abs(dx) > Math.abs(dy) * 1.3) setIndex(i => (i + (dx < 0 ? 1 : count - 1)) % count) } : undefined}
      className={`landing-hero absolute inset-x-0 top-0 overflow-hidden ${modern ? 'h-full' : 'h-[17rem] sm:h-[21rem] lg:h-full'} ${hasPhotos ? 'bg-slate-900' : 'landing-plain'}`}>
      {hasPhotos ? <Slideshow slides={list} index={current} reduced={reduced} />
        : branding?.logo_url && <img src={branding.logo_url} alt="" aria-hidden="true" referrerPolicy="no-referrer" className="absolute -right-16 top-1/2 h-[28rem] w-[28rem] -translate-y-1/2 object-contain opacity-[0.08] lg:right-[8%]" />}
      <div aria-hidden="true" className={`absolute inset-0 ${hasPhotos ? 'landing-overlay' : 'landing-plain-overlay'}`} />
      {count > 1 && (modern ? <SlideDots slides={list} index={current} onIndex={setIndex} /> : <SlideControls {...controls} placement="phone" className="absolute bottom-3 left-1/2 flex -translate-x-1/2 lg:hidden" />)}
    </section>

    <div className={`relative mx-auto grid grid-cols-1 max-w-6xl gap-6 px-4 sm:px-6 ${modern ? 'min-h-[calc(100svh-4rem)] items-end pb-24 pt-20 sm:pb-28 lg:pb-24' : 'lg:min-h-[calc(100svh-4rem)] lg:grid-cols-[minmax(0,1fr)_minmax(22rem,25rem)] lg:items-center lg:gap-12 lg:py-12'}`}>
      <div className={`pd-hero-copy flex min-w-0 flex-col justify-end text-white ${modern ? 'max-w-4xl' : 'h-[17rem] pb-14 sm:h-[21rem] lg:h-auto lg:pb-0'}`}>
        <div className="flex min-w-0 items-center gap-3 sm:gap-4">
          {branding?.logo_url
            ? <img src={branding.logo_url} alt="" referrerPolicy="no-referrer" className={`${modern ? 'h-11 w-11 sm:h-20 sm:w-20' : 'h-14 w-14 sm:h-20 sm:w-20'} shrink-0 rounded-2xl bg-white object-contain p-1.5 shadow-lg ring-1 ring-white/40`} />
            : <span aria-hidden="true" className={`landing-mark grid shrink-0 place-items-center rounded-2xl font-bold shadow-lg ${modern ? 'h-11 w-11 text-lg sm:h-20 sm:w-20 sm:text-2xl' : 'h-14 w-14 text-xl sm:h-20 sm:w-20 sm:text-2xl'}`}>{[...name.replace(/\s+/g, '')][0] ?? ''}</span>}
          <div className="min-w-0">
            <h1 className={`font-bold leading-tight tracking-tight [text-wrap:balance] ${modern ? 'pd-modern-hero-name' : 'text-2xl sm:text-4xl lg:text-5xl'}`} style={{ overflowWrap: 'anywhere' }}>{name}</h1>
            {otherName && otherName !== name && <p className="mt-1 text-sm font-medium text-white/80 sm:text-base" lang={bn ? 'en' : 'bn'}>{otherName}</p>}
          </div>
        </div>
        {tagline && <p className="mt-3 max-w-xl text-sm leading-6 text-white/90 sm:mt-5 sm:text-lg sm:leading-8">{tagline}</p>}
        {design === 'minimal' && <div className="pd-hero-actions mt-5 flex flex-wrap gap-3 sm:mt-8">
          <a href={exploreTarget} className="pd-hero-button pd-hero-button-primary">{t('pdHeroExplore')} <span aria-hidden="true">↗</span></a>
          <a href="#public-sign-in" className="pd-hero-button pd-hero-button-secondary">{t('pdHeroSignIn')}</a>
        </div>}
        {count > 1 && !modern && <SlideControls {...controls} placement="desktop" showCaption className="mt-10 hidden max-w-xl lg:flex" />}
      </div>
      {!modern && <div id="public-sign-in" className="min-w-0 pb-10 lg:pb-0"><SignInForm /></div>}
    </div>
  </div>
  {modern && location.pathname === '/login' && <SignInOverlay />}
  {showPublic && <Suspense fallback={null}><PublicSections /></Suspense>}
  </>
}
