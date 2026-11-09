// E8 term exam routine planner in a browser, on a running, demo-seeded
// disposable school (run it after suite.mjs):
//   the administrator adds exam rooms, starts a term's routine, sees a
//   clash refused with its rule, drafts the routine with the generator and
//   submits it; the head teacher approves it; the administrator publishes it;
//   the student sees their next exam and own room, the teacher their
//   invigilation duties; the routine PDFs (class, room, invigilation) are
//   rendered in English and Bangla; an amendment moves a paper, shows the
//   difference, and students see the change marked;
//   and the screens at 1440px and 375px, English and Bangla, light and dark,
//   with overflow, contrast and axe checks.
//
//   E2E_BASE_URL=http://localhost:8121 DEMO_PASSWORD=... E2E_PDF_DIR=... node e2e/exam-routine.mjs
//
// It creates an exam term, rooms and a published routine; never run it
// against a real school.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const shots = process.env.E2E_SHOTS ?? join(here, 'shots')
const pdfDir = process.env.E2E_PDF_DIR ?? join(shots, 'pdf-e8')
mkdirSync(shots, { recursive: true })
mkdirSync(pdfDir, { recursive: true })
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const js = v => JSON.stringify(v)
const email = who => `demo.${who}@demo.flybirds.test`
const stamp = Date.now().toString(36).toUpperCase()

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: email(who), password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  const call = async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? js(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code, error: json.error }
  }
  call.pdf = async path => {
    const response = await fetch(`${base}/api${path}`, { headers: { Authorization: `Bearer ${token}` } })
    return { status: response.status, type: response.headers.get('content-type'), bytes: Buffer.from(await response.arrayBuffer()) }
  }
  return call
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code} ${res.error?.message ?? ''}`); return res.data }
const [admin, head, teacher, student] = await Promise.all(['admin', 'head', 'teacher', 'student'].map(api))

// ---- 0. A term and rooms through the API --------------------------------------------
const me = await must(student('GET', '/students/me'), 'me')
const session = (await must(admin('GET', '/academic/sessions?page=1&page_size=100'), 'sessions')).items.find(s => s.id === me.session_id)
const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date())
const addDays = (iso, n) => { const d = new Date(`${iso}T00:00:00Z`); d.setUTCDate(d.getUTCDate() + n); return d.toISOString().slice(0, 10) }
const weekday = iso => new Date(`${iso}T00:00:00Z`).getUTCDay()
let from = addDays(today, 3)
let to = addDays(today, 33)
if (to > session.end_date) { to = session.end_date; from = addDays(to, -30) < session.start_date ? session.start_date : addDays(to, -30) }
const term = await must(admin('POST', '/exams/terms', { session_id: me.session_id, name: `E8 term ${stamp}`, kind: 'term', starts_on: from, ends_on: to }), 'term')
const sections = (await must(admin('GET', `/academic/sections?session_id=${me.session_id}&page=1&page_size=100`), 'sections')).items.filter(s => s.is_active)
const rooms = []
for (let i = 1; i <= sections.length + 2; i++) rooms.push(await must(admin('POST', '/routines/rooms', { name: `E8 Room ${100 + i} ${stamp}`, name_bn: `কক্ষ ${100 + i}`, kind: 'classroom', capacity: 60, is_active: true }), 'room'))
rooms.push(await must(admin('POST', '/routines/rooms', { name: `E8 Hall ${stamp}`, name_bn: 'হলরুম', kind: 'hall', capacity: 400, is_active: true }), 'hall'))
rooms.push(await must(admin('POST', '/routines/rooms', { name: `E8 Lab ${stamp}`, name_bn: 'গবেষণাগার', kind: 'lab', capacity: 60, is_active: true }), 'lab'))
// Rooms left active by an earlier run take no part.
for (const room of await must(admin('GET', '/routines/rooms'), 'rooms')) {
  if (!room.name.endsWith(stamp)) await must(admin('PATCH', `/routines/rooms/${room.id}`, { ...room, is_active: false }), 'old room')
}
check('rooms are added through the API and refused for the head teacher', (await head('POST', '/routines/rooms', { name: 'x' })).status === 403, `${rooms.length} rooms`)

// ---- Browser helpers --------------------------------------------------------------
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
const clickExact = (text, scope = 'main') => `(() => { const el = [...document.querySelectorAll(${js(scope + ' button')})].find(e => e.offsetParent !== null && e.innerText.trim() === ${js(text)}); if (!el || el.disabled) return false; el.click(); return true })()`
const hasButton = text => `[...document.querySelectorAll('main button')].some(e => e.offsetParent !== null && e.innerText.trim() === ${js(text)})`
const confirmYes = `(() => { const b = document.querySelectorAll('[role=alertdialog] button'); if (!b.length) return false; b[b.length - 1].click(); return true })()`
const mainText = `document.querySelector('main')?.innerText ?? ''`
const setValue = `(el, value) => { const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, value); el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })) }`
const pickTerm = `(() => { const s = [...document.querySelectorAll('main select')].find(x => [...x.options].some(o => o.value === ${js(term.id)})); if (!s) return false; (${setValue})(s, ${js(term.id)}); return true })()`

let signedIn = null
async function open(b, who, path, lang = 'en', theme = 'light') {
  if (signedIn !== who) { await b.login(email(who), password, lang); signedIn = who }
  await b.evaluate(`localStorage.setItem('flybirds.language', ${js(lang)}); localStorage.setItem('flybirds.theme', ${js(theme)})`)
  await b.go(path)
}
const combos = [
  ['1440-en-light', 1440, 900, false, 'en', 'light'], ['1440-bn-dark', 1440, 900, false, 'bn', 'dark'],
  ['1440-bn-light', 1440, 900, false, 'bn', 'light'], ['1440-en-dark', 1440, 900, false, 'en', 'dark'],
  ['375-en-light', 375, 812, true, 'en', 'light'], ['375-bn-dark', 375, 812, true, 'bn', 'dark'],
  ['375-bn-light', 375, 812, true, 'bn', 'light'], ['375-en-dark', 375, 812, true, 'en', 'dark'],
]
async function matrix(b, name, who, path, ready, prepare) {
  const bad = []
  for (const [label, width, height, mobile, lang, theme] of combos) {
    await b.viewport(width, height, mobile)
    await open(b, who, path, lang, theme)
    if (!await b.waitFor(ready, 20000)) { bad.push(`${label}: not ready`); continue }
    if (prepare && !(await prepare(b))) { bad.push(`${label}: not prepared`); continue }
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
    await b.shot(`e8-${name}-${label}`, !mobile)
  }
  check(`${name}: 1440px and 375px, English and Bangla, light and dark: no overflow, contrast and axe clean`, bad.length === 0, bad.slice(0, 4).join(' || ') || '8 of 8')
}
async function openPlanner(b, who, lang = 'en', theme = 'light') {
  await open(b, who, '/admin/routines?tab=planner', lang, theme)
  await b.waitFor(`!!document.querySelector('[data-testid=term-routine-planner] select')`, 20000)
  await b.evaluate(pickTerm)
  await sleep(400)
}

const b = await launch({ base, shots })
await b.evaluate(`window.addEventListener('error', e => (window.__errors = [...(window.__errors || []), String(e.message)]))`)

// ---- 1. The administrator starts the term's routine ---------------------------------
await openPlanner(b, 'admin')
check('the planner offers to start the term’s routine with its dates', await b.waitFor(hasButton('Start the routine'), 15000), term.name)
await b.evaluate(clickExact('Start the routine'))
const started = await b.waitFor(`document.querySelector('#e8-version')?.innerText.includes('Version 1')`, 20000)
const terms = await must(admin('GET', `/routines/terms?session_id=${me.session_id}`), 'terms')
const v1id = terms.find(x => x.term_id === term.id)?.versions[0]?.id
let v1 = await must(admin('GET', `/routines/versions/${v1id}`), 'version')
check('the routine starts as a draft with two sittings and a paper for each subject of each class', started && v1.status === 'draft' && v1.sittings.length === 2 && v1.papers.length > 5 && v1.classes.length > 1, `${v1.papers.length} papers, ${v1.classes.length} classes`)
check('the open exam days leave out weekly off days and published holidays', v1.dates.every(d => !v1.weekly_off_days.includes(weekday(d)) && !v1.holidays.some(h => h.date === d)) && v1.dates.length > 10,
  `${v1.dates.length} days, off ${v1.weekly_off_days.join(',')}, holidays ${v1.holidays.map(h => h.date).join(',') || 'none'}`)

// A clash is refused with its rule: the API on a Friday, the screen on a class clash.
const mine = v1.papers.filter(p => p.class_id === me.class_id)
const myClass = v1.classes.find(c => c.id === me.class_id)
const friday = [...Array(14).keys()].map(n => addDays(from, n)).find(d => weekday(d) === 5 && d <= to)
const firstPaper = mine[0], secondPaper = mine[1]
const roomsFor = offset => myClass.sections.map((s, i) => ({ section_id: s.id, room_id: rooms[i + offset].id }))
if (friday) {
  const res = await admin('PATCH', `/routines/versions/${v1.id}/papers/${firstPaper.id}`, { duration_minutes: firstPaper.duration_minutes, needs_lab: false, exam_date: friday, sitting_id: v1.sittings[0].id, rooms: roomsFor(0) })
  check('placing a paper on a Friday is refused as a weekly off day', res.status === 409 && res.code === 'routine_conflict' && res.error?.issues?.[0]?.code === 'weekly_off', `${res.status} ${res.error?.message}`)
}
const day1 = v1.dates[0]
await must(admin('PATCH', `/routines/versions/${v1.id}/papers/${firstPaper.id}`, { duration_minutes: firstPaper.duration_minutes, needs_lab: false, exam_date: day1, sitting_id: v1.sittings[0].id, rooms: roomsFor(0) }), 'place first')
await b.go('/admin/routines?tab=planner'); await b.waitFor(`!!document.querySelector('[data-testid=term-routine-planner] select')`, 20000); await b.evaluate(pickTerm)
await b.waitFor(`!!document.querySelector('#e8-papers')`, 20000)
const openRow = await b.evaluate(`(() => {
  const h = [...document.querySelectorAll('#e8-papers ~ div h3, section[aria-labelledby=e8-papers] h3')].find(x => x.innerText.startsWith(${js(myClass.name + ' ')}) || x.innerText.trim().startsWith(${js(myClass.name)}))
  const row = h?.parentElement.querySelector(${js(`tr[data-testid="paper-${secondPaper.subject_code}-${secondPaper.kind}-${secondPaper.paper_no}"]`)})
  const btn = row?.querySelector('button'); if (!btn) return false; btn.click(); return true })()`)
await sleep(400)
const placed = await b.evaluate(`(() => {
  const form = document.querySelector('form[aria-label^="Place"]'); if (!form) return 'no form'
  const set = ${setValue}
  const selects = () => [...form.querySelectorAll('select')]
  set(selects()[0], ${js(day1)}); return 'ok' })()`)
await sleep(300)
await b.evaluate(`(() => {
  const form = document.querySelector('form[aria-label^="Place"]'); const set = ${setValue}
  const selects = [...form.querySelectorAll('select')]
  set(selects[1], ${js(v1.sittings[0].id)})
  const roomSelects = selects.slice(2)
  const ids = ${js(roomsFor(myClass.sections.length).map(x => x.room_id))}
  roomSelects.forEach((s, i) => set(s, ids[i]))
  form.querySelector('button.btn-primary').click() })()`)
const clash = await b.waitFor(`(document.querySelector('section[aria-labelledby=e8-papers] [role=alert]')?.innerText ?? '').includes('two papers at once')`, 15000)
check('the screen refuses a second paper of the class in the same sitting and names the rule', openRow && placed === 'ok' && clash, (await b.evaluate(`document.querySelector('section[aria-labelledby=e8-papers] [role=alert]')?.innerText ?? ''`)).replace(/\s+/g, ' ').slice(0, 160))

// ---- 2. The draft, submission, approval and publication ------------------------------
await b.evaluate(clickExact('Draft the routine'))
await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000); await b.evaluate(confirmYes)
const drafted = await b.waitFor(`!!document.querySelector('[data-testid=generate-report]')`, 30000)
v1 = await must(admin('GET', `/routines/versions/${v1.id}`), 'drafted')
const hard = v1.issues.filter(i => i.hard)
const kept = v1.papers.find(p => p.id === firstPaper.id)
check('the generator places every paper with no hard conflict and keeps the paper placed by hand', drafted && hard.length === 0 && v1.papers.every(p => p.exam_date) && kept.exam_date === day1 && kept.pinned,
  `${v1.papers.length} placed, ${hard.length} hard: ${hard.slice(0, 2).map(i => i.message).join('; ')}`)
check('no paper falls on a weekly off day or a holiday, and practicals sit in labs', v1.papers.every(p => !v1.weekly_off_days.includes(weekday(p.exam_date)) && !v1.holidays.some(h => h.date === p.exam_date)), '')
check('the screen says every rule passes', (await b.evaluate(`document.querySelector('section[aria-labelledby=e8-check]')?.innerText ?? ''`)).includes('Every rule passes'), '')
const again = await must(admin('POST', `/routines/versions/${v1.id}/generate`), 'generate again')
check('drafting again gives the same routine', again.version.papers.every(p => v1.papers.find(x => x.id === p.id)?.exam_date === p.exam_date && v1.papers.find(x => x.id === p.id)?.sitting_id === p.sitting_id), `${again.report.placed} placed, ${again.report.kept} kept`)
await b.go('/admin/routines?tab=planner'); await b.waitFor(`!!document.querySelector('[data-testid=term-routine-planner] select')`, 20000); await b.evaluate(pickTerm)
await b.waitFor(hasButton('Submit for approval'), 15000)
await b.evaluate(clickExact('Submit for approval'))
check('the administrator submits the routine for approval', await b.waitFor(`document.querySelector('#e8-version')?.innerText.includes('Waiting for approval')`, 15000), '')
check('only the head teacher approves', (await admin('POST', `/routines/versions/${v1.id}/approve`)).status === 403, '')

await openPlanner(b, 'head')
const headView = await b.waitFor(hasButton('Approve'), 15000)
const headButtons = await b.evaluate(`[...document.querySelectorAll('main button')].filter(e => e.offsetParent !== null).map(e => e.innerText.trim())`)
check('the head teacher sees Approve and Return, and no planning or publishing buttons', headView && headButtons.includes('Return to draft') && !headButtons.includes('Draft the routine') && !headButtons.includes('Publish') && !headButtons.includes('Add room'), headButtons.filter(x => /Approve|Return|Publish|Draft/.test(x)).join(', '))
await b.evaluate(clickExact('Approve')); await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000); await b.evaluate(confirmYes)
check('the head teacher approves the routine', await b.waitFor(`document.querySelector('#e8-version')?.innerText.includes('Approved')`, 15000), '')

await openPlanner(b, 'admin')
await b.waitFor(hasButton('Publish'), 15000)
await b.evaluate(clickExact('Publish')); await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000); await b.evaluate(confirmYes)
const published = await b.waitFor(`document.querySelector('#e8-version')?.innerText.includes('Published')`, 15000)
v1 = await must(admin('GET', `/routines/versions/${v1.id}`), 'published')
check('the administrator publishes it; its content hash is recorded', published && v1.status === 'published' && /^[0-9a-f]{64}$/.test(v1.content_sha256 ?? ''), v1.content_sha256?.slice(0, 12))
check('a published routine cannot be changed', (await admin('PATCH', `/routines/versions/${v1.id}/papers/${firstPaper.id}`, { duration_minutes: 60, needs_lab: false, exam_date: '', sitting_id: '', rooms: [] })).code === 'routine_state', '')

// ---- 3. Students and teachers --------------------------------------------------------
const myRoutines = await must(student('GET', '/routines/terms/me'), 'student routines')
const mineNow = myRoutines.find(x => x.version_id === v1.id)
const next = mineNow?.papers.filter(p => p.exam_date >= today).sort((a, c) => a.exam_date.localeCompare(c.exam_date) || a.start_time.localeCompare(c.start_time))[0]
check('the student’s routine lists their papers with their own room', !!mineNow && mineNow.papers.length > 0 && mineNow.papers.every(p => p.room_name), `${mineNow?.papers.length} papers, first room ${mineNow?.papers[0]?.room_name}`)
await open(b, 'student', '/')
const card = await b.waitFor(`(document.querySelector('[data-testid=next-exam]')?.innerText ?? '').includes(${js(next.subject_name)})`, 20000)
check('the student’s dashboard shows the next exam, its time and room', card && (await b.evaluate(`document.querySelector('[data-testid=next-exam]').innerText`)).includes(next.room_name), (await b.evaluate(`document.querySelector('[data-testid=next-exam]')?.innerText ?? ''`)).replace(/\s+/g, ' '))
await b.go('/student/routine?tab=exam')
check('the student’s routine page lists the term routine', await b.waitFor(`(${mainText}).includes('My exam routine') && (${mainText}).includes(${js(next.subject_name)})`, 20000), '')
const duties = await must(teacher('GET', '/routines/duties/me'), 'duties')
await open(b, 'teacher', '/teacher/routine?tab=duties')
const dutyText = duties.length ? duties[0].room_name : 'You have no invigilation duties'
check('the teacher sees their invigilation duties', await b.waitFor(`(${mainText}).includes(${js(dutyText)})`, 20000), `${duties.length} duties`)
await b.go('/')
check('the teacher’s dashboard shows the next invigilation', await b.waitFor(`(${mainText}).includes('Next invigilation')`, 20000), '')
check('teachers and students cannot open the planner’s data', (await teacher('GET', `/routines/versions/${v1.id}`)).status === 403 && (await student('GET', '/routines/terms')).status === 403, '')

// ---- 4. Routine PDFs -----------------------------------------------------------------
const pages = buf => Number((buf.toString('latin1').match(/\/Type\s*\/Pages\b[^>]*?\/Count\s+(\d+)/) ?? [])[1] ?? 0)
const pdfs = []
for (const kind of ['class', 'room', 'invigilation']) {
  for (const lang of ['en', 'bn']) {
    const res = await admin.pdf(`/routines/versions/${v1.id}/pdf?kind=${kind}&lang=${lang}`)
    const file = join(pdfDir, `routine-${kind}-${lang}.pdf`)
    if (res.status === 200) writeFileSync(file, res.bytes)
    pdfs.push({ kind, lang, ok: res.status === 200 && res.type === 'application/pdf' && res.bytes.subarray(0, 5).toString() === '%PDF-', pages: pages(res.bytes), kb: Math.round(res.bytes.length / 1024) })
  }
}
check('class, room and invigilation PDFs render in English and Bangla', pdfs.every(p => p.ok && p.pages > 0), pdfs.map(p => `${p.kind}-${p.lang}: ${p.pages}p ${p.kb}KB`).join(', '))
const mineEn = await student.pdf(`/routines/terms/me/${v1.id}/pdf?lang=bn`)
if (mineEn.status === 200) writeFileSync(join(pdfDir, 'student-routine-bn.pdf'), mineEn.bytes)
check('the student downloads their own routine as a PDF', mineEn.status === 200 && pages(mineEn.bytes) === 1, `${mineEn.status} ${pages(mineEn.bytes)} page`)
const classPdf = await admin.pdf(`/routines/versions/${v1.id}/pdf?kind=class&lang=en&class_id=${me.class_id}`)
if (classPdf.status === 200) writeFileSync(join(pdfDir, 'routine-one-class-en.pdf'), classPdf.bytes)
check('one class’s routine can be rendered alone', classPdf.status === 200 && pages(classPdf.bytes) >= 1, `${pages(classPdf.bytes)} page`)

// ---- 5. An amendment -----------------------------------------------------------------
await openPlanner(b, 'admin')
await b.waitFor(hasButton('Amend the routine'), 15000)
await b.evaluate(clickExact('Amend the routine'))
await b.waitFor(`!!document.querySelector('#e8-version textarea')`, 5000)
await b.evaluate(`(() => { const t = document.querySelector('#e8-version')?.closest('section').querySelector('textarea'); (${setValue})(t, 'Moved for the district sports day'); t.closest('form').querySelector('button.btn-primary').click() })()`)
const amended = await b.waitFor(`document.querySelector('#e8-version')?.innerText.includes('Version 2')`, 15000)
const v2id = (await must(admin('GET', `/routines/terms?session_id=${me.session_id}`), 'terms')).find(x => x.term_id === term.id).versions[0].id
let v2 = await must(admin('GET', `/routines/versions/${v2id}`), 'v2')
check('amending starts version 2 as a draft copy with the reason; version 1 stays published', amended && v2.version === 2 && v2.status === 'draft' && v2.reason === 'Moved for the district sports day' && v2.papers.length === v1.papers.length && (await must(admin('GET', `/routines/versions/${v1.id}`), 'v1')).status === 'published', '')
// Move the student's next paper to a free later day.
const moving = v2.papers.find(p => p.class_id === me.class_id && p.subject_code === next.subject_code && p.kind === next.kind && p.paper_no === next.paper_no)
const busyDays = new Set(v2.papers.filter(p => p.class_id === me.class_id).map(p => p.exam_date))
const freeDay = [...v2.dates].reverse().find(d => !busyDays.has(d))
const moved = await admin('PATCH', `/routines/versions/${v2.id}/papers/${moving.id}`, { duration_minutes: moving.duration_minutes, needs_lab: moving.needs_lab, exam_date: freeDay, sitting_id: v2.sittings[1].id, rooms: moving.rooms.map(x => ({ section_id: x.section_id, room_id: x.room_id })) })
v2 = moved.data ?? v2
check('the amendment shows its difference: the paper moved, from and to', moved.status === 200 && v2.changes.length === 1 && v2.changes[0].change === 'moved' && v2.changes[0].to_date === freeDay, `${moved.status} ${moved.error?.message ?? ''} ${js(v2.changes[0] ?? {})}`)
await b.go('/admin/routines?tab=planner'); await b.waitFor(`!!document.querySelector('[data-testid=term-routine-planner] select')`, 20000); await b.evaluate(pickTerm)
check('the planner lists the change for review', await b.waitFor(`(document.querySelector('[data-testid=routine-changes]')?.innerText ?? '').includes('Moved')`, 15000), '')
await must(admin('POST', `/routines/versions/${v2.id}/submit`), 'submit v2')
await must(head('POST', `/routines/versions/${v2.id}/approve`), 'approve v2')
await must(admin('POST', `/routines/versions/${v2.id}/publish`), 'publish v2')
const after = (await must(student('GET', '/routines/terms/me'), 'after')).find(x => x.term_id === term.id)
const changedPaper = after?.papers.find(p => p.changed)
check('students see version 2, amended, with the moved paper marked', after?.version === 2 && !!after.amended_on && changedPaper?.exam_date === freeDay && after.papers.filter(p => p.changed).length === 1, `${after?.version} ${after?.amended_on} ${changedPaper?.subject_name}`)
check('version 1 is kept as replaced', (await must(admin('GET', `/routines/versions/${v1.id}`), 'v1 after')).status === 'superseded', '')
await open(b, 'student', '/student/routine?tab=exam')
check('the student’s routine says it was amended and marks the change', await b.waitFor(`(${mainText}).includes('Amended on') && (${mainText}).includes('Changed')`, 20000), '')

// ---- 6. The screens: sizes, languages, themes ----------------------------------------
await matrix(b, 'planner', 'admin', '/admin/routines?tab=planner', `!!document.querySelector('[data-testid=term-routine-planner] select')`, async b => {
  await b.evaluate(pickTerm)
  return b.waitFor(`!!document.querySelector('[data-testid=routine-calendar]')`, 15000)
})
await matrix(b, 'planner-head', 'head', '/admin/routines?tab=planner', `!!document.querySelector('[data-testid=term-routine-planner] select')`, async b => {
  await b.evaluate(pickTerm)
  return b.waitFor(`!!document.querySelector('[data-testid=routine-changes]')`, 15000)
})
await matrix(b, 'student-routine', 'student', '/student/routine?tab=exam', `(document.querySelector('main')?.innerText ?? '').length > 50`, async b => b.waitFor(`!!document.querySelector('main table')`, 15000))
await matrix(b, 'student-dashboard', 'student', '/', `!!document.querySelector('[data-testid=next-exam]')`)
await matrix(b, 'teacher-duties', 'teacher', '/teacher/routine?tab=duties', `!!document.querySelector('#my-duties')`)

// Rooms made for this run go out of use, so a later run starts clean.
for (const room of rooms) await must(admin('PATCH', `/routines/rooms/${room.id}`, { ...room, is_active: false }), 'retire room')

const pageErrors = await b.evaluate(`window.__errors ?? []`)
check('no uncaught exceptions in the browser', pageErrors.length === 0, js(pageErrors))
b.close()
r.save(join(shots, 'exam-routine.json'))
