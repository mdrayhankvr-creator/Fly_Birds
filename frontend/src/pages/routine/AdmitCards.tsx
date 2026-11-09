import { useEffect, useId, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../api/client'
import { saveBlob } from '../../api/admin'
import { downloadMyAdmitCard, getAdmitSettings, getAdmitTerms, getHeldStudents, getMyAdmitCards, printAdmitCards, releaseAdmitCard, saveAdmitSettings, unreleaseAdmitCard, type AdmitStudent, type AdmitTerm } from '../../api/marksheetAdmit'
import { Icon } from '../../components/icons'
import { CardTitle, EmptyState, Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { useDay, useNum } from './routineShared'

// Admit cards (প্রবেশপত্র) from published term routines: staff print a
// section or class, the office sees and releases students held for dues,
// and the administrator sets the rules and the hold.

type Lang = 'en' | 'bn'
type Message = { tone: 'success' | 'error' | 'info'; text: string } | null

export function useAdmitError() {
  const { t } = useTranslation()
  return (cause: unknown) => {
    if (cause instanceof ApiError) {
      const text = t(`msAdmitErr_${cause.code}`, { defaultValue: '' })
      if (text) return text
      if (cause.status === 403) return t('msErrForbidden')
    }
    return t('msAdmitErrGeneric')
  }
}

export function AdmitCardsPage({ canPrint, canRelease, canSettings }: { canPrint: boolean; canRelease: boolean; canSettings: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const uid = useId()
  const num = useNum()
  const terms = useQuery({ queryKey: ['ms', 'admit-terms'], queryFn: () => authorized(getAdmitTerms), retry: false })
  const [termId, setTermId] = useState('')
  useEffect(() => { if (!termId && terms.data?.length) setTermId(terms.data[0].term_id) }, [terms.data, termId])
  const term = terms.data?.find(x => x.term_id === termId)
  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_academics')} title={t('msAdmitTitle')} description={t('msAdmitIntro')} />
    {terms.isPending ? <Skeleton rows={3} /> : terms.isError ? <Notice tone="error">{t('msLoadError')}</Notice> : terms.data.length === 0 ?
      <EmptyState icon="card" title={t('msAdmitNoTerms')}>{t('msAdmitNoTermsHint')}</EmptyState> : <>
        <section className={`${ui.card} p-4 sm:p-5`} aria-label={t('msAdmitTerm')}>
          <label className={`${ui.label} block max-w-md`} htmlFor={`${uid}-term`}>{t('msAdmitTerm')}
            <select id={`${uid}-term`} className={ui.field} value={termId} onChange={e => setTermId(e.target.value)}>
              {terms.data.map(x => <option key={x.term_id} value={x.term_id}>{x.term_name} · {num(x.session_name)}</option>)}
            </select>
          </label>
          {term && <p className="mt-2 text-sm text-slate-600">{t('msAdmitVersion', { n: num(String(term.version)) })}</p>}
        </section>
        {term && canPrint && <PrintCard term={term} />}
        {term && canRelease && <HeldCard term={term} />}
      </>}
    {canSettings && <SettingsCard />}
  </div>
}

function PrintCard({ term }: { term: AdmitTerm }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useAdmitError()
  const num = useNum()
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState<Message>(null)
  const classes = Array.from(new Map(term.sections.map(s => [s.class_id, s.class_name])).entries())
  // A whole class is printed only when every one of its sections is here
  // (the administrator and the head teacher).
  const wholeClass = (classId: string) => term.sections.filter(s => s.class_id === classId).length > 1
  async function print(key: string, target: { class_id?: string; section_id?: string }, language: Lang) {
    setBusy(key); setMessage({ tone: 'info', text: t('msAdmitPreparing') })
    try {
      const blob = await authorized(token => printAdmitCards(token, term.term_id, target, language))
      saveBlob(blob, `admit-cards-${key}-${language}.pdf`)
      setMessage({ tone: 'success', text: t('msAdmitDone') })
    } catch (cause) { setMessage({ tone: 'error', text: explain(cause) }) } finally { setBusy('') }
  }
  const langs: Lang[] = ['en', 'bn']
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="admit-print" data-testid="admit-print">
    <CardTitle id="admit-print" icon="print">{t('msAdmitTitle')}</CardTitle>
    {message && <div className="mt-3" role={message.tone === 'info' ? 'status' : undefined}><Notice tone={message.tone}>{message.text}</Notice></div>}
    <div className="mt-4 space-y-4">{classes.map(([classId, className]) => <div key={classId} className="rounded-xl border border-slate-200">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-3 py-2.5">
        <p className="font-semibold text-slate-900">{className}</p>
        {wholeClass(classId) && <div className="flex flex-wrap gap-2">{langs.map(l => <button key={l} type="button" className={`${ui.secondary} btn-sm`} disabled={!!busy}
          aria-label={`${t('msAdmitWholeClass', { class: className })} — ${l === 'en' ? 'English' : 'বাংলা'}`} onClick={() => void print(`class-${className}`, { class_id: classId }, l)}>
          <Icon name="download" className="h-4 w-4" />{t('msAdmitWholeClass', { class: className })} · {l === 'en' ? 'EN' : 'বাংলা'}</button>)}</div>}
      </div>
      <ul className="divide-y divide-slate-100">{term.sections.filter(s => s.class_id === classId).map(s => <li key={s.section_id} className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5">
        <div className="min-w-0 text-sm">
          <p className="font-semibold text-slate-900">{t('msSection')} {s.section_name}</p>
          <p className="text-slate-600">{t('msAdmitStudents')}: {num(String(s.students))}{s.held > 0 && <span className="ml-2 font-semibold text-amber-900">{t('msAdmitHeldCol')}: {num(String(s.held))}</span>}</p>
        </div>
        <div className="flex flex-wrap gap-2">{langs.map(l => <button key={l} type="button" className={`${ui.primary} btn-sm`} disabled={!!busy} aria-busy={busy === `${s.section_id}-${l}`}
          aria-label={t('msAdmitPrintSection', { class: className, section: s.section_name, language: l === 'en' ? 'English' : 'বাংলা' })}
          onClick={() => void print(`${className}-${s.section_name}`.replace(/\s+/g, '-'), { section_id: s.section_id }, l)}>
          <Icon name="print" className="h-4 w-4" />{l === 'en' ? 'English' : 'বাংলা'}</button>)}</div>
      </li>)}</ul>
    </div>)}</div>
  </section>
}

function HeldCard({ term }: { term: AdmitTerm }) {
  const { t, i18n } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const explain = useAdmitError()
  const num = useNum()
  const day = useDay()
  const settings = useQuery({ queryKey: ['ms', 'admit-settings'], queryFn: () => authorized(getAdmitSettings), retry: false })
  const held = useQuery({ queryKey: ['ms', 'admit-held', term.term_id], queryFn: () => authorized(token => getHeldStudents(token, term.term_id)), retry: false })
  const [notes, setNotes] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState<Message>(null)
  const bn = i18n.language === 'bn'
  const nameOf = (s: AdmitStudent) => bn && s.name_bn ? s.name_bn : s.name_en
  async function toggle(s: AdmitStudent) {
    setBusy(s.student_id); setMessage(null)
    try {
      if (s.released) await authorized(token => unreleaseAdmitCard(token, term.term_id, s.student_id))
      else await authorized(token => releaseAdmitCard(token, term.term_id, s.student_id, notes[s.student_id] ?? ''))
      await client.invalidateQueries({ queryKey: ['ms'] })
      setMessage({ tone: 'success', text: t(s.released ? 'msAdmitHeldMsg' : 'msAdmitReleasedMsg', { name: nameOf(s) }) })
    } catch (cause) { setMessage({ tone: 'error', text: explain(cause) }) } finally { setBusy('') }
  }
  const on = settings.data?.withhold_for_dues
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="admit-held" data-testid="admit-held">
    <CardTitle id="admit-held" icon="money">{t('msAdmitHeldTitle')}</CardTitle>
    {settings.data && <p className="mt-1 text-sm text-slate-600">{on && settings.data.dues_cutoff ? t('msAdmitHeldIntro', { date: day(settings.data.dues_cutoff) }) : t('msAdmitHeldOff')}</p>}
    {message && <div className="mt-3"><Notice tone={message.tone}>{message.text}</Notice></div>}
    {on && (held.isPending ? <Skeleton rows={2} /> : held.isError ? <Notice tone="error">{t('msLoadError')}</Notice> : held.data.length === 0 ?
      <p className="mt-3 text-sm text-slate-600">{t('msAdmitNoHeld')}</p> :
      <ul className="mt-4 divide-y divide-slate-100">{held.data.map(s => <li key={s.student_id} className="grid gap-2 py-3 md:grid-cols-[minmax(0,1fr)_minmax(0,14rem)_auto] md:items-center">
        <div className="min-w-0 text-sm">
          <p className="font-semibold text-slate-900">{nameOf(s)} <span className="font-normal text-slate-600">· {s.class_name} {s.section_name}{s.roll_no != null ? ` · ${t('msRoll')} ${num(String(s.roll_no))}` : ''}</span></p>
          <p className="text-slate-700">{t('msAdmitDue')}: <strong className="tabular-nums">{num(s.due)}</strong>{s.oldest_due && <> · {t('msAdmitOldest')}: {day(s.oldest_due)}</>}</p>
          <p className="mt-0.5"><span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${s.released ? 'bg-emerald-50 text-emerald-800 ring-emerald-600/30' : 'bg-amber-50 text-amber-900 ring-amber-600/30'}`}>{t(s.released ? 'msAdmitReleasedState' : 'msAdmitHeldState')}</span>
            {s.released && s.release_note && <span className="ml-2 text-xs text-slate-600">{s.release_note}</span>}</p>
        </div>
        {!s.released ? <input aria-label={t('msAdmitNoteFor', { name: nameOf(s) })} placeholder={t('msAdmitNote')} maxLength={240} className={ui.field} value={notes[s.student_id] ?? ''} onChange={e => setNotes({ ...notes, [s.student_id]: e.target.value })} /> : <span />}
        <button type="button" className={s.released ? ui.secondary : ui.primary} disabled={!!busy} aria-busy={busy === s.student_id} onClick={() => void toggle(s)}>{t(s.released ? 'msAdmitHoldAgain' : 'msAdmitRelease')}</button>
      </li>)}</ul>)}
  </section>
}

function SettingsCard() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const explain = useAdmitError()
  const uid = useId()
  const settings = useQuery({ queryKey: ['ms', 'admit-settings'], queryFn: () => authorized(getAdmitSettings), retry: false })
  const [form, setForm] = useState<{ rules_en: string; rules_bn: string; withhold_for_dues: boolean; dues_cutoff: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<Message>(null)
  useEffect(() => {
    if (settings.data && !form) setForm({ rules_en: settings.data.rules_en, rules_bn: settings.data.rules_bn, withhold_for_dues: settings.data.withhold_for_dues, dues_cutoff: settings.data.dues_cutoff ?? '' })
  }, [settings.data, form])
  if (settings.isPending || !form) return settings.isError ? <Notice tone="error">{t('msLoadError')}</Notice> : <Skeleton rows={3} />
  async function save() {
    if (!form) return
    setBusy(true); setMessage(null)
    try {
      await authorized(token => saveAdmitSettings(token, { ...form, dues_cutoff: form.dues_cutoff || null }))
      await client.invalidateQueries({ queryKey: ['ms'] })
      setMessage({ tone: 'success', text: t('msAdmitSettingsSaved') })
    } catch (cause) { setMessage({ tone: 'error', text: explain(cause) }) } finally { setBusy(false) }
  }
  return <form className={`${ui.card} space-y-4 p-4 sm:p-5`} aria-labelledby="admit-settings" onSubmit={e => { e.preventDefault(); void save() }} data-testid="admit-settings">
    <CardTitle id="admit-settings" icon="settings">{t('msAdmitSettingsTitle')}</CardTitle>
    <div className="grid gap-4 lg:grid-cols-2">
      <label className={ui.label} htmlFor={`${uid}-en`}>{t('msAdmitRulesEN')}
        <textarea id={`${uid}-en`} className={`${ui.field} min-h-36`} maxLength={2000} placeholder={settings.data?.default_rules_en} value={form.rules_en} onChange={e => setForm({ ...form, rules_en: e.target.value })} /></label>
      <label className={ui.label} htmlFor={`${uid}-bn`}>{t('msAdmitRulesBN')}
        <textarea id={`${uid}-bn`} className={`${ui.field} min-h-36`} maxLength={2000} placeholder={settings.data?.default_rules_bn} value={form.rules_bn} onChange={e => setForm({ ...form, rules_bn: e.target.value })} /></label>
    </div>
    <p className="text-sm text-slate-600">{t('msAdmitRulesHint')}</p>
    <div className="flex flex-wrap items-end gap-4">
      <label className="flex min-h-10 items-center gap-2 text-sm font-semibold text-slate-800"><input type="checkbox" checked={form.withhold_for_dues} onChange={e => setForm({ ...form, withhold_for_dues: e.target.checked })} />{t('msAdmitHold')}</label>
      <label className={`${ui.label} w-48`} htmlFor={`${uid}-cutoff`}>{t('msAdmitCutoff')}
        <input id={`${uid}-cutoff`} type="date" className={ui.field} required={form.withhold_for_dues} value={form.dues_cutoff} onChange={e => setForm({ ...form, dues_cutoff: e.target.value })} /></label>
    </div>
    <p className="text-sm text-slate-600">{t('msAdmitHoldHint')}</p>
    {message && <Notice tone={message.tone}>{message.text}</Notice>}
    <button className={ui.primary} disabled={busy}>{t('msAdmitSaveSettings')}</button>
  </form>
}

// MyAdmitCards: a student's own admit cards, once a routine is published.
export function MyAdmitCards() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useAdmitError()
  const num = useNum()
  const cards = useQuery({ queryKey: ['ms', 'my-admit'], queryFn: () => authorized(getMyAdmitCards), retry: false })
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  async function get(termId: string, language: Lang) {
    setBusy(`${termId}-${language}`); setError('')
    try { saveBlob(await authorized(token => downloadMyAdmitCard(token, termId, language)), `admit-card-${language}.pdf`) }
    catch (cause) { setError(explain(cause)) } finally { setBusy('') }
  }
  if (cards.isPending) return <Skeleton rows={1} />
  if (cards.isError) return null
  return <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="my-admit" data-testid="my-admit-cards">
    <CardTitle id="my-admit" icon="card">{t('msMyAdmitTitle')}</CardTitle>
    {cards.data.length === 0 ? <p className="mt-2 text-sm text-slate-600">{t('msMyAdmitNone')}</p> :
      <ul className="mt-3 divide-y divide-slate-100">{cards.data.map(c => <li key={c.term_id} className="flex flex-wrap items-center justify-between gap-3 py-3">
        <div className="min-w-0"><p className="font-semibold text-slate-900">{c.term_name}</p><p className="text-xs text-slate-600">{t('msAdmitVersion', { n: num(String(c.version)) })}</p></div>
        {c.held ? <p className="text-sm font-semibold text-amber-900">{t('msMyAdmitHeld')}</p> :
          <div className="flex flex-wrap gap-2">{(['en', 'bn'] as const).map(l => <button key={l} type="button" className={ui.secondary} disabled={!!busy} aria-busy={busy === `${c.term_id}-${l}`} onClick={() => void get(c.term_id, l)}>
            <Icon name="download" className="h-4 w-4" />{t(l === 'en' ? 'msDownloadEnglish' : 'msDownloadBangla')}</button>)}</div>}
      </li>)}</ul>}
    {error && <div className="mt-3"><Notice tone="error">{error}</Notice></div>}
  </section>
}
