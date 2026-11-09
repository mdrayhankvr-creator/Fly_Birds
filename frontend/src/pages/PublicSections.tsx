import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router'
import { galleryImageUrl, getPublicPage, headPhotoUrl, type GalleryPhoto } from '../api/publicPage'
import { Icon } from '../components/icons'

// The public school page below the sign-in form (checkpoint 5). It is a
// separate chunk, requested after the sign-in form has rendered, so it never
// slows signing in. Sections with no content are not shown.

const pick = (bn: boolean, en: string, other: string) => (bn ? other || en : en || other)

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return <section aria-labelledby={id} className="public-section mx-auto max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
    <h2 id={id} className="text-2xl font-bold tracking-tight text-slate-900 sm:text-3xl">{title}</h2>
    <div className="mt-5">{children}</div>
  </section>
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
  const number = (n: number) => new Intl.NumberFormat(bn ? 'bn-BD' : 'en-US').format(n)
  const date = (iso: string) => new Intl.DateTimeFormat(bn ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(new Date(iso))
  const facts: [string, string][] = []
  if (v.about?.founded_year) facts.push([t('c5Founded'), bn ? number(v.about.founded_year).replace(/,/g, '') : String(v.about.founded_year)])
  if (v.about?.eiin) facts.push([t('c5EIIN'), v.about.eiin])
  if (v.about?.students != null) facts.push([t('c5Students'), number(v.about.students)])
  if (v.about?.teachers != null) facts.push([t('c5Teachers'), number(v.about.teachers)])
  const about = v.about && pick(bn, v.about.about_en, v.about.about_bn)
  const message = v.head && pick(bn, v.head.message_en, v.head.message_bn)
  const headName = v.head && pick(bn, v.head.name_en, v.head.name_bn)
  const address = v.contact && pick(bn, v.contact.address_en, v.contact.address_bn)
  return <div className="public-page border-t border-slate-200 bg-slate-50" data-public-page>
    {v.about && <Section id="public-about" title={t('c5AboutTitle')}>
      {about && <Paragraphs text={about} />}
      {facts.length > 0 && <dl className="mt-6 grid max-w-3xl grid-cols-2 gap-3 sm:grid-cols-4">{facts.map(([label, value]) => <div key={label} className="rounded-2xl border border-slate-200 bg-white p-4"><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt><dd className="mt-1 text-2xl font-extrabold tabular-nums text-slate-900">{value}</dd></div>)}</dl>}
    </Section>}

    {v.head && message && <Section id="public-head" title={t('c5HeadTitle')}>
      <figure className="grid max-w-4xl gap-6 rounded-3xl border border-slate-200 bg-white p-5 sm:grid-cols-[10rem_minmax(0,1fr)] sm:p-8">
        {v.head.photo_version ? <img src={headPhotoUrl(v.head.photo_version)} alt={headName ? t('c5HeadPhotoAlt', { name: headName }) : ''} width={480} height={480} loading="lazy" decoding="async" className="h-32 w-32 rounded-2xl object-cover sm:h-40 sm:w-40" />
          : <span aria-hidden="true" className="grid h-32 w-32 place-items-center rounded-2xl bg-[var(--brand-soft)] text-4xl font-bold text-[var(--brand-ink)] sm:h-40 sm:w-40">{[...(headName || '·')][0]}</span>}
        <div className="min-w-0">
          <blockquote><Paragraphs text={message} /></blockquote>
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
      <ul className="grid gap-3 md:grid-cols-2">{v.notices.map(n => <li key={n.id} className={`rounded-2xl border bg-white p-5 ${n.priority === 'urgent' ? 'border-rose-300' : n.priority === 'important' ? 'border-amber-300' : 'border-slate-200'}`}>
        <p className="text-xs font-semibold text-slate-500"><time dateTime={n.publish_at}>{date(n.publish_at)}</time>{n.priority !== 'normal' && <> · <span className={n.priority === 'urgent' ? 'text-rose-700' : 'text-amber-800'}>{t(`m9Priority_${n.priority}`)}</span></>}</p>
        <h3 className="mt-1 text-lg font-bold text-slate-900">{pick(bn, n.title_en, n.title_bn)}</h3>
        <p className="mt-2 whitespace-pre-line text-sm leading-6 text-slate-700">{pick(bn, n.body_en, n.body_bn)}</p>
      </li>)}</ul>
    </Section>}

    {v.contact && <Section id="public-contact" title={t('c5ContactTitle')}>
      <dl className="grid max-w-4xl gap-3 sm:grid-cols-2">
        {address && <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:col-span-2"><dt className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500"><Icon name="home" className="h-4 w-4" />{t('c5Address')}</dt><dd className="mt-1 whitespace-pre-line text-slate-900">{address}</dd>
          {v.contact.map_url && <dd className="mt-2"><a href={v.contact.map_url} target="_blank" rel="noopener noreferrer" className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{t('c5OpenMap')}</a></dd>}</div>}
        {!address && v.contact.map_url && <div className="rounded-2xl border border-slate-200 bg-white p-4 sm:col-span-2"><a href={v.contact.map_url} target="_blank" rel="noopener noreferrer" className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{t('c5OpenMap')}</a></div>}
        {v.contact.phone && <div className="rounded-2xl border border-slate-200 bg-white p-4"><dt className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500"><Icon name="phone" className="h-4 w-4" />{t('phoneLabel')}</dt><dd className="mt-1"><a href={`tel:${v.contact.phone.replace(/[^+0-9]/g, '')}`} className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{v.contact.phone}</a></dd></div>}
        {v.contact.email && <div className="rounded-2xl border border-slate-200 bg-white p-4"><dt className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500"><Icon name="mail" className="h-4 w-4" />{t('email')}</dt><dd className="mt-1" style={{ overflowWrap: 'anywhere' }}><a href={`mailto:${v.contact.email}`} className="font-semibold text-[var(--brand-ink)] underline decoration-2 underline-offset-4">{v.contact.email}</a></dd></div>}
      </dl>
    </Section>}
  </div>
}
