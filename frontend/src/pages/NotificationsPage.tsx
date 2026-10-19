import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { getInbox, getPreferences, getSMSLog, markAllNotificationsRead, markNotificationRead, runDispatch, savePreferences, type AppNotification, type DispatchResult, type Preference } from '../api/communication'
import { EmptyState, Notice as Banner, PageHeader, Pager, Skeleton, Tabs, ui } from './attendanceUi'
import { useApi, useDhakaFormat, useErrorText, useLocal } from './commShared'

const categoryIcon: Record<AppNotification['category'], string> = { notice: '📢', absence: '🕒', result: '🎓', fee: '৳', diary: '📘' }

// NotificationBell polls the unread count; it renders nothing until the
// notices module answers, so disabled modules stay quiet.
export function NotificationBell() {
  const { t } = useTranslation()
  const authorized = useApi()
  const unread = useQuery({ queryKey: ['m9', 'unread'], refetchInterval: 60_000, retry: false, queryFn: () => authorized(token => getInbox(token, new URLSearchParams({ page: '1', page_size: '1', unread: '1' }))) })
  if (unread.isError || !unread.data) return null
  const count = unread.data.unread
  return <Link to="/notifications" className="relative inline-flex h-9 w-9 items-center justify-center rounded-full text-slate-700 hover:bg-slate-100" aria-label={t('m9NotificationsCount', { count })}>
    <svg aria-hidden="true" viewBox="0 0 20 20" className="h-5 w-5 fill-current"><path d="M10 2a5 5 0 0 0-5 5v3.6L3.3 13.3A1 1 0 0 0 4.1 15h11.8a1 1 0 0 0 .8-1.7L15 10.6V7a5 5 0 0 0-5-5zm-2 14a2 2 0 0 0 4 0z" /></svg>
    {count > 0 && <span className="absolute -right-0.5 -top-0.5 min-w-5 rounded-full bg-rose-600 px-1 text-center text-[0.65rem] font-bold leading-5 text-white">{count > 99 ? '99+' : count}</span>}
  </Link>
}

export function NotificationsPage() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'inbox' | 'preferences'>('inbox')
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9NotificationsEyebrow')} title={t('m9Notifications')} description={t('m9NotificationsIntro')} />
    <Tabs label={t('m9Notifications')} value={tab} onChange={setTab} items={[{ value: 'inbox', label: t('m9Inbox') }, { value: 'preferences', label: t('m9Preferences') }]} />
    {tab === 'inbox' ? <InboxPanel /> : <PreferencesPanel />}
  </div>
}

function InboxPanel() {
  const { t } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const navigate = useNavigate()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [page, setPage] = useState(1)
  const params = new URLSearchParams({ page: String(page), page_size: '15', ...(unreadOnly ? { unread: '1' } : {}) })
  const inbox = useQuery({ queryKey: ['m9', 'inbox', params.toString()], queryFn: () => authorized(token => getInbox(token, params)), retry: false })
  async function open(n: AppNotification) {
    if (!n.read) { await authorized(token => markNotificationRead(token, n.id)); await client.invalidateQueries({ queryKey: ['m9'] }) }
    if (n.link) navigate(n.link)
  }
  return <section className={`${ui.card} p-2 sm:p-3`} aria-label={t('m9Inbox')}>
    <div className="flex flex-wrap items-center justify-between gap-2 p-2">
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={unreadOnly} onChange={e => { setUnreadOnly(e.target.checked); setPage(1) }} />{t('m9UnreadOnly')}</label>
      <button type="button" className={ui.secondary} disabled={!inbox.data?.unread} onClick={async () => { await authorized(token => markAllNotificationsRead(token)); await client.invalidateQueries({ queryKey: ['m9'] }) }}>{t('m9MarkAllRead')}</button>
    </div>
    {inbox.isPending ? <Skeleton rows={4} /> : inbox.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : inbox.data.items.length === 0 ? <EmptyState title={t('m9NoNotifications')} /> : <>
      <ul className="divide-y divide-slate-100">{inbox.data.items.map(n => <li key={n.id}>
        <button type="button" onClick={() => void open(n)} className={`flex w-full items-start gap-3 rounded-xl p-3 text-left hover:bg-slate-50 ${n.read ? '' : 'bg-[var(--brand-soft)]'}`}>
          <span aria-hidden="true" className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-slate-100 text-base">{categoryIcon[n.category]}</span>
          <span className="min-w-0 flex-1"><span className="flex flex-wrap items-center gap-2"><span className={`font-semibold ${n.read ? 'text-slate-700' : 'text-slate-900'}`}>{local(n.title_en, n.title_bn)}</span>
            {n.urgent && <span className="rounded-full bg-rose-600 px-2 text-xs font-bold text-white">{t('m9Priority_urgent')}</span>}{!n.read && <span className="sr-only">{t('m9Unread')}</span>}</span>
            <span className="mt-0.5 block text-sm text-slate-600">{local(n.body_en, n.body_bn)}</span>
            <span className="mt-1 block text-xs text-slate-500">{t(`m9Category_${n.category}`)} · {fmt.dateTime(n.created_at)}</span></span>
          {!n.read && <span aria-hidden="true" className="mt-2 h-2.5 w-2.5 shrink-0 rounded-full bg-[var(--brand-primary)]" />}
        </button></li>)}</ul>
      <Pager page={page} total={inbox.data.total} size={15} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9Inbox')} />
    </>}
  </section>
}

function PreferencesPanel() {
  const { t } = useTranslation()
  const authorized = useApi()
  const errorText = useErrorText()
  const prefs = useQuery({ queryKey: ['m9', 'preferences'], queryFn: () => authorized(token => getPreferences(token)), retry: false })
  const [draft, setDraft] = useState<Preference[] | null>(null)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const items = draft ?? prefs.data ?? []
  const set = (category: string, key: 'in_app' | 'sms', value: boolean) => setDraft(items.map(p => p.category === category ? { ...p, [key]: value } : p))
  async function save() {
    try { setDraft(await authorized(token => savePreferences(token, items))); setMessage({ tone: 'success', text: t('m9Saved') }) } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) }
  }
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="prefs-title">
    <h2 id="prefs-title" className="text-lg font-bold">{t('m9Preferences')}</h2><p className="text-sm text-slate-600">{t('m9PreferencesHint')}</p>
    {message && <div className="mt-3"><Banner tone={message.tone}>{message.text}</Banner></div>}
    {prefs.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : <div className="mt-3 overflow-x-auto"><table className="w-full min-w-[22rem] text-left text-sm">
      <thead><tr className="border-b text-xs uppercase tracking-wide text-slate-500"><th className="py-2">{t('m9Category')}</th><th className="py-2 text-center">{t('m9InApp')}</th><th className="py-2 text-center">{t('m9SMS')}</th></tr></thead>
      <tbody>{items.map(p => <tr key={p.category} className="border-b border-slate-100"><td className="py-3 font-medium">{t(`m9Category_${p.category}`)}</td>
        <td className="text-center"><input type="checkbox" className="h-4 w-4" aria-label={`${t(`m9Category_${p.category}`)} ${t('m9InApp')}`} checked={p.in_app} onChange={e => set(p.category, 'in_app', e.target.checked)} /></td>
        <td className="text-center"><input type="checkbox" className="h-4 w-4" aria-label={`${t(`m9Category_${p.category}`)} ${t('m9SMS')}`} checked={p.sms} onChange={e => set(p.category, 'sms', e.target.checked)} /></td></tr>)}</tbody></table></div>}
    <p className="mt-2 text-xs text-slate-500">{t('m9SmsMockNote')}</p>
    <button type="button" className={`${ui.primary} mt-3`} disabled={!draft} onClick={() => void save()}>{t('m9Save')}</button>
  </section>
}

const smsTone: Record<string, string> = { sent_mock: 'bg-sky-100 text-sky-900', queued: 'bg-amber-100 text-amber-900', failed: 'bg-rose-100 text-rose-900', cancelled: 'bg-slate-200 text-slate-700' }

export function AdminNotificationsPage() {
  const { t } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const errorText = useErrorText()
  const fmt = useDhakaFormat()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [result, setResult] = useState<DispatchResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const params = new URLSearchParams({ page: String(page), page_size: '20', ...(status ? { status } : {}) })
  const log = useQuery({ queryKey: ['m9', 'sms', params.toString()], queryFn: () => authorized(token => getSMSLog(token, params)), retry: false })
  async function dispatch() {
    setBusy(true); setError('')
    try { setResult(await authorized(token => runDispatch(token))); await client.invalidateQueries({ queryKey: ['m9'] }) } catch (cause) { setError(errorText(cause)) } finally { setBusy(false) }
  }
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9NotificationsEyebrow')} title={t('m9Delivery')} description={t('m9DeliveryIntro')} actions={<button type="button" className={ui.primary} disabled={busy} onClick={() => void dispatch()}>{busy ? t('m9Running') : t('m9RunDispatch')}</button>} />
    <Banner tone="warning">{t('m9MockBanner')}</Banner>
    {error && <Banner tone="error">{error}</Banner>}
    {result && <Banner tone="success">{result.disabled ? t('m9DispatchDisabled') : result.skipped ? t('m9DispatchSkipped') : t('m9DispatchResult', { created: Object.values(result.created).reduce((a, b) => a + b, 0), withdrawn: result.withdrawn, sent: result.sms_sent_mock, failed: result.sms_failed })}</Banner>}
    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="sms-title">
      <div className="flex flex-wrap items-end justify-between gap-3"><div><h2 id="sms-title" className="text-lg font-bold">{t('m9SmsLog')}</h2>
        {log.data && <p className="mt-1 flex flex-wrap gap-2 text-xs">{Object.entries(log.data.counts).map(([k, v]) => <span key={k} className={`rounded-full px-2 py-0.5 font-semibold ${smsTone[k]}`}>{t(`m9Sms_${k}`)}: {fmt.number(v)}</span>)}</p>}</div>
        <label><span className="sr-only">{t('m9Status')}</span><select className={`${ui.field} mt-0`} value={status} onChange={e => { setStatus(e.target.value); setPage(1) }}><option value="">{t('m9AllStatuses')}</option>{['queued', 'sent_mock', 'failed', 'cancelled'].map(s => <option key={s} value={s}>{t(`m9Sms_${s}`)}</option>)}</select></label></div>
      {log.isPending ? <div className="mt-3"><Skeleton /></div> : log.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : log.data.items.length === 0 ? <div className="mt-3"><EmptyState title={t('m9NoSms')} /></div> : <>
        <div className="mt-3 overflow-x-auto"><table className="w-full min-w-[44rem] text-left text-sm"><thead><tr className="border-b text-xs uppercase tracking-wide text-slate-500"><th className="py-2 pr-2">{t('m9Recipient')}</th><th className="px-2">{t('m9Message')}</th><th className="px-2">{t('m9Status')}</th><th className="px-2">{t('m9Reference')}</th><th className="pl-2">{t('m9When')}</th></tr></thead>
          <tbody>{log.data.items.map(m => <tr key={m.id} className="border-b border-slate-100 align-top"><td className="py-2.5 pr-2"><p className="font-medium">{m.recipient_name}</p><p className="font-mono text-xs text-slate-500">{m.phone}</p></td>
            <td className="max-w-xs px-2 py-2.5"><p className="text-xs text-slate-500">{t(`m9Category_${m.category}`)}</p><p className="break-words">{m.body}</p>{m.error && <p className="text-xs text-rose-700">{m.error}</p>}</td>
            <td className="px-2 py-2.5"><span className={`whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold ${smsTone[m.status]}`}>{t(`m9Sms_${m.status}`)}</span></td>
            <td className="px-2 py-2.5 font-mono text-xs">{m.provider_ref || '—'}</td><td className="py-2.5 pl-2 text-xs text-slate-600">{fmt.dateTime(m.processed_at ?? m.created_at)}</td></tr>)}</tbody></table></div>
        <Pager page={page} total={log.data.total} size={20} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9SmsLog')} />
      </>}
    </section>
  </div>
}
