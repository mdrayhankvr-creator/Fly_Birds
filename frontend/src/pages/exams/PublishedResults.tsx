import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getPublishedResults, type PublishedResult } from '../../api/client'
import { CardTitle, Notice, Skeleton } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { OutcomeChip, RevisedChip, useLineName, useWhen } from './resultUi'

// Student Results (UX spec §11): published term and annual results only,
// newest first; a revised result shows only its latest version and date. A
// withheld result shows the status message and no marks. Class-test
// contributions never reach this view (the server leaves them out).
export function PublishedResults({ childId }: { childId?: string }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const results = useQuery({ queryKey: ['e6', 'published', childId || 'me'], queryFn: () => authorized(token => getPublishedResults(token, childId)), retry: false })
  const [open, setOpen] = useState('')
  if (results.isPending) return <Skeleton rows={2} />
  if (results.isError) return <Notice tone="error">{t('rvErrGeneric')}</Notice>
  if (results.data.length === 0) return null
  const chosen = results.data.find(r => r.id === open)
  return <section aria-labelledby="rv-published-title" className="space-y-3 print:hidden">
    <CardTitle id="rv-published-title" icon="grade">{t('rvTermResultsTitle')}</CardTitle>
    <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{results.data.map(r => <li key={r.id}><ResultCardButton r={r} selected={open === r.id} onOpen={() => setOpen(open === r.id ? '' : r.id)} /></li>)}</ul>
    {chosen && <ResultDetail r={chosen} />}
  </section>
}

function heading(t: (k: string, o?: Record<string, unknown>) => string, r: PublishedResult) {
  return r.scope === 'annual' ? t('rvAnnualResultOf', { session: r.session_name }) : r.term_name
}

function method(t: (k: string, o?: Record<string, unknown>) => string, r: PublishedResult) {
  if (r.scope !== 'annual') return ''
  if (r.method === 'weighted') return t('rvMethod_weighted', { terms: r.method_terms.map(x => `${x.weight} % ${x.name}`).join(', ') })
  return t(`rvMethod_${r.method || 'final_only'}`)
}

function ResultCardButton({ r, selected, onOpen }: { r: PublishedResult; selected: boolean; onOpen: () => void }) {
  const { t } = useTranslation()
  const when = useWhen()
  return <button type="button" aria-pressed={selected} onClick={onOpen} className={`w-full rounded-2xl border bg-white p-5 text-left shadow-sm transition hover:border-slate-300 focus-visible:outline-2 focus-visible:outline-[var(--brand-primary)] ${selected ? 'border-[var(--brand-primary)] ring-2 ring-[color-mix(in_srgb,var(--brand-primary)_20%,transparent)]' : 'border-slate-200'}`}>
    <span className="flex items-start justify-between gap-2"><span className="font-semibold text-slate-900">{heading(t, r)}</span>{r.withheld ? <OutcomeChip o="withheld" /> : <OutcomeChip o={r.outcome} />}</span>
    <span className="mt-1 block text-sm text-slate-600">{r.session_name} · {r.class_name}</span>
    {r.scope === 'annual' && <span className="mt-1 block text-xs text-slate-600">{method(t, r)}</span>}
    {r.revised && <span className="mt-2 block"><RevisedChip when={when(r.published_at)} /></span>}
    <span className="mt-4 flex items-end justify-between gap-2">{r.withheld ? <span className="text-sm text-amber-900">{t('e3ResultWithheld')}</span> :
      <span><span className="block text-xs font-semibold uppercase tracking-wide text-slate-500">{t('rvGpa')}</span><span className="text-3xl font-bold tabular-nums text-slate-900" aria-label={t('rvGpaAria', { gpa: r.gpa })}>{r.gpa}</span></span>}
      <span className="text-sm font-semibold text-[var(--brand-ink)]">{t(selected ? 'rvHideResult' : 'rvViewResult')}</span></span>
  </button>
}

function ResultDetail({ r }: { r: PublishedResult }) {
  const { t, i18n } = useTranslation()
  const when = useWhen()
  const name = useLineName()
  const student = i18n.language === 'bn' && r.student.name_bn ? r.student.name_bn : r.student.name_en
  return <article className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-8" aria-labelledby="rv-detail-title">
    <header className="border-b border-slate-200 pb-4">
      <h3 id="rv-detail-title" className="text-lg font-semibold text-slate-900">{heading(t, r)}</h3>
      <p className="text-sm text-slate-600">{student} · {r.student.admission_no} · {r.class_name} / {r.section_name}</p>
      {r.scope === 'annual' && <p className="mt-1 text-sm text-slate-600">{method(t, r)}</p>}
    </header>
    {r.withheld ? <p role="status" className="mt-4 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 font-semibold text-amber-900">{t('e3ResultWithheld')}</p> : <>
      <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <div><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('rvGpa')}</dt><dd className="text-2xl font-bold tabular-nums text-slate-900">{r.gpa}</dd></div>
        <div><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('rvOutcome')}</dt><dd className="mt-1"><OutcomeChip o={r.outcome} /></dd></div>
        <div><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('rvTotal')}</dt><dd className="tabular-nums text-slate-900">{r.total} / {r.full}</dd></div>
        <div><dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{t('rvPositions')}</dt><dd className="tabular-nums text-slate-900">{r.class_position ?? '—'} · {r.section_position ?? '—'}</dd></div>
      </dl>
      <div className="mt-5 overflow-x-auto"><table className="ds-cards w-full min-w-[26rem] border-collapse text-left text-sm">
        <caption className="sr-only">{t('rvSubjectsOf', { name: heading(t, r) })}</caption>
        <thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th scope="col" className="px-3 py-2.5">{t('rvSubject')}</th><th scope="col" className="px-3 py-2.5 text-right">{t('rvMarks')}</th><th scope="col" className="px-3 py-2.5">{t('rvGrade')}</th><th scope="col" className="px-3 py-2.5 text-right">{t('rvPoint')}</th></tr></thead>
        <tbody>{r.subjects.map(l => <tr key={l.key} className="border-b border-slate-100 align-top">
          <th scope="row" className="px-3 py-2.5 font-medium text-slate-900">{name(l)}{l.optional && <span className="ml-1 text-xs font-normal text-slate-500">({t('rvOptionalShort')})</span>}
            {l.reasons.map(code => <span key={code} className="block text-xs font-semibold text-rose-700">{t(`rvReason_${code.split(':')[0]}`, { component: code.split(':')[1] || '' })}</span>)}
            {l.terms && l.terms.length > 0 && <span className="block text-xs font-normal text-slate-500">{l.terms.map(x => `${x.term_name}: ${x.status === 'graded' ? `${x.marks}/${x.full}` : t(`rvLine_${x.status}`)}`).join(' · ')}</span>}</th>
          <td className="px-3 py-2.5 text-right tabular-nums">{l.status === 'graded' ? `${l.marks} / ${l.full}` : <span className="font-semibold text-slate-700">{t(`rvLine_${l.status}`)}</span>}</td>
          <td className={`px-3 py-2.5 ${l.passed === false ? 'font-semibold text-rose-700' : ''}`}>{l.grade ?? '—'}</td>
          <td className="px-3 py-2.5 text-right tabular-nums">{l.point ?? '—'}</td>
        </tr>)}</tbody>
      </table></div>
    </>}
    <footer className="mt-5 text-xs text-slate-600">{t('rvPublishedFooter', { when: when(r.published_at), version: r.version })}{r.revised ? ` · ${t('rvRevised')}` : ''}</footer>
  </article>
}
