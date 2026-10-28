import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getClassRoutine, type ClassRoutineSlot } from '../../api/client'
import { useFmt } from '../../pages/AnalyticsPages'
import { useApi, useLocal } from '../../pages/commShared'
import { Icon } from '../icons'
import { Empty, Lines, Panel, Unavailable, ViewAll, tone } from './kit'

// Today's classes from the published routine as a swipeable strip. It opens
// on the class in progress (or the next one), marks Done, Now and Next from
// the Dhaka clock, and has a period indicator that can be tapped.

const DHAKA = 'Asia/Dhaka'
function dhakaClock() {
  const parts = new Intl.DateTimeFormat('en-GB', { timeZone: DHAKA, weekday: 'short', hour: '2-digit', minute: '2-digit', hour12: false }).formatToParts(new Date())
  const get = (type: string) => parts.find(p => p.type === type)?.value ?? ''
  return { weekday: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].indexOf(get('weekday')), minute: (Number(get('hour')) % 24) * 60 + Number(get('minute')) }
}
const minutes = (time: string) => { const [h, m] = time.split(':').map(Number); return h * 60 + m }

// A subject keeps one colour wherever it appears; its name is always printed beside it.
const SUBJECT_COLOURS = ['#0284c7', '#7c3aed', '#059669', '#e11d48', '#d97706', '#0d9488', '#2563eb', '#c026d3', '#ea580c', '#4f46e5']
export function subjectColour(id: string) {
  let h = 0
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return SUBJECT_COLOURS[h % SUBJECT_COLOURS.length]
}

type State = 'done' | 'now' | 'next' | 'later'

export function ClassStrip({ path, title, teacherView = false, link }: { path: string; title: string; teacherView?: boolean; link?: string }) {
  const { t } = useTranslation()
  const api = useApi()
  const local = useLocal()
  const f = useFmt()
  const [clock, setClock] = useState(dhakaClock)
  useEffect(() => { const id = window.setInterval(() => setClock(dhakaClock()), 60_000); return () => window.clearInterval(id) }, [])
  const q = useQuery({ queryKey: ['dash', 'routine', path], queryFn: () => api(token => getClassRoutine(token, path, new URLSearchParams({ page: '1', page_size: '100' }))), retry: false })
  const slots = (q.data?.items ?? []).filter((s: ClassRoutineSlot) => s.day_of_week === clock.weekday).sort((a, b) => a.start_time.localeCompare(b.start_time))
  const nowIdx = slots.findIndex(s => clock.minute >= minutes(s.start_time) && clock.minute < minutes(s.end_time))
  const nextIdx = slots.findIndex(s => minutes(s.start_time) > clock.minute)
  const stateOf = (i: number): State => i === nowIdx ? 'now' : i === nextIdx ? 'next' : minutes(slots[i].end_time) <= clock.minute ? 'done' : 'later'
  const left = slots.filter(s => minutes(s.end_time) > clock.minute).length
  const focus = nowIdx >= 0 ? nowIdx : nextIdx >= 0 ? nextIdx : Math.max(0, slots.length - 1)

  const track = useRef<HTMLOListElement>(null)
  const [active, setActive] = useState(0)
  const go = (i: number, smooth = true) => {
    const el = track.current
    const first = el?.children[0] as HTMLElement | undefined
    const card = el?.children[i] as HTMLElement | undefined
    if (!el || !first || !card) return
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    el.scrollTo({ left: card.offsetLeft - first.offsetLeft, behavior: smooth && !reduce ? 'smooth' : 'auto' })
    setActive(i)
  }
  // Open on the live class once the routine arrives.
  const opened = useRef(false)
  useEffect(() => { if (!opened.current && slots.length) { opened.current = true; go(focus, false) } }) // eslint-disable-line react-hooks/exhaustive-deps
  const onScroll = () => {
    const el = track.current
    if (!el || !el.children.length) return
    const kids = Array.from(el.children) as HTMLElement[]
    if (el.scrollLeft + el.clientWidth >= el.scrollWidth - 4) { setActive(kids.length - 1); return }
    let best = 0, dist = Infinity
    kids.forEach((c, i) => { const d = Math.abs(c.offsetLeft - kids[0].offsetLeft - el.scrollLeft); if (d < dist) { dist = d; best = i } })
    setActive(best)
  }

  const label = (s: ClassRoutineSlot) => teacherView ? `${s.class_name} · ${s.section_name}` : local(s.subject_name, s.subject_name_bn)
  const line = (s: ClassRoutineSlot) => teacherView ? local(s.subject_name, s.subject_name_bn) : s.teacher_name
  // Times stay in 24-hour form; digits follow the language (Bangla digits in Bangla).
  const time = (s: ClassRoutineSlot) => `${s.start_time.slice(0, 5)}–${s.end_time.slice(0, 5)}`.replace(/\d/g, d => f.num(Number(d)))
  const nav = slots.length > 1 && <span className="hidden items-center gap-1.5 sm:flex">
    <button type="button" onClick={() => go(Math.max(0, active - 1))} disabled={active === 0} aria-label={t('p1PrevClass')} className="grid h-9 w-9 place-items-center rounded-full border border-slate-200 bg-white text-slate-600 hover:bg-slate-50 disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]"><Icon name="chevronLeft" className="h-4 w-4" /></button>
    <button type="button" onClick={() => go(Math.min(slots.length - 1, active + 1))} disabled={active >= slots.length - 1} aria-label={t('p1NextClass')} className="grid h-9 w-9 place-items-center rounded-full border border-slate-200 bg-white text-slate-600 hover:bg-slate-50 disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]"><Icon name="chevronRight" className="h-4 w-4" /></button>
  </span>

  return <Panel title={title} icon="clock" module="routine" action={<span className="flex items-center gap-2">{nav}{link && <ViewAll to={link} />}</span>}>
    {q.isPending ? <Lines /> : q.isError ? <Unavailable error={q.error} /> : slots.length === 0 ? <Empty>{clock.weekday === 5 ? t('m12WeeklyHoliday') : t('m12NoClassesToday')}</Empty> : <>
      <p className="-mt-2 mb-3 text-xs font-medium text-slate-500">{left > 0 ? t('p1ClassesLeft', { left: f.num(left), total: f.num(slots.length) }) : t('p1AllClassesDone')}</p>
      <ol ref={track} onScroll={onScroll} aria-label={title} className="ds-scroll-x ds-snap -mx-4 flex gap-3 px-4 pb-1 sm:-mx-5 sm:px-5">
        {slots.map((s, i) => {
          const st = stateOf(i)
          const size = 'w-[80%] shrink-0 min-[420px]:w-[70%] sm:w-[calc(50%-6px)] xl:w-[calc(33.333%-8px)]'
          if (st === 'now') {
            const total = minutes(s.end_time) - minutes(s.start_time), gone = clock.minute - minutes(s.start_time)
            return <li key={s.id} className={`${size} ds-fill relative flex min-h-[156px] flex-col rounded-[20px] p-4`} style={tone(subjectColour(s.subject_id))} aria-current="time">
              <div className="flex items-center justify-between gap-2">
                <span className="inline-flex items-center gap-1.5 rounded-full bg-white px-2 py-0.5 text-[11px] font-bold text-slate-900"><span className="ds-blink h-1.5 w-1.5 rounded-full bg-rose-600" />{t('m12Now')}</span>
                <span className="text-xs font-semibold tabular-nums text-white/90">{time(s)}</span>
              </div>
              <p className="mt-3 truncate text-lg font-extrabold leading-tight">{label(s)}</p>
              <p className="truncate text-xs text-white/90">{line(s)}</p>
              <div className="mt-auto pt-3">
                <div className="flex items-center justify-between gap-2 text-[11px] font-semibold text-white/90"><span>{s.room && t('p1Room', { room: s.room })}</span><span>{t('p1MinLeft', { n: f.num(total - gone) })}</span></div>
                <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-white/25"><span className="block h-full rounded-full bg-white" style={{ width: `${Math.round((gone / total) * 100)}%` }} /></div>
              </div>
            </li>
          }
          const done = st === 'done', next = st === 'next'
          return <li key={s.id} className={`${size} relative flex min-h-[156px] flex-col rounded-[20px] p-4 ${done ? 'ds-sunken' : 'ds-tint'} ${next ? 'ring-2 ring-[color-mix(in_oklab,var(--s)_50%,transparent)]' : ''}`} style={tone(subjectColour(s.subject_id))}>
            <div className="flex items-center justify-between gap-2">
              {next ? <span className="ds-fill rounded-full px-2 py-0.5 text-[11px] font-bold">{t('p1Next')} · {t('p1InMin', { n: f.num(minutes(s.start_time) - clock.minute) })}</span>
                : done ? <span className="inline-flex items-center gap-1 rounded-full bg-white px-2 py-0.5 text-[11px] font-bold text-emerald-700"><Icon name="check" className="h-3 w-3" />{t('p1Done')}</span>
                : <span />}
              <span className="text-xs font-semibold tabular-nums text-slate-600">{time(s)}</span>
            </div>
            <div className={`mt-3 ${done ? 'opacity-75' : ''}`}>
              <p className="ds-ink truncate text-lg font-extrabold leading-tight">{label(s)}</p>
              <p className="truncate text-xs text-slate-600">{line(s)}</p>
            </div>
            {s.room && <p className="mt-auto pt-3 text-xs font-semibold text-slate-600">{t('p1Room', { room: s.room })}</p>}
          </li>
        })}
      </ol>
      {slots.length > 1 && <ol className="mt-3 flex items-center gap-1" aria-label={t('p1Periods')}>
        {slots.map((s, i) => {
          const st = stateOf(i)
          const bar = st === 'now' ? 'bg-[var(--s)]' : st === 'done' ? 'bg-slate-400 opacity-40' : 'ds-soft'
          return <li key={s.id} className="min-w-0 flex-1">
            <button type="button" onClick={() => go(i)} aria-current={active === i ? 'true' : undefined} style={tone(subjectColour(s.subject_id))}
              aria-label={`${label(s)}, ${time(s)}${st === 'now' ? `, ${t('m12Now')}` : st === 'next' ? `, ${t('p1Next')}` : st === 'done' ? `, ${t('p1Done')}` : ''}`}
              className="flex min-h-[28px] w-full items-center rounded focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]">
              <span className={`block w-full rounded-full transition-all ${bar} ${active === i ? 'h-2.5 ring-2 ring-[color-mix(in_oklab,var(--s)_45%,transparent)]' : 'h-1.5'}`} />
            </button>
          </li>
        })}
      </ol>}
    </>}
  </Panel>
}
