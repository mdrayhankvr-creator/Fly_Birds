// Pure input rules of the marks workspace: digits, cell values and pasted
// tables. No React, no network; unit-tested in scripts/marks-input.test.mjs.

export type MarkStatus = 'present' | 'absent' | 'exempt' | 'incomplete' | 'withheld'

const bnDigits = '০১২৩৪৫৬৭৮৯'

// toLatinDigits turns Bangla digits into ASCII so "৪৫.৫" parses as 45.5.
export function toLatinDigits(text: string): string {
  let out = ''
  for (const ch of text) {
    const i = bnDigits.indexOf(ch)
    out += i >= 0 ? String(i) : ch
  }
  return out
}

export type ParsedCell =
  | { kind: 'empty' }
  | { kind: 'marks'; value: number }
  | { kind: 'status'; status: 'absent' | 'incomplete' }
  | { kind: 'error'; code: 'not_a_number' | 'too_many_decimals' | 'negative' | 'above_full_marks' | 'needs_reason' }

const statusWords: Record<string, 'absent' | 'incomplete'> = {
  a: 'absent', ab: 'absent', abs: 'absent', absent: 'absent', 'অনুপস্থিত': 'absent', 'অ': 'absent',
  inc: 'incomplete', incomplete: 'incomplete', 'অসম্পূর্ণ': 'incomplete',
}
const reasonWords = new Set(['exempt', 'ex', 'withheld', 'অব্যাহতি', 'স্থগিত'])

// parseCell reads one typed or pasted value. Marks have at most two decimals
// and lie between 0 and full; a comma is accepted as the decimal separator
// when there is no point. Exempt and withheld need a reason, so they cannot
// come from a single cell.
export function parseCell(raw: string, full: number): ParsedCell {
  // Only the ends are trimmed (trim covers no-break and thin spaces): "4 5" is
  // ambiguous, so it is refused, not read as 45.
  const text = toLatinDigits(raw).trim()
  if (text === '') return { kind: 'empty' }
  const word = text.toLowerCase()
  if (statusWords[word]) return { kind: 'status', status: statusWords[word] }
  if (reasonWords.has(word)) return { kind: 'error', code: 'needs_reason' }
  const normal = !text.includes('.') && (text.match(/,/g) || []).length === 1 ? text.replace(',', '.') : text
  if (/^-\d/.test(normal)) return { kind: 'error', code: 'negative' }
  if (/^\d+\.\d{3,}$/.test(normal)) return { kind: 'error', code: 'too_many_decimals' }
  if (!/^\d+(\.\d{1,2})?$|^\.\d{1,2}$/.test(normal)) return { kind: 'error', code: 'not_a_number' }
  const value = Math.round(Number(normal) * 100) / 100
  if (Math.round(value * 100) > Math.round(full * 100)) return { kind: 'error', code: 'above_full_marks' }
  return { kind: 'marks', value }
}

// parseTable splits pasted spreadsheet text (tab-separated, as Excel and
// Google Sheets copy it) or a CSV file into rows of trimmed cells. Blank
// trailing lines are dropped.
export function parseTable(text: string): string[][] {
  const lines = text.replace(/\r\n?/g, '\n').split('\n')
  while (lines.length && lines[lines.length - 1].trim() === '') lines.pop()
  const tabbed = lines.some(line => line.includes('\t'))
  return lines.map(line => (tabbed ? line.split('\t') : splitCSV(line)).map(cell => cell.trim()))
}

function splitCSV(line: string): string[] {
  if (!line.includes(',') && !line.includes(';')) return [line]
  const sep = line.includes(';') && !line.includes(',') ? ';' : ','
  const out: string[] = []
  let cell = '', quoted = false
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (quoted) {
      if (ch === '"' && line[i + 1] === '"') { cell += '"'; i++ }
      else if (ch === '"') quoted = false
      else cell += ch
    } else if (ch === '"') quoted = true
    else if (ch === sep) { out.push(cell); cell = '' }
    else cell += ch
  }
  out.push(cell)
  return out
}

export type PasteTarget = { student_id: string; admission_no: string }

export type PlanItem = {
  line: number // 1-based line of the pasted text
  student_id: string | null
  raw: string
  parsed: ParsedCell
  // apply: a valid value; skip: blank or a header line; reject: see reason.
  result: 'apply' | 'skip' | 'reject'
  // no_student, duplicate_student, unknown_student, or a ParsedCell error code
  reason?: string
}

export type PastePlan = { mode: 'by_admission' | 'by_position'; items: PlanItem[] }

// planPaste maps pasted rows to students by stable id, never by what happens
// to be on screen later:
//   - by admission number when the first column of any row is the
//     admission number of a student of this sheet (other columns: the value is
//     the last non-empty cell);
//   - otherwise by position, starting at `start` in `targets` (the rows the
//     teacher sees, in order), one value per pasted row (the last non-empty
//     cell);
//   - a student named on more than one line gets none of them.
// Every planned item names its student id, and the preview shows the name.
export function planPaste(table: string[][], targets: PasteTarget[], start: number, full: number): PastePlan {
  const byAdmission = new Map(targets.map(t => [t.admission_no.trim().toLowerCase(), t.student_id]))
  const keyed = table.some(row => row.length > 1 && byAdmission.has((row[0] || '').toLowerCase()))
  // A student named on two lines is ambiguous: every one of those lines is
  // rejected, so nothing is guessed.
  const resolve = (row: string[], index: number) => keyed ? byAdmission.get((row[0] || '').toLowerCase()) ?? null : targets[start + index]?.student_id ?? null
  const lines = new Map<string, number>()
  table.forEach((row, index) => { const id = resolve(row, index); if (id) lines.set(id, (lines.get(id) ?? 0) + 1) })
  const items: PlanItem[] = []
  table.forEach((row, index) => {
    const line = index + 1
    const values = keyed ? row.slice(1) : row
    const raw = [...values].reverse().find(cell => cell !== '') ?? ''
    const parsed = parseCell(raw, full)
    const student = resolve(row, index)
    if (!student) {
      // keyed: a header line ("Admission no, Marks") or a stranger; by position: past the list.
      const reason = keyed ? 'unknown_student' : 'no_student'
      items.push({ line, student_id: null, raw, parsed, result: keyed && parsed.kind !== 'marks' && parsed.kind !== 'status' ? 'skip' : 'reject', reason })
      return
    }
    if ((lines.get(student) ?? 0) > 1) items.push({ line, student_id: student, raw, parsed, result: 'reject', reason: 'duplicate_student' })
    else if (parsed.kind === 'empty') items.push({ line, student_id: student, raw, parsed, result: 'skip' })
    else if (parsed.kind === 'error') items.push({ line, student_id: student, raw, parsed, result: 'reject', reason: parsed.code })
    else items.push({ line, student_id: student, raw, parsed, result: 'apply' })
  })
  return { mode: keyed ? 'by_admission' : 'by_position', items }
}

// isMultiCell tells a block paste from typing a single value.
export function isMultiCell(text: string): boolean {
  return /[\t\n\r]/.test(text.replace(/[\r\n]+$/, ''))
}
