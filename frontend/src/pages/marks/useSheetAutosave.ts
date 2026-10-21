import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, type ExamCellError, type ExamMarkCell, type ExamMarkRow } from '../../api/client'
import { parseCell, type MarkStatus } from './marksInput'

// Autosave for one mark sheet on top of the E3 per-cell API.
//
// Guarantees:
//   - an edit is never lost silently: it stays a draft until the server
//     confirms it, and drafts survive a reload of the tab (sessionStorage,
//     values and ids only);
//   - one request at a time; edits are batched after a short pause;
//   - every cell is sent with the version the user edited from, so a stale
//     edit becomes a conflict that shows the server's value, never an
//     overwrite;
//   - a retried request whose earlier attempt did land is recognised as saved.

// parts are a component sheet's typed values by component (E5); text is then
// their sum, for display and the percentage.
export type CellValue = { status: MarkStatus | ''; text: string; reason: string; parts: Record<string, string> }
export type ServerCell = { status: MarkStatus | ''; marks: number | null; reason: string; version: number; components?: Record<string, number> }
export type Draft = { status: MarkStatus; text: string; reason: string; base: number; parts?: Record<string, string> }
/** A component of a component sheet. */
export type SheetPart = { key: string; full: number }
export type CellState =
  | { kind: 'saving' }
  | { kind: 'saved'; at: number }
  | { kind: 'error'; code: string }
  | { kind: 'conflict'; server: ServerCell }
export type SaveState = 'idle' | 'saving' | 'saved' | 'offline' | 'error' | 'conflict' | 'blocked'
export type BatchReport = { saved: number; failed: { student_id: string; code: string }[] }

type SaveFn = (entries: ExamMarkCell[]) => Promise<{ saved: ExamMarkCell[]; errors: ExamCellError[] }>

type Options = {
  sheetId: string
  rows: ExamMarkRow[] | undefined
  fullMarks: number
  enabled: boolean
  storageKey: string | null
  save: SaveFn
  onBlocked: () => void
  components?: SheetPart[] // a component sheet's components, in order
}

const idleDelay = 600
const backoff = [2000, 4000, 8000, 15000, 30000]
const storagePrefix = 'flybirds.marks.v1.'

// Drafts kept in sessionStorage: ids and values only, never names; gone when
// the tab closes, and cleared on save and on sign-out (clearMarksDrafts).
type Stored = { savedAt: number; drafts: Record<string, Draft> }

export function clearMarksDrafts() {
  try {
    for (let i = sessionStorage.length - 1; i >= 0; i--) {
      const key = sessionStorage.key(i)
      if (key?.startsWith(storagePrefix)) sessionStorage.removeItem(key)
    }
  } catch { /* storage unavailable */ }
}

export function marksDraftKey(userId: string, sheetId: string) {
  return `${storagePrefix}${userId}.${sheetId}`
}

function serverOf(row: ExamMarkRow): ServerCell {
  return { status: (row.status || '') as ServerCell['status'], marks: row.marks, reason: row.status_reason || '', version: row.row_version, components: row.components }
}

const noParts: SheetPart[] = []

/** Typed parts of a server cell, as text. */
function partsOf(cell: ServerCell | undefined): Record<string, string> {
  return Object.fromEntries(Object.entries(cell?.components || {}).map(([k, v]) => [k, String(v)]))
}

/** The exact sum of the parts as text, or '' while any part is not a mark. */
export function sumParts(parts: Record<string, string>, components: SheetPart[]): string {
  let total = 0
  for (const c of components) {
    const p = parseCell(parts[c.key] ?? '', c.full)
    if (p.kind !== 'marks') return ''
    total += Math.round(p.value * 100)
  }
  return String(total / 100)
}

function textOf(marks: number | null) {
  return marks == null ? '' : String(marks)
}

// sameValue compares what the user means, not how it is typed: "45.50" = 45.5.
// A cell with no server row equals a present draft with nothing typed.
function sameValue(draft: { status: string; text: string; reason: string; parts?: Record<string, string> }, server: ServerCell, components: SheetPart[] = noParts): boolean {
  const empty = Object.values(draft.parts || {}).every(v => v.trim() === '')
  if (server.status === '') return draft.status === 'present' && draft.text.trim() === '' && empty
  if (draft.status !== server.status) return false
  // A component sheet: every component equal (the total follows).
  if (components.length > 0 && (draft.status === 'present' || (draft.status === 'withheld' && !empty))) {
    return components.every(c => {
      const p = parseCell(draft.parts?.[c.key] ?? '', c.full)
      const stored = server.components?.[c.key]
      return p.kind === 'marks' && stored != null && Math.round(p.value * 100) === Math.round(stored * 100)
    })
  }
  if ((draft.status === 'exempt' || draft.status === 'withheld') && draft.reason.trim() !== server.reason) return false
  if (draft.status !== 'present' && draft.status !== 'withheld') return true
  const parsed = parseCell(draft.text, 10_000_000)
  if (parsed.kind === 'empty') return server.marks == null
  return parsed.kind === 'marks' && server.marks != null && Math.round(parsed.value * 100) === Math.round(server.marks * 100)
}

export function useSheetAutosave({ sheetId, rows, fullMarks, enabled, storageKey, save, onBlocked, components = noParts }: Options) {
  const server = useRef(new Map<string, ServerCell>())
  const drafts = useRef(new Map<string, Draft>())
  const cells = useRef(new Map<string, CellState>())
  const inFlight = useRef(false)
  const timer = useRef<number | undefined>(undefined)
  const attempt = useRef(0)
  const saveRef = useRef(save)
  const blockedRef = useRef(onBlocked)
  saveRef.current = save
  blockedRef.current = onBlocked
  const [state, setState] = useState<SaveState>('idle')
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [recovery, setRecovery] = useState<{ count: number; savedAt: number; drafts: Record<string, Draft> } | null>(null)
  const [report, setReport] = useState<BatchReport | null>(null)
  const [requests, setRequests] = useState(0)
  const [, setTick] = useState(0)
  const bump = useCallback(() => setTick(t => t + 1), [])
  const storageOK = useRef(true)

  const persist = useCallback(() => {
    if (!storageKey) return
    try {
      if (drafts.current.size === 0) sessionStorage.removeItem(storageKey)
      else sessionStorage.setItem(storageKey, JSON.stringify({ savedAt: Date.now(), drafts: Object.fromEntries(drafts.current) } satisfies Stored))
      storageOK.current = true
    } catch { storageOK.current = false }
  }, [storageKey])

  // Server rows arrive (first load or refresh): adopt them, keep drafts, and
  // offer to restore drafts a reload left behind.
  const loaded = useRef(false)
  useEffect(() => {
    if (!rows) return
    for (const row of rows) server.current.set(row.student_id, serverOf(row))
    for (const [id, draft] of drafts.current) {
      const cell = server.current.get(id)
      if (cell && sameValue(draft, cell, components)) { drafts.current.delete(id); cells.current.delete(id) }
    }
    if (!loaded.current && storageKey) {
      loaded.current = true
      try {
        // Another account signed in to this tab (for example after its session
        // expired): its drafts are not this user's to see or keep.
        const mine = storageKey.slice(0, storageKey.lastIndexOf('.') + 1)
        for (let i = sessionStorage.length - 1; i >= 0; i--) {
          const key = sessionStorage.key(i)
          if (key?.startsWith(storagePrefix) && !key.startsWith(mine)) sessionStorage.removeItem(key)
        }
        const stored = JSON.parse(sessionStorage.getItem(storageKey) || 'null') as Stored | null
        if (stored) {
          const pending = Object.entries(stored.drafts).filter(([id, d]) => { const cell = server.current.get(id); return cell && !sameValue(d, cell, components) })
          if (pending.length && enabled) setRecovery({ count: pending.length, savedAt: stored.savedAt, drafts: Object.fromEntries(pending) })
          else sessionStorage.removeItem(storageKey)
        }
      } catch { /* unreadable or unavailable storage: nothing to restore */ }
    }
    bump()
  }, [rows, fullMarks, storageKey, enabled, bump, components])

  const schedule = useCallback((ms = idleDelay) => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => { timer.current = undefined; void flushRef.current() }, ms)
  }, [])

  const flush = useCallback(async () => {
    if (inFlight.current || !enabled) return
    const entries: ExamMarkCell[] = []
    const sent = new Map<string, Draft>()
    let localErrors = 0
    for (const [id, draft] of drafts.current) {
      const cell = cells.current.get(id)
      if (cell?.kind === 'conflict') continue
      let marks: number | null = null
      // A component sheet sends its components and the server adds them up.
      // A row still being filled in waits as an unsaved draft, quietly.
      if (components.length > 0 && (draft.status === 'present' || (draft.status === 'withheld' && Object.values(draft.parts || {}).some(v => v.trim() !== '')))) {
        if (components.some(c => (draft.parts?.[c.key] ?? '').trim() === '')) continue
        const values: Record<string, number> = {}
        let bad = ''
        for (const c of components) {
          const p = parseCell(draft.parts?.[c.key] ?? '', c.full)
          if (p.kind === 'marks') values[c.key] = p.value
          else bad = p.kind === 'error' ? p.code : 'not_a_number'
        }
        if (bad) { cells.current.set(id, { kind: 'error', code: bad }); localErrors++; continue }
        entries.push({ exam_subject_id: sheetId, student_id: id, status: draft.status, marks: null, components: values, reason: draft.reason.trim(), row_version: draft.base })
        sent.set(id, { ...draft })
        cells.current.set(id, { kind: 'saving' })
        continue
      }
      if (draft.status === 'present' || draft.status === 'withheld') {
        const parsed = parseCell(draft.text, fullMarks)
        if (parsed.kind === 'error') { cells.current.set(id, { kind: 'error', code: parsed.code }); localErrors++; continue }
        if (parsed.kind === 'empty' && draft.status === 'present') { cells.current.set(id, { kind: 'error', code: 'marks_required' }); localErrors++; continue }
        marks = parsed.kind === 'marks' ? parsed.value : null
      }
      if ((draft.status === 'exempt' || draft.status === 'withheld') && !draft.reason.trim()) { cells.current.set(id, { kind: 'error', code: 'reason_required' }); localErrors++; continue }
      entries.push({ exam_subject_id: sheetId, student_id: id, status: draft.status, marks, reason: draft.reason.trim(), row_version: draft.base })
      sent.set(id, { ...draft })
      cells.current.set(id, { kind: 'saving' })
    }
    if (entries.length === 0) {
      if (localErrors) setState('error')
      bump()
      return
    }
    inFlight.current = true
    setState('saving')
    bump()
    setRequests(n => n + 1)
    try {
      const result = await saveRef.current(entries)
      attempt.current = 0
      const failed: BatchReport['failed'] = []
      const apply = (id: string, cell: ServerCell) => {
        server.current.set(id, cell)
        const now = drafts.current.get(id), was = sent.get(id)
        if (now && was && now.status === was.status && now.text === was.text && now.reason === was.reason && JSON.stringify(now.parts || {}) === JSON.stringify(was.parts || {})) {
          drafts.current.delete(id)
          cells.current.set(id, { kind: 'saved', at: Date.now() })
        } else if (now) {
          // Edited again while saving: the new edit builds on what was saved.
          drafts.current.set(id, { ...now, base: cell.version })
          cells.current.delete(id)
        }
      }
      for (const c of result.saved) apply(c.student_id, { status: c.status, marks: c.marks, reason: c.reason, version: c.row_version, components: c.components })
      let blocked = false
      for (const e of result.errors) {
        const was = sent.get(e.student_id)
        if (e.code === 'version_conflict' && e.current && was) {
          const current: ServerCell = { status: e.current.status, marks: e.current.marks, reason: e.current.reason, version: e.current.row_version, components: e.current.components }
          if (sameValue(was, current, components)) { apply(e.student_id, current); continue } // an earlier attempt landed
          server.current.set(e.student_id, current)
          cells.current.set(e.student_id, { kind: 'conflict', server: current })
        } else {
          cells.current.set(e.student_id, { kind: 'error', code: e.code })
          if (e.code === 'sheet_locked' || e.code === 'not_assigned') blocked = true
        }
        failed.push({ student_id: e.student_id, code: e.code })
      }
      setReport({ saved: result.saved.length, failed })
      persist()
      const conflicts = [...cells.current.values()].some(c => c.kind === 'conflict')
      const errors = localErrors > 0 || [...cells.current.values()].some(c => c.kind === 'error')
      setState(blocked ? 'blocked' : conflicts ? 'conflict' : errors ? 'error' : 'saved')
      if (!errors && !conflicts) setSavedAt(Date.now())
      if (blocked) blockedRef.current()
    } catch (cause) {
      const api = cause instanceof ApiError ? cause : null
      const network = !api
      if (!api || api.status >= 500 || api.status === 429 || api.status === 408) {
        // Transient: the drafts stay pending and the batch is retried with
        // backoff, then every 30 s, and at once when the browser is online.
        for (const id of sent.keys()) cells.current.delete(id)
        setState(network ? 'offline' : 'error')
        schedule(backoff[Math.min(attempt.current, backoff.length - 1)])
        attempt.current++
      } else {
        // Permanent: the same request would fail again, so these cells wait
        // for the user (an edit retries them) instead of looping.
        const blocked = api.code === 'exam_not_draft' || api.status === 403 || api.status === 401
        for (const id of sent.keys()) cells.current.set(id, { kind: 'error', code: blocked ? 'sheet_locked' : 'save_failed' })
        setState(blocked ? 'blocked' : 'error')
        if (blocked) blockedRef.current()
      }
    } finally {
      inFlight.current = false
      bump()
      const waiting = [...drafts.current.keys()].some(id => !cells.current.has(id))
      if (waiting && timer.current === undefined) schedule()
    }
  }, [enabled, fullMarks, sheetId, persist, schedule, bump, components])
  const flushRef = useRef(flush)
  flushRef.current = flush

  // The browser reports the network back: retry at once.
  useEffect(() => {
    const online = () => { attempt.current = 0; void flushRef.current() }
    window.addEventListener('online', online)
    return () => window.removeEventListener('online', online)
  }, [])

  // Leaving with unsaved edits: the browser asks first. Drafts also stay in
  // sessionStorage for a reload of this tab.
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (drafts.current.size > 0 || inFlight.current) { event.preventDefault(); event.returnValue = '' }
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [])
  useEffect(() => () => window.clearTimeout(timer.current), [])

  const value = useCallback((id: string): CellValue => {
    const draft = drafts.current.get(id)
    if (draft) return { status: draft.status, text: draft.text, reason: draft.reason, parts: draft.parts || {} }
    const cell = server.current.get(id)
    return { status: cell?.status || '', text: textOf(cell?.marks ?? null), reason: cell?.reason || '', parts: partsOf(cell) }
  }, [])

  const edit = useCallback((id: string, patch: Partial<CellValue>) => {
    if (!enabled) return
    const cell = server.current.get(id)
    if (!cell) return
    const current = value(id)
    const base = drafts.current.get(id)?.base ?? cell.version
    const next: Draft = { status: ((patch.status ?? current.status) || 'present') as MarkStatus, text: patch.text ?? current.text, reason: patch.reason ?? current.reason, base }
    if (components.length > 0) {
      next.parts = patch.parts ? { ...current.parts, ...patch.parts } : current.parts
      next.text = sumParts(next.parts, components)
    }
    if (next.status !== 'present' && next.status !== 'withheld') { next.text = ''; if (next.parts) next.parts = {} }
    if (sameValue(next, cell, components) && base === cell.version) {
      drafts.current.delete(id)
      if (cells.current.get(id)?.kind !== 'conflict') cells.current.delete(id)
    } else {
      drafts.current.set(id, next)
      const state = cells.current.get(id)
      if (state && state.kind !== 'conflict') cells.current.delete(id)
    }
    persist()
    bump()
    schedule()
  }, [enabled, value, fullMarks, persist, bump, schedule, components])

  const revert = useCallback((id: string) => {
    drafts.current.delete(id)
    cells.current.delete(id)
    persist()
    bump()
  }, [persist, bump])

  // A conflict: take the server's value, or keep the user's on top of it.
  const resolve = useCallback((id: string, keep: 'mine' | 'theirs') => {
    const state = cells.current.get(id)
    if (state?.kind !== 'conflict') return
    server.current.set(id, state.server)
    cells.current.delete(id)
    const draft = drafts.current.get(id)
    if (keep === 'theirs' || !draft) drafts.current.delete(id)
    else drafts.current.set(id, { ...draft, base: state.server.version })
    persist()
    bump()
    if (keep === 'mine') schedule(0)
    else setState([...cells.current.values()].some(c => c.kind === 'conflict') ? 'conflict' : 'saved')
  }, [persist, bump, schedule])

  const restore = useCallback(() => {
    if (!recovery) return
    for (const [id, draft] of Object.entries(recovery.drafts)) if (server.current.has(id)) drafts.current.set(id, draft)
    setRecovery(null)
    persist()
    bump()
    schedule(0)
  }, [recovery, persist, bump, schedule])

  const discard = useCallback(() => {
    setRecovery(null)
    if (storageKey) try { sessionStorage.removeItem(storageKey) } catch { /* ignore */ }
  }, [storageKey])

  const flushNow = useCallback(() => { window.clearTimeout(timer.current); timer.current = undefined; void flushRef.current() }, [])
  const isDraft = useCallback((id: string) => drafts.current.has(id), [])
  const cell = useCallback((id: string) => cells.current.get(id), [])
  const serverCell = useCallback((id: string) => server.current.get(id), [])
  const clearReport = useCallback(() => setReport(null), [])

  return {
    state, savedAt, recovery, report, requests,
    pending: drafts.current.size,
    conflicts: [...cells.current.values()].filter(c => c.kind === 'conflict').length,
    errors: [...cells.current.values()].filter(c => c.kind === 'error').length,
    isDraft, cell, serverCell,
    value, edit, revert, resolve, restore, discard, flushNow, clearReport,
  }
}
