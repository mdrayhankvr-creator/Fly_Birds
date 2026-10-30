import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, addSchoolSlide, deleteSchoolSlide, getSchoolSlides, reorderSchoolSlides, slideImageUrl, updateSchoolSlide, type SchoolSlide } from '../api/client'
import { confirmAction } from '../components/ConfirmDialog'
import { CardTitle, ui } from './attendanceUi'

const MAX_PHOTOS = 8
const MAX_WIDTH = 1600
const MIN_WIDTH = 320

class PhotoTooSmall extends Error {}

// Shrinks a photo to at most 1600px wide and re-encodes it as JPEG in the
// browser, so cheap phones and slow connections upload a few hundred KB
// instead of the camera original. The server checks and re-encodes again.
async function preparePhoto(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
  try {
    if (bitmap.width < MIN_WIDTH || bitmap.height < MIN_WIDTH / 2) throw new PhotoTooSmall()
    const scale = Math.min(1, MAX_WIDTH / bitmap.width, 2400 / bitmap.height)
    const canvas = document.createElement('canvas')
    canvas.width = Math.round(bitmap.width * scale)
    canvas.height = Math.round(bitmap.height * scale)
    const context = canvas.getContext('2d')
    if (!context) throw new Error('canvas unavailable')
    context.fillStyle = '#ffffff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.82))
    if (!blob) throw new Error('encode failed')
    return blob
  } finally {
    bitmap.close()
  }
}

function PhotoRow({ slide, index, total, busy, authorized, onChanged, onMove, onError }: {
  slide: SchoolSlide; index: number; total: number; busy: boolean
  authorized: <T>(request: (token: string) => Promise<T>) => Promise<T>
  onChanged: (message: string) => Promise<void>; onMove: (from: number, to: number) => void; onError: (message: string) => void
}) {
  const { t } = useTranslation()
  const [en, setEn] = useState(slide.caption_en)
  const [bnCaption, setBn] = useState(slide.caption_bn)
  const [saving, setSaving] = useState(false)
  useEffect(() => { setEn(slide.caption_en); setBn(slide.caption_bn) }, [slide.caption_en, slide.caption_bn])
  const dirty = en !== slide.caption_en || bnCaption !== slide.caption_bn
  async function save() {
    setSaving(true)
    try { await authorized(token => updateSchoolSlide(token, slide.id, en, bnCaption)); await onChanged(t('p3PhotoSaved')) }
    catch { onError(t('p3PhotoError')) }
    finally { setSaving(false) }
  }
  async function remove() {
    if (!(await confirmAction(t('p3PhotoRemoveConfirm'), { tone: 'danger', confirmLabel: t('p3PhotoRemove') }))) return
    setSaving(true)
    try { await authorized(token => deleteSchoolSlide(token, slide.id)); await onChanged(t('p3PhotoSaved')) }
    catch { onError(t('p3PhotoError')) }
    finally { setSaving(false) }
  }
  const label = t('p3PhotoAlt', { n: index + 1 })
  return <li className="grid gap-3 rounded-xl border border-slate-200 p-3 sm:grid-cols-[10rem_minmax(0,1fr)] sm:p-4">
    <div className="relative">
      <img src={slideImageUrl(slide)} alt={label} loading="lazy" decoding="async" className="aspect-[16/10] w-full rounded-lg bg-slate-100 object-cover" />
      <span className="absolute left-1.5 top-1.5 rounded-md bg-slate-950/70 px-1.5 py-0.5 text-xs font-bold text-white">{index + 1}</span>
    </div>
    <div className="min-w-0 space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className={ui.label}>{t('p3PhotoCaptionEN')}<input className={ui.field} maxLength={140} value={en} onChange={e => setEn(e.target.value)} disabled={busy || saving} /></label>
        <label className={ui.label}>{t('p3PhotoCaptionBN')}<input className={ui.field} maxLength={140} lang="bn" value={bnCaption} onChange={e => setBn(e.target.value)} disabled={busy || saving} /></label>
      </div>
      <div className="flex flex-wrap gap-2">
        {dirty && <button type="button" className={ui.primary} disabled={busy || saving} onClick={() => void save()}>{t('p3PhotoSaveCaption')}</button>}
        <button type="button" className={ui.secondary} disabled={busy || saving || index === 0} onClick={() => onMove(index, index - 1)} aria-label={`${t('p3PhotoUp')} · ${label}`}>{t('p3PhotoUp')}</button>
        <button type="button" className={ui.secondary} disabled={busy || saving || index === total - 1} onClick={() => onMove(index, index + 1)} aria-label={`${t('p3PhotoDown')} · ${label}`}>{t('p3PhotoDown')}</button>
        <button type="button" className={ui.danger} disabled={busy || saving} onClick={() => void remove()} aria-label={`${t('p3PhotoRemove')} · ${label}`}>{t('p3PhotoRemove')}</button>
      </div>
    </div>
  </li>
}

// Landing-page photos, managed with the rest of the school's white-label settings.
export function LandingPhotos({ authorized }: { authorized: <T>(request: (token: string) => Promise<T>) => Promise<T> }) {
  const { t } = useTranslation()
  const cache = useQueryClient()
  const input = useRef<HTMLInputElement>(null)
  const slides = useQuery({ queryKey: ['school', 'slides', 'manage'], queryFn: ({ signal }) => getSchoolSlides(signal, true), retry: false })
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const list = slides.data ?? []
  const full = list.length >= MAX_PHOTOS

  async function refresh(note: string) {
    await cache.invalidateQueries({ queryKey: ['school', 'slides'] })
    setError(''); setMessage(note)
  }
  async function upload(files: FileList | null) {
    if (!files?.length) return
    setBusy(true); setError(''); setMessage('')
    let added = 0
    try {
      for (const file of [...files].slice(0, MAX_PHOTOS - list.length)) {
        try { await authorized(async token => addSchoolSlide(token, await preparePhoto(file))); added++ }
        catch (cause) {
          setError(cause instanceof PhotoTooSmall ? t('p3PhotoTooSmall') : cause instanceof ApiError && cause.status === 409 ? t('p3PhotoFull') : t('p3PhotoError'))
          break
        }
      }
      if (files.length > MAX_PHOTOS - list.length) setError(t('p3PhotoFull'))
    } finally {
      if (input.current) input.current.value = ''
      await cache.invalidateQueries({ queryKey: ['school', 'slides'] })
      if (added) setMessage(t('p3PhotoSaved'))
      setBusy(false)
    }
  }
  async function move(from: number, to: number) {
    const ids = list.map(s => s.id)
    const [id] = ids.splice(from, 1)
    ids.splice(to, 0, id)
    setBusy(true)
    try { await authorized(token => reorderSchoolSlides(token, ids)); await refresh(t('p3PhotoSaved')) }
    catch { setError(t('p3PhotoError')) }
    finally { setBusy(false) }
  }

  return <section aria-labelledby="landing-photos" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <CardTitle id="landing-photos" icon="image">{t('p3Photos')}</CardTitle>
        <p className="mt-1 max-w-2xl text-sm leading-6 text-slate-600">{t('p3PhotosHelp')}</p>
      </div>
      <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700">{t('p3PhotosCount', { count: list.length, max: MAX_PHOTOS })}</span>
    </div>
    {message && <p role="status" className="mt-4 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800">{message}</p>}
    {error && <p role="alert" className="mt-4 rounded-lg bg-rose-50 p-3 text-sm text-rose-800">{error}</p>}
    {slides.isPending ? <p role="status" className="mt-4 text-sm text-slate-600">{t('loadingSettings')}</p>
      : slides.isError ? <p role="alert" className="mt-4 text-sm text-rose-700">{t('p3PhotoError')}</p>
      : list.length === 0 ? <p className="mt-4 rounded-xl border border-dashed border-slate-300 p-4 text-sm text-slate-600">{t('p3PhotosEmpty')}</p>
      : <ol className="mt-4 space-y-3">{list.map((s, i) => <PhotoRow key={s.id} slide={s} index={i} total={list.length} busy={busy} authorized={authorized}
          onChanged={refresh} onMove={(a, b) => void move(a, b)} onError={setError} />)}</ol>}
    <div className="mt-5 flex flex-wrap items-center gap-3">
      <label className={`${ui.primary} cursor-pointer ${full || busy ? 'pointer-events-none opacity-50' : ''}`}>
        {busy ? t('p3PhotoAdding') : t('p3PhotoAdd')}
        <input ref={input} type="file" accept="image/jpeg,image/png,image/webp" multiple className="sr-only" disabled={full || busy} onChange={e => void upload(e.target.files)} />
      </label>
      {full && <span className="text-sm text-slate-600">{t('p3PhotoFull')}</span>}
    </div>
  </section>
}
