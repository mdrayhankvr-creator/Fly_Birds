import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../api/client'
import { deleteSchoolLogo, uploadSchoolLogo } from '../api/studentPhotos'
import { Icon } from '../components/icons'
import { ui } from './attendanceUi'

// The school logo in School settings: upload a file (saved at once) or give
// a web link (saved with the form). An uploaded logo's address is
// /api/school/logo?v=…, which pages and documents use like any logo.
export const isUploadedLogo = (url: string) => /^\/api\/school\/logo\?v=[0-9a-f]{12}$/.test(url)

// Scales a large logo down before upload (the server resizes again); PNG
// stays PNG so transparency survives.
async function shrinkLogo(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file)
  try {
    const scale = Math.min(1, 1024 / Math.max(bitmap.width, bitmap.height))
    if (scale === 1 && file.size < 1_500_000) return file
    const canvas = document.createElement('canvas')
    canvas.width = Math.round(bitmap.width * scale); canvas.height = Math.round(bitmap.height * scale)
    const context = canvas.getContext('2d')
    if (!context) return file
    const png = file.type === 'image/png'
    if (!png) { context.fillStyle = '#ffffff'; context.fillRect(0, 0, canvas.width, canvas.height) }
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    return await new Promise<Blob>((resolve, reject) => canvas.toBlob(b => b ? resolve(b) : reject(new Error('encode failed')), png ? 'image/png' : 'image/jpeg', 0.9))
  } finally { bitmap.close() }
}

export function LogoField({ value, disabled, inputClass, authorized, onChange }: { value: string; disabled: boolean; inputClass: string; authorized: <T>(request: (token: string) => Promise<T>) => Promise<T>; onChange: (url: string) => void }) {
  const { t } = useTranslation()
  const cache = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<{ tone: 'ok' | 'error'; text: string } | null>(null)
  const uploaded = isUploadedLogo(value)
  async function run(work: () => Promise<string>, done: string) {
    setBusy(true); setStatus(null)
    try {
      onChange(await work())
      setStatus({ tone: 'ok', text: done })
      await cache.invalidateQueries({ queryKey: ['school', 'branding'] })
    } catch (cause) {
      setStatus({ tone: 'error', text: t(cause instanceof ApiError && cause.status === 413 ? 'uxLogoTooLarge' : 'uxLogoFailed') })
    } finally { setBusy(false) }
  }
  return <div className="sm:col-span-2" data-logo-field>
    <span className="block text-sm font-semibold text-slate-700">{t('uxLogoLabel')}</span>
    <div className="mt-1.5 flex flex-col gap-3 sm:flex-row sm:items-center">
      {uploaded && <img src={value} alt={t('uxLogoCurrent')} className="h-16 w-16 shrink-0 rounded-lg bg-white object-contain ring-1 ring-slate-200" />}
      {uploaded
        ? <p className="min-w-0 flex-1 text-sm text-slate-700">{t('uxLogoUploaded')}</p>
        : <input className={`${inputClass} mt-0 min-w-0 flex-1`} type="url" maxLength={500} placeholder="https://" aria-label={t('uxLogoLink')} value={value} disabled={disabled || busy} onChange={e => onChange(e.target.value)} />}
      {!disabled && <div className="flex flex-wrap gap-2">
        <label className={`${ui.secondary} cursor-pointer ${busy ? 'pointer-events-none opacity-60' : ''}`}>
          <Icon name="camera" className="h-4 w-4" />{t(uploaded ? 'uxLogoReplace' : 'uxLogoUpload')}
          <input type="file" accept="image/png,image/jpeg" className="sr-only" disabled={busy} data-logo-input
            onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) void run(async () => (await authorized(async token => uploadSchoolLogo(token, await shrinkLogo(f)))).logo_url, t('uxLogoSaved')) }} />
        </label>
        {uploaded && <button type="button" className={ui.ghost} disabled={busy} onClick={() => void run(async () => { await authorized(token => deleteSchoolLogo(token)); return '' }, t('uxLogoRemoved'))}>{t('uxLogoRemove')}</button>}
      </div>}
    </div>
    <p className="mt-1 text-xs text-slate-600">{t('uxLogoHint')}</p>
    {status && <p role={status.tone === 'error' ? 'alert' : 'status'} className={`mt-1 text-sm font-semibold ${status.tone === 'error' ? 'text-rose-700' : 'text-emerald-800'}`}>{status.text}</p>}
  </div>
}
