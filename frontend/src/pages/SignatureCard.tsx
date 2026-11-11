import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { deleteOwnSignature, getOwnSignature, saveOwnSignature } from '../api/marksheetAdmit'
import { Icon } from '../components/icons'
import { CardTitle, Notice, ui } from './attendanceUi'
import { usePeopleApi } from './peopleShared'

// SignatureCard: the signed-in member of staff's own signature, printed on
// the marksheets of the sections they are class teacher of.
export function SignatureCard() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const signature = useQuery({ queryKey: ['ms', 'signature'], queryFn: () => authorized(getOwnSignature), retry: false })
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  useEffect(() => {
    if (!signature.data) { setUrl(''); return }
    const next = URL.createObjectURL(signature.data)
    setUrl(next)
    return () => URL.revokeObjectURL(next)
  }, [signature.data])
  async function run(task: () => Promise<unknown>, done: string) {
    setBusy(true); setMessage(null)
    try {
      await task()
      await client.invalidateQueries({ queryKey: ['ms', 'signature'] })
      setMessage({ tone: 'success', text: done })
    } catch { setMessage({ tone: 'error', text: t('msSignatureInvalid') }) } finally { setBusy(false) }
  }
  return <section className={`${ui.card} p-4 sm:p-6`} aria-labelledby="staff-signature" data-testid="signature-card">
    <CardTitle id="staff-signature" icon="file">{t('msSignatureTitle')}</CardTitle>
    <p className="mt-1 text-sm text-slate-600">{t('msSignatureIntro')}</p>
    <div className="mt-4 flex flex-wrap items-center gap-4">
      {/* A checkerboard behind the image shows its transparent paper. */}
      <div className="grid h-24 w-64 max-w-full place-items-center rounded-xl border border-slate-200 bg-white bg-[length:16px_16px] bg-[image:conic-gradient(#f1f5f9_25%,#fff_0_50%,#f1f5f9_0_75%,#fff_0)] p-2">
        {url ? <img src={url} alt={t('msSignatureAlt')} className="max-h-full max-w-full object-contain" /> : <span className="text-sm text-slate-600">{t('msSignatureNone')}</span>}
      </div>
      <div className="flex flex-wrap gap-2">
        <label className={`${ui.secondary} ${busy ? 'pointer-events-none opacity-50' : ''}`}><Icon name="upload" className="h-4 w-4" />{t(url ? 'msSignatureReplace' : 'msSignatureUpload')}
          <input type="file" accept="image/png,image/jpeg" className="sr-only" disabled={busy}
            onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) void run(() => authorized(token => saveOwnSignature(token, f)), t('msSignatureSaved')) }} /></label>
        {url && <button type="button" className={ui.quiet} disabled={busy} onClick={() => void run(() => authorized(deleteOwnSignature), t('msSignatureRemoved'))}>{t('msSignatureRemove')}</button>}
      </div>
    </div>
    {message && <div className="mt-3"><Notice tone={message.tone}>{message.text}</Notice></div>}
  </section>
}
