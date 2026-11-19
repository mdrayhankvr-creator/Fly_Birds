import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError } from '../api/client'
import { applyTeacherSetup, previewTeacherSetup, type TeacherPlan } from '../api/teacherSetup'
import { confirmAction } from '../components/ConfirmDialog'
import { Icon } from '../components/icons'
import { newAdmissionKey } from './admissionModel'
import { Notice, PageHeader, SectionCard, ui, useNumber } from './attendanceUi'
import { usePeopleApi } from './peopleShared'
import { decryptCredentialList, encryptCredentialList, teacherCredentialCSV } from './studentCredentialExport'

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url; link.download = filename
  document.body.append(link); link.click(); link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

export function TeacherBulkSetupPage() {
  const { t, i18n } = useTranslation()
  const number = useNumber()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [plan, setPlan] = useState<TeacherPlan | null>(null)
  const [key, setKey] = useState(newAdmissionKey)
  const [created, setCreated] = useState<number | null>(null)
  const [replayed, setReplayed] = useState(false)
  const [encrypted, setEncrypted] = useState<Blob | null>(null)
  const [passphrase, setPassphrase] = useState('')
  const [confirmPassphrase, setConfirmPassphrase] = useState('')
  const [unlockFile, setUnlockFile] = useState<File | null>(null)
  const [unlockPassphrase, setUnlockPassphrase] = useState('')
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [unlockError, setUnlockError] = useState('')

  useEffect(() => {
    if (created === null || replayed || saved) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [created, replayed, saved])

  async function preview() {
    if (busy) return
    setBusy(true); setError(''); setPlan(null); setKey(newAdmissionKey())
    try { setPlan(await authorized(previewTeacherSetup)) }
    catch (cause) { setError(t(cause instanceof ApiError && cause.status === 409 ? 'teacherSetupConflict' : 'teacherSetupPreviewFailed')) }
    finally { setBusy(false) }
  }

  async function apply() {
    if (!plan?.to_add || busy || passphrase.length < 16 || passphrase !== confirmPassphrase) return
    setBusy(true); setError('')
    let applied = false
    try {
      // Check the browser's crypto support and passphrase before any write.
      await encryptCredentialList('preflight', passphrase)
      const approved = await confirmAction(t('teacherSetupConfirm', { count: number(plan.to_add) }), { tone: 'default', confirmLabel: t('teacherSetupApply', { count: number(plan.to_add) }) })
      if (!approved) return
      const result = await authorized(token => applyTeacherSetup(token, { fingerprint: plan.fingerprint, idempotency_key: key, confirm: true }))
      applied = true
      setCreated(result.created); setReplayed(result.replayed); setPlan(null)
      setPassphrase(''); setConfirmPassphrase('')
      if (result.credentials.length) {
        // Plaintext exists only in this response and local stack frame. The
        // passphrase and encrypted file are never sent back to the server.
        const file = await encryptCredentialList(teacherCredentialCSV(result.credentials), passphrase)
        setEncrypted(file)
        downloadBlob(file, 'TEST-teacher-credentials.fbcred')
      }
      await cache.invalidateQueries({ queryKey: ['staff'] })
    } catch (cause) {
      if (applied) setError(t('teacherSetupExportFailure'))
      else {
        if (cause instanceof ApiError && cause.code === 'setup_stale') { setPlan(null); setKey(newAdmissionKey()) }
        setError(t(cause instanceof ApiError && cause.code === 'setup_stale' ? 'teacherSetupStale' : cause instanceof ApiError && cause.status === 409 ? 'teacherSetupConflict' : 'teacherSetupFailed'))
      }
    } finally { setBusy(false) }
  }

  async function unlock() {
    if (!unlockFile || !unlockPassphrase || busy) return
    setBusy(true); setUnlockError('')
    try {
      const csv = await decryptCredentialList(unlockFile, unlockPassphrase)
      downloadBlob(new Blob([csv], { type: 'text/csv;charset=utf-8' }), 'TEST-teacher-credentials-unlocked.csv')
      setUnlockPassphrase('')
    } catch { setUnlockError(t('teacherSetupUnlockFailed')) }
    finally { setBusy(false) }
  }

  return <div className="min-w-0 space-y-6">
    <PageHeader eyebrow={t('m12Group_people')} title={t('teacherSetupTitle')} description={t('teacherSetupIntro')} actions={<Link to="/admin/staff" aria-disabled={created !== null && !replayed && !saved} onClick={event => { if (created !== null && !replayed && !saved) event.preventDefault() }} className={ui.secondary}>{t('m12Nav_staff')}</Link>} />
    {error && <Notice tone="error">{error}</Notice>}
    {created === null && <SectionCard title={t('teacherSetupPreview')} icon="teacher">
      <button type="button" data-teacher-preview className={ui.primary} disabled={busy} onClick={() => void preview()}>{busy ? t('teacherSetupWorking') : t('teacherSetupPreview')}</button>
    </SectionCard>}
    {plan && <>
      <div className="grid gap-3 sm:grid-cols-3">{([['teacherSetupExisting', plan.existing], ['teacherSetupToAdd', plan.to_add], ['teacherSetupTarget', plan.target]] as const).map(([label, value]) => <div className={`${ui.card} p-4`} key={label}><p className="text-sm text-slate-600">{t(label)}</p><p className="mt-1 text-3xl font-bold text-slate-900">{number(value)}</p></div>)}</div>
      <SectionCard title={t('teacherSetupExistingList')} icon="teacher">
        <div className="grid gap-2 sm:grid-cols-2">{plan.teachers.map(teacher => <div className="min-w-0 rounded-xl border border-slate-200 p-3 text-sm" key={teacher.id}><p className="break-words font-semibold">{teacher.name_en} · {teacher.employee_code}</p><p className="text-slate-600">{t(teacher.linked_account ? 'teacherSetupLinked' : 'teacherSetupUnlinked')}{teacher.linked_account ? ` · ${teacher.linked_role}` : ''} · {t(teacher.active ? 'staffActive' : 'staffInactive')}</p></div>)}</div>
      </SectionCard>
      <SectionCard title={t('teacherSetupCandidates')} icon="check">
        <div data-teacher-candidates className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{plan.candidates.map(candidate => <div className="min-w-0 rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm" key={candidate.employee_code}><strong className="break-words">{i18n.language === 'bn' ? candidate.name_bn : candidate.name_en}</strong><p lang={i18n.language === 'bn' ? 'en' : 'bn'} className="break-words text-slate-600">{i18n.language === 'bn' ? candidate.name_en : candidate.name_bn}</p><p className="font-mono">{candidate.employee_code}</p><p className="break-all text-slate-600">{candidate.login_id}</p><span className="font-bold text-amber-900">{candidate.test_marker}</span></div>)}</div>
        <p className="mt-4 break-all text-xs text-slate-500">{t('teacherSetupFingerprint')}: {plan.fingerprint}</p>
      </SectionCard>
      <Notice tone="warning">{t('teacherSetupMissing')}</Notice>
      {plan.to_add > 0 ? <SectionCard title={t('teacherSetupExportTitle')} icon="lock">
        <p className="text-sm text-slate-700">{t('teacherSetupExportIntro')}</p>
        <div className="mt-3 grid gap-3 sm:grid-cols-2"><label className={ui.label}>{t('teacherSetupPassphrase')}<input type="password" autoComplete="new-password" className={ui.field} minLength={16} maxLength={256} value={passphrase} onChange={event => setPassphrase(event.target.value)} /></label><label className={ui.label}>{t('teacherSetupConfirmPassphrase')}<input type="password" autoComplete="new-password" className={ui.field} value={confirmPassphrase} onChange={event => setConfirmPassphrase(event.target.value)} /></label></div>
        <p className="mt-2 text-sm text-slate-600">{t('teacherSetupPassphraseWarning')}</p>
        <button type="button" data-teacher-apply data-teacher-key={key} className={`${ui.primary} mt-4`} disabled={busy || passphrase.length < 16 || passphrase !== confirmPassphrase} onClick={() => void apply()}>{t('teacherSetupApply', { count: number(plan.to_add) })}</button>
      </SectionCard> : <Notice tone="success">{t('teacherSetupNoChanges')}</Notice>}
    </>}
    {created !== null && <SectionCard title={t('teacherSetupDone')} icon="check">
      <Notice tone={replayed ? 'warning' : 'success'}>{t(replayed ? 'teacherSetupRetry' : 'teacherSetupCreated', { count: number(created) })}</Notice>
      {encrypted && <><p className="mt-3 text-sm">{t('teacherSetupOnce')}</p><button type="button" data-teacher-download className={`${ui.primary} mt-3`} onClick={() => downloadBlob(encrypted, 'TEST-teacher-credentials.fbcred')}><Icon name="download" className="h-4 w-4" />{t('teacherSetupDownload')}</button><label className="mt-4 flex gap-2 text-sm"><input type="checkbox" checked={saved} onChange={event => setSaved(event.target.checked)} />{t('teacherSetupSaved')}</label></>}
      <p className="mt-4 font-semibold">{t('teacherSetupChecklistTitle')}</p><ul className="mt-2 list-disc space-y-1 ps-5 text-sm text-slate-700"><li>{t('teacherSetupChecklistIdentity')}</li><li>{t('teacherSetupChecklistSubjects')}</li><li>{t('teacherSetupChecklistRooms')}</li></ul>
    </SectionCard>}
    <SectionCard title={t('teacherSetupUnlockTitle')} icon="lock">
      <p className="text-sm text-slate-600">{t('teacherSetupUnlockIntro')}</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-2"><label className={ui.label}>{t('teacherSetupEncryptedFile')}<input type="file" accept=".fbcred" className={ui.field} onChange={event => setUnlockFile(event.target.files?.[0] ?? null)} /></label><label className={ui.label}>{t('teacherSetupPassphrase')}<input type="password" autoComplete="off" className={ui.field} value={unlockPassphrase} onChange={event => setUnlockPassphrase(event.target.value)} /></label></div>
      {unlockError && <Notice tone="error">{unlockError}</Notice>}
      <button type="button" className={`${ui.secondary} mt-3`} disabled={busy || !unlockFile || !unlockPassphrase} onClick={() => void unlock()}>{t('teacherSetupUnlock')}</button>
    </SectionCard>
  </div>
}
