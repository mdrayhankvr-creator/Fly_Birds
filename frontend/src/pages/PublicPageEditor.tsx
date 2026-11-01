import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { addGalleryPhoto, deleteGalleryPhoto, deleteHeadPhoto, galleryImageUrl, getPublicPageForEdit, headPhotoUrl, saveHeadPhoto, savePublicPage, updateGalleryPhoto, type GalleryPhoto, type PublicPageInput } from '../api/publicPage'
import { confirmAction } from '../components/ConfirmDialog'
import { Icon } from '../components/icons'
import { CardTitle, Notice, ui } from './attendanceUi'

// The administrator's editor for the public school page (checkpoint 5).
type Authorized = <T>(request: (token: string) => Promise<T>) => Promise<T>

const blank: PublicPageInput = { about_en: '', about_bn: '', founded_year: null, eiin: '', show_counts: false, head_name_en: '', head_name_bn: '', head_message_en: '', head_message_bn: '', contact_address_en: '', contact_address_bn: '', contact_phone: '', contact_email: '', map_url: '' }

// Shrinks a photo in the browser before upload (the server re-encodes again),
// so slow connections send a few hundred KB instead of the camera original.
async function shrink(file: File, edge: number): Promise<Blob> {
  const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
  try {
    const scale = Math.min(1, edge / Math.max(bitmap.width, bitmap.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.round(bitmap.width * scale)
    canvas.height = Math.round(bitmap.height * scale)
    const context = canvas.getContext('2d')
    if (!context) throw new Error('canvas unavailable')
    context.fillStyle = '#ffffff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.85))
    if (!blob) throw new Error('encode failed')
    return blob
  } finally { bitmap.close() }
}

function Pair({ label, children }: { label: string; children: ReactNode }) {
  return <fieldset className="min-w-0"><legend className="text-sm font-bold text-slate-800">{label}</legend><div className="mt-2 grid gap-3 md:grid-cols-2">{children}</div></fieldset>
}

function GalleryRow({ photo, index, authorized, busy, onChanged, onError }: { photo: GalleryPhoto; index: number; authorized: Authorized; busy: boolean; onChanged: (m: string) => Promise<void>; onError: (m: string) => void }) {
  const { t } = useTranslation()
  const [en, setEn] = useState(photo.caption_en)
  const [bn, setBn] = useState(photo.caption_bn)
  const [saving, setSaving] = useState(false)
  useEffect(() => { setEn(photo.caption_en); setBn(photo.caption_bn) }, [photo.caption_en, photo.caption_bn])
  const label = t('c5GalleryPhotoLabel', { n: index + 1 })
  async function run(task: () => Promise<unknown>) {
    setSaving(true)
    try { await task(); await onChanged(t('c5PublicSaved')) } catch { onError(t('c5PublicFailed')) } finally { setSaving(false) }
  }
  return <li className="grid gap-3 rounded-xl border border-slate-200 p-3 sm:grid-cols-[8rem_minmax(0,1fr)]">
    <img src={galleryImageUrl(photo, true)} alt={label} loading="lazy" decoding="async" className="aspect-[4/3] w-full rounded-lg bg-slate-100 object-cover" />
    <div className="min-w-0 space-y-2">
      <div className="grid gap-2 sm:grid-cols-2">
        <label className={ui.label}>{t('p3PhotoCaptionEN')}<input className={ui.field} maxLength={140} value={en} disabled={busy || saving} onChange={e => setEn(e.target.value)} /></label>
        <label className={ui.label}>{t('p3PhotoCaptionBN')}<input className={ui.field} maxLength={140} lang="bn" value={bn} disabled={busy || saving} onChange={e => setBn(e.target.value)} /></label>
      </div>
      <div className="flex flex-wrap gap-2">
        {(en !== photo.caption_en || bn !== photo.caption_bn) && <button type="button" className={`${ui.primary} btn-sm`} disabled={busy || saving} onClick={() => void run(() => authorized(token => updateGalleryPhoto(token, photo.id, en, bn)))}>{t('p3PhotoSaveCaption')}</button>}
        <button type="button" className={`${ui.danger} btn-sm`} disabled={busy || saving} aria-label={`${t('p3PhotoRemove')} · ${label}`}
          onClick={async () => { if (await confirmAction(t('c5GalleryRemoveConfirm'), { confirmLabel: t('p3PhotoRemove') })) await run(() => authorized(token => deleteGalleryPhoto(token, photo.id))) }}>{t('p3PhotoRemove')}</button>
      </div>
    </div>
  </li>
}

export function PublicPageEditor({ authorized }: { authorized: Authorized }) {
  const { t } = useTranslation()
  const cache = useQueryClient()
  const data = useQuery({ queryKey: ['school', 'public-page', 'edit'], queryFn: () => authorized(token => getPublicPageForEdit(token)), retry: false })
  const [form, setForm] = useState<PublicPageInput>(blank)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  useEffect(() => {
    if (!data.data) return
    const { head_photo_version: _ignored, ...rest } = data.data.page
    void _ignored
    setForm(rest)
  }, [data.data])
  const set = <K extends keyof PublicPageInput>(key: K, value: PublicPageInput[K]) => setForm(f => ({ ...f, [key]: value }))
  async function refresh(text: string) {
    await cache.invalidateQueries({ queryKey: ['school', 'public-page'] })
    setMessage({ tone: 'success', text })
  }
  async function save(event: FormEvent) {
    event.preventDefault()
    setBusy(true); setMessage(null)
    try { await authorized(token => savePublicPage(token, form)); await refresh(t('c5PublicSaved')) }
    catch (cause) { setMessage({ tone: 'error', text: t(cause instanceof ApiError && cause.status === 400 ? 'c5PublicInvalid' : 'c5PublicFailed') }) }
    finally { setBusy(false) }
  }
  async function upload(files: FileList | null, target: 'head' | 'gallery') {
    if (!files?.length) return
    setBusy(true); setMessage(null)
    try {
      for (const file of Array.from(files)) {
        const image = await shrink(file, target === 'head' ? 960 : 1600)
        if (target === 'head') await authorized(token => saveHeadPhoto(token, image))
        else await authorized(token => addGalleryPhoto(token, image))
      }
      await refresh(t('c5PublicSaved'))
    } catch (cause) {
      setMessage({ tone: 'error', text: t(cause instanceof ApiError && cause.code === 'gallery_limit' ? 'c5GalleryFull' : 'c5PhotoFailed') })
      await cache.invalidateQueries({ queryKey: ['school', 'public-page'] })
    } finally { setBusy(false) }
  }
  const page = data.data?.page
  const gallery = data.data?.gallery ?? []
  const area = `${ui.field} min-h-28`
  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="public-page-editor">
    <CardTitle id="public-page-editor" icon="home">{t('c5PublicEditorTitle')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('c5PublicEditorIntro')}</p>
    {message && <div className="mt-4"><Notice tone={message.tone}>{message.text}</Notice></div>}
    {data.isError && <div className="mt-4"><Notice tone="error">{t('c5PublicFailed')}</Notice></div>}
    <form className="mt-5 space-y-6" onSubmit={e => void save(e)}>
      <Pair label={t('c5AboutTitle')}>
        <label className={ui.label}>{t('c5InEnglish')}<textarea className={area} maxLength={3000} value={form.about_en} onChange={e => set('about_en', e.target.value)} /></label>
        <label className={ui.label}>{t('c5InBangla')}<textarea className={area} maxLength={3000} lang="bn" value={form.about_bn} onChange={e => set('about_bn', e.target.value)} /></label>
        <label className={ui.label}>{t('c5Founded')}<input className={ui.field} inputMode="numeric" min={1800} max={2100} type="number" value={form.founded_year ?? ''} onChange={e => set('founded_year', e.target.value ? Number(e.target.value) : null)} /></label>
        <label className={ui.label}>{t('c5EIIN')}<input className={ui.field} inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={form.eiin} onChange={e => set('eiin', e.target.value.replace(/\D/g, ''))} /></label>
        <label className="flex min-h-11 items-center gap-3 text-sm font-semibold text-slate-800 md:col-span-2"><input type="checkbox" className="h-5 w-5 accent-[var(--brand-primary)]" checked={form.show_counts} onChange={e => set('show_counts', e.target.checked)} />{t('c5ShowCounts')}</label>
      </Pair>
      <Pair label={t('c5HeadTitle')}>
        <label className={ui.label}>{t('c5HeadNameEN')}<input className={ui.field} maxLength={160} value={form.head_name_en} onChange={e => set('head_name_en', e.target.value)} /></label>
        <label className={ui.label}>{t('c5HeadNameBN')}<input className={ui.field} maxLength={160} lang="bn" value={form.head_name_bn} onChange={e => set('head_name_bn', e.target.value)} /></label>
        <label className={ui.label}>{t('c5MessageEN')}<textarea className={area} maxLength={3000} value={form.head_message_en} onChange={e => set('head_message_en', e.target.value)} /></label>
        <label className={ui.label}>{t('c5MessageBN')}<textarea className={area} maxLength={3000} lang="bn" value={form.head_message_bn} onChange={e => set('head_message_bn', e.target.value)} /></label>
        <div className="flex flex-wrap items-center gap-3 md:col-span-2">
          {page?.head_photo_version ? <img src={headPhotoUrl(page.head_photo_version)} alt={t('c5HeadPhoto')} className="h-20 w-20 rounded-xl object-cover" /> : <span className="grid h-20 w-20 place-items-center rounded-xl bg-slate-100 text-xs text-slate-500">{t('noPhoto')}</span>}
          <label className={`${ui.secondary} ${busy ? 'pointer-events-none opacity-50' : ''}`}><Icon name="camera" className="h-4 w-4" />{t(page?.head_photo_version ? 'c4ChangePhoto' : 'c4AddPhoto')}<input type="file" accept="image/jpeg,image/png" className="sr-only" disabled={busy} onChange={e => { const f = e.target.files; void upload(f, 'head'); e.target.value = '' }} /></label>
          {page?.head_photo_version && <button type="button" className={ui.quiet} disabled={busy} onClick={async () => { setBusy(true); try { await authorized(token => deleteHeadPhoto(token)); await refresh(t('c5PublicSaved')) } catch { setMessage({ tone: 'error', text: t('c5PublicFailed') }) } finally { setBusy(false) } }}>{t('c4RemovePhoto')}</button>}
        </div>
      </Pair>
      <Pair label={t('c5ContactTitle')}>
        <label className={ui.label}>{t('c5AddressEN')}<textarea className={`${ui.field} min-h-20`} maxLength={300} value={form.contact_address_en} onChange={e => set('contact_address_en', e.target.value)} /></label>
        <label className={ui.label}>{t('c5AddressBN')}<textarea className={`${ui.field} min-h-20`} maxLength={300} lang="bn" value={form.contact_address_bn} onChange={e => set('contact_address_bn', e.target.value)} /></label>
        <label className={ui.label}>{t('phoneLabel')}<input className={ui.field} type="tel" maxLength={40} value={form.contact_phone} onChange={e => set('contact_phone', e.target.value)} /></label>
        <label className={ui.label}>{t('email')}<input className={ui.field} type="email" maxLength={254} value={form.contact_email} onChange={e => set('contact_email', e.target.value)} /></label>
        <label className={`${ui.label} md:col-span-2`}>{t('c5MapLink')}<input className={ui.field} type="url" pattern="https://.*" maxLength={500} placeholder="https://maps.google.com/…" value={form.map_url} onChange={e => set('map_url', e.target.value)} /><span className="mt-1 block text-xs font-normal normal-case tracking-normal text-slate-500">{t('c5MapLinkHint')}</span></label>
      </Pair>
      <div className="flex flex-wrap items-center gap-3"><button className={ui.primary} disabled={busy || data.isPending}><Icon name="check" className="h-4 w-4" />{busy ? t('c3Saving') : t('c5SavePublic')}</button><a className={ui.link} href="/" target="_blank" rel="noopener">{t('c5ViewPublic')}</a></div>
    </form>

    <div className="mt-8 border-t border-slate-100 pt-6">
      <h3 className="text-base font-bold text-slate-900">{t('c5GalleryTitle')}</h3>
      <div className="mt-3 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" role="note"><strong className="block">{t('c5ConsentTitle')}</strong>{t('c5ConsentText')}</div>
      <label className={`${ui.secondary} mt-3 ${busy || gallery.length >= 48 ? 'pointer-events-none opacity-50' : ''}`}><Icon name="upload" className="h-4 w-4" />{t('c5AddGalleryPhotos')}<input type="file" multiple accept="image/jpeg,image/png" className="sr-only" disabled={busy || gallery.length >= 48} onChange={e => { const f = e.target.files; void upload(f, 'gallery'); e.target.value = '' }} /></label>
      <p className="mt-1 text-xs text-slate-500">{t('c5GalleryLimits', { count: gallery.length })}</p>
      {gallery.length > 0 && <ul className="mt-4 space-y-3">{gallery.map((p, i) => <GalleryRow key={p.id} photo={p} index={i} authorized={authorized} busy={busy} onChanged={refresh} onError={text => setMessage({ tone: 'error', text })} />)}</ul>}
    </div>
  </section>
}
