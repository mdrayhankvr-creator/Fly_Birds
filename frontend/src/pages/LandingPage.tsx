import { useCallback, useEffect, useState, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router'
import { getSchoolSlides, slideImageUrl, type SchoolBranding, type SchoolSlide } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { Icon } from '../components/icons'
import { SignInForm } from './LoginPage'

const SLIDE_MS = 7000

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

// The school's photos, cross-fading behind the identity and sign-in form.
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

export function LandingPage({ branding }: { branding?: SchoolBranding }) {
  const { t, i18n } = useTranslation()
  const { user, ready } = useAuth()
  const slides = useQuery({ queryKey: ['school', 'slides'], queryFn: ({ signal }) => getSchoolSlides(signal), staleTime: 60_000, retry: 1 })
  const reduced = useReducedMotion()
  const [index, setIndex] = useState(0)
  const [playing, setPlaying] = useState(true)
  const [hold, setHold] = useState(false) // hover or keyboard focus inside the slideshow
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

  return <div className="landing relative isolate">
    {/* Banner: the photos on phones, the whole first screen on large screens. */}
    <section aria-label={t('p3Slides')} onMouseEnter={() => setHold(true)} onMouseLeave={() => setHold(false)}
      onFocus={() => setHold(true)} onBlur={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setHold(false) }}
      className={`landing-hero absolute inset-x-0 top-0 h-[17rem] overflow-hidden sm:h-[21rem] lg:h-full ${hasPhotos ? 'bg-slate-900' : 'landing-plain'}`}>
      {hasPhotos ? <Slideshow slides={list} index={current} reduced={reduced} />
        : branding?.logo_url && <img src={branding.logo_url} alt="" aria-hidden="true" referrerPolicy="no-referrer" className="absolute -right-16 top-1/2 h-[28rem] w-[28rem] -translate-y-1/2 object-contain opacity-[0.08] lg:right-[8%]" />}
      <div aria-hidden="true" className={`absolute inset-0 ${hasPhotos ? 'landing-overlay' : 'landing-plain-overlay'}`} />
      {count > 1 && <SlideControls {...controls} placement="phone" className="absolute bottom-3 left-1/2 flex -translate-x-1/2 lg:hidden" />}
    </section>

    <div className="relative mx-auto grid max-w-6xl gap-6 px-4 sm:px-6 lg:min-h-[calc(100svh-4rem)] lg:grid-cols-[minmax(0,1fr)_minmax(22rem,25rem)] lg:items-center lg:gap-12 lg:py-12">
      <div className="flex h-[17rem] min-w-0 flex-col justify-end pb-14 text-white sm:h-[21rem] lg:h-auto lg:pb-0">
        <div className="flex min-w-0 items-center gap-3 sm:gap-4">
          {branding?.logo_url
            ? <img src={branding.logo_url} alt="" referrerPolicy="no-referrer" className="h-14 w-14 shrink-0 rounded-2xl bg-white object-contain p-1.5 shadow-lg ring-1 ring-white/40 sm:h-20 sm:w-20" />
            : <span aria-hidden="true" className="landing-mark grid h-14 w-14 shrink-0 place-items-center rounded-2xl text-xl font-bold shadow-lg sm:h-20 sm:w-20 sm:text-2xl">{[...name.replace(/\s+/g, '')][0] ?? ''}</span>}
          <div className="min-w-0">
            <h1 className="text-2xl font-bold leading-tight tracking-tight [text-wrap:balance] sm:text-4xl lg:text-5xl" style={{ overflowWrap: 'anywhere' }}>{name}</h1>
            {otherName && otherName !== name && <p className="mt-1 text-sm font-medium text-white/80 sm:text-base" lang={bn ? 'en' : 'bn'}>{otherName}</p>}
          </div>
        </div>
        {tagline && <p className="mt-3 max-w-xl text-sm leading-6 text-white/90 sm:mt-5 sm:text-lg sm:leading-8">{tagline}</p>}
        {count > 1 && <SlideControls {...controls} placement="desktop" showCaption className="mt-10 hidden max-w-xl lg:flex" />}
      </div>
      <div className="min-w-0 pb-10 lg:pb-0"><SignInForm /></div>
    </div>
  </div>
}
