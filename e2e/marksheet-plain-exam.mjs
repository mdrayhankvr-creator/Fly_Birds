// A marksheet for an exam run the simple way: subjects marked out of their
// full marks, with no component breakdown and no blueprint. That is what a
// school gets from Exams → create an exam → add subjects, and it is the most
// common setup; it leaves the published result with no component columns.
// Checks the staff download and the student's own, in English and Bangla,
// while the exam is published and again once it is locked.
//
//   DEMO_PASSWORD=...  E2E_BASE_URL=http://localhost:8121  node e2e/marksheet-plain-exam.mjs
//   E2E_PDF_DIR=<folder> keeps the PDFs (default e2e/shots/plain-exam)
import { launch, reporter, sleep } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { mkdirSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const out = process.env.E2E_PDF_DIR || join(here, 'shots', 'plain-exam')
mkdirSync(out, { recursive: true })
const pw = process.env.DEMO_PASSWORD
const r = reporter(), check = r.check
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://localhost:8121', shots: out })
const email = who => `demo.${who}@demo.flybirds.test`
const stamp = String(Date.now()).slice(-8)

const must = async (promise, what) => {
  const res = await promise
  if (res.status >= 300) throw new Error(`${what}: ${res.status} ${JSON.stringify(res.body).slice(0, 300)}`)
  return res.body.data
}
const api = (path, body, method = 'POST') => b.api(path, body === undefined
  ? { method: 'GET' }
  : { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
const isPDF = bytes => bytes.startsWith('%PDF-')
// A PDF's pages, counted the way the Go side does.
const pages = bytes => (bytes.match(/\/Type\s*\/Page[^s]/g) || []).length || 1

// Downloads a document the browser's own session is allowed to fetch.
async function fetchDoc(id) {
  const got = await b.evaluate(`(async () => {
    const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1])
    const t = await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })
    const tok = (await t.json()).data.access_token
    const res = await fetch('/api/exams/documents/${id}/file', { headers: { Authorization: 'Bearer ' + tok } })
    if (!res.ok) return { status: res.status, body: '' }
    const buf = new Uint8Array(await res.arrayBuffer())
    let s = ''
    for (const byte of buf) s += String.fromCharCode(byte)
    return { status: res.status, body: s }
  })()`)
  return got
}

try {
  // -------------------------------------------------------------------------
  // 1. An exam created the simple way, with no components anywhere.

  await b.login(email('admin'), pw)
  const me = await must(api('/auth/me'), 'me')
  const roster = (await must(api('/students?page=1&page_size=100&status=active'), 'students')).items
  const first = roster[0]
  const cls = first.class_id, section = first.section_id, session = first.session_id
  const mine = roster.filter(s => s.class_id === cls && s.section_id === section)
  check('the demo school has a section to examine', mine.length >= 2, `${mine.length} student(s)`)

  const exam = await must(api('/exams', {
    name: `Plain Half Yearly ${stamp}`, exam_type: 'half_yearly',
    session_id: session, class_id: cls, section_id: section,
  }), 'create the exam')
  const mapping = await must(api(`/academic/classes/${cls}/subjects`), 'class subjects')
  const subjects = (Array.isArray(mapping) ? mapping : mapping.items).filter(m => !m.is_fourth_subject).slice(0, 3)
  check('three ordinary subjects are mapped to the class', subjects.length === 3, `${subjects.length}`)
  for (const m of subjects) {
    await must(api(`/exams/${exam.id}/subjects`, { subject_id: m.subject_id, full_marks: 100, pass_marks: 33 }), `add ${m.subject_id}`)
  }
  const full = await must(api(`/exams/${exam.id}`), 'exam')
  check('every subject is marked out of 100 with no components', full.subjects.length === 3 &&
    full.subjects.every(s => s.full_marks === 100 && !s.components), `${full.subjects.length} subject(s)`)

  // -------------------------------------------------------------------------
  // 2. Marks for the whole section, then publish.

  const sheet = await must(api(`/exams/${exam.id}/roster`), 'roster')
  const rows = Array.isArray(sheet) ? sheet : sheet.items ?? sheet.rows
  const entries = rows.map((row, i) => ({
    exam_subject_id: row.exam_subject_id, student_id: row.student_id,
    marks: [78, 85, 71, 64, 92, 56][i % 6],
  }))
  await must(api(`/exams/${exam.id}/marks`, { entries }, 'PUT'), 'enter marks')
  await must(api(`/exams/${exam.id}/state`, { status: 'published', reason: 'plain exam check' }, 'PATCH'), 'publish')
  const published = await must(api(`/exams/${exam.id}`), 'exam after publishing')
  check('the exam is published', published.status === 'published', published.status)

  // -------------------------------------------------------------------------
  // 3. The staff marksheet, in both languages. This is what used to answer
  //    "The document could not be prepared".

  const docs = {}
  for (const language of ['en', 'bn']) {
    let doc = await must(api('/exams/documents', {
      kind: 'marksheet', scope: 'assessment', scope_id: exam.id,
      class_id: cls, student_id: first.id, language,
    }), `request the ${language} marksheet`)
    for (let i = 0; i < 40 && doc.status !== 'done' && doc.status !== 'failed'; i++) {
      await sleep(500)
      doc = await must(api(`/exams/documents/${doc.id}`), 'poll')
    }
    check(`the ${language} marksheet of an exam with no components is prepared`, doc.status === 'done',
      `${doc.status}${doc.error ? ': ' + String(doc.error).slice(0, 160) : ''}`)
    docs[language] = doc
    if (doc.status !== 'done') continue
    const file = await fetchDoc(doc.id)
    check(`the ${language} marksheet is a one-page PDF`, isPDF(file.body) && pages(file.body) === 1,
      `${file.status}, ${file.body.length} bytes, ${pages(file.body)} page(s)`)
    writeFileSync(join(out, `plain-marksheet-${language}.pdf`), Buffer.from(file.body, 'binary'))
  }

  // -------------------------------------------------------------------------
  // 4. The student's own download of the same result.

  const version = await must(api(`/exams/results/history?scope=assessment&scope_id=${exam.id}&student_id=${first.id}`), 'history')
  const live = (Array.isArray(version) ? version : []).find(v => v.status === 'published')
  check('the exam published a result version for the student', !!live, `${(version || []).length} version(s)`)

  // -------------------------------------------------------------------------
  // 5. Locked: marks are frozen, and the marksheet must still come out.

  await must(api(`/exams/${exam.id}/state`, { status: 'locked', reason: 'plain exam check' }, 'PATCH'), 'lock')
  const locked = await must(api(`/exams/${exam.id}`), 'exam after locking')
  check('the exam is locked', locked.status === 'locked', locked.status)
  let again = await must(api('/exams/documents', {
    kind: 'marksheet', scope: 'assessment', scope_id: exam.id,
    class_id: cls, student_id: first.id, language: 'en',
  }), 'request the marksheet of a locked exam')
  for (let i = 0; i < 40 && again.status !== 'done' && again.status !== 'failed'; i++) {
    await sleep(500)
    again = await must(api(`/exams/documents/${again.id}`), 'poll')
  }
  check('a locked exam still prepares its marksheet', again.status === 'done',
    `${again.status}${again.error ? ': ' + String(again.error).slice(0, 160) : ''}`)

  // -------------------------------------------------------------------------
  // 6. Another student of the same section, so this is not one lucky row.

  const other = mine.find(s => s.id !== first.id)
  let second = await must(api('/exams/documents', {
    kind: 'marksheet', scope: 'assessment', scope_id: exam.id,
    class_id: cls, student_id: other.id, language: 'bn',
  }), 'request a second student')
  for (let i = 0; i < 40 && second.status !== 'done' && second.status !== 'failed'; i++) {
    await sleep(500)
    second = await must(api(`/exams/documents/${second.id}`), 'poll')
  }
  check('a second student of the section is prepared too', second.status === 'done',
    `${second.status}${second.error ? ': ' + String(second.error).slice(0, 160) : ''}`)

  console.log(`\nexam ${exam.id} (${published.name}) left locked on this school; PDFs in ${out}`)
} finally {
  r.save(join(out, 'plain-exam-checks.json'))
  await b.close()
}
