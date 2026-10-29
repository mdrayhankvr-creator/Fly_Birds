import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { getResultOverview, type ResultOverviewCell } from '../../api/client'
import { CardTitle, EmptyState, Notice, PageHeader, Skeleton, ui } from '../attendanceUi'
import { Progress } from '../marks/examUi'
import { usePeopleApi } from '../peopleShared'
import { SessionPicker, useSessionChoice } from './setupShared'
import { StateChip, useResultError, useWhen } from './resultUi'

// Result Publication Center (UX spec §10): where each class's term and
// annual results stand in a session, with the way into review, approval and
// publication. Exams are still published one by one on the exam page.
export function ResultsCentre() {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const explain = useResultError()
  const { sessions, items, chosen, choose } = useSessionChoice()
  const overview = useQuery({ queryKey: ['e6', 'overview', chosen], enabled: !!chosen, queryFn: () => authorized(token => getResultOverview(token, chosen)), retry: false })

  return <div className="space-y-5">
    <PageHeader eyebrow={t('m12Group_academics')} title={t('rvCentreTitle')} description={t('rvCentreIntro')}
      actions={<><Link to="/admin/exams" className={ui.secondary}>{t('rvBackToExams')}</Link><Link to={`/admin/exams/setup${chosen ? `?session=${chosen}` : ''}`} className={ui.secondary}>{t('bpSetupLink')}</Link></>} />
    {sessions.isPending ? <Skeleton rows={2} /> : items.length === 0 ? <EmptyState icon="grade" title={t('bpNoSessions')} /> : <SessionPicker items={items} value={chosen} onChange={choose} />}
    {chosen && (overview.isPending ? <Skeleton rows={6} /> : overview.isError ? <Notice tone="error">{explain(overview.error)}</Notice> : overview.data.terms.length === 0 ?
      <EmptyState icon="grade" title={t('rvNoTerms')}><Link to={`/admin/exams/setup?session=${chosen}`} className={ui.link}>{t('rvNoTermsHint')}</Link></EmptyState> : <>
        {overview.data.terms.map(term => <section key={term.id} className={`${ui.card} p-4 sm:p-5`} aria-labelledby={`rv-term-${term.id}`}>
          <h2 id={`rv-term-${term.id}`} className={ui.sectionTitle}>{term.name}</h2>
          {term.classes.length === 0 ? <p className="mt-2 text-sm text-slate-600">{t('rvTermNoExams')}</p> :
            <Cells cells={term.classes} link={c => `/admin/exams/results/review?scope=term&scope_id=${term.id}&class_id=${c.class_id}&session=${chosen}`} />}
        </section>)}
        <section className={`${ui.card} p-4 sm:p-5`} aria-labelledby="rv-annual-title">
          <CardTitle id="rv-annual-title" icon="calendar">{t('rvAnnualSection')}</CardTitle>
          <p className="mt-1 text-sm text-slate-600">{t('rvAnnualIntro')}</p>
          {overview.data.annual.length === 0 ? <p className="mt-2 text-sm text-slate-600">{t('rvNoAnnual')}</p> :
            <Cells cells={overview.data.annual} link={c => `/admin/exams/results/review?scope=annual&scope_id=${chosen}&class_id=${c.class_id}&session=${chosen}`} annual />}
        </section>
      </>)}
  </div>
}

function Cells({ cells, link, annual = false }: { cells: ResultOverviewCell[]; link: (c: ResultOverviewCell) => string; annual?: boolean }) {
  const { t } = useTranslation()
  const when = useWhen()
  return <ul className="mt-3 divide-y divide-slate-100 rounded-xl border border-slate-200">{cells.map(c => <li key={c.class_id} className="grid gap-2 p-3 sm:grid-cols-[minmax(0,17rem)_minmax(0,1fr)_auto] sm:items-center">
    <div className="min-w-0"><p className="font-semibold text-slate-900">{c.class_name}</p>
      {!annual && c.sheets > 0 && <Progress done={c.approved} total={c.sheets} label={t('rvSheetsApproved', { approved: c.approved, sheets: c.sheets })} />}
    </div>
    <div className="flex min-w-0 flex-col gap-1 text-sm">
      <span className="flex flex-wrap items-center gap-2"><StateChip state={c.state} />
        {c.published && <span className="text-slate-600">{t('rvRevisionShort', { revision: c.published.revision, when: when(c.published.published_at) })}</span>}</span>
      {c.approval && <span className="text-xs text-emerald-800">{t('rvApprovedBy', { name: c.approval.approved_by || '—', when: when(c.approval.approved_at) })}</span>}
      {c.corrections > 0 && <span className="text-xs font-semibold text-violet-900">{t('rvCorrectionsOpenShort', { n: c.corrections })}</span>}
      {annual && c.missing && c.missing.length > 0 && <span className="text-xs text-amber-900">{t('rvTermsMissing', { terms: c.missing.join(', ') })}</span>}
    </div>
    <div className="sm:text-right">{c.state !== 'no_exams' && <Link to={link(c)} className={`${ui.secondary} w-full sm:w-auto`}>{t(c.state === 'published' ? 'rvOpenResults' : 'rvReviewAction')}</Link>}</div>
  </li>)}</ul>
}
