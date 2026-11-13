import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, setLoginPhone } from '../api/client'
import { usePeopleApi } from './peopleShared'
import { Notice, ui } from './attendanceUi'

// The number an account signs in with. It lives on the user record, so the
// same card serves the student profile (usually the guardian's number, which
// siblings may share) and the staff profile (unique to that member of staff).
// Administrators only; accounts without a login have nothing to set.
export function LoginPhoneCard({ userId, current, kind }: { userId: string; current: string; kind: 'student' | 'staff' }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [value, setValue] = useState(current)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  async function save(event: FormEvent) {
    event.preventDefault()
    setBusy(true); setError(''); setMessage('')
    try {
      const saved = await authorized(token => setLoginPhone(token, userId, value.trim()))
      setValue(saved.login_phone ?? '')
      setMessage(t(saved.login_phone ? 'phLoginPhoneSaved' : 'phLoginPhoneCleared'))
      await cache.invalidateQueries({ queryKey: ['people'] })
      await cache.invalidateQueries({ queryKey: ['staff'] })
    } catch (cause) {
      if (cause instanceof ApiError && cause.code === 'duplicate_user') setError(t('phLoginPhoneTaken'))
      else if (cause instanceof ApiError && cause.status === 400) setError(t('phLoginPhoneInvalid'))
      else setError(t('peopleSaveError'))
    } finally { setBusy(false) }
  }

  return <form className="space-y-2" onSubmit={e => void save(e)}>
    <label className={ui.label} htmlFor={`login-phone-${userId}`}>{t('phLoginPhone')}
      <input id={`login-phone-${userId}`} type="tel" inputMode="tel" autoComplete="off" maxLength={20} className={ui.field}
        value={value} placeholder="01711-000000" onChange={e => setValue(e.target.value)} disabled={busy} />
    </label>
    <p className="text-xs text-slate-600">{t(kind === 'staff' ? 'phLoginPhoneStaffHint' : 'phLoginPhoneHint')}</p>
    {error && <Notice tone="error">{error}</Notice>}
    {message && <Notice tone="success">{message}</Notice>}
    <button disabled={busy || value.trim() === current.trim()} className={ui.secondary}>{t('phLoginPhoneSave')}</button>
  </form>
}
