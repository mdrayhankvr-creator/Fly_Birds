// Marksheet v2 and admit cards on a running, demo-seeded disposable school:
// class teachers assigned on the Sections page, a teacher's signature, class
// teacher comments (written by the class teacher, reviewed by the head
// teacher), the marksheet PDF of a published term (photo, parents, combined
// papers, the fourth subject, attendance, comment, signatures), the old
// report card page opening the PDF, admit cards from the term's published
// routine (class teacher, administrator, student), the fee hold with the
// office's release, layouts, languages, themes and accessibility.
//
//   E2E_BASE_URL=http://localhost:8121 DEMO_PASSWORD=... E2E_PDF_DIR=... node e2e/marksheet-admit.mjs
//
// It adds subjects, a term, results, a routine and rooms, and changes
// settings (restored at the end); never run it against a real school.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { deflateSync } from 'node:zlib'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const shots = process.env.E2E_SHOTS ?? join(here, 'shots')
const pdfDir = process.env.E2E_PDF_DIR ?? join(shots, 'pdf-marksheet-admit')
mkdirSync(shots, { recursive: true })
mkdirSync(pdfDir, { recursive: true })
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const js = v => JSON.stringify(v)
const email = who => `demo.${who}@demo.flybirds.test`
const stamp = Date.now().toString(36).slice(-4).toUpperCase()
const pages = bytes => { const m = [...bytes.toString('latin1').matchAll(/\/Type\s*\/Pages\b[^>]*?\/Count\s+(\d+)/g)]; return m.length ? Number(m[0][1]) : 0 }
const isPDF = b => !!b && b.subarray(0, 5).toString() === '%PDF-'

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: email(who), password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  const call = async (method, path, body, type) => {
    const raw = type !== undefined
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': raw ? type : 'application/json' } : {}) }, body: body ? (raw ? body : js(body)) : undefined })
    const kind = response.headers.get('content-type') || ''
    if (kind.includes('application/pdf')) return { status: response.status, pdf: Buffer.from(await response.arrayBuffer()), held: response.headers.get('x-admit-cards-held') }
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
  return call
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }
const [admin, head, teacher, student, accounts] = await Promise.all(['admin', 'head', 'teacher', 'student', 'accounts'].map(api))

// A small PNG encoder for the signature and the photo.
const crcTable = [...Array(256).keys()].map(n => { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; return c >>> 0 })
const crc = buf => { let c = 0xffffffff; for (const b of buf) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0 }
function png(w, h, pixel) {
  const rows = Buffer.alloc((w * 4 + 1) * h)
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) { const [r_, g, b, a] = pixel(x, y); rows.set([r_, g, b, a], y * (w * 4 + 1) + 1 + x * 4) }
  const chunk = (type, data) => { const len = Buffer.alloc(4); len.writeUInt32BE(data.length); const td = Buffer.concat([Buffer.from(type), data]); const c = Buffer.alloc(4); c.writeUInt32BE(crc(td)); return Buffer.concat([len, td, c]) }
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr.set([8, 6, 0, 0, 0], 8)
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(rows)), chunk('IEND', Buffer.alloc(0))])
}
// A handwritten-looking stroke on transparent paper.
const signature = png(420, 140, (x, y) => {
  const t = x / 420, curve = 70 + Math.sin(t * 9) * 28 * (1 - t * 0.4) + Math.cos(t * 23) * 6
  return Math.abs(y - curve) < 2.6 && x > 18 && x < 400 ? [20, 30, 110, 255] : [255, 255, 255, 0]
})
// A portrait-like photo: a face shape on a blue background.
const photo = png(240, 288, (x, y) => {
  const head = ((x - 120) / 62) ** 2 + ((y - 115) / 78) ** 2 < 1, body = y > 200 && Math.abs(x - 120) < 95 - (288 - y) * 0.3
  return head ? [224, 182, 150, 255] : body ? [40, 60, 90, 255] : [196, 214, 236, 255]
})

// ---- 0. The class, its term and results, through the API ---------------------------
const me = await must(student('GET', '/students/me'), 'me')
const session = (await must(admin('GET', '/academic/sessions?page=1&page_size=100'), 'sessions')).items.find(s => s.id === me.session_id)
const cls = me.class_id
const teacherEmployee = await must(teacher('GET', '/staff/me'), 'teacher')
const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date())
const addDays = (iso, n) => { const d = new Date(`${iso}T00:00:00Z`); d.setUTCDate(d.getUTCDate() + n); return d.toISOString().slice(0, 10) }
let from = addDays(today, 3), to = addDays(today, 33)
if (to > session.end_date) { to = session.end_date; from = addDays(to, -30) < session.start_date ? session.start_date : addDays(to, -30) }

// The demo student's parents, birth date and photo.
const full = await must(admin('GET', `/students/${me.id}`), 'student')
await must(admin('PATCH', `/students/${me.id}`, { ...full, date_of_birth: full.date_of_birth || '2012-03-05', father_name_en: 'Abdul Karim', father_name_bn: 'আবদুল করিম', mother_name_en: 'Rahima Begum', mother_name_bn: 'রহিমা বেগম' }), 'parents')
const photoSaved = await admin('PUT', `/students/${me.id}/photo`, photo, 'image/png')
check('the demo student has a photo and their parents’ names', photoSaved.status === 200, `${photoSaved.status} ${photoSaved.code ?? ''}`)

// Bangla as two papers graded together (combined), added to the class.
const paperIds = []
for (const [code, en, bn] of [['B1', 'Bangla 1st Paper', 'বাংলা প্রথম পত্র'], ['B2', 'Bangla 2nd Paper', 'বাংলা দ্বিতীয় পত্র']]) {
  const s = await must(admin('POST', '/academic/subjects', { name_en: en, name_bn: bn, code: `${code}${stamp}`, is_active: true }), `subject ${code}`)
  paperIds.push(s.id)
  await must(admin('POST', `/academic/classes/${cls}/subjects`, { subject_id: s.id, is_required: true, is_fourth_subject: false }), `map ${code}`)
}
const term = await must(admin('POST', '/exams/terms', { session_id: me.session_id, name: `Half-Yearly ${stamp}`, kind: 'term', starts_on: from, ends_on: to }), 'term')
const classes = await must(admin('GET', `/exams/blueprints?session_id=${me.session_id}`), 'blueprints')
const mine = classes.find(c => c.class_id === cls)
if (mine.draft) await must(admin('DELETE', `/exams/blueprint-versions/${mine.draft.id}`), 'discard a leftover draft')
const draft = mine.active ? await must(admin('POST', `/exams/blueprints/${mine.blueprint_id}/versions`, { from_version_id: mine.active.id }), 'new version')
  : await must(admin('POST', '/exams/blueprints', { session_id: me.session_id, class_id: cls }), 'new blueprint')
const d = draft.definition
const EX = `H${stamp}`
d.assessments = d.assessments.filter(a => a.term_id !== term.id).concat([{ key: EX, name: 'Half-yearly exam', term_id: term.id, kind: 'term_exam', visibility: 'after_term_publication' }])
let cqSubject = ''
for (const s of d.subjects) {
  if (s.role === 'non_graded') continue
  const paper = paperIds.includes(s.subject_id)
  if (paper) s.combined_key = `BAN${stamp}`
  if (!cqSubject && s.role === 'required' && !paper) cqSubject = s.subject_id
  const components = paper ? [{ key: 'CQ', full: 70, pass: 23 }, { key: 'MCQ', full: 30, pass: 10 }] : s.subject_id === cqSubject ? [{ key: 'CQ', full: 70, pass: 23 }, { key: 'MCQ', full: 30, pass: 10 }] : [{ key: 'CQ', full: 50, pass: 17 }, { key: 'MCQ', full: 25, pass: 8 }, { key: 'PRAC', full: 25, pass: 8 }]
  s.sheets = s.sheets.filter(x => x.assessment !== EX).concat([{ assessment: EX, pass: 33, components }])
  s.terms = s.terms.filter(x => x.term_id !== term.id).concat([{ term_id: term.id, full: 100, pass: 33, contributions: [{ from: [EX], combine: 'single', worth: 100 }] }])
}
const saved = await must(admin('PUT', `/exams/blueprint-versions/${draft.id}`, { definition: d }), 'save draft')
check('the blueprint with combined Bangla papers validates', saved.issues.length === 0, saved.issues.map(i => i.message).slice(0, 3).join(' | '))
await must(admin('POST', `/exams/blueprint-versions/${draft.id}/activate`, {}), 'activate')
await must(admin('PUT', `/exams/terms/${term.id}/setup`, { class_ids: [cls], due: { [EX]: to }, assignees: {} }), 'setup')
await must(admin('POST', `/exams/terms/${term.id}/create`, {}), 'create')
const listed = (await must(admin('GET', `/exams?page=1&page_size=100&session_id=${me.session_id}&class_id=${cls}`), 'exams')).items.filter(e => e.term_id === term.id)
const exams = await Promise.all(listed.map(e => must(admin('GET', `/exams/${e.id}`), 'exam')))
const roster = (await must(admin('GET', `/students?page=1&page_size=100&session_id=${me.session_id}&class_id=${cls}&status=active`), 'class')).items
const ordered = [...roster].sort((a, b) => a.section_name.localeCompare(b.section_name) || (a.roll_no ?? 0) - (b.roll_no ?? 0))
const sameSection = ordered.filter(s => s.section_id === me.section_id && s.id !== me.id)
const failing = sameSection[1] ?? sameSection[0]
for (const exam of exams) {
  const rows = await must(admin('GET', `/exams/${exam.id}/roster`), 'roster')
  const entries = rows.map(row => {
    const i = ordered.findIndex(s => s.id === row.student_id)
    const sheet = exam.subjects.find(s => s.id === row.exam_subject_id)
    const strong = row.student_id === me.id ? 0.9 : 0.45 + ((i * 7 + sheet.subject_id.charCodeAt(0)) % 50) / 100
    const comps = sheet.subject_id === cqSubject || paperIds.includes(sheet.subject_id) ? ['CQ', 70, 'MCQ', 30] : ['CQ', 50, 'MCQ', 25, 'PRAC', 25]
    const components = {}
    for (let k = 0; k < comps.length; k += 2) components[comps[k]] = Math.round(comps[k + 1] * strong)
    if (row.student_id === failing.id && sheet.subject_id === cqSubject) components.CQ = 11
    return { exam_subject_id: row.exam_subject_id, student_id: row.student_id, status: 'present', components, row_version: row.row_version }
  })
  for (let k = 0; k < entries.length; k += 400) {
    const res = await must(admin('PATCH', `/exams/${exam.id}/marks`, { entries: entries.slice(k, k + 400) }), 'marks')
    if (res.errors.length) throw new Error(`marks for ${exam.name}: ${js(res.errors.slice(0, 2))}`)
  }
}
for (const exam of exams) await must(admin('PATCH', `/exams/${exam.id}/state`, { status: 'published' }), `publish ${exam.name}`)

// ---- Browser helpers ---------------------------------------------------------------
const axePath = join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
const contrast = `(() => {
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  const rgb = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = '#000'; ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const opaque = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return ctx.getImageData(0, 0, 1, 1).data[3] > 250 }
  const bg = el => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (c && opaque(c)) return c } return getComputedStyle(document.body).backgroundColor }
  const ratio = el => { const a = lum(rgb(getComputedStyle(el).color)), b = lum(rgb(bg(el))); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) }
  const sel = 'main h1, main h2, main h3, main p, main span, main td, main th, main button, main a, main select, main label, main dt, main dd, main li, main legend, main summary'
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && el.children.length === 0 && !el.closest('[disabled]') && !el.disabled && !el.closest('[aria-hidden=true]')).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
const setValue = `(el, value) => { const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value); el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })) }`
const set = (selector, value) => `(() => { const el = document.querySelector(${js(selector)}); if (!el) return false; (${setValue})(el, ${js(value)}); return true })()`
const clickText = (text, scope = 'main') => `(() => { const el = [...document.querySelectorAll(${js(scope + ' button')})].find(e => e.offsetParent !== null && !e.disabled && e.innerText.trim() === ${js(text)}); if (!el) return false; el.click(); return true })()`
const mainText = `document.querySelector('main')?.innerText ?? ''`
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
async function download(b, click, timeout = 60000) {
  await b.evaluate(captureDownloads)
  const before = await b.evaluate('window.__dl.length')
  if (!await b.evaluate(click)) return null
  await b.waitFor(`window.__dl.length > ${before}`, timeout)
  return b.evaluate('window.__dl[window.__dl.length - 1] || null')
}

let signedIn = null
async function open(b, who, path, lang = 'en', theme = 'light') {
  if (signedIn !== who) { await b.login(email(who), password, lang); signedIn = who }
  await b.evaluate(`localStorage.setItem('flybirds.language', ${js(lang)}); localStorage.setItem('flybirds.theme', ${js(theme)})`)
  await b.go(path)
}
const combos = [
  ['1440-en-light', 1440, 900, false, 'en', 'light'], ['1440-bn-dark', 1440, 900, false, 'bn', 'dark'],
  ['375-en-dark', 375, 812, true, 'en', 'dark'], ['375-bn-light', 375, 812, true, 'bn', 'light'],
]
async function matrix(b, name, who, path, ready) {
  const bad = []
  for (const [label, width, height, mobile, lang, theme] of combos) {
    await b.viewport(width, height, mobile)
    await open(b, who, path, lang, theme)
    if (!await b.waitFor(ready, 20000)) { bad.push(`${label}: not ready`); continue }
    await sleep(500)
    const state = await b.evaluate(`({ overflow: document.documentElement.scrollWidth > window.innerWidth + 1, theme: document.documentElement.dataset.theme, lang: document.documentElement.lang })`)
    if (state.overflow || state.theme !== theme || state.lang !== lang) bad.push(`${label}: ${js(state)}`)
    const low = await b.evaluate(contrast)
    if (low.length) bad.push(`${label} contrast: ${low.join(' | ')}`)
    if (existsSync(axePath)) {
      await b.evaluate(readFileSync(axePath, 'utf8'))
      const axe = await b.evaluate(`axe.run(document.querySelector('main'), { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(x => x.id + ':' + x.nodes.length))`)
      if (axe.length) bad.push(`${label} axe: ${axe.join(', ')}`)
    }
    await b.shot(`ms-${name}-${label}`, !mobile)
  }
  check(`${name}: 1440px and 375px, English and Bangla, light and dark: no overflow, contrast and axe clean`, bad.length === 0, bad.slice(0, 4).join(' || ') || `${combos.length} of ${combos.length}`)
}

const b = await launch({ base, shots })
await b.evaluate(`window.addEventListener('error', e => (window.__errors = [...(window.__errors || []), String(e.message)]))`)
await b.viewport(1440, 900)

// ---- 1. The administrator assigns the class teacher on the Sections page -------------
await open(b, 'admin', '/admin/academic/sections')
await b.waitFor(`!!document.querySelector('[data-testid=class-teachers] select[aria-label^="Class teacher of"]')`, 20000)
const label = `Class teacher of ${me.class_name} ${me.section_name}`
await b.evaluate(set(`[data-testid=class-teachers] select[aria-label=${js(label)}]`, teacherEmployee.id))
await b.waitFor(`(document.querySelector('[data-testid=class-teachers]')?.innerText ?? '').includes('Class teacher saved.')`, 15000)
const assigned = (await must(admin('GET', `/academic/class-teachers?session_id=${me.session_id}`), 'class teachers')).find(s => s.section_id === me.section_id)
check('the administrator assigns the class teacher of the demo student’s section on the Sections page', assigned?.employee_id === teacherEmployee.id, assigned?.teacher_name)
const refused = await head('PUT', `/academic/sections/${me.section_id}/class-teacher`, { employee_id: teacherEmployee.id })
check('the head teacher cannot assign class teachers', refused.status === 403, `${refused.status}`)

// ---- 2. The teacher's signature ----------------------------------------------------
const upload = await teacher('PUT', '/staff/me/signature', signature, 'image/png')
const bad = await teacher('PUT', '/staff/me/signature', Buffer.from('not an image'), 'image/png')
check('a teacher uploads their signature (PNG); a broken file is refused', upload.status === 200 && /^[0-9a-f]{12}$/.test(upload.data?.version ?? '') && bad.status === 400, `${upload.status} ${bad.status}`)
await open(b, 'teacher', '/staff/me')
check('the profile shows the signature as printed', await b.waitFor(`!!document.querySelector('[data-testid=signature-card] img[alt]')?.naturalWidth`, 20000), '')

// ---- 3. Comments: the class teacher writes, the head teacher reviews ----------------
const long = 'Steady and careful all term. Helps classmates in group work, keeps neat notebooks and asks good questions; should practise written answers in Science every evening and read one story book a week to grow her vocabulary before the annual examination.'
await open(b, 'teacher', '/exams/comments')
const scopeValue = `term:${term.id}`
await b.waitFor(`[...document.querySelectorAll('main select')].some(s => [...s.options].some(o => o.value === ${js(scopeValue)}))`, 20000)
await b.evaluate(`(() => { const s = [...document.querySelectorAll('main select')].find(x => [...x.options].some(o => o.value === ${js(scopeValue)})); (${setValue})(s, ${js(scopeValue)}); return true })()`)
await b.waitFor(`!!document.querySelector('[data-testid=comment-rows]')`, 20000)
const nameOf = s => s.name_en
const pick = async (who, value) => b.evaluate(`(() => { const s = document.querySelector('select[aria-label=${js(`Comment for ${nameOf(who)}`)}]'); if (!s) return false; const v = ${js(value)} === 'first' ? s.options[1].value : ${js(value)}; (${setValue})(s, v); return true })()`)
const saveRow = who => b.evaluate(`(() => { const s = document.querySelector('select[aria-label=${js(`Comment for ${nameOf(who)}`)}]'); const btn = s?.closest('li')?.querySelector('button.btn-primary'); if (!btn || btn.disabled) return false; btn.click(); return true })()`)
const meRow = roster.find(s => s.id === me.id)
const picked = await pick(meRow, 'first') && await saveRow(meRow)
await b.waitFor(`(document.querySelector('main')?.innerText ?? '').includes('Saved.')`, 15000)
await pick(failing, '__own'); await sleep(200)
await b.evaluate(set(`textarea[aria-label=${js(`Own words for ${failing.name_en}`)}]`, long)); await sleep(200)
const ownSaved = await saveRow(failing)
await sleep(800)
let sheet = await must(teacher('GET', `/exams/comments?scope=term&scope_id=${term.id}&section_id=${me.section_id}`), 'sheet')
const c0 = sheet.students.find(s => s.student_id === me.id), c1 = sheet.students.find(s => s.student_id === failing.id)
check('the class teacher picks a preset for one student and writes their own words for another', picked && ownSaved && c0?.preset_id && c0.status === 'written' && c1?.custom_text === long, `${c0?.status} / ${c1?.status}`)
const outsider = await accounts('GET', `/exams/comments?scope=term&scope_id=${term.id}&section_id=${me.section_id}`)
check('the accountant cannot read comments', outsider.status === 403, `${outsider.status}`)
await open(b, 'head', '/exams/comments')
await b.waitFor(`[...document.querySelectorAll('main select')].some(s => [...s.options].some(o => o.value === ${js(me.section_id)}))`, 20000)
await b.evaluate(`(() => { const s = [...document.querySelectorAll('main select')].find(x => [...x.options].some(o => o.value === ${js(me.section_id)})); (${setValue})(s, ${js(me.section_id)}); return true })()`)
await sleep(400)
await b.evaluate(`(() => { const s = [...document.querySelectorAll('main select')].find(x => [...x.options].some(o => o.value === ${js(scopeValue)})); (${setValue})(s, ${js(scopeValue)}); return true })()`)
await b.waitFor(clickText('Mark written comments reviewed').replace('el.click(); return true', 'return true'), 20000)
await b.evaluate(clickText('Mark written comments reviewed'))
await b.waitFor(`(document.querySelector('main')?.innerText ?? '').includes('comments marked reviewed')`, 15000)
sheet = await must(head('GET', `/exams/comments?scope=term&scope_id=${term.id}&section_id=${me.section_id}`), 'sheet')
check('the head teacher marks the section’s comments reviewed', sheet.students.filter(s => s.status === 'reviewed').length >= 2, `${sheet.students.filter(s => s.status === 'reviewed').length} reviewed`)
check('the presets are listed for the head teacher to edit', (await b.evaluate(mainText)).includes('Preset comments'), '')

// ---- 4. Publish, then the marksheets ---------------------------------------------
const ref = { scope: 'term', scope_id: term.id, class_id: cls }
const review = await must(head('GET', `/exams/results/review?scope=term&scope_id=${term.id}&class_id=${cls}`), 'review')
const ok = await must(head('POST', '/exams/results/approve', { ...ref, inputs_sha256: review.inputs_sha256 }), 'approve')
await must(admin('POST', '/exams/results/publish', { ...ref, publication_id: ok.approval.id }), 'publish')
const published = await must(student('GET', '/exams/me/term-results'), 'my results')
const myVersion = published.find(x => x.scope === 'term' && x.term_id === term.id) ?? published.find(x => x.term_name === term.name)
const sample = {}
for (const lang of ['en', 'bn']) {
  const res = await student('GET', `/exams/me/documents/${myVersion?.id}?lang=${lang}`)
  sample[lang] = res
  if (isPDF(res.pdf)) writeFileSync(join(pdfDir, `marksheet-photo-4th-combined-${lang}.pdf`), res.pdf)
}
check('the student downloads their marksheet in English and Bangla, one A4 page each', isPDF(sample.en.pdf) && isPDF(sample.bn.pdf) && pages(sample.en.pdf) === 1 && pages(sample.bn.pdf) === 1,
  `${sample.en.status}/${sample.bn.status}, ${Math.round((sample.en.pdf?.length ?? 0) / 1024)} KB`)
async function staffMarksheet(who, studentId, lang, name) {
  let doc = await must(who('POST', '/exams/documents', { kind: 'marksheet', ...ref, student_id: studentId, language: lang }), 'marksheet')
  for (let i = 0; i < 60 && doc.status !== 'done' && doc.status !== 'failed'; i++) { await sleep(800); doc = await must(who('GET', `/exams/documents/${doc.id}`), 'doc') }
  const file = await who('GET', `/exams/documents/${doc.id}/file`)
  if (isPDF(file.pdf)) writeFileSync(join(pdfDir, name), file.pdf)
  return { doc, file }
}
const failed = await staffMarksheet(admin, failing.id, 'en', 'marksheet-failed-long-comment-no-photo-en.pdf')
await staffMarksheet(admin, failing.id, 'bn', 'marksheet-failed-long-comment-no-photo-bn.pdf')
check('a failed student’s marksheet (long comment, no photo) is one page', failed.doc.status === 'done' && pages(failed.file.pdf) === 1, `${failed.doc.status} ${failed.doc.pages} page`)
const before = failed.doc.id
await must(teacher('PUT', '/exams/comments', { scope: 'term', scope_id: term.id, student_id: failing.id, preset_id: null, custom_text: 'Must work harder in Mathematics.' }), 'change comment')
const again = await staffMarksheet(admin, failing.id, 'en', 'marksheet-failed-changed-comment-en.pdf')
const versionsAfter = await must(admin('GET', `/exams/results/history?scope=term&scope_id=${term.id}&student_id=${failing.id}`), 'history')
check('a changed comment makes a new document and leaves the published result as it was', again.doc.id !== before && versionsAfter.length === 1, `${versionsAfter.length} version(s)`)

// The old report card page now offers the PDF instead of printing the page.
await open(b, 'student', '/student/results')
const cardBtn = await b.waitFor(`[...document.querySelectorAll('main button[aria-pressed]')].some(x => x.innerText.includes('Half-yearly exam'))`, 20000)
await b.evaluate(`(() => { const el = [...document.querySelectorAll('main button[aria-pressed]')].find(x => x.innerText.includes('Half-yearly exam')); el?.click(); return !!el })()`)
const offered = await b.waitFor(`(document.querySelector('main')?.innerText ?? '').includes('Marksheet (PDF)')`, 20000)
const text = await b.evaluate(mainText)
check('the old report card offers the marksheet PDF and no longer prints the page', cardBtn && offered && !text.includes('Print report card'), '')
const got = await download(b, `(() => { const el = [...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Download PDF (English)'); if (!el) return false; el.click(); return true })()`)
check('the report card’s download is the marksheet PDF', got?.head === '%PDF-', got && `${got.name} ${got.size} bytes`)

// ---- 5. The term's routine, published ---------------------------------------------
const sections = (await must(admin('GET', `/academic/sections?session_id=${me.session_id}&page=1&page_size=100`), 'sections')).items.filter(s => s.is_active)
const rooms = []
for (let i = 1; i <= sections.length + 1; i++) rooms.push(await must(admin('POST', '/routines/rooms', { name: `MS Room ${200 + i} ${stamp}`, name_bn: `কক্ষ ${200 + i}`, kind: 'classroom', capacity: 80, is_active: true }), 'room'))
rooms.push(await must(admin('POST', '/routines/rooms', { name: `MS Hall ${stamp}`, name_bn: 'হলরুম', kind: 'hall', capacity: 400, is_active: true }), 'hall'))
for (const room of await must(admin('GET', '/routines/rooms'), 'rooms')) {
  if (!room.name.endsWith(stamp)) await must(admin('PATCH', `/routines/rooms/${room.id}`, { ...room, is_active: false }), 'old room')
}
let v = await must(admin('POST', '/routines/versions', { term_id: term.id }), 'routine')
v = (await must(admin('POST', `/routines/versions/${v.id}/generate`), 'generate')).version
check('the term’s routine is drafted with every paper placed', v.papers.every(p => p.exam_date) && v.issues.filter(i => i.hard).length === 0, `${v.papers.length} papers`)
await must(admin('POST', `/routines/versions/${v.id}/submit`, {}), 'submit')
await must(head('POST', `/routines/versions/${v.id}/approve`, {}), 'approve')
await must(admin('POST', `/routines/versions/${v.id}/publish`, {}), 'publish')

// ---- 6. Admit cards ---------------------------------------------------------------
const tTerms = await must(teacher('GET', '/routines/admit-cards/terms'), 'teacher terms')
const tTerm = tTerms.find(x => x.term_id === term.id)
check('the class teacher sees only their own section', tTerm?.sections.length === 1 && tTerm.sections[0].section_id === me.section_id, tTerm?.sections.map(s => s.section_name).join(','))
const other = sections.find(s => s.class_id === cls && s.id !== me.section_id)
if (other) {
  const res = await teacher('GET', `/routines/admit-cards/terms/${term.id}/pdf?section_id=${other.id}`)
  check('the class teacher cannot print another section', res.status === 403, `${res.status}`)
}
await open(b, 'teacher', '/admit-cards')
await b.waitFor(`!!document.querySelector('[data-testid=admit-print] button[aria-label^="Print"]')`, 20000)
const tCards = await download(b, `(() => { const el = document.querySelector('[data-testid=admit-print] button[aria-label$="in English"]'); if (!el) return false; el.click(); return true })()`)
check('the class teacher prints their section’s admit cards in the browser', tCards?.head === '%PDF-', tCards && `${tCards.name} ${tCards.size} bytes`)
const classSize = roster.length
for (const lang of ['en', 'bn']) {
  const res = await admin('GET', `/routines/admit-cards/terms/${term.id}/pdf?class_id=${cls}&lang=${lang}`)
  if (isPDF(res.pdf)) writeFileSync(join(pdfDir, `admit-cards-class-${lang}.pdf`), res.pdf)
  check(`the administrator prints the whole class’s admit cards (${lang}): two per page`, isPDF(res.pdf) && pages(res.pdf) === Math.ceil(classSize / 2) && res.held === '0', `${pages(res.pdf ?? Buffer.alloc(0))} pages for ${classSize} students`)
}
const myCards = await must(student('GET', '/routines/admit-cards/me'), 'my cards')
const own = await student('GET', `/routines/admit-cards/me/${term.id}/pdf?lang=bn`)
if (isPDF(own.pdf)) writeFileSync(join(pdfDir, 'admit-card-student-bn.pdf'), own.pdf)
check('the student downloads their own admit card once the routine is published', myCards.some(c => c.term_id === term.id && !c.held) && isPDF(own.pdf) && pages(own.pdf) === 1, `${own.status}`)

// The fee hold: an unpaid exam fee due today for the demo student, the hold on
// with a cutoff of today; the office releases the student.
const category = await must(admin('POST', '/fees/categories', { code: `EF${stamp}`, name_en: 'Exam fee', name_bn: 'পরীক্ষার ফি', frequency: 'one_time', is_active: true }), 'fee category')
const structure = await must(admin('POST', '/fees/structures', { session_id: me.session_id, class_id: cls, category_id: category.id, amount: 300, due_day: 10, is_active: true, late_fine: 0 }), 'fee structure')
await must(admin('POST', '/fees/dues', { student_id: me.id, structure_id: structure.id, period: '' }), 'exam fee due')
await open(b, 'admin', '/admit-cards')
await b.waitFor(`!!document.querySelector('[data-testid=admit-settings]')`, 20000)
await b.evaluate(`(() => { const f = document.querySelector('[data-testid=admit-settings]'); const box = f.querySelector('input[type=checkbox]'); if (!box.checked) box.click(); return true })()`)
await sleep(200)
await b.evaluate(set('[data-testid=admit-settings] input[type=date]', today))
await b.evaluate(clickText('Save settings'))
await b.waitFor(`(document.querySelector('main')?.innerText ?? '').includes('Settings saved.')`, 15000)
const settings = await must(admin('GET', '/routines/admit-cards/settings'), 'settings')
check('the administrator turns the fee hold on with a cutoff date', settings.withhold_for_dues && settings.dues_cutoff === today, js({ on: settings.withhold_for_dues, cutoff: settings.dues_cutoff }))
const heldList = await must(accounts('GET', `/routines/admit-cards/withheld?term_id=${term.id}`), 'held')
const target = heldList.find(s => s.student_id === me.id && s.held)
const held = await admin('GET', `/routines/admit-cards/terms/${term.id}/pdf?class_id=${cls}&lang=en`)
check('held students are left out of the class’s cards', heldList.length > 0 && Number(held.held) === heldList.filter(s => s.held && s.class_id === cls).length, `${heldList.length} with dues, ${held.held} left out`)
const heldRes = await student('GET', `/routines/admit-cards/me/${term.id}/pdf?lang=en`)
check('the student with the unpaid fee is held and cannot download their card', !!target && heldRes.status === 409 && heldRes.code === 'admit_card_held', `${heldRes.status} ${heldRes.code}`)
await open(b, 'student', '/student/routine?tab=exam')
check('the student sees why their card is held', await b.waitFor(`(document.querySelector('[data-testid=my-admit-cards]')?.innerText ?? '').includes('contact the school office')`, 20000), '')
// The office (accountant) sees the list and releases a student in the browser.
await open(b, 'accounts', '/admit-cards')
await b.waitFor(`!!document.querySelector('[data-testid=admit-held] button')`, 20000)
const releaseName = target?.name_en
await b.evaluate(`(() => { const li = [...document.querySelectorAll('[data-testid=admit-held] li')].find(x => x.innerText.includes(${js(releaseName ?? '')})); const input = li?.querySelector('input'); if (input) (${setValue})(input, 'Paid at the counter'); const btn = li && [...li.querySelectorAll('button')].find(x => x.innerText.trim() === 'Release'); if (!btn) return false; btn.click(); return true })()`)
await b.waitFor(`(document.querySelector('[data-testid=admit-held]')?.innerText ?? '').includes('Released:')`, 15000)
const afterRelease = (await must(accounts('GET', `/routines/admit-cards/withheld?term_id=${term.id}`), 'held')).find(s => s.student_id === target?.student_id)
check('the office (accountant) releases the held student in the browser', !!target && afterRelease?.released && !afterRelease.held && afterRelease.release_note === 'Paid at the counter', target ? `${target.name_en}` : 'nobody held')
const released = await student('GET', `/routines/admit-cards/me/${term.id}/pdf?lang=en`)
check('once released, the student downloads their card', isPDF(released.pdf), `${released.status}`)
const teacherSettings = await teacher('PUT', '/routines/admit-cards/settings', { rules_en: '', rules_bn: '', withhold_for_dues: false, dues_cutoff: null })
check('only the administrator changes the admit card settings', teacherSettings.status === 403, `${teacherSettings.status}`)

// ---- 7. Screens --------------------------------------------------------------------
await matrix(b, 'admit-cards-admin', 'admin', '/admit-cards', `!!document.querySelector('[data-testid=admit-settings]') && !!document.querySelector('[data-testid=admit-print]')`)
await matrix(b, 'comments-teacher', 'teacher', '/exams/comments', `!!document.querySelector('[data-testid=comment-rows]')`)
await matrix(b, 'staff-signature', 'teacher', '/staff/me', `!!document.querySelector('[data-testid=signature-card] img')`)
await matrix(b, 'class-teachers', 'admin', '/admin/academic/sections', `!!document.querySelector('[data-testid=class-teachers] select[aria-label^="Class teacher of"]')`)
await matrix(b, 'student-admit', 'student', '/student/routine?tab=exam', `!!document.querySelector('[data-testid=my-admit-cards]')`)

// ---- Leave the school as it was: the hold off, the rules empty --------------------
await must(admin('PUT', '/routines/admit-cards/settings', { rules_en: '', rules_bn: '', withhold_for_dues: false, dues_cutoff: null }), 'restore settings')
for (const room of rooms) await must(admin('PATCH', `/routines/rooms/${room.id}`, { ...room, is_active: false }), 'retire room')
const pageErrors = await b.evaluate('window.__errors ?? []')
check('no uncaught exceptions in the browser', pageErrors.length === 0, js(pageErrors))
b.close()
r.save(join(shots, 'marksheet-admit.json'))
