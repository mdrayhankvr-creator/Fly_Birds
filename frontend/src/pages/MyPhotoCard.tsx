import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { getMyPhotoSubmission, getMyPhotoSubmissionImage, submitMyPhoto } from '../api/studentPhotos'
import { Icon } from '../components/icons'
import { PhotoCropper, PhotoGuidance } from '../components/PhotoCropper'
import { CardTitle, Notice, StatusBadge, ui } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

// The student's own photo: upload one, see whether it is waiting, approved
// or rejected (with the reason). Only an approved photo is used on the
// profile, the ID card, the marksheet and the admit card.
export function MyPhotoCard() {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const state = useQuery({ queryKey: ['people', 'my-photo'], queryFn: () => authorized(token => getMyPhotoSubmission(token)), retry: false })
  const waiting = state.data?.status === 'pending' || state.data?.status === 'rejected'
  const image = useQuery({ queryKey: ['people', 'my-photo-image', state.data?.submitted_at], queryFn: () => authorized(token => getMyPhotoSubmissionImage(token)), enabled: waiting, retry: false })
  const [preview, setPreview] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  useEffect(() => {
    if (!image.data) { setPreview(''); return }
    const url = URL.createObjectURL(image.data); setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [image.data])

  if (state.isPending || state.isError || !state.data) return null
  const s = state.data
  const when = (iso: string | null) => iso ? new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { dateStyle: 'medium', timeZone: 'Asia/Dhaka' }).format(new Date(iso)) : ''

  async function send(blob: Blob) {
    setBusy(true); setMessage(null)
    try {
      await authorized(token => submitMyPhoto(token, blob))
      setFile(null)
      setMessage({ tone: 'success', text: t('uxPhotoSent') })
      await cache.invalidateQueries({ queryKey: ['people', 'my-photo'] })
    } catch (cause) {
      setMessage({ tone: 'error', text: t(cause instanceof ApiError && cause.code === 'photo_uploads_off' ? 'uxPhotoUploadsOff' : 'uxPhotoFailed') })
    } finally { setBusy(false) }
  }

  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="my-photo-title" data-my-photo>
    <CardTitle id="my-photo-title" icon="camera">{t('uxMyPhoto')}</CardTitle>
    <div className="mt-3 space-y-3">
      {s.status === 'pending' && <div className="flex flex-wrap items-center gap-3"><StatusBadge tone="leave">{t('uxPhotoPending')}</StatusBadge><span className="text-sm text-slate-700">{t('uxPhotoPendingHelp', { date: when(s.submitted_at) })}</span></div>}
      {s.status === 'approved' && <div className="flex flex-wrap items-center gap-3"><StatusBadge tone="present">{t('uxPhotoApproved')}</StatusBadge><span className="text-sm text-slate-700">{t('uxPhotoApprovedHelp', { date: when(s.reviewed_at) })}</span></div>}
      {s.status === 'rejected' && <div role="status" className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-950" data-photo-rejected>
        <p className="font-bold">{t('uxPhotoRejected')}</p>
        <p className="mt-1"><span className="font-semibold">{t('uxPhotoReason')}:</span> <span style={{ overflowWrap: 'anywhere' }}>{s.reason}</span></p>
        <p className="mt-1">{t('uxPhotoTryAgain')}</p>
      </div>}
      {preview && <figure className="m-0 flex items-center gap-3"><img src={preview} alt={t('uxPhotoYourUpload')} className="h-24 w-24 rounded-xl object-cover ring-1 ring-slate-200" /><figcaption className="text-xs text-slate-600">{t('uxPhotoYourUpload')}</figcaption></figure>}
      {message && <Notice tone={message.tone}>{message.text}</Notice>}
      {!s.enabled ? <p className="text-sm text-slate-600">{t('uxPhotoUploadsOff')}</p>
        : file ? <PhotoCropper file={file} busy={busy} onCancel={() => setFile(null)} onDone={blob => void send(blob)} />
        : <>
          {s.status === '' && !s.has_photo && <PhotoGuidance />}
          <label className={`${ui.secondary} cursor-pointer`}>
            <Icon name="camera" className="h-4 w-4" />{t(s.has_photo || s.status ? 'uxPhotoChoose' : 'uxPhotoChooseFirst')}
            <input type="file" accept="image/jpeg,image/png" className="sr-only" data-photo-input onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) { setMessage(null); setFile(f) } }} />
          </label>
          <p className="text-xs text-slate-600">{t('uxPhotoApprovalNote')}</p>
        </>}
    </div>
  </section>
}
