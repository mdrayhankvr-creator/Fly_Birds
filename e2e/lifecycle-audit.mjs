// Lifecycle audit fixes in a browser, on a running, demo-seeded disposable
// school (run it after suite.mjs and daily-tasks.mjs):
//   QA-08  a classmate is withdrawn: the later unpaid months are cancelled
//          (never deleted, paid months kept); the office sees it on the Dues
//          tab and the administrator reverts it with a reason;
//   QA-09  the administrator reverses a counter payment from its receipt
//          (refund, reason, confirmation): the dues are owed again, the
//          receipt says Reversed, the ledger nets to zero; a late fine set on
//          a structure is charged to a later overdue due and shown at the counter;
//   QA-12  Friday is a weekly off day in School settings; attendance on a
//          Friday and on a published holiday is refused with a clear message,
//          while attendance already recorded stays;
//   and the screens at 1440px and 375px, English and Bangla, light and dark,
//   with overflow, contrast and axe checks.
//
//   E2E_BASE_URL=http://localhost:8121 DEMO_PASSWORD=... node e2e/lifecycle-audit.mjs
//
// It records and reverses a payment, withdraws and readmits a student, sets
// and clears a late fine and adds and removes a holiday; never run it against
// a real school.
import { randomUUID } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('set DEMO_PASSWORD')
const shots = process.env.E2E_SHOTS ?? join(here, 'shots')
mkdirSync(shots, { recursive: true })
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const js = v => JSON.stringify(v)
const email = who => `demo.${who}@demo.flybirds.test`
const cents = v => Math.round(v * 100)

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: email(who), password }) })
  if (res.status !== 200) throw new Error(`sign-in ${who}: ${res.status}`)
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? js(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code, error: json.error }
  }
}
const must = async (promise, what) => { const res = await promise; if (res.status >= 300) throw new Error(`${what}: ${res.status} ${res.code}`); return res.data }
const [admin, accounts, student] = await Promise.all(['admin', 'accounts', 'student'].map(api))

// ---- 0. Data through the API ------------------------------------------------------
const me = await must(student('GET', '/students/me'), 'me')
const classmates = (await must(admin('GET', `/students?session_id=${me.session_id}&class_id=${me.class_id}&status=active&page=1&page_size=100`), 'class')).items.filter(s => s.id !== me.id)
const leaver = classmates[0]
const payer = classmates[1]
const session = (await must(admin('GET', '/academic/sessions?page=1&page_size=100'), 'sessions')).items.find(s => s.id === me.session_id)
const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Dhaka' }).format(new Date())
const monthAdd = (n) => { const [y, m] = today.split('-').map(Number); const d = new Date(Date.UTC(y, m - 1 + n, 1)); return d.toISOString().slice(0, 7) }
const tuition = (await must(accounts('GET', `/fees/dues?student_id=${leaver.id}&page=1&page_size=100`), 'dues')).items.find(d => d.period)
if (!tuition) throw new Error('the demo seed has no monthly due to copy')
const structureID = tuition.structure_id
// Two later months for the leaver, inside the session.
const later = [1, 2].map(monthAdd).filter(m => `${m}-01` <= session.end_date)
for (const period of later) {
  const res = await accounts('POST', '/fees/dues', { student_id: leaver.id, structure_id: structureID, period })
  if (![201, 409].includes(res.status)) throw new Error(`assign ${period}: ${res.status} ${res.code}`)
}

// ---- QA-08: withdrawing a student cancels the later months -------------------------
const full = await must(admin('GET', `/students/${leaver.id}`), 'student')
const patch = status => ({ admission_no: full.admission_no, name_en: full.name_en, name_bn: full.name_bn, date_of_birth: full.date_of_birth ?? '', status, session_id: full.session_id, class_id: full.class_id, section_id: full.section_id, roll_no: full.roll_no, user_id: full.user_id ?? '' })
await must(admin('PATCH', `/students/${leaver.id}`, patch('inactive')), 'withdraw')
const cancelled = await must(accounts('GET', `/fees/cancellations?student_id=${leaver.id}&status=open`), 'cancellations')
const laterDues = (await must(accounts('GET', `/fees/dues?student_id=${leaver.id}&page=1&page_size=100`), 'dues after')).items.filter(d => later.includes(d.period))
check('withdrawing a student cancels the unpaid months after the month they left, keeping every due', cancelled.items.length === 1 && cancelled.items[0].reason === 'withdrawn' && laterDues.length === later.length && laterDues.every(d => d.status === 'cancelled' && d.balance === 0),
  `${cancelled.items.length} cancellation, months ${cancelled.items[0]?.periods?.join(',')}, statuses ${laterDues.map(d => d.status).join(',')}`)
const thisMonth = (await must(accounts('GET', `/fees/dues?student_id=${leaver.id}&period=${today.slice(0, 7)}&page=1&page_size=10`), 'this month')).items
check('the month the student left is not cancelled', thisMonth.every(d => d.status !== 'cancelled'), thisMonth.map(d => d.status).join(',') || 'no due this month')

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
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && el.children.length === 0 && !el.closest('[disabled]') && !el.disabled).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
const set = (selector, value) => `(() => { const el = document.querySelector(${js(selector)}); if (!el) return false
  const proto = el.tagName === 'SELECT' ? HTMLSelectElement.prototype : el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, ${js(value)}); el.dispatchEvent(new Event(el.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })); return true })()`
const clickText = (text, scope = 'main') => `(() => { const el = [...document.querySelectorAll(${js(scope + ' button, ' + scope + ' summary, ' + scope + ' [role=tab]')})].find(e => e.offsetParent !== null && e.innerText.trim().startsWith(${js(text)})); if (!el || el.disabled) return false; el.click(); return true })()`
const clickExact = (text, scope = 'main') => `(() => { const el = [...document.querySelectorAll(${js(scope + ' button')})].find(e => e.offsetParent !== null && e.innerText.trim() === ${js(text)}); if (!el || el.disabled) return false; el.click(); return true })()`
const confirmYes = `(() => { const b = document.querySelectorAll('[role=alertdialog] button'); if (!b.length) return false; b[b.length - 1].click(); return true })()`
const mainText = `document.querySelector('main')?.innerText ?? ''`

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
    await b.shot(`audit-${name}-${label}`, !mobile)
  }
  check(`${name}: 1440px and 375px, English and Bangla, light and dark: no overflow, contrast and axe clean`, bad.length === 0, bad.slice(0, 4).join(' || ') || '8 of 8')
}

const b = await launch({ base, shots })
const errors = []
await b.evaluate(`window.addEventListener('error', e => (window.__errors = [...(window.__errors || []), String(e.message)]))`)

// The Dues tab lists the cancellation; the accountant sees it but cannot revert.
const duesTab = `(() => { const t = [...document.querySelectorAll('main [role=tab]')].find(e => /Dues|বকেয়া/.test(e.innerText)); if (!t) return false; t.click(); return true })()`
await b.viewport(1440, 900)
await open(b, 'accounts', '/accounts/fees')
await b.waitFor(`!!document.querySelector('main [role=tab]')`, 20000)
await b.evaluate(duesTab)
await b.waitFor(`!!document.querySelector('#fee-cancellations')`, 15000)
await b.waitFor(`(document.querySelector('section[aria-labelledby=fee-cancellations]')?.innerText ?? '').includes(${js(leaver.admission_no)})`, 15000)
const accountantView = await b.evaluate(`document.querySelector('section[aria-labelledby=fee-cancellations]').innerText`)
check('the office sees the cancelled months with the reason and leaving date, and an accountant cannot revert', accountantView.includes('Withdrawn') && accountantView.includes('Left on') && !/\bRevert\b/.test(accountantView.replace('Reverted', '')), accountantView.slice(0, 160).replace(/\n/g, ' · '))

// The administrator reverts it with a reason, through the confirmation.
await open(b, 'admin', '/accounts/fees')
await b.waitFor(`!!document.querySelector('main [role=tab]')`, 20000)
await b.evaluate(duesTab)
await b.waitFor(`(document.querySelector('section[aria-labelledby=fee-cancellations]')?.innerText ?? '').includes(${js(leaver.admission_no)})`, 15000)
await b.evaluate(clickExact('Revert', 'section[aria-labelledby=fee-cancellations] li'))
await b.waitFor(`!!document.querySelector('section[aria-labelledby=fee-cancellations] form input')`, 5000)
await b.evaluate(set('section[aria-labelledby=fee-cancellations] form input', 'Withdrawal recorded by mistake'))
await b.evaluate(clickExact('Revert', 'section[aria-labelledby=fee-cancellations] form'))
await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
await b.evaluate(confirmYes)
await b.waitFor(`(document.querySelector('section[aria-labelledby=fee-cancellations]')?.innerText ?? '').includes('owed again')`, 10000)
const owedAgain = (await must(accounts('GET', `/fees/dues?student_id=${leaver.id}&page=1&page_size=100`), 'dues reverted')).items.filter(d => later.includes(d.period))
const revertedList = await must(accounts('GET', `/fees/cancellations?student_id=${leaver.id}&status=reverted`), 'reverted')
check('the administrator reverts the cancellation with a reason: the months are owed again and the record stays', owedAgain.every(d => d.status === 'unpaid' && d.balance > 0) && revertedList.items[0]?.revert_reason === 'Withdrawal recorded by mistake',
  `${owedAgain.map(d => d.status).join(',')} · ${revertedList.items[0]?.revert_reason}`)
await must(admin('PATCH', `/students/${leaver.id}`, patch('active')), 'readmit')

// ---- QA-09: reversing a counter payment, and a late fine -----------------------------
const balance = async id => cents((await must(accounts('GET', `/fees/students/${id}/statement`), 'statement')).totals.balance)
let before = await balance(payer.id)
if (before === 0) {
  for (const period of [3, 4, 5].map(monthAdd)) { const res = await accounts('POST', '/fees/dues', { student_id: payer.id, structure_id: structureID, period }); if (res.status === 201) break }
  before = await balance(payer.id)
}
const paid = await must(accounts('POST', '/fees/payments', { student_id: payer.id, amount: Math.min(250, before / 100), method: 'cash', reference: '', note: 'lifecycle audit', paid_on: today, idempotency_key: randomUUID(), fee_ids: [] }), 'payment')
const receiptNo = paid.payment.receipt_no
await open(b, 'admin', '/accounts/fees')
await b.waitFor(`!!document.querySelector('#counter-search')`, 20000)
await b.evaluate(set('#counter-search', payer.admission_no))
await b.waitFor(`!!document.querySelector('#counter-results li')`, 10000)
await b.evaluate(`document.querySelector('#counter-results li').click()`)
await b.waitFor(`[...document.querySelectorAll('main td')].some(td => td.innerText.includes(${js(receiptNo)}))`, 15000)
await b.evaluate(`(() => { const row = [...document.querySelectorAll('main tr')].find(tr => tr.innerText.includes(${js(receiptNo)})); row.querySelector('button').click() })()`)
await b.waitFor(`!!document.querySelector('.fee-receipt')`, 10000)
const panelShown = await b.evaluate(clickText('Reverse this payment'))
await b.waitFor(`!!document.querySelector('main details[open] input[name=reversal-kind]')`, 5000)
await b.evaluate(`[...document.querySelectorAll('main input[name=reversal-kind]')][1].click()`)
await b.evaluate(set('main details[open] input[maxlength="240"]', 'Paid twice; cash returned'))
await sleep(200)
await b.evaluate(clickText('Reverse payment', 'main details[open]'))
await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000)
await b.evaluate(confirmYes)
await b.waitFor(`(document.querySelector('.fee-receipt')?.innerText ?? '').includes('Paid twice')`, 10000)
const receiptText = await b.evaluate(`document.querySelector('.fee-receipt').innerText`)
await b.shot('audit-receipt-reversed')
const reversedPayment = (await must(accounts('GET', `/fees/payments?student_id=${payer.id}&status=reversed&page=1&page_size=10`), 'reversed')).items.find(p => p.receipt_no === receiptNo)
const ledger = (await must(admin('GET', `/accounts/entries?type=income&category=fees&from=${today}&to=${today}&search=${encodeURIComponent(receiptNo)}&page=1&page_size=10`), 'ledger')).items
check('the administrator reverses a counter payment from its receipt as a refund, with a reason', panelShown && /REVERSED/i.test(receiptText) && reversedPayment?.reversal_kind === 'refund' && reversedPayment?.reversal_reason === 'Paid twice; cash returned', `${receiptNo} ${reversedPayment?.reversal_kind}`)
check('after the reversal the dues are owed again and the ledger nets to zero for the receipt', (await balance(payer.id)) === before && ledger.length === 2 && ledger.reduce((n, e) => n + cents(e.amount), 0) === 0, `${ledger.map(e => e.amount).join(' + ')}`)
const twice = await admin('POST', `/fees/payments/${paid.payment.id}/reverse`, { kind: 'error', reason: 'again' })
const byAccountant = await accounts('POST', `/fees/payments/${paid.payment.id}/reverse`, { kind: 'error', reason: 'again' })
check('a payment is reversed once, and only by an administrator', twice.status === 409 && byAccountant.status === 403, `${twice.status} ${byAccountant.status}`)

// A late fine on the structure applies to dues created afterwards only.
const structure = await must(accounts('GET', `/fees/structures?session_id=${me.session_id}&page=1&page_size=100`), 'structures').then(p => p.items.find(s => s.id === structureID))
const restore = { session_id: structure.session_id, class_id: structure.class_id, category_id: structure.category_id, amount: structure.amount, due_day: structure.due_day, is_active: structure.is_active }
await open(b, 'admin', '/accounts/fees')
await b.waitFor(`!!document.querySelector('main [role=tab]')`, 20000)
await b.evaluate(`[...document.querySelectorAll('main [role=tab]')].find(e => /Fee setup/.test(e.innerText)).click()`)
await b.waitFor(`[...document.querySelectorAll('main tr')].some(tr => tr.innerText.includes(${js(structure.class_name)}) && tr.innerText.includes(${js(structure.category_name_en)}))`, 15000)
await b.evaluate(`(() => { const row = [...document.querySelectorAll('main tr')].find(tr => tr.innerText.includes(${js(structure.class_name)}) && tr.innerText.includes(${js(structure.category_name_en)})); [...row.querySelectorAll('button')].find(x => x.innerText.trim() === 'Edit').click() })()`)
await sleep(300)
await b.evaluate(set('input[aria-describedby=late-fine-hint]', '50'))
await b.evaluate(clickText('Update'))
await b.waitFor(`[...document.querySelectorAll('main tr')].some(tr => tr.innerText.includes(${js(structure.class_name)}) && tr.innerText.includes(${js(structure.category_name_en)}) && /50\\.00/.test(tr.innerText))`, 10000)
const savedFine = (await must(accounts('GET', `/fees/structures?session_id=${me.session_id}&page=1&page_size=100`), 'structures')).items.find(s => s.id === structureID)
check('the late fine is set on the fee structure in Fee setup', savedFine.late_fine === 50, `late_fine ${savedFine.late_fine}`)
const olderDue = (await must(accounts('GET', `/fees/dues?student_id=${payer.id}&page=1&page_size=100`), 'old dues')).items.find(d => d.balance > 0 && d.due_date < today)
// A new due already past its date (the first month of the session the payer has none for).
let finedPeriod = ''
for (let m = Number(session.start_date.slice(5, 7)); m <= 12 && !finedPeriod; m++) {
  const period = `${session.start_date.slice(0, 4)}-${String(m).padStart(2, '0')}`
  if (`${period}-28` >= today) break
  const res = await accounts('POST', '/fees/dues', { student_id: payer.id, structure_id: structureID, period })
  if (res.status === 201) finedPeriod = period
}
const statement = await must(accounts('GET', `/fees/students/${payer.id}/statement`), 'statement with fine')
const fined = statement.dues.find(d => d.period === finedPeriod)
check('an overdue due created after the fine is charged it once; earlier dues are not', !!finedPeriod && fined?.fine_amount === 50 && statement.totals.fines >= 50 && (!olderDue || statement.dues.find(d => d.id === olderDue.id)?.fine_amount === 0), `${finedPeriod}: fine ${fined?.fine_amount}; totals ${statement.totals.fines}`)
await open(b, 'admin', '/accounts/fees')
await b.waitFor(`!!document.querySelector('#counter-search')`, 20000)
await b.evaluate(set('#counter-search', payer.admission_no))
await b.waitFor(`!!document.querySelector('#counter-results li')`, 10000)
await b.evaluate(`document.querySelector('#counter-results li').click()`)
await b.waitFor(`(document.querySelector('section[aria-labelledby=counter-dues]')?.innerText ?? '').includes('late fine')`, 15000)
check('the counter shows the late fine in the due', (await b.evaluate(`document.querySelector('section[aria-labelledby=counter-dues]').innerText`)).includes('includes a late fine of'), 'fine note shown')
await must(admin('PATCH', `/fees/structures/${structureID}`, { ...restore, late_fine: 0 }), 'fine off')

// ---- QA-12: weekly off days and holidays --------------------------------------------
const settings = await must(admin('GET', '/school/settings'), 'settings')
check('Friday is the weekly off day by default', js(settings.weekly_off_days) === '[5]', js(settings.weekly_off_days))
const lastWeekday = (wd, maxBack = 14) => { for (let i = 1; i <= maxBack; i++) { const d = new Date(`${today}T00:00:00Z`); d.setUTCDate(d.getUTCDate() - i); if (d.getUTCDay() === wd) return d.toISOString().slice(0, 10) } return '' }
const friday = lastWeekday(5)
const sections = await must(admin('GET', `/attendance/students/sections?session_id=${me.session_id}`), 'sections')
const section = sections.find(s => s.id === me.section_id) ?? sections[0]
const roster = await must(admin('GET', `/attendance/students/roster?session_id=${me.session_id}&section_id=${section.id}&date=${friday}&page=1&page_size=100`), 'friday roster')
const fresh = roster.items.find(i => !i.record_id)
const refused = fresh ? await admin('PUT', '/attendance/students/roster', { session_id: me.session_id, section_id: section.id, attendance_date: friday, entries: [{ student_id: fresh.student_id, status: 'present', note: '' }] }) : { status: 0 }
check('new attendance on a Friday is refused with a clear message', refused.status === 409 && refused.code === 'attendance_day_off' && /weekly off day/.test(refused.error?.message ?? ''), `${refused.status} ${refused.error?.message ?? ''}`)
const recorded = roster.items.filter(i => i.record_id).length
const after = await must(admin('GET', `/attendance/students/roster?session_id=${me.session_id}&section_id=${section.id}&date=${friday}&page=1&page_size=100`), 'friday roster after')
check('attendance already recorded on that Friday is untouched', after.items.filter(i => i.record_id).length === recorded, `${recorded} records before and after`)

// A published holiday on the last working day before today.
let holidayDate = ''
for (let i = 1; i <= 10 && !holidayDate; i++) { const d = new Date(`${today}T00:00:00Z`); d.setUTCDate(d.getUTCDate() - i); if (d.getUTCDay() !== 5 && d.toISOString().slice(0, 10) >= session.start_date) holidayDate = d.toISOString().slice(0, 10) }
const holiday = await must(admin('POST', '/events', { title_en: 'Audit holiday', title_bn: 'নিরীক্ষা ছুটি', description_en: '', description_bn: '', kind: 'holiday', starts_on: holidayDate, ends_on: holidayDate, start_time: null, end_time: null, location: '', audience_roles: ['teacher', 'student', 'guardian'], targets: [] }), 'holiday')
await must(admin('POST', `/events/${holiday.id}/publish`), 'publish holiday')
const dayInfo = await must(admin('GET', `/attendance/day?date=${holidayDate}`), 'day')
check('a published holiday closes its day', dayInfo.open === false && dayInfo.reason === 'holiday' && dayInfo.holiday_title_en === 'Audit holiday', js(dayInfo))

// The attendance screen shows the reason and takes no new marks that day.
const pickDate = date => async b => { await b.evaluate(set('#attn-date', date)); return b.waitFor(`(${mainText}).includes('No attendance on') || (${mainText}).includes('হাজিরা নেওয়া যাবে না')`, 10000) }
await b.viewport(1440, 900)
await open(b, 'admin', '/admin/attendance/students')
await b.waitFor(`!!document.querySelector('#attn-date')`, 20000)
await b.evaluate(`(() => { const s = [...document.querySelectorAll('main select')][1]; if (!s) return; Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, ${js(section.id)}); s.dispatchEvent(new Event('change', { bubbles: true })) })()`)
await sleep(600)
await pickDate(friday)(b)
const fridayText = await b.evaluate(mainText)
const markAll = await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.includes('Mark all present'))?.disabled ?? null`)
check('the attendance screen says a Friday is a weekly off day and offers no new marks', fridayText.includes('Friday is a weekly off day') && fridayText.includes('can still be corrected') && markAll !== false, `mark all disabled: ${markAll}`)
await pickDate(holidayDate)(b)
check('the attendance screen names the holiday', (await b.evaluate(mainText)).includes('Audit holiday (holiday)'), holidayDate)

// School settings: the weekly off days can be changed by the administrator.
await open(b, 'admin', '/admin/school-settings')
await b.waitFor(`[...document.querySelectorAll('main legend')].some(l => l.innerText.includes('Weekly off days'))`, 20000)
const fridayBox = await b.evaluate(`(() => { const l = [...document.querySelectorAll('main fieldset label')].find(x => x.innerText.trim() === 'Friday'); return l?.querySelector('input').checked })()`)
check('School settings shows Friday ticked as the weekly off day', fridayBox === true, String(fridayBox))

// ---- The screens: sizes, languages, themes ------------------------------------------
await must(admin('PATCH', `/students/${leaver.id}`, patch('inactive')), 'withdraw again')
await matrix(b, 'fees-dues-cancellations', 'admin', '/accounts/fees', `!!document.querySelector('main [role=tab]')`, async b => {
  await b.evaluate(duesTab)
  return b.waitFor(`(document.querySelector('section[aria-labelledby=fee-cancellations]')?.innerText ?? '').includes(${js(leaver.admission_no)})`, 15000)
})
await matrix(b, 'attendance-off-day', 'admin', '/admin/attendance/students', `!!document.querySelector('#attn-date')`, pickDate(friday))
await matrix(b, 'school-settings-off-days', 'admin', '/admin/school-settings', `[...document.querySelectorAll('main legend')].length > 0`, async b => {
  await b.evaluate(`[...document.querySelectorAll('main fieldset')].find(f => f.querySelector('legend')?.innerText.match(/Weekly off days|সাপ্তাহিক/))?.scrollIntoView()`)
  return true
})
await matrix(b, 'fee-setup-late-fine', 'admin', '/accounts/fees', `!!document.querySelector('main [role=tab]')`, async b => {
  await b.evaluate(`[...document.querySelectorAll('main [role=tab]')].find(e => /Fee setup|ফি নির্ধারণ|ফি সেটআপ/.test(e.innerText))?.click()`)
  return b.waitFor(`!!document.querySelector('#late-fine-hint')`, 15000)
})

// Clean up what the screens needed.
await must(admin('PATCH', `/students/${leaver.id}`, patch('active')), 'readmit again')
const open2 = await must(admin('GET', `/fees/cancellations?student_id=${leaver.id}&status=open`), 'open')
for (const c of open2.items) await must(admin('POST', `/fees/cancellations/${c.id}/revert`, { reason: 'Test withdrawal undone' }), 'revert')
// A published event is cancelled, not deleted; a cancelled holiday closes no day.
await must(admin('POST', `/events/${holiday.id}/cancel`), 'cancel holiday')
check('a cancelled holiday no longer closes its day', (await must(admin('GET', `/attendance/day?date=${holidayDate}`), 'day after')).open === true, holidayDate)

const pageErrors = await b.evaluate(`window.__errors ?? []`)
check('no uncaught exceptions in the browser', pageErrors.length === 0 && errors.length === 0, js(pageErrors))
b.close()
r.save(join(shots, 'lifecycle-audit.json'))
