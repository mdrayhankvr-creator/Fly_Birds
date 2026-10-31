import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, getAcademicSessions } from '../api/client'
import { admitBulk, getBulkTemplate, previewBulk, type BulkPreview, type BulkResult, type BulkRow } from '../api/bulkAdmission'
import { confirmAction } from '../components/ConfirmDialog'
import { Icon } from '../components/icons'
import { newAdmissionKey } from './admissionModel'
import { usePeopleApi } from './peopleShared'
import { LoginSlipSheet } from './LoginSlips'
import { EmptyState, Notice, PageHeader, SectionCard, StatusBadge, ui, useNumber } from './attendanceUi'

// Bulk admission from a spreadsheet (checkpoint 5): download the template,
// upload it, see every row with its problems, then admit them all at once.
export function BulkAdmissionPage({ canCreateLogins }: { canCreateLogins: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const cache = useQueryClient()
  const num = useNumber()
  const [sessionId, setSessionId] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [key, setKey] = useState(newAdmissionKey)
  const [preview, setPreview] = useState<BulkPreview | null>(null)
  const [result, setResult] = useState<BulkResult | null>(null)
  const [logins, setLogins] = useState(false)
  const [onlyProblems, setOnlyProblems] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const sessions = useQuery({ queryKey: ['bulk', 'sessions'], queryFn: () => authorized(token => getAcademicSessions(token, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  // The current session is the usual choice.
  useEffect(() => {
    if (!sessionId && sessions.data?.items.length) setSessionId((sessions.data.items.find(s => s.is_active) ?? sessions.data.items[0]).id)
  }, [sessions.data, sessionId])

  function failure(cause: unknown) {
    const code = cause instanceof ApiError ? cause.code : ''
    const known = ['sheet_format', 'sheet_not_utf8', 'too_many_rows', 'empty_sheet', 'missing_columns', 'rows_have_errors', 'idempotency_key_reused']
    setError(t(known.includes(code) ? `c5File_${code}` : cause instanceof ApiError && cause.status === 403 ? 'permissionDenied' : 'c5BulkFailed'))
  }
  async function check(chosen: File | null) {
    setFile(chosen); setPreview(null); setResult(null); setError(''); setKey(newAdmissionKey())
    if (!chosen || !sessionId) return
    setBusy(true)
    try { setPreview(await authorized(token => previewBulk(token, sessionId, chosen))) } catch (cause) { failure(cause) } finally { setBusy(false) }
  }
  async function admit() {
    if (!file || !preview || !(await confirmAction(t('c5AdmitConfirm', { count: preview.ready, n: num(preview.ready) }), { tone: 'default', confirmLabel: t('c5AdmitAll', { count: preview.ready, n: num(preview.ready) }) }))) return
    setBusy(true); setError('')
    try {
      setResult(await authorized(token => admitBulk(token, sessionId, key, canCreateLogins && logins, file)))
      setPreview(null)
      await cache.invalidateQueries({ queryKey: ['people'] })
    } catch (cause) {
      failure(cause)
      // The rows changed since the preview (someone else admitted a student): show them again.
      if (cause instanceof ApiError && cause.code === 'rows_have_errors') await check(file)
    } finally { setBusy(false) }
  }
  async function download(format: 'xlsx' | 'csv') {
    try {
      const blob = await authorized(token => getBulkTemplate(token, format))
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url; a.download = `student-admission-template.${format}`; a.click()
      URL.revokeObjectURL(url)
    } catch (cause) { failure(cause) }
  }
  const rows = preview?.rows.filter(r => !onlyProblems || r.status === 'error') ?? []
  return <div className="space-y-6">
    <PageHeader eyebrow={t('m12Group_people')} title={t('c5BulkTitle')} description={t('c5BulkIntro')}
      actions={<Link to="/admin/students" className={ui.secondary}><Icon name="chevronLeft" className="h-4 w-4" />{t('studentsHeading')}</Link>} />

    <SectionCard title={t('c5BulkStep1')} id="bulk-template" icon="download">
      <p className="text-sm text-slate-700">{t('c5BulkTemplateHelp')}</p>
      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" className={ui.primary} onClick={() => void download('xlsx')}><Icon name="download" className="h-4 w-4" />{t('c5TemplateXLSX')}</button>
        <button type="button" className={ui.secondary} onClick={() => void download('csv')}><Icon name="download" className="h-4 w-4" />{t('c5TemplateCSV')}</button>
      </div>
    </SectionCard>

    <SectionCard title={t('c5BulkStep2')} id="bulk-upload" icon="upload">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className={ui.label}>{t('sessionLabel')}<select className={ui.field} value={sessionId} onChange={e => { setSessionId(e.target.value); void check(null) }}>{sessions.data?.items.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
        <label className={ui.label}>{t('c5ChooseFile')}<input type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" className={ui.field} disabled={!sessionId || busy} onChange={e => { const f = e.target.files?.[0] ?? null; e.target.value = ''; void check(f) }} /></label>
      </div>
      <p className="mt-2 text-xs text-slate-500">{t('c5BulkLimits')}</p>
      {busy && !preview && <p role="status" className="mt-3 text-sm font-semibold text-slate-700">{t('c5Checking')}</p>}
      {error && <div className="mt-4"><Notice tone="error">{error}</Notice></div>}
    </SectionCard>

    {preview && <SectionCard title={t('c5BulkStep3')} id="bulk-preview" icon="check">
      <div className="flex flex-wrap items-center gap-2" data-bulk-summary>
        <StatusBadge tone="present">{t('c5ReadyCount', { count: preview.ready, n: num(preview.ready) })}</StatusBadge>
        {preview.skipped > 0 && <StatusBadge tone="unmarked">{t('c5SkipCount', { count: preview.skipped, n: num(preview.skipped) })}</StatusBadge>}
        {preview.errors > 0 && <StatusBadge tone="absent">{t('c5ErrorCount', { count: preview.errors, n: num(preview.errors) })}</StatusBadge>}
        {preview.errors > 0 && <label className="ml-auto flex min-h-11 items-center gap-2 text-sm font-semibold text-slate-700"><input type="checkbox" className="h-5 w-5 accent-[var(--brand-primary)]" checked={onlyProblems} onChange={e => setOnlyProblems(e.target.checked)} />{t('c5OnlyProblems')}</label>}
      </div>
      {preview.unknown_columns.length > 0 && <p className="mt-3 text-sm text-amber-800">{t('c5UnknownColumns', { columns: preview.unknown_columns.join(', ') })}</p>}
      {preview.errors > 0 && <div className="mt-3"><Notice tone="error">{t('c5FixAndUpload')}</Notice></div>}
      <div className="mt-4 overflow-x-auto rounded-xl border border-slate-200" tabIndex={0} role="region" aria-label={t('c5BulkStep3')}>
        <table className="w-full min-w-[46rem] text-left text-sm">
          <thead><tr className="text-xs uppercase tracking-wide text-slate-500"><th className={ui.th}>{t('c5Line')}</th><th className={ui.th}>{t('admissionNo')}</th><th className={ui.th}>{t('c5Student')}</th><th className={ui.th}>{t('c5Placement')}</th><th className={ui.th}>{t('c5Guardian')}</th><th className={ui.th}>{t('status')}</th></tr></thead>
          <tbody>{rows.map(r => <Row key={r.line} row={r} />)}</tbody>
        </table>
      </div>
      {preview.ready > 0 && preview.errors === 0 && <div className="mt-5 flex flex-col gap-3 rounded-2xl border border-slate-200 bg-slate-50 p-4 sm:flex-row sm:items-center sm:justify-between">
        {canCreateLogins ? <label className="flex min-h-11 items-center gap-3 text-sm font-semibold text-slate-800"><input type="checkbox" className="h-5 w-5 accent-[var(--brand-primary)]" checked={logins} onChange={e => setLogins(e.target.checked)} />{t('c5CreateLoginsToo')}</label> : <span />}
        <button type="button" className={`${ui.primary} min-h-12 px-6`} disabled={busy} onClick={() => void admit()}><Icon name="check" className="h-5 w-5" />{busy ? t('c3Saving') : t('c5AdmitAll', { count: preview.ready, n: num(preview.ready) })}</button>
      </div>}
      {preview.ready === 0 && preview.errors === 0 && <div className="mt-4"><EmptyState icon="check" title={t('c5NothingNew')} /></div>}
    </SectionCard>}

    {result && <SectionCard title={t('c5BulkDone')} id="bulk-result" icon="check">
      <Notice tone="success">{t(result.replayed ? 'c5BulkReplayed' : 'c5BulkAdmitted', { admitted: num(result.admitted), skipped: num(result.skipped) })}</Notice>
      {result.logins.length > 0 && <div className="mt-4"><LoginSlipSheet logins={result.logins} /></div>}
      <Link to="/admin/students" className={`${ui.link} mt-4 inline-block text-sm`}>{t('c5OpenStudents')}</Link>
    </SectionCard>}
  </div>
}

function Row({ row: r }: { row: BulkRow }) {
  const { t } = useTranslation()
  const tone = r.status === 'ready' ? 'present' : r.status === 'skip' ? 'unmarked' : 'absent'
  const guardian = r.guardian && (r.guardian.action === 'existing' ? t('c5GuardianExisting', { name: r.guardian.existing_name }) : r.guardian.action === 'same_as_row' ? t('c5GuardianSameAs', { name: r.guardian.name, line: r.guardian.same_as_line }) : t('c5GuardianNew', { name: r.guardian.name }))
  return <tr className={`border-t border-slate-100 align-top ${r.status === 'error' ? 'bg-rose-50/60' : ''}`} data-bulk-line={r.line} data-bulk-status={r.status}>
    <td className={`${ui.td} tabular-nums`}>{r.line}</td>
    <td className={`${ui.td} font-mono`}>{r.admission_no || '—'}</td>
    <td className={ui.td}><span className="block font-semibold text-slate-900">{r.name_en || '—'}</span>{r.name_bn && <span lang="bn" className="block text-slate-700">{r.name_bn}</span>}{r.login_email && <span className="block text-xs text-slate-500">{r.login_email}</span>}</td>
    <td className={ui.td}>{[r.class_name, r.section_name].filter(Boolean).join(' · ') || '—'}{r.roll_no != null && <span className="block text-xs text-slate-500">{t('rollNo')} {r.roll_no}</span>}</td>
    <td className={`${ui.td} text-xs text-slate-700`}>{guardian ? `${guardian}${r.guardian?.relationship ? ` · ${r.guardian.relationship}` : ''}` : '—'}</td>
    <td className={ui.td}><StatusBadge tone={tone}>{t(`c5Status_${r.status}`)}</StatusBadge>
      {r.errors.length > 0 && <ul className="mt-1.5 space-y-0.5 text-xs font-semibold text-rose-800">{r.errors.map(code => <li key={code}>{t(`c5Err_${code}`, { defaultValue: code })}</li>)}</ul>}</td>
  </tr>
}
