// Sign-in by phone number: one field takes an email, a Bangladeshi mobile
// number in any written form, or a Student ID; a guardian's number opens
// whichever child's password was given; and the account menu switches
// between the children of that family.
//
//   DEMO_PASSWORD=...  E2E_BASE_URL=http://localhost:8121  node e2e/phone-login.mjs
//   E2E_SHOTS=<folder> keeps the two sign-in screenshots (default e2e/shots/phone-login)
import { launch, reporter, sleep } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const shots = process.env.E2E_SHOTS || join(here, 'shots', 'phone-login')
mkdirSync(shots, { recursive: true })
const pw = process.env.DEMO_PASSWORD
const r = reporter(), check = r.check
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173', shots })
const email = who => `demo.${who}@demo.flybirds.test`

// The family's number and the passwords the two children end up with.
const stamp = String(Date.now()).slice(-8)
const phone = `+88017${stamp}`
const local = `0${phone.slice(4)}`
const passwords = ['ElderChildPassword1!', 'YoungerChildPassword1!']

const must = async (promise, what) => {
  const res = await promise
  if (res.status >= 300) throw new Error(`${what}: ${res.status} ${JSON.stringify(res.body).slice(0, 200)}`)
  return res.body.data
}
const signIn = (identifier, password) => b.evaluate(`(async () => {
  const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || '')
  if (csrf) await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }).catch(() => null)
  const res = await fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ identifier: ${JSON.stringify(identifier)}, password: ${JSON.stringify(password)} }) })
  const body = await res.json().catch(() => ({}))
  return { status: res.status, code: body.error && body.error.code, user: body.data && body.data.user && { id: body.data.user.id, name: body.data.user.display_name } }
})()`)

// ---------------------------------------------------------------------------
// Two children of one family, each with their own password and the one number.

await b.login(email('admin'), pw)
const roster = await must(b.api('/students?page=1&page_size=100&status=active'), 'students')
const chosen = roster.items.filter(s => !s.user_id).slice(0, 2)
check('two students without a login are available', chosen.length === 2, `${chosen.length} found`)

// The administrator creates both logins first; each child then sets their
// own password, which signs that child in and ends the admin session.
const children = []
for (const student of chosen) {
  const login = await must(b.api(`/students/${student.id}/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }), 'create login')
  children.push({ ...student, userID: login.user_id, loginID: login.login_id, temporary: login.initial_password, name: student.name_en })
}
for (const [i, child] of children.entries()) {
  const first = await signIn(child.loginID, child.temporary)
  check(`child ${i + 1} signs in with the Student ID`, first.status === 200, `status ${first.status}`)
  const changed = await b.api('/auth/password', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ current_password: child.temporary, new_password: passwords[i] }) })
  check(`child ${i + 1} sets their own password`, changed.status === 200, `status ${changed.status}`)
}

// The administrator puts the guardian's number on both accounts.
await b.login(email('admin'), pw)
for (const child of children) {
  const saved = await must(b.api(`/users/${child.userID}/login-phone`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login_phone: local }) }), 'set login phone')
  check(`the number is stored canonically for ${child.name}`, saved.login_phone === phone, saved.login_phone)
}
const written = await b.api(`/users/${children[0].userID}/login-phone`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login_phone: '0171100' }) })
check('a number that is not a Bangladeshi mobile is refused', written.status === 400, `status ${written.status}`)

// ---------------------------------------------------------------------------
// Signing in each way.

const byEmail = await signIn(email('teacher'), pw)
check('a teacher still signs in by email', byEmail.status === 200, `status ${byEmail.status}`)

for (const form of [local, `880${phone.slice(4)}`, phone, `${local.slice(0, 5)}-${local.slice(5)}`]) {
  const res = await signIn(form, passwords[0])
  check(`the number written as ${form} opens the first child`, res.status === 200 && res.user.id === children[0].userID, `status ${res.status}`)
}
const second = await signIn(local, passwords[1])
check('the same number with the second password opens the second child', second.status === 200 && second.user.id === children[1].userID, `status ${second.status}`)

const wrong = await signIn(local, 'NotTheRightPassword1!')
check('a wrong password on the number is refused', wrong.status === 401 && wrong.code === 'invalid_credentials', `status ${wrong.status} ${wrong.code}`)
const unknown = await signIn('01999000000', passwords[0])
check('an unknown number answers exactly the same way', unknown.status === wrong.status && unknown.code === wrong.code, `${unknown.status} ${unknown.code}`)

const byID = await signIn(children[0].loginID, passwords[0])
check('the Student ID still works', byID.status === 200, `status ${byID.status}`)

// ---------------------------------------------------------------------------
// The child switch.

const siblings = await must(b.api('/auth/siblings'), 'siblings')
check('both children are listed on the number', siblings.length === 2 && siblings.filter(s => s.current).length === 1, JSON.stringify(siblings.map(s => s.display_name)))
const moved = await b.api('/auth/switch', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ user_id: children[1].userID }) })
check('switching to the sibling needs no password', moved.status === 200 && moved.body.data.user.id === children[1].userID, `status ${moved.status}`)

// A child of another family is not on this number, so it cannot be opened.
const strangerLogin = await signIn(email('student'), pw)
check('the demo student signs in by email', strangerLogin.status === 200, `status ${strangerLogin.status}`)
const strangerID = strangerLogin.user.id
await signIn(local, passwords[0])
const refused = await b.api('/auth/switch', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ user_id: strangerID }) })
check('switching outside the family is refused', refused.status === 403, `status ${refused.status}`)
const noSwitch = await signIn(email('student'), pw)
check('a child with no shared number signs in normally', noSwitch.status === 200, `status ${noSwitch.status}`)
const alone = await must(b.api('/auth/siblings'), 'siblings for an only child')
check('an only child is offered no switch', alone.length === 0, JSON.stringify(alone))

// The menu itself, in the browser.
await signIn(local, passwords[0])
await b.go('/')
await b.waitFor(`document.querySelector('header button[aria-haspopup="menu"]')`, 15000)
await b.evaluate(`document.querySelector('header button[aria-haspopup="menu"]').click()`)
await sleep(400)
const menu = await b.evaluate(`(() => { const m = document.querySelector('[role="menu"]'); return m ? m.innerText : '' })()`)
check("the account menu offers the family's children", children.every(c => menu.includes(c.name)), menu.split(String.fromCharCode(10)).join(' | ').slice(0, 160))

// ---------------------------------------------------------------------------
// The sign-in page, in both languages at phone width.

// Signed out, so the form itself is on screen.
await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); if (csrf) await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }).catch(() => null); return true })()`)
for (const lang of ['en', 'bn']) {
  await b.evaluate(`localStorage.setItem('flybirds.language', '${lang}')`)
  await b.viewport(375, 760, true)
  await b.go('/login')
  await b.waitFor(`document.querySelector('#login-email')`, 15000)
  const label = await b.evaluate(`document.querySelector('label[for="login-email"]').innerText.trim()`)
  check(`the ${lang} field names email, phone and Student ID`, lang === 'en' ? label === 'Email, phone or Student ID' : label.includes('ফোন'), label)
  await b.shot(`sign-in-${lang}-375`)
}
await b.viewport(1440, 900)

const issues = await b.evaluate(`window.__consoleErrors ? window.__consoleErrors.length : 0`)
check('no console errors on the sign-in page', !issues, String(issues))

r.save(join(shots, 'phone-login-checks.json'))
await b.close()
