// E7: result documents in a browser, on a running, demo-seeded disposable
// school. It makes the demo class a realistic one (60 students, nine
// subjects with CQ/MCQ/practical, a fourth subject, class tests), publishes a
// term through the API, then checks: the provisional preview (head teacher
// only), students' own downloads in English and Bangla, class documents for
// the administrator and the class's teacher (the 60-student bundle in the
// background with progress), who may not have what (server side), the public
// QR check (no personal data), a correction making new documents and the old
// QR saying "superseded", the blueprint presets and copying another class's
// blueprint, and the new screens at 1440px and 375px in both languages and
// themes with overflow, contrast and axe checks.
//
//   E2E_BASE_URL=http://localhost:8121 DEMO_PASSWORD=... node e2e/result-documents.mjs
//   E7_SAMPLES=<folder>  keeps the sample PDFs (default e2e/shots/pdf-e7)
//   E2E_DB_CONTAINER=<name> the stack's database container, to read one QR
//                           token (default m15b1-db-1); without it that check is skipped
//
// It adds subjects, students, a term and a blueprint to the demo class; never
// run it against a real school.
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const shots = process.env.E2E_SHOTS ?? join(here, 'shots')
const samples = process.env.E7_SAMPLES ?? join(shots, 'pdf-e7')
const dbContainer = process.env.E2E_DB_CONTAINER ?? 'm15b1-db-1'
mkdirSync(samples, { recursive: true })
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const stamp = Date.now().toString(36).slice(-4).toUpperCase()
const email = who => `demo.${who}@demo.flybirds.test`
const js = v => JSON.stringify(v)
const metrics = {}

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: email(who), password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  const call = async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? js(body) : undefined })
    const type = response.headers.get('content-type') || ''
    if (type.includes('application/pdf')) return { status: response.status, pdf: Buffer.from(await response.arrayBuffer()), disposition: response.headers.get('content-disposition') }
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
  return call
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }
const [admin, head, teacher, student, accounts] = await Promise.all(['admin', 'head', 'teacher', 'student', 'accounts'].map(api))
const isPDF = b => !!b && b.subarray(0, 5).toString() === '%PDF-'

// ---- 0. A realistic class, through the API -------------------------------------------
const me = await must(student('GET', '/students/me'), 'me')
const session = me.session_id, cls = me.class_id
const added = [
  ['GSC', 'General Science', 'সাধারণ বিজ্ঞান', 'prac'], ['BGS', 'Bangladesh and Global Studies', 'বাংলাদেশ ও বিশ্বপরিচয়', 'cq'],
  ['ICT', 'Information and Communication Technology', 'তথ্য ও যোগাযোগ প্রযুক্তি', 'prac'], ['REL', 'Islam and Moral Education', 'ইসলাম ও নৈতিক শিক্ষা', 'cq'],
  ['PEH', 'Physical Education and Health', 'শারীরিক শিক্ষা ও স্বাস্থ্য', 'cq'], ['AGR', 'Agriculture Studies', 'কৃষিশিক্ষা', 'prac', 'fourth'],
]
const subjectIds = {}
for (const [code, en, bn, , fourth] of added) {
  const s = await must(admin('POST', '/academic/subjects', { name_en: en, name_bn: bn, code: `${code}${stamp}`, is_active: true }), `subject ${code}`)
  subjectIds[code] = s.id
  await must(admin('POST', `/academic/classes/${cls}/subjects`, { subject_id: s.id, is_required: !fourth, is_fourth_subject: !!fourth }), `map ${code}`)
}
// 60 students: the demo class has 12 in two sections; 24 more in each.
const first = ['Arif', 'Nusrat', 'Tanvir', 'Sadia', 'Rakib', 'Farzana', 'Imran', 'Mitu', 'Shakil', 'Jannat', 'Rafiq', 'Sumaiya', 'Hasan', 'Tania', 'Mahin', 'Anika']
const firstBN = ['আরিফ', 'নুসরাত', 'তানভীর', 'সাদিয়া', 'রাকিব', 'ফারজানা', 'ইমরান', 'মিতু', 'শাকিল', 'জান্নাত', 'রফিক', 'সুমাইয়া', 'হাসান', 'তানিয়া', 'মাহিন', 'আনিকা']
const last = ['Rahman', 'Akter', 'Hossain', 'Islam', 'Chowdhury', 'Saha', 'Ahmed', 'Begum']
const lastBN = ['রহমান', 'আক্তার', 'হোসেন', 'ইসলাম', 'চৌধুরী', 'সাহা', 'আহমেদ', 'বেগম']
const sections = (await must(admin('GET', `/academic/sections?page=1&page_size=100&session_id=${session}&class_id=${cls}`), 'sections')).items.filter(s => s.is_active)
let n = 0
for (const section of sections) {
  const existing = (await must(admin('GET', `/students?page=1&page_size=100&session_id=${session}&class_id=${cls}&section_id=${section.id}`), 'students')).items
  let roll = Math.max(0, ...existing.map(s => s.roll_no || 0))
  for (let i = existing.length; i < 30; i++) {
    n++; roll++
    await must(admin('POST', '/students', { admission_no: `E7-${stamp}-${String(n).padStart(3, '0')}`, name_en: `${first[n % 16]} ${last[(n * 3) % 8]}`, name_bn: `${firstBN[n % 16]} ${lastBN[(n * 3) % 8]}`,
      date_of_birth: '', status: 'active', session_id: session, class_id: cls, section_id: section.id, roll_no: roll }), 'student')
  }
}
const roster = (await must(admin('GET', `/students?page=1&page_size=100&session_id=${session}&class_id=${cls}&status=active`), 'class')).items
check('the class has 60 students in two sections', roster.length === 60 && sections.length === 2, `${roster.length} students`)
// Half the class takes the fourth subject, the demo student among them.
const takers = roster.filter((s, i) => s.id === me.id || i % 2 === 0)
await must(admin('PUT', '/exams/subject-choices', { session_id: session, class_id: cls, choices: takers.map(s => ({ student_id: s.id, subject_id: subjectIds.AGR })) }), 'choices')

// A term with a class test (20 %) and a term exam (80 %); CQ/MCQ/practical.
const term = await must(admin('POST', '/exams/terms', { session_id: session, name: `Second Term ${stamp}`, kind: 'term', starts_on: '', ends_on: '' }), 'term')
const classes = await must(admin('GET', `/exams/blueprints?session_id=${session}`), 'blueprints')
const mine = classes.find(c => c.class_id === cls)
if (mine.draft) await must(admin('DELETE', `/exams/blueprint-versions/${mine.draft.id}`), 'discard a leftover draft')
const draft = mine.active ? await must(admin('POST', `/exams/blueprints/${mine.blueprint_id}/versions`, { from_version_id: mine.active.id }), 'new version')
  : await must(admin('POST', '/exams/blueprints', { session_id: session, class_id: cls }), 'new blueprint')
const d = draft.definition
const CT = `C${stamp}`, EX = `X${stamp}`
const termKeys = new Set(d.assessments.filter(a => a.term_id === term.id).map(a => a.key).concat([CT, EX]))
d.assessments = d.assessments.filter(a => a.term_id !== term.id).concat([
  { key: CT, name: 'Class test', term_id: term.id, kind: 'class_test', visibility: 'hidden' },
  { key: EX, name: 'Second term exam', term_id: term.id, kind: 'term_exam', visibility: 'after_term_publication' },
])
const pattern = {}
for (const [code, , , kind] of added) pattern[subjectIds[code]] = kind
for (const s of d.subjects) {
  if (s.role === 'non_graded') continue
  const components = pattern[s.subject_id] === 'prac' ? [{ key: 'CQ', full: 50, pass: 17 }, { key: 'MCQ', full: 25, pass: 8 }, { key: 'PRAC', full: 25, pass: 8 }] : [{ key: 'CQ', full: 70, pass: 23 }, { key: 'MCQ', full: 30, pass: 10 }]
  s.sheets = s.sheets.filter(x => !termKeys.has(x.assessment)).concat([{ assessment: CT, pass: 7, components: [{ key: 'TOTAL', full: 20 }] }, { assessment: EX, pass: 33, components }])
  s.terms = s.terms.filter(x => x.term_id !== term.id).concat([{ term_id: term.id, full: 100, pass: 33, contributions: [{ from: [CT], combine: 'average', worth: 20 }, { from: [EX], combine: 'single', worth: 80 }] }])
}
const saved = await must(admin('PUT', `/exams/blueprint-versions/${draft.id}`, { definition: d }), 'save draft')
check('the blueprint with CQ, MCQ, practical and class tests validates', saved.issues.length === 0, saved.issues.map(i => i.message).slice(0, 3).join(' | '))
await must(admin('POST', `/exams/blueprint-versions/${draft.id}/activate`, {}), 'activate')
await must(admin('PUT', `/exams/terms/${term.id}/setup`, { class_ids: [cls], due: { [CT]: '2030-01-15', [EX]: '2030-02-15' }, assignees: {} }), 'setup')
await must(admin('POST', `/exams/terms/${term.id}/create`, {}), 'create')
const listed = (await must(admin('GET', `/exams?page=1&page_size=100&session_id=${session}&class_id=${cls}`), 'exams')).items.filter(e => e.term_id === term.id)
const exams = await Promise.all(listed.map(e => must(admin('GET', `/exams/${e.id}`), 'exam')))
// Marks: deterministic; one student fails Mathematics on CQ, one is withheld.
const ordered = [...roster].sort((a, b) => a.section_name.localeCompare(b.section_name) || (a.roll_no ?? 0) - (b.roll_no ?? 0))
const failing = ordered.find(s => s.id !== me.id && !takers.includes(s))
const withheld = ordered.find(s => s.id !== me.id && s.id !== failing.id && takers.includes(s))
const mapping = await must(admin('GET', `/academic/classes/${cls}/subjects`), 'mapping')
const mathId = (Array.isArray(mapping) ? mapping : mapping.items).find(m => /^MATH/.test(m.code || m.subject_code || '') || /Mathematics/.test(m.name_en || m.subject_name_en || ''))?.subject_id
for (const exam of exams) {
  const rows = await must(admin('GET', `/exams/${exam.id}/roster`), 'roster')
  const entries = rows.map(row => {
    const i = ordered.findIndex(s => s.id === row.student_id)
    if (exam.assessment_key === CT) return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'present', marks: 10 + (i * 3) % 11, row_version: row.row_version }
    const sheet = exam.subjects.find(s => s.id === row.exam_subject_id)
    if (row.student_id === withheld.id && sheet.subject_id === subjectIds.GSC) return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'withheld', reason: 'Exam committee review', row_version: row.row_version }
    const strong = row.student_id === me.id ? 0.92 : 0.45 + ((i * 7 + sheet.subject_id.charCodeAt(0)) % 50) / 100
    const prac = pattern[sheet.subject_id] === 'prac'
    const cq = row.student_id === failing.id && sheet.subject_id === (mathId ?? sheet.subject_id) ? 12 : Math.round((prac ? 50 : 70) * strong)
    const components = prac ? { CQ: cq, MCQ: Math.round(25 * strong), PRAC: Math.min(25, Math.round(25 * strong) + 2) } : { CQ: cq, MCQ: Math.round(30 * strong) }
    return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'present', components, row_version: row.row_version }
  })
  for (let k = 0; k < entries.length; k += 400) {
    const res = await must(admin('PATCH', `/exams/${exam.id}/marks`, { entries: entries.slice(k, k + 400) }), 'marks')
    if (res.errors.length) throw new Error(`marks for ${exam.name}: ${js(res.errors.slice(0, 2))}`)
  }
}
for (const exam of exams) await must(admin('PATCH', `/exams/${exam.id}/state`, { status: 'published' }), `publish ${exam.name}`)
const ref = { scope: 'term', scope_id: term.id, class_id: cls }
const reviewPath = `/admin/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}&session=${session}`

// ---- Browser helpers --------------------------------------------------------------
const axePath = join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
const contrast = `(() => {
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  const rgb = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = '#000'; ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const opaque = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return ctx.getImageData(0, 0, 1, 1).data[3] > 250 }
  const bg = el => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (c && opaque(c)) return c } return getComputedStyle(document.body).backgroundColor }
  const ratio = el => { const a = lum(rgb(getComputedStyle(el).color)), b = lum(rgb(bg(el))); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) }
  const sel = 'main h1, main h2, main h3, main p, main span, main td, main th, main button, main a, main select, main label, main dt, main dd, main li, main legend'
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && el.children.length === 0 && !el.closest('[disabled]') && !el.disabled).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
// Downloads are blob links clicked by the page; record each one's name, size
// and first bytes instead of saving it. The page's Content-Security-Policy
// does not allow fetching blob: addresses, so the Blob is kept when its link
// is made.
const captureDownloads = `(() => { if (window.__dl) return true; window.__dl = []
  const blobs = new Map(), make = URL.createObjectURL
  URL.createObjectURL = function (o) { const u = make.call(URL, o); if (o instanceof Blob) blobs.set(u, o); return u }
  const orig = HTMLAnchorElement.prototype.click
  HTMLAnchorElement.prototype.click = function () {
    const blob = blobs.get(this.href)
    if (this.download && blob) { const name = this.download; blob.slice(0, 5).arrayBuffer().then(b => window.__dl.push({ name, size: blob.size, head: String.fromCharCode(...new Uint8Array(b)) })); return }
    return orig.call(this)
  }
  return true })()`
const clickButton = text => `(() => { const el = [...document.querySelectorAll('main button')].find(e => e.offsetParent !== null && !e.disabled && e.innerText.includes(${js(text)})); if (!el) return false; el.click(); return true })()`
const confirmYes = `(() => { const b = document.querySelectorAll('[role=alertdialog] button'); if (!b.length) return false; b[b.length - 1].click(); return true })()`
const mainText = b => b.evaluate(`document.querySelector('main')?.innerText || ''`)
const pageText = b => b.evaluate(`document.body.innerText`)

let signedIn = null
async function open(b, who, path, lang = 'en', theme = 'light') {
  if (who && signedIn !== who) { await b.login(email(who), password, lang); signedIn = who }
  await b.evaluate(`localStorage.setItem('flybirds.language', ${js(lang)}); localStorage.setItem('flybirds.theme', ${js(theme)})`)
  await b.go(path)
}
// Whoever scans a QR code is not signed in: sign out first.
async function signOut(b) {
  await b.go('/')
  await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/) || [])[1] || ''); if (csrf) await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }).catch(() => null); return true })()`)
  signedIn = null
}
async function download(b, text, timeout = 90000) {
  await b.evaluate(captureDownloads)
  const before = await b.evaluate('window.__dl.length')
  if (!await b.evaluate(clickButton(text))) return null
  await b.waitFor(`window.__dl.length > ${before}`, timeout)
  return b.evaluate(`window.__dl[window.__dl.length - 1] || null`)
}
const combos = [['1440-en-light', 1440, 900, false, 'en', 'light'], ['1440-bn-dark', 1440, 900, false, 'bn', 'dark'], ['375-en-dark', 375, 812, true, 'en', 'dark'], ['375-bn-light', 375, 812, true, 'bn', 'light']]
async function matrix(b, name, who, path, ready, scope = 'main') {
  const bad = []
  for (const [label, width, height, mobile, lang, theme] of combos) {
    await b.viewport(width, height, mobile)
    await open(b, who, path, lang, theme)
    if (!await b.waitFor(ready, 20000)) { bad.push(`${label}: not ready`); continue }
    await sleep(400)
    const state = await b.evaluate(`({ overflow: document.documentElement.scrollWidth > window.innerWidth + 1, theme: document.documentElement.dataset.theme || '', lang: document.documentElement.lang })`)
    if (state.overflow || state.lang !== lang) bad.push(`${label}: ${js(state)}`)
    const low = await b.evaluate(contrast)
    if (low.length) bad.push(`${label} contrast: ${low.join(' | ')}`)
    if (existsSync(axePath)) {
      await b.evaluate(readFileSync(axePath, 'utf8'))
      const axe = await b.evaluate(`axe.run(document.querySelector(${js(scope)}) ?? document.body, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(x => x.id + ':' + x.nodes.length))`)
      if (axe.length) bad.push(`${label} axe: ${axe.join(', ')}`)
    }
    await b.shot(`e7-${name}-${label}`, !mobile)
  }
  check(`${name}: 1440px and 375px, English and Bangla, light and dark: no overflow, contrast and axe clean`, bad.length === 0, bad.slice(0, 4).join(' || ') || `${combos.length} of ${combos.length}`)
}
const save = (name, pdf) => { writeFileSync(join(samples, name), pdf); return pdf.length }

const b = await launch({ base, shots })
await b.viewport(1440, 900)

// ---- 1. Before publication: a provisional preview for the head teacher only ---------
const tPreview = await teacher('POST', '/exams/documents', { kind: 'marksheets', ...ref, language: 'en', provisional: true })
check('a teacher cannot preview unpublished results', tPreview.status === 403, `${tPreview.status} ${tPreview.code}`)
const unpublished = await admin('POST', '/exams/documents', { kind: 'tabulation', ...ref, language: 'en' })
check('class documents need published results', unpublished.status === 409 && unpublished.code === 'results_not_published', `${unpublished.status} ${unpublished.code}`)
await open(b, 'head', reviewPath)
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Preview marksheets')`, 20000)
const headText = await mainText(b)
check('before publication the review offers only a provisional preview', headText.includes('Documents become available once these results are published') && !headText.includes('Tabulation sheet'), 'panel')
let t0 = Date.now()
const preview = await download(b, 'Preview marksheets')
metrics.provisionalMs = Date.now() - t0
check('the head teacher downloads the provisional preview (PDF, marked provisional)', preview?.head === '%PDF-' && /provisional/.test(preview.name), preview && `${preview.name} ${preview.size} bytes in ${metrics.provisionalMs} ms`)
const prov = await must(head('POST', '/exams/documents', { kind: 'marksheets', ...ref, language: 'bn', provisional: true }), 'provisional bn')
for (let i = 0; i < 60 && prov.status !== 'done'; i++) { await sleep(1000); Object.assign(prov, await must(head('GET', `/exams/documents/${prov.id}`), 'doc')) }
const provFile = await head('GET', `/exams/documents/${prov.id}/file`)
if (isPDF(provFile.pdf)) save('provisional-marksheets-bn.pdf', provFile.pdf)

// ---- 2. Publish the term ----------------------------------------------------------------
const review = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'review')
const ok = await must(head('POST', '/exams/results/approve', { ...ref, inputs_sha256: review.inputs_sha256 }), 'approve')
await must(admin('POST', '/exams/results/publish', { ...ref, publication_id: ok.approval.id }), 'publish')
const outcomes = review.students.reduce((m, s) => ({ ...m, [s.outputs.outcome]: (m[s.outputs.outcome] || 0) + 1 }), {})
check('the published term has passed, failed and withheld results', outcomes.passed > 0 && outcomes.failed > 0 && outcomes.withheld === 1, js(outcomes))

// ---- 3. The student's own marksheet ------------------------------------------------------
const published = await must(student('GET', '/exams/me/term-results'), 'my results')
const myResult = published.find(p => p.term_id === term.id)
await open(b, 'student', '/student/results')
await b.waitFor(`[...document.querySelectorAll('main button')].some(e => e.innerText.includes(${js(term.name)}))`, 20000)
await b.evaluate(`[...document.querySelectorAll('main button')].find(e => e.innerText.includes(${js(term.name)})).click()`)
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Download PDF (English)')`, 10000)
t0 = Date.now()
const myEN = await download(b, 'Download PDF (English)')
metrics.studentMarksheetMs = Date.now() - t0
const myBN = await download(b, 'Download PDF (বাংলা)')
check('the student downloads their own marksheet in English and Bangla', myEN?.head === '%PDF-' && myBN?.head === '%PDF-' && myEN.name.endsWith('-en.pdf') && myBN.name.endsWith('-bn.pdf'), `${myEN?.size} and ${myBN?.size} bytes; first in ${metrics.studentMarksheetMs} ms`)
for (const [lang, file] of [['en', 'marksheet-4th-subject-en.pdf'], ['bn', 'marksheet-4th-subject-bn.pdf']]) {
  const res = await student('GET', `/exams/me/documents/${myResult.id}?lang=${lang}`)
  if (isPDF(res.pdf)) save(file, res.pdf)
}
const otherVersion = (await must(admin('GET', `/exams/results/history?scope=term&scope_id=${term.id}&student_id=${failing.id}`), 'history'))[0]
const others = await student('GET', `/exams/me/documents/${otherVersion.id}?lang=en`)
const staffRoute = await student('POST', '/exams/documents', { kind: 'tabulation', ...ref, language: 'en' })
const staffList = await student('GET', '/exams/documents/classes')
check('a student cannot download another student\'s marksheet or any class document', others.status === 404 && staffRoute.status === 403 && staffList.status === 403, `${others.status} ${staffRoute.status} ${staffList.status}`)

// ---- 4. Class documents for the administrator --------------------------------------------
await open(b, 'admin', reviewPath)
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Tabulation sheet')`, 20000)
t0 = Date.now()
const tab = await download(b, 'Tabulation sheet')
metrics.tabulationMs = Date.now() - t0
check('the administrator downloads the tabulation sheet from the review', tab?.head === '%PDF-' && tab.name.startsWith('tabulation-'), tab && `${tab.size} bytes in ${metrics.tabulationMs} ms`)
t0 = Date.now()
await b.evaluate(captureDownloads)
await b.evaluate(clickButton('Marksheets'))
const sawProgress = await b.waitFor(`/of 60 marksheets ready/.test(document.querySelector('main')?.innerText || '')`, 30000)
await b.waitFor(`window.__dl.some(d => d.name.startsWith('marksheets-'))`, 180000)
metrics.bulk60Ms = Date.now() - t0
const bulk = await b.evaluate(`window.__dl.find(d => d.name.startsWith('marksheets-'))`)
check('all 60 marksheets are made in the background with progress, then download', sawProgress && bulk?.head === '%PDF-', bulk && `${bulk.size} bytes in ${metrics.bulk60Ms} ms`)
// The samples, through the API.
async function make(who, input, file) {
  t0 = Date.now()
  let doc = await must(who('POST', '/exams/documents', input), `document ${input.kind}`)
  while (doc.status === 'queued' || doc.status === 'running') { await sleep(800); doc = await must(who('GET', `/exams/documents/${doc.id}`), 'poll') }
  const ms = Date.now() - t0
  const res = await who('GET', `/exams/documents/${doc.id}/file`)
  if (file && isPDF(res.pdf)) save(file, res.pdf)
  return { doc, ms, pdf: res.pdf }
}
const bulkBN = await make(admin, { kind: 'marksheets', ...ref, language: 'bn' }, 'marksheets-class-60-bn.pdf')
metrics.bulk60BnMs = bulkBN.ms
const bulkEN = await make(admin, { kind: 'marksheets', ...ref, language: 'en' }, 'marksheets-class-60-en.pdf')
check('the class bundle has one page per student (60) in both languages', bulkBN.doc.pages === 60 && bulkEN.doc.pages === 60, `bn ${bulkBN.doc.pages} pages, ${bulkBN.pdf.length} bytes, ${bulkBN.ms} ms; en ${bulkEN.doc.pages} pages (cached: ${bulkEN.ms} ms)`)
const failedSheet = await make(admin, { kind: 'marksheet', ...ref, student_id: failing.id, language: 'en' }, 'marksheet-failed-en.pdf')
const withheldSheet = await make(admin, { kind: 'marksheet', ...ref, student_id: withheld.id, language: 'bn' }, 'marksheet-withheld-bn.pdf')
const docs = {}
for (const [kind, lang] of [['tabulation', 'en'], ['tabulation', 'bn'], ['merit_list', 'en'], ['merit_list', 'bn'], ['failed_list', 'en'], ['failed_list', 'bn'], ['summary', 'en'], ['summary', 'bn']]) {
  docs[`${kind}-${lang}`] = await make(admin, { kind, ...ref, language: lang }, `${kind.replace('_', '-')}-class-${lang}.pdf`)
}
const sectionTab = await make(admin, { kind: 'tabulation', ...ref, section_id: sections[0].id, language: 'en' }, 'tabulation-section-a-en.pdf')
check('every class document is a PDF (English and Bangla), and a section has its own', Object.values(docs).every(x => isPDF(x.pdf) && x.doc.status === 'done') && isPDF(failedSheet.pdf) && isPDF(withheldSheet.pdf) && isPDF(sectionTab.pdf),
  Object.entries(docs).map(([k, v]) => `${k} ${v.ms} ms`).join(', '))
const again = await make(admin, { kind: 'tabulation', ...ref, language: 'en' })
check('asking again returns the cached document', again.doc.id === docs['tabulation-en'].doc.id, `${again.ms} ms`)

// ---- 5. The class's teacher, and who may not -----------------------------------------------
const accountsTry = await accounts('POST', '/exams/documents', { kind: 'merit_list', ...ref, language: 'en' })
const accountsGet = await accounts('GET', `/exams/documents/${docs['tabulation-en'].doc.id}`)
check('the accountant gets no class documents', accountsTry.status === 403 && accountsGet.status === 403, `${accountsTry.status} ${accountsGet.status}`)
const tList = await must(teacher('GET', '/exams/documents/classes'), 'teacher classes')
check('the teacher sees the class they teach', tList.some(c => c.class_id === cls && c.scope_id === term.id && c.students === 60), `${tList.length} class result(s)`)
await open(b, 'teacher', '/teacher/exams')
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Result documents of my classes')`, 20000)
await b.evaluate(`[...document.querySelectorAll('main button')].find(e => e.innerText.includes(${js(term.name)}))?.click()`)
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Merit list')`, 10000)
const merit = await download(b, 'Merit list')
check('the teacher downloads the merit list from their marks page', merit?.head === '%PDF-' && merit.name.startsWith('merit-list-'), merit && `${merit.size} bytes`)

// ---- 6. The public check behind the QR code ------------------------------------------------
let token = ''
try { token = execFileSync('docker', ['exec', dbContainer, 'psql', '-U', 'flybirds', '-d', 'flybirds', '-tAc', `SELECT token FROM result_verifications WHERE version_id = '${myResult.id}'`], { encoding: 'utf8' }).trim() } catch { token = '' }
let oldFailingToken = ''
if (token) {
  const v = await fetch(`${base}/api/public/results/${token}`).then(x => x.json())
  check('the QR check answers without a sign-in, current, with no personal data', v.data?.state === 'current' && !js(v).includes(me.name_en) && !js(v).includes(me.admission_no) && !/gpa|marks/i.test(js(v)), js(v.data).slice(0, 160))
  await signOut(b)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
  await b.go(`/verify/result/${token}`)
  await b.waitFor(`document.body.innerText.includes('genuine and current')`, 15000)
  const verifyText = await pageText(b)
  check('the result check page says genuine and current, with the document code', verifyText.includes(v.data.code) && !verifyText.includes(me.name_en), v.data.code)
  await b.go('/verify/result/AAAAAAAAAAAAAAAAAAAAAA')
  check('an unknown code is not a result issued by the school', await b.waitFor(`document.body.innerText.includes('not a result issued by the school')`, 15000))
  try { oldFailingToken = execFileSync('docker', ['exec', dbContainer, 'psql', '-U', 'flybirds', '-d', 'flybirds', '-tAc', `SELECT token FROM result_verifications WHERE version_id = '${otherVersion.id}'`], { encoding: 'utf8' }).trim() } catch { oldFailingToken = '' }
} else check('the QR check (database container not reachable; skipped)', true, 'skipped')

// ---- 7. A correction makes new documents; the old QR says superseded ------------------------
const exExam = exams.find(e => e.assessment_key === EX && e.section_id === failing.section_id)
const corr = await must(admin('POST', `/exams/${exExam.id}/corrections`, { student_id: failing.id, reason: 'CQ script re-marked' }), 'correction')
const cell = corr.cells.find(c => c.subject_id === (mathId ?? c.subject_id)) ?? corr.cells[0]
await must(admin('PATCH', `/exams/${exExam.id}/marks`, { entries: [{ exam_subject_id: cell.exam_subject_id, student_id: failing.id, status: 'present', components: { CQ: 40, MCQ: 20 }, row_version: cell.row_version }] }), 'corrected mark')
const aRef = { scope: 'assessment', scope_id: exExam.id }
const arv = await must(head('GET', `/exams/results/review?scope=assessment&scope_id=${exExam.id}`), 'assessment review')
const aok = await must(head('POST', '/exams/results/approve', { ...aRef, inputs_sha256: arv.inputs_sha256 }), 'approve correction')
await must(admin('POST', '/exams/results/publish', { ...aRef, publication_id: aok.approval.id }), 'publish correction')
const trv = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'term revision')
const tok2 = await must(head('POST', '/exams/results/approve', { ...ref, inputs_sha256: trv.inputs_sha256, reason: 'CQ script re-marked' }), 'approve revision')
await must(admin('POST', '/exams/results/publish', { ...ref, publication_id: tok2.approval.id }), 'publish revision')
const after = await make(admin, { kind: 'tabulation', ...ref, language: 'en' }, 'tabulation-class-en-after-correction.pdf')
const gone = await admin('GET', `/exams/documents/${docs['tabulation-en'].doc.id}`)
check('after a correction the tabulation sheet is a new document and the old one is gone', after.doc.id !== docs['tabulation-en'].doc.id && gone.status === 404, `${after.doc.id.slice(0, 8)} replaces ${docs['tabulation-en'].doc.id.slice(0, 8)}`)
if (oldFailingToken) {
  const v = await fetch(`${base}/api/public/results/${oldFailingToken}`).then(x => x.json())
  await b.go(`/verify/result/${oldFailingToken}`)
  const superseded = await b.waitFor(`document.body.innerText.includes('corrected later')`, 15000)
  check('the corrected student\'s old QR now says the result was corrected later', v.data?.state === 'superseded' && superseded, v.data?.state)
}
const oldVersion = await student('GET', `/exams/me/documents/${otherVersion.id}?lang=en`)
check('a superseded version cannot be downloaded', oldVersion.status === 404, String(oldVersion.status))

// ---- 8. Blueprint presets and copying another class ------------------------------------------
const bps = await must(admin('GET', `/exams/blueprints?session_id=${session}`), 'blueprints')
const six = bps.find(c => c.class_id === cls)
const other = bps.find(c => c.class_id !== cls && !c.active && !c.draft)
let otherActive = null
if (other) {
  const od = await must(admin('POST', '/exams/blueprints', { session_id: session, class_id: other.class_id }), 'other class blueprint')
  const odSaved = await must(admin('PUT', `/exams/blueprint-versions/${od.id}`, { definition: od.definition }), 'other save')
  if (odSaved.issues.length === 0) { await must(admin('POST', `/exams/blueprint-versions/${od.id}/activate`, {}), 'other activate'); otherActive = other }
}
const nd = await must(admin('POST', `/exams/blueprints/${six.blueprint_id}/versions`, { from_version_id: six.active.id }), 'new draft')
await open(b, 'admin', `/admin/exams/blueprints/${nd.id}?session=${session}`)
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Quick setup')`, 20000)
await b.evaluate(`[...document.querySelectorAll('main button')].find(e => e.innerText.includes('CQ 50 + MCQ 25 + Practical 25') && e.innerText.includes('20% class test'))?.click()`)
await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
await b.evaluate(confirmYes)
await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Every subject now uses')`, 10000)
await b.waitFor(`document.querySelector('[data-save-state]')?.dataset.saveState === 'saved'`, 15000)
const presetDef = (await must(admin('GET', `/exams/blueprint-versions/${nd.id}`), 'draft')).definition
const graded = presetDef.subjects.filter(s => s.role !== 'non_graded')
const finalKey = presetDef.assessments.filter(a => a.term_id === term.id && a.kind === 'term_exam')[0]?.key
const allPrac = graded.every(s => s.sheets.find(x => x.assessment === finalKey)?.components.map(c => c.key).join(',') === 'CQ,MCQ,PRAC' && s.terms.find(t => t.term_id === term.id)?.contributions.map(c => c.worth).join(',') === '20,80')
check('one click sets every subject to CQ 50 + MCQ 25 + Practical 25 with class tests at 20 %', allPrac, `${graded.length} subjects`)
if (otherActive) {
  await open(b, 'admin', `/admin/exams/blueprints/${nd.id}?session=${session}`)
  await b.waitFor(`!!document.querySelector('main select option[value=${js(otherActive.class_id)}]')`, 15000)
  await b.evaluate(`(() => { const el = [...document.querySelectorAll('main select')].find(s => s.querySelector('option[value=${js(otherActive.class_id)}]')); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, ${js(otherActive.class_id)}); el.dispatchEvent(new Event('change', { bubbles: true })); return true })()`)
  await b.evaluate(clickButton('Copy blueprint'))
  await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
  await b.evaluate(confirmYes)
  check('a blueprint is copied from another class of the session', await b.waitFor(`(document.querySelector('main')?.innerText || '').includes('Copied from ${otherActive.class_name}')`, 15000), otherActive.class_name)
} else check('copying from another class (no other class without a blueprint; skipped)', true, 'skipped')
await sleep(1200)
await must(admin('DELETE', `/exams/blueprint-versions/${nd.id}`), 'discard the trial draft')

// ---- 9. The new screens: sizes, languages, themes ------------------------------------------
await matrix(b, 'review-documents', 'admin', reviewPath, `/Result summary|ফলাফলের সারসংক্ষেপ/.test(document.querySelector('main')?.innerText || '')`)
const nd2 = await must(admin('POST', `/exams/blueprints/${six.blueprint_id}/versions`, { from_version_id: six.active.id }), 'draft for screens')
await matrix(b, 'blueprint-quick-setup', 'admin', `/admin/exams/blueprints/${nd2.id}?session=${session}`, `/Quick setup|দ্রুত সেটআপ/.test(document.querySelector('main')?.innerText || '')`)
await must(admin('DELETE', `/exams/blueprint-versions/${nd2.id}`), 'discard')
await matrix(b, 'teacher-documents', 'teacher', '/teacher/exams', `/Result documents of my classes|আমার শ্রেণির ফলাফলের ডকুমেন্ট/.test(document.querySelector('main')?.innerText || '')`)
await matrix(b, 'student-results', 'student', '/student/results', `[...document.querySelectorAll('main button')].some(e => e.innerText.includes(${js(term.name)}))`)
if (token) { await signOut(b); await matrix(b, 'result-check', null, `/verify/result/${token}`, `/genuine and current|আসল এবং বর্তমান/.test(document.body.innerText)`, 'body') }

const errors = [...(b.errors || []), ...(b.consoleErrors || [])].filter(e => !String(e).includes('Failed to load resource'))
check('no uncaught exceptions or console errors in the browser', errors.length === 0, js(errors.slice(0, 3)))
await b.close()
writeFileSync(join(samples, 'result-documents.json'), js({ metrics, students: roster.length, outcomes, samples: true }, null, 2))
console.log('metrics', js(metrics))
r.save(join(samples, 'result-documents-checks.json'))
