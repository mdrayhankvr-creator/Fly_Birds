// Isolated account-link and avatar check on the disposable fbqa-cp1b demo stack.
import { launch, reporter } from './cdp.mjs'
import { randomBytes } from 'node:crypto'

const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8092'
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('DEMO_PASSWORD is required')
const teacherPassword = `${password}T`
const r = reporter(), check = r.check
const b = await launch({ base, shots: process.env.E2E_SHOTS_DIR || 'e2e/shots/staff-account-link' })
const suffix = randomBytes(4).toString('hex')
const email = `link-${suffix}@example.test`
const code = `LINK${suffix.toUpperCase()}`
const adminEmail = 'demo.admin@demo.flybirds.test'

async function adminToken() {
  const response = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: adminEmail, password }) })
  if (!response.ok) throw new Error(`admin fixture login: ${response.status}`)
  return (await response.json()).data.access_token
}
let token
async function api(method, path, body, contentType = 'application/json') {
  const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': contentType } : {}) }, body: body ? Buffer.isBuffer(body) ? body : JSON.stringify(body) : undefined })
  const result = (response.headers.get('content-type') || '').includes('json') ? await response.json() : null
  if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${result?.error?.code || ''}`)
  return result?.data
}
const fill = (selector, value) => b.evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return false; el.focus(); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)
const click = selector => b.evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return false; el.click(); return true })()`)
const portrait = async color => Buffer.from(await b.evaluate(`(() => { const c = document.createElement('canvas'); c.width = c.height = 128; const x = c.getContext('2d'); x.fillStyle = ${JSON.stringify(color)}; x.fillRect(0,0,128,128); return c.toDataURL('image/png').split(',')[1] })()`), 'base64')

try {
  token = await adminToken()
  await api('POST', '/users', { email, display_name: `Account ${suffix}`, role: 'teacher', password })
  // New administrator-created accounts must replace their initial password
  // before they may open the dashboard.
  const firstLogin = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) })
  if (!firstLogin.ok) throw new Error(`teacher first login: ${firstLogin.status}`)
  const teacherToken = (await firstLogin.json()).data.access_token
  const changed = await fetch(`${base}/api/auth/password`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${teacherToken}` }, body: JSON.stringify({ current_password: password, new_password: teacherPassword }) })
  if (!changed.ok) throw new Error(`teacher first password change: ${changed.status}`)
  const employee = await api('POST', '/staff/employees', { employee_code: code, user_id: null, kind: 'teacher', name_en: `Photo Teacher ${suffix}`, name_bn: '', designation: 'Teacher', phone: '', joined_on: '2026-01-01', left_on: null, is_active: true })
  check('unlinked teacher profile created', !!employee?.id && !employee.user_id)
  await api('PUT', `/staff/employees/${employee.id}/photo`, await portrait('#2786ba'), 'image/png')
  await b.viewport(360, 780, true)
  await b.login(adminEmail, password)
  await b.go(`/staff/profiles/${employee.id}`)
  check('admin sees unlinked account status', await b.waitFor(`!!document.querySelector('#staff-account-link-title') && document.querySelector('[aria-labelledby="staff-account-link-title"]')?.innerText.includes('No login account linked yet')`, 15000))
  check('account search field is available', await fill('[aria-labelledby="staff-account-link-title"] input[type=email]', email))
  await click('[aria-labelledby="staff-account-link-title"] button[type=submit]')
  check('exact matching account offered', await b.waitFor(`document.querySelector('[aria-labelledby="staff-account-link-title"]')?.innerText.includes(${JSON.stringify(email)}) && !!document.querySelector('[aria-labelledby="staff-account-link-title"] input[maxlength="32"]')`, 15000))
  await fill('[aria-labelledby="staff-account-link-title"] input[maxlength="32"]', code)
  await click('[aria-labelledby="staff-account-link-title"] button[type=button]')
  check('confirmation names both records', await b.waitFor(`document.querySelector('[role=alertdialog]')?.innerText.includes(${JSON.stringify(code)}) && document.querySelector('[role=alertdialog]')?.innerText.includes(${JSON.stringify(email)})`, 5000))
  await click('[role=alertdialog] button:last-child')
  check('linked status appears on profile', await b.waitFor(`document.querySelector('[aria-labelledby="staff-account-link-title"]')?.innerText.includes('Linked account')`, 15000))
  check('admin profile has no sideways scroll at 360px', !(await b.overflow()))
  await b.login(email, teacherPassword)
  await b.go('/')
  check('teacher header and dashboard show stored photo', await b.waitFor(`!!document.querySelector('header .shell-avatar img') && !!document.querySelector('.ds-role-head img')`, 15000))
  const before = await b.evaluate(`document.querySelector('header .shell-avatar img')?.src`)
  await api('PUT', `/staff/employees/${employee.id}/photo`, await portrait('#b13b58'), 'image/png')
  check('new photo reaches open dashboard without reload', await b.waitFor(`!!document.querySelector('header .shell-avatar img') && document.querySelector('header .shell-avatar img').src !== ${JSON.stringify(before)}`, 30000))
  await b.go(`/staff/profiles/${employee.id}`)
  check('staff profile banner shows the photo', await b.waitFor(`!!document.querySelector('.pr-hero img')`, 15000))
  check('linked staff cannot see admin-only account card', !(await b.evaluate(`!!document.querySelector('#staff-account-link-title')`)))
} catch (error) { check('script error', false, error.stack) }
finally {
  check('no browser JavaScript errors', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  r.save(process.env.E2E_RESULTS || 'e2e/shots/staff-account-link/results.json')
  await b.close()
}
