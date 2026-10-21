import { memo, useCallback, useEffect, useMemo, useRef, useState, type ClipboardEvent, type FormEvent, type KeyboardEvent, type MutableRefObject } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ApiError, examSheetAction, getExamSheetHistory, getExamSheetWorkspace, saveExamMarkCells, type ExamMarkRow, type ExamMarkStatus, type ExamSheetComponent, type ExamSheetHistoryEntry, type ExamSheetPermissions } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { confirmAction } from '../../components/ConfirmDialog'
import { Notice, Skeleton, ui } from '../attendanceUi'
import { usePeopleApi } from '../peopleShared'
import { Progress, SheetChip } from './examUi'
import { isMultiCell, parseCell, parseTable, planPaste, type PastePlan } from './marksInput'
import { marksDraftKey, useSheetAutosave, type CellState, type CellValue, type ServerCell } from './useSheetAutosave'

type RowFilter = 'all' | 'missing' | 'attention'
const letterStatus: Record<string, ExamMarkStatus> = { a: 'absent', i: 'incomplete', p: 'present' }
const clock = (ms: number) => new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit' }).format(ms)
const carries = (status: string) => status === 'present' || status === 'withheld' || status === ''

// useWide renders exactly one layout (grid from 640 px, list below), so a
// save never re-renders a hidden copy of every row.
function useWide() {
  const query = '(min-width: 640px)'
  const [wide, setWide] = useState(() => window.matchMedia(query).matches)
  useEffect(() => {
    const media = window.matchMedia(query)
    const change = () => setWide(media.matches)
    media.addEventListener('change', change)
    return () => media.removeEventListener('change', change)
  }, [])
  return wide
}

// MarksWorkspace is one mark sheet: context, autosaving grid, paste, review
// actions and history. scope "teacher" uses the teacher's own routes; "staff"
// the exams:read routes (head teacher and administrator).
export function MarksWorkspace({ scope }: { scope: 'teacher' | 'staff' }) {
  const { examId = '', sheetId = '' } = useParams()
  const teacher = scope === 'teacher'
  const { t, i18n } = useTranslation()
  const { user } = useAuth()
  const authorized = usePeopleApi()
  const client = useQueryClient()
  const ws = useQuery({ queryKey: ['marks', 'sheet', scope, examId, sheetId], gcTime: 0, queryFn: () => authorized(token => getExamSheetWorkspace(token, examId, sheetId, teacher)), retry: false })
  const data = ws.data
  const perms = data?.permissions
  const full = data?.sheet.full_marks ?? 100
  // A component sheet (CQ, MCQ, practical …): one input per component (E5).
  const components = useMemo(() => data?.components ?? [], [data?.components])
  const parts = useMemo(() => components.map(c => ({ key: c.key, full: c.full })), [components])
  const reload = useCallback(() => client.invalidateQueries({ queryKey: ['marks'] }), [client])
  const save = useCallback((entries: Parameters<typeof saveExamMarkCells>[2]) => authorized(token => saveExamMarkCells(token, examId, entries)), [authorized, examId])
  const autosave = useSheetAutosave({ sheetId, rows: data?.rows, fullMarks: full, enabled: !!perms?.can_edit, storageKey: user ? marksDraftKey(user.id, sheetId) : null, save, onBlocked: () => void reload(), components: parts })

  const [search, setSearch] = useState(''), [filter, setFilter] = useState<RowFilter>('all')
  const [single, setSingle] = useState(false), [cursor, setCursor] = useState(0)
  const [historyOpen, setHistoryOpen] = useState(false), [shortcuts, setShortcuts] = useState(false)
  const [paste, setPaste] = useState<{ plan: PastePlan | null; error: string; box: boolean; text: string } | null>(null)
  const [pasteReport, setPasteReport] = useState(false)
  const [busy, setBusy] = useState(false), [message, setMessage] = useState(''), [problem, setProblem] = useState('')
  const [returning, setReturning] = useState(false), [returnReason, setReturnReason] = useState('')
  const inputs = useRef(new Map<string, HTMLInputElement>())
  const wide = useWide()

  const rows = data?.rows ?? []
  const name = (row: ExamMarkRow) => row.student_name
  const editableRow = useCallback((row: ExamMarkRow) => {
    if (!perms?.can_edit) return false
    const status = autosave.serverCell(row.student_id)?.status
    return !((status === 'exempt' || status === 'withheld') && !perms.statuses.includes('exempt'))
  }, [perms, autosave.serverCell])
  const autosaveRef = useRef(autosave)
  autosaveRef.current = autosave
  const isMissing = (row: ExamMarkRow) => { const v = autosave.value(row.student_id); return v.status === '' || (v.status === 'present' && v.text.trim() === '') }
  const needsAttention = (row: ExamMarkRow) => { const c = autosave.cell(row.student_id); return c?.kind === 'error' || c?.kind === 'conflict' || autosave.value(row.student_id).status === 'incomplete' }
  const query = search.trim().toLowerCase()
  const visible = rows.filter(row => (!query || name(row).toLowerCase().includes(query) || row.admission_no.toLowerCase().includes(query) || String(row.roll_no ?? '') === query)
    && (filter === 'all' || (filter === 'missing' ? isMissing(row) : needsAttention(row))))
  const filtered = query !== '' || filter !== 'all'
  const missing = rows.filter(isMissing).length
  const counts = { absent: 0, incomplete: 0, exempt: 0, withheld: 0 }
  for (const row of rows) { const s = autosave.value(row.student_id).status; if (s in counts) counts[s as keyof typeof counts]++ }

  const focusRow = useCallback((index: number, part?: string) => {
    const row = visibleRef.current[Math.max(0, Math.min(index, visibleRef.current.length - 1))]
    if (!row) return
    const el = inputs.current.get(part ? `${row.student_id}|${part}` : row.student_id)
    if (el && !el.disabled) { el.focus(); el.select() }
    setCursor(Math.max(0, Math.min(index, visibleRef.current.length - 1)))
  }, [])
  const visibleRef = useRef(visible)
  visibleRef.current = visible

  const errText = useCallback((code: string) => i18n.exists(`e3Err_${code}`) ? t(`e3Err_${code}`) : i18n.exists(`mwErr_${code}`) ? t(`mwErr_${code}`) : i18n.exists(`mwPasteReason_${code}`) ? t(`mwPasteReason_${code}`) : code, [i18n, t])

  // ---- paste ---------------------------------------------------------------
  const openPaste = useCallback((text: string, start: number) => {
    const table = parseTable(text)
    const all = rows.map(r => ({ student_id: r.student_id, admission_no: r.admission_no }))
    const plan = planPaste(table, all, start, full, parts)
    if (plan.mode === 'by_position' && filtered) { setPaste({ plan: null, error: t('mwPasteFiltered'), box: false, text: '' }); return }
    for (const item of plan.items) {
      const row = rows.find(r => r.student_id === item.student_id)
      if (item.result === 'apply' && row && !editableRow(row)) { item.result = 'reject'; item.reason = 'locked_by_admin' }
    }
    setPaste({ plan, error: '', box: false, text: '' })
  }, [rows, full, filtered, t, editableRow, parts])
  function applyPaste() {
    if (!paste?.plan) return
    for (const item of paste.plan.items) {
      if (item.result !== 'apply' || !item.student_id) continue
      if (item.parsed.kind === 'marks') autosave.edit(item.student_id, { status: 'present', text: String(item.parsed.value) })
      else if (item.parsed.kind === 'components') autosave.edit(item.student_id, { status: 'present', parts: Object.fromEntries(Object.entries(item.parsed.values).map(([k, v]) => [k, String(v)])) })
      else if (item.parsed.kind === 'status') autosave.edit(item.student_id, { status: item.parsed.status })
    }
    setPaste(null)
    setPasteReport(true)
    autosave.clearReport()
    autosave.flushNow()
  }
  const onCellPaste = useCallback((event: ClipboardEvent<HTMLInputElement>, index: number) => {
    const text = event.clipboardData.getData('text/plain')
    if (!isMultiCell(text)) return
    event.preventDefault()
    openPasteRef.current(text, filteredRef.current ? 0 : rowsRef.current.findIndex(r => r.student_id === visibleRef.current[index]?.student_id))
  }, [])
  const openPasteRef = useRef(openPaste)
  openPasteRef.current = openPaste
  const filteredRef = useRef(filtered)
  filteredRef.current = filtered
  const rowsRef = useRef(rows)
  rowsRef.current = rows

  // ---- keyboard ------------------------------------------------------------
  const onMarksKey = useCallback((event: KeyboardEvent<HTMLInputElement>, index: number, row: ExamMarkRow, part?: string) => {
    const id = row.student_id
    if (event.key === 'Enter' || event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      // Down the same column: a component sheet keeps the component.
      focusRow(index + (event.key === 'ArrowUp' || (event.key === 'Enter' && event.shiftKey) ? -1 : 1), part)
      return
    }
    if (event.key === 'Escape') { event.preventDefault(); autosaveRef.current.revert(id); return }
    if (event.key === '?') { event.preventDefault(); setShortcuts(v => !v); return }
    const status = letterStatus[event.key.toLowerCase()]
    if (status && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault()
      if (permsRef.current?.statuses.includes(status)) autosaveRef.current.edit(id, { status })
    }
  }, [focusRow])
  const permsRef = useRef(perms)
  permsRef.current = perms
  const ctx = useMemo<RowCtx>(() => ({ edit: autosave.edit, resolve: autosave.resolve, inputs, errText, onMarksKey, onCellPaste, t, components }),
    [autosave.edit, autosave.resolve, errText, onMarksKey, onCellPaste, t, components])

  // Reviewers and teachers: submit, approve, return.
  async function act(action: 'submit' | 'approve' | 'return', reason?: string) {
    if (busy || !data) return
    setBusy(true); setProblem(''); setMessage('')
    try {
      await authorized(token => examSheetAction(token, examId, sheetId, action, reason))
      setMessage(t(action === 'submit' ? 'mwSubmitted' : action === 'approve' ? 'mwApproved' : 'mwReturnedDone'))
      setReturning(false); setReturnReason('')
      await reload()
    } catch (cause) {
      setProblem(cause instanceof ApiError && i18n.exists(`e3Code_${cause.code}`) ? t(`e3Code_${cause.code}`) : t('examError'))
      await reload()
    } finally { setBusy(false) }
  }

  // "?" toggles the shortcut list from anywhere outside a text field.
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.key === '?' && !(event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement)) setShortcuts(v => !v)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  useEffect(() => {
    if (pasteReport && autosave.report && autosave.state !== 'saving') {
      setMessage(t('mwPasteReport', { count: autosave.report.saved }) + (autosave.report.failed.length ? ' ' + t('mwPasteReportFailed', { count: autosave.report.failed.length }) : ''))
      setPasteReport(false)
    }
  }, [pasteReport, autosave.report, autosave.state, t])

  if (ws.isPending) return <div className="space-y-4" aria-busy="true"><p className="sr-only" role="status">{t('mwLoading')}</p><Skeleton rows={2} /><Skeleton rows={8} /></div>
  if (ws.isError || !data) {
    const status = ws.error instanceof ApiError ? ws.error.status : 0
    return <div className="space-y-4"><BackLink teacher={teacher} examId={examId} />
      <Notice tone="error">{status === 403 || status === 404 ? t('e3Err_not_assigned') : t('mwLoadError')}</Notice>
      <button type="button" className={ui.secondary} onClick={() => void ws.refetch()}>{t('mwRetry')}</button></div>
  }

  const subject = i18n.language === 'bn' && data.sheet.name_bn ? data.sheet.name_bn : data.sheet.name_en
  const why = !perms?.can_submit ? '' : missing > 0 ? t('mwWhy_missing', { count: missing }) : autosave.pending > 0 || autosave.state === 'saving' ? t('mwWhy_pending') : autosave.conflicts > 0 ? t('mwWhy_conflicts') : autosave.errors > 0 ? t('mwWhy_errors') : ''
  const entered = rows.length - missing
  const readOnly = perms?.read_only_reason

  return <div className="space-y-4">
    <BackLink teacher={teacher} examId={examId} />

    {/* Context: what this sheet is, how far it is, and what can be done. */}
    <header className={`${ui.card} p-4 sm:p-5`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-bold tracking-tight text-slate-900 sm:text-2xl">{subject}</h1>
            {/* A save moves a not-started sheet to in progress on the server. */}
            <SheetChip status={data.sheet.status === 'not_started' && autosave.savedAt ? 'in_progress' : data.sheet.status} />
          </div>
          <p className="mt-1 text-sm text-slate-600">{data.exam.name} · {data.exam.session_name} · {data.exam.class_name} {data.exam.section_name}</p>
          <p className="mt-0.5 text-sm font-medium tabular-nums text-slate-700">{t('mwFullMarks', { full: data.sheet.full_marks, pass: data.sheet.pass_marks })}</p>
        </div>
        <SaveIndicator autosave={autosave} canEdit={!!perms?.can_edit} />
      </div>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 pt-4">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5">
          <Progress done={entered} total={rows.length} label={t('mwProgress', { entered, students: rows.length })} />
          {counts.absent > 0 && <span className="text-xs text-slate-600">{t('mwAbsentN', { n: counts.absent })}</span>}
          {counts.incomplete > 0 && <span className="text-xs font-semibold text-amber-800">{t('mwIncompleteN', { n: counts.incomplete })}</span>}
          {counts.exempt > 0 && <span className="text-xs text-slate-600">{t('mwExemptN', { n: counts.exempt })}</span>}
          {counts.withheld > 0 && <span className="text-xs text-slate-600">{t('mwWithheldN', { n: counts.withheld })}</span>}
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          {perms?.can_submit && <div className="flex flex-col items-end gap-1">
            <button type="button" className={ui.primary} disabled={busy || !!why} aria-describedby={why ? 'submit-why' : undefined}
              onClick={() => void (async () => { if (await confirmAction(t('mwSubmitConfirm', { subject }), { tone: 'default', confirmLabel: t('mwSubmit') })) await act('submit') })()}>{t('mwSubmit')}</button>
            {why && <p id="submit-why" className="text-xs text-slate-600">{why}</p>}
          </div>}
          {perms?.can_review && data.sheet.status === 'submitted' && <button type="button" className={ui.primary} disabled={busy}
            onClick={() => void (async () => { if (await confirmAction(t('mwApproveConfirm'), { tone: 'default', confirmLabel: t('mwApprove') })) await act('approve') })()}>{t('mwApprove')}</button>}
          {perms?.can_review && !returning && <button type="button" className={ui.secondary} disabled={busy} onClick={() => setReturning(true)}>{t('mwReturn')}</button>}
        </div>
      </div>
      {returning && <form className="mt-3 flex flex-wrap items-end gap-2" onSubmit={(e: FormEvent) => { e.preventDefault(); void act('return', returnReason) }}>
        <label className={`${ui.label} min-w-0 flex-1`}>{t('mwReturnReason')}<input required autoFocus maxLength={240} className={ui.field} value={returnReason} onChange={e => setReturnReason(e.target.value)} /></label>
        <button className={ui.primary} disabled={busy || !returnReason.trim()}>{t('mwReturn')}</button>
        <button type="button" className={ui.ghost} onClick={() => setReturning(false)}>{t('mwCancel')}</button>
      </form>}
    </header>

    {(message || problem) && <Notice tone={problem ? 'error' : 'success'}>{problem || message}</Notice>}
    {readOnly && <p role="status" className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">{t(`mwRO_${readOnly}`)}</p>}
    {data.sheet.status === 'returned' && data.sheet.return_reason && <p role="status" className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm font-medium text-rose-800">{t('mwReturned', { name: data.sheet.reviewed_by, reason: data.sheet.return_reason })}</p>}
    {autosave.recovery && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
      <span>{t('mwRecover', { count: autosave.recovery.count, time: clock(autosave.recovery.savedAt) })}</span>
      <span className="flex gap-2"><button type="button" className={ui.primary} onClick={autosave.restore}>{t('mwRestore')}</button><button type="button" className={ui.secondary} onClick={autosave.discard}>{t('mwDiscard')}</button></span>
    </div>}

    <div className={historyOpen ? 'grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]' : ''}>
      <section className={`${ui.card} min-w-0 overflow-hidden`} aria-label={t('mwMarks')}>
        {/* Toolbar */}
        <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 p-3">
          <label className="min-w-0 basis-full lg:basis-auto lg:flex-1 lg:max-w-xs"><span className="sr-only">{t('mwSearch')}</span>
            <input type="search" placeholder={t('mwSearch')} className="block min-h-10 w-full rounded-xl border border-slate-300 bg-white px-3 text-sm" value={search} onChange={e => setSearch(e.target.value)} /></label>
          <div role="group" aria-label={t('mwRowFilter')} className="flex rounded-xl bg-slate-100 p-0.5">
            {(['all', 'missing', 'attention'] as const).map(f => <button key={f} type="button" aria-pressed={filter === f} onClick={() => setFilter(f)}
              className={`min-h-9 rounded-lg px-2.5 text-xs font-semibold ${filter === f ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600 hover:text-slate-900'}`}>
              {t(f === 'all' ? 'mwRowsAll' : f === 'missing' ? 'mwRowsMissing' : 'mwRowsAttention')}</button>)}
          </div>
          <div className="ml-auto flex flex-wrap gap-1">
            {perms?.can_edit && <>
              <button type="button" className={ui.ghost} onClick={() => setPaste({ plan: null, error: '', box: true, text: '' })}>{t('mwPaste')}</button>
              <label className={`${ui.ghost} cursor-pointer`}>{t('mwImport')}<input type="file" accept=".csv,.tsv,.txt,text/csv,text/plain" className="sr-only" onChange={async e => { const file = e.target.files?.[0]; e.target.value = ''; if (file) openPaste(await file.text(), 0) }} /></label>
            </>}
            {!wide && <button type="button" className={ui.ghost} aria-pressed={single} onClick={() => setSingle(v => !v)}>{t(single ? 'mwAllStudents' : 'mwOneAtATime')}</button>}
            <button type="button" className={ui.ghost} aria-pressed={historyOpen} onClick={() => setHistoryOpen(v => !v)}>{t('mwHistory')}</button>
            <button type="button" className={ui.ghost} aria-expanded={shortcuts} aria-controls="mw-shortcuts" onClick={() => setShortcuts(v => !v)}>{t('mwShortcuts')}</button>
          </div>
        </div>
        {shortcuts && <ul id="mw-shortcuts" className="grid gap-x-6 gap-y-1 border-b border-slate-100 bg-slate-50 px-4 py-3 text-xs text-slate-700 sm:grid-cols-2">
          {['mwKeyEnter', 'mwKeyArrows', 'mwKeyTab', 'mwKeyEsc', 'mwKeyLetters', 'mwKeyPaste'].map(k => <li key={k}>{t(k)}</li>)}
        </ul>}

        {/* Phones: one student at a time */}
        {!wide && single && visible.length > 0 && <div className="p-4">
          <p className="text-xs text-slate-600">{t('mwPosition', { n: Math.min(cursor, visible.length - 1) + 1, total: visible.length })}</p>
          <ul><RowCard row={visible[Math.min(cursor, visible.length - 1)]} index={Math.min(cursor, visible.length - 1)} large {...rowProps(visible[Math.min(cursor, visible.length - 1)])} /></ul>
          <div className="mt-3 flex gap-2"><button type="button" className={`${ui.secondary} flex-1`} disabled={cursor <= 0} onClick={() => setCursor(c => Math.max(0, c - 1))}>{t('mwPrev')}</button>
            <button type="button" className={`${ui.primary} flex-1`} disabled={cursor >= visible.length - 1} onClick={() => setCursor(c => Math.min(visible.length - 1, c + 1))}>{t('mwNext')}</button></div>
        </div>}

        {/* Phones: the whole list */}
        {!wide && !single && <ul className="divide-y divide-slate-100">{visible.map((row, index) => <RowCard key={row.student_id} row={row} index={index} {...rowProps(row)} />)}</ul>}

        {/* Tablets and desktops: the grid */}
        {wide && <div className="max-h-[70vh] overflow-auto">
          <table className="w-full table-fixed border-separate border-spacing-0 text-sm">
            <colgroup><col className="w-14 lg:w-16" /><col className="w-52 lg:w-80" />{components.length ? <col style={{ width: `${components.length * 4.25 + 4.5}rem` }} /> : <col className="w-32 lg:w-36" />}<col className="w-14 lg:w-16" /><col className="w-40 lg:w-44" /><col /></colgroup>
            <thead><tr className="text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
              <th scope="col" className="sticky left-0 top-0 z-20 border-b border-slate-200 bg-slate-50 px-3 py-2.5">{t('mwRoll')}</th>
              <th scope="col" className="sticky left-14 lg:left-16 top-0 z-20 border-b border-slate-200 bg-slate-50 px-3 py-2.5">{t('mwStudent')}</th>
              <th scope="col" className="sticky top-0 z-10 border-b border-slate-200 bg-slate-50 px-3 py-2.5 text-right">{t('mwMarks')} <span className="font-normal normal-case">/ {data.sheet.full_marks}</span>
                {components.length > 0 && <span className="block font-normal normal-case tracking-normal">{components.map(c => `${c.key} ${c.full}`).join(' · ')}</span>}</th>
              <th scope="col" className="sticky top-0 z-10 border-b border-slate-200 bg-slate-50 px-3 py-2.5 text-right">{t('mwPercent')}</th>
              <th scope="col" className="sticky top-0 z-10 border-b border-slate-200 bg-slate-50 px-3 py-2.5">{t('mwStatus')}</th>
              <th scope="col" className="sticky top-0 z-10 border-b border-slate-200 bg-slate-50 px-2 py-2.5"><span className="sr-only">{t('mwCellSaved')}</span></th>
            </tr></thead>
            <tbody>{visible.map((row, index) => <GridRow key={row.student_id} row={row} index={index} {...rowProps(row)} />)}</tbody>
          </table>
        </div>}
        {visible.length === 0 && <p className="px-4 py-8 text-center text-sm text-slate-600">{t('mwQueueEmpty')}</p>}
      </section>

      {historyOpen && <HistoryPanel examId={examId} sheetId={sheetId} teacher={teacher} onClose={() => setHistoryOpen(false)} />}
    </div>

    {paste && <PasteDialog state={paste} rows={rows} errText={errText} full={full}
      onText={text => setPaste({ ...paste, text })} onPreview={() => openPaste(paste.text, 0)} onApply={applyPaste} onCancel={() => setPaste(null)} value={autosave.value} />}
  </div>

  // A row gets a snapshot of its own cell, so a memoised row re-renders only
  // when that cell changes.
  function rowProps(row: ExamMarkRow) {
    const id = row.student_id
    const v = autosave.value(id)
    const view: View = { status: v.status, text: v.text, reason: v.reason, parts: v.parts, cell: autosave.cell(id), draft: autosave.isDraft(id), server: autosave.serverCell(id), editable: editableRow(row) }
    return { view, perms: perms!, full, ctx }
  }
}

function BackLink({ teacher, examId }: { teacher: boolean; examId: string }) {
  const { t } = useTranslation()
  return <Link to={teacher ? '/teacher/exams' : '/admin/exams'} className={`${ui.link} inline-flex items-center gap-1 text-sm`} data-exam={examId}>← {t(teacher ? 'mwBack' : 'mwBackStaff')}</Link>
}

function SaveIndicator({ autosave, canEdit }: { autosave: ReturnType<typeof useSheetAutosave>; canEdit: boolean }) {
  const { t } = useTranslation()
  if (!canEdit) return null
  const { state, pending, conflicts, errors, savedAt } = autosave
  const [tone, text] = state === 'saving' ? ['bg-sky-500 animate-pulse motion-reduce:animate-none', t('mwSaving')]
    : state === 'offline' ? ['bg-amber-500', t('mwOffline', { count: pending })]
      : state === 'conflict' ? ['bg-rose-600', t('mwConflictState', { count: conflicts })]
        : state === 'error' ? ['bg-rose-600', t('mwSaveError', { count: errors || pending })]
          : state === 'blocked' ? ['bg-rose-600', t('mwBlocked')]
            : pending > 0 ? ['bg-slate-400', t('mwUnsaved', { n: pending })]
              : savedAt ? ['bg-emerald-600', t('mwSaved', { time: clock(savedAt) })] : ['bg-slate-300', t('mwSaveIdle')]
  return <p role="status" aria-live="polite" data-save-state={state} className="flex max-w-xs items-center gap-2 text-sm text-slate-700">
    <span aria-hidden="true" className={`h-2 w-2 shrink-0 rounded-full ${tone}`} />{text}
  </p>
}

type View = CellValue & { cell?: CellState; draft: boolean; server?: ServerCell; editable: boolean }
type RowCtx = {
  edit: ReturnType<typeof useSheetAutosave>['edit']
  resolve: ReturnType<typeof useSheetAutosave>['resolve']
  inputs: MutableRefObject<Map<string, HTMLInputElement>>
  errText: (code: string) => string
  onMarksKey: (event: KeyboardEvent<HTMLInputElement>, index: number, row: ExamMarkRow, part?: string) => void
  onCellPaste: (event: ClipboardEvent<HTMLInputElement>, index: number) => void
  t: (key: string, options?: Record<string, unknown>) => string
  components: ExamSheetComponent[] // empty: one total per student
}
type RowProps = { row: ExamMarkRow; index: number; view: View; perms: ExamSheetPermissions; full: number; ctx: RowCtx; large?: boolean }

function percentOf(view: View, full: number) {
  if (!carries(view.status)) return ''
  const parsed = parseCell(view.text, full)
  return parsed.kind === 'marks' ? String(Math.round(parsed.value * 100 / full)) : ''
}

// statusTone keeps "present" quiet and makes the exceptions stand out.
function statusTone(status: string) {
  switch (status) {
    case 'absent': return 'border-rose-200 bg-rose-50 font-semibold text-rose-800'
    case 'incomplete': return 'border-amber-200 bg-amber-50 font-semibold text-amber-900'
    case 'exempt': case 'withheld': return 'border-slate-300 bg-slate-100 font-semibold text-slate-800'
    default: return 'border-transparent bg-transparent text-slate-600 hover:border-slate-300'
  }
}

function StatusControl({ row, view, perms, ctx }: RowProps) {
  const { t } = ctx
  if (!view.editable) {
    return <span className="text-sm text-slate-700">{view.status ? t(`e3Status_${view.status}`) : '—'}{(view.status === 'exempt' || view.status === 'withheld') && <span className="block text-xs text-slate-500">{t('mwLockedByAdmin')}{view.reason ? ` · ${view.reason}` : ''}</span>}</span>
  }
  const options = perms.statuses.includes(view.status as ExamMarkStatus) || !view.status ? perms.statuses : [...perms.statuses, view.status as ExamMarkStatus]
  return <div className="space-y-1">
    <select aria-label={t('mwStatusFor', { name: row.student_name })} className={`min-h-10 w-full max-w-[10rem] rounded-lg border px-2 text-sm transition-colors focus:border-[var(--brand-primary)] focus:outline-none focus:ring-4 focus:ring-[color-mix(in_srgb,var(--brand-primary)_18%,transparent)] sm:min-h-9 ${statusTone(view.status)}`} value={view.status || 'present'}
      onChange={e => ctx.edit(row.student_id, { status: e.target.value as ExamMarkStatus })}>
      {options.map(s => <option key={s} value={s}>{t(`e3Status_${s}`)}</option>)}
    </select>
    {(view.status === 'exempt' || view.status === 'withheld') && <input aria-label={t('mwReasonFor', { name: row.student_name })} placeholder={t('mwReasonPlaceholder')} maxLength={240}
      className="block min-h-9 w-full max-w-[12rem] rounded-lg border border-slate-300 bg-white px-2 text-sm" value={view.reason} onChange={e => ctx.edit(row.student_id, { reason: e.target.value })} />}
  </div>
}

function MarksControl(p: RowProps) {
  return p.ctx.components.length > 0 ? <ComponentInputs {...p} /> : <TotalInput {...p} />
}

// ComponentInputs: one input per component and their exact total (E5).
function ComponentInputs({ row, index, view, ctx, large }: RowProps) {
  const { t } = ctx
  const errorId = `mw-err-${row.student_id}`
  const error = view.cell?.kind === 'error' ? ctx.errText(view.cell.code) : ''
  const marks = carries(view.status)
  return <div className="flex flex-col items-end gap-1">
    <div className="flex items-end gap-1.5">
      {ctx.components.map((c, k) => <label key={c.key} className="block text-center text-[11px] font-semibold uppercase tracking-wide text-slate-500">{c.key}
        <input ref={el => {
          const key = `${row.student_id}|${c.key}`
          if (el) { ctx.inputs.current.set(key, el); if (k === 0) ctx.inputs.current.set(row.student_id, el) }
          else { ctx.inputs.current.delete(key); if (k === 0) ctx.inputs.current.delete(row.student_id) }
        }}
          type="text" inputMode="decimal" autoComplete="off" spellCheck={false} disabled={!view.editable}
          aria-label={t('mwPartFor', { part: c.key, name: row.student_name, full: c.full })} aria-invalid={!!error} aria-describedby={error ? errorId : undefined}
          className={`${large ? 'min-h-14 w-20 text-xl' : 'min-h-10 w-16 text-sm sm:min-h-9'} mt-0.5 block rounded-lg border bg-white px-2 text-right font-medium normal-case tabular-nums tracking-normal text-slate-900 transition-colors focus:border-[var(--brand-primary)] focus:outline-none focus:ring-4 focus:ring-[color-mix(in_srgb,var(--brand-primary)_18%,transparent)] disabled:bg-slate-50 disabled:text-slate-500 ${error ? 'border-rose-300' : view.cell?.kind === 'conflict' ? 'border-amber-300' : 'border-slate-300'}`}
          value={marks ? view.parts[c.key] ?? '' : ''} placeholder={marks || k > 0 ? '' : t(`e3Status_${view.status}`)}
          onChange={e => ctx.edit(row.student_id, { parts: { [c.key]: e.target.value }, status: marks && view.status ? view.status as ExamMarkStatus : 'present' })}
          onKeyDown={e => ctx.onMarksKey(e, index, row, c.key)} onPaste={e => ctx.onCellPaste(e, index)} />
      </label>)}
      <span className="min-w-[3rem] pb-2 text-right text-sm font-semibold tabular-nums text-slate-800" aria-label={t('mwPartsTotal', { name: row.student_name })}>{marks && view.text ? `= ${view.text}` : '—'}</span>
    </div>
    {error && <p id={errorId} className="max-w-[16rem] text-right text-xs font-medium text-rose-700">{error}</p>}
  </div>
}

function TotalInput({ row, index, view, full, ctx, large }: RowProps) {
  const { t } = ctx
  const errorId = `mw-err-${row.student_id}`
  const error = view.cell?.kind === 'error' ? ctx.errText(view.cell.code) : ''
  return <div className="flex flex-col items-end gap-1">
    <input ref={el => { if (el) ctx.inputs.current.set(row.student_id, el); else ctx.inputs.current.delete(row.student_id) }}
      type="text" inputMode="decimal" autoComplete="off" spellCheck={false} disabled={!view.editable}
      aria-label={t('mwMarksFor', { name: row.student_name, full })} aria-invalid={!!error} aria-describedby={error ? errorId : undefined}
      className={`${large ? 'min-h-14 w-32 text-2xl' : 'min-h-10 w-24 text-sm sm:min-h-9'} rounded-lg border bg-white px-2.5 text-right font-medium tabular-nums text-slate-900 transition-colors focus:border-[var(--brand-primary)] focus:outline-none focus:ring-4 focus:ring-[color-mix(in_srgb,var(--brand-primary)_18%,transparent)] disabled:bg-slate-50 disabled:text-slate-500 ${error ? 'border-rose-300' : view.cell?.kind === 'conflict' ? 'border-amber-300' : 'border-slate-300'}`}
      value={carries(view.status) ? view.text : ''} placeholder={carries(view.status) ? '' : t(`e3Status_${view.status}`)}
      onChange={e => ctx.edit(row.student_id, { text: e.target.value, status: carries(view.status) && view.status ? view.status as ExamMarkStatus : 'present' })}
      onKeyDown={e => ctx.onMarksKey(e, index, row)} onPaste={e => ctx.onCellPaste(e, index)} />
    {error && <p id={errorId} className="max-w-[14rem] text-right text-xs font-medium text-rose-700">{error}</p>}
  </div>
}

function StateIcon({ view, t }: { view: View; t: RowCtx['t'] }) {
  const cell = view.cell
  const [glyph, label, tone] = cell?.kind === 'saving' ? ['…', t('mwCellSaving'), 'text-sky-700']
    : cell?.kind === 'error' ? ['!', t('mwSaveError', { count: 1 }), 'text-rose-700']
      : cell?.kind === 'conflict' ? ['⇄', t('mwConflictState', { count: 1 }), 'text-amber-800']
        : view.draft ? ['•', t('mwCellDraft'), 'text-slate-500']
          : cell?.kind === 'saved' ? ['✓', t('mwCellSaved'), 'text-emerald-700'] : ['', '', '']
  if (!glyph) return null
  return <span className={`inline-block w-5 text-center text-sm font-bold ${tone}`} title={label}><span aria-hidden="true">{glyph}</span><span className="sr-only">{label}</span></span>
}

function ConflictNote({ row, view, ctx, server }: RowProps & { server: ServerCell }) {
  const { t } = ctx
  const show = (status: string, text: string) => status && status !== 'present' ? t(`e3Status_${status}`) : text || t('mwEmptyValue')
  // On a component sheet, both sides show their components.
  const split = (values: Record<string, string | number> | undefined, total: string) => ctx.components.length && values && Object.keys(values).length
    ? `${ctx.components.map(c => `${c.key} ${values[c.key] ?? '—'}`).join(' · ')} = ${total || '—'}` : total
  return <div role="alert" className="flex flex-wrap items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
    <span>{t('mwConflict', { theirs: show(server.status, split(server.components, server.marks == null ? '' : String(server.marks))), mine: show(view.status, split(view.parts, view.text)) })}</span>
    <button type="button" className={`${ui.secondary} min-h-8 px-2 py-1 text-xs`} onClick={() => ctx.resolve(row.student_id, 'theirs')}>{t('mwUseTheirs')}</button>
    <button type="button" className={`${ui.secondary} min-h-8 px-2 py-1 text-xs`} onClick={() => ctx.resolve(row.student_id, 'mine')}>{t('mwKeepMine')}</button>
  </div>
}

const GridRow = memo(function GridRow(p: RowProps) {
  const conflict = p.view.cell?.kind === 'conflict' ? p.view.cell.server : null
  const tint = p.view.cell?.kind === 'error' ? 'bg-rose-50' : conflict ? 'bg-amber-50' : 'bg-white'
  return <>
    <tr className="group">
      <td className={`sticky left-0 z-[1] border-b border-slate-100 px-3 py-1.5 align-middle tabular-nums text-slate-600 group-focus-within:bg-[var(--brand-soft)] ${tint}`}>{p.row.roll_no ?? '—'}</td>
      <th scope="row" className={`sticky left-14 lg:left-16 z-[1] truncate border-b border-slate-100 px-3 py-1.5 text-left align-middle font-normal group-focus-within:bg-[var(--brand-soft)] ${tint}`}><span className="block truncate font-medium text-slate-900">{p.row.student_name}</span><span className="block text-xs text-slate-500">{p.row.admission_no}</span></th>
      <td className={`border-b border-slate-100 px-3 py-1.5 align-middle group-focus-within:bg-[var(--brand-soft)] ${tint}`}><MarksControl {...p} /></td>
      <td className={`border-b border-slate-100 px-3 py-1.5 text-right align-middle tabular-nums text-slate-500 group-focus-within:bg-[var(--brand-soft)] ${tint}`}>{percentOf(p.view, p.full)}</td>
      <td className={`border-b border-slate-100 px-3 py-1.5 align-middle group-focus-within:bg-[var(--brand-soft)] ${tint}`}><StatusControl {...p} /></td>
      <td className={`border-b border-slate-100 px-2 py-1.5 text-left align-middle group-focus-within:bg-[var(--brand-soft)] ${tint}`}><StateIcon view={p.view} t={p.ctx.t} /></td>
    </tr>
    {conflict && <tr><td colSpan={6} className="border-b border-slate-100 bg-amber-50 px-3 pb-2"><ConflictNote {...p} server={conflict} /></td></tr>}
  </>
}, sameRow)

const RowCard = memo(function RowCard(p: RowProps) {
  const conflict = p.view.cell?.kind === 'conflict' ? p.view.cell.server : null
  return <li className={`space-y-2 px-4 py-3 ${p.view.cell?.kind === 'error' ? 'bg-rose-50' : conflict ? 'bg-amber-50' : ''}`}>
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0"><span className="block truncate font-medium text-slate-900">{p.row.student_name}</span><span className="block text-xs text-slate-500">{p.ctx.t('mwRoll')} {p.row.roll_no ?? '—'} · {p.row.admission_no}</span></div>
      <StateIcon view={p.view} t={p.ctx.t} />
    </div>
    <div className="flex items-start justify-between gap-3"><div className="min-w-0 flex-1"><StatusControl {...p} /></div><MarksControl {...p} /></div>
    {conflict && <ConflictNote {...p} server={conflict} />}
  </li>
}, sameRow)

// sameRow: a row re-renders only when its own snapshot, its position or the
// shared context changes.
function sameRow(a: RowProps, b: RowProps) {
  const x = a.view, y = b.view
  return a.row === b.row && a.index === b.index && a.perms === b.perms && a.full === b.full && a.large === b.large && a.ctx === b.ctx
    && x.status === y.status && x.text === y.text && x.reason === y.reason && x.cell === y.cell && x.draft === y.draft && x.server === y.server && x.editable === y.editable
    && JSON.stringify(x.parts) === JSON.stringify(y.parts)
}

function PasteDialog({ state, rows, errText, full, onText, onPreview, onApply, onCancel, value }: {
  state: { plan: PastePlan | null; error: string; box: boolean; text: string }; rows: ExamMarkRow[]; errText: (code: string) => string; full: number
  onText: (text: string) => void; onPreview: () => void; onApply: () => void; onCancel: () => void; value: (id: string) => CellValue
}) {
  const { t } = useTranslation()
  const dialog = useRef<HTMLDivElement>(null)
  useEffect(() => { dialog.current?.querySelector<HTMLElement>('textarea, button[data-primary]')?.focus() }, [state.box, state.plan])
  const byId = new Map(rows.map(r => [r.student_id, r]))
  const plan = state.plan
  const counts = { apply: 0, reject: 0, skip: 0 }
  for (const item of plan?.items ?? []) counts[item.result]++
  const firstStudent = plan?.mode === 'by_position' ? byId.get(plan.items.find(i => i.student_id)?.student_id ?? '')?.student_name ?? '' : ''
  return <div className="fixed inset-0 z-50 flex items-end justify-center bg-slate-900/40 p-0 sm:items-center sm:p-6" onKeyDown={e => { if (e.key === 'Escape') onCancel() }}>
    <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby="mw-paste-title" className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-t-2xl bg-white shadow-xl sm:rounded-2xl">
      <div className="border-b border-slate-100 px-5 py-4"><h2 id="mw-paste-title" className="text-base font-bold text-slate-900">{t('mwPasteTitle')}</h2>
        {plan && <p className="mt-1 text-sm text-slate-600">{plan.mode === 'by_admission' ? t('mwPasteByAdmission') : t('mwPasteByPosition', { name: firstStudent })} {t('mwPasteSummary', counts)}</p>}</div>
      <div className="min-h-0 flex-1 overflow-auto px-5 py-4">
        {state.error && <p role="alert" className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">{state.error}</p>}
        {state.box && !plan && <label className={`${ui.label} block`}>{t('mwPasteBox')}<textarea rows={8} className={`${ui.field} font-mono`} value={state.text} onChange={e => onText(e.target.value)} />
          <span className="mt-1 block text-xs font-normal normal-case tracking-normal text-slate-600">{t('mwPasteHint')}</span></label>}
        {plan && <table className="w-full text-left text-sm"><thead><tr className="text-xs uppercase tracking-wide text-slate-500">
          <th className="py-1.5 pr-2">{t('mwPasteLine')}</th><th className="py-1.5 pr-2">{t('mwStudent')}</th><th className="py-1.5 pr-2 text-right">{t('mwPasteCurrent')}</th><th className="py-1.5 pr-2 text-right">{t('mwPasteNew')}</th><th className="py-1.5">{t('mwPasteResult')}</th></tr></thead>
          <tbody>{plan.items.map(item => {
            const row = item.student_id ? byId.get(item.student_id) : undefined
            const now = row ? value(row.student_id) : null
            const shown = item.parsed.kind === 'marks' ? String(item.parsed.value) : item.parsed.kind === 'status' ? t(`e3Status_${item.parsed.status}`)
              : item.parsed.kind === 'components' ? Object.entries(item.parsed.values).map(([k, v]) => `${k} ${v}`).join(' · ') : item.raw || '—'
            return <tr key={item.line} className={`border-t border-slate-100 ${item.result === 'reject' ? 'text-rose-800' : item.result === 'skip' ? 'text-slate-500' : 'text-slate-900'}`}>
              <td className="py-1.5 pr-2 tabular-nums">{item.line}</td>
              <td className="py-1.5 pr-2">{row ? <>{row.student_name} <span className="text-xs text-slate-500">{row.admission_no}</span></> : '—'}</td>
              <td className="py-1.5 pr-2 text-right tabular-nums">{now ? (now.status && now.status !== 'present' ? t(`e3Status_${now.status}`) : now.text || '—') : '—'}</td>
              <td className="py-1.5 pr-2 text-right tabular-nums">{shown}</td>
              <td className="py-1.5 text-xs font-medium">{item.result === 'apply' ? t('mwPasteOK') : item.result === 'skip' ? t('mwPasteSkip') : errText(item.reason || '')}</td>
            </tr>
          })}</tbody></table>}
      </div>
      <div className="flex flex-wrap justify-end gap-2 border-t border-slate-100 px-5 py-3">
        <button type="button" className={ui.secondary} onClick={onCancel}>{t('mwCancel')}</button>
        {state.box && !plan && <button type="button" data-primary className={ui.primary} disabled={!state.text.trim()} onClick={onPreview}>{t('mwPreview')}</button>}
        {plan && <button type="button" data-primary className={ui.primary} disabled={counts.apply === 0} onClick={onApply}>{counts.apply ? t('mwPasteApply', { count: counts.apply }) : t('mwPasteNothing')}</button>}
      </div>
      <span hidden>{full}</span>
    </div>
  </div>
}

function HistoryPanel({ examId, sheetId, teacher, onClose }: { examId: string; sheetId: string; teacher: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const authorized = usePeopleApi()
  const history = useQuery({ queryKey: ['marks', 'history', examId, sheetId], gcTime: 0, queryFn: () => authorized(token => getExamSheetHistory(token, examId, sheetId, teacher)), retry: false })
  const mark = (status: string | null | undefined, marks: number | string | null | undefined) => !status || status === 'present' ? (marks == null ? '—' : String(Number(marks))) : t(`e3Status_${status}`)
  const line = (h: ExamSheetHistoryEntry) => h.action === 'exam.mark_saved' ? `${h.student_name}: ${mark(h.metadata.old_status, h.metadata.old_marks)} → ${mark(h.metadata.new_status, h.metadata.new_marks)}`
    : h.action === 'exam.sheet_returned' ? t('mwH_returned', { reason: h.metadata.reason ?? '' }) : t(h.action === 'exam.sheet_submitted' ? 'mwH_submitted' : 'mwH_approved')
  return <aside className={`${ui.card} max-h-[70vh] overflow-auto p-4`} aria-labelledby="mw-history-title">
    <div className="flex items-center justify-between gap-2"><h2 id="mw-history-title" className={ui.sectionTitle}>{t('mwHistoryTitle')}</h2>
      <button type="button" className={ui.ghost} onClick={onClose}>{t('mwHistoryClose')}</button></div>
    {history.isPending ? <div className="mt-3"><Skeleton rows={4} /></div> : history.isError ? <p role="alert" className="mt-3 text-sm text-rose-700">{t('mwLoadError')}</p>
      : history.data.length === 0 ? <p className="mt-3 text-sm text-slate-600">{t('mwHistoryEmpty')}</p>
        : <ol className="mt-3 space-y-2.5">{history.data.map((h, i) => <li key={i} className="text-sm">
          <span className="block text-slate-900">{line(h)}</span>
          <span className="block text-xs text-slate-500">{h.at.slice(0, 16)} · {h.actor}</span>
        </li>)}</ol>}
  </aside>
}
