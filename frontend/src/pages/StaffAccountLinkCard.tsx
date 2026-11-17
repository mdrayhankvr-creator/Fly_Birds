import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getUsers, type ManagedUser } from '../api/client'
import { linkStaffAccount, type StaffProfile } from '../api/staffProfile'
import { confirmAction } from '../components/ConfirmDialog'
import { usePeopleApi } from './peopleShared'
import { CardTitle, Notice, ui } from './attendanceUi'

// Only the administrator sees this card. The server repeats every identity,
// role, permission and uniqueness check inside the linking transaction.
export function StaffAccountLinkCard({ profile }: { profile: StaffProfile }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'teacher' | 'head_teacher'>('teacher')
  const [code, setCode] = useState('')
  const [candidate, setCandidate] = useState<ManagedUser | null>(null)
  const [searched, setSearched] = useState(false)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)

  async function search(event: FormEvent) {
    event.preventDefault()
    setCandidate(null); setSearched(false); setMessage(null); setBusy(true)
    try {
      const exact = email.trim().toLowerCase()
      const params = new URLSearchParams({ search: exact, role, status: 'active', page: '1', page_size: '20' })
      const page = await authorized(token => getUsers(token, params))
      setCandidate(page.items.find(user => user.email.toLowerCase() === exact && user.role === role && user.is_active) ?? null)
      setSearched(true)
    } catch { setMessage({ tone: 'error', text: t('staffLoadError') }) }
    finally { setBusy(false) }
  }

  async function link() {
    if (!candidate || code.trim().toUpperCase() !== profile.employee_code) {
      setMessage({ tone: 'error', text: t('staffAccountLinkFailed') }); return
    }
    const accepted = await confirmAction(t('staffAccountConfirm', { code: profile.employee_code, email: candidate.email, role: t(`role_${candidate.role}`) }), { tone: 'default', confirmLabel: t('staffAccountLink') })
    if (!accepted) return
    setBusy(true); setMessage(null)
    try {
      await authorized(token => linkStaffAccount(token, profile.id, { user_id: candidate.id, confirm_email: candidate.email, confirm_employee_code: code.trim().toUpperCase() }))
      await cache.invalidateQueries({ queryKey: ['staff-profile'] })
      await cache.invalidateQueries({ queryKey: ['staff', 'employees'] })
      setMessage({ tone: 'success', text: t('staffAccountLinkedSuccess') })
    } catch { setMessage({ tone: 'error', text: t('staffAccountLinkFailed') }) }
    finally { setBusy(false) }
  }

  return <section className={`${ui.card} min-w-0 p-4 sm:p-6`} aria-labelledby="staff-account-link-title">
    <CardTitle id="staff-account-link-title" icon="user">{t('staffAccountTitle')}</CardTitle>
    {message && <div className="mt-3"><Notice tone={message.tone}>{message.text}</Notice></div>}
    {profile.user_id ? <div className="mt-4 min-w-0">
      <p className="text-sm font-semibold text-slate-800">{t('staffAccountLinked')}</p>
      <p className="break-all text-sm text-slate-600">{profile.linked_account_email || t('staffAccountLinked')}</p>
      {profile.linked_account_role && <p className="text-xs text-slate-500">{t(`role_${profile.linked_account_role}`)}</p>}
    </div> : <>
      <p className="mt-2 text-sm text-slate-600">{t('staffAccountUnlinked')} {t('staffAccountHint')}</p>
      <form className="mt-4 grid gap-3 sm:grid-cols-2" onSubmit={event => void search(event)}>
        <label className={`${ui.label} min-w-0`}>{t('staffAccountEmail')}<input type="email" autoComplete="off" required maxLength={254} className={ui.field} value={email} onChange={event => { setEmail(event.target.value); setCandidate(null); setSearched(false) }} /></label>
        <label className={ui.label}>{t('staffAccountRole')}<select className={ui.field} value={role} onChange={event => { setRole(event.target.value as typeof role); setCandidate(null); setSearched(false) }}><option value="teacher">{t('role_teacher')}</option><option value="head_teacher">{t('role_head_teacher')}</option></select></label>
        <button type="submit" disabled={busy} className={`${ui.secondary} w-fit sm:col-span-2`}>{t('staffAccountSearch')}</button>
      </form>
      {searched && !candidate && <p className="mt-3 text-sm text-slate-600">{t('staffAccountNoMatch')}</p>}
      {candidate && <div className="mt-4 min-w-0 rounded-xl border border-slate-200 bg-slate-50 p-4">
        <p className="font-semibold text-slate-900">{candidate.display_name || candidate.email}</p>
        <p className="break-all text-sm text-slate-700">{candidate.email}</p>
        <p className="text-xs text-slate-600">{t(`role_${candidate.role}`)}</p>
        <label className={`${ui.label} mt-3 block`}>{t('staffAccountCodeConfirm')}<input autoComplete="off" maxLength={32} className={ui.field} value={code} onChange={event => setCode(event.target.value)} /></label>
        <button type="button" className={`${ui.primary} mt-3`} disabled={busy || code.trim().toUpperCase() !== profile.employee_code} onClick={() => void link()}>{t('staffAccountChoose')}</button>
      </div>}
    </>}
  </section>
}
