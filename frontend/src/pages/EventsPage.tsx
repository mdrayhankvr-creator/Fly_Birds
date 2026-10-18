import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { deleteEvent, getEvents, saveEvent, setEventStatus, type EventInput, type EventKind, type SchoolEvent } from '../api/communication'
import { EmptyState, Notice as Banner, PageHeader, Skeleton, Tabs, ui } from './attendanceUi'
import { Field, RolePicker, TargetPicker, todayDhaka, useApi, useDhakaFormat, useErrorText, useLocal } from './commShared'

const kinds: EventKind[] = ['holiday', 'exam', 'meeting', 'event', 'other']
const kindTone: Record<EventKind, string> = { holiday: 'bg-emerald-100 text-emerald-900 border-emerald-300', exam: 'bg-rose-100 text-rose-900 border-rose-300', meeting: 'bg-sky-100 text-sky-900 border-sky-300', event: 'bg-violet-100 text-violet-900 border-violet-300', other: 'bg-slate-100 text-slate-800 border-slate-300' }
const blank = (day: string): EventInput => ({ title_en: '', title_bn: '', description_en: '', description_bn: '', kind: 'event', starts_on: day, ends_on: day, start_time: null, end_time: null, location: '', audience_roles: ['student', 'guardian', 'teacher', 'head_teacher', 'accountant'], targets: [] })

function monthRange(month: string) {
  const [y, m] = month.split('-').map(Number)
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate()
  return { from: `${month}-01`, to: `${month}-${String(last).padStart(2, '0')}`, days: last, offset: (new Date(Date.UTC(y, m - 1, 1)).getUTCDay() + 6) % 7 }
}
function shiftMonth(month: string, delta: number) {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1 + delta, 1))
  return d.toISOString().slice(0, 7)
}

export function EventsPage({ canManage }: { canManage: boolean }) {
  const { t, i18n } = useTranslation()
  const authorized = useApi()
  const client = useQueryClient()
  const local = useLocal()
  const fmt = useDhakaFormat()
  const errorText = useErrorText()
  const [month, setMonth] = useState(todayDhaka().slice(0, 7))
  const [view, setView] = useState<'calendar' | 'list'>('calendar')
  const [editing, setEditing] = useState<SchoolEvent | 'new' | null>(null)
  const [form, setForm] = useState<EventInput>(blank(todayDhaka()))
  const [allDay, setAllDay] = useState(true)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const range = monthRange(month)
  const params = new URLSearchParams({ from: range.from, to: range.to, ...(canManage ? { manage: '1' } : {}) })
  const events = useQuery({ queryKey: ['m9', 'events', params.toString()], queryFn: () => authorized(token => getEvents(token, params)), retry: false })
  const locale = i18n.language === 'bn' ? 'bn-BD' : 'en-GB'
  const today = todayDhaka()
  const weekdays = Array.from({ length: 7 }, (_, i) => new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' }).format(new Date(Date.UTC(2024, 0, 1 + i))))
  const onDay = (day: string) => (events.data ?? []).filter(e => e.starts_on <= day && e.ends_on >= day)

  function open(e: SchoolEvent | 'new', day = today) {
    setEditing(e); setMessage(null)
    if (e === 'new') { setForm(blank(day)); setAllDay(true) }
    else { setForm({ title_en: e.title_en, title_bn: e.title_bn, description_en: e.description_en, description_bn: e.description_bn, kind: e.kind, starts_on: e.starts_on, ends_on: e.ends_on, start_time: e.start_time, end_time: e.end_time, location: e.location, audience_roles: e.audience_roles, targets: e.targets }); setAllDay(!e.start_time) }
  }
  async function run(task: () => Promise<unknown>, success: string, close = false) {
    setBusy(true); setMessage(null)
    try {
      const result = await task()
      await client.invalidateQueries({ queryKey: ['m9', 'events'] })
      if (close) setEditing(null)
      else if (result && typeof result === 'object' && 'id' in result) open(result as SchoolEvent)
      setMessage({ tone: 'success', text: t(success) })
    } catch (cause) { setMessage({ tone: 'error', text: errorText(cause) }) } finally { setBusy(false) }
  }
  const current = editing && editing !== 'new' ? editing : null
  const payload = () => ({ ...form, start_time: allDay ? null : form.start_time, end_time: allDay ? null : form.end_time })
  function submit(event: FormEvent) {
    event.preventDefault()
    void run(() => authorized(token => saveEvent(token, payload(), current?.id)), 'm9Saved')
  }
  const timeText = (e: SchoolEvent) => e.start_time ? `${e.start_time}–${e.end_time}` : t('m9AllDay')
  const chip = (e: SchoolEvent) => `${kindTone[e.kind]} ${e.status === 'cancelled' ? 'line-through opacity-70' : ''} ${e.status === 'draft' ? 'border-dashed' : ''}`

  return <div className="space-y-5">
    <PageHeader eyebrow={t('m9EventsEyebrow')} title={t('m9Events')} description={t('m9EventsIntro')} actions={canManage ? <button type="button" className={ui.primary} onClick={() => open('new')}>{t('m9NewEvent')}</button> : undefined} />
    {message && <Banner tone={message.tone}>{message.text}</Banner>}
    {editing && <form onSubmit={submit} className={`${ui.card} space-y-4 p-4 sm:p-6`} aria-labelledby="event-editor">
      <div className="flex flex-wrap items-center justify-between gap-2"><h2 id="event-editor" className="text-lg font-bold">{current ? t('m9EditEvent') : t('m9NewEvent')}</h2>{current && <span className="text-sm text-slate-500">{t(`m9Status_${current.status}`)}</span>}</div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Field label={t('m9TitleEn')} className="lg:col-span-2"><input required maxLength={200} className={ui.field} value={form.title_en} onChange={e => setForm({ ...form, title_en: e.target.value })} /></Field>
        <Field label={t('m9TitleBn')} className="lg:col-span-2"><input lang="bn" className={ui.field} value={form.title_bn} onChange={e => setForm({ ...form, title_bn: e.target.value })} /></Field>
        <Field label={t('m9Kind')}><select className={ui.field} value={form.kind} onChange={e => setForm({ ...form, kind: e.target.value as EventKind })}>{kinds.map(k => <option key={k} value={k}>{t(`m9Kind_${k}`)}</option>)}</select></Field>
        <Field label={t('m9StartsOn')}><input required type="date" className={ui.field} value={form.starts_on} onChange={e => setForm({ ...form, starts_on: e.target.value, ends_on: form.ends_on < e.target.value ? e.target.value : form.ends_on })} /></Field>
        <Field label={t('m9EndsOn')}><input required type="date" min={form.starts_on} className={ui.field} value={form.ends_on} onChange={e => setForm({ ...form, ends_on: e.target.value })} /></Field>
        <Field label={t('m9Location')}><input maxLength={300} className={ui.field} value={form.location} onChange={e => setForm({ ...form, location: e.target.value })} /></Field>
        <label className="flex items-center gap-2 self-end pb-2 text-sm font-medium"><input type="checkbox" checked={allDay} onChange={e => setAllDay(e.target.checked)} />{t('m9AllDay')}</label>
        {!allDay && <><Field label={t('m9StartTime')}><input required type="time" className={ui.field} value={form.start_time ?? ''} onChange={e => setForm({ ...form, start_time: e.target.value })} /></Field>
          <Field label={t('m9EndTime')}><input required type="time" className={ui.field} value={form.end_time ?? ''} onChange={e => setForm({ ...form, end_time: e.target.value })} /></Field></>}
        <Field label={t('m9DescriptionEn')} className="md:col-span-2"><textarea rows={3} className={ui.field} value={form.description_en} onChange={e => setForm({ ...form, description_en: e.target.value })} /></Field>
        <Field label={t('m9DescriptionBn')} className="md:col-span-2"><textarea lang="bn" rows={3} className={ui.field} value={form.description_bn} onChange={e => setForm({ ...form, description_bn: e.target.value })} /></Field>
      </div>
      <RolePicker value={form.audience_roles} onChange={roles => setForm({ ...form, audience_roles: roles })} />
      <TargetPicker value={form.targets} onChange={targets => setForm({ ...form, targets })} />
      <div className="flex flex-wrap gap-2 border-t border-slate-100 pt-4">
        <button type="submit" className={ui.secondary} disabled={busy}>{t('m9Save')}</button>
        {(!current || current.status !== 'published') && <button type="button" className={ui.primary} disabled={busy || !form.title_en || form.audience_roles.length === 0} onClick={() => void run(async () => { const saved = await authorized(token => saveEvent(token, payload(), current?.id)); return authorized(token => setEventStatus(token, saved.id, 'publish')) }, 'm9PublishedOk')}>{t('m9Publish')}</button>}
        {current?.status === 'published' && <button type="button" className={ui.secondary} disabled={busy} onClick={() => { if (window.confirm(t('m9ConfirmCancelEvent'))) void run(() => authorized(token => setEventStatus(token, current.id, 'cancel')), 'm9CancelledOk') }}>{t('m9CancelEvent')}</button>}
        {current?.status === 'draft' && <button type="button" className={`${ui.secondary} text-rose-700`} disabled={busy} onClick={() => { if (window.confirm(t('m9ConfirmDelete'))) void run(() => authorized(token => deleteEvent(token, current.id)), 'm9DeletedOk', true) }}>{t('m9Delete')}</button>}
        <button type="button" className={ui.secondary} onClick={() => setEditing(null)}>{t('m9Close')}</button>
      </div>
    </form>}

    <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="calendar-title">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <button type="button" className={ui.secondary} aria-label={t('m9PreviousMonth')} onClick={() => setMonth(shiftMonth(month, -1))}>‹</button>
          <h2 id="calendar-title" className="min-w-40 text-center text-lg font-bold">{new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${month}-01T00:00:00Z`))}</h2>
          <button type="button" className={ui.secondary} aria-label={t('m9NextMonth')} onClick={() => setMonth(shiftMonth(month, 1))}>›</button>
          <button type="button" className={ui.ghost} onClick={() => setMonth(today.slice(0, 7))}>{t('m9ThisMonth')}</button>
        </div>
        <Tabs label={t('m9View')} value={view} onChange={setView} items={[{ value: 'calendar', label: t('m9CalendarView') }, { value: 'list', label: t('m9ListView') }]} />
      </div>
      <div className="mt-3 flex flex-wrap gap-3 text-xs">{kinds.map(k => <span key={k} className={`rounded-full border px-2 py-0.5 ${kindTone[k]}`}>{t(`m9Kind_${k}`)}</span>)}</div>
      {events.isPending ? <div className="mt-4"><Skeleton rows={4} /></div> : events.isError ? <div className="mt-4"><Banner tone="error">{t('m9ErrGeneric')}</Banner></div> : view === 'calendar' ?
        <div className="mt-4 grid grid-cols-7 gap-1" role="grid" aria-label={t('m9CalendarView')}>
          {weekdays.map(d => <div key={d} role="columnheader" className="pb-1 text-center text-xs font-semibold text-slate-500">{d}</div>)}
          {Array.from({ length: range.offset }, (_, i) => <div key={`b${i}`} aria-hidden="true" />)}
          {Array.from({ length: range.days }, (_, i) => {
            const day = `${month}-${String(i + 1).padStart(2, '0')}`
            const list = onDay(day)
            return <div key={day} role="gridcell" aria-label={`${fmt.date(day)}: ${list.length ? list.map(e => local(e.title_en, e.title_bn)).join(', ') : t('m9NoEventsDay')}`}
              className={`min-h-16 rounded-lg border p-1 sm:min-h-24 ${day === today ? 'border-[var(--brand-primary)] ring-1 ring-[var(--brand-primary)]' : 'border-slate-100'} ${canManage ? 'cursor-pointer hover:bg-slate-50' : ''}`}
              onClick={canManage && list.length === 0 ? () => open('new', day) : undefined}>
              <span className="text-xs font-semibold text-slate-600">{fmt.number(i + 1)}</span>
              <ul className="mt-0.5 space-y-0.5">{list.slice(0, 3).map(e => <li key={e.id}><button type="button" disabled={!canManage} onClick={ev => { ev.stopPropagation(); open(e) }} className={`block w-full truncate rounded border px-1 text-left text-[0.65rem] font-semibold sm:text-xs ${chip(e)}`}>{local(e.title_en, e.title_bn)}</button></li>)}
                {list.length > 3 && <li className="text-[0.65rem] text-slate-500">+{list.length - 3}</li>}</ul>
            </div>
          })}
        </div> : (events.data.length === 0 ? <div className="mt-4"><EmptyState title={t('m9NoEvents')} /></div> :
          <ul className="mt-4 divide-y divide-slate-100">{events.data.map(e => <li key={e.id} className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className={`rounded-full border px-2 py-0.5 text-xs font-semibold ${chip(e)}`}>{t(`m9Kind_${e.kind}`)}</span>{e.status !== 'published' && <span className="text-xs font-bold uppercase text-slate-500">{t(`m9Status_${e.status}`)}</span>}</div>
              <p className={`mt-1 font-semibold ${e.status === 'cancelled' ? 'line-through' : ''}`}>{local(e.title_en, e.title_bn)}</p>
              <p className="text-sm text-slate-600">{fmt.date(e.starts_on)}{e.ends_on !== e.starts_on ? ` – ${fmt.date(e.ends_on)}` : ''} · {timeText(e)}{e.location ? ` · ${e.location}` : ''}</p>
              {local(e.description_en, e.description_bn) && <p className="mt-1 whitespace-pre-line text-sm text-slate-700">{local(e.description_en, e.description_bn)}</p>}</div>
            {canManage && <button type="button" className={ui.secondary} onClick={() => open(e)}>{t('m9Edit')}</button>}
          </li>)}</ul>)}
    </section>
  </div>
}
