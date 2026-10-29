import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ApiError, getResultHistory, getResultVersion, type ResultLine, type ResultOutputs, type ResultReview, type ResultScope, type ResultVersionView } from '../../api/client'
import { CardTitle, Notice, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'

// Shared pieces of the E6 result screens, in the "calm ledger" style of the
// E1 UX spec: states are words + colour, numbers are tabular and right
// aligned, provisional output is framed and never looks final.

/** A date and time in the reader's language, digits kept Latin. */
export function useWhen() {
  const { i18n } = useTranslation()
  return (iso?: string | null) => {
    if (!iso) return '—'
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleString(i18n.language === 'bn' ? 'bn-BD-u-nu-latn' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' })
  }
}

/** A sentence for an API failure, by its code. */
export function useResultError() {
  const { t, i18n } = useTranslation()
  return (cause: unknown): string => {
    if (cause instanceof ApiError && cause.code) {
      for (const prefix of ['rvErr_', 'e3Code_', 'bpErr_']) if (i18n.exists(`${prefix}${cause.code}`)) return t(`${prefix}${cause.code}`)
    }
    return t('rvErrGeneric')
  }
}

/** A subject line's name in the reader's language. */
export function useLineName() {
  const { i18n } = useTranslation()
  return (l: { name_en: string; name_bn: string } | undefined) => !l ? '' : i18n.language === 'bn' && l.name_bn ? l.name_bn : l.name_en
}

const stateTones: Record<ResultReview['state'] | 'no_exams' | 'sheets_pending' | 'terms_pending' | 'no_blueprint', string> = {
  blocked: 'bg-amber-50 text-amber-900 ring-1 ring-inset ring-amber-600/30',
  sheets_pending: 'bg-amber-50 text-amber-900 ring-1 ring-inset ring-amber-600/30',
  terms_pending: 'bg-amber-50 text-amber-900 ring-1 ring-inset ring-amber-600/30',
  no_exams: 'border border-dashed border-slate-300 bg-white text-slate-700',
  no_blueprint: 'border border-dashed border-slate-300 bg-white text-slate-700',
  ready: 'bg-sky-50 text-sky-800 ring-1 ring-inset ring-sky-600/25',
  approved: 'bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-600/25',
  changes: 'border border-violet-300 bg-white text-violet-900',
  published: 'bg-[var(--brand-primary)] text-[var(--brand-on)]',
}

export function StateChip({ state }: { state: keyof typeof stateTones }) {
  const { t } = useTranslation()
  return <span className={`inline-flex shrink-0 items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${stateTones[state]}`}>{t(`rvState_${state}`)}</span>
}

const versionTones: Record<ResultVersionView['status'], string> = {
  published: 'bg-[var(--brand-primary)] text-[var(--brand-on)]',
  superseded: 'bg-slate-100 text-slate-700 ring-1 ring-inset ring-slate-500/20',
  withdrawn: 'border border-rose-300 bg-white text-rose-800',
}

export function VersionStatusChip({ status }: { status: ResultVersionView['status'] }) {
  const { t } = useTranslation()
  return <span className={`inline-flex shrink-0 items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${versionTones[status]}`}>{t(`rvVersion_${status}`)}</span>
}

export function RevisedChip({ when }: { when?: string }) {
  const { t } = useTranslation()
  return <span className="inline-flex shrink-0 items-center rounded-full border border-violet-300 bg-white px-2.5 py-0.5 text-xs font-semibold text-violet-900">{when ? t('rvRevisedOn', { when }) : t('rvRevised')}</span>
}

const outcomeTones: Record<string, string> = {
  passed: 'bg-emerald-50 text-emerald-800 ring-emerald-600/20',
  failed: 'bg-rose-50 text-rose-800 ring-rose-600/20',
  incomplete: 'bg-amber-50 text-amber-900 ring-amber-600/25',
  withheld: 'bg-slate-100 text-slate-700 ring-slate-500/20',
}

export function OutcomeChip({ o }: { o: string }) {
  const { t } = useTranslation()
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset ${outcomeTones[o] || outcomeTones.withheld}`}>{t(`rvOutcome_${o}`)}</span>
}

/** Provisional output: dashed frame and a label (UX spec §1.6). */
export function ProvisionalFrame({ id, label, children }: { id: string; label: string; children: ReactNode }) {
  return <section aria-labelledby={id} className="relative rounded-2xl border-2 border-dashed border-indigo-300 bg-white p-3 sm:p-4">
    <p id={id} className="mb-3 inline-flex items-center rounded-full bg-indigo-50 px-3 py-1 text-xs font-bold uppercase tracking-wide text-indigo-800 ring-1 ring-inset ring-indigo-600/20">{label}</p>
    {children}
  </section>
}

/** One subject cell: marks and grade, or the line's status. */
export function LineCell({ l }: { l?: ResultLine }) {
  const { t } = useTranslation()
  if (!l) return <span className="text-slate-500">—</span>
  if (l.status !== 'graded' && l.status !== 'present') return <span className="font-semibold text-slate-600">{t(`rvLine_${l.status}`)}</span>
  const failed = l.passed === false
  return <span className={failed ? 'font-semibold text-rose-700' : 'text-slate-900'}>{l.marks ?? '—'} <span className="text-xs font-semibold">{l.grade ?? ''}</span>{l.optional && <span className="ml-1 text-xs text-slate-500">({t('rvOptionalShort')})</span>}</span>
}

/** Why a subject failed whatever its total. */
export function reasonText(t: (k: string, o?: Record<string, unknown>) => string, code: string, component?: string) {
  return t(`rvReason_${code}`, { component: component || '' })
}

export type MatrixRow = { id: string; name: string; meta: string; outputs: ResultOutputs; highlight?: boolean; blocker?: string; action?: ReactNode }

/**
 * A tabulation of results: student column pinned, a column per subject line,
 * then GPA, outcome and positions. Phones get one card per student.
 */
export function ResultMatrix({ subjects, rows, label }: { subjects: { key: string; name_en: string; name_bn: string }[]; rows: MatrixRow[]; label: string }) {
  const { t } = useTranslation()
  const name = useLineName()
  const [open, setOpen] = useState('')
  const cell = (o: ResultOutputs, key: string) => o.subjects.find(s => s.key === key)
  return <>
    <div className="hidden max-h-[70vh] overflow-auto md:block" tabIndex={0} role="region" aria-label={label}>
      <table className="w-full border-separate border-spacing-0 text-left text-sm">
        <thead><tr>
          <th scope="col" className={`${ui.th} sticky left-0 top-0 z-20`}>{t('rvStudent')}</th>
          {subjects.map(s => <th key={s.key} scope="col" className={`${ui.th} sticky top-0 z-10 text-right`}>{name(s)}</th>)}
          <th scope="col" className={`${ui.th} sticky top-0 z-10 text-right`}>{t('rvGpa')}</th>
          <th scope="col" className={`${ui.th} sticky top-0 z-10`}>{t('rvOutcome')}</th>
          <th scope="col" className={`${ui.th} sticky top-0 z-10 text-right`}>{t('rvPositions')}</th>
          <th scope="col" className={`${ui.th} sticky top-0 z-10`}><span className="sr-only">{t('rvActions')}</span></th>
        </tr></thead>
        <tbody>{rows.map(r => {
          const bg = r.highlight ? 'bg-violet-50' : 'bg-white'
          return <tr key={r.id} className="group" data-changed={r.highlight ? 'true' : undefined}>
            <th scope="row" className={`sticky left-0 z-[1] min-w-[12rem] border-b border-slate-100 px-3 py-2 text-left font-medium ${bg} group-hover:bg-slate-50`}>{r.name}<span className="block text-xs font-normal text-slate-500">{r.meta}</span>{r.highlight && <span className="mt-0.5 block text-xs font-semibold text-violet-900">{t('rvChanged')}</span>}</th>
            {subjects.map(s => <td key={s.key} className={`whitespace-nowrap border-b border-slate-100 px-3 py-2 text-right tabular-nums ${bg} group-hover:bg-slate-50`}><LineCell l={cell(r.outputs, s.key)} /></td>)}
            <td className={`border-b border-slate-100 px-3 py-2 text-right font-semibold tabular-nums ${bg} group-hover:bg-slate-50`}>{r.outputs.gpa ?? '—'}</td>
            <td className={`border-b border-slate-100 px-3 py-2 ${bg} group-hover:bg-slate-50`}>{r.blocker ? <OutcomeChip o="incomplete" /> : <OutcomeChip o={r.outputs.outcome} />}</td>
            <td className={`border-b border-slate-100 px-3 py-2 text-right tabular-nums text-slate-600 ${bg} group-hover:bg-slate-50`}>{r.outputs.class_position ? `${r.outputs.class_position} · ${r.outputs.section_position ?? '—'}` : '—'}</td>
            <td className={`border-b border-slate-100 px-3 py-2 ${bg} group-hover:bg-slate-50`}>{r.action}</td>
          </tr>
        })}</tbody>
      </table>
    </div>
    <ul className="space-y-3 md:hidden">{rows.map(r => <li key={r.id} className={`rounded-xl border p-3 ${r.highlight ? 'border-violet-300' : 'border-slate-200'}`}>
      <button type="button" aria-expanded={open === r.id} onClick={() => setOpen(open === r.id ? '' : r.id)} className="flex min-h-11 w-full items-start justify-between gap-2 text-left">
        <span className="min-w-0"><span className="block font-semibold text-slate-900">{r.name}</span><span className="block text-xs text-slate-500">{r.meta}</span>{r.highlight && <span className="block text-xs font-semibold text-violet-900">{t('rvChanged')}</span>}</span>
        <span className="text-right"><span className="block text-lg font-bold tabular-nums text-slate-900">{r.outputs.gpa ?? '—'}</span>{r.blocker ? <OutcomeChip o="incomplete" /> : <OutcomeChip o={r.outputs.outcome} />}</span>
      </button>
      {open === r.id && <>
        <dl className="mt-3 divide-y divide-slate-100 text-sm">{subjects.map(s => <div key={s.key} className="flex justify-between gap-2 py-1.5"><dt className="text-slate-600">{name(s)}</dt><dd className="tabular-nums"><LineCell l={cell(r.outputs, s.key)} /></dd></div>)}
          <div className="flex justify-between gap-2 py-1.5"><dt className="text-slate-600">{t('rvPositions')}</dt><dd className="tabular-nums">{r.outputs.class_position ? `${r.outputs.class_position} · ${r.outputs.section_position ?? '—'}` : '—'}</dd></div>
        </dl>
        {r.action && <div className="mt-2">{r.action}</div>}
      </>}
    </li>)}</ul>
  </>
}

type Change = { field: string; before: string; after: string }

/** What differs between two outputs, as labelled before → after pairs. */
export function useOutputChanges() {
  const { t } = useTranslation()
  const name = useLineName()
  return (before: ResultOutputs | undefined, after: ResultOutputs): Change[] => {
    const out: Change[] = []
    const v = (x: unknown) => x === null || x === undefined || x === '' ? '—' : String(x)
    if (!before) return out
    if (before.outcome !== after.outcome) out.push({ field: t('rvOutcome'), before: t(`rvOutcome_${before.outcome}`), after: t(`rvOutcome_${after.outcome}`) })
    if (before.gpa !== after.gpa) out.push({ field: t('rvGpa'), before: v(before.gpa), after: v(after.gpa) })
    if (before.total !== after.total) out.push({ field: t('rvTotal'), before: `${v(before.total)} / ${v(before.full)}`, after: `${v(after.total)} / ${v(after.full)}` })
    if (before.class_position !== after.class_position) out.push({ field: t('rvClassPosition'), before: v(before.class_position), after: v(after.class_position) })
    if (before.section_position !== after.section_position) out.push({ field: t('rvSectionPosition'), before: v(before.section_position), after: v(after.section_position) })
    const keys = Array.from(new Set([...before.subjects.map(s => s.key), ...after.subjects.map(s => s.key)]))
    for (const key of keys) {
      const a = before.subjects.find(s => s.key === key), b = after.subjects.find(s => s.key === key)
      const show = (l?: ResultLine) => !l ? t('rvNotTaken') : l.status !== 'graded' && l.status !== 'present' ? t(`rvLine_${l.status}`) : `${v(l.marks)} · ${v(l.grade)}`
      if (show(a) !== show(b)) out.push({ field: name(b || a), before: show(a), after: show(b) })
    }
    return out
  }
}

/** Before → after for one student's changed result (DiffTable, UX §3). */
export function DiffTable({ rows, caption }: { rows: Change[]; caption: string }) {
  const { t } = useTranslation()
  if (rows.length === 0) return <p className="text-sm text-slate-600">{t('rvNoValueChange')}</p>
  return <div className="overflow-x-auto"><table className="w-full min-w-[22rem] text-left text-sm">
    <caption className="sr-only">{caption}</caption>
    <thead><tr className="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500"><th scope="col" className="px-3 py-2">{t('rvField')}</th><th scope="col" className="px-3 py-2 text-right">{t('rvBefore')}</th><th scope="col" className="px-3 py-2 text-right">{t('rvAfter')}</th></tr></thead>
    <tbody>{rows.map(r => <tr key={r.field} className="border-b border-slate-100"><th scope="row" className="px-3 py-2 font-medium text-slate-800">{r.field}</th><td className="px-3 py-2 text-right tabular-nums text-slate-600 line-through decoration-slate-400">{r.before}</td><td className="px-3 py-2 text-right font-semibold tabular-nums text-slate-900">{r.after}</td></tr>)}</tbody>
  </table></div>
}

/**
 * Every version of a student's result: published, superseded and withdrawn,
 * each openable with its snapshot check.
 */
export function VersionHistory({ scope, scopeId, studentId, studentName, onClose }: { scope: ResultScope; scopeId: string; studentId: string; studentName: string; onClose: () => void }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const when = useWhen()
  const [openId, setOpenId] = useState('')
  const history = useQuery({ queryKey: ['e6', 'history', scope, scopeId, studentId], queryFn: () => authorized(token => getResultHistory(token, scope, scopeId, studentId)), retry: false })
  return <section aria-labelledby="rv-history-title" className={`${ui.card} p-4 sm:p-5`}>
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div><CardTitle id="rv-history-title" icon="clock">{t('rvHistoryTitle', { name: studentName })}</CardTitle><p className="text-sm text-slate-600">{t('rvHistoryIntro')}</p></div>
      <button type="button" className={ui.ghost} onClick={onClose}>{t('rvClose')}</button>
    </div>
    {history.isPending ? <div className="mt-3"><Skeleton rows={3} /></div> : history.isError ? <div className="mt-3"><Notice tone="error">{t('rvErrGeneric')}</Notice></div> : history.data.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('rvHistoryEmpty')}</p> :
      <ol className="mt-3 space-y-2">{history.data.map(v => <li key={v.id} className="rounded-xl border border-slate-200 p-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold tabular-nums text-slate-900">{t('rvVersionN', { n: v.version })}</span><VersionStatusChip status={v.status} />
          <span className="text-xs text-slate-600">{t(`rvKind_${v.kind}`)}{v.source === 'migration' ? ` · ${t('rvFromUpgrade')}` : ''}</span>
        </div>
        <p className="mt-1 text-sm text-slate-700"><OutcomeChip o={v.outcome} /> <span className="ml-1 tabular-nums">{t('rvGpa')} {v.gpa ?? '—'} · {t('rvTotal')} {v.total ?? '—'} / {v.full ?? '—'} · {t('rvPositions')} {v.class_position ?? '—'} · {v.section_position ?? '—'}</span></p>
        <p className="mt-1 text-xs text-slate-600">{t('rvPublishedBy', { name: v.published_by || '—', when: when(v.published_at) })}{v.reason ? ` · ${t('rvBecause', { reason: v.reason })}` : ''}</p>
        {v.closed_at && <p className="mt-0.5 text-xs text-slate-600">{t(v.status === 'withdrawn' ? 'rvWithdrawnBy' : 'rvSupersededBy', { name: v.closed_by || '—', when: when(v.closed_at) })}{v.close_reason ? ` · ${t('rvBecause', { reason: v.close_reason })}` : ''}</p>}
        <button type="button" className={`${ui.link} mt-1 text-sm`} aria-expanded={openId === v.id} onClick={() => setOpenId(openId === v.id ? '' : v.id)}>{t(openId === v.id ? 'rvHideSnapshot' : 'rvShowSnapshot')}</button>
        {openId === v.id && <VersionDetail id={v.id} />}
      </li>)}</ol>}
  </section>
}

function VersionDetail({ id }: { id: string }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const detail = useQuery({ queryKey: ['e6', 'version', id], queryFn: () => authorized(token => getResultVersion(token, id)), retry: false })
  if (detail.isPending) return <div className="mt-2"><Skeleton rows={2} /></div>
  if (detail.isError) return <div className="mt-2"><Notice tone="error">{t('rvErrGeneric')}</Notice></div>
  const d = detail.data
  const ok = d.verification.hash_matches && d.verification.reproduced
  return <div className="mt-2 space-y-2 rounded-lg bg-slate-50 p-3 text-sm">
    <p role="status" className={ok ? 'font-semibold text-emerald-800' : 'font-semibold text-amber-900'}>{ok ? t('rvVerified') : t('rvNotReproduced', { fields: d.verification.differences.join(', ') })}</p>
    {d.verification.note && <p className="text-xs text-slate-600">{d.verification.note}</p>}
    <dl className="grid gap-1 text-xs text-slate-600 sm:grid-cols-[auto_minmax(0,1fr)] sm:gap-x-3">
      <dt>{t('rvInputsHash')}</dt><dd className="break-all font-mono">{d.version.inputs_sha256}</dd>
      <dt>{t('rvRulesHash')}</dt><dd className="break-all font-mono">{d.rules_sha256}</dd>
      <dt>{t('rvBands')}</dt><dd>{t(d.rules.bands_source === 'blueprint_version' ? 'rvBandsVersion' : 'rvBandsSchool')}</dd>
    </dl>
    <SnapshotLines outputs={d.snapshot.outputs} />
  </div>
}

/** The subject lines of a stored snapshot. */
export function SnapshotLines({ outputs }: { outputs: ResultOutputs }) {
  const { t } = useTranslation()
  const name = useLineName()
  if (outputs.outcome === 'withheld') return <p className="text-sm font-semibold text-slate-700">{t('rvOutcome_withheld')}</p>
  return <div className="overflow-x-auto"><table className="w-full min-w-[20rem] text-left text-sm">
    <thead><tr className="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500"><th scope="col" className="px-2 py-1.5">{t('rvSubject')}</th><th scope="col" className="px-2 py-1.5 text-right">{t('rvMarks')}</th><th scope="col" className="px-2 py-1.5">{t('rvGrade')}</th><th scope="col" className="px-2 py-1.5 text-right">{t('rvPoint')}</th></tr></thead>
    <tbody>{outputs.subjects.map(l => <tr key={l.key + (l.sheet_id || '')} className="border-b border-slate-100">
      <th scope="row" className="px-2 py-1.5 font-medium">{name(l)}{l.components.length > 1 && <span className="block text-xs font-normal text-slate-500">{l.components.map(c => `${c.key} ${c.marks ?? '—'}/${c.full}`).join(' · ')}</span>}{l.reasons.map((r, i) => <span key={i} className="block text-xs font-semibold text-rose-700">{reasonText(t, r.code, r.component)}</span>)}</th>
      <td className="px-2 py-1.5 text-right tabular-nums">{l.status === 'graded' || l.status === 'present' ? `${l.marks ?? '—'} / ${l.full}` : t(`rvLine_${l.status}`)}</td>
      <td className="px-2 py-1.5">{l.grade ?? '—'}</td>
      <td className="px-2 py-1.5 text-right tabular-nums">{l.point ?? '—'}</td>
    </tr>)}</tbody>
  </table></div>
}
