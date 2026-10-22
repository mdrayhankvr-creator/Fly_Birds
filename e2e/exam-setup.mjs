// E5 exam setup on a running, demo-seeded disposable school: terms, a class
// blueprint built in the browser (class tests + term exam, components,
// combined papers, optional subjects), the term setup wizard, optional
// subject choices, component mark entry, the provisional term results, who
// sees what, layouts, languages, themes and accessibility.
//
//   E2E_BASE_URL=http://localhost:8107 DEMO_PASSWORD=... node e2e/exam-setup.mjs
//
// It adds students, subjects, a term, a blueprint version and exams, and
// writes and publishes marks; never run it against a real school.
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
const stamp = Date.now().toString(36).slice(-4).toUpperCase()

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${who}@demo.flybirds.test`, password }) })
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
const [adminAPI, teacherAPI, headAPI, studentAPI] = await Promise.all(['admin', 'teacher', 'head', 'student'].map(api))
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }

// ---- 0. A realistic class: 48 + 44 students, two papers, two fourth subjects
const session = (await must(adminAPI('GET', '/academic/sessions?page=1&page_size=100'), 'sessions')).items.find(s => s.is_active)
const six = (await must(adminAPI('GET', '/academic/classes?page=1&page_size=100'), 'classes')).items.find(c => c.name === 'Class Six')
const allSubjects = (await must(adminAPI('GET', '/academic/subjects?page=1&page_size=100'), 'subjects')).items
const ensureSubject = async (code, en, bn) => allSubjects.find(s => s.code === code) || must(adminAPI('POST', '/academic/subjects', { name_en: en, name_bn: bn, code, is_active: true }), code)
const ban2 = await ensureSubject('BAN2', 'Bangla 2nd Paper', 'বাংলা দ্বিতীয় পত্র')
const hmath = await ensureSubject('HMATH', 'Higher Mathematics', 'উচ্চতর গণিত')
const agri = await ensureSubject('AGRI', 'Agriculture', 'কৃষিশিক্ষা')
const mapping = await must(adminAPI('GET', `/academic/classes/${six.id}/subjects`), 'mapping')
const mapped = new Set((mapping.items || mapping).map(m => m.subject_id))
for (const [s, fourth] of [[ban2, false], [hmath, true], [agri, true]]) if (!mapped.has(s.id)) await must(adminAPI('POST', `/academic/classes/${six.id}/subjects`, { subject_id: s.id, is_required: !fourth, is_fourth_subject: fourth }), 'map ' + s.code)
const sections = (await must(adminAPI('GET', `/academic/sections?page=1&page_size=100&session_id=${session.id}&class_id=${six.id}`), 'sections')).items.filter(s => s.is_active)
const names = [['Arif', 'Nusrat', 'Tanvir', 'Sadia', 'Rakib', 'Farzana', 'Imran', 'Mitu'], ['Rahman', 'Akter', 'Hossain', 'Islam', 'Chowdhury', 'Saha', 'Ahmed']]
for (const [i, section] of sections.entries()) {
  const target = i === 0 ? 48 : 44
  const existing = (await must(adminAPI('GET', `/students?page=1&page_size=100&session_id=${session.id}&class_id=${six.id}&section_id=${section.id}`), 'students')).items
  let roll = Math.max(0, ...existing.map(s => s.roll_no || 0))
  for (let k = existing.length; k < target; k++) {
    roll++
    await must(adminAPI('POST', '/students', { admission_no: `E5-${section.code}-${String(roll).padStart(3, '0')}`, name_en: `${names[0][(roll + i) % 8]} ${names[1][roll % 7]}`, name_bn: '', date_of_birth: '', status: 'active', session_id: session.id, class_id: six.id, section_id: section.id, roll_no: roll, user_id: '' }), 'student')
  }
}
const classStudents = (await Promise.all(sections.map(s => adminAPI('GET', `/students?page=1&page_size=100&session_id=${session.id}&class_id=${six.id}&section_id=${s.id}`)))).map(res => res.data.items)
check('the class has two sections with 40–60 students each', classStudents.every(list => list.length >= 40 && list.length <= 60), classStudents.map(l => l.length).join(' + '))
const subjectByCode = Object.fromEntries((await must(adminAPI('GET', '/academic/subjects?page=1&page_size=100'), 'subjects')).items.map(s => [s.code, s]))

// Browser helpers: actions are counted as interactions, typed characters as keystrokes.
const shots = join(here, 'shots')
let interactions = 0, keystrokes = 0
const textOf = b => b.evaluate(`document.querySelector('main') ? document.querySelector('main').innerText : ''`)
const waitText = (b, text, timeout = 12000) => b.waitFor(`(document.querySelector('main')?.innerText || '').includes(${JSON.stringify(text)})`, timeout)
async function click(b, text, { within = 'main', exact = true, label = false } = {}) {
  const ok = await b.evaluate(`(() => {
    const root = ${JSON.stringify(within)} === 'body' ? document.body : document.querySelector(${JSON.stringify(within)})
    const els = [...root.querySelectorAll(${label ? `'label'` : `'button, a, [role=tab]'`})].filter(e => e.offsetParent !== null && !e.disabled)
    const want = ${JSON.stringify(text)}
    const el = els.find(e => ${exact ? `e.innerText.trim() === want || e.getAttribute('aria-label') === want` : `e.innerText.includes(want) || (e.getAttribute('aria-label') || '').includes(want)`})
    if (!el) return false
    el.click(); return true
  })()`)
  if (!ok) { await b.shot(`bp-failed-click`, false); throw new Error(`nothing to click: ${text}`) }
  interactions++
  await sleep(250)
}
async function confirm(b) {
  await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
  await b.evaluate(`[...document.querySelectorAll('[role=alertdialog] button')].at(-1).click()`)
  interactions++
  await sleep(250)
}
// Sets an input or select found by its label's text (React sees a real change).
async function setField(b, labelText, value, { type = false } = {}) {
  const ok = await b.evaluate(`(() => {
    const label = [...document.querySelectorAll('main label')].find(l => l.offsetParent !== null && l.innerText.trim().toLowerCase().startsWith(${JSON.stringify(labelText.toLowerCase())}))
    const el = label && label.querySelector('input, select')
    if (!el) return false
    const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : HTMLInputElement.prototype
    Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, ${JSON.stringify(value)})
    el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }))
    return true
  })()`)
  if (!ok) throw new Error(`no field: ${labelText}`)
  interactions++
  if (type) keystrokes += String(value).length
  await sleep(200)
}
// Network accounting inside a page: requests and failures to the API.
const instrument = `(() => { if (window.__e5) return; window.__e5 = { api: 0, failed: 0 }; const f = window.fetch
  window.fetch = async (...args) => { const isAPI = String(args[0]).includes('/api/'); let ok = false
    try { const res = await f(...args); ok = res.ok || res.status === 422; return res } finally { if (isAPI) { window.__e5.api++; if (!ok) window.__e5.failed++ } } } })()`

const b = await launch({ base, shots, port: 9581 })
await b.viewport(1440, 900)
await b.login('demo.admin@demo.flybirds.test', password)

// ---- 1. A term for the session ---------------------------------------------
const termName = `First Term ${stamp}`
await b.go(`/admin/exams/setup?session=${session.id}`)
await b.evaluate(instrument)
await waitText(b, 'Add term')
let t0 = Date.now(); interactions = 0; keystrokes = 0
await setField(b, 'Term name', termName, { type: true })
await click(b, 'Add term')
await waitText(b, termName)
check('an administrator adds a term to the session', (await textOf(b)).includes(termName), termName)
const term = (await must(adminAPI('GET', `/exams/terms?session_id=${session.id}`), 'terms')).find(x => x.name === termName)

// ---- 2. The class blueprint, built in the browser --------------------------
await click(b, 'Blueprints')
await waitText(b, 'Class Six')
// A draft left by an earlier run is discarded, so every run builds the same way.
let sixBlueprint = (await adminAPI('GET', `/exams/blueprints?session_id=${session.id}`)).data.find(c => c.class_id === six.id)
if (sixBlueprint.draft) {
  await must(adminAPI('DELETE', `/exams/blueprint-versions/${sixBlueprint.draft.id}`), 'old draft')
  sixBlueprint = (await adminAPI('GET', `/exams/blueprints?session_id=${session.id}`)).data.find(c => c.class_id === six.id)
  await b.go(`/admin/exams/setup?session=${session.id}`)
  await b.evaluate(instrument) // a new page: count its requests too
  await waitText(b, 'Add term')
  await click(b, 'Blueprints')
  await waitText(b, 'Class Six')
}
// Upgraded schools have a legacy version; a fresh class starts a blueprint.
const startLabel = sixBlueprint.draft ? 'Continue draft' : sixBlueprint.active ? 'New version' : 'Start blueprint'
await b.evaluate(`(() => { const li = [...document.querySelectorAll('main li')].find(li => li.innerText.startsWith('Class Six')); [...li.querySelectorAll('button, a')].find(x => x.innerText.trim() === ${JSON.stringify(startLabel)}).click() })()`)
interactions++
await b.waitFor(`location.pathname.startsWith('/admin/exams/blueprints/')`, 10000)
await waitText(b, 'Assessments by term', 15000)
const versionId = await b.evaluate(`location.pathname.split('/').pop()`)
const inTerm = async (buttonText) => {
  await b.evaluate(`(() => { const li = [...document.querySelectorAll('main ol > li')].find(li => li.querySelector('p')?.innerText.trim().toUpperCase() === ${JSON.stringify(termName.toUpperCase())}); [...li.querySelectorAll('button')].find(x => x.innerText.trim() === ${JSON.stringify(buttonText)}).click() })()`)
  interactions++
  await sleep(300)
}
// Legacy assessments are left to the legacy version: this version is the new design.
for (const legacy of (await must(adminAPI('GET', `/exams/blueprint-versions/${versionId}`), 'version')).definition.assessments.filter(a => a.term_id !== term.id)) {
  await click(b, `Remove ${legacy.name}`); await confirm(b)
}
await inTerm('+ Class test'); await inTerm('+ Class test')
// A new blueprint is seeded with a term exam for each term (fresh schools);
// a new version of an older design is not, so add one when it is offered.
const hasTermExam = (await must(adminAPI('GET', `/exams/blueprint-versions/${versionId}`), 'version')).definition.assessments.some(a => a.term_id === term.id && a.kind === 'term_exam')
if (!hasTermExam) await inTerm('+ Term exam')
await sleep(900) // the draft saves itself
// Subjects the design does not cover yet (all three on an upgraded school).
for (const name of ['Bangla 2nd Paper', 'Higher Mathematics', 'Agriculture']) {
  if (await b.evaluate(`[...document.querySelectorAll('main button')].some(x => x.innerText.trim() === ${JSON.stringify('+ ' + name)})`)) await click(b, `+ ${name}`)
}
// Subjects are picked by their stable id, not by how the button's text renders.
const pickSubject = async code => {
  const id = subjectByCode[code].id
  const ok = await b.evaluate(`(() => { const el = document.querySelector('section[aria-labelledby=bp-subjects] button[data-subject-id="${id}"]'); if (!el) return false; el.click(); return true })()`)
  if (!ok) throw new Error('no subject ' + code)
  interactions++
  await sleep(300)
}
await pickSubject('BAN')
await b.evaluate(`[...document.querySelectorAll('main [role=tab]')].find(x => x.innerText.trim() === ${JSON.stringify(termName)})?.click()`)
// Validation before activation: a custom rule with an empty contribution is
// refused with the subject and term named, and activation stays disabled.
await click(b, 'Custom', { label: true, exact: false })
await click(b, '+ Contribution')
await b.waitFor(`document.querySelector('[data-save-state]')?.dataset.saveState === 'saved'`, 10000)
await sleep(400)
const issues = await b.evaluate(`[...document.querySelectorAll('section[aria-labelledby=bp-check] li')].map(li => li.innerText)`)
check('validation names the subject, term and problem before activation', issues.some(text => text.startsWith(`Bangla · ${termName}:`)) && await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Activate version').disabled`), issues.slice(0, 2).join(' | '))
await click(b, 'CT average + final', { label: true, exact: false })
await click(b, 'Apply to all subjects'); await confirm(b)
// Mathematics' term exam: CQ 70 (pass 23) + MCQ 30 (pass 10).
await pickSubject('MATH')
await b.evaluate(`(() => { const box = [...document.querySelectorAll('main div.rounded-xl')].find(d => d.querySelector('p')?.innerText.startsWith(${JSON.stringify(termName + ' ·')})); [...box.querySelectorAll('button')].find(x => x.innerText.trim() === 'CQ + MCQ').click() })()`)
interactions++
// Bangla 1st and 2nd papers graded together.
for (const name of ['BAN2', 'BAN']) {
  await pickSubject(name)
  await setField(b, 'Combined papers key', 'BAN', { type: true })
}
await b.waitFor(`document.querySelector('[data-save-state]')?.dataset.saveState === 'saved'`, 15000)
await sleep(1500)
const draftAfter = await must(adminAPI('GET', `/exams/blueprint-versions/${versionId}`), 'draft')
check('the draft saved itself with no issues left', draftAfter.issues.length === 0, JSON.stringify(draftAfter.issues.slice(0, 3)))

// The worked sample: Mathematics, CT 17 and 14 of 20, term exam CQ 50 + MCQ 18 = 68.
await pickSubject('MATH')
for (const [label, v] of [['CT 1 marks', '17'], ['CT 2 marks', '14'], [`${termName} CQ marks`, '50'], [`${termName} MCQ marks`, '18']]) {
  await b.evaluate(`(() => { const el = [...document.querySelectorAll('section[aria-labelledby=bp-sample] input')].find(i => i.getAttribute('aria-label').startsWith(${JSON.stringify(label)})); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, ${JSON.stringify(v)}); el.dispatchEvent(new Event('input', { bubbles: true })) })()`)
  interactions++; keystrokes += v.length
}
await click(b, 'Work it out')
await b.waitFor(`(document.querySelector('section[aria-labelledby=bp-sample]')?.innerText || '').includes('/ 100.00')`, 8000)
const sample = await b.evaluate(`document.querySelector('section[aria-labelledby=bp-sample]').innerText`)
check('the worked sample matches example 1 of the calculation rules (15.50 + 54.40 = 69.90, A-)', /15\.50 \/ 20\.00/.test(sample) && /54\.40 \/ 80\.00/.test(sample) && /69\.90 \/ 100\.00 · A-/.test(sample), sample.split('\n').slice(-3).join(' | '))
await b.shot('bp-builder-desktop', false)
await click(b, 'Activate version')
await waitText(b, 'Version activated', 12000)
const builderSeconds = (Date.now() - t0) / 1000
const active = await must(adminAPI('GET', `/exams/blueprint-versions/${versionId}`), 'active')
check('the version activates and is then fixed', active.status === 'active' && active.sha256.length === 64 && (await adminAPI('PUT', `/exams/blueprint-versions/${versionId}`, { definition: active.definition })).status === 409, active.status)
metrics.blueprint = { seconds: +builderSeconds.toFixed(1), interactions, keystrokes, subjects: active.definition.subjects.length, assessments: active.definition.assessments.length }
const net1 = await b.evaluate(`window.__e5`)

// ---- 3. The term setup wizard ----------------------------------------------
await b.go(`/admin/exams/setup?session=${session.id}`)
await b.evaluate(instrument)
await waitText(b, termName)
t0 = Date.now(); interactions = 0; keystrokes = 0
await b.evaluate(`(() => { const li = [...document.querySelectorAll('main li')].find(li => li.innerText.includes(${JSON.stringify(termName)})); [...li.querySelectorAll('a')].find(x => x.innerText.trim() === 'Set up exams').click() })()`)
interactions++
await waitText(b, 'Save and continue', 12000)
await click(b, 'Save and continue')
await waitText(b, 'Class Six')
await click(b, 'Class Six', { label: true, exact: false })
await click(b, 'Save and continue')
await waitText(b, 'Same due date for all')
await setField(b, 'Same due date for all', '2026-11-30')
await click(b, 'Apply')
await click(b, 'Save and continue')
await waitText(b, 'mark sheets in')
const gapSelects = await b.evaluate(`document.querySelectorAll('main select[id^="gap-"]').length`)
// Every subject nobody teaches gets the first teacher on the list.
await b.evaluate(`[...document.querySelectorAll('main select[id^="gap-"]')].forEach(sel => { const opt = [...sel.options].find(o => o.value); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(sel, opt.value); sel.dispatchEvent(new Event('change', { bubbles: true })) })`)
interactions += gapSelects
await click(b, 'Save and continue')
await waitText(b, 'Create term')
await b.shot('bp-wizard-review', false)
await click(b, 'Create term')
await waitText(b, 'Created ', 20000)
const wizardSeconds = (Date.now() - t0) / 1000
const created = await b.evaluate(`document.querySelector('main').innerText.match(/Created (\\d+) exams and (\\d+) sheets/)`)
const examsCreated = Number(created?.[1]), sheetsCreated = Number(created?.[2])
check('one Create makes an exam per section per assessment and a sheet per subject', examsCreated === 6 && sheetsCreated === 36, created?.[0])
metrics.wizard = { seconds: +wizardSeconds.toFixed(1), interactions, keystrokes, exams: examsCreated, sheets: sheetsCreated, teacherGaps: gapSelects }
// The page shows the plan the reply carried: nothing left to create.
await b.waitFor(`[...document.querySelectorAll('main button')].some(x => x.innerText.trim() === 'Create what is missing' && !x.disabled)`, 10000)
const after = await b.evaluate(`[...document.querySelectorAll('main dl dd')].map(d => d.innerText.trim()).join(' ')`)
check('after Create the review shows nothing new left to create', /^1 0 \+ 6 existing 0 \+ 36 existing/.test(after), after)
await click(b, 'Create what is missing')
await waitText(b, 'Created 0 exams and 0 sheets', 15000)
check('running Create again creates nothing new', true, 'Created 0 exams and 0 sheets')
const net2 = await b.evaluate(`window.__e5`)
const termExams = (await must(adminAPI('GET', `/exams?page=1&page_size=100&session_id=${session.id}&class_id=${six.id}`), 'exams')).items.filter(e => e.term_id === term.id)
check('the exams are linked to the term, assessment and version, with due dates', termExams.length === 6 && termExams.every(e => e.blueprint_version_id === versionId && e.marks_due_on === '2026-11-30' && e.assessment_key), termExams.map(e => e.name).slice(0, 3).join(' | '))

// ---- 4. Optional subject choices -------------------------------------------
// Earlier runs' choices are cleared, so the page saves every choice itself.
await must(adminAPI('PUT', '/exams/subject-choices', { session_id: session.id, class_id: six.id, choices: classStudents.flat().map(s => ({ student_id: s.id, subject_id: '' })) }), 'clear choices')
await b.go(`/admin/exams/setup?session=${session.id}`)
await waitText(b, 'Add term')
await click(b, 'Optional subjects')
await setField(b, 'Class', six.id)
await waitText(b, 'Higher Mathematics')
const [secA, secB] = classStudents
// Section A: the first 20 (by admission number) take Higher Mathematics, the
// rest Agriculture; section B alternates. Rows are matched by admission number.
const hmathA = secA.slice(0, 20), agriA = secA.slice(20), allB = secB
const wanted = Object.fromEntries([...hmathA.map(s => [s.admission_no, hmath.id]), ...agriA.map(s => [s.admission_no, agri.id]), ...allB.map((s, i) => [s.admission_no, i % 2 ? agri.id : hmath.id])])
const set = await b.evaluate(`(() => { const want = ${JSON.stringify(wanted)}; let n = 0
  for (const tr of document.querySelectorAll('main tbody tr')) { const adm = tr.querySelector('th span')?.innerText.trim(); const sel = tr.querySelector('select'); if (!sel || !want[adm]) continue
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(sel, want[adm]); sel.dispatchEvent(new Event('change', { bubbles: true })); n++ }
  return n })()`)
interactions += set
await click(b, `Save ${secA.length + secB.length} changes`)
await waitText(b, 'choices saved')
const choices = await must(adminAPI('GET', `/exams/subject-choices?session_id=${session.id}&class_id=${six.id}`), 'choices')
check('every student of the class has an optional subject', choices.in_use && choices.students.every(s => s.subject_id), JSON.stringify(choices.counts))
b.close()

// ---- 5. The teacher enters component marks ---------------------------------
const examFor = (sectionId, key) => termExams.find(e => e.section_id === sectionId && e.assessment_key === key)
const keys = active.definition.assessments.map(a => a.key) // CT1, CT2, term exam
const exKey = active.definition.assessments.find(a => a.kind === 'term_exam').key
const [ct1Key, ct2Key] = active.definition.assessments.filter(a => a.kind === 'class_test').map(a => a.key)
const sheetOf = async (exam, code) => (await must(adminAPI('GET', `/exams/${exam.id}`), 'exam')).subjects.find(s => s.subject_id === subjectByCode[code].id)
const queue = await must(teacherAPI('GET', '/exams/my-sheets'), 'queue')
const mathA = await sheetOf(examFor(sections[0].id, exKey), 'MATH')
check('the teacher\'s queue lists the new sheets with their due date', queue.some(q => q.sheet_id === mathA.id && q.due_on === '2026-11-30'), `${queue.filter(q => q.due_on).length} sheets with a due date`)
const tb = await launch({ base, shots, port: 9582 })
await tb.viewport(1440, 900)
await tb.login('demo.teacher@demo.flybirds.test', password)
await tb.go(`/teacher/exams/${examFor(sections[0].id, exKey).id}/sheets/${mathA.id}`)
await tb.waitFor(`document.querySelectorAll('main table tbody input[inputmode="decimal"]').length === ${secA.length * 2}`, 15000)
await tb.evaluate(`(() => { window.__saves = 0; const f = window.fetch; window.fetch = async (...a) => { if (String(a[0]).includes('/marks') && (a[1] || {}).method === 'PATCH') window.__saves++; return f(...a) } })()`)
const cq = i => 30 + (i * 7) % 41, mcq = i => 10 + (i * 3) % 21 // CQ ≤ 70, MCQ ≤ 30
t0 = Date.now()
let teacherKeys = 0
// Column by column: CQ down the sheet with Enter, then MCQ.
for (const [part, value] of [['CQ', cq], ['MCQ', mcq]]) {
  await tb.evaluate(`document.querySelector('main table tbody input[aria-label^="${part} for"]').focus()`)
  for (let i = 0; i < secA.length; i++) {
    await tb.send('Input.insertText', { text: String(value(i)) }); teacherKeys += String(value(i)).length
    await tb.key('Enter', 'Enter', 13); teacherKeys++
  }
}
await tb.waitFor(`(document.querySelector('[data-save-state]')?.innerText || '').includes('All changes saved')`, 30000)
const entrySeconds = (Date.now() - t0) / 1000
const saved = await must(teacherAPI('GET', `/exams/assigned/${examFor(sections[0].id, exKey).id}/sheets/${mathA.id}`), 'sheet')
// Values were typed in grid order (roll, then admission number).
const rowsOK = saved.rows.every((row, i) => row.components?.CQ === cq(i) && row.components?.MCQ === mcq(i) && row.marks === cq(i) + mcq(i))
check('component marks typed down each column are saved with their exact sum', rowsOK, `${saved.rows.length} students`)
metrics.componentEntry = { students: secA.length, components: 2, seconds: +entrySeconds.toFixed(1), keystrokes: teacherKeys, saveRequests: await tb.evaluate('window.__saves') }
await tb.shot('bp-components-desktop', false)
// The optional subject's sheet lists only the students who chose it.
const hmathSheetA = await sheetOf(examFor(sections[0].id, exKey), 'HMATH')
const hmathWS = await adminAPI('GET', `/exams/${examFor(sections[0].id, exKey).id}/sheets/${hmathSheetA.id}`)
check('an optional subject\'s sheet has only the students who chose it', hmathWS.data.rows.length === hmathA.length && hmathWS.data.sheet.students === hmathA.length, `${hmathWS.data.rows.length} of ${secA.length}`)
tb.close()

// ---- 6. Everything else through the API, then submit and approve ------------
const markOf = (code, key, i) => key === exKey ? 35 + (i * 5 + code.length * 3) % 60 : 8 + (i * 3 + code.length + key.length) % 12
const paperOf = (s, code) => code === 'HMATH' || code === 'AGRI'
let apiSaves = 0
for (const [si, section] of sections.entries()) {
  const students = classStudents[si]
  for (const key of keys) {
    const exam = examFor(section.id, key)
    const full = await must(adminAPI('GET', `/exams/${exam.id}`), 'exam')
    const entries = []
    for (const sheet of full.subjects) {
      const code = Object.values(subjectByCode).find(s => s.id === sheet.subject_id).code
      if (code === 'MATH' && key === exKey && si === 0) continue // typed by the teacher
      const takers = paperOf(null, code) ? students.filter(s => choices.students.find(c => c.student_id === s.id)?.subject_id === sheet.subject_id) : students
      for (const [i, st] of students.entries()) {
        if (!takers.includes(st)) continue
        if (code === 'MATH' && key === exKey) entries.push({ exam_subject_id: sheet.id, student_id: st.id, status: 'present', marks: null, components: { CQ: cq(i % 40), MCQ: mcq(i % 40) }, reason: '', row_version: 0 })
        else entries.push({ exam_subject_id: sheet.id, student_id: st.id, status: 'present', marks: markOf(code, key, i), reason: '', row_version: 0 })
      }
    }
    for (let k = 0; k < entries.length; k += 400) {
      const res = await must(adminAPI('PATCH', `/exams/${exam.id}/marks`, { entries: entries.slice(k, k + 400) }), 'marks')
      if (res.errors.length) throw new Error('marks: ' + JSON.stringify(res.errors.slice(0, 2)))
      apiSaves++
    }
    for (const sheet of full.subjects) {
      await must(adminAPI('POST', `/exams/${exam.id}/sheets/${sheet.id}/submit`, {}), 'submit')
      await must(headAPI('POST', `/exams/${exam.id}/sheets/${sheet.id}/approve`, {}), 'approve')
    }
  }
}

// ---- 7. Provisional term results ---------------------------------------------
const pStart = Date.now()
const preview = await must(adminAPI('GET', `/exams/terms/${term.id}/results?class_id=${six.id}`), 'preview')
metrics.preview = { students: preview.students.length, ms: Date.now() - pStart }
// The first student of the grid: typed CQ cq(0) and MCQ mcq(0); CT marks by
// their place in the list the API fill used.
const firstId = saved.rows[0].student_id, apiIndex = secA.findIndex(s => s.id === firstId)
const first = preview.students.find(s => s.student_id === firstId)
const mathLine = first.result.subjects.find(x => x.subject_ids[0] === subjectByCode.MATH.id)
// CT 1 and CT 2 averaged and worth 20; the term exam (CQ + MCQ of 100) worth 80.
const want = (markOf('MATH', ct1Key, apiIndex) + markOf('MATH', ct2Key, apiIndex)) / 2 + 0.8 * (cq(0) + mcq(0))
check('the term mark is the CT average (20) plus the term exam (80), exactly', Number(mathLine.marks) === Math.round(want * 100) / 100, `${mathLine.marks} = ${mathLine.contributions.map(c => c.value).join(' + ')}`)
const bangla = first.result.subjects.find(x => x.combined_key === 'BAN')
check('Bangla 1st and 2nd papers are graded together', bangla && bangla.subject_ids.length === 2 && bangla.full === '200.00', bangla && `${bangla.marks} / ${bangla.full} ${bangla.grade}`)
check('each student has one optional subject in the result', preview.students.every(s => s.result.subjects.filter(x => x.optional).length === 1), preview.outcomes && JSON.stringify(preview.outcomes))
check('the preview is ready once every sheet is approved, with positions', preview.state === 'ready' && preview.students.every(s => s.result.class_position > 0), `${preview.approved}/${preview.sheets} sheets`)
const ab = await launch({ base, shots, port: 9583 })
await ab.viewport(1440, 900)
await ab.login('demo.admin@demo.flybirds.test', password)
await ab.go(`/admin/exams/terms/${term.id}/results?session=${session.id}&class=${six.id}`)
await ab.waitFor(`!!document.querySelector('#bp-preview-title')`, 15000)
await sleep(800)
const shown = await textOf(ab)
check('the results page is framed as provisional and lists every student', /provisional — not published/i.test(shown) && preview.students.every(s => shown.includes(s.name)), `${preview.students.length} students`)
await ab.shot('bp-results-desktop', false)
ab.close()

// ---- 8. Who sees what ----------------------------------------------------------
const me = (await studentAPI('GET', '/students/me')).data
if (me && me.class_id === six.id) {
  const ct = examFor(me.section_id, ct1Key), ex = examFor(me.section_id, exKey)
  await must(adminAPI('PATCH', `/exams/${ct.id}/state`, { status: 'published' }), 'publish CT')
  await must(adminAPI('PATCH', `/exams/${ex.id}/state`, { status: 'published' }), 'publish term exam')
  const cards = (await studentAPI('GET', '/exams/me/results')).data.map(c => c.exam.id)
  check('the student sees the term exam but never the class test', cards.includes(ex.id) && !cards.includes(ct.id) && (await studentAPI('GET', `/exams/me/results/${ct.id}`)).status === 404, `${cards.length} results`)
} else check('the demo student is in Class Six (for the visibility check)', false, me?.class_id)
const studentDenied = await Promise.all([`/exams/terms?session_id=${session.id}`, `/exams/blueprint-versions/${versionId}`, `/exams/terms/${term.id}/results?class_id=${six.id}`].map(p => studentAPI('GET', p)))
const teacherDenied = await teacherAPI('GET', `/exams/blueprint-versions/${versionId}`)
check('students and teachers cannot read terms, blueprints or term results', studentDenied.every(x => x.status === 403) && teacherDenied.status === 403, studentDenied.map(x => x.status).join(' '))
// The head teacher looks but cannot change anything.
const hb = await launch({ base, shots, port: 9584 })
await hb.viewport(1440, 900)
await hb.login('demo.head@demo.flybirds.test', password)
await hb.go(`/admin/exams/blueprints/${versionId}?session=${session.id}`)
await hb.waitFor(`(document.querySelector('main')?.innerText || '').includes('Assessments by term')`, 15000)
await sleep(600)
const headView = await hb.evaluate(`({ text: document.querySelector('main').innerText, buttons: [...document.querySelectorAll('main button')].map(x => x.innerText.trim()) })`)
await hb.go(`/admin/exams/terms/${term.id}/setup?session=${session.id}`)
await hb.waitFor(`(document.querySelector('main')?.innerText || '').includes('an administrator creates the term')`, 15000)
const headWizard = await hb.evaluate(`[...document.querySelectorAll('main button')].map(x => x.innerText.trim())`)
check('the head teacher sees blueprints and the wizard read-only', !headView.buttons.includes('Activate version') && !headView.buttons.includes('New version') && !headWizard.includes('Create term') && (await headAPI('POST', `/exams/terms/${term.id}/create`, {})).status === 403, 'no Activate, New version or Create')
hb.close()

// ---- 9. Layouts, languages, themes and accessibility --------------------------
const { updated_at: _ignored, ...settings } = (await adminAPI('GET', '/school/settings')).data
const putSettings = async patch => { const res = await adminAPI('PUT', '/school/settings', { ...settings, ...patch }); if (res.status !== 200) throw new Error(`school settings: ${res.status} ${res.code}`) }
const v = await launch({ base, shots, port: 9585 })
const axePath = join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
const runAxe = async () => {
  if (!existsSync(axePath)) return null
  await v.evaluate(readFileSync(axePath, 'utf8'))
  return v.evaluate(`axe.run(document.querySelector('main'), { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(x => x.id + ':' + x.nodes.length))`)
}
const contrast = `(() => {
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  const rgb = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = '#000'; ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const opaque = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return ctx.getImageData(0, 0, 1, 1).data[3] > 250 }
  const bg = el => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (c && opaque(c)) return c } return getComputedStyle(document.body).backgroundColor }
  const ratio = el => { const a = lum(rgb(getComputedStyle(el).color)), b = lum(rgb(bg(el))); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) }
  const sel = 'main h1, main h2, main p, main span, main td, main th, main button, main a, main select, main input, main label, main dt, main dd'
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && !el.closest('[disabled]') && !el.disabled).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
const pages = [
  ['setup', `/admin/exams/setup?session=${session.id}`, 'Exam setup', 'পরীক্ষার সেটআপ'],
  // The class name heads the builder in every layout (phones open on its rules).
  ['builder', `/admin/exams/blueprints/${versionId}?session=${session.id}`, 'Class Six', 'Class Six'],
  ['wizard', `/admin/exams/terms/${term.id}/setup?session=${session.id}`, 'Save and continue', 'সংরক্ষণ করে এগিয়ে যান'],
  ['results', `/admin/exams/terms/${term.id}/results?session=${session.id}&class=${six.id}`, 'Provisional', 'অস্থায়ী'],
]
for (const [label, width, height, mobile, lang, theme, colour] of [
  ['desktop-en-light', 1440, 900, false, 'en', 'light', '#1d4ed8'],
  ['tablet-en-light', 768, 1024, true, 'en', 'light', '#0f766e'],
  ['mobile-bn-light', 390, 844, true, 'bn', 'light', '#0f766e'],
  ['desktop-bn-dark', 1440, 900, false, 'bn', 'dark', '#b45309'],
  ['mobile-en-dark', 390, 844, true, 'en', 'dark', '#1d4ed8'],
]) {
  await putSettings({ primary_color: colour, dashboard_theme: theme })
  await v.viewport(width, height, mobile)
  await v.login('demo.admin@demo.flybirds.test', password, lang)
  for (const [page, path, en, bn] of pages) {
    await v.go(path)
    await v.waitFor(`(document.querySelector('main')?.innerText || '').includes(${JSON.stringify(lang === 'bn' ? bn : en)})`, 15000).catch(() => {})
    await sleep(700)
    const state = await v.evaluate(`({ overflow: document.documentElement.scrollWidth > window.innerWidth + 1, theme: document.documentElement.dataset.theme, lang: document.documentElement.lang, text: (document.querySelector('main')?.innerText || '').includes(${JSON.stringify(lang === 'bn' ? bn : en)}) })`)
    const low = await v.evaluate(contrast)
    const axe = await runAxe()
    check(`${page} ${label}: in ${lang}, ${theme} theme, no horizontal overflow`, !state.overflow && state.theme === theme && state.text, JSON.stringify(state))
    check(`${page} ${label}: WCAG AA text contrast`, low.length === 0, low.join(' | ') || 'ok')
    if (axe) check(`${page} ${label}: axe finds no WCAG 2.1 AA violations`, axe.length === 0, axe.join(', ') || 'clean')
    await v.shot(`bp-${page}-${label}`, false)
  }
}
await putSettings({ primary_color: settings.primary_color, dashboard_theme: settings.dashboard_theme })
v.close()

metrics.requests = { builder: net1, wizard: net2, apiMarkSaves: apiSaves }
check('no request failed in the builder or the wizard', net1.failed === 0 && net2.failed === 0, JSON.stringify(metrics.requests))
console.log('\nMETRICS ' + JSON.stringify(metrics))
r.save(join(shots, 'exam-setup.json'))
