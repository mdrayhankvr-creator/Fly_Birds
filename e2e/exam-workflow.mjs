// E3 exam workflow on a running, demo-seeded disposable school: mark
// statuses, per-cell errors, version conflicts, sheet submit / return /
// approve, publication readiness, withheld results and who can see what.
//
//   E2E_BASE_URL=http://localhost:8103 DEMO_PASSWORD=... node e2e/exam-workflow.mjs
//
// It creates and publishes a new exam; never run it against a real school.
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)

async function signIn(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${who}@demo.flybirds.test`, password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
const [admin, head, teacher, teacher2, student, guardian] = await Promise.all(['admin', 'head', 'teacher', 'teacher2', 'student', 'guardian'].map(signIn))
const me = (await student('GET', '/students/me')).data
const subjectOf = async t => (await t('GET', '/staff/me/assignments?page=1&page_size=100')).data.items.find(a => a.class_id === me.class_id && a.session_id === me.session_id).subject_id
const [subjectA, subjectB] = [await subjectOf(teacher), await subjectOf(teacher2)]

// The administrator sets up an exam in the demo student's section. It is a
// term exam: class tests are never shown to students on their own (E1-06,
// checked by e2e/exam-setup.mjs), and this suite checks what students see.
const name = `E3 Workflow ${Date.now()}`
const created = await admin('POST', '/exams', { name, exam_type: 'half_yearly', session_id: me.session_id, class_id: me.class_id, section_id: me.section_id, routine_id: null })
const examID = created.data.id
await admin('POST', `/exams/${examID}/subjects`, { subject_id: subjectA, full_marks: 50, pass_marks: 17 })
const exam = (await admin('POST', `/exams/${examID}/subjects`, { subject_id: subjectB, full_marks: 100, pass_marks: 33 })).data
const sheetA = exam.subjects.find(s => s.subject_id === subjectA).id, sheetB = exam.subjects.find(s => s.subject_id === subjectB).id

// Teachers see their own sheets only.
const mine = await teacher('GET', `/exams/assigned/${examID}/sheets`)
check('a teacher sees only the sheet of the assigned subject', mine.status === 200 && mine.data.sheets.length === 1 && mine.data.sheets[0].id === sheetA && mine.data.sheets[0].status === 'not_started', `${mine.data?.sheets.length} sheet(s)`)
const rows = (await teacher('GET', `/exams/${examID}/assigned-roster`)).data
const students = [...new Set(rows.map(row => row.student_id))]
check('the roster carries status and version for every cell', rows.every(row => row.status === '' && row.row_version === 0), `${rows.length} cells`)

// Per-cell errors: one bad mark does not stop the others.
const first = await teacher('PATCH', `/exams/${examID}/marks`, { entries: students.map((id, i) => ({ exam_subject_id: sheetA, student_id: id, status: 'present', marks: i === 0 ? 51 : 30 + i, row_version: 0 })) })
check('valid cells save while an invalid one is reported (above full marks)', first.status === 200 && first.data.saved.length === students.length - 1 && first.data.errors.length === 1 && first.data.errors[0].code === 'above_full_marks', `${first.data?.saved.length} saved, ${first.data?.errors.map(e => e.code)}`)
const exempt = await teacher('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetA, student_id: students[0], status: 'exempt', marks: null, reason: 'medical', row_version: 0 }] })
check('a teacher cannot exempt a student', exempt.data?.errors[0]?.code === 'status_not_allowed', exempt.data?.errors[0]?.code)
const fixed = await teacher('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetA, student_id: students[0], status: 'present', marks: 45.5, row_version: 0 }, { exam_subject_id: sheetA, student_id: students[1], status: 'absent', marks: null, row_version: 1 }] })
check('the teacher corrects the cell and marks a student absent', fixed.data?.saved.length === 2 && fixed.data.saved.find(c => c.student_id === students[1]).status === 'absent', JSON.stringify(fixed.data?.errors))

// Optimistic concurrency: a stale write never overwrites.
const fresh = await teacher('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetA, student_id: students[2], status: 'present', marks: 40, row_version: 1 }] })
const stale = await admin('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetA, student_id: students[2], status: 'present', marks: 20, row_version: 1 }] })
check('a stale version is a conflict that shows the current value', fresh.data?.saved[0]?.row_version === 2 && stale.data?.errors[0]?.code === 'version_conflict' && stale.data.errors[0].current.marks === 40, `current ${stale.data?.errors[0]?.current?.marks}`)

// Students and guardians never reach drafts or sheets.
const denied = await Promise.all([student('GET', `/exams/${examID}/sheets`), guardian('GET', `/exams/${examID}/sheets`), student('PATCH', `/exams/${examID}/marks`, { entries: [] }), student('GET', `/exams/me/results/${examID}`)])
check('students and guardians cannot read sheets, write marks or see the draft', denied[0].status === 403 && denied[1].status === 403 && denied[2].status === 403 && denied[3].status === 404, denied.map(d => d.status).join(','))

// Submit → read-only → return with a reason → resubmit → approve.
const submitted = await teacher('POST', `/exams/${examID}/sheets/${sheetA}/submit`)
const locked = await teacher('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetA, student_id: students[3], status: 'present', marks: 10, row_version: 1 }] })
check('a submitted sheet is read-only for the teacher', submitted.status === 200 && submitted.data.status === 'submitted' && locked.data?.errors[0]?.code === 'sheet_locked', `${submitted.data?.status} / ${locked.data?.errors[0]?.code}`)
const early = await admin('PATCH', `/exams/${examID}/state`, { status: 'published' })
check('publication waits for sheets under review', early.status === 409 && early.code === 'exam_sheets_pending', `${early.status} ${early.code}`)
const teacherApprove = await teacher('POST', `/exams/${examID}/sheets/${sheetA}/approve`)
const returned = await head('POST', `/exams/${examID}/sheets/${sheetA}/return`, { reason: 'Please recheck the second script' })
const seen = (await teacher('GET', `/exams/assigned/${examID}/sheets`)).data.sheets[0]
check('the head teacher returns it with a reason the teacher sees; teachers cannot approve', teacherApprove.status === 403 && returned.data?.status === 'returned' && seen.return_reason === 'Please recheck the second script', `${teacherApprove.status} ${seen.status}: ${seen.return_reason}`)
await teacher('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetA, student_id: students[3], status: 'present', marks: 33.5, row_version: 1 }] })
const resubmitted = await teacher('POST', `/exams/${examID}/sheets/${sheetA}/submit`)
const approved = await head('POST', `/exams/${examID}/sheets/${sheetA}/approve`)
check('after correction the sheet is resubmitted and approved', resubmitted.data?.status === 'submitted' && approved.data?.status === 'approved' && approved.data.reviewed_by !== '', approved.data?.reviewed_by)

// The second teacher's sheet, then a withheld result for the demo student.
const fillB = await teacher2('PATCH', `/exams/${examID}/marks`, { entries: students.map((id, i) => ({ exam_subject_id: sheetB, student_id: id, status: 'present', marks: 60 + i, row_version: 0 })) })
const ready = (await admin('GET', `/exams/${examID}/sheets`)).data
const withheld = await admin('PATCH', `/exams/${examID}/marks`, { entries: [{ exam_subject_id: sheetB, student_id: me.id, status: 'withheld', marks: 60 + students.indexOf(me.id), reason: 'Under review by the exam committee', row_version: 1 }] })
check('the administrator withholds one result with a reason; the exam is ready to publish', fillB.data?.saved.length === students.length && withheld.data?.saved[0]?.status === 'withheld' && ready.readiness.ready, JSON.stringify(ready.readiness.blockers))
const published = await admin('PATCH', `/exams/${examID}/state`, { status: 'published' })
const after = (await admin('GET', `/exams/${examID}/sheets`)).data
check('publication approves every sheet', published.status === 200 && after.sheets.every(s => s.status === 'approved'), after.sheets.map(s => s.status).join(','))

// What families see.
const studentCard = await student('GET', `/exams/me/results/${examID}`)
const guardianCard = await guardian('GET', `/exams/children/${me.id}/results/${examID}`)
check('the student and guardian see "withheld" and no marks', studentCard.data?.withheld === true && studentCard.data.subjects.length === 0 && guardianCard.data?.withheld === true, `${studentCard.status} ${guardianCard.status}`)
const absentCard = (await admin('GET', `/exams/${examID}/results/${students[1]}`)).data
const absentRow = absentCard.subjects.find(s => s.status === 'absent')
check('an absent required subject is graded F and fails the result', absentRow?.grade === 'F' && absentCard.result.passed === false && absentCard.result.gpa === 0, `${absentRow?.grade} ${absentCard.result.gpa}`)

// Browser: the head teacher's sheet panel, and the student's withheld card at 390 px.
const b = await launch({ base, shots: join(here, 'shots'), port: 9474 })
await b.viewport(1280, 900)
await b.login('demo.head@demo.flybirds.test', password)
await b.go('/admin/exams'); await b.waitFor(`document.querySelector('main') && document.querySelector('main').innerText.includes('Exams')`)
await b.evaluate(`document.querySelector('main input').value=''`)
await b.waitFor(`[...document.querySelectorAll('main button')].some(x => x.innerText.includes(${JSON.stringify(name)}))`)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.includes(${JSON.stringify(name)})).click()`)
await b.waitFor(`document.querySelector('main').innerText.includes('Mark sheets')`)
await sleep(500)
const panel = await b.evaluate(`document.querySelector('main').innerText`)
check('the head teacher sees the sheet panel with statuses and no publish button', panel.includes('Mark sheets') && panel.includes('Approved') && !panel.includes('Publish results') && !panel.includes('Save marks'), 'sheet panel')
await b.shot('exam-workflow-head-teacher')
b.close()
const s = await launch({ base, shots: join(here, 'shots'), port: 9475 })
await s.viewport(390, 844, true)
await s.login('demo.student@demo.flybirds.test', password)
await s.go('/student/results'); await s.waitFor(`document.querySelector('main').innerText.includes(${JSON.stringify(name)})`)
const results = await s.evaluate(`document.querySelector('main').innerText`)
const overflow = await s.evaluate(`document.documentElement.scrollWidth > window.innerWidth`)
check('the student results page shows the withheld result without marks at 390 px', results.includes('Result withheld') && !overflow, `overflow ${overflow}`)
await s.shot('exam-workflow-student-withheld')
s.close()

r.save(join(here, 'shots', 'exam-workflow.json'))
