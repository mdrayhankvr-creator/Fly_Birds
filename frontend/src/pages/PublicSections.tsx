import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router'
import { getSchoolBranding } from '../api/client'
import { galleryImageUrl, getPublicPage, headPhotoUrl, type GalleryPhoto, type PublicView } from '../api/publicPage'
import { SchoolMark } from '../components/AppShell'
import { Icon } from '../components/icons'

// The public school page below the sign-in form (checkpoint 5). It is a
// separate chunk, requested after the sign-in form has rendered, so it never
// slows signing in. Sections with no content are not shown.

const pick = (bn: boolean, en: string, other: string) => (bn ? other || en : en || other)

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    const node = ref.current
    if (!node || !('IntersectionObserver' in window) || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    node.classList.add('pd-reveal')
    const observer = new IntersectionObserver(entries => { if (entries.some(entry => entry.isIntersecting)) { node.classList.add('pd-entered'); observer.disconnect() } }, { threshold: 0.06 })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  return <section ref={ref} aria-labelledby={id} className="public-section py-10 sm:py-16">
    <div className="mx-auto max-w-6xl px-4 sm:px-6"><h2 id={id} className="public-section-title text-2xl font-bold tracking-tight text-slate-900 sm:text-3xl">{title}</h2>
    <div className="mt-5">{children}</div></div>
  </section>
}

function CountUp({ value, suffix = '', format }: { value: number; suffix?: string; format: (n: number) => string }) {
  const ref = useRef<HTMLElement>(null)
  const [shown, setShown] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches ? value : 0)
  useEffect(() => {
    const node = ref.current
    if (!node || !('IntersectionObserver' in window) || window.matchMedia('(prefers-reduced-motion: reduce)').matches) { setShown(value); return }
    let frame = 0
    const observer = new IntersectionObserver(entries => {
      if (!entries.some(entry => entry.isIntersecting)) return
      observer.disconnect()
      const start = performance.now()
      const tick = (now: number) => {
        const progress = Math.min(1, (now - start) / 1000)
        setShown(Math.round(value * (1 - (1 - progress) ** 3)))
        if (progress < 1) frame = requestAnimationFrame(tick)
      }
      frame = requestAnimationFrame(tick)
    }, { threshold: 0.3 })
    observer.observe(node)
    return () => { observer.disconnect(); cancelAnimationFrame(frame) }
  }, [value])
  return <><span ref={ref} data-count-up={value} aria-hidden="true" className="inline-block tabular-nums" style={{ minWidth: `${String(value).length + 1}ch` }}>{format(shown)}{suffix}</span><span className="sr-only">{format(value)}{suffix}</span></>
}

function Paragraphs({ text, lang }: { text: string; lang?: string }) {
  return <div lang={lang} className="max-w-3xl space-y-3 text-base leading-7 text-slate-700">{text.split(/\n{2,}|\n/).filter(Boolean).map((p, i) => <p key={i}>{p}</p>)}</div>
}

// A photo viewer: the full-size photo loads only when opened.
function Lightbox({ photos, index, onIndex, onClose, bn }: { photos: GalleryPhoto[]; index: number; onIndex: (i: number) => void; onClose: () => void; bn: boolean }) {
  const { t } = useTranslation()
  const closeRef = useRef<HTMLButtonElement>(null)
  const photo = photos[index]
  const caption = pick(bn, photo.caption_en, photo.caption_bn)
  const go = (i: number) => onIndex((i + photos.length) % photos.length)
  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      else if (e.key === 'ArrowRight') go(index + 1)
      else if (e.key === 'ArrowLeft') go(index - 1)
    }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => { document.removeEventListener('keydown', onKey); document.body.style.overflow = ''; opener?.focus?.() }
  }) // eslint-disable-line react-hooks/exhaustive-deps
  const button = 'grid h-11 w-11 place-items-center rounded-full bg-white/10 text-white hover:bg-white/20 focus-visible:outline-2 focus-visible:outline-white'
  return <div role="dialog" aria-modal="true" aria-label={caption || t('c5GalleryPhotoN', { n: index + 1, total: photos.length })} className="fixed inset-0 z-50 flex flex-col bg-slate-950/95 p-3 sm:p-6" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
    <div className="flex items-center justify-between gap-3 text-sm text-white/80">
      <span>{t('c5GalleryPhotoN', { n: index + 1, total: photos.length })}</span>
      <button ref={closeRef} type="button" onClick={onClose} aria-label={t('c5Close')} className={button}><Icon name="close" className="h-5 w-5" /></button>
    </div>
    <div className="relative flex min-h-0 flex-1 items-center justify-center py-3" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <img key={photo.id} src={galleryImageUrl(photo)} alt={caption} width={photo.width} height={photo.height} className="max-h-full max-w-full rounded-lg object-contain" />
      {photos.length > 1 && <>
        <button type="button" onClick={() => go(index - 1)} aria-label={t('p3SlidePrev')} className={`${button} absolute left-0 top-1/2 -translate-y-1/2`}><Icon name="chevronLeft" className="h-5 w-5" /></button>
        <button type="button" onClick={() => go(index + 1)} aria-label={t('p3SlideNext')} className={`${button} absolute right-0 top-1/2 -translate-y-1/2`}><Icon name="chevronRight" className="h-5 w-5" /></button>
      </>}
    </div>
    {caption && <p className="text-center text-sm text-white/90">{caption}</p>}
  </div>
}

function ModernSection({ id, title, action, children }: { id: string; title: string; action?: ReactNode; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    const node = ref.current
    if (!node || !('IntersectionObserver' in window) || matchMedia('(prefers-reduced-motion: reduce)').matches) return
    node.classList.add('pd-reveal')
    const observer = new IntersectionObserver(entries => { if (entries.some(entry => entry.isIntersecting)) { node.classList.add('pd-entered'); observer.disconnect() } }, { threshold: 0.06 })
    observer.observe(node)
    return () => observer.disconnect()
  }, [])
  return <section ref={ref} id={id} className="public-section pd-modern-section" aria-labelledby={`${id}-title`}>
    <div className="mx-auto max-w-6xl px-4 sm:px-6">
      <div className="pd-section-heading"><h2 id={`${id}-title`} className="public-section-title">{title}</h2>{action}</div>
      <div className="pd-section-content">{children}</div>
    </div>
  </section>
}

function ModernSections({ view: v, bn }: { view: PublicView; bn: boolean }) {
  const { t } = useTranslation()
  const branding = useQuery({ queryKey: ['school', 'branding'], queryFn: ({ signal }) => getSchoolBranding(signal), staleTime: 60_000 })
  const [galleryAll, setGalleryAll] = useState(false)
  const [noticesAll, setNoticesAll] = useState(false)
  const [open, setOpen] = useState<number | null>(null)
  const name = branding.data ? pick(bn, branding.data.name_en, branding.data.name_bn) : ''
  const about = v.about ? pick(bn, v.about.about_en, v.about.about_bn) : ''
  const activities = pick(bn, v.activities_en, v.activities_bn).split('\n').map(s => s.trim()).filter(Boolean)
  const message = v.head ? pick(bn, v.head.message_en, v.head.message_bn) : ''
  const headName = v.head ? pick(bn, v.head.name_en, v.head.name_bn) : ''
  const address = v.contact ? pick(bn, v.contact.address_en, v.contact.address_bn) : ''
  const number = (n: number) => new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US').format(n)
  const date = (iso: string) => new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date(iso))
  const thisYear = Number(new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Dhaka', year: 'numeric' }).format(new Date()))
  const facts: { label: string; value: number; suffix?: string }[] = []
  if (v.about?.founded_year) facts.push({ label: t('pdYears'), value: Math.max(0, thisYear - v.about.founded_year) })
  if (v.about?.students != null) facts.push({ label: t('c5Students'), value: v.about.students })
  if (v.about?.teachers != null) facts.push({ label: t('c5Teachers'), value: v.about.teachers })
  if (v.about?.pass_rate != null) facts.push({ label: t('pdPassRateStat'), value: v.about.pass_rate, suffix: '%' })
  return <div className="public-page pd-modern" data-public-page data-public-style="modern_bold">
    {v.features.length > 0 && <ModernSection id="public-why" title={t('pdWhyTitle')}>
      <div className="pd-feature-grid">{v.features.map(feature => <article key={feature.position} className="pd-feature-card">
        <span className="pd-feature-icon"><Icon name={feature.icon} className="h-6 w-6" /></span>
        <h3>{pick(bn, feature.title_en, feature.title_bn)}</h3>
        <p>{pick(bn, feature.description_en, feature.description_bn)}</p>
      </article>)}</div>
    </ModernSection>}
    {(v.about || activities.length > 0) && <ModernSection id="public-about" title={t('c5AboutTitle')}>
      <div className="pd-about-grid">
        {v.gallery.length > 0 && <div className="pd-about-photos" aria-label={t('c5GalleryTitle')}>
          <img src={galleryImageUrl(v.gallery[0], true)} alt={pick(bn, v.gallery[0].caption_en, v.gallery[0].caption_bn)} width={v.gallery[0].width} height={v.gallery[0].height} loading="lazy" decoding="async" />
          {v.gallery[1] && <img src={galleryImageUrl(v.gallery[1], true)} alt={pick(bn, v.gallery[1].caption_en, v.gallery[1].caption_bn)} width={v.gallery[1].width} height={v.gallery[1].height} loading="lazy" decoding="async" />}
        </div>}
        <div className="pd-about-copy">
          <div className="flex min-w-0 items-center gap-3"><SchoolMark branding={branding.data} size="h-14 w-14" /><h3>{name}</h3></div>
          {about && <Paragraphs text={about} />}
          {activities.length > 0 && <div className="pd-activities"><h4>{t('pdActivitiesTitle')}</h4><ul>{activities.map((item, i) => <li key={i}><Icon name="check" className="h-5 w-5" />{item}</li>)}</ul></div>}
          {(v.about?.founded_year || v.about?.eiin) && <p className="pd-about-details">{v.about?.founded_year ? `${t('c5Founded')}: ${new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US', { useGrouping: false }).format(v.about.founded_year)}` : ''}{v.about?.founded_year && v.about?.eiin ? ' · ' : ''}{v.about?.eiin ? `${t('c5EIIN')}: ${v.about.eiin}` : ''}</p>}
        </div>
      </div>
      {facts.length > 0 && <dl className="pd-stats">{facts.map(fact => <div key={fact.label} className="pd-stat-card"><dt>{fact.label}</dt><dd><CountUp value={fact.value} suffix={fact.suffix} format={number} /></dd></div>)}</dl>}
    </ModernSection>}
    {message && v.head && <ModernSection id="public-head" title={t('c5HeadTitle')}>
      <figure className="pd-head-card">
        {v.head.photo_version && <img src={headPhotoUrl(v.head.photo_version)} alt={headName ? t('c5HeadPhotoAlt', { name: headName }) : ''} width={480} height={480} loading="lazy" decoding="async" />}
        <div className="min-w-0"><span className="pd-head-mark" aria-hidden="true">“</span><blockquote><Paragraphs text={message} /></blockquote>{headName && <figcaption>{headName}<span>{t('c5HeadRole')}</span></figcaption>}</div>
      </figure>
    </ModernSection>}
    <ModernSection id="public-results" title={t('pdNavResults')}>
      <div className="pd-results-card"><Icon name="report" className="h-9 w-9" /><p>{t('pdResultsPrivate')}</p><a href="/#public-sign-in" className="pd-pill">{t('pdHeroSignIn')} <Icon name="arrowRight" className="h-4 w-4" /></a></div>
    </ModernSection>
    {v.notices.length > 0 && <ModernSection id="public-notices" title={t('c5NoticesTitle')} action={v.notices.length > 3 ? <button type="button" className="pd-pill" onClick={() => setNoticesAll(x => !x)} aria-expanded={noticesAll}>{t(noticesAll ? 'pdShowLess' : 'pdSeeAll')} <Icon name="arrowRight" className="h-4 w-4" /></button> : undefined}>
      <ul className="pd-notice-grid">{v.notices.slice(0, noticesAll ? undefined : 3).map(notice => <li key={notice.id} className="pd-notice-card">
        <time dateTime={notice.publish_at}>{date(notice.publish_at)}</time><h3>{pick(bn, notice.title_en, notice.title_bn)}</h3><p>{pick(bn, notice.body_en, notice.body_bn)}</p>
      </li>)}</ul>
    </ModernSection>}
    {v.gallery.length > 0 && <ModernSection id="public-gallery" title={t('c5GalleryTitle')} action={v.gallery.length > 4 ? <button type="button" className="pd-pill" onClick={() => setGalleryAll(x => !x)} aria-expanded={galleryAll}>{t(galleryAll ? 'pdShowLess' : 'pdSeeAll')} <Icon name="arrowRight" className="h-4 w-4" /></button> : undefined}>
      <ul className="pd-gallery-grid">{v.gallery.slice(0, galleryAll ? undefined : 4).map((photo, i) => <li key={photo.id}><button type="button" onClick={() => setOpen(i)} aria-label={t('c5OpenPhoto', { n: i + 1 })}>
        <img src={galleryImageUrl(photo, true)} alt="" width={photo.width} height={photo.height} loading="lazy" decoding="async" /><span>{pick(bn, photo.caption_en, photo.caption_bn)}</span>
      </button></li>)}</ul>
      {open !== null && <Lightbox photos={v.gallery} index={open} onIndex={setOpen} onClose={() => setOpen(null)} bn={bn} />}
    </ModernSection>}
    {v.contact && <ModernSection id="public-contact" title={t('c5ContactTitle')}>
      <div className="pd-contact-grid">
        {address && <div className="pd-contact-card"><Icon name="home" className="h-6 w-6" /><div><h3>{t('c5Address')}</h3><p>{address}</p>{v.contact.map_url && <a href={v.contact.map_url} target="_blank" rel="noopener noreferrer">{t('c5OpenMap')}</a>}</div></div>}
        {v.contact.email && <div className="pd-contact-card"><Icon name="mail" className="h-6 w-6" /><div><h3>{t('email')}</h3><a href={`mailto:${v.contact.email}`}>{v.contact.email}</a></div></div>}
        {!address && v.contact.map_url && <div className="pd-contact-card"><Icon name="home" className="h-6 w-6" /><a href={v.contact.map_url} target="_blank" rel="noopener noreferrer">{t('c5OpenMap')}</a></div>}
      </div>
    </ModernSection>}
  </div>
}

export function PublicSections() {
  const { t, i18n } = useTranslation()
  const bn = i18n.language === 'bn'
  const page = useQuery({ queryKey: ['school', 'public-page'], queryFn: ({ signal }) => getPublicPage(signal), staleTime: 60_000, retry: 1 })
  const [open, setOpen] = useState<number | null>(null)
  const v = page.data
  // A link such as /#public-notices (from the footer) scrolls once the sections are in.
  const { hash } = useLocation()
  useEffect(() => {
    if (!v || !hash.startsWith('#public-')) return
    document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [v, hash])
  if (!v) return null
  if (v.design_style === 'modern_bold') return <ModernSections view={v} bn={bn} />
  const number = (n: number) => new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US').format(n)
  const date = (iso: string) => new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date(iso))
  const style = v.design_style || 'modern_bold'
  const facts: { label: string; value: number | string; suffix?: string }[] = []
  if (v.about?.students != null) facts.push({ label: t('c5Students'), value: v.about.students })
  if (v.about?.teachers != null) facts.push({ label: t('c5Teachers'), value: v.about.teachers })
  if (v.about?.pass_rate != null) facts.push({ label: t('pdPassRateStat'), value: v.about.pass_rate, suffix: '%' })
  if (v.about?.founded_year) facts.unshift({ label: t('c5Founded'), value: new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US', { useGrouping: false }).format(v.about.founded_year) })
  if (v.about?.eiin) facts.push({ label: t('c5EIIN'), value: v.about.eiin })
  const about = v.about && pick(bn, v.about.about_en, v.about.about_bn)
  const message = v.head && pick(bn, v.head.message_en, v.head.message_bn)
  const headName = v.head && pick(bn, v.head.name_en, v.head.name_bn)
  const address = v.contact && pick(bn, v.contact.address_en, v.contact.address_bn)
  return <div className="public-page border-t border-slate-200 bg-slate-50" data-public-page data-public-style={style}>
    {v.about && <Section id="public-about" title={t('c5AboutTitle')}>
      {about && <Paragraphs text={about} />}
      {facts.length > 0 && <dl className="public-stats mt-6 grid grid-cols-2 gap-3 sm:grid-cols-4">{facts.map(({label, value, suffix}) => <div key={label} className="public-stat rounded-2xl border border-slate-200 bg-white p-4"><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt><dd className="mt-1 text-2xl font-extrabold tabular-nums text-slate-900">{typeof value === 'number' ? `${number(value)}${suffix || ''}` : value}</dd></div>)}</dl>}
    </Section>}

    {v.head && message && <Section id="public-head" title={t('c5HeadTitle')}>
      <figure className="grid grid-cols-1 max-w-4xl gap-6 rounded-3xl border border-slate-200 bg-white p-5 sm:grid-cols-[10rem_minmax(0,1fr)] sm:p-8">
        {v.head.photo_version ? <img src={headPhotoUrl(v.head.photo_version)} alt={headName ? t('c5HeadPhotoAlt', { name: headName }) : ''} width={480} height={480} loading="lazy" decoding="async" className="h-32 w-32 rounded-2xl object-cover sm:h-40 sm:w-40" />
          : <span aria-hidden="true" className="grid h-32 w-32 place-items-center rounded-2xl bg-[var(--brand-soft)] text-4xl font-bold text-[var(--brand-ink)] sm:h-40 sm:w-40">{[...(headName || '·')][0]}</span>}
        <div className="min-w-0">
          <blockquote className="public-head-quote relative"><span aria-hidden="true" className="public-quote-mark">“</span><Paragraphs text={message} /></blockquote>
          {headName && <figcaption className="mt-4 font-semibold text-slate-900">{headName}<span className="block text-sm font-normal text-slate-600">{t('c5HeadRole')}</span></figcaption>}
        </div>
      </figure>
    </Section>}

    {v.gallery.length > 0 && <Section id="public-gallery" title={t('c5GalleryTitle')}>
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-3 lg:grid-cols-4">{v.gallery.map((p, i) => {
        const caption = pick(bn, p.caption_en, p.caption_bn)
        return <li key={p.id}><button type="button" onClick={() => setOpen(i)} className="group block w-full overflow-hidden rounded-xl bg-slate-200 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)]" aria-label={caption ? t('c5OpenPhotoCaption', { caption }) : t('c5OpenPhoto', { n: i + 1 })}>
          <img src={galleryImageUrl(p, true)} alt="" loading="lazy" decoding="async" width={480} height={Math.round(480 * p.height / p.width)} className="aspect-[4/3] w-full object-cover transition-transform duration-300 group-hover:scale-[1.03]" />
          {caption && <span className="block truncate px-2.5 py-1.5 text-xs font-medium text-slate-700">{caption}</span>}
        </button></li>
      })}</ul>
      {open !== null && <Lightbox photos={v.gallery} index={open} onIndex={setOpen} onClose={() => setOpen(null)} bn={bn} />}
    </Section>}

    {v.notices.length > 0 && <Section id="public-notices" title={t('c5NoticesTitle')}>
      <ul className="grid grid-cols-1 gap-3 md:grid-cols-2">{v.notices.map(n => <li key={n.id} className={`rounded-2xl border bg-white p-5 ${n.priority === 'urgent' ? 'border-rose-300' : n.priority === 'important' ? 'border-amber-300' : 'border-slate-200'}`}>
        <p className="text-xs font-semibold text-slate-500"><time dateTime={n.publish_at}>{date(n.publish_at)}</time>{n.priority !== 'normal' && <> · <span className={n.priority === 'urgent' ? 'text-rose-700' : 'text-amber-800'}>{t(`m9Priority_${n.priority}`)}</span></>}</p>
        <h3 className="mt-1 text-lg font-bold text-slate-900">{pick(bn, n.title_en, n.title_bn)}</h3>
        <p className="mt-2 whitespace-pre-line text-sm leading-6 text-slate-700">{pick(bn, n.body_en, n.body_bn)}</p>
      </li>)}</ul>
    </Section>}

    {v.contact && <Section id="public-contact" title={t('c5ContactTitle')}>
      <dl className="grid grid-cols-1 max-w-4xl gap-3 sm:grid-cols-2">
        {address && <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:col-span-2"><dt className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500"><Icon name="home" className="h-4 w-4" />{t('c5Address')}</dt><dd className="mt-1 whitespace-pre-line text-slate-900">{address}</dd>
          {v.contact.map_url && <dd className="mt-2"><a href={v.contact.map_url} target="_blank" rel="noopener noreferrer" className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{t('c5OpenMap')}</a></dd>}</div>}
        {!address && v.contact.map_url && <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:col-span-2"><a href={v.contact.map_url} target="_blank" rel="noopener noreferrer" className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{t('c5OpenMap')}</a></div>}
        {v.contact.email && <div className="rounded-2xl border border-slate-200 bg-white p-4"><dt className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500"><Icon name="mail" className="h-4 w-4" />{t('email')}</dt><dd className="mt-1" style={{ overflowWrap: 'anywhere' }}><a href={`mailto:${v.contact.email}`} className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{v.contact.email}</a></dd></div>}
      </dl>
    </Section>}
  </div>
}
