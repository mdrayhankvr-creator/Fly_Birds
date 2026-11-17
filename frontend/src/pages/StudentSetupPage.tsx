import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ApiError, getAcademicSessions } from '../api/client'
import { applyStudentSetup, previewStudentSetup, type StudentSetupPlan, type StudentSetupResult } from '../api/studentSetup'
import { confirmAction } from '../components/ConfirmDialog'
import { Icon } from '../components/icons'
import { newAdmissionKey } from './admissionModel'
import { usePeopleApi } from './peopleShared'
import { LoginSlipSheet } from './LoginSlips'
import { Notice, PageHeader, SectionCard, ui, useNumber } from './attendanceUi'
import { credentialCSV, decryptCredentialList, encryptCredentialList } from './studentCredentialExport'

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url; anchor.download = filename
  document.body.append(anchor); anchor.click(); anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

export function StudentSetupPage() {
  const { t, i18n } = useTranslation()
  const num = useNumber()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const [sessionID, setSessionID] = useState('')
  const [plan, setPlan] = useState<StudentSetupPlan | null>(null)
  const [result, setResult] = useState<StudentSetupResult | null>(null)
  const [resultSession, setResultSession] = useState('')
  const [key, setKey] = useState(newAdmissionKey)
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)
  const [exportBusy, setExportBusy] = useState(false)
  const [passphrase, setPassphrase] = useState('')
  const [restorePassphrase, setRestorePassphrase] = useState('')
  const [restoreFile, setRestoreFile] = useState<File | null>(null)
  const [exportError, setExportError] = useState('')
  const [restoreError, setRestoreError] = useState('')
  const [error, setError] = useState('')
  const [conflicts, setConflicts] = useState<{ kind: string; value: string; reason: string }[]>([])
  const sessions = useQuery({ queryKey: ['student-setup', 'sessions'], queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const active = sessions.data?.items.find(item => item.is_active)

  useEffect(() => { if (active && !sessionID) setSessionID(active.id) }, [active, sessionID])
  useEffect(() => {
    if (!result?.logins.length || saved) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [result, saved])

  async function preview() {
    if (!sessionID || busy) return
    setBusy(true); setError(''); setConflicts([]); setPlan(null); setKey(newAdmissionKey())
    try { setPlan(await authorized(token => previewStudentSetup(token, sessionID))) }
    catch (cause) {
      if (cause instanceof ApiError && Array.isArray(cause.details?.conflicts)) setConflicts(cause.details.conflicts as { kind: string; value: string; reason: string }[])
      setError(t(cause instanceof ApiError && cause.status === 409 ? 'setupConflicts' : 'setupPreviewError'))
    }
    finally { setBusy(false) }
  }

  async function apply() {
    if (!plan || busy || !plan.to_add) return
    const confirmed = await confirmAction(t('setupConfirm', { count: num(plan.to_add), session: plan.session }), { tone: 'default', confirmLabel: t('setupApply', { count: num(plan.to_add) }) })
    if (!confirmed) return
    setBusy(true); setError('')
    try {
      const created = await authorized(token => applyStudentSetup(token, { session_id: plan.session_id, fingerprint: plan.fingerprint, idempotency_key: key, confirm: true }))
      setResultSession(plan.session); setResult(created); setPlan(null)
      await cache.invalidateQueries({ queryKey: ['people'] })
    } catch (cause) {
      if (cause instanceof ApiError && cause.code === 'setup_stale') { setPlan(null); setKey(newAdmissionKey()) }
      setError(t(cause instanceof ApiError && cause.code === 'setup_stale' ? 'setupStale' : cause instanceof ApiError && cause.status === 409 ? 'setupConflicts' : cause instanceof ApiError && cause.status === 400 ? 'setupInvalid' : 'setupFailed'))
    } finally { setBusy(false) }
  }

  async function download() {
    if (!result?.logins.length || exportBusy) return
    setExportBusy(true); setExportError('')
    try {
      const file = await encryptCredentialList(credentialCSV(result.logins, resultSession), passphrase)
      downloadBlob(file, `TEST-student-credentials-${sessionID}.fbcred`)
      setPassphrase('')
    } catch { setExportError(t('setupExportFailed')) }
    finally { setExportBusy(false) }
  }

  async function restore() {
    if (!restoreFile || exportBusy) return
    setExportBusy(true); setRestoreError('')
    try {
      const csv = await decryptCredentialList(restoreFile, restorePassphrase)
      downloadBlob(new Blob([csv], { type: 'text/csv;charset=utf-8' }), 'TEST-student-credentials-unlocked.csv')
      setRestorePassphrase('')
    } catch { setRestoreError(t('setupUnlockFailed')) }
    finally { setExportBusy(false) }
  }

  return <div className="min-w-0 space-y-6">
    <PageHeader eyebrow={t('m12Group_people')} title={t('setupTitle')} description={t('setupIntro')} actions={!result ? <Link to="/admin/students" className={ui.secondary}>{t('studentsHeading')}</Link> : undefined} />
    {error && <Notice tone="error">{error}</Notice>}
    {!!conflicts.length && <ul className={`${ui.card} list-disc space-y-1 p-4 ps-8 text-sm text-rose-800`}>{conflicts.map((item, index) => <li key={`${item.kind}-${item.value}-${index}`} className="break-words">{t(`setupConflictKind_${item.kind}`, { defaultValue: item.kind })}: {item.value} — {t(`setupConflictReason_${item.reason}`, { defaultValue: item.reason })}</li>)}</ul>}
    {!result && <SectionCard title={t('setupPreview')} icon="check">
      <label className={ui.label}>{t('setupSession')}<select className={ui.field} value={sessionID} disabled={busy} onChange={event => { setSessionID(event.target.value); setPlan(null) }}>
        <option value="">{t('setupSession')}</option>{sessions.data?.items.filter(item => item.is_active).map(item => <option key={item.id} value={item.id}>{item.name}</option>)}
      </select></label>
      {sessions.isError && <p role="alert" className="mt-2 text-rose-700">{t('setupPreviewError')}</p>}
      <button type="button" className={`${ui.primary} mt-4`} disabled={!sessionID || busy} onClick={() => void preview()}>{busy ? t('setupPreviewing') : t('setupPreview')}</button>
    </SectionCard>}
    {plan && <>
      <div className="grid gap-3 sm:grid-cols-3">
        {([['setupExisting', plan.existing], ['setupToAdd', plan.to_add], ['setupTarget', 15]] as const).map(([label, value]) => <div className={`${ui.card} p-4`} key={label}><p className="text-sm text-slate-600">{t(label)}</p><p className="mt-1 text-3xl font-bold text-slate-900">{num(value)}</p></div>)}
      </div>
      <div data-setup-plan><SectionCard title={t('setupPlanHeading')} icon="check">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{plan.sections.map(section => <div className="min-w-0 rounded-xl border border-slate-200 p-3" key={section.section_id}>
          <p className="break-words font-semibold text-slate-900">{section.class} · {section.section}</p><p className="mt-1 text-sm text-slate-600">{t('setupExisting')}: {num(section.existing)} · {t('setupToAdd')}: {num(section.to_add)}</p>
        </div>)}</div>
        {plan.students.length > 0 && <details className="mt-4"><summary className="cursor-pointer font-semibold text-slate-800">{t('setupPlanHeading')} ({num(plan.students.length)})</summary>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">{plan.students.map(student => <div className="min-w-0 rounded-xl border border-slate-200 p-3 text-sm" key={student.admission_no}>
            <p className="break-all font-semibold text-slate-900">{i18n.language === 'bn' ? student.name_bn : student.name_en}</p><p lang={i18n.language === 'bn' ? 'en' : 'bn'} className="text-slate-600">{i18n.language === 'bn' ? student.name_en : student.name_bn}</p><p className="text-slate-600">{student.class} · {student.section} · {t('setupRoll')} {num(student.roll_no)} · {t(`gender_${student.gender}`)}</p><p className="break-all font-mono text-xs text-slate-600">{student.admission_no}</p>
          </div>)}</div>
        </details>}
        <p className="mt-4 break-all text-xs text-slate-500">{t('setupPreviewFingerprint')}: {plan.fingerprint}</p>
      </SectionCard></div>
      <Notice tone="warning"><strong>{t('setupIncomplete')}:</strong> {t('setupMissing')}</Notice>
      <p className="text-sm text-slate-600">{t('setupNoGuardian')}</p>
      {plan.to_add ? <button type="button" data-setup-apply data-setup-key={key} className={ui.primary} disabled={busy} onClick={() => void apply()}>{busy ? t('setupWorking') : t('setupApply', { count: num(plan.to_add) })}</button> : <Notice tone="success">{t('setupNoChanges')}</Notice>}
    </>}
    {result && <SectionCard title={t('setupTitle')} icon="check">
      <Notice tone={result.replayed ? 'warning' : 'success'}>{t(result.replayed ? 'setupRetry' : 'setupSuccess', { count: num(result.admitted) })}</Notice>
      {!!result.logins.length && <>
        <p className="mt-4 text-sm font-semibold text-amber-900">{t('setupOnce')}</p>
        <label className={`${ui.label} mt-3 block`}>{t('setupExportPassphrase')}<input type="password" autoComplete="off" minLength={16} maxLength={256} className={ui.field} value={passphrase} onChange={event => setPassphrase(event.target.value)} /></label>
        <p className="mt-2 text-sm text-slate-600">{t('setupExportWarning')}</p>
        {exportError && <Notice tone="error">{exportError}</Notice>}
        <button type="button" data-setup-download className={`${ui.primary} mt-3`} disabled={exportBusy || passphrase.length < 16} onClick={() => void download()}><Icon name="download" className="h-4 w-4" />{t('setupDownloadEncrypted')}</button>
        <div className="mt-5"><LoginSlipSheet logins={result.logins} /></div>
        <label className="mt-5 flex items-center gap-3 text-sm font-semibold text-slate-800"><input type="checkbox" checked={saved} onChange={event => setSaved(event.target.checked)} />{t('setupSaved')}</label>
      </>}
      <Link to="/admin/students" aria-disabled={!!result.logins.length && !saved} onClick={event => { if (result.logins.length && !saved) event.preventDefault() }} className={`${ui.secondary} mt-4 inline-flex ${result.logins.length && !saved ? 'pointer-events-none opacity-50' : ''}`}>{t('setupDone')}</Link>
    </SectionCard>}
    <SectionCard title={t('setupUnlockTitle')} icon="lock">
      <p className="text-sm text-slate-600">{t('setupUnlockIntro')}</p>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <label className={ui.label}>{t('setupEncryptedFile')}<input type="file" accept=".fbcred" className={ui.field} onChange={event => setRestoreFile(event.target.files?.[0] ?? null)} /></label>
        <label className={ui.label}>{t('setupExportPassphrase')}<input type="password" autoComplete="off" className={ui.field} value={restorePassphrase} onChange={event => setRestorePassphrase(event.target.value)} /></label>
      </div>
      {restoreError && <Notice tone="error">{restoreError}</Notice>}
      <button type="button" className={`${ui.secondary} mt-3`} disabled={exportBusy || !restoreFile || !restorePassphrase} onClick={() => void restore()}>{t('setupUnlock')}</button>
      <p className="mt-4 text-sm text-slate-600">{t('setupResetHint')} <Link className="font-semibold underline" to="/admin/users">{t('setupResetAccounts')}</Link></p>
    </SectionCard>
  </div>
}
