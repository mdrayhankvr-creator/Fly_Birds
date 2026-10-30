import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from './icons'

// An accessible replacement for window.confirm. confirmAction() opens a modal
// dialog and resolves true or false; Escape and the backdrop cancel. One
// <ConfirmHost /> is mounted at the app root.
type Request = { message: string; tone: 'danger' | 'default'; confirmLabel?: string; resolve: (ok: boolean) => void }
let open: ((r: Request) => void) | null = null

export function confirmAction(message: string, options: { tone?: 'danger' | 'default'; confirmLabel?: string } = {}): Promise<boolean> {
  if (!open) return Promise.resolve(window.confirm(message))
  return new Promise(resolve => open!({ message, tone: options.tone ?? 'danger', confirmLabel: options.confirmLabel, resolve }))
}

export function ConfirmHost() {
  const { t } = useTranslation()
  const [request, setRequest] = useState<Request | null>(null)
  const confirmRef = useRef<HTMLButtonElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const returnFocus = useRef<Element | null>(null)
  useEffect(() => { open = r => { returnFocus.current = document.activeElement; setRequest(r) }; return () => { open = null } }, [])
  useEffect(() => {
    if (!request) return
    cancelRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.preventDefault(); close(false) }
      if (e.key === 'Tab') { // keep focus inside the two buttons
        const items = [cancelRef.current, confirmRef.current].filter(Boolean) as HTMLElement[]
        const i = items.indexOf(document.activeElement as HTMLElement)
        e.preventDefault(); items[(i + (e.shiftKey ? items.length - 1 : 1)) % items.length]?.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  })
  function close(ok: boolean) {
    request?.resolve(ok)
    setRequest(null)
    ;(returnFocus.current as HTMLElement | null)?.focus?.()
  }
  if (!request) return null
  const danger = request.tone === 'danger'
  return <div className="fixed inset-0 z-50 flex items-end justify-center p-4 sm:items-center">
    <button type="button" aria-label={t('m12Cancel')} tabIndex={-1} className="absolute inset-0 bg-slate-900/45" onClick={() => close(false)} />
    <div role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-body" className="relative w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-xl">
      <div className="flex items-start gap-4">
        <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-full ${danger ? 'bg-rose-50 text-rose-700' : 'shell-mark'}`}><Icon name="alert" /></span>
        <div className="min-w-0"><h2 id="confirm-title" className="text-base font-semibold text-slate-900">{t(danger ? 'm12ConfirmTitleDanger' : 'm12ConfirmTitle')}</h2>
          <p id="confirm-body" className="mt-1.5 text-sm leading-6 text-slate-600">{request.message}</p></div>
      </div>
      <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <button ref={cancelRef} type="button" onClick={() => close(false)} className="btn btn-secondary">{t('m12Cancel')}</button>
        <button ref={confirmRef} type="button" onClick={() => close(true)}
          className={danger ? 'btn btn-danger-solid' : 'btn btn-primary'}>
          {request.confirmLabel ?? t('m12Continue')}</button>
      </div>
    </div>
  </div>
}
