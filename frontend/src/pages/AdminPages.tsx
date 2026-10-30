import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'
import { ApiError } from '../api/client'
import {
  createBackup, deleteBackup, downloadBackup, downloadExport, formatBytes, getAdminOverview, getAuditActions, getAuditEvents, getBackups,
  getExportDatasets, getManagedModules, saveBlob, setManagedModule, verifyBackup, type AuditItem, type BackupRun, type ManagedModule,
} from '../api/admin'
import { CardTitle, EmptyState, Notice, PageHeader, Pager, Skeleton, ui } from './attendanceUi'
import { shiftDay, todayDhaka, useApi, useDhakaFormat } from './commShared'

function useAdminError() {
  const { t } = useTranslation()
  return (cause: unknown) => t(cause instanceof ApiError && cause.status === 403 ? 'm10ErrForbidden' : 'm10ErrGeneric')
}

function Shell({ title, description, actions, children }: { title: string; description: string; actions?: ReactNode; children: ReactNode }) {
  const { t } = useTranslation()
  return <section className="space-y-6">
    <PageHeader eyebrow={t('m10Eyebrow')} title={title} description={description} actions={actions} />
    {children}
  </section>
}

function Tile({ label, value, hint, to, alert }: { label: string; value: ReactNode; hint?: ReactNode; to?: string; alert?: boolean }) {
  const body = <><p className="text-xs font-semibold tracking-wide text-slate-500">{label}</p>
    <p className={`mt-1 text-2xl font-bold tabular-nums ${alert ? 'text-rose-700' : 'text-slate-900'}`}>{value}</p>
    {hint && <p className="mt-0.5 text-sm text-slate-600">{hint}</p>}</>
  return to
    ? <Link to={to} className={`${ui.card} block p-4 transition hover:ring-2 hover:ring-[color-mix(in_srgb,var(--brand-primary)_30%,transparent)] focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]`}>{body}</Link>
    : <div className={`${ui.card} p-4`}>{body}</div>
}

function OutcomeBadge({ outcome }: { outcome: string }) {
  const { t } = useTranslation()
  const denied = outcome === 'denied'
  return <span className={`inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${denied ? 'bg-rose-50 text-rose-800 ring-rose-600/20' : 'bg-emerald-50 text-emerald-800 ring-emerald-600/20'}`}>
    <span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${denied ? 'bg-rose-500' : 'bg-emerald-500'}`} />{t(denied ? 'm10Outcome_denied' : 'm10Outcome_success')}</span>
}

export function AdminOverviewPage() {
  const { t, i18n } = useTranslation()
  const api = useApi()
  const f = useDhakaFormat()
  const errorText = useAdminError()
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  const overview = useQuery({ queryKey: ['admin', 'overview'], queryFn: () => api(getAdminOverview), retry: false, refetchInterval: 60_000 })
  const money = (n: number) => `৳ ${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(n)}`
  const o = overview.data
  return <Shell title={t('m10ControlCenter')} description={t('m10ControlIntro')} actions={<button type="button" className={ui.secondary} onClick={() => void overview.refetch()}>{t('m10Refresh')}</button>}>
    {overview.isPending && <Skeleton rows={4} />}
    {overview.isError && <Notice tone="error" action={<button type="button" className={ui.ghost} onClick={() => void overview.refetch()}>{t('m10Retry')}</button>}>{errorText(overview.error)}</Notice>}
    {o && <>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Tile label={t('m10ActiveStudents')} value={f.number(o.active_students)} hint={`${t('m10Guardians')}: ${f.number(o.guardians)}`} to="/admin/students" />
        <Tile label={t('m10ActiveStaff')} value={f.number(o.active_staff)} to="/admin/staff" />
        <Tile label={t('m10FeesOutstanding')} value={money(o.fees_outstanding)} hint={`${t('m10FeesOverdue')}: ${money(o.fees_overdue)}`} alert={o.fees_overdue > 0} to="/accounts/fees" />
        <Tile label={t('m10Exams')} value={f.number(o.exams_published + o.exams_draft)} hint={t('m10ExamsState', { published: f.number(o.exams_published), draft: f.number(o.exams_draft) })} to="/admin/exams" />
        <Tile label={t('m10Modules')} value={`${f.number(o.modules_enabled)}/${f.number(o.modules_total)}`} hint={t('m10ModulesOn', { enabled: f.number(o.modules_enabled), total: f.number(o.modules_total) })} to="/admin/modules" />
        <Tile label={t('m10Sms')} value={f.number(o.sms_queued)} hint={t('m10SmsState', { queued: f.number(o.sms_queued), failed: f.number(o.sms_failed) })} alert={o.sms_failed > 0} to="/admin/notifications" />
        <Tile label={t('m10Storage')} value={formatBytes(o.attachment_bytes, locale)} hint={t('m10Attachments', { count: o.attachments, n: f.number(o.attachments) })} />
        <Tile label={t('m10Database')} value={formatBytes(o.database_bytes, locale)} hint={t('m10Schema', { version: o.schema_version })} />
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        <div className={`${ui.card} p-5`}>
          <CardTitle icon="users">{t('m10UsersByRole')}</CardTitle>
          <ul className="mt-3 divide-y divide-slate-100">{o.users.map(r => <li key={r.role} className="flex items-center justify-between gap-3 py-2 text-sm">
            <span className="font-medium text-slate-800">{t(`m10Role_${r.role}`, { defaultValue: r.role })}</span>
            <span className="tabular-nums text-slate-600">{t('m10ActiveOf', { active: f.number(r.active), total: f.number(r.total) })}</span></li>)}</ul>
          <p className="mt-2 text-sm text-slate-600">{t('m10LockedUsers')}: <span className="font-semibold tabular-nums">{f.number(o.locked_users)}</span></p>
          <Link to="/admin/users" className={`${ui.ghost} mt-2`}>{t('m10NavUsers')} →</Link>
        </div>
        <div className={`${ui.card} p-5`}>
          <CardTitle icon="shield">{t('m10Security')}</CardTitle>
          <dl className="mt-3 grid grid-cols-2 gap-3">
            <div><dt className="text-xs font-semibold text-slate-500">{t('m10AuditEvents')}</dt><dd className="text-2xl font-bold tabular-nums">{f.number(o.audit_last_day)}</dd></div>
            <div><dt className="text-xs font-semibold text-slate-500">{t('m10DeniedAttempts')}</dt><dd className={`text-2xl font-bold tabular-nums ${o.denied_last_day > 0 ? 'text-rose-700' : ''}`}>{f.number(o.denied_last_day)}</dd></div>
          </dl>
          <h3 className="mt-5 text-sm font-bold text-slate-900">{t('m10LastBackup')}</h3>
          {o.last_backup
            ? <p className="mt-1 text-sm text-slate-700">{f.dateTime(o.last_backup.finished_at ?? o.last_backup.started_at)} · {o.last_backup.size_bytes != null ? formatBytes(o.last_backup.size_bytes, locale) : ''}</p>
            : <p className="mt-1 text-sm text-amber-800">{t('m10NoBackupYet')}</p>}
          <Link to="/admin/backups" className={`${ui.ghost} mt-2`}>{t('m10NavBackups')} →</Link>
        </div>
        <div className={`${ui.card} p-5`}>
          <div className="flex items-center justify-between gap-2"><CardTitle icon="report">{t('m10RecentActivity')}</CardTitle><Link to="/admin/audit" className={ui.ghost}>{t('m10ViewAll')}</Link></div>
          {o.recent_events.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('m10NoEvents')}</p>
            : <ul className="mt-3 space-y-2">{o.recent_events.map(e => <li key={e.id} className="text-sm">
              <div className="flex items-center justify-between gap-2"><code className="truncate font-mono text-xs text-slate-800">{e.action}</code><OutcomeBadge outcome={e.outcome} /></div>
              <p className="text-xs text-slate-500">{e.actor_name || t('m10System')} · {f.dateTime(e.created_at)}</p></li>)}</ul>}
        </div>
      </div>
    </>}
  </Shell>
}

type AuditFilters = { search: string; action: string; outcome: string; from: string; to: string; sort: 'newest' | 'oldest'; page: number }
const emptyFilters: AuditFilters = { search: '', action: '', outcome: '', from: '', to: '', sort: 'newest', page: 1 }

export function AuditLogPage() {
  const { t } = useTranslation()
  const api = useApi()
  const f = useDhakaFormat()
  const errorText = useAdminError()
  const [filters, setFilters] = useState<AuditFilters>(emptyFilters)
  const [search, setSearch] = useState('')
  const invalid = !!filters.from && !!filters.to && filters.from > filters.to
  const params = new URLSearchParams({ page: String(filters.page), page_size: '25', sort: filters.sort })
  for (const key of ['search', 'action', 'outcome', 'from', 'to'] as const) if (filters[key]) params.set(key, filters[key])
  const events = useQuery({ queryKey: ['admin', 'audit', params.toString()], queryFn: () => api(token => getAuditEvents(token, params)), enabled: !invalid, retry: false, placeholderData: previous => previous })
  const actions = useQuery({ queryKey: ['admin', 'audit-actions'], queryFn: () => api(getAuditActions), retry: false, staleTime: 60_000 })
  const update = (patch: Partial<AuditFilters>) => setFilters(current => ({ ...current, ...patch, page: patch.page ?? 1 }))
  function submit(event: FormEvent) { event.preventDefault(); update({ search: search.trim() }) }
  const exportLink = filters.from && filters.to && !invalid ? `/admin/exports?from=${filters.from}&to=${filters.to}` : '/admin/exports'
  return <Shell title={t('m10AuditTitle')} description={t('m10AuditIntro')} actions={<Link to={exportLink} className={ui.secondary}>{t('m10ExportRange')}</Link>}>
    <form onSubmit={submit} className={`${ui.card} grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-6`} role="search">
      <label className="sm:col-span-2"><span className={ui.label}>{t('m10Search')}</span>
        <input className={ui.field} type="search" value={search} maxLength={100} placeholder={t('m10SearchHint')} onChange={e => setSearch(e.target.value)} onBlur={() => search.trim() !== filters.search && update({ search: search.trim() })} /></label>
      <label><span className={ui.label}>{t('m10Action')}</span>
        <select className={ui.field} value={filters.action} onChange={e => update({ action: e.target.value })}>
          <option value="">{t('m10AllActions')}</option>{actions.data?.map(a => <option key={a.action} value={a.action}>{a.action} ({f.number(a.count)})</option>)}</select></label>
      <label><span className={ui.label}>{t('m10Outcome')}</span>
        <select className={ui.field} value={filters.outcome} onChange={e => update({ outcome: e.target.value })}>
          <option value="">{t('m10AllOutcomes')}</option><option value="success">{t('m10Outcome_success')}</option><option value="denied">{t('m10Outcome_denied')}</option></select></label>
      <label><span className={ui.label}>{t('m10From')}</span><input className={ui.field} type="date" value={filters.from} max={todayDhaka()} onChange={e => update({ from: e.target.value })} /></label>
      <label><span className={ui.label}>{t('m10To')}</span><input className={ui.field} type="date" value={filters.to} max={todayDhaka()} onChange={e => update({ to: e.target.value })} /></label>
      <div className="flex flex-wrap items-end gap-2 sm:col-span-2 lg:col-span-6">
        <button type="submit" className={ui.primary}>{t('m10Search')}</button>
        <div className="w-44"><select aria-label={t('m10Sort')} className={`${ui.field} mt-0`} value={filters.sort} onChange={e => update({ sort: e.target.value as AuditFilters['sort'] })}>
          <option value="newest">{t('m10Newest')}</option><option value="oldest">{t('m10Oldest')}</option></select></div>
        <button type="button" className={ui.ghost} onClick={() => { setSearch(''); setFilters(emptyFilters) }}>{t('m10ClearFilters')}</button>
        {events.data && <span className="ml-auto text-sm text-slate-600" aria-live="polite">{t('m10EventsFound', { count: events.data.total, n: f.number(events.data.total) })}</span>}
      </div>
    </form>
    {invalid && <Notice tone="warning">{t('m10InvalidFilters')}</Notice>}
    {events.isError && <Notice tone="error" action={<button type="button" className={ui.ghost} onClick={() => void events.refetch()}>{t('m10Retry')}</button>}>{errorText(events.error)}</Notice>}
    {events.isPending && !invalid && <Skeleton rows={6} />}
    {events.data && !invalid && (events.data.items.length === 0 ? <EmptyState title={t('m10NoEvents')} /> : <>
      <ul className={`${ui.card} divide-y divide-slate-100`} aria-label={t('m10AuditTitle')}>{events.data.items.map(e => <AuditRow key={e.id} event={e} />)}</ul>
      <Pager page={filters.page} total={events.data.total} size={25} onPage={page => update({ page })} previous={t('m10Previous')} next={t('m10Next')} label={t('m10AuditTitle')} />
    </>)}
  </Shell>
}

function AuditRow({ event }: { event: AuditItem }) {
  const { t } = useTranslation()
  const f = useDhakaFormat()
  const metadata = event.metadata && typeof event.metadata === 'object' && Object.keys(event.metadata as object).length > 0 ? JSON.stringify(event.metadata, null, 2) : ''
  return <li className="grid gap-1 px-4 py-3 sm:grid-cols-[11rem_minmax(0,1fr)_auto] sm:items-start sm:gap-4">
    <time dateTime={event.created_at} className="text-xs tabular-nums text-slate-500 sm:pt-0.5">{f.dateTime(event.created_at)}</time>
    <div className="min-w-0">
      <p className="flex flex-wrap items-center gap-x-2 gap-y-1"><code className="break-all font-mono text-sm font-semibold text-slate-900">{event.action}</code>
        <span className="text-sm text-slate-600">{event.target_type}{event.target_id ? `: ${event.target_id}` : ''}</span></p>
      <p className="mt-0.5 text-sm text-slate-700">{event.actor_name || t('m10System')}{event.actor_role && <span className="text-slate-500"> · {t(`m10Role_${event.actor_role}`, { defaultValue: event.actor_role })}</span>}{event.actor_email && event.actor_email !== event.actor_name && <span className="break-all text-slate-500"> · {event.actor_email}</span>}</p>
      {metadata && <details className="mt-1 text-sm"><summary className="cursor-pointer font-semibold text-[var(--brand-ink)]">{t('m10Details')}</summary>
        <pre className="mt-1 max-h-64 overflow-auto rounded-lg bg-slate-50 p-3 text-xs text-slate-800">{metadata}</pre></details>}
    </div>
    <div className="sm:justify-self-end"><OutcomeBadge outcome={event.outcome} /></div>
  </li>
}

const datasetOrder = ['users', 'students', 'guardians', 'staff', 'student_attendance', 'exam_results', 'fee_payments', 'ledger', 'audit_log']

export function DataExportPage() {
  const { t } = useTranslation()
  const api = useApi()
  const f = useDhakaFormat()
  const errorText = useAdminError()
  const query = new URLSearchParams(useLocation().search)
  const [from, setFrom] = useState(query.get('from') || shiftDay(todayDhaka(), -30))
  const [to, setTo] = useState(query.get('to') || todayDhaka())
  const [busy, setBusy] = useState('')
  const [result, setResult] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const maxDays = 400
  const days = (Date.parse(to) - Date.parse(from)) / 86400e3
  const rangeOk = !!from && !!to && days >= 0 && days <= maxDays
  const datasets = useQuery({ queryKey: ['admin', 'exports', rangeOk ? from : '', rangeOk ? to : ''], queryFn: () => api(token => getExportDatasets(token, rangeOk ? from : '', rangeOk ? to : '')), retry: false, placeholderData: previous => previous })
  async function download(key: string, ranged: boolean) {
    setBusy(key); setResult(null)
    const name = ranged ? `flybirds-${key}-${from}-to-${to}.csv` : `flybirds-${key}-${todayDhaka().replaceAll('-', '')}.csv`
    try {
      const blob = await api(token => downloadExport(token, key, ranged ? from : '', ranged ? to : ''))
      saveBlob(blob, name)
      setResult({ tone: 'success', text: t('m10Downloaded', { name }) })
    } catch (cause) { setResult({ tone: 'error', text: errorText(cause) }) } finally { setBusy('') }
  }
  const items = [...(datasets.data?.items ?? [])].sort((a, b) => datasetOrder.indexOf(a.key) - datasetOrder.indexOf(b.key))
  return <Shell title={t('m10ExportTitle')} description={t('m10ExportIntro')}>
    <Notice tone="warning">{t('m10ExportWarning')}</Notice>
    <fieldset className={`${ui.card} flex flex-wrap items-end gap-3 p-4`}>
      <legend className="sr-only">{t('m10DateRange')}</legend>
      <div className="w-full"><p className="font-semibold text-slate-900">{t('m10DateRange')}</p><p className="text-sm text-slate-600">{t('m10RangeHint', { days: maxDays })}</p></div>
      <label><span className={ui.label}>{t('m10From')}</span><input className={ui.field} type="date" value={from} max={to || undefined} onChange={e => setFrom(e.target.value)} /></label>
      <label><span className={ui.label}>{t('m10To')}</span><input className={ui.field} type="date" value={to} min={from || undefined} max={todayDhaka()} onChange={e => setTo(e.target.value)} /></label>
      {!rangeOk && <p role="alert" className="w-full text-sm font-medium text-rose-700">{t('m10RangeInvalid', { days: maxDays })}</p>}
    </fieldset>
    {result && <Notice tone={result.tone}>{result.text}</Notice>}
    {datasets.isError && <Notice tone="error" action={<button type="button" className={ui.ghost} onClick={() => void datasets.refetch()}>{t('m10Retry')}</button>}>{errorText(datasets.error)}</Notice>}
    {datasets.isPending && <Skeleton rows={4} />}
    <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{items.map(d => <li key={d.key} className={`${ui.card} flex flex-col p-4`}>
      <div className="flex items-start justify-between gap-2"><h2 className="font-bold text-slate-900">{t(`m10Dataset_${d.key}`)}</h2>
        {d.ranged && <span className="shrink-0 rounded-full bg-sky-50 px-2 py-0.5 text-xs font-semibold text-sky-800 ring-1 ring-inset ring-sky-600/20">{t('m10Dated')}</span>}</div>
      <p className="mt-1 flex-1 text-sm text-slate-600">{t(`m10Dataset_${d.key}_desc`)}</p>
      <p className="mt-2 text-sm font-medium tabular-nums text-slate-800">{d.rows >= 0 ? t(d.ranged ? 'm10RowsInRange' : 'm10Rows', { count: d.rows, n: f.number(d.rows) }) : '—'}</p>
      <button type="button" className={`${ui.secondary} mt-3`} disabled={!!busy || (d.ranged && !rangeOk)} onClick={() => void download(d.key, d.ranged)}>{busy === d.key ? t('m10Downloading') : t('m10Download')}</button>
    </li>)}</ul>
    <p className="text-sm text-slate-600">{t('m10NoPasswords')}</p>
  </Shell>
}

export function BackupsPage() {
  const { t, i18n } = useTranslation()
  const api = useApi()
  const f = useDhakaFormat()
  const errorText = useAdminError()
  const client = useQueryClient()
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  const [page, setPage] = useState(1)
  const [message, setMessage] = useState<{ tone: 'success' | 'error' | 'warning'; text: string } | null>(null)
  const [confirmDelete, setConfirmDelete] = useState('')
  const [busy, setBusy] = useState('')
  const runs = useQuery({ queryKey: ['admin', 'backups', page], queryFn: () => api(token => getBackups(token, page)), retry: false })
  const refresh = () => Promise.all([client.invalidateQueries({ queryKey: ['admin', 'backups'] }), client.invalidateQueries({ queryKey: ['admin', 'overview'] })])
  const create = useMutation({
    mutationFn: () => api(createBackup),
    onMutate: () => setMessage(null),
    onSuccess: async run => { setMessage({ tone: 'success', text: t('m10BackupDone', { name: run.file_name }) }); setPage(1); await refresh() },
    onError: async cause => { setMessage({ tone: cause instanceof ApiError && cause.code === 'backup_running' ? 'warning' : 'error', text: cause instanceof ApiError && cause.code === 'backup_running' ? t('m10BackupBusy') : errorText(cause) }); await refresh() },
  })
  async function act(run: BackupRun, kind: 'verify' | 'download' | 'delete') {
    setBusy(run.id + kind); setMessage(null)
    try {
      if (kind === 'verify') {
        const v = await api(token => verifyBackup(token, run.id))
        setMessage(v.ok ? { tone: 'success', text: t('m10Verified', { entries: f.number(v.entries) }) } : { tone: 'error', text: t('m10VerifyFailed') })
      } else if (kind === 'download') {
        saveBlob(await api(token => downloadBackup(token, run.id)), run.file_name ?? 'backup.tar.gz')
      } else {
        await api(token => deleteBackup(token, run.id))
        setConfirmDelete(''); setMessage({ tone: 'success', text: t('m10BackupDeleted') }); await refresh()
      }
    } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) } finally { setBusy('') }
  }
  const statusTone = { running: 'bg-sky-50 text-sky-800 ring-sky-600/20', completed: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20', failed: 'bg-rose-50 text-rose-800 ring-rose-600/20' }
  return <Shell title={t('m10BackupsTitle')} description={t('m10BackupsIntro')}
    actions={<button type="button" className={ui.primary} disabled={create.isPending} onClick={() => create.mutate()}>{create.isPending ? t('m10BackingUp') : t('m10BackupNow')}</button>}>
    <div className="grid gap-3 lg:grid-cols-3">
      {runs.data && <Notice tone="info">{t('m10Retention', { count: runs.data.retention })}</Notice>}
      <Notice tone="warning">{t('m10BackupSecurity')}</Notice>
      <Notice tone="info">{t('m10RestoreHelp')}</Notice>
    </div>
    {message && <Notice tone={message.tone}>{message.text}</Notice>}
    {runs.isError && <Notice tone="error" action={<button type="button" className={ui.ghost} onClick={() => void runs.refetch()}>{t('m10Retry')}</button>}>{errorText(runs.error)}</Notice>}
    {runs.isPending && <Skeleton rows={3} />}
    {runs.data && (runs.data.items.length === 0 ? <EmptyState title={t('m10NoBackups')} /> : <>
      <ul className={`${ui.card} divide-y divide-slate-100`} aria-label={t('m10BackupsTitle')}>{runs.data.items.map(run => {
        const available = run.status === 'completed' && !run.deleted
        return <li key={run.id} className="flex flex-col gap-3 px-4 py-3 lg:flex-row lg:items-center">
          <div className="min-w-0 flex-1">
            <p className="flex flex-wrap items-center gap-2">
              <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${run.deleted ? 'bg-slate-100 text-slate-600 ring-slate-500/20' : statusTone[run.status]}`}>{run.deleted ? t('m10Deleted') : t(`m10Status_${run.status}`)}</span>
              <span className="font-semibold tabular-nums text-slate-900">{f.dateTime(run.started_at)}</span>
              <span className="text-sm text-slate-600">{t(`m10Trigger_${run.trigger}`)}{run.started_by_name && ` · ${t('m10By', { name: run.started_by_name })}`}</span></p>
            {run.file_name && <p className="mt-1 break-all font-mono text-xs text-slate-500">{run.file_name}</p>}
            {run.status === 'completed' && <p className="mt-0.5 text-sm text-slate-700">{run.size_bytes != null && formatBytes(run.size_bytes, locale)} · {t('m10ContentsValue', { tables: f.number(run.table_count ?? 0), rows: f.number(run.row_count ?? 0), files: f.number(run.file_count ?? 0) })}</p>}
            {run.status === 'failed' && run.error && <p className="mt-0.5 text-sm text-rose-700">{run.error}</p>}
          </div>
          {available && (confirmDelete === run.id
            ? <div role="alertdialog" aria-label={t('m10ConfirmDeleteBackup')} className="flex flex-wrap items-center gap-2 rounded-xl bg-rose-50 p-2 text-sm text-rose-900">
              <span className="px-1">{t('m10ConfirmDeleteBackup')}</span>
              <button type="button" className={`${ui.primary} bg-rose-700`} disabled={!!busy} onClick={() => void act(run, 'delete')}>{t('m10DeleteBackup')}</button>
              <button type="button" className={ui.secondary} onClick={() => setConfirmDelete('')}>{t('m10Cancel')}</button></div>
            : <div className="flex flex-wrap gap-2">
              <button type="button" className={ui.secondary} disabled={!!busy} onClick={() => void act(run, 'verify')}>{busy === run.id + 'verify' ? t('m10Verifying') : t('m10Verify')}</button>
              <button type="button" className={ui.secondary} disabled={!!busy} onClick={() => void act(run, 'download')}>{busy === run.id + 'download' ? t('m10Downloading') : t('m10DownloadFile')}</button>
              <button type="button" className={`${ui.secondary} text-rose-700`} disabled={!!busy} onClick={() => setConfirmDelete(run.id)}>{t('m10DeleteBackup')}</button></div>)}
        </li>
      })}</ul>
      <Pager page={page} total={runs.data.total} size={10} onPage={setPage} previous={t('m10Previous')} next={t('m10Next')} label={t('m10BackupsTitle')} />
    </>)}
  </Shell>
}

// closure lists the modules a change would also switch: prerequisites when
// enabling, enabled dependents when disabling. The server applies the same plan.
function closure(all: ManagedModule[], key: string, enabled: boolean) {
  const byKey = new Map(all.map(m => [m.key, m]))
  const out: string[] = []
  const visit = (k: string) => {
    for (const next of (enabled ? byKey.get(k)?.depends_on : byKey.get(k)?.dependents) ?? []) {
      const m = byKey.get(next)
      if (m && m.enabled !== enabled && !out.includes(next)) { out.push(next); visit(next) }
    }
  }
  visit(key)
  return out
}

export function ModulesPage({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const api = useApi()
  const errorText = useAdminError()
  const client = useQueryClient()
  const [pending, setPending] = useState<{ key: string; enabled: boolean; others: string[] } | null>(null)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const modules = useQuery({ queryKey: ['school', 'modules', 'managed'], queryFn: () => api(getManagedModules), retry: false })
  const name = (key: string) => t(`m10Module_${key}`, { defaultValue: modules.data?.items.find(m => m.key === key)?.name ?? key })
  const names = (keys: string[]) => keys.map(name).join(', ')
  const save = useMutation({
    mutationFn: (change: { key: string; enabled: boolean; cascade: boolean }) => api(token => setManagedModule(token, change.key, change.enabled, change.cascade)),
    onMutate: () => setMessage(null),
    onSuccess: async (result, change) => {
      setPending(null)
      const changed = result.changed ?? [change.key]
      setMessage({ tone: 'success', text: changed.length > 1 ? t('m10ModulesSaved', { names: names(changed) }) : t('m10ModuleSaved', { name: name(change.key), state: t(change.enabled ? 'm10On' : 'm10Off') }) })
      await client.invalidateQueries({ queryKey: ['school', 'modules'] })
    },
    onError: async (cause, change) => {
      const details = cause instanceof ApiError ? cause.details : undefined
      if (cause instanceof ApiError && cause.code === 'module_dependencies' && Array.isArray(details?.modules)) setPending({ key: change.key, enabled: change.enabled, others: details.modules as string[] })
      else setMessage({ tone: 'error', text: cause instanceof ApiError && cause.status !== 403 ? t('m10ModuleError') : errorText(cause) })
      await client.invalidateQueries({ queryKey: ['school', 'modules'] })
    },
  })
  function toggle(m: ManagedModule) {
    setMessage(null)
    const others = closure(modules.data?.items ?? [], m.key, !m.enabled)
    if (m.enabled || others.length > 0) setPending({ key: m.key, enabled: !m.enabled, others })
    else save.mutate({ key: m.key, enabled: true, cascade: false })
  }
  return <Shell title={t('m10ModulesTitle')} description={t('m10ModulesIntro')}>
    {!canManage && <Notice tone="info">{t('m10ReadOnly')}</Notice>}
    {message && <Notice tone={message.tone}>{message.text}</Notice>}
    {pending && <div role="alertdialog" aria-labelledby="module-confirm-title" className="rounded-2xl border border-amber-300 bg-amber-50 p-4 text-amber-950">
      <h2 id="module-confirm-title" className="font-bold">{pending.others.length > 0 ? t(pending.enabled ? 'm10NeedsTitle' : 'm10UsedByTitle', { name: name(pending.key) }) : `${t('m10TurnOff')}: ${name(pending.key)}`}</h2>
      <p className="mt-1 text-sm">{pending.others.length > 0 ? t(pending.enabled ? 'm10NeedsBody' : 'm10UsedByBody', { name: name(pending.key), modules: names(pending.others) }) : t('m10ConfirmOff', { name: name(pending.key) })}</p>
      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" className={ui.primary} disabled={save.isPending} onClick={() => save.mutate({ key: pending.key, enabled: pending.enabled, cascade: pending.others.length > 0 })}>{pending.others.length > 0 ? t('m10ApplyAll') : t('m10Confirm')}</button>
        <button type="button" className={ui.secondary} onClick={() => setPending(null)}>{t('m10Cancel')}</button></div>
    </div>}
    {modules.isError && <Notice tone="error" action={<button type="button" className={ui.ghost} onClick={() => void modules.refetch()}>{t('m10Retry')}</button>}>{errorText(modules.error)}</Notice>}
    {modules.isPending && <Skeleton rows={5} />}
    {modules.data && <ul className="grid gap-3 md:grid-cols-2">{modules.data.items.map(m => <li key={m.key} className={`${ui.card} flex items-start justify-between gap-4 p-4`}>
      <div className="min-w-0">
        <p className="flex flex-wrap items-center gap-2 font-semibold text-slate-900">{name(m.key)}
          {m.is_core && <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700">{t('m10Core')}</span>}</p>
        {m.depends_on.length > 0 && <p className="mt-1 text-sm text-slate-600">{t('m10Requires')}: {m.depends_on.map(k => <span key={k} className={`mr-1 inline-block rounded px-1.5 text-xs font-medium ${modules.data.items.find(x => x.key === k)?.enabled ? 'bg-emerald-50 text-emerald-800' : 'bg-rose-50 text-rose-800'}`}>{name(k)}</span>)}</p>}
        {m.dependents.length > 0 && <p className="mt-1 text-sm text-slate-600">{t('m10RequiredBy')}: {names(m.dependents)}</p>}
      </div>
      <button type="button" role="switch" aria-checked={m.enabled} aria-label={name(m.key)} disabled={m.is_core || !canManage || save.isPending} onClick={() => toggle(m)}
        className={`relative mt-0.5 inline-flex h-7 w-12 shrink-0 items-center rounded-full transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-not-allowed disabled:opacity-60 ${m.enabled ? 'bg-[var(--brand-primary)]' : 'bg-slate-300'}`}>
        <span className="sr-only">{t(m.enabled ? 'm10On' : 'm10Off')}</span>
        <span aria-hidden="true" className={`inline-block h-5 w-5 rounded-full bg-white shadow transition ${m.enabled ? 'translate-x-6' : 'translate-x-1'}`} />
      </button>
    </li>)}</ul>}
  </Shell>
}
