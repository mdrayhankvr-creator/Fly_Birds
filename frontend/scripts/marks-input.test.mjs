// Unit tests for the marks workspace input rules (src/pages/marks/marksInput.ts).
// Run with: node --experimental-strip-types --test scripts/marks-input.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { isMultiCell, parseCell, parseTable, parseComponents, planPaste, toLatinDigits } from '../src/pages/marks/marksInput.ts'

test('Bangla digits become ASCII', () => {
  assert.equal(toLatinDigits('৪৫.৫০'), '45.50')
  assert.equal(toLatinDigits('Class ১০'), 'Class 10')
})

test('cell values: marks, statuses and errors', () => {
  const cases = [
    ['45', { kind: 'marks', value: 45 }],
    ['45.5', { kind: 'marks', value: 45.5 }],
    ['৪৫.৫', { kind: 'marks', value: 45.5 }],
    ['16.83', { kind: 'marks', value: 16.83 }],
    ['45,5', { kind: 'marks', value: 45.5 }],
    [' 50 ', { kind: 'marks', value: 50 }],
    ['0', { kind: 'marks', value: 0 }],
    ['', { kind: 'empty' }],
    ['A', { kind: 'status', status: 'absent' }],
    ['Absent', { kind: 'status', status: 'absent' }],
    ['অনুপস্থিত', { kind: 'status', status: 'absent' }],
    ['inc', { kind: 'status', status: 'incomplete' }],
    ['অসম্পূর্ণ', { kind: 'status', status: 'incomplete' }],
    ['50.01', { kind: 'error', code: 'above_full_marks' }],
    ['51', { kind: 'error', code: 'above_full_marks' }],
    ['33.333', { kind: 'error', code: 'too_many_decimals' }],
    ['-1', { kind: 'error', code: 'negative' }],
    ['forty', { kind: 'error', code: 'not_a_number' }],
    ['4 5', { kind: 'error', code: 'not_a_number' }],
    ['1e2', { kind: 'error', code: 'not_a_number' }],
    ['exempt', { kind: 'error', code: 'needs_reason' }],
    ['withheld', { kind: 'error', code: 'needs_reason' }],
  ]
  for (const [raw, want] of cases) assert.deepEqual(parseCell(raw, 50), want, raw)
  // Exactness at the full marks: 0.1 + 0.2 style float noise must not matter.
  assert.deepEqual(parseCell('16.83', 16.83), { kind: 'marks', value: 16.83 })
  assert.deepEqual(parseCell('16.84', 16.83), { kind: 'error', code: 'above_full_marks' })
})

test('tables from Excel, Google Sheets and CSV', () => {
  assert.deepEqual(parseTable('45\n38.5\nA\n'), [['45'], ['38.5'], ['A']])
  assert.deepEqual(parseTable('DEMO-001\t45\r\nDEMO-002\t38.5\r\n\r\n'), [['DEMO-001', '45'], ['DEMO-002', '38.5']])
  assert.deepEqual(parseTable('"DEMO-001","Ayesha, H",45\nDEMO-002,Rahim,38'), [['DEMO-001', 'Ayesha, H', '45'], ['DEMO-002', 'Rahim', '38']])
  assert.deepEqual(parseTable('DEMO-001;45'), [['DEMO-001', '45']])
  assert.equal(isMultiCell('45'), false)
  assert.equal(isMultiCell('45\n'), false)
  assert.equal(isMultiCell('45\n46'), true)
  assert.equal(isMultiCell('DEMO-001\t45'), true)
})

const targets = [
  { student_id: 's1', admission_no: 'DEMO-001' },
  { student_id: 's2', admission_no: 'DEMO-002' },
  { student_id: 's3', admission_no: 'DEMO-003' },
]

test('paste by admission number ignores row order and headers', () => {
  const plan = planPaste(parseTable('Admission\tName\tMarks\nDEMO-003\tNusrat\t40\nDEMO-001\tAyesha\t45.5\nDEMO-999\tStranger\t30\nDEMO-001\tAgain\t12\nDEMO-002\tRahim\t51'), targets, 0, 50)
  assert.equal(plan.mode, 'by_admission')
  const byStudent = Object.fromEntries(plan.items.filter(i => i.student_id).map(i => [i.student_id + ':' + i.line, i]))
  assert.equal(plan.items[0].result, 'skip') // header
  assert.equal(byStudent['s3:2'].result, 'apply')
  assert.deepEqual(byStudent['s3:2'].parsed, { kind: 'marks', value: 40 })
  // DEMO-001 appears twice: neither line is applied.
  assert.equal(byStudent['s1:3'].result, 'reject')
  assert.equal(byStudent['s1:3'].reason, 'duplicate_student')
  assert.equal(byStudent['s1:5'].reason, 'duplicate_student')
  assert.equal(plan.items[3].result, 'reject')
  assert.equal(plan.items[3].reason, 'unknown_student')
  assert.equal(byStudent['s2:6'].reason, 'above_full_marks')
})

test('paste by position starts at the focused row and never runs past the list', () => {
  const plan = planPaste(parseTable('40\n\nA\n30'), targets, 1, 50)
  assert.equal(plan.mode, 'by_position')
  assert.deepEqual(plan.items.map(i => [i.student_id, i.result]), [['s2', 'apply'], ['s3', 'skip'], [null, 'reject'], [null, 'reject']])
  assert.equal(plan.items[2].reason, 'no_student')
})

test('a component sheet pastes one value per component, in order', () => {
  const parts = [{ key: 'CQ', full: 50 }, { key: 'MCQ', full: 25 }, { key: 'PRAC', full: 25 }]
  const targets = [{ student_id: 's1', admission_no: 'A-1' }, { student_id: 's2', admission_no: 'A-2' }, { student_id: 's3', admission_no: 'A-3' }, { student_id: 's4', admission_no: 'A-4' }]
  const table = parseTable('A-2\tNila\t40\t20\t21\nA-1\t45\t25\t25\nA-3\t51\t20\t20\nA-4\t80\nA-9\t1\t2\t3')
  const plan = planPaste(table, targets, 0, 100, parts)
  assert.equal(plan.mode, 'by_admission')
  assert.deepEqual(plan.items[0].parsed, { kind: 'components', values: { CQ: 40, MCQ: 20, PRAC: 21 } }) // the name column is ignored
  assert.equal(plan.items[0].student_id, 's2')
  assert.deepEqual(plan.items[1].parsed, { kind: 'components', values: { CQ: 45, MCQ: 25, PRAC: 25 } })
  assert.equal(plan.items[2].reason, 'above_full_marks') // CQ 51 of 50
  assert.equal(plan.items[3].reason, 'components_required') // one total cannot be split
  assert.equal(plan.items[4].reason, 'unknown_student')
  assert.deepEqual(parseComponents(['ab'], parts, 100), { kind: 'status', status: 'absent' })
  assert.deepEqual(parseComponents(['', ''], parts, 100), { kind: 'empty' })
})
