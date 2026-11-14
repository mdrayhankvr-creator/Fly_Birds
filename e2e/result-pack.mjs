// The result pack: the four documents a school hands out when results are
// published — the transcript with its signatures placed automatically, the
// class tabulation sheet, the merit list and the admit cards — in English and
// Bangla, with the rules about who may have them, and samples kept for review.
//
//   E2E_BASE_URL=http://localhost:8121 DEMO_PASSWORD=... node e2e/result-pack.mjs
//   RESULT_PACK_SAMPLES=<folder> keeps the PDFs (default e2e/shots/pdf-result-pack)
//
// It needs a class with published results, so run e2e/result-publication.mjs
// first (CI does). Admit cards also need a published exam routine; without
// one the suite checks that the school is told there is nothing to print
// instead, and e2e/marksheet-admit.mjs covers the cards themselves.
//
// It uploads a signature for the demo head teacher; never run it against a
// real school.
import { deflateSync } from 'node:zlib'
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8121'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const samples = process.env.RESULT_PACK_SAMPLES ?? join(here, 'shots', 'pdf-result-pack')
mkdirSync(samples, { recursive: true })
const r = reporter(), check = r.check
const email = who => `demo.${who}@demo.flybirds.test`
const js = v => JSON.stringify(v)
const isPDF = b => !!b && b.subarray(0, 5).toString() === '%PDF-'
const save = (name, pdf) => { writeFileSync(join(samples, name), pdf); return pdf.length }

// A signature-shaped PNG: a dark stroke across a white page, so the upload is
// a real image the backend can trim.
const crcTable = (() => {
  const t = []
  for (let n = 0; n < 256; n++) { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; t[n] = c >>> 0 }
  return t
})()
const crc32 = buf => { let c = 0xffffffff; for (const x of buf) c = crcTable[(c ^ x) & 0xff] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0 }
const chunk = (type, data) => {
  const len = Buffer.alloc(4); len.writeUInt32BE(data.length)
  const body = Buffer.concat([Buffer.from(type, 'ascii'), data])
  const sum = Buffer.alloc(4); sum.writeUInt32BE(crc32(body))
  return Buffer.concat([len, body, sum])
}
function signaturePNG(seed = 0) {
  const w = 160, h = 48, rows = []
  for (let y = 0; y < h; y++) {
    const row = Buffer.alloc(1 + w * 3, 0xff)
    row[0] = 0
    for (let x = 0; x < w; x++) {
      if (Math.abs(Math.sin((x + seed) / 18) * 14 + 24 - y) < 2.2) {
        row[1 + x * 3] = 18; row[2 + x * 3] = 22; row[3 + x * 3] = 70
      }
    }
    rows.push(row)
  }
  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 2
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(Buffer.concat(rows))), chunk('IEND', Buffer.alloc(0))])
}

async function signIn(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: email(who), password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  return async (method, path, body, type) => {
    const headers = { Authorization: `Bearer ${token}` }
    let payload
    if (Buffer.isBuffer(body)) { headers['Content-Type'] = type; payload = body }
    else if (body) { headers['Content-Type'] = 'application/json'; payload = js(body) }
    const response = await fetch(`${base}/api${path}`, { method, headers, body: payload })
    const got = response.headers.get('content-type') || ''
    if (got.includes('application/pdf') || got.includes('image/')) {
      return { status: response.status, bytes: Buffer.from(await response.arrayBuffer()), type: got }
    }
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code, type: got }
  }
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code ?? ''}`); return res.data }

const [admin, head, teacher, teacher3, student, accounts] = await Promise.all(['admin', 'head', 'teacher', 'teacher3', 'student', 'accounts'].map(signIn))

// ---- A class whose results are published --------------------------------------------
const classes = await must(admin('GET', '/exams/documents/classes'), 'classes with published results')
check('a class with published results is there to make the pack from', classes.length > 0,
  classes.length ? `${classes.length}: ${classes.map(c => `${c.class_name} (${c.scope})`).join(', ')}` : 'none — run e2e/result-publication.mjs first')
if (!classes.length) { r.save(join(samples, 'result-pack-checks.json')); process.exit(1) }
const set = classes[0]
const ref = { scope: set.scope, scope_id: set.scope_id, class_id: set.class_id }

// ---- The signatures that the transcript places by itself ----------------------------
// A signature hangs on a staff record, so the head teacher needs one. New
// demo schools are seeded with it; an older one is given it here.
let headStaff = await head('GET', '/staff/me')
if (headStaff.status === 404) {
  const made = await admin('POST', '/staff/employees', {
    employee_code: `DEMO-H${Date.now().toString().slice(-4)}`,
    user_id: (await must(admin('GET', '/users?page=1&page_size=100'), 'the accounts')).items.find(u => u.role === 'head_teacher').id,
    kind: 'teacher', name_en: 'Shahana Begum', name_bn: 'শাহানা বেগম', designation: 'Head Teacher', joined_on: '2015-01-05',
  })
  check('the head teacher can be put on the staff list', made.status === 201 || made.status === 200, `${made.status} ${made.code ?? ''}`)
  headStaff = await head('GET', '/staff/me')
}
check('the head teacher has a staff record to hang a signature on', headStaff.status === 200, `${headStaff.status} ${headStaff.code ?? ''}`)
const png = signaturePNG(3)
const upload = await head('PUT', '/staff/me/signature', png, 'image/png')
check('the head teacher uploads their own signature once', upload.status === 200, `${upload.status} ${upload.code ?? ''}`)
const own = await head('GET', '/staff/me/signature')
check('the head teacher can see their stored signature', own.status === 200 && own.type.includes('image/png'), `${own.status} ${own.type}`)
const stolen = await fetch(`${base}/api/staff/me/signature`)
check('a signature is never public', stolen.status === 401, String(stolen.status))
const byStudent = await student('GET', '/staff/me/signature')
check('a student cannot read a signature', byStudent.status === 403, `${byStudent.status} ${byStudent.code}`)

// ---- The four documents, English and Bangla -----------------------------------------
async function document(who, input, file) {
  let doc = await must(who('POST', '/exams/documents', input), `request ${input.kind} ${input.language}`)
  for (let i = 0; i < 90 && (doc.status === 'queued' || doc.status === 'running'); i++) {
    await sleep(700)
    doc = await must(who('GET', `/exams/documents/${doc.id}`), 'poll')
  }
  const got = await who('GET', `/exams/documents/${doc.id}/file`)
  const ok = got.status === 200 && isPDF(got.bytes)
  if (ok) save(file, got.bytes)
  return { ok, doc, bytes: got.bytes, detail: `${doc.status} ${got.status} ${got.bytes?.length ?? 0}B` }
}

const sheets = {}
for (const lang of ['en', 'bn']) {
  const tab = await document(admin, { kind: 'tabulation', ...ref, language: lang }, `tabulation-${lang}.pdf`)
  check(`the tabulation sheet renders in ${lang}`, tab.ok, tab.detail)
  sheets[lang] = tab.bytes
  const merit = await document(admin, { kind: 'merit_list', ...ref, language: lang }, `merit-list-${lang}.pdf`)
  check(`the merit list renders in ${lang}`, merit.ok, merit.detail)
}

// The sheet is landscape: its page is wider than it is tall.
const box = (sheets.en?.toString('latin1') ?? '').match(/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)/)
check('the tabulation sheet is landscape', !!box && Number(box[1]) > Number(box[2]), box ? `${box[1]} x ${box[2]}` : 'no MediaBox')

// One student's transcript, with the signature boxes the school never fills
// in by hand. A withheld result prints only the withholding notice, with no
// boxes at all, so the sample is of a student who sat and passed.
const review = await must(head('GET', `/exams/results/review?scope=${set.scope}&scope_id=${set.scope_id}&class_id=${set.class_id}`), 'the published results')
const sat = review.students.find(s => s.outputs.outcome === 'passed') ?? review.students[0]
check('a student who sat the exam is on the sheet to make a transcript of', sat.outputs.outcome === 'passed',
  `${sat.name}: ${sat.outputs.outcome}`)
const me = await must(student('GET', '/students/me'), 'the demo student')
const mine = await must(student('GET', '/exams/me/term-results'), 'my results')
const published = mine.find(v => v.term_id === set.scope_id) ?? mine[0]
check('the demo student has a published result of their own', !!published, `${mine.length} results`)
for (const lang of ['en', 'bn']) {
  const sheet = await document(admin, { kind: 'marksheet', ...ref, student_id: sat.student_id, language: lang }, `transcript-${lang}.pdf`)
  check(`a transcript renders in ${lang}`, sheet.ok, sheet.detail)
  if (!published) continue
  const own = await student('GET', `/exams/me/documents/${published.id}?lang=${lang}`)
  const ownOK = own.status === 200 && isPDF(own.bytes)
  if (ownOK) save(`transcript-own-${lang}.pdf`, own.bytes)
  check(`a student downloads their own transcript in ${lang}`, ownOK, `${own.status} ${own.bytes?.length ?? 0}B`)
}

// The head teacher's signature is really on the page, and replacing it makes
// a new transcript rather than handing back the cached one: the image is part
// of what the document depends on. Signed, the page carries one image more
// (the signature beside the QR code) and is bigger.
const signed = await document(admin, { kind: 'marksheet', ...ref, student_id: sat.student_id, language: 'en' }, 'transcript-en.pdf')
const imagesIn = pdf => (pdf?.toString('latin1').match(/\/Subtype\s*\/Image/g) ?? []).length
check('the head teacher removes their signature again', (await head('DELETE', '/staff/me/signature')).status === 200)
const unsigned = await document(admin, { kind: 'marksheet', ...ref, student_id: sat.student_id, language: 'en' }, 'transcript-en-unsigned.pdf')
check('the uploaded signature is printed in the head teacher\'s box', imagesIn(signed.bytes) > imagesIn(unsigned.bytes) && signed.bytes.length > unsigned.bytes.length,
  `signed ${imagesIn(signed.bytes)} images / ${signed.bytes?.length}B, unsigned ${imagesIn(unsigned.bytes)} / ${unsigned.bytes?.length}B`)
check('a changed signature is a new transcript, not the cached one', signed.doc.id !== unsigned.doc.id,
  `${signed.doc.id?.slice(0, 8)} then ${unsigned.doc.id?.slice(0, 8)}`)
check('the head teacher signs again for the samples', (await head('PUT', '/staff/me/signature', signaturePNG(3), 'image/png')).status === 200)
const resigned = await document(admin, { kind: 'marksheet', ...ref, student_id: sat.student_id, language: 'en' }, 'transcript-en.pdf')
check('the transcript is signed again', resigned.ok && imagesIn(resigned.bytes) === imagesIn(signed.bytes), resigned.detail)

// ---- Who may have what ---------------------------------------------------------------
const theirs = await must(teacher3('GET', '/exams/documents/classes'), 'a teacher\'s own classes')
const outside = !theirs.some(c => c.class_id === set.class_id)
const refused = await teacher3('POST', '/exams/documents', { kind: 'tabulation', ...ref, language: 'en' })
check('a teacher gets only their own classes', outside ? refused.status === 403 : refused.status === 200,
  `${theirs.length} own classes, asking for another: ${refused.status} ${refused.code ?? ''}`)
const byAccounts = await accounts('POST', '/exams/documents', { kind: 'merit_list', ...ref, language: 'en' })
check('the accountant has no result documents', byAccounts.status === 403, `${byAccounts.status} ${byAccounts.code}`)
const studentSheet = await student('POST', '/exams/documents', { kind: 'tabulation', ...ref, language: 'en' })
const studentList = await student('GET', '/exams/documents/classes')
check('a student cannot have a class document', studentSheet.status === 403 && studentList.status === 403, `${studentSheet.status} / ${studentList.status}`)
const ownTeacher = await teacher('GET', '/exams/documents/classes')
check('a class teacher can reach the document screen', ownTeacher.status === 200, String(ownTeacher.status))

// ---- Admit cards ---------------------------------------------------------------------
const terms = await must(admin('GET', '/routines/admit-cards/terms'), 'terms with a published routine')
if (terms.length > 0) {
  const term = terms[0]
  for (const lang of ['en', 'bn']) {
    const cards = await admin('GET', `/routines/admit-cards/terms/${term.id}/pdf?class_id=${set.class_id}&lang=${lang}`)
    const ok = cards.status === 200 && isPDF(cards.bytes)
    if (ok) save(`admit-cards-${lang}.pdf`, cards.bytes)
    check(`the admit cards of a class print in ${lang}`, ok, `${cards.status} ${cards.bytes?.length ?? 0}B`)
  }
  const mineCard = await student('GET', '/routines/admit-cards/me')
  check('a student can reach their own admit card', mineCard.status === 200 || mineCard.status === 404, `${mineCard.status} ${mineCard.code ?? ''}`)
} else {
  const nothing = await admin('GET', `/routines/admit-cards/terms/${set.scope_id}/pdf?class_id=${set.class_id}&lang=en`)
  check('without a published routine the school is told there are no cards to print', nothing.status === 404 || nothing.status === 400,
    `no term has a published routine; asking anyway answers ${nothing.status} ${nothing.code ?? ''} (the cards themselves are covered by marksheet-admit.mjs)`)
}

r.save(join(samples, 'result-pack-checks.json'))
