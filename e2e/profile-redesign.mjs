// Run on a fresh fbqa-cp1b demo seed. Captures both profiles at phone and desktop widths.
import { launch, reporter, sleep } from './cdp.mjs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mkdirSync, writeFileSync } from 'node:fs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8092'
const shots = process.env.E2E_SHOTS_DIR || join(here, 'shots', 'profile-redesign')
const pw = process.env.DEMO_PASSWORD
if (!pw) throw new Error('DEMO_PASSWORD is required')
mkdirSync(shots, { recursive: true })
const r = reporter(), check = r.check
const b = await launch({ base, shots })
const imageList = ['# Profile redesign screenshots', '', 'Fresh fbqa-cp1b demo school. Full-page captures.', '']
const email = who => `demo.${who}@demo.flybirds.test`
const roleTokens = new Map()
async function as(role, method, path, body, contentType = 'application/json') {
  if (!roleTokens.has(role)) {
    const login = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: email(role), password: pw }) })
    if (!login.ok) throw new Error(`API sign-in for ${role}: ${login.status}`)
    roleTokens.set(role, (await login.json()).data.access_token)
  }
  const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${roleTokens.get(role)}`, ...(body ? { 'Content-Type': contentType } : {}) }, body: body ? Buffer.isBuffer(body) ? body : JSON.stringify(body) : undefined })
  return { status: response.status, data: (await response.json().catch(() => ({}))).data }
}
const visibleTabs = () => b.evaluate(`[...document.querySelectorAll('main [role=tab]')].map(e => e.innerText.trim())`)
const choose = id => b.evaluate(`document.querySelector('main [role=tab][id$="-${id}"]')?.click()`)
const setLanguage = async lang => { await b.evaluate(`localStorage.setItem('flybirds.language', ${JSON.stringify(lang)})`); await b.go(locationPath) }
let locationPath = ''
async function open(path) { locationPath = path; await b.go(path); return b.waitFor(`!!document.querySelector('main .pr-hero h1')`, 15000) }
async function shot(kind, lang, width) {
  await b.viewport(width, width === 375 ? 812 : 900, true)
  await setLanguage(lang)
  await b.waitFor(`!!document.querySelector('main .pr-hero h1') && !!document.querySelector('main .pr-cover')`, 15000)
  await sleep(350)
  const label = `${kind}-${lang}-${width}`
  check(`${label}: no horizontal page scroll`, !(await b.overflow()))
  check(`${label}: hero, stats and tabs render`, await b.evaluate(`!!document.querySelector('main .pr-hero h1') && !!document.querySelector('main .pr-stats') && !!document.querySelector('main [role=tablist]')`))
  if (width === 1440) check(`${label}: name clears the cover edge`, await b.evaluate(`document.querySelector('.pr-hero h1').getBoundingClientRect().top >= document.querySelector('.pr-cover').getBoundingClientRect().bottom - 2`))
  await b.shot(label, true)
  imageList.push(`- [${label}](${label}.png)`)
}

try {
  await b.viewport(1440, 900)
  await b.login(email('admin'), pw)
  const student = (await b.api('/students?page=1&page_size=1&search=DEMO-001')).body.data.items[0]
  check('demo student exists', !!student?.id)
  await open(`/admin/students/${student.id}`)
  check('student identity includes both names and placement', await b.evaluate(`(() => { const t = document.querySelector('.pr-hero').innerText; return t.includes(${JSON.stringify(student.name_en)}) && t.includes(${JSON.stringify(student.name_bn)}) && t.includes(${JSON.stringify(student.class_name)}) && t.includes(${JSON.stringify(student.section_name)}) })()`))
  const studentTabs = await visibleTabs()
  check('student has five profile tabs', ['Overview', 'Academics', 'Attendance', 'Fees', 'Documents'].every(x => studentTabs.includes(x)))
  await choose('academics')
  check('academic subjects and results render', await b.waitFor(`!!document.querySelector('#student-profile-panel-academics') && !!document.querySelector('.pr-subject')`, 15000))
  await choose('attendance')
  check('month heatmap renders', await b.waitFor(`document.querySelectorAll('[data-attendance-day]').length >= 28`, 15000))
  await choose('overview')
  for (const lang of ['en', 'bn']) for (const width of [1440, 375]) await shot('student', lang, width)

  await b.login(email('teacher'), pw)
  const staff = (await b.api('/staff/me/profile')).body.data
  await open(`/staff/profiles/${staff.id}`)
  check('staff header has designation and employee ID', await b.evaluate(`(() => { const t = document.querySelector('.pr-hero').innerText; return t.includes(${JSON.stringify(staff.designation)}) && t.includes(${JSON.stringify(staff.employee_code)}) })()`))
  const staffTabs = await visibleTabs()
  check('staff has four profile tabs', ['Overview', 'Classes & routine', 'Attendance', 'Documents'].every(x => staffTabs.includes(x)))
  await choose('classes')
  check('assigned classes and today timetable render', await b.waitFor(`!!document.querySelector('#staff-assignments') && !!document.querySelector('#staff-today')`, 15000))
  await choose('overview')
  for (const lang of ['en', 'bn']) for (const width of [1440, 375]) await shot('teacher', lang, width)

  await b.viewport(360, 800, true)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'en'); localStorage.setItem('flybirds.theme', 'dark')`)
  await open(`/staff/profiles/${staff.id}`)
  check('dark profile at 360px has no horizontal scroll', !(await b.overflow()))
  await b.go(`/admin/students/${student.id}`)
  check('assigned teacher sees the student without a Fees tab or ID card', await b.waitFor(`!!document.querySelector('main .pr-hero')`) && !(await visibleTabs()).includes('Fees') && !(await visibleTabs()).includes('Documents'))
  check('teacher receives no student login or birth date', await (async () => { const body = (await b.api(`/students/${student.id}`)).body.data; return !body.login_id && !body.login_phone && !body.date_of_birth && !body.user_id })())
  await b.login(email('head'), pw)
  await open(`/admin/students/${student.id}`)
  const headTabs = await visibleTabs()
  check('head can review student fees and documents', headTabs.includes('Fees') && headTabs.includes('Documents'))
  check('head fee statement is available', (await b.api(`/fees/students/${student.id}/statement`)).status === 200)

  // An approval in another session updates the open student's shell and
  // greeting through the photo version; no page reload or navigation occurs.
  await b.login(email('student'), pw)
  const own = (await b.api('/students/me')).body.data
  await b.go('/')
  check('student dashboard loads', await b.waitFor(`!!document.querySelector('.ds-role-head')`, 15000))
  const before = (await as('student', 'GET', '/students/me')).data.photo_version
  const portrait = Buffer.from(await b.evaluate(`(() => { const c = document.createElement('canvas'); c.width = c.height = 800; const x = c.getContext('2d'); x.fillStyle = '#dbeafe'; x.fillRect(0,0,800,800); x.fillStyle = '#334155'; x.beginPath(); x.arc(400,330,170,0,Math.PI*2); x.fill(); return c.toDataURL('image/jpeg', .85).split(',')[1] })()`), 'base64')
  const submitted = await as('student', 'PUT', '/students/me/photo-submission', portrait, 'image/jpeg')
  check('student photo awaits approval', submitted.status === 200 && submitted.data?.status === 'pending')
  const approved = await as('admin', 'POST', `/students/${own.id}/photo-submission/approve`)
  check('administrator approves the photo', approved.status === 200)
  const updated = (await as('student', 'GET', '/students/me')).data
  check('approved student photo has a new version', !!updated.photo_version && updated.photo_version !== before)
  check('open header and greeting update without reload', await b.waitFor(`!!document.querySelector('header .shell-avatar img') && !!document.querySelector('.ds-role-head img')`, 30000))
  for (const lang of ['en', 'bn']) {
    await b.evaluate(`localStorage.setItem('flybirds.language', ${JSON.stringify(lang)})`)
    await b.go('/')
    await b.waitFor(`(document.querySelector('.ds-role-head')?.innerText || '').includes(${JSON.stringify(own.section_name)})`, 15000)
    const text = await b.evaluate(`document.querySelector('.ds-role-head')?.innerText || ''`)
    check(`${lang}: section label appears once`, text.includes(own.section_name) && !/Section\s+Section\b/i.test(text) && !/শাখা\s+শাখা/u.test(text) && !/শাখা\s+Section\b/iu.test(text), text.slice(0, 140))
  }
  await open('/student/me')
  check('student profile banner uses approved photo', await b.waitFor(`!!document.querySelector('.pr-hero img')`, 15000))
  await b.login(email('admin'), pw)
  await b.go('/admin/students')
  check('student people list uses approved photo', await b.waitFor(`!![...document.querySelectorAll('main li')].find(e => e.innerText.includes(${JSON.stringify(own.name_en)}))?.querySelector('img')`, 15000))

  // A staff photo appears in the same three surfaces and in the staff list.
  const staffPhoto = await as('teacher', 'PUT', '/staff/me/photo', portrait, 'image/jpeg')
  check('staff photo is saved with a version', staffPhoto.status === 200 && !!staffPhoto.data?.photo_version)
  await b.login(email('teacher'), pw)
  await b.go('/')
  check('teacher header and greeting use staff photo', await b.waitFor(`!!document.querySelector('header .shell-avatar img') && !!document.querySelector('.ds-role-head img')`, 15000))
  await open(`/staff/profiles/${staff.id}`)
  check('staff profile banner uses photo', await b.waitFor(`!!document.querySelector('.pr-hero img')`, 15000))
  await b.login(email('admin'), pw)
  await b.go('/admin/staff')
  check('staff people list uses photo', await b.waitFor(`!![...document.querySelectorAll('main button')].find(e => e.innerText.includes(${JSON.stringify(staff.name_en)}))?.querySelector('img')`, 15000))

  const roster = (await as('admin', 'GET', '/students?page=1&page_size=100&status=active')).data.items
  const available = roster.find(s => !s.user_id) || roster.find(s => s.user_id && s.user_id !== own.user_id)
  if (available) {
    const newLogin = available.user_id ? { status: 200, data: { user_id: available.user_id } } : await as('admin', 'POST', `/students/${available.id}/login`, {})
    check('a second child account is available', (newLogin.status === 201 || newLogin.status === 200) && !!newLogin.data?.user_id)
    const familyPhone = `+88017${String(Date.now()).slice(-8)}`
    await as('admin', 'PATCH', `/users/${own.user_id}/login-phone`, { login_phone: familyPhone })
    await as('admin', 'PATCH', `/users/${newLogin.data.user_id}/login-phone`, { login_phone: familyPhone })
    await as('admin', 'PUT', `/students/${available.id}/photo`, portrait, 'image/jpeg')
    await b.login(email('student'), pw)
    await b.go('/')
    await b.evaluate(`document.querySelector('header button[aria-haspopup="menu"]')?.click()`)
    check('switch-child menu uses approved photo', await b.waitFor(`!![...document.querySelectorAll('[role="menu"] [role="menuitem"]')].find(e => e.innerText.includes(${JSON.stringify(available.name_en)}))?.querySelector('img')`, 15000))
    await as('admin', 'PATCH', `/users/${own.user_id}/login-phone`, { login_phone: own.login_phone || '' })
    await as('admin', 'PATCH', `/users/${newLogin.data.user_id}/login-phone`, { login_phone: available.login_phone || '' })
  } else check('an unlinked second child exists for switcher test', false)
} catch (error) { check('script error', false, error.stack) } finally {
  check('no browser JavaScript errors', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  writeFileSync(join(shots, 'index.md'), imageList.join('\n') + '\n')
  r.save(join(shots, 'results.json'))
  b.close()
}
