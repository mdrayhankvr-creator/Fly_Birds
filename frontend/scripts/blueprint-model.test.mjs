// Unit tests for the blueprint builder's pure helpers (run with Node's test
// runner and type stripping: npm run test:unit).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { addAssessment, applyBlueprintPreset, copyFromClass, applyToAll, passFor, presetOf, presetRule, removeAssessment, sheetFull, sumMarks, setTermRule } from '../src/pages/exams/blueprintModel.ts'

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

test('a blueprint preset sets every graded subject in one click', () => {
  const def = base()
  const plain = applyBlueprintPreset(def, 'T1', 'cq_mcq', false)
  for (const s of plain.subjects.filter(x => x.role !== 'non_graded')) {
    const ex = s.sheets.find(x => x.assessment === 'EX1')
    assert.deepEqual(ex.components.map(c => `${c.key}:${c.full}`), ['CQ:70', 'MCQ:30'])
    assert.equal(ex.pass, 33)
    assert.equal(presetOf(s.terms[0], plain), 'final_only')
    assert.equal(s.sheets.length, 1, 'no class-test sheets without class tests')
  }
  assert.deepEqual(plain.subjects[2], def.subjects[2], 'a non-graded subject is untouched')
  const ct = applyBlueprintPreset(def, 'T1', 'cq_mcq_prac', true)
  const math = ct.subjects[0]
  assert.equal(presetOf(math.terms[0], ct), 'ct_final')
  assert.deepEqual(math.terms[0].contributions.map(c => c.worth), [20, 80])
  assert.deepEqual(math.sheets.find(x => x.assessment === 'EX1').components.map(c => c.key), ['CQ', 'MCQ', 'PRAC'])
  assert.equal(math.sheets.filter(x => x.assessment.startsWith('CT')).length, 3)
})

test('a class-test preset adds a class test when the term has none', () => {
  const def = base()
  def.assessments = def.assessments.filter(a => a.kind === 'term_exam')
  const next = applyBlueprintPreset(def, 'T1', 'cq_mcq', true)
  assert.equal(next.assessments.filter(a => a.kind === 'class_test').length, 1)
  assert.equal(presetOf(next.subjects[1].terms[0], next), 'ct_final')
  assert.equal(def.assessments.length, 1, 'the original is not changed')
  assert.equal(applyBlueprintPreset({ ...def, assessments: [] }, 'T1', 'cq_mcq', false), null)
})

test('copying another class takes its structure and the shared subjects', () => {
  const source = applyBlueprintPreset(base(), 'T1', 'cq_mcq', true)
  source.policy.ranking = 'dense'
  const target = base()
  target.assessments = [{ key: 'EX1', name: 'First Term', term_id: 'T1', kind: 'term_exam', visibility: 'with_assessment_publication' }]
  target.subjects = [{ subject_id: 'MATH', role: 'required', sheets: [], terms: [] }, { subject_id: 'BIO', role: 'required', sheets: [{ assessment: 'OLD1', pass: 7, components: [{ key: 'TOTAL', full: 20 }] }], terms: [] }]
  const { def, copied } = copyFromClass(target, source)
  assert.equal(copied, 1)
  assert.equal(def.policy.ranking, 'dense')
  assert.deepEqual(def.assessments.map(a => a.key), source.assessments.map(a => a.key))
  assert.deepEqual(def.subjects[0].terms, source.subjects[0].terms)
  assert.deepEqual(def.subjects[1].sheets, [], 'a subject the other class lacks loses sheets of unknown assessments')
  assert.equal(target.subjects[0].terms.length, 0, 'the draft passed in is not changed')
})
