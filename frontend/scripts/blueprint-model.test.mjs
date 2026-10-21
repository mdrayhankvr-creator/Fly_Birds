// Unit tests for the blueprint builder's pure helpers (run with Node's test
// runner and type stripping: npm run test:unit).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { addAssessment, applyToAll, passFor, presetOf, presetRule, removeAssessment, sheetFull, sumMarks, setTermRule } from '../src/pages/exams/blueprintModel.ts'

const base = () => ({
  assessments: [
    { key: 'CT1', name: 'CT 1', term_id: 'T1', kind: 'class_test', visibility: 'hidden' },
    { key: 'CT2', name: 'CT 2', term_id: 'T1', kind: 'class_test', visibility: 'hidden' },
    { key: 'CT3', name: 'CT 3', term_id: 'T1', kind: 'class_test', visibility: 'hidden' },
    { key: 'EX1', name: 'First Term', term_id: 'T1', kind: 'term_exam', visibility: 'with_assessment_publication' },
  ],
  subjects: [
    { subject_id: 'MATH', role: 'required', sheets: [], terms: [] },
    { subject_id: 'ENG', role: 'required', sheets: [], terms: [] },
    { subject_id: 'PE', role: 'non_graded', sheets: [], terms: [] },
  ],
  paper_rule: 'combined',
  annual: { method: 'final_only', final_must_pass: false },
  policy: { round_contributions: false, whole_marks: false, gpa_rounding: 'half_up', absent: 'zero', ranking: 'competition' },
})

test('sums and pass marks are exact', () => {
  assert.equal(sumMarks([0.1, 0.2]), 0.3)
  assert.equal(sumMarks([50, 25, 25]), 100)
  assert.equal(passFor(51, 33), 16.83)
  assert.equal(passFor(100, 33), 33)
  assert.equal(sheetFull({ assessment: 'EX1', pass: 33, components: [{ key: 'CQ', full: 50 }, { key: 'MCQ', full: 25 }, { key: 'PRAC', full: 25 }] }), 100)
})

test('presets build and are recognised: CT 20 + final 80, best 2 of 3, final only', () => {
  const def = base()
  const ct = presetRule(def, 'T1', 'ct_final', { ctWorth: 20, finalWorth: 80, best: 2 })
  assert.deepEqual(ct, { term_id: 'T1', full: 100, pass: 33, contributions: [{ from: ['CT1', 'CT2', 'CT3'], combine: 'average', worth: 20 }, { from: ['EX1'], combine: 'single', worth: 80 }] })
  assert.equal(presetOf(ct, def), 'ct_final')
  const best = presetRule(def, 'T1', 'best_ct_final', { ctWorth: 20, finalWorth: 80, best: 2 })
  assert.equal(best.contributions[0].best, 2)
  assert.equal(presetOf(best, def), 'best_ct_final')
  const final = presetRule(def, 'T1', 'final_only', { ctWorth: 20, finalWorth: 80, best: 2 })
  assert.equal(presetOf(final, def), 'final_only')
  // Best N never exceeds the number of class tests.
  assert.equal(presetRule(def, 'T1', 'best_ct_final', { ctWorth: 20, finalWorth: 80, best: 9 }).contributions[0].best, 3)
  // A changed worth is no longer the preset.
  assert.equal(presetOf({ ...ct, contributions: [ct.contributions[0], { ...ct.contributions[1], from: ['CT1'] }] }, def), 'custom')
})

test('a term rule brings the sheets it needs and drops unused ones', () => {
  const def = base()
  let math = setTermRule(def.subjects[0], presetRule(def, 'T1', 'ct_final', { ctWorth: 20, finalWorth: 80, best: 2 }), def)
  assert.deepEqual(math.sheets.map(s => [s.assessment, s.components[0].full, s.pass]), [['CT1', 20, 6.6], ['CT2', 20, 6.6], ['CT3', 20, 6.6], ['EX1', 100, 33]])
  math = setTermRule(math, presetRule(def, 'T1', 'final_only', { ctWorth: 20, finalWorth: 80, best: 2 }), def)
  assert.deepEqual(math.sheets.map(s => s.assessment), ['EX1'])
})

test('apply to all copies sheets and the rule to graded subjects only', () => {
  let def = base()
  def.subjects[0] = setTermRule(def.subjects[0], presetRule(def, 'T1', 'ct_final', { ctWorth: 20, finalWorth: 80, best: 2 }), def)
  def.subjects[0].sheets[3].components = [{ key: 'CQ', full: 70, pass: 23 }, { key: 'MCQ', full: 30, pass: 10 }]
  def = applyToAll(def, 'MATH', 'T1')
  assert.equal(def.subjects[1].sheets.length, 4)
  assert.equal(def.subjects[1].sheets[3].components[0].key, 'CQ')
  assert.equal(def.subjects[2].sheets.length, 0)
  // A copy, not a shared object.
  def.subjects[1].sheets[3].components[0].full = 60
  assert.equal(def.subjects[0].sheets[3].components[0].full, 70)
})

test('adding and removing assessments keeps the definition consistent', () => {
  let def = addAssessment(base(), 'T1', 'class_test', 'CT 4')
  assert.equal(def.assessments.at(-1).key, 'CT4')
  assert.equal(def.assessments.at(-1).visibility, 'hidden')
  def.subjects[0] = setTermRule(def.subjects[0], presetRule(def, 'T1', 'ct_final', { ctWorth: 20, finalWorth: 80, best: 2 }), def)
  def = removeAssessment(def, 'CT2')
  assert.ok(!def.subjects[0].sheets.some(s => s.assessment === 'CT2'))
  assert.deepEqual(def.subjects[0].terms[0].contributions[0].from, ['CT1', 'CT3', 'CT4'])
})
