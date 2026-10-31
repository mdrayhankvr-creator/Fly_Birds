import { useEffect, useId, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../api/client'
import { saveBlob } from '../../api/admin'
import { downloadDocument, downloadMyMarksheet, getDocument, getDocumentClasses, requestDocument, type DocumentClass, type DocumentKind, type DocumentLanguage, type DocumentRequest, type ResultDocument } from '../../api/documents'
import { Icon, type IconName } from '../../components/icons'
import { CardTitle, Notice, Skeleton, ui } from '../attendanceUi'
import { Progress } from '../marks/examUi'
import { usePeopleApi } from '../peopleShared'

// Result documents (E7): PDF marksheets and class documents, made on the
// server from published results only. Small documents arrive at once; a
// large class's marksheets are made in the background with progress.

const KINDS: { kind: DocumentKind; icon: IconName }[] = [
  { kind: 'marksheets', icon: 'users' },
  { kind: 'tabulation', icon: 'layers' },
  { kind: 'merit_list', icon: 'trend' },
  { kind: 'failed_list', icon: 'alert' },
  { kind: 'summary', icon: 'chart' },
]

export function useDocumentError() {
  const { t } = useTranslation()
  return (cause: unknown) => {
    if (cause instanceof ApiError) {
      const key = `e7Err_${cause.code}`
      const text = t(key, { defaultValue: '' })
      if (text) return text
      if (cause.status === 403) return t('e7Err_forbidden')
    }
    return t('e7Err_generic')
  }
}

type Run = { key: string; doc?: ResultDocument; error?: string }

// useDocuments asks for a document, follows it until it is ready and then
// saves the PDF. Only one document is prepared at a time.
export function useDocuments() {
  const authorized = usePeopleApi()
  const explain = useDocumentError()
  const [run, setRun] = useState<Run | null>(null)
  const alive = useRef(true)
  useEffect(() => () => { alive.current = false }, [])

  async function start(key: string, input: DocumentRequest) {
    setRun({ key })
    try {
      let doc = await authorized(token => requestDocument(token, input))
      while (alive.current && (doc.status === 'queued' || doc.status === 'running')) {
        setRun({ key, doc })
        await new Promise(resolve => setTimeout(resolve, 1200))
        doc = await authorized(token => getDocument(token, doc.id))
      }
      if (!alive.current) return
      if (doc.status !== 'done') { setRun({ key, doc, error: explain(new ApiError(500, 'document_failed')) }); return }
      setRun({ key, doc })
      const blob = await authorized(token => downloadDocument(token, doc.id))
      saveBlob(blob, doc.file_name)
      if (alive.current) setRun(null)
    } catch (cause) {
      if (alive.current) setRun({ key, error: explain(cause) })
    }
  }
  return { run, start, busy: !!run && !run.error }
}

function LanguagePicker({ value, onChange, name }: { value: DocumentLanguage; onChange: (v: DocumentLanguage) => void; name: string }) {
  const { t } = useTranslation()
  return <fieldset className="min-w-0">
    <legend className={ui.label}>{t('e7Language')}</legend>
    <div className="mt-1.5 inline-flex rounded-xl bg-slate-100 p-1">
      {(['en', 'bn'] as const).map(l => <label key={l} className={`flex min-h-10 cursor-pointer items-center rounded-lg px-3.5 text-sm font-semibold ${value === l ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600'}`}>
        <input type="radio" name={name} value={l} checked={value === l} onChange={() => onChange(l)} className="sr-only" />{l === 'en' ? 'English' : 'বাংলা'}
      </label>)}
    </div>
  </fieldset>
}

function RunStatus({ run, label }: { run: Run | null; label: string }) {
  const { t } = useTranslation()
  if (!run) return null
  if (run.error) return <Notice tone="error">{run.error}</Notice>
  const doc = run.doc
  return <div role="status" aria-live="polite" className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
    <p className="font-semibold">{label}</p>
    {doc && doc.status !== 'done' && doc.progress_total > 1 ? <div className="mt-1"><Progress done={doc.progress_done} total={doc.progress_total} label={t('e7Progress', { done: doc.progress_done, total: doc.progress_total })} /></div>
      : <p className="mt-0.5 text-slate-600">{t(doc?.status === 'done' ? 'e7Downloading' : 'e7Preparing')}</p>}
  </div>
}

// ClassDocuments is the documents of one class's term or annual result.
export function ClassDocuments({ scope, scopeId, classId, published, provisionalAllowed }: { scope: 'term' | 'annual'; scopeId: string; classId: string; published: boolean; provisionalAllowed: boolean }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const uid = useId()
  const [language, setLanguage] = useState<DocumentLanguage>('en')
  const [section, setSection] = useState('')
  const { run, start, busy } = useDocuments()
  const classes = useQuery({ queryKey: ['e7', 'classes'], queryFn: () => authorized(getDocumentClasses), enabled: published, retry: false })
  const sections = classes.data?.find(c => c.scope === scope && c.scope_id === scopeId && c.class_id === classId)?.sections ?? []
  const base = { scope, scope_id: scopeId, class_id: classId, language, ...(section ? { section_id: section } : {}) }
  const runLabel = run ? t(`e7Kind_${run.key.split(':')[0]}`) : ''
  return <section className={`${ui.card} space-y-4 p-4 sm:p-5`} aria-labelledby={`${uid}-title`}>
    <div>
      <CardTitle id={`${uid}-title`} icon="report">{t('e7DocumentsTitle')}</CardTitle>
      <p className="mt-1 text-sm text-slate-600">{t(published ? 'e7DocumentsIntro' : 'e7DocumentsUnpublished')}</p>
    </div>
    {published && <>
      <div className="flex flex-wrap items-end gap-4">
        <LanguagePicker value={language} onChange={setLanguage} name={`${uid}-lang`} />
        {sections.length > 1 && <label className={`${ui.label} w-full sm:w-56`}>{t('e7Section')}
          <select className={ui.field} value={section} onChange={e => setSection(e.target.value)}>
            <option value="">{t('e7AllSections')}</option>
            {sections.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </label>}
      </div>
      <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{KINDS.map(k => <li key={k.kind}>
        <button type="button" disabled={busy} onClick={() => void start(`${k.kind}:${language}`, { ...base, kind: k.kind })}
          className="flex h-full w-full items-start gap-3 rounded-xl border border-slate-200 bg-white p-3 text-left transition hover:border-slate-300 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] disabled:cursor-not-allowed disabled:opacity-60">
          <span aria-hidden="true" className="ds-soft ds-ink grid h-9 w-9 shrink-0 place-items-center rounded-xl"><Icon name={k.icon} className="h-[18px] w-[18px]" /></span>
          <span className="min-w-0"><span className="block font-semibold text-slate-900">{t(`e7Kind_${k.kind}`)}</span><span className="block text-xs leading-5 text-slate-600">{t(`e7KindHint_${k.kind}`)}</span></span>
          <Icon name="download" className="ml-auto mt-1 h-4 w-4 shrink-0 text-slate-400" />
        </button>
      </li>)}</ul>
    </>}
    {!published && provisionalAllowed && <div className="flex flex-wrap items-end gap-4">
      <LanguagePicker value={language} onChange={setLanguage} name={`${uid}-plang`} />
      <button type="button" className={ui.secondary} disabled={busy} onClick={() => void start(`marksheets:${language}`, { ...base, kind: 'marksheets', provisional: true })}>
        <Icon name="eye" className="h-4 w-4" />{t('e7ProvisionalPreview')}
      </button>
    </div>}
    <RunStatus run={run} label={runLabel} />
  </section>
}

// MyMarksheet: a student's own published marksheet, in either language.
export function MyMarksheet({ versionId }: { versionId: string }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useDocumentError()
  const [busy, setBusy] = useState<DocumentLanguage | ''>('')
  const [error, setError] = useState('')
  async function get(language: DocumentLanguage) {
    setBusy(language); setError('')
    try {
      const blob = await authorized(token => downloadMyMarksheet(token, versionId, language))
      saveBlob(blob, `marksheet-${language}.pdf`)
    } catch (cause) { setError(explain(cause)) } finally { setBusy('') }
  }
  return <div className="space-y-2 print:hidden">
    <div className="flex flex-wrap gap-2">
      {(['en', 'bn'] as const).map(l => <button key={l} type="button" className={ui.secondary} disabled={!!busy} onClick={() => void get(l)} aria-busy={busy === l}>
        <Icon name="download" className="h-4 w-4" />{t(l === 'en' ? 'e7DownloadEnglish' : 'e7DownloadBangla')}
      </button>)}
    </div>
    {busy && <p role="status" className="text-sm text-slate-600">{t('e7Preparing')}</p>}
    {error && <Notice tone="error">{error}</Notice>}
  </div>
}

// TeacherDocuments lists the classes a teacher teaches that have published
// results, with their documents.
export function TeacherDocuments() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useDocumentError()
  const classes = useQuery({ queryKey: ['e7', 'classes'], queryFn: () => authorized(getDocumentClasses), retry: false })
  const [open, setOpen] = useState('')
  const keyOf = (c: DocumentClass) => `${c.scope}:${c.scope_id}:${c.class_id}`
  if (classes.isPending) return <Skeleton rows={2} />
  // Without exams:documents the section is simply not shown.
  if (classes.isError) return classes.error instanceof ApiError && classes.error.status === 403 ? null : <Notice tone="error">{explain(classes.error)}</Notice>
  return <section aria-labelledby="e7-teacher-title" className="space-y-3">
    <CardTitle id="e7-teacher-title" icon="report">{t('e7TeacherTitle')}</CardTitle>
    {classes.data.length === 0 ? <p className="text-sm text-slate-600">{t('e7TeacherNone')}</p> : <ul className="space-y-3">{classes.data.map(c => {
      const key = keyOf(c)
      return <li key={key} className="space-y-3">
        <button type="button" aria-expanded={open === key} onClick={() => setOpen(open === key ? '' : key)}
          className={`${ui.card} flex w-full items-center justify-between gap-3 p-4 text-left focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)]`}>
          <span className="min-w-0"><span className="block font-semibold text-slate-900">{c.class_name} · {c.scope === 'annual' ? t('e7AnnualOf', { session: c.session_name }) : c.title}</span>
            <span className="block text-sm text-slate-600">{t('e7ClassLine', { session: c.session_name, students: c.students, revision: c.revision })}</span></span>
          <Icon name="chevronDown" className={`h-4 w-4 shrink-0 text-slate-500 transition ${open === key ? 'rotate-180' : ''}`} />
        </button>
        {open === key && <ClassDocuments scope={c.scope} scopeId={c.scope_id} classId={c.class_id} published provisionalAllowed={false} />}
      </li>
    })}</ul>}
  </section>
}
