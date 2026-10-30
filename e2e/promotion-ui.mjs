// M15 stage B2: the year-end promotion screens in a browser, on a running,
// demo-seeded disposable school. Class progression set on its screen, a batch
// created from the form, one student overridden with a reason in the edit
// dialog, approval by the head teacher, apply by the administrator through
// the confirmation, revert refused after activity in the new year (and the
// reason shown), then reverted. Who sees what, the concession review, and
// every screen at 1440px and 375px in English and Bangla, light and dark,
// with overflow, contrast and axe checks.
//
//   E2E_BASE_URL=http://localhost:8121 DEMO_PASSWORD=... node e2e/promotion-ui.mjs
//   (E2E_SHOTS=<folder> keeps the screenshots elsewhere)
//
// It adds a session, a class, sections, a fee category and structure, and
// promotes the demo class before reverting; never run it against a real school.
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const shots = process.env.E2E_SHOTS ?? join(here, 'shots')
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const stamp = Date.now().toString(36).slice(-4).toUpperCase()
const email = who => `demo.${who}@demo.flybirds.test`

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email(who), password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }
const [admin, student] = await Promise.all(['admin', 'student'].map(api))

// ---- 0. The demo class, a next session, a next class and its sections (API) ----
const me = await must(student('GET', '/students/me'), 'me')
const from = me.session_id, cls = me.class_id
const classStudents = (await must(admin('GET', `/students?session_id=${from}&class_id=${cls}&status=active&page=1&page_size=100`), 'class')).items
const className = (await must(admin('GET', '/academic/classes?page=1&page_size=100'), 'classes')).items.find(c => c.id === cls).name
const sessions = (await must(admin('GET', '/academic/sessions?page=1&page_size=100'), 'sessions')).items
const year = Math.max(...sessions.map(s => Number(s.name.slice(0, 4)))) + 1
const to = await must(admin('POST', '/academic/sessions', { name: String(year), start_date: `${year}-01-01`, end_date: `${year}-12-31` }), 'next session')
const nextName = `E2E Next ${stamp}`
const next = await must(admin('POST', '/academic/classes', { name: nextName, display_order: 99000, is_active: true }), 'next class')
for (const id of new Set(classStudents.map(s => s.section_id))) {
  const sec = await must(admin('GET', `/academic/sections/${id}`), 'section')
  for (const target of [next.id, cls]) await must(admin('POST', '/academic/sections', { session_id: to.id, class_id: target, name: sec.name, code: sec.code, display_order: sec.display_order, is_active: true }), 'next-year section')
}
const peers = classStudents.filter(s => s.id !== me.id)
// A concession this year for one classmate, for the concession review.
const category = await must(admin('POST', '/fees/categories', { code: `PM${stamp}`, name_en: `E2E tuition ${stamp}`, name_bn: `ই২ই বেতন ${stamp}`, frequency: 'one_time', is_active: true }), 'fee category')
await must(admin('POST', '/fees/concessions', { student_id: peers[0].id, category_id: category.id, percent: 25, reason: 'Sibling discount' }), 'concession')

// ---- Browser helpers --------------------------------------------------------------
const axePath = join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
const contrast = `(() => {
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  const rgb = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = '#000'; ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const opaque = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return ctx.getImageData(0, 0, 1, 1).data[3] > 250 }
  const bg = el => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (c && opaque(c)) return c } return getComputedStyle(document.body).backgroundColor }
  const ratio = el => { const a = lum(rgb(getComputedStyle(el).color)), b = lum(rgb(bg(el))); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) }
  const sel = 'main h1, main h2, main h3, main p, main span, main td, main th, main button, main a, main select, main label, main dt, main dd, main li, [role=dialog] *'
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && el.children.length === 0 && !el.closest('[disabled]') && !el.disabled).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
const js = v => JSON.stringify(v)
// set: change a React-controlled field the way a user would.
const set = (selector, value) => `(() => { const el = document.querySelector(${js(selector)}); if (!el) return false
  const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, ${js(value)}); el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })); return true })()`
const click = selector => `(() => { const el = [...document.querySelectorAll(${js(selector)})].find(e => e.offsetParent !== null); if (!el || el.disabled) return false; el.click(); return true })()`
const visible = selector => `[...document.querySelectorAll(${js(selector)})].some(e => e.offsetParent !== null)`
const confirmYes = `(() => { const b = document.querySelectorAll('[role=alertdialog] button'); if (!b.length) return false; b[b.length - 1].click(); return true })()`

// open signs in only when the account changes (the server rate-limits sign-in);
// language and theme come from local storage on each load.
let signedIn = null
async function open(b, who, path, lang = 'en', theme = 'light') {
  if (signedIn !== who) { await b.login(email(who), password, lang); signedIn = who }
  await b.evaluate(`localStorage.setItem('flybirds.language', ${js(lang)}); localStorage.setItem('flybirds.theme', ${js(theme)})`)
  await b.go(path)
}
async function batchVia(id) { return must(admin('GET', `/promotions/${id}`), 'batch') }

const combos = [
  ['1440-en-light', 1440, 900, false, 'en', 'light'], ['1440-bn-dark', 1440, 900, false, 'bn', 'dark'],
  ['1440-bn-light', 1440, 900, false, 'bn', 'light'], ['1440-en-dark', 1440, 900, false, 'en', 'dark'],
  ['375-en-light', 375, 812, true, 'en', 'light'], ['375-bn-dark', 375, 812, true, 'bn', 'dark'],
  ['375-bn-light', 375, 812, true, 'bn', 'light'], ['375-en-dark', 375, 812, true, 'en', 'dark'],
]
// matrix: the screen in every size, language and theme; overflow, contrast and
// axe on each, and a screenshot. prepare() opens a dialog when the screen is one.
async function matrix(b, name, who, path, ready, prepare) {
  const bad = []
  for (const [label, width, height, mobile, lang, theme] of combos) {
    await b.viewport(width, height, mobile)
    await open(b, who, path, lang, theme)
    if (!await b.waitFor(ready, 20000)) { bad.push(`${label}: not ready`); continue }
    if (prepare) await prepare(b)
    await sleep(500)
    const state = await b.evaluate(`({ overflow: document.documentElement.scrollWidth > window.innerWidth + 1, theme: document.documentElement.dataset.theme, lang: document.documentElement.lang })`)
    if (state.overflow || state.theme !== theme || state.lang !== lang) bad.push(`${label}: ${js(state)}`)
    const low = await b.evaluate(contrast)
    if (low.length) bad.push(`${label} contrast: ${low.join(' | ')}`)
    if (existsSync(axePath)) {
      await b.evaluate(readFileSync(axePath, 'utf8'))
      const axe = await b.evaluate(`axe.run(document.querySelector('[role=dialog]') ?? document.querySelector('main'), { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(x => x.id + ':' + x.nodes.length))`)
      if (axe.length) bad.push(`${label} axe: ${axe.join(', ')}`)
    }
    await b.shot(`m15b2-${name}-${label}`, !mobile)
  }
  check(`${name}: 1440px and 375px, English and Bangla, light and dark: no overflow, contrast and axe clean`, bad.length === 0, bad.slice(0, 4).join(' || ') || '8 of 8')
}

const b = await launch({ base, shots })

// ---- 1. Who sees the promotion screens ----------------------------------------------
for (const who of ['teacher', 'accounts', 'student', 'guardian']) {
  await open(b, who, '/dashboard')
  await b.waitFor(`!!document.querySelector('main')`, 15000)
  await sleep(600)
  const link = await b.evaluate(`!!document.querySelector('a[href="/admin/promotions"]')`)
  await b.go('/admin/promotions')
  await b.waitFor(`!!document.querySelector('main [role=alert], main h1')`, 15000)
  await sleep(400)
  const page = await b.evaluate(`({ denied: !!document.querySelector('main section[role=alert]'), screen: !!document.querySelector('[aria-labelledby=pm-batch-list]') })`)
  check(`${who}: no promotion menu entry and the page is refused`, !link && page.denied && !page.screen, js({ link, ...page }))
}
for (const who of ['admin', 'head']) {
  await open(b, who, '/dashboard')
  check(`${who}: the menu offers year-end promotion`, await b.waitFor(`!!document.querySelector('a[href="/admin/promotions"]')`, 15000))
}

// ---- 2. Class progression, set on its screen --------------------------------------
const row = `li[aria-label=${js(className)}]`
await matrix(b, 'progression', 'admin', '/admin/promotions/progression', `!!document.querySelector(${js(row)})`)
await open(b, 'admin', '/admin/promotions/progression')
await b.waitFor(`!!document.querySelector(${js(row)})`, 15000)
const before = await b.evaluate(`document.querySelector(${js(row)}).innerText`)
check('the demo class is marked "Not set" with a suggestion that is not saved', before.includes('Not set') && before.includes('Suggested next class'), before.replace(/\s+/g, ' ').slice(0, 160))
await b.evaluate(click(`${row} input[type=radio]`))
await b.evaluate(set(`${row} select`, next.id))
await b.evaluate(click(`${row} button[type=submit]`))
await b.waitFor(`document.querySelector(${js(row)})?.innerText.includes(${js(nextName)}) && !document.querySelector(${js(row)})?.innerText.includes('Not saved yet')`, 10000)
const saved = (await must(admin('GET', '/promotions/progression'), 'progression')).find(c => c.class_id === cls)
check('saving on the screen sets "next class = X"', saved.setting === 'next' && saved.next_class_id === next.id, saved.setting)

// ---- 3. A batch from the create form -------------------------------------------------
await open(b, 'admin', '/admin/promotions')
await b.waitFor(`document.querySelectorAll('section[aria-labelledby=pm-create] select').length === 3`, 15000)
const selects = 'section[aria-labelledby=pm-create] select'
await b.evaluate(`(() => { const s = document.querySelectorAll(${js(selects)}); const put = (el, v) => { Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, v); el.dispatchEvent(new Event('change', { bubbles: true })) }; put(s[0], ${js(from)}); put(s[1], ${js(cls)}); return true })()`)
await sleep(200)
await b.evaluate(`(() => { const s = document.querySelectorAll(${js(selects)}); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s[2], ${js(to.id)}); s[2].dispatchEvent(new Event('change', { bubbles: true })); return true })()`)
await b.evaluate(click('section[aria-labelledby=pm-create] button[type=submit]'))
await b.waitFor(`/\\/admin\\/promotions\\/[0-9a-f-]{36}$/.test(location.pathname)`, 15000)
const batchId = (await b.evaluate('location.pathname')).split('/').pop()
let batch = await batchVia(batchId)
check('the create form makes a batch of the whole class and opens it', batch.items.length === classStudents.length && batch.status === 'draft', `${batch.items.length} students`)
check('admin sees no approve button', !await b.evaluate(visible('section[aria-labelledby=pm-approve] button')))

// Every classmate but the demo student is decided through the API; the demo
// student is overridden on the screen.
for (const it of batch.items.filter(i => i.student_id !== me.id)) {
  const body = it.recommendation === 'promote' ? { decision: 'promote' } : { decision: 'promote', reason: 'E2E: promoted by the office' }
  await must(admin('PATCH', `/promotions/${batchId}/items/${it.student_id}`, body), 'decide')
}
const meName = batch.items.find(i => i.student_id === me.id).student_name
const editButton = `button[aria-label=${js(`Edit decision for ${meName}`)}]`
await b.go(`/admin/promotions/${batchId}`)
await b.waitFor(visible(editButton), 15000)
await b.evaluate(click(editButton))
await b.waitFor(`!!document.querySelector('[role=dialog] input[value=withdraw]')`, 5000)
await b.evaluate(click('[role=dialog] input[value=withdraw]'))
await sleep(200)
const blocked = await b.evaluate(`document.querySelector('[role=dialog] footer button:last-child').disabled`)
const loginWarning = await b.evaluate(`document.querySelector('[role=dialog]').innerText.includes('login is disabled')`)
check('an override cannot be saved without a reason, and withdrawal warns about the login', blocked && loginWarning, js({ blocked, loginWarning }))
await b.evaluate(set('[role=dialog] textarea', 'Family moved to another district'))
await sleep(200)
await b.evaluate(click('[role=dialog] footer button:last-child'))
await b.waitFor(`!document.querySelector('[role=dialog]')`, 10000)
batch = await batchVia(batchId)
const mine = batch.items.find(i => i.student_id === me.id)
check('the override is saved with its reason', mine.decision === 'withdraw' && mine.reason === 'Family moved to another district', `${mine.decision} ${mine.reason}`)
check('the batch is ready for approval', batch.blockers.length === 0, js(batch.blockers.slice(0, 3)))
await matrix(b, 'batches', 'admin', '/admin/promotions', `!!document.querySelector('a[href="/admin/promotions/${batchId}"]')`)
await matrix(b, 'review', 'admin', `/admin/promotions/${batchId}`, `!!document.querySelector('section[aria-labelledby=pm-students]')`)
await matrix(b, 'override-dialog', 'admin', `/admin/promotions/${batchId}`, `!!document.querySelector('section[aria-labelledby=pm-students]')`, async v => {
  await v.evaluate(`(() => { const e = [...document.querySelectorAll('section[aria-labelledby=pm-students] button[aria-label]')].find(x => x.offsetParent !== null); e?.click(); return !!e })()`)
  await v.waitFor(`!!document.querySelector('[role=dialog]')`, 5000)
})

// ---- 4. Approval by the head teacher ----------------------------------------------------
await open(b, 'head', `/admin/promotions/${batchId}`)
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-approve]')`, 15000)
const panel = await b.evaluate(`document.querySelector('section[aria-labelledby=pm-approve]').innerText`)
check('the head teacher sees what they approve: counts, the override and its reason, the login to disable',
  panel.includes(`${peers.length} promoted to ${nextName}`) && panel.includes('Family moved to another district') && panel.includes('1 student login will be disabled'), panel.replace(/\s+/g, ' ').slice(0, 220))
check('the head teacher has no edit, create or apply controls', !await b.evaluate(visible(editButton)) && !await b.evaluate(visible('section[aria-labelledby=pm-tools]')) && !await b.evaluate(visible('section[aria-labelledby=pm-approved] button')))
await matrix(b, 'approval', 'head', `/admin/promotions/${batchId}`, `!!document.querySelector('section[aria-labelledby=pm-approve]')`)
await open(b, 'head', `/admin/promotions/${batchId}`)
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-approve] button')`, 15000)
await b.evaluate(click('section[aria-labelledby=pm-approve] button'))
await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
await b.evaluate(confirmYes)
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-approved]')`, 10000)
check('the head teacher approves on the screen', (await batchVia(batchId)).status === 'approved')
check('after approval the head teacher sees status only', !await b.evaluate(visible('section[aria-labelledby=pm-approved] button')))

// ---- 5. Apply through the confirmation ---------------------------------------------------
const applyButton = 'section[aria-labelledby=pm-approved] button'
await matrix(b, 'apply-confirm', 'admin', `/admin/promotions/${batchId}`, `!!document.querySelector(${js(applyButton)})`, async v => {
  await v.evaluate(click(applyButton)); await v.waitFor(`!!document.querySelector('[role=dialog]')`, 5000)
})
await open(b, 'admin', `/admin/promotions/${batchId}`)
await b.waitFor(`!!document.querySelector(${js(applyButton)})`, 15000)
await b.evaluate(click(applyButton))
await b.waitFor(`!!document.querySelector('[role=dialog]')`, 5000)
const confirmText = await b.evaluate(`document.querySelector('[role=dialog]').innerText`)
check('the confirmation shows exactly what will happen', confirmText.includes(`${peers.length} promoted to ${nextName}`) && confirmText.includes('1 withdrawn') && /Student logins disabled and signed out\s*1/.test(confirmText), confirmText.replace(/\s+/g, ' ').slice(0, 260))
await b.evaluate(click('[role=dialog] footer button:last-child'))
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-applied]')`, 15000)
check('the administrator applies it', (await batchVia(batchId)).status === 'applied')
check('the withdrawn student can no longer sign in', (await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email('student'), password }) })).status !== 200)

// ---- 6. Revert refused after activity, with the reason shown ----------------------------------
const structure = await must(admin('POST', '/fees/structures', { session_id: to.id, class_id: next.id, category_id: category.id, amount: 100, due_day: 10, is_active: true }), 'structure')
const due = await must(admin('POST', '/fees/dues', { student_id: peers[0].id, structure_id: structure.id, period: '' }), 'a due in the new year')
await open(b, 'admin', `/admin/promotions/${batchId}`)
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-applied]')`, 15000)
const refusedText = await b.evaluate(`document.querySelector('section[aria-labelledby=pm-applied]').innerText`)
check('revert is refused after activity in the new year, naming the student and why', refusedText.includes('can no longer be reverted') && refusedText.includes(`${peers[0].name_en} already has records in the new session`) && !await b.evaluate(visible('section[aria-labelledby=pm-applied] textarea')), refusedText.replace(/\s+/g, ' ').slice(0, 200))
const viaApi = await admin('POST', `/promotions/${batchId}/revert`, { reason: 'E2E' })
check('the API refuses the same revert', viaApi.code === 'revert_refused', viaApi.code)
await matrix(b, 'revert-refused', 'admin', `/admin/promotions/${batchId}`, `!!document.querySelector('section[aria-labelledby=pm-applied]')`)
await matrix(b, 'concessions', 'admin', `/admin/promotions/concessions`, `!!document.querySelector('section[aria-labelledby=pm-conc]') && !document.querySelector('section[aria-labelledby=pm-conc] .animate-pulse')`, async v => {
  await v.evaluate(`(() => { const s = document.querySelectorAll('section[aria-labelledby=pm-conc] select'); const put = (el, x) => { Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, x); el.dispatchEvent(new Event('change', { bubbles: true })) }; put(s[0], ${js(from)}); return true })()`)
  await sleep(150)
  await v.evaluate(`(() => { const s = document.querySelectorAll('section[aria-labelledby=pm-conc] select'); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s[1], ${js(to.id)}); s[1].dispatchEvent(new Event('change', { bubbles: true })); return true })()`)
  await v.waitFor(`document.querySelector('section[aria-labelledby=pm-conc]').innerText.includes(${js(peers[0].name_en)})`, 10000)
})
const concText = await b.evaluate(`document.querySelector('section[aria-labelledby=pm-conc]').innerText`)
check('the concession review lists last year\'s concession as not set for the new session', concText.includes(peers[0].name_en) && concText.includes('Not set'), concText.replace(/\s+/g, ' ').slice(0, 160))
await open(b, 'head', '/admin/promotions/concessions')
await b.waitFor(`!!document.querySelector('main section[role=alert], main h1')`, 15000)
await sleep(400)
check('the head teacher gets no concession review (no fees access) and no tab for it', await b.evaluate(`!!document.querySelector('main section[role=alert]') && !document.querySelector('a[href="/admin/promotions/concessions"]')`))

// ---- 7. Revert once the record is gone ---------------------------------------------------------
await must(admin('DELETE', `/fees/dues/${due.id}`), 'remove the due')
await open(b, 'admin', `/admin/promotions/${batchId}`)
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-applied] textarea')`, 15000)
await b.evaluate(set('section[aria-labelledby=pm-applied] textarea', 'E2E run finished'))
await sleep(200)
await b.evaluate(click('section[aria-labelledby=pm-applied] button[type=submit]'))
await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
await b.evaluate(confirmYes)
await b.waitFor(`!!document.querySelector('section[aria-labelledby=pm-reverted]')`, 15000)
const back = (await must(admin('GET', `/students?session_id=${from}&class_id=${cls}&status=active&page=1&page_size=100`), 'back')).items
check('revert on the screen puts every student back and restores the login', (await batchVia(batchId)).status === 'reverted' && back.length === classStudents.length
  && (await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email('student'), password }) })).status === 200, `${back.length}`)
// Refused requests (403 for other roles, 409 for the refused revert) are expected; anything else is not.
const scriptErrors = [...b.errors, ...b.consoleErrors.filter(e => !/Failed to load resource/.test(e))]
check('no uncaught exceptions or console errors in the browser', scriptErrors.length === 0, js(scriptErrors.slice(0, 2)))

// Leave the school as it was: no progression for the demo class.
await must(admin('PUT', `/promotions/progression/${cls}`, { next_class_id: null }), 'clear progression')
await b.close()
r.save(join(shots, 'promotion-ui.json'))
