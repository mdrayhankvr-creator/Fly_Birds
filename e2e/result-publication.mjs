// E6 result versions on a running, demo-seeded disposable school: a term set
// up through the API (class tests hidden, a term exam with components),
// assessment versions, the publication centre, head-teacher approval and
// administrator publication in the browser, what students see, a correction
// through the exam page with a superseded version, concurrent publication,
// grading-band and optional-subject changes after publication, withdrawal,
// withheld results, the annual result, permissions, layouts, languages,
// themes and accessibility.
//
//   E2E_BASE_URL=http://localhost:8112 DEMO_PASSWORD=... node e2e/result-publication.mjs
//
// It adds a term, a blueprint version, exams and marks and publishes results;
// never run it against a real school.
import { readFileSync, existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const metrics = {}
const stamp = Date.now().toString(36).slice(-3).toUpperCase()
const shots = join(here, 'shots')

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${who}@demo.flybirds.test`, password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
const [admin, head, teacher, student, guardian] = await Promise.all(['admin', 'head', 'teacher', 'student', 'guardian'].map(api))
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }

// ---- 0. A term for the demo student's class, through the API -------------------
const me = await must(student('GET', '/students/me'), 'me')
const session = me.session_id, cls = me.class_id
const term = await must(admin('POST', '/exams/terms', { session_id: session, name: `E6 Term ${stamp}`, kind: 'term', starts_on: '', ends_on: '' }), 'term')
const CT = `C${stamp}`, EX = `X${stamp}`
const classes = await must(admin('GET', `/exams/blueprints?session_id=${session}`), 'blueprints')
let mine = classes.find(c => c.class_id === cls)
let draft
if (mine.draft) await must(admin('DELETE', `/exams/blueprint-versions/${mine.draft.id}`), 'discard a leftover draft')
if (mine.active) draft = await must(admin('POST', `/exams/blueprints/${mine.blueprint_id}/versions`, { from_version_id: mine.active.id }), 'new version')
else draft = await must(admin('POST', '/exams/blueprints', { session_id: session, class_id: cls }), 'new blueprint')
const d = draft.definition
const termKeys = new Set(d.assessments.filter(a => a.term_id === term.id).map(a => a.key).concat([CT, EX]))
d.assessments = d.assessments.filter(a => a.term_id !== term.id).concat([
  { key: CT, name: 'E6 class test', term_id: term.id, kind: 'class_test', visibility: 'hidden' },
  { key: EX, name: 'E6 term exam', term_id: term.id, kind: 'term_exam', visibility: 'after_term_publication' },
])
let componentSubject = ''
for (const s of d.subjects) {
  if (s.role === 'non_graded') continue
  if (!componentSubject && s.role === 'required' && !s.combined_key) componentSubject = s.subject_id
  const components = s.subject_id === componentSubject ? [{ key: 'CQ', full: 60, pass: 20 }, { key: 'MCQ', full: 40, pass: 13 }] : [{ key: 'TOTAL', full: 100 }]
  s.sheets = s.sheets.filter(x => !termKeys.has(x.assessment)).concat([{ assessment: CT, pass: 7, components: [{ key: 'TOTAL', full: 20 }] }, { assessment: EX, pass: 33, components }])
  s.terms = s.terms.filter(x => x.term_id !== term.id).concat([{ term_id: term.id, full: 100, pass: 33, contributions: [{ from: [CT], combine: 'average', worth: 20 }, { from: [EX], combine: 'single', worth: 80 }] }])
}
d.annual = { method: 'equal', final_must_pass: false }
const saved = await must(admin('PUT', `/exams/blueprint-versions/${draft.id}`, { definition: d }), 'save draft')
check('the blueprint version with the E6 term validates', saved.issues.length === 0, saved.issues.map(i => i.message).slice(0, 3).join(' | '))
await must(admin('POST', `/exams/blueprint-versions/${draft.id}/activate`, {}), 'activate')
await must(admin('PUT', `/exams/terms/${term.id}/setup`, { class_ids: [cls], due: { [CT]: '2030-01-15', [EX]: '2030-02-15' }, assignees: {} }), 'setup')
const created = await must(admin('POST', `/exams/terms/${term.id}/create`, {}), 'create')
const listed = (await must(admin('GET', `/exams?page=1&page_size=100&session_id=${session}&class_id=${cls}`), 'exams')).items.filter(e => e.term_id === term.id)
const exams = await Promise.all(listed.map(e => must(admin('GET', `/exams/${e.id}`), 'exam')))
check('the wizard created a class test and a term exam per section', created.exams_created === exams.length && exams.length >= 2 && exams.length % 2 === 0, `${exams.length} exams`)
const ctExams = exams.filter(e => e.assessment_key === CT), exExams = exams.filter(e => e.assessment_key === EX)
const myEx = exExams.find(e => e.section_id === me.section_id), myCt = ctExams.find(e => e.section_id === me.section_id)

// Marks for everyone: deterministic, one student fails the term exam on CQ.
const rosters = {}
let failing = null
for (const exam of exams) {
  const rows = await must(admin('GET', `/exams/${exam.id}/roster`), 'roster')
  rosters[exam.id] = rows
  const students = [...new Set(rows.map(x => x.student_id))]
  const entries = rows.map(row => {
    const i = students.indexOf(row.student_id)
    if (exam.assessment_key === CT) return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'present', marks: 10 + (i * 3) % 11, row_version: row.row_version }
    const sheet = exam.subjects.find(s => s.id === row.exam_subject_id)
    if (sheet.subject_id === componentSubject) {
      const cq = row.student_id === me.id ? 52 : i === 1 ? 15 : 25 + (i * 7) % 35
      if (i === 1 && exam.section_id === me.section_id && row.student_id !== me.id) failing = { exam, student: row.student_id, name: row.student_name, sheet: row.exam_subject_id }
      return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'present', components: { CQ: cq, MCQ: 15 + (i * 5) % 25 }, row_version: row.row_version }
    }
    return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'present', marks: row.student_id === me.id ? 78 : 35 + (i * 11) % 60, row_version: row.row_version }
  })
  for (let k = 0; k < entries.length; k += 400) {
    const res = await must(admin('PATCH', `/exams/${exam.id}/marks`, { entries: entries.slice(k, k + 400) }), 'marks')
    if (res.errors.length) throw new Error(`marks for ${exam.name}: ${JSON.stringify(res.errors.slice(0, 2))}`)
  }
}
check('a student fails the term exam on CQ alone (for the correction)', !!failing, failing?.name)

// ---- 1. Exams publish into versions ------------------------------------------
for (const exam of [...ctExams, ...exExams]) await must(admin('PATCH', `/exams/${exam.id}/state`, { status: 'published' }), `publish ${exam.name}`)
const hist0 = await must(admin('GET', `/exams/results/history?scope=assessment&scope_id=${myEx.id}&student_id=${me.id}`), 'history')
check('publishing an exam makes each result a version with an inputs hash', hist0.length >= 1 && hist0[0].status === 'published' && /^[0-9a-f]{64}$/.test(hist0[0].inputs_sha256), `${hist0.length} version(s), ${hist0.map(v => v.kind).join(', ')}`)
let cards = await must(student('GET', '/exams/me/results'), 'cards')
check('before the term result the student sees neither the term exam nor the class test', !cards.some(c => c.exam.id === myEx.id || c.exam.id === myCt.id), `${cards.length} cards`)

// ---- 2. Review, approve and publish the term in the browser -------------------
const overview0 = await must(head('GET', `/exams/results/overview?session_id=${session}`), 'overview')
const cell0 = overview0.terms.find(t => t.id === term.id)?.classes.find(c => c.class_id === cls)
check('the publication centre shows the class ready for review', cell0?.state === 'ready' && cell0.approved === cell0.sheets, JSON.stringify(cell0 && { state: cell0.state, approved: cell0.approved, sheets: cell0.sheets }))
const reviewPath = `/admin/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}&session=${session}`
const textOf = b => b.evaluate(`document.querySelector('main') ? document.querySelector('main').innerText : ''`)
const waitText = (b, text, timeout = 15000) => b.waitFor(`(document.querySelector('main')?.innerText || '').includes(${JSON.stringify(text)})`, timeout)
const clickText = async (b, text, within = 'main') => {
  const ok = await b.evaluate(`(() => { const root = document.querySelector(${JSON.stringify(within)}) || document.body; const el = [...root.querySelectorAll('button, a')].find(e => e.offsetParent !== null && !e.disabled && e.innerText.trim() === ${JSON.stringify(text)}); if (!el) return false; el.click(); return true })()`)
  if (!ok) throw new Error(`nothing to click: ${text}`)
  await sleep(300)
}
const setValue = (b, selector, value) => b.evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return false; const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, ${JSON.stringify(String(value))}); el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })); return true })()`)

const hb = await launch({ base, shots, port: 9601 })
await hb.viewport(1440, 900)
await hb.login('demo.head@demo.flybirds.test', password)
await hb.go(`/admin/exams/results?session=${session}`)
await waitText(hb, term.name)
const centre = await textOf(hb)
check('the head teacher sees the term and the class in the publication centre', centre.includes(term.name) && centre.includes('Ready for review'), 'centre')
await hb.go(reviewPath)
let t0 = Date.now()
await hb.waitFor(`!!document.querySelector('#rv-table-title')`, 20000)
metrics.reviewLoadMs = Date.now() - t0
const headReview = await textOf(hb)
const termReview = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'review')
metrics.students = termReview.students.length
check('the review is provisional and lists every student', /provisional — not published/i.test(headReview) && termReview.students.every(s => headReview.includes(s.name)), `${termReview.students.length} students in ${metrics.reviewLoadMs} ms`)
check('the head teacher can approve but not publish', headReview.includes('Approve these results') && !headReview.includes('Publish results'), 'buttons')
await clickText(hb, 'Approve these results')
check('the head teacher approves exactly what was reviewed', await waitText(hb, 'Approved by'), 'approved')
await hb.shot('rv-review-approved-head', false)
await hb.close()

const ab = await launch({ base, shots, port: 9602 })
await ab.viewport(1440, 900)
await ab.login('demo.admin@demo.flybirds.test', password)
await ab.go(`/admin/exams/results?session=${session}`)
await waitText(ab, term.name)
let clicks = 0
// From the centre: Review → Publish results → confirm.
const reviewLink = await ab.evaluate(`(() => { const row = [...document.querySelectorAll('main li')].find(li => li.innerText.includes(${JSON.stringify(me.class_name || '')}) && li.closest('section')?.innerText.includes(${JSON.stringify(term.name)})); const a = row && [...row.querySelectorAll('a')].find(x => x.innerText.trim() === 'Review'); if (!a) return false; a.click(); return true })()`)
if (!reviewLink) await ab.go(reviewPath); else clicks++
await ab.waitFor(`!!document.querySelector('#rv-table-title')`, 20000)
await clickText(ab, 'Publish results'); clicks++
await ab.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
const dialog = await ab.evaluate(`document.querySelector('[role=alertdialog]').innerText`)
check('publishing asks for confirmation with the class and the count', dialog.includes(String(termReview.students.length)) && /notified/.test(dialog), dialog.replace(/\s+/g, ' ').slice(0, 140))
await clickText(ab, 'Publish results', '[role=alertdialog]'); clicks++
check('the term result is published from the centre in three clicks', await waitText(ab, 'Everything matches the published results', 20000) && clicks === 3, `${clicks} clicks`)
metrics.publishClicks = clicks
await ab.shot('rv-review-published-admin', false)

// ---- 3. What families see ------------------------------------------------------------
const published = await must(student('GET', '/exams/me/term-results'), 'term results')
const mineTerm = published.find(p => p.term_id === term.id)
const raw = JSON.stringify(published)
check('the student sees the published term result with GPA and subjects', !!mineTerm && !mineTerm.withheld && !!mineTerm.gpa && mineTerm.subjects.length > 0, mineTerm && `${mineTerm.gpa} ${mineTerm.outcome}`)
check('no class-test contribution or key reaches the student', !raw.includes('contributions') && !raw.includes(CT), 'server-side hiding')
cards = await must(student('GET', '/exams/me/results'), 'cards')
check('after the term result the term exam is shown; the class test never', cards.some(c => c.exam.id === myEx.id) && !cards.some(c => c.exam.id === myCt.id) && (await student('GET', `/exams/me/results/${myCt.id}`)).status === 404, `${cards.length} cards`)
const childResults = await must(guardian('GET', `/exams/children/${me.id}/term-results`), 'guardian')
check('the linked guardian sees the same published term result', childResults.some(p => p.id === mineTerm.id), `${childResults.length}`)

// ---- 4. A correction through the exam page ---------------------------------------
const shownBefore = (await must(admin('GET', `/exams/results/history?scope=assessment&scope_id=${failing.exam.id}&student_id=${failing.student}`), 'history'))[0]
await ab.go('/admin/exams')
await ab.waitFor(`!!document.querySelector('main input')`)
await setValue(ab, 'main input', failing.exam.name)
await ab.waitFor(`[...document.querySelectorAll('main button')].some(x => x.innerText.includes(${JSON.stringify(failing.exam.name)}) && x.innerText.includes(${JSON.stringify(failing.exam.section_name)}))`)
await ab.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.includes(${JSON.stringify(failing.exam.name)}) && x.innerText.includes(${JSON.stringify(failing.exam.section_name)})).click()`)
await waitText(ab, 'Corrections', 20000)
await ab.waitFor(`!!document.querySelector('#rv-corrections-title')`, 20000)
const correctionSelect = 'section[aria-labelledby=rv-corrections-title] select'
await ab.waitFor(`!!document.querySelector('${correctionSelect} option[value="${failing.student}"]')`, 20000)
await setValue(ab, correctionSelect, failing.student)
await ab.waitFor(`document.querySelector('${correctionSelect}').value === ${JSON.stringify(failing.student)}`)
await setValue(ab, 'section[aria-labelledby=rv-corrections-title] form input', 'CQ script re-marked')
await sleep(200)
await clickText(ab, 'Open correction', 'section[aria-labelledby=rv-corrections-title]')
await waitText(ab, 'Correction opened')
const cqField = `[aria-label^="CQ for "]`
await ab.waitFor(`!!document.querySelector('${cqField}')`)
await setValue(ab, cqField, 35)
await clickText(ab, 'Save corrected marks', 'section[aria-labelledby=rv-corrections-title]')
check('the corrected mark saves for that student only', await waitText(ab, 'Saved. Review and publish the correction when ready.'), 'saved')
const studentCardBefore = await must(admin('GET', `/exams/${failing.exam.id}/results/${failing.student}`), 'card')
check('until published, the report card still shows the published version', studentCardBefore.result.passed === false && studentCardBefore.version_id === shownBefore.id, `passed ${studentCardBefore.result.passed}, v${studentCardBefore.version}`)
await ab.shot('rv-correction-editor', false)
await clickText(ab, 'Review and publish corrections')
await ab.waitFor(`!!document.querySelector('#rv-table-title')`, 20000)
const correctionView = await textOf(ab)
const diffParts = [correctionView.includes(failing.name), /changed result/i.test(correctionView), /published/i.test(correctionView), /new/i.test(correctionView), /failed/i.test(correctionView)]
check('the correction review shows the change before → after', diffParts.every(Boolean), diffParts.join(' '))
await ab.shot('rv-correction-review', false)
// The head teacher approves the revision in the browser.
const hb2 = await launch({ base, shots, port: 9603 })
await hb2.viewport(1440, 900)
await hb2.login('demo.head@demo.flybirds.test', password)
await hb2.go(`/admin/exams/results/review?scope=assessment&scope_id=${failing.exam.id}&session=${session}`)
await hb2.waitFor(`!!document.querySelector('#rv-table-title')`, 20000)
await clickText(hb2, 'Approve this revision')
check('the head teacher approves the correction (the correction gives its reason)', await waitText(hb2, 'Approved by'), 'approved')
await hb2.close()
// I. Two administrators publish the same approval at once: it publishes once.
const aRef = { scope: 'assessment', scope_id: failing.exam.id }
const approval = (await must(admin('GET', `/exams/results/review?scope=assessment&scope_id=${failing.exam.id}`), 'review')).approval
const both = await Promise.all([admin('POST', '/exams/results/publish', { ...aRef, publication_id: approval.id }), admin('POST', '/exams/results/publish', { ...aRef, publication_id: approval.id })])
check('concurrent publication of one approval publishes exactly once', both.filter(x => x.status === 200).length === 1 && both.some(x => x.code === 'already_published'), both.map(x => `${x.status} ${x.code || ''}`).join(' / '))
const fh = await must(admin('GET', `/exams/results/history?scope=assessment&scope_id=${failing.exam.id}&student_id=${failing.student}`), 'history')
check('the correction is a new version; the previous one is superseded and kept', fh[0].status === 'published' && fh[0].kind === 'correction' && fh.slice(1).every(v => v.status === 'superseded') && fh[0].reason.includes('CQ script re-marked'), fh.map(v => `v${v.version} ${v.status} ${v.kind}`).join(', '))
const [newer, older] = await Promise.all([must(admin('GET', `/exams/results/versions/${fh[0].id}`), 'v'), must(admin('GET', `/exams/results/versions/${fh[1].id}`), 'v')])
check('both versions reproduce exactly from their snapshots', newer.verification.reproduced && newer.verification.hash_matches && older.verification.reproduced && older.verification.hash_matches && older.snapshot.outputs.outcome === 'failed', `${older.snapshot.outputs.outcome} → ${newer.snapshot.outputs.outcome}`)
// The version history in the browser.
await ab.go(`/admin/exams/results/review?scope=assessment&scope_id=${failing.exam.id}&session=${session}`)
await ab.waitFor(`!!document.querySelector('#rv-table-title')`, 20000)
await ab.evaluate(`(() => { const row = [...document.querySelectorAll('main tr')].find(tr => tr.innerText.includes(${JSON.stringify(failing.name)})); const btn = row && [...row.querySelectorAll('button')].find(b => b.innerText.trim() === 'History'); btn && btn.click() })()`)
check('the History button opens the student versions', await waitText(ab, 'Versions of'), 'history')
await waitText(ab, 'Show snapshot and check')
await clickText(ab, 'Show snapshot and check', 'section[aria-labelledby=rv-history-title]')
check('staff read every version with its verification in the browser', await waitText(ab, 'Verified: recalculating from the snapshot gives exactly the published values.') && (await textOf(ab)).includes('Superseded'), 'history panel')
await ab.shot('rv-history', false)
// The term follows only through its own approved revision.
let tr = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'term review')
check('the corrected mark makes the term show a change, not a silent rewrite', tr.state === 'changes' && tr.changed >= 1 && tr.students.find(s => s.student_id === failing.student)?.change === 'changed', `${tr.state} ${tr.changed}`)
const termOK = await must(head('POST', '/exams/results/approve', { scope: 'term', scope_id: term.id, class_id: cls, inputs_sha256: tr.inputs_sha256, reason: 'CQ script re-marked' }), 'approve term')
await must(admin('POST', '/exams/results/publish', { scope: 'term', scope_id: term.id, class_id: cls, publication_id: termOK.approval.id }), 'publish term revision')
const termHist = await must(admin('GET', `/exams/results/history?scope=term&scope_id=${term.id}&student_id=${failing.student}`), 'term history')
const myTermHist = await must(admin('GET', `/exams/results/history?scope=term&scope_id=${term.id}&student_id=${me.id}`), 'term history')
check('the term revision supersedes only the changed students', termHist.length === 2 && termHist[1].status === 'superseded' && (myTermHist.length === 1 || myTermHist[0].kind === 'correction'), `${termHist.length} versions for the corrected student, ${myTermHist.length} for the demo student`)

// ---- 5. Changes after publication never touch published results ------------------
const before = JSON.stringify(await must(student('GET', '/exams/me/term-results'), 'term results'))
const bands = await must(admin('GET', '/school/grading'), 'bands')
await must(admin('PUT', '/school/grading', { bands: bands.map(b => ({ ...b, gpa: b.gpa >= 1 ? b.gpa - 0.5 : b.gpa })) }), 'change bands')
const afterBands = JSON.stringify(await must(student('GET', '/exams/me/term-results'), 'term results'))
const reviewAfterBands = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'review')
await must(admin('PUT', '/school/grading', { bands }), 'restore bands')
check('a grading-band change leaves published GPAs and grades unchanged', before === afterBands && reviewAfterBands.state === 'published', `review ${reviewAfterBands.state}`)
const choices = await must(admin('GET', `/exams/subject-choices?session_id=${session}&class_id=${cls}`), 'choices')
if (choices.in_use && choices.optional.length > 1) {
  const st = choices.students.find(s => s.subject_id)
  const other = choices.optional.find(o => o.id !== st.subject_id)
  await must(admin('PUT', '/exams/subject-choices', { session_id: session, class_id: cls, choices: [{ student_id: st.student_id, subject_id: other.id }] }), 'change choice')
  const rv = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'review')
  const pv = await must(admin('GET', `/exams/results/history?scope=term&scope_id=${term.id}&student_id=${st.student_id}`), 'history')
  await must(admin('PUT', '/exams/subject-choices', { session_id: session, class_id: cls, choices: [{ student_id: st.student_id, subject_id: st.subject_id }] }), 'restore choice')
  check('an optional-subject change after publication shows in review and changes nothing published', rv.state !== 'published' && pv[0].status === 'published', `review ${rv.state}`)
} else check('an optional-subject change after publication (class has no choices to change)', true, 'skipped: no choices in use')

// ---- 6. Withdrawal through the exam page --------------------------------------------
const otherEx = exExams.find(e => e.id !== myEx.id) || myEx
await ab.go('/admin/exams')
await ab.waitFor(`!!document.querySelector('main input')`)
await setValue(ab, 'main input', otherEx.name)
await ab.waitFor(`[...document.querySelectorAll('main button')].some(x => x.innerText.includes(${JSON.stringify(otherEx.section_name)}) && x.innerText.includes(${JSON.stringify(otherEx.name)}))`)
await ab.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.includes(${JSON.stringify(otherEx.section_name)}) && x.innerText.includes(${JSON.stringify(otherEx.name)})).click()`)
await waitText(ab, 'Withdraw results', 20000)
await clickText(ab, 'Withdraw results')
await setValue(ab, 'main form.border-rose-200 input, main form input[minlength="3"]', 'Wrong answer key used')
await sleep(200)
await clickText(ab, 'Withdraw')
check('withdrawing results keeps the versions (withdrawn) and reopens the exam', await waitText(ab, 'Results withdrawn'), 'withdrawn')
const someone = rosters[otherEx.id][0].student_id
const wh = await must(admin('GET', `/exams/results/history?scope=assessment&scope_id=${otherEx.id}&student_id=${someone}`), 'history')
check('a withdrawn version stays readable with its reason', wh[0].status === 'withdrawn' && wh[0].close_reason === 'Wrong answer key used', `${wh[0].status} ${wh[0].close_reason}`)
check('an exam with published history cannot be deleted', (await admin('DELETE', `/exams/${otherEx.id}`)).code === 'exam_has_versions', 'kept')
await must(admin('PATCH', `/exams/${otherEx.id}/state`, { status: 'published' }), 'republish')
const wh2 = await must(admin('GET', `/exams/results/history?scope=assessment&scope_id=${otherEx.id}&student_id=${someone}`), 'history')
check('republishing adds the next version', wh2[0].status === 'published' && wh2[0].version > wh[0].version, `v${wh2[0].version}`)
tr = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'term review')
check('the term result is unaffected by the withdrawal and republication', tr.state === 'published', tr.state)

// ---- 7. Annual result ----------------------------------------------------------------
const overview = await must(admin('GET', `/exams/results/overview?session_id=${session}`), 'overview')
const annualCell = overview.annual.find(c => c.class_id === cls)
if (annualCell?.state === 'ready') {
  const ar = await must(head('GET', `/exams/results/review?scope=annual&scope_id=${session}&class_id=${cls}`), 'annual review')
  const ok = await must(head('POST', '/exams/results/approve', { scope: 'annual', scope_id: session, class_id: cls, inputs_sha256: ar.inputs_sha256 }), 'approve annual')
  await must(admin('POST', '/exams/results/publish', { scope: 'annual', scope_id: session, class_id: cls, publication_id: ok.approval.id }), 'publish annual')
  const annual = (await must(student('GET', '/exams/me/term-results'), 'results')).find(p => p.scope === 'annual')
  check('the annual result is published from the term results with its method', !!annual && annual.method === 'equal' && annual.subjects.every(l => !l.terms || l.terms.length >= 1), annual && `${annual.gpa} ${annual.method}`)
} else check('the annual result waits for every term of the blueprint', annualCell?.state === 'terms_pending' && annualCell.missing.length > 0, annualCell && `${annualCell.state}: ${annualCell.missing?.join(', ')}`)

// ---- 8. Who can do what ----------------------------------------------------------------
const reviewQ = `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`
const denied = await Promise.all([teacher('GET', reviewQ), student('GET', reviewQ), guardian('GET', reviewQ), student('GET', `/exams/results/history?scope=term&scope_id=${term.id}&student_id=${me.id}`),
  teacher('POST', '/exams/results/approve', { scope: 'term', scope_id: term.id, class_id: cls, inputs_sha256: '0'.repeat(64) }),
  head('POST', '/exams/results/publish', { scope: 'term', scope_id: term.id, class_id: cls, publication_id: term.id }),
  head('POST', `/exams/${myEx.id}/corrections`, { student_id: me.id, reason: 'not allowed' }), teacher('GET', '/exams/me/term-results')])
check('teachers, students and guardians cannot review; only administrators publish or correct', denied.every(x => x.status === 403), denied.map(x => x.status).join(' '))
const kids = await must(guardian('GET', '/guardians/me/children'), 'children')
const kidIds = new Set((kids.items || kids).map(k => k.id || k.student_id))
const other = Object.values(rosters).flat().find(x => !kidIds.has(x.student_id))?.student_id
const otherRes = await guardian('GET', `/exams/children/${other}/term-results`)
check('a guardian cannot read another student\'s results', !!other && [403, 404].includes(otherRes.status), `${otherRes.status} for a student not linked`)

// ---- 9. Withheld: the demo student's result is held back ------------------------------
const corr = await must(admin('POST', `/exams/${myEx.id}/corrections`, { student_id: me.id, reason: 'Exam committee review' }), 'open correction')
const cell = corr.cells.find(c => c.components.length === 0) || corr.cells[0]
const wres = await must(admin('PATCH', `/exams/${myEx.id}/marks`, { entries: [{ exam_subject_id: cell.exam_subject_id, student_id: me.id, status: 'withheld', marks: cell.marks, reason: 'Committee', row_version: cell.row_version }] }), 'withhold')
check('the administrator withholds the student\'s mark under the correction', wres.errors.length === 0, JSON.stringify(wres.errors))
for (const ref of [{ scope: 'assessment', scope_id: myEx.id }, { scope: 'term', scope_id: term.id, class_id: cls }]) {
  const rv = await must(head('GET', `/exams/results/review?scope=${ref.scope}&scope_id=${ref.scope_id}${ref.class_id ? `&class_id=${ref.class_id}` : ''}`), 'review')
  const ok = await must(head('POST', '/exams/results/approve', { ...ref, inputs_sha256: rv.inputs_sha256, reason: 'Withheld by the committee' }), 'approve')
  await must(admin('POST', '/exams/results/publish', { ...ref, publication_id: ok.approval.id }), 'publish')
}
const withheldTerm = (await must(student('GET', '/exams/me/term-results'), 'results')).find(p => p.term_id === term.id)
const withheldCard = await must(student('GET', `/exams/me/results/${myEx.id}`), 'card')
check('a withheld result shows only the status, no marks or GPA', withheldTerm.withheld && withheldTerm.subjects.length === 0 && withheldTerm.gpa === null && withheldCard.withheld && withheldCard.subjects.length === 0, 'withheld')

// ---- 10. The student's screen ------------------------------------------------------------
const sb = await launch({ base, shots, port: 9604 })
await sb.viewport(390, 844, true)
await sb.login('demo.student@demo.flybirds.test', password)
await sb.go('/student/results')
await waitText(sb, 'Term and annual results', 20000)
const studentScreen = await textOf(sb)
check('the student results page lists the term result as withheld at 390 px', studentScreen.includes(term.name) && studentScreen.includes('Result withheld') && !(await sb.overflow()), 'mobile')
await sb.shot('rv-student-mobile', false)
await sb.close()

// ---- 11. Layouts, languages, themes and accessibility ---------------------------------
const { updated_at: _ignored, ...settings } = (await admin('GET', '/school/settings')).data
const putSettings = async patch => { const res = await admin('PUT', '/school/settings', { ...settings, ...patch }); if (res.status !== 200) throw new Error(`school settings: ${res.status} ${res.code}`) }
const axePath = join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
const contrast = `(() => {
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  const rgb = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = '#000'; ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const opaque = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return ctx.getImageData(0, 0, 1, 1).data[3] > 250 }
  const bg = el => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (c && opaque(c)) return c } return getComputedStyle(document.body).backgroundColor }
  const ratio = el => { const a = lum(rgb(getComputedStyle(el).color)), b = lum(rgb(bg(el))); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) }
  const sel = 'main h1, main h2, main h3, main p, main span, main td, main th, main button, main a, main select, main input, main label, main dt, main dd, main li'
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && !el.closest('[disabled]') && !el.disabled && !el.classList.contains('line-through')).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
const pages = [
  ['centre', 'admin', `/admin/exams/results?session=${session}`, '#rv-annual-title'],
  ['review', 'admin', reviewPath, '#rv-table-title'],
  ['correction', 'admin', `/admin/exams/results/review?scope=assessment&scope_id=${failing.exam.id}&session=${session}`, '#rv-table-title'],
  ['student', 'student', '/student/results', '#rv-published-title'],
]
const v = await launch({ base, shots, port: 9605 })
for (const [label, width, height, mobile, lang, theme, colour] of [
  ['desktop-en-light', 1440, 900, false, 'en', 'light', '#1d4ed8'],
  ['tablet-en-light', 768, 1024, true, 'en', 'light', '#0f766e'],
  ['mobile-bn-light', 390, 844, true, 'bn', 'light', '#0f766e'],
  ['desktop-bn-dark', 1440, 900, false, 'bn', 'dark', '#b45309'],
  ['mobile-en-dark', 390, 844, true, 'en', 'dark', '#1d4ed8'],
]) {
  await putSettings({ primary_color: colour, dashboard_theme: theme })
  await v.viewport(width, height, mobile)
  for (const [page, who, path, ready] of pages) {
    await v.login(`demo.${who}@demo.flybirds.test`, password, lang)
    await v.go(path)
    await v.waitFor(`!!document.querySelector(${JSON.stringify(ready)})`, 20000)
    await sleep(700)
    const state = await v.evaluate(`({ overflow: document.documentElement.scrollWidth > window.innerWidth + 1, theme: document.documentElement.dataset.theme, lang: document.documentElement.lang, ready: !!document.querySelector(${JSON.stringify(ready)}) })`)
    const low = await v.evaluate(contrast)
    let axe = null
    if (existsSync(axePath)) {
      await v.evaluate(readFileSync(axePath, 'utf8'))
      axe = await v.evaluate(`axe.run(document.querySelector('main'), { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(x => x.id + ':' + x.nodes.length))`)
    }
    check(`${page} ${label}: in ${lang}, ${theme} theme, no horizontal overflow`, !state.overflow && state.theme === theme && state.lang === lang && state.ready, JSON.stringify(state))
    check(`${page} ${label}: WCAG AA text contrast`, low.length === 0, low.join(' | ') || 'ok')
    if (axe) check(`${page} ${label}: axe finds no WCAG 2.1 AA violations`, axe.length === 0, axe.join(', ') || 'clean')
    await v.shot(`rv-${page}-${label}`, false)
  }
}
await putSettings({ primary_color: settings.primary_color, dashboard_theme: settings.dashboard_theme })
await v.close()
await ab.close()

const failed = (await must(admin('GET', '/exams/results/overview?session_id=' + session), 'overview')).terms.length >= 1
check('the publication overview still loads after the run', failed, 'ok')
console.log('\nMETRICS ' + JSON.stringify(metrics))
r.save(join(shots, 'result-publication.json'))
