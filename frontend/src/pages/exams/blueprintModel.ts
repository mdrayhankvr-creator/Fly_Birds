// Pure helpers for the assessment blueprint builder: exact sums, term-rule
// presets ("CT 20 % + final 80 %", "best 2 of 3 CTs + final", "final only"),
// new assessment keys and validation issues by field. The server validates
// and calculates; these only shape the definition the admin edits.
import type { AssessmentKind, BlueprintAssessment, BlueprintComponent, BlueprintContribution, BlueprintDefinition, BlueprintIssue, BlueprintSheet, BlueprintSubject, BlueprintTermRule } from '../../api/client'

/** Marks as integer hundredths, so sums are exact (16.83 → 1683). */
export const hundredths = (v: number): number => Math.round(v * 100)
export const fromHundredths = (h: number): number => h / 100
export const sumMarks = (values: number[]): number => fromHundredths(values.reduce((t, v) => t + hundredths(v), 0))
export const twoDecimals = (v: number): string => (hundredths(v) / 100).toFixed(2)

/** A sheet's full marks: the sum of its components. */
export const sheetFull = (sheet: BlueprintSheet): number => sumMarks(sheet.components.map(c => c.full))

/** Rounds a pass mark to two decimals, up, so marks reaching it pass. */
export const passFor = (full: number, percent: number): number => Math.ceil(hundredths(full) * percent / 100) / 100

export type Preset = 'final_only' | 'ct_final' | 'best_ct_final' | 'custom'
export type PresetOptions = { ctWorth: number; finalWorth: number; best: number }
export const defaultPresetOptions: PresetOptions = { ctWorth: 20, finalWorth: 80, best: 2 }

const isCT = (a: BlueprintAssessment) => a.kind !== 'term_exam'

/** The assessments of one term, split into class tests (and similar) and term exams. */
export function termAssessments(def: BlueprintDefinition, termId: string) {
  const all = def.assessments.filter(a => a.term_id === termId)
  return { all, cts: all.filter(isCT), finals: all.filter(a => !isCT(a)) }
}

/** Recognises which preset a term rule is, or "custom". */
export function presetOf(rule: BlueprintTermRule | undefined, def: BlueprintDefinition): Preset {
  if (!rule) return 'custom'
  const { cts, finals } = termAssessments(def, rule.term_id)
  const c = rule.contributions
  const finalKeys = finals.map(a => a.key).sort().join(',')
  const ctKeys = cts.map(a => a.key).sort().join(',')
  if (c.length === 1 && c[0].combine === 'single' && finals.length === 1 && c[0].from[0] === finals[0].key) return 'final_only'
  if (c.length === 2 && c[1].combine === 'single' && c[1].from.join(',') === finalKeys && [...c[0].from].sort().join(',') === ctKeys && cts.length > 0) {
    if (c[0].combine === 'average') return 'ct_final'
    if (c[0].combine === 'best') return 'best_ct_final'
  }
  return 'custom'
}

/** Builds a term rule from a preset, passing at passPercent of the term full
 * marks. Returns null when the term lacks what the preset needs. */
export function presetRule(def: BlueprintDefinition, termId: string, preset: Exclude<Preset, 'custom'>, options: PresetOptions, passPercent = 33): BlueprintTermRule | null {
  const { cts, finals } = termAssessments(def, termId)
  if (finals.length !== 1) return null
  const final = finals[0].key
  if (preset === 'final_only') return { term_id: termId, full: 100, pass: passFor(100, passPercent), contributions: [{ from: [final], combine: 'single', worth: 100 }] }
  if (cts.length === 0) return null
  const full = sumMarks([options.ctWorth, options.finalWorth])
  const ct: BlueprintContribution = preset === 'ct_final'
    ? { from: cts.map(a => a.key), combine: 'average', worth: options.ctWorth }
    : { from: cts.map(a => a.key), combine: 'best', best: Math.min(Math.max(1, options.best), cts.length), worth: options.ctWorth }
  return { term_id: termId, full, pass: passFor(full, passPercent), contributions: [ct, { from: [final], combine: 'single', worth: options.finalWorth }] }
}

/** Ensures a subject has a sheet in every assessment its term rule draws from. */
export function withSheets(subject: BlueprintSubject, rule: BlueprintTermRule, def: BlueprintDefinition): BlueprintSubject {
  const keys = new Set(rule.contributions.flatMap(c => c.from))
  const sheets = [...subject.sheets]
  for (const key of keys) {
    if (sheets.some(s => s.assessment === key)) continue
    const a = def.assessments.find(x => x.key === key)
    const full = a && isCT(a) ? 20 : 100
    sheets.push({ assessment: key, pass: passFor(full, 33), components: [{ key: 'TOTAL', full }] })
  }
  // Sheets of this term that no contribution uses would be refused: drop them.
  const termKeys = new Set(def.assessments.filter(a => a.term_id === rule.term_id).map(a => a.key))
  return { ...subject, sheets: sheets.filter(s => !termKeys.has(s.assessment) || keys.has(s.assessment)) }
}

/** Sets one subject's rule for a term, adding the sheets it needs. */
export function setTermRule(subject: BlueprintSubject, rule: BlueprintTermRule, def: BlueprintDefinition): BlueprintSubject {
  const terms = subject.terms.some(t => t.term_id === rule.term_id)
    ? subject.terms.map(t => (t.term_id === rule.term_id ? rule : t))
    : [...subject.terms, rule]
  return withSheets({ ...subject, terms }, rule, def)
}

/** Copies one subject's sheets and rule for a term to every graded subject. */
export function applyToAll(def: BlueprintDefinition, fromSubject: string, termId: string): BlueprintDefinition {
  const source = def.subjects.find(s => s.subject_id === fromSubject)
  const rule = source?.terms.find(t => t.term_id === termId)
  if (!source || !rule) return def
  const termKeys = new Set(def.assessments.filter(a => a.term_id === termId).map(a => a.key))
  const sheets = source.sheets.filter(s => termKeys.has(s.assessment))
  return {
    ...def,
    subjects: def.subjects.map(s => {
      if (s.subject_id === fromSubject || s.role === 'non_graded') return s
      const kept = s.sheets.filter(x => !termKeys.has(x.assessment))
      const terms = s.terms.some(t => t.term_id === termId) ? s.terms.map(t => (t.term_id === termId ? structuredClone(rule) : t)) : [...s.terms, structuredClone(rule)]
      return { ...s, sheets: [...kept, ...structuredClone(sheets)], terms }
    }),
  }
}

/** The next free key for a new assessment of a kind: CT1, CT2 … or EX1 … */
export function nextKey(def: BlueprintDefinition, kind: AssessmentKind): string {
  const base = kind === 'term_exam' ? 'EX' : kind === 'class_test' ? 'CT' : kind === 'performance' ? 'PF' : 'CA'
  const used = new Set(def.assessments.map(a => a.key))
  for (let i = 1; i < 100; i++) if (!used.has(`${base}${i}`)) return `${base}${i}`
  return `${base}${Date.now() % 1000}`
}

/** Adds an assessment to a term with sensible defaults (class tests hidden). */
export function addAssessment(def: BlueprintDefinition, termId: string, kind: AssessmentKind, name: string): BlueprintDefinition {
  const key = nextKey(def, kind)
  const visibility = kind === 'term_exam' ? 'with_assessment_publication' : 'hidden'
  return { ...def, assessments: [...def.assessments, { key, name, term_id: termId, kind, visibility }] }
}

/** Removes an assessment and every sheet and contribution source that used it. */
export function removeAssessment(def: BlueprintDefinition, key: string): BlueprintDefinition {
  return {
    ...def,
    assessments: def.assessments.filter(a => a.key !== key),
    subjects: def.subjects.map(s => ({
      ...s,
      sheets: s.sheets.filter(x => x.assessment !== key),
      // A term rule left with nothing to draw from goes too.
      terms: s.terms.map(t => ({ ...t, contributions: t.contributions.map(c => ({ ...c, from: c.from.filter(f => f !== key) })).filter(c => c.from.length > 0) })).filter(t => t.contributions.length > 0),
    })),
  }
}

/** Component presets for a term exam sheet. */
export const componentPresets: Record<string, BlueprintComponent[]> = {
  total: [{ key: 'TOTAL', full: 100 }],
  cq_mcq: [{ key: 'CQ', full: 70, pass: 23 }, { key: 'MCQ', full: 30, pass: 10 }],
  cq_mcq_prac: [{ key: 'CQ', full: 50, pass: 17 }, { key: 'MCQ', full: 25, pass: 8 }, { key: 'PRAC', full: 25, pass: 8 }],
}

/** Issues whose path is a field or lies under it. */
export const issuesAt = (issues: BlueprintIssue[], path: string): BlueprintIssue[] =>
  issues.filter(i => i.path === path || i.path.startsWith(path + '.'))

/** The subject index an issue concerns, if any ("subjects.3.terms.0" → 3). */
export function issueSubject(issue: BlueprintIssue): number | null {
  const m = /^subjects\.(\d+)/.exec(issue.path)
  return m ? Number(m[1]) : null
}
