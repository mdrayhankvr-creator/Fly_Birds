// Run against a fresh demo school after suite.mjs, whitelabel.mjs and daily-tasks.mjs: see e2e/README.md.
// Checkpoint 4: rich profiles, buttons and account safety, in the browser.
//   1. Student profile (admin and the student): header, section bar,
//      attendance, results with report-card links, fees, 375px.
//   2. Staff profile: the teacher shares details per audience and adds a
//      photo; the administrator sets the NID, which shows masked and is
//      revealed on request (audited); each role sees only its fields.
//   3. Buttons: one system, pointer cursor, 44px targets on phones.
//   4. Account safety: an administrator's reset with a one-time slip, the
//      account's sessions ended, and the forced password change.
import { launch, sleep, reporter } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { writeFileSync, mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { deflateSync, crc32 } from 'node:zlib'
const here = dirname(fileURLToPath(import.meta.url))
const pw = process.env.DEMO_PASSWORD
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:5173'
const r = reporter(), check = r.check
const b = await launch({ base, shots: join(here, 'shots', 'profiles'), port: 9349 })
const email = who => `demo.${who}@demo.flybirds.test`
const js = JSON.stringify
const stamp = Date.now().toString().slice(-6)
const heading = text => b.waitFor(`[...document.querySelectorAll('main h1, main h2')].some(h => h.innerText.trim() === ${js(text)})`, 15000)
const mainText = () => b.evaluate(`document.querySelector('main')?.innerText ?? ''`)
const click = text => b.evaluate(`(() => { const e = [...document.querySelectorAll('main button, main a, main label')].find(e => e.offsetParent && !e.disabled && e.innerText.trim().replace(/\\s+/g, ' ') === ${js(text)}); if (!e) return false; e.click(); return true })()`)
const setValue = (selector, value) => b.evaluate(`(() => { const f = ${selector}; if (!f) return false; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(f, ${js(value)}); f.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)

// A 320×240 PNG made here, so the suite needs no image files.
function png(width, height) {
  const chunk = (type, data) => { const len = Buffer.alloc(4); len.writeUInt32BE(data.length); const td = Buffer.concat([Buffer.from(type), data]); const crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(td) >>> 0); return Buffer.concat([len, td, crc]) }
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(width, 0); ihdr.writeUInt32BE(height, 4); ihdr[8] = 8; ihdr[9] = 2
  const rows = Buffer.alloc((width * 3 + 1) * height)
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) { const i = y * (width * 3 + 1) + 1 + x * 3; rows[i] = 40 + x % 200; rows[i + 1] = 90 + y % 150; rows[i + 2] = 160 }
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(rows)), chunk('IEND', Buffer.alloc(0))])
}
const photoPath = join(mkdtempSync(join(tmpdir(), 'fb-photo-')), 'teacher.png')
writeFileSync(photoPath, png(320, 240))

try {
  await b.viewport(1366, 900)
  // ---------- 1. student profile ----------
  await b.login(email('admin'), pw)
  const student = (await b.api('/students?page=1&page_size=1&search=DEMO-001')).body.data.items[0]
  await b.go(`/admin/students/${student.id}`)
  check('the profile header shows both names and the placement', await b.waitFor(`(() => { const t = document.querySelector('main').innerText; return t.includes(${js(student.name_en)}) && t.includes(${js(student.name_bn)}) && t.includes(${js(student.class_name)}) && t.includes(${js(student.section_name)}) && t.includes(${js(student.session_name)}) })()`))
  const sectionBar = await b.evaluate(`[...document.querySelectorAll('main nav[aria-label] a')].map(a => a.innerText.trim())`)
  check('a section bar lists the sections the admin can read', ['Personal', 'Academic', 'Guardians', 'Attendance', 'Results', 'Fees', 'ID card'].every(s => sectionBar.includes(s)), sectionBar.join(','))
  check('attendance shows this month and this year', await b.waitFor(`/This month/.test(document.querySelector('main').innerText) && /This year\\s+\\d+%/.test(document.querySelector('main').innerText)`))
  check('fees show the total due and recent payments', await b.waitFor(`/Total due/.test(document.querySelector('main').innerText) && /Last payments/i.test(document.querySelector('main').innerText)`))
  const resultLink = await b.evaluate(`[...document.querySelectorAll('main a')].find(a => a.innerText.trim() === 'Report card')?.getAttribute('href') ?? ''`)
  check('results link to their report cards', resultLink.startsWith('/admin/exams?exam=') && resultLink.includes(student.id), resultLink)
  await b.shot('student-profile-admin', true)
  if (resultLink) {
    await b.go(resultLink)
    check('the report-card link opens that student\'s card', await b.waitFor(`/GPA|Grade/.test(document.querySelector('main').innerText) && document.querySelector('main').innerText.includes(${js(student.name_en)})`, 15000))
  }
  await b.go('/admin/attendance/students')
  check('names in the attendance roster open the profile', await b.waitFor(`[...document.querySelectorAll('main a[href^="/admin/students/"]')].some(a => a.innerText.trim() === ${js(student.name_en)})`))
  await b.viewport(375, 812, true)
  await b.go(`/admin/students/${student.id}`); await heading(student.name_en); await sleep(1500)
  check('student profile at 375px: no sideways scroll', !(await b.overflow()))
  await b.viewport(1366, 900)
  await b.login(email('student'), pw)
  await b.go('/student/me')
  check('the student sees their own attendance, results and fees', await b.waitFor(`(() => { const t = document.querySelector('main').innerText; return t.includes('Latest results') && t.includes('Total due') && /This year/.test(t) })()`, 15000))
  check('the student\'s report-card link stays in their own results', (await b.evaluate(`[...document.querySelectorAll('main a')].find(a => a.innerText.trim() === 'Report card')?.getAttribute('href') ?? ''`)).startsWith('/student/results?exam='))

  // ---------- 2. staff profile ----------
  await b.login(email('teacher'), pw)
  await b.go('/staff/me')
  check('the own staff page links to the profile', await b.waitFor(`[...document.querySelectorAll('main a')].some(a => a.innerText.trim() === 'My profile')`))
  await click('My profile')
  check('the teacher opens their profile', await b.waitFor(`!!document.querySelector('#staff-own')`, 15000))
  const own = (await b.api('/staff/me/profile')).body.data
  check('today\'s classes and subjects are shown', own.teaching.length > 0 && (await mainText()).includes("Today's classes"))
  // Photo upload from the profile header.
  const { root } = await b.send('DOM.getDocument', { depth: -1, pierce: true })
  const { nodeId } = await b.send('DOM.querySelector', { nodeId: root.nodeId, selector: 'main input[type=file]' })
  await b.send('DOM.setFileInputFiles', { nodeId, files: [photoPath] })
  check('the teacher adds a photo', await b.waitFor(`/Photo saved/.test(document.querySelector('main').innerText) && !!document.querySelector('main section img')`, 15000))
  // Qualifications and sharing: phone with families only, email with nobody.
  await setValue(`document.querySelector('form[aria-labelledby=staff-own] input[maxlength="160"]')`, `MSc in Mathematics ${stamp}`)
  const tick = (row, col) => b.evaluate(`(() => { const tr = [...document.querySelectorAll('form[aria-labelledby=staff-own] tbody tr')].find(r => r.querySelector('th')?.innerText.trim() === ${js(row)}); const box = tr?.querySelectorAll('input[type=checkbox]')[${col}]; if (!box) return false; if (!box.checked) box.click(); return true })()`)
  await tick('Phone', 1); await tick('Qualifications', 0); await tick('Qualifications', 1)
  check('NID and MPO/index are never offered for sharing', /Never shared/.test(await mainText()) && await b.evaluate(`[...document.querySelectorAll('form[aria-labelledby=staff-own] tbody tr')].find(r => /National ID/.test(r.innerText))?.querySelectorAll('input').length === 0`))
  await b.evaluate(`document.querySelector('form[aria-labelledby=staff-own] > button:not([type=button])').click()`)
  check('the teacher saves their privacy choices', await b.waitFor(`/privacy choices are saved/.test(document.querySelector('main').innerText)`))
  await b.shot('staff-profile-teacher', true)

  // The administrator sets the NID; it shows masked and is revealed on request.
  const nid = '199' + stamp + '0123'
  await b.login(email('admin'), pw)
  await b.go(`/staff/profiles/${own.id}`)
  await b.waitFor(`!!document.querySelector('#staff-official')`, 15000)
  await b.evaluate(`[...document.querySelectorAll('#staff-official ~ * button, main section button')].find(e => e.innerText.trim() === 'Edit' && e.closest('section')?.querySelector('#staff-official'))?.click()`)
  await b.waitFor(`!!document.querySelector('main input[inputmode=numeric]')`)
  await setValue(`document.querySelector('main input[inputmode=numeric]')`, nid)
  await setValue(`[...document.querySelectorAll('main section label')].find(l => (l.firstChild?.textContent ?? '').trim() === 'MPO / index number')?.querySelector('input')`, `MPO-${stamp}`)
  await b.evaluate(`[...document.querySelectorAll('main form button:not([type=button])')].find(e => e.innerText.trim() === 'Save changes').click()`)
  check('the administrator saves the official details', await b.waitFor(`/Official details saved/.test(document.querySelector('main').innerText)`))
  const masked = await b.evaluate(`document.querySelector('[data-nid]')?.innerText ?? ''`)
  check('the NID shows masked', masked.endsWith(nid.slice(-3)) && !masked.includes(nid.slice(0, 6)), masked)
  check('the page holds no full NID before a reveal', !(await b.evaluate(`document.body.innerHTML`)).includes(nid))
  await click('Show')
  check('a reveal shows the full NID', await b.waitFor(`document.querySelector('[data-nid]')?.innerText === ${js(nid)}`))
  const reveals = (await b.api(`/audit/events?page=1&page_size=20&action=staff.nid_revealed`)).body.data
  check('the reveal is in the audit log without the number', JSON.stringify(reveals).includes(own.id) && !JSON.stringify(reveals).includes(nid))
  check('the administrator sees the salary section', /Salary/.test(await mainText()))
  await b.shot('staff-profile-admin', true)
  const fieldsFor = async who => { await b.login(email(who), pw); return (await b.api(`/staff/profiles/${own.id}`)).body }
  const studentView = await fieldsFor('student')
  check('a student sees the shared phone and qualifications only', studentView.data.phone !== undefined && studentView.data.qualifications?.some(q => q.includes(stamp)) && studentView.data.email === undefined && studentView.data.mpo_index === undefined && studentView.data.nid_masked === undefined && !JSON.stringify(studentView).includes(nid))
  await b.go(`/staff/profiles/${own.id}`)
  check('the student opens the teacher profile with the photo', await b.waitFor(`!!document.querySelector('main section img') && document.querySelector('main').innerText.includes(${js(own.name_en)})`, 15000))
  check('the student has no reveal or edit controls', !(await b.evaluate(`[...document.querySelectorAll('main button')].some(e => ['Show', 'Edit', 'Change photo'].includes(e.innerText.trim()))`)))
  await b.viewport(375, 812, true); await b.go(`/staff/profiles/${own.id}`); await sleep(1500)
  check('staff profile at 375px: no sideways scroll', !(await b.overflow()))
  await b.shot('staff-profile-student-375', true)
  await b.viewport(1366, 900)
  const accountsView = await fieldsFor('accounts')
  check('other staff see only what is shared with staff', accountsView.data.phone === undefined && accountsView.data.qualifications !== undefined && accountsView.data.nid_masked === undefined)
  const headView = await fieldsFor('head')
  check('the head teacher sees contact and MPO/index but no NID', headView.data.email !== undefined && headView.data.mpo_index === `MPO-${stamp}` && headView.data.nid_masked === undefined && !JSON.stringify(headView).includes(nid))
  check('nobody outside the school sees a profile', (await fetch(`${base}/api/staff/profiles/${own.id}`)).status === 401)
  // Removing the NID is the administrator's own confirmed action.
  await b.login(email('admin'), pw)
  await b.go(`/staff/profiles/${own.id}`)
  await b.waitFor(`!!document.querySelector('[data-remove-nid]')`, 15000)
  await b.evaluate(`document.querySelector('[data-remove-nid]').click()`)
  check('removing the NID asks first', await b.waitFor(`/Remove the NID stored for/.test(document.querySelector('[role=alertdialog]')?.innerText ?? '')`))
  await b.evaluate(`[...document.querySelectorAll('[role=alertdialog] button')].find(e => e.innerText.trim() === 'Remove NID').click()`)
  check('the NID is removed', await b.waitFor(`/NID removed/.test(document.querySelector('main').innerText) && /Not provided/.test(document.querySelector('[data-nid]')?.innerText ?? '') && !document.querySelector('[data-remove-nid]')`))
  check('the removal is audited without the number', await (async () => { const ev = (await b.api(`/audit/events?page=1&page_size=10&action=staff.nid_removed`)).body.data; return JSON.stringify(ev).includes(own.id) && !JSON.stringify(ev).includes(nid) })())

  // ---------- 3. buttons ----------
  await b.login(email('admin'), pw)
  await b.viewport(375, 812, true)
  await b.go('/admin/students'); await heading('Students'); await sleep(800)
  const buttons = await b.evaluate(`[...document.querySelectorAll('main .btn')].filter(e => e.offsetParent).map(e => ({ h: e.getBoundingClientRect().height, cursor: getComputedStyle(e).cursor, disabled: e.disabled }))`)
  check('buttons are at least 44px tall on phones', buttons.length > 5 && buttons.every(x => x.h >= 44), buttons.map(x => Math.round(x.h)).join(','))
  check('enabled buttons show a pointer', buttons.filter(x => !x.disabled).every(x => x.cursor === 'pointer'))
  const looks = await b.evaluate(`(() => { const p = document.querySelector('main .btn-primary'), s = document.querySelector('main .btn-secondary'); const cs = e => getComputedStyle(e); return { primaryFill: cs(p).backgroundColor !== 'rgba(0, 0, 0, 0)', secondaryBorder: parseFloat(cs(s).borderTopWidth) >= 1 && cs(s).borderTopColor !== cs(s).backgroundColor } })()`)
  check('primary buttons are filled and secondary ones have a clear border', looks.primaryFill && looks.secondaryBorder)
  await b.viewport(1366, 900)

  // ---------- 4. account safety ----------
  // Administrators change their own password themselves: no reset button.
  await b.go(`/admin/users?search=${encodeURIComponent(email('admin'))}`)
  await b.waitFor(`[...document.querySelectorAll('main tbody button')].length === 1`)
  await b.evaluate(`document.querySelector('main tbody button').click()`)
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => /^(Deactivate|Activate)$/.test(e.innerText.trim()))`)
  check('an administrator gets no reset for their own account', !(await b.evaluate(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Reset password')`)))
  const target = `reset.${stamp}@demo.flybirds.test`
  const created = (await b.api('/users', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: target, display_name: `Reset Test ${stamp}`, role: 'teacher', password: 'Original-pass-2026' }) })).body.data
  const sessionLogin = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: target, password: 'Original-pass-2026' }) })
  const oldToken = (await sessionLogin.json()).data.access_token
  await b.go(`/admin/users?search=${encodeURIComponent(target)}`)
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => (e.getAttribute('aria-label') ?? '').includes(${js(`Reset Test ${stamp}`)}))`)
  await b.evaluate(`[...document.querySelectorAll('main button')].find(e => (e.getAttribute('aria-label') ?? '').includes(${js(`Reset Test ${stamp}`)})).click()`)
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Reset password')`)
  await click('Reset password')
  await b.waitFor(`!!document.querySelector('[role=alertdialog]')`)
  await b.evaluate(`[...document.querySelectorAll('[role=alertdialog] button')].find(e => e.innerText.trim() === 'Reset password').click()`)
  check('the reset shows a one-time slip', await b.waitFor(`!!document.querySelector('[data-temporary-password]')`))
  const temporary = await b.evaluate(`document.querySelector('[data-temporary-password]').innerText.trim()`)
  check('the slip says how many sessions ended', /1 signed-in session was ended/.test(await mainText()))
  await b.shot('reset-slip', true)
  check('the reset ended the account\'s sessions', (await fetch(`${base}/api/auth/me`, { headers: { Authorization: `Bearer ${oldToken}` } })).status === 401)
  check('the reset is audited without the password', await (async () => { const ev = (await b.api(`/audit/events?page=1&page_size=10&action=user.password_reset`)).body.data; return JSON.stringify(ev).includes(created.id) && !JSON.stringify(ev).includes(temporary) })())
  // The account must choose its own password first.
  await b.login(target, temporary)
  await b.go('/dashboard')
  check('a forced change shows only the password page', await b.waitFor(`document.querySelector('main h1')?.innerText === 'Choose your own password'`) && !(await b.evaluate(`!!document.querySelector('aside nav')`)))
  check('the server refuses other requests meanwhile', (await b.api('/dashboard')).body?.error?.code === 'password_change_required')
  await b.shot('forced-change', true)
  const fields = `[...document.querySelectorAll('main input[type=password]')]`
  await setValue(`${fields}[0]`, temporary); await setValue(`${fields}[1]`, 'My-own-password-2026'); await setValue(`${fields}[2]`, 'My-own-password-2026x')
  check('mismatched new passwords are caught', await b.waitFor(`/not the same/.test(document.querySelector('main').innerText)`))
  await setValue(`${fields}[2]`, 'My-own-password-2026')
  await b.evaluate(`document.querySelector('main form button:not([type=button])').click()`)
  check('after the change the app opens', await b.waitFor(`!!document.querySelector('aside nav') && /Good (morning|afternoon|evening)/.test(document.body.innerText)`, 15000))
  check('the new password signs in', (await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: target, password: 'My-own-password-2026' }) })).status === 200)
  await b.login(email('admin'), pw)
  await b.api(`/users/${created.id}/status`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: js({ is_active: false }) })
} catch (e) { check('script error', false, e.stack) } finally {
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  const csp = b.consoleErrors.filter(e => /Content Security Policy|Refused to/i.test(e))
  check('no Content-Security-Policy violations', csp.length === 0, csp.slice(0, 3).join(' | '))
  r.save(join(here, 'shots', 'profiles.json')); b.close()
}
