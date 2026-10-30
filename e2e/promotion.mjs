// M15 year-end promotion through the API on a running, demo-seeded
// disposable school: class progression, a promotion batch for the demo
// student's class (blocked without a published annual result, decided with
// reasons), head-teacher approval bound to the inputs, two administrators
// applying at once, the old year's tabulations and report cards unchanged,
// withdrawal suspending the student's login, history for the guardian,
// the concession review, the generic-edit guard, permissions and a revert
// that restores everything.
//
//   E2E_BASE_URL=http://localhost:8081 DEMO_PASSWORD=... node e2e/promotion.mjs
//
// It adds a session, a class and sections, and moves the demo class into the
// new session before reverting; never run it against a real school.
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { reporter } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const stamp = Date.now().toString(36).slice(-4).toUpperCase()

async function signIn(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${who}@demo.flybirds.test`, password }) })
  return { status: res.status, token: res.status === 200 ? (await res.json()).data.access_token : '' }
}
function client(token) {
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
async function api(who) {
  const s = await signIn(who)
  if (s.status !== 200) throw new Error(`sign-in ${who}: ${s.status}`)
  return client(s.token)
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }
const [admin, head, teacher, student, guardian, accounts] = await Promise.all(['admin', 'head', 'teacher', 'student', 'guardian', 'accounts'].map(api))

// ---- 0. The demo student's year and a next session --------------------------
const me = await must(student('GET', '/students/me'), 'me')
const from = me.session_id, cls = me.class_id
const classStudents = (await must(admin('GET', `/students?session_id=${from}&class_id=${cls}&status=active&page=1&page_size=100`), 'class')).items
const sessions = (await must(admin('GET', '/academic/sessions?page=1&page_size=100'), 'sessions')).items
const year = Math.max(...sessions.map(s => Number(s.name.slice(0, 4)))) + 1
const to = await must(admin('POST', '/academic/sessions', { name: String(year), start_date: `${year}-01-01`, end_date: `${year}-12-31` }), 'next session')
const next = await must(admin('POST', '/academic/classes', { name: `E2E Next ${stamp}`, display_order: 99000, is_active: true }), 'next class')
const sectionCodes = [...new Set(classStudents.map(s => s.section_id))]
for (const id of sectionCodes) {
  const sec = await must(admin('GET', `/academic/sections/${id}`), 'section')
  for (const target of [next.id, cls]) await must(admin('POST', '/academic/sections', { session_id: to.id, class_id: target, name: sec.name, code: sec.code, display_order: sec.display_order, is_active: true }), 'next-year section')
}

// The old year as people see it, before promotion.
const exams = (await must(admin('GET', `/exams?session_id=${from}&class_id=${cls}&page=1&page_size=100`), 'exams')).items
const published = exams.filter(e => e.status !== 'draft')
async function oldYear() {
  const out = {}
  for (const e of published) out['tab ' + e.id] = await must(admin('GET', `/exams/${e.id}/tabulation`), 'tabulation')
  out.children = await must(guardian('GET', `/exams/children/${me.id}/results`), 'children results')
  return JSON.stringify(out)
}
const before = await oldYear()
check('the demo class has students and published exams to protect', classStudents.length > 0 && published.length > 0, `${classStudents.length} students, ${published.length} published exams`)

// ---- 1. Permissions ------------------------------------------------------------
const createBody = { from_session_id: from, to_session_id: to.id, class_id: cls }
for (const [who, call] of [['teacher', teacher], ['accountant', accounts], ['student', student], ['guardian', guardian]]) {
  const res = await Promise.all([call('GET', '/promotions'), call('POST', '/promotions', createBody), call('GET', '/promotions/progression')])
  check(`${who} reaches no promotion endpoint`, res.every(x => x.status === 403), res.map(x => x.status).join(','))
}
check('the head teacher cannot prepare a batch', (await head('POST', '/promotions', createBody)).status === 403)

// ---- 2. Progression and the batch ------------------------------------------------
const unset = (await must(admin('GET', '/promotions/progression'), 'progression')).find(c => c.class_id === cls)
check('the demo class starts with no progression, only a suggestion that is not saved', unset.setting === 'unset' && unset.next_class_id === null && unset.suggested_next_class_id !== null, `${unset.setting}, suggested ${unset.suggested_next_class_name}`)
check('a batch for a class with no progression is refused', (await admin('POST', '/promotions', createBody)).code === 'progression_unset')
check('a class cannot be both final and promoted', (await admin('PUT', `/promotions/progression/${cls}`, { next_class_id: next.id, final: true })).status === 400)
const prog = await must(admin('PUT', `/promotions/progression/${cls}`, { next_class_id: next.id }), 'progression')
check('the class now promotes into the new class', prog.find(c => c.class_id === cls)?.setting === 'next' && prog.find(c => c.class_id === cls)?.next_class_id === next.id)
let batch = await must(admin('POST', '/promotions', createBody), 'create')
check('a batch lists every active student of the class', batch.items.length === classStudents.length, `${batch.items.length}`)
const annual = batch.items.filter(i => i.annual_outcome !== null).length
check('without a published annual result students are blocked, not guessed', batch.items.every(i => (i.annual_outcome === null) === (i.recommendation === 'blocked')), `${annual} with an annual result`)
check('a second open batch for the class is refused', (await admin('POST', '/promotions', createBody)).code === 'batch_open')
if (batch.blockers.length) check('approval waits for decisions', (await head('POST', `/promotions/${batch.id}/approve`, { inputs_sha256: batch.inputs_sha256 })).code === 'batch_blocked')

const noReason = await admin('PATCH', `/promotions/${batch.id}/items/${me.id}`, { decision: 'withdraw' })
check('a decision against the recommendation needs a reason', noReason.code === 'reason_required', noReason.code)
for (const it of batch.items) {
  const decision = it.student_id === me.id ? 'withdraw' : 'promote'
  const body = decision === it.recommendation ? { decision } : { decision, reason: 'E2E: decided by the office' }
  batch = await must(admin('PATCH', `/promotions/${batch.id}/items/${it.student_id}`, body), 'decide')
}
check('every student is decided and placed', batch.blockers.length === 0, JSON.stringify(batch.blockers.slice(0, 3)))

// ---- 3. Approve and apply ----------------------------------------------------------
check('the administrator cannot approve', (await admin('POST', `/promotions/${batch.id}/approve`, { inputs_sha256: batch.inputs_sha256 })).status === 403)
check('approval of other inputs is refused', (await head('POST', `/promotions/${batch.id}/approve`, { inputs_sha256: '0'.repeat(64) })).code === 'inputs_changed')
const approved = await must(head('POST', `/promotions/${batch.id}/approve`, { inputs_sha256: batch.inputs_sha256 }), 'approve')
check('the head teacher approves', approved.status === 'approved')
check('the head teacher cannot apply', (await head('POST', `/promotions/${batch.id}/apply`)).status === 403)
const admin2 = await api('admin')
const both = await Promise.all([admin('POST', `/promotions/${batch.id}/apply`), admin2('POST', `/promotions/${batch.id}/apply`)])
check('two applies at once both answer with the applied batch', both.every(x => x.status === 200 && x.data.status === 'applied'), both.map(x => x.status).join(','))

// ---- 4. After promotion -------------------------------------------------------------------
check('the old year\'s tabulations and the guardian\'s report cards are unchanged', (await oldYear()) === before)
const moved = (await must(admin('GET', `/students?session_id=${to.id}&class_id=${next.id}&page=1&page_size=100`), 'moved')).items
check('the rest of the class is in the new class and session', moved.length === classStudents.length - 1, `${moved.length}`)
const relogin = await signIn('student')
check('the withdrawn student can no longer sign in', relogin.status !== 200, `${relogin.status}`)
check('the withdrawn student\'s old token stops working', (await student('GET', '/students/me')).status === 401)
const history = await must(guardian('GET', `/students/${me.id}/enrolments`), 'history')
check('the guardian still reads the withdrawn child\'s history', history.length === 1 && history[0].outcome === 'withdrawn' && history[0].state === 'closed')
const peer = moved[0]
const peerHistory = await must(admin('GET', `/students/${peer.id}/enrolments`), 'peer history')
check('a promoted student has both years', peerHistory.length === 2 && peerHistory[0].outcome === 'promoted' && peerHistory[1].source === 'promotion')
const review = await admin('GET', `/fees/concessions/review?from_session_id=${from}&to_session_id=${to.id}`)
check('the concession review answers for the new session', review.status === 200 && Array.isArray(review.data), `${review.status}`)
const patch = { admission_no: peer.admission_no, name_en: peer.name_en, name_bn: peer.name_bn, date_of_birth: peer.date_of_birth, status: 'active', session_id: from, class_id: cls, section_id: classStudents.find(s => s.id === peer.id).section_id, roll_no: peer.roll_no, user_id: peer.user_id ?? '' }
const generic = await admin('PATCH', `/students/${peer.id}`, patch)
check('a generic edit cannot move a student back into a year they have', generic.status === 409, generic.code)

// ---- 5. Revert --------------------------------------------------------------------------------
check('a revert needs a reason', (await admin('POST', `/promotions/${batch.id}/revert`, { reason: '' })).status === 400)
const reverted = await must(admin('POST', `/promotions/${batch.id}/revert`, { reason: 'E2E run finished' }), 'revert')
check('the batch is reverted', reverted.status === 'reverted')
const back = (await must(admin('GET', `/students?session_id=${from}&class_id=${cls}&status=active&page=1&page_size=100`), 'back')).items
check('every student is back in the old year', back.length === classStudents.length, `${back.length}`)
check('the student can sign in again', (await signIn('student')).status === 200)
check('the old year is still unchanged after the revert', (await oldYear()) === before)
await must(admin('PUT', `/promotions/progression/${cls}`, { next_class_id: null }), 'clear progression')

mkdirSync(join(here, 'shots'), { recursive: true })
r.save(join(here, 'shots', 'promotion.json'))
