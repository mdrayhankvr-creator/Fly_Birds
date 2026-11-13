import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { approvePhoto, getPhotoReviews, getReviewImage, rejectPhoto, type PhotoReview } from '../api/studentPhotos'
import { Icon } from '../components/icons'
import { EmptyState, Notice, PageHeader, Skeleton, ui } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

// Student photos waiting for review: the class teacher sees their own
// sections, the administrator every section. Approving puts the photo into
// use; rejecting needs a reason, which the student sees.

const quickReasons = ['uxReasonFace', 'uxReasonBackground', 'uxReasonOthers', 'uxReasonOld'] as const
const MAX_REASON = 300

function ReviewRow({ item, onDone }: { item: PhotoReview; onDone: (text: string) => Promise<void> }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const image = useQuery({ queryKey: ['people', 'review-image', item.student_id, item.submitted_at], queryFn: () => authorized(token => getReviewImage(token, item.student_id)), retry: false })
  const [url, setUrl] = useState('')
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!image.data) { setUrl(''); return }
    const u = URL.createObjectURL(image.data); setUrl(u)
    return () => URL.revokeObjectURL(u)
  }, [image.data])
  const name = i18n.language === 'bn' && item.name_bn ? item.name_bn : item.name_en
  const place = [item.class_name, item.section_name, item.roll_no == null ? '' : `${t('rollNo')} ${new Intl.NumberFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-US').format(item.roll_no)}`].filter(Boolean).join(' · ')
  const when = new Intl.DateTimeFormat(i18n.language === 'bn' ? 'bn-BD' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Dhaka' }).format(new Date(item.submitted_at))
  async function act(kind: 'approve' | 'reject') {
    setBusy(true); setError('')
    try {
      if (kind === 'approve') await authorized(token => approvePhoto(token, item.student_id))
      else await authorized(token => rejectPhoto(token, item.student_id, reason.trim()))
      await onDone(t(kind === 'approve' ? 'uxReviewApproved' : 'uxReviewRejected', { name }))
    } catch (cause) {
      setError(t(cause instanceof ApiError && cause.code === 'not_pending' ? 'uxReviewGone' : 'uxReviewFailed'))
      setBusy(false)
    }
  }
  return <li className="rounded-2xl border border-slate-200 bg-white p-4" data-photo-review={item.student_id}>
    <div className="flex flex-col gap-4 sm:flex-row">
      <div className="h-40 w-40 shrink-0 overflow-hidden rounded-xl bg-slate-100 ring-1 ring-slate-200">
        {url ? <img src={url} alt={t('uxReviewPhotoOf', { name })} className="h-full w-full object-cover" /> : <span className="grid h-full place-items-center text-xs text-slate-500">{image.isError ? t('uxReviewNoImage') : '…'}</span>}
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        <p className="text-lg font-bold text-slate-900" style={{ overflowWrap: 'anywhere' }}>{name}</p>
        <p className="text-sm text-slate-600">{place}</p>
        <p className="text-xs text-slate-600">{t('uxReviewSent', { date: when })}{item.has_photo ? ` · ${t('uxReviewReplaces')}` : ''}</p>
        {error && <Notice tone="error">{error}</Notice>}
        {!rejecting ? <div className="flex flex-wrap gap-2">
          <button type="button" className={ui.primary} disabled={busy} onClick={() => void act('approve')}><Icon name="check" className="h-4 w-4" />{t('uxReviewApprove')}</button>
          <button type="button" className={ui.secondary} disabled={busy} onClick={() => setRejecting(true)}>{t('uxReviewReject')}</button>
        </div> : <form className="space-y-2" onSubmit={e => { e.preventDefault(); if (reason.trim()) void act('reject') }}>
          <div className="flex flex-wrap gap-1.5" role="group" aria-label={t('uxReviewQuickReasons')}>
            {quickReasons.map(k => <button key={k} type="button" className="rounded-full border border-slate-300 bg-white px-3 py-1 text-xs font-semibold text-slate-800 hover:bg-slate-50" onClick={() => setReason(t(k))}>{t(k)}</button>)}
          </div>
          <label className={ui.label}>{t('uxReviewReason')}
            <textarea className={`${ui.field} min-h-20`} required maxLength={MAX_REASON} value={reason} onChange={e => setReason(e.target.value)} data-reject-reason /></label>
          <div className="flex flex-wrap gap-2">
            <button className={ui.danger} disabled={busy || !reason.trim()}>{t('uxReviewSendReject')}</button>
            <button type="button" className={ui.secondary} disabled={busy} onClick={() => { setRejecting(false); setReason('') }}>{t('cancel')}</button>
          </div>
        </form>}
      </div>
    </div>
  </li>
}

export function PhotoReviewsPage() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const reviews = useQuery({ queryKey: ['people', 'photo-reviews'], queryFn: () => authorized(token => getPhotoReviews(token)) })
  const [message, setMessage] = useState('')
  async function done(text: string) {
    setMessage(text)
    await cache.invalidateQueries({ queryKey: ['people', 'photo-reviews'] })
  }
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_people')} title={t('uxReviewTitle')} description={t('uxReviewIntro')} icon="camera" />
    {message && <Notice tone="success">{message}</Notice>}
    <section className={`${ui.card} p-4 sm:p-6`} aria-label={t('uxReviewTitle')}>
      {reviews.isPending ? <Skeleton rows={3} /> : reviews.isError ? <Notice tone="error">{t('uxReviewLoadError')}</Notice>
        : reviews.data.items.length === 0 ? <EmptyState icon="camera" title={t('uxReviewNone')}>{t('uxReviewNoneHelp')}</EmptyState>
        : <ul className="space-y-3">{reviews.data.items.map(item => <ReviewRow key={item.student_id} item={item} onDone={done} />)}</ul>}
    </section>
  </div>
}
