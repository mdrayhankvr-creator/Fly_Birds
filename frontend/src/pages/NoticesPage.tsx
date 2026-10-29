import { confirmAction } from '../components/ConfirmDialog'
import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { archiveNotice, deleteNotice, getManagedNotices, getNoticeFeed, publishNotice, saveNotice, type Notice, type NoticeInput, type Priority } from '../api/communication'
import { CardTitle, EmptyState, Notice as Banner, PageHeader, Pager, Skeleton, Tabs, ui } from './attendanceUi'
import { AttachmentList, Field, FileUploader, PriorityBadge, RolePicker, TargetPicker, isoToLocal, localToIso, useApi, useDhakaFormat, useErrorText, useLocal } from './commShared'

const blank: NoticeInput = { title_en: '', title_bn: '', body_en: '', body_bn: '', priority: 'normal', audience_roles: ['student', 'guardian', 'teacher'], targets: [], expires_at: null, file_ids: [] }

function NoticeCard({ notice, manager }: { notice: Notice; manager?: boolean }) {
  const { t } = useTranslation()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const body = local(notice.body_en, notice.body_bn)
  return <article className={`${ui.card} p-4 sm:p-5 ${notice.priority === 'urgent' ? 'border-rose-300 ring-1 ring-rose-200' : ''}`}>
    <div className="flex flex-wrap items-center gap-2"><PriorityBadge priority={notice.priority} />{manager && <span className="text-xs font-semibold uppercase text-slate-500">{t(`m9Status_${notice.status}`)}{notice.status === 'published' && !notice.live ? ` · ${t(notice.publish_at && new Date(notice.publish_at) > new Date() ? 'm9Scheduled' : 'm9Expired')}` : ''}</span>}</div>
    <h3 className="mt-2 text-lg font-bold text-slate-900">{local(notice.title_en, notice.title_bn)}</h3>
    <p className="mt-1 text-xs text-slate-500">{notice.author_name} · {fmt.dateTime(notice.publish_at ?? notice.updated_at)}{notice.expires_at ? ` · ${t('m9Until')} ${fmt.dateTime(notice.expires_at)}` : ''}</p>
    {body && <p className="mt-3 whitespace-pre-line text-sm leading-relaxed text-slate-700">{body}</p>}
    {notice.targets.length > 0 && <p className="mt-2 text-xs text-slate-500">{t('m9For')}: {notice.targets.map(x => `${x.class_name}${x.section_name ? ` · ${x.section_name}` : ''}`).join(', ')}</p>}
    <AttachmentList items={notice.attachments} pathFor={f => `/notices/${notice.id}/files/${f.id}`} />
  </article>
}

export function NoticesPage({ canManage }: { canManage: boolean }) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'feed' | 'manage'>('feed')
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9NoticesEyebrow')} title={t('m9Notices')} description={t('m9NoticesIntro')} />
    {canManage && <Tabs label={t('m9Notices')} value={tab} onChange={setTab} items={[{ value: 'feed', label: t('m9NoticeBoard') }, { value: 'manage', label: t('m9ManageNotices') }]} />}
    {tab === 'feed' ? <NoticeFeed /> : <NoticeManager />}
  </div>
}

function NoticeFeed() {
  const { t } = useTranslation()
  const authorized = useApi()
  const [search, setSearch] = useState('')
  const [priority, setPriority] = useState('')
  const [page, setPage] = useState(1)
  const params = new URLSearchParams({ page: String(page), page_size: '10', search, ...(priority ? { priority } : {}) })
  const feed = useQuery({ queryKey: ['m9', 'notice-feed', params.toString()], queryFn: () => authorized(token => getNoticeFeed(token, params)), retry: false })
  return <section className="space-y-3" aria-label={t('m9NoticeBoard')}>
    <div className="flex flex-col gap-3 sm:flex-row">
      <label className="flex-1"><span className="sr-only">{t('m9Search')}</span><input type="search" className={`${ui.field} mt-0`} placeholder={t('m9SearchNotices')} value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} /></label>
      <label><span className="sr-only">{t('m9Priority')}</span><select className={`${ui.field} mt-0`} value={priority} onChange={e => { setPriority(e.target.value); setPage(1) }}><option value="">{t('m9AllPriorities')}</option>{(['urgent', 'important', 'normal'] as Priority[]).map(p => <option key={p} value={p}>{t(`m9Priority_${p}`)}</option>)}</select></label>
    </div>
    {feed.isPending ? <Skeleton rows={3} /> : feed.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : feed.data.items.length === 0 ? <EmptyState title={t('m9NoNotices')} /> : <>
      {feed.data.items.map(n => <NoticeCard key={n.id} notice={n} />)}
      <Pager page={page} total={feed.data.total} size={10} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9Notices')} />
    </>}
  </section>
}

function NoticeManager() {
  const { t } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const errorText = useErrorText()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<Notice | 'new' | null>(null)
  const [form, setForm] = useState<NoticeInput>(blank)
  const [files, setFiles] = useState<Notice['attachments']>([])
  const [expires, setExpires] = useState('')
  const [scheduleAt, setScheduleAt] = useState('')
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const params = new URLSearchParams({ page: String(page), page_size: '10', ...(status ? { status } : {}) })
  const list = useQuery({ queryKey: ['m9', 'notices-managed', params.toString()], queryFn: () => authorized(token => getManagedNotices(token, params)), retry: false })

  function open(n: Notice | 'new') {
    setEditing(n); setMessage(null); setScheduleAt('')
    if (n === 'new') { setForm(blank); setFiles([]); setExpires('') }
    else { setForm({ title_en: n.title_en, title_bn: n.title_bn, body_en: n.body_en, body_bn: n.body_bn, priority: n.priority, audience_roles: n.audience_roles, targets: n.targets, expires_at: n.expires_at, file_ids: n.attachments.map(a => a.id) }); setFiles(n.attachments); setExpires(isoToLocal(n.expires_at)) }
  }
  async function run(task: () => Promise<Notice | unknown>, success: string, close = false) {
    setBusy(true); setMessage(null)
    try {
      const result = await task()
      await client.invalidateQueries({ queryKey: ['m9'] })
      if (close) setEditing(null)
      else if (result && typeof result === 'object' && 'id' in result) open(result as Notice)
      setMessage({ tone: 'success', text: t(success) })
    } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) } finally { setBusy(false) }
  }
  const current = editing && editing !== 'new' ? editing : null
  const payload = (): NoticeInput => ({ ...form, expires_at: localToIso(expires), file_ids: files.map(f => f.id) })
  function save(event: FormEvent) {
    event.preventDefault()
    void run(() => authorized(token => saveNotice(token, payload(), current?.id)), 'm9Saved')
  }
  async function saveAndPublish(at: string | null) {
    await run(async () => {
      const saved = await authorized(token => saveNotice(token, payload(), current?.id))
      return authorized(token => publishNotice(token, saved.id, at))
    }, at ? 'm9ScheduledOk' : 'm9PublishedOk')
  }

  return <div className="space-y-4">
    {message && <Banner tone={message.tone}>{message.text}</Banner>}
    {editing ? <form onSubmit={save} className={`${ui.card} space-y-4 p-4 sm:p-6`} aria-labelledby="notice-editor">
      <div className="flex flex-wrap items-center justify-between gap-2"><CardTitle id="notice-editor" icon="megaphone">{current ? t('m9EditNotice') : t('m9NewNotice')}</CardTitle>{current && <span className="text-sm text-slate-500">{t(`m9Status_${current.status}`)}</span>}</div>
      <div className="grid gap-4 md:grid-cols-2">
        <Field label={t('m9TitleEn')}><input required maxLength={200} className={ui.field} value={form.title_en} onChange={e => setForm({ ...form, title_en: e.target.value })} /></Field>
        <Field label={t('m9TitleBn')}><input lang="bn" className={ui.field} value={form.title_bn} onChange={e => setForm({ ...form, title_bn: e.target.value })} /></Field>
        <Field label={t('m9BodyEn')}><textarea rows={5} className={ui.field} value={form.body_en} onChange={e => setForm({ ...form, body_en: e.target.value })} /></Field>
        <Field label={t('m9BodyBn')}><textarea lang="bn" rows={5} className={ui.field} value={form.body_bn} onChange={e => setForm({ ...form, body_bn: e.target.value })} /></Field>
        <fieldset><legend className={ui.label}>{t('m9Priority')}</legend><div className="mt-2 flex flex-wrap gap-2">{(['normal', 'important', 'urgent'] as Priority[]).map(p => <label key={p} className={`flex cursor-pointer items-center gap-2 rounded-full border px-3 py-1.5 text-sm ${form.priority === p ? 'border-[var(--brand-primary)] font-semibold' : 'border-slate-200'}`}><input type="radio" name="priority" checked={form.priority === p} onChange={() => setForm({ ...form, priority: p })} />{t(`m9Priority_${p}`)}</label>)}</div>
          {form.priority === 'urgent' && <p className="mt-2 text-xs text-rose-700">{t('m9UrgentHint')}</p>}</fieldset>
        <Field label={t('m9ExpiresAt')}><input type="datetime-local" className={ui.field} value={expires} onChange={e => setExpires(e.target.value)} /></Field>
      </div>
      <RolePicker value={form.audience_roles} onChange={roles => setForm({ ...form, audience_roles: roles })} />
      <TargetPicker value={form.targets} onChange={targets => setForm({ ...form, targets })} />
      <div><p className={ui.label}>{t('m9Attachments')}</p><AttachmentList items={files} pathFor={f => current && current.attachments.some(a => a.id === f.id) ? `/notices/${current.id}/files/${f.id}` : ''} onRemove={id => setFiles(files.filter(f => f.id !== id))} />
        <div className="mt-2"><FileUploader path="/notices/files" max={10 - files.length} onBusy={setUploading} onUploaded={f => setFiles(prev => [...prev, f])} /></div></div>
      <div className="flex flex-col gap-3 border-t border-slate-100 pt-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="flex flex-wrap gap-2">
          <button type="submit" className={ui.secondary} disabled={busy || uploading || current?.status === 'archived'}>{t('m9SaveDraft')}</button>
          <button type="button" className={ui.primary} disabled={busy || uploading || current?.status === 'archived' || !form.title_en || form.audience_roles.length === 0} onClick={() => void saveAndPublish(null)}>{current?.status === 'published' ? t('m9SaveAndKeepLive') : t('m9PublishNow')}</button>
          <button type="button" className={ui.secondary} onClick={() => setEditing(null)}>{t('m9Close')}</button>
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <Field label={t('m9ScheduleFor')}><input type="datetime-local" className={ui.field} value={scheduleAt} onChange={e => setScheduleAt(e.target.value)} /></Field>
          <button type="button" className={ui.secondary} disabled={busy || !scheduleAt || current?.status === 'archived'} onClick={() => void saveAndPublish(localToIso(scheduleAt))}>{t('m9Schedule')}</button>
          {current && current.status !== 'archived' && <button type="button" className={ui.secondary} disabled={busy} onClick={async () => { if ((await confirmAction(t('m9ConfirmArchive')))) void run(() => authorized(token => archiveNotice(token, current.id)), 'm9ArchivedOk', true) }}>{t('m9Archive')}</button>}
          {current?.status === 'draft' && <button type="button" className={`${ui.secondary} text-rose-700`} disabled={busy} onClick={async () => { if ((await confirmAction(t('m9ConfirmDelete')))) void run(() => authorized(token => deleteNotice(token, current.id)), 'm9DeletedOk', true) }}>{t('m9Delete')}</button>}
        </div>
      </div>
    </form> : <div className="flex flex-wrap items-center justify-between gap-3">
      <label><span className="sr-only">{t('m9Status')}</span><select className={`${ui.field} mt-0`} value={status} onChange={e => { setStatus(e.target.value); setPage(1) }}><option value="">{t('m9AllStatuses')}</option>{['draft', 'published', 'scheduled', 'expired', 'archived'].map(s => <option key={s} value={s}>{t(`m9Status_${s}`)}</option>)}</select></label>
      <button type="button" className={ui.primary} onClick={() => open('new')}>{t('m9NewNotice')}</button>
    </div>}
    {!editing && (list.isPending ? <Skeleton rows={3} /> : list.isError ? <Banner tone="error">{t('m9ErrGeneric')}</Banner> : list.data.items.length === 0 ? <EmptyState title={t('m9NoNotices')} /> : <>
      <ul className={`${ui.card} divide-y divide-slate-100`}>{list.data.items.map(n => <li key={n.id} className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><PriorityBadge priority={n.priority} /><span className="text-xs font-semibold uppercase text-slate-500">{t(`m9Status_${n.status}`)}{n.status === 'published' && !n.live ? ` · ${t(n.publish_at && new Date(n.publish_at) > new Date() ? 'm9Scheduled' : 'm9Expired')}` : ''}</span></div>
          <p className="mt-1 truncate font-semibold">{local(n.title_en, n.title_bn)}</p><p className="text-xs text-slate-500">{n.audience_roles.map(r => t(`role_${r}`)).join(', ')} · {fmt.dateTime(n.updated_at)}</p></div>
        <button type="button" className={ui.secondary} onClick={() => open(n)}>{t('m9Edit')}</button></li>)}</ul>
      <Pager page={page} total={list.data.total} size={10} onPage={setPage} previous={t('m9Previous')} next={t('m9Next')} label={t('m9ManageNotices')} />
    </>)}
  </div>
}
