// Fresh disposable fbqa-cp1b demo school only. Never logs temporary passwords.
import { launch, reporter } from './cdp.mjs'
import { randomBytes } from 'node:crypto'

const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8092'
const demoPassword = process.env.DEMO_PASSWORD
if (!demoPassword) throw new Error('DEMO_PASSWORD is required')
const r = reporter(), check = r.check
const b = await launch({ base, shots: process.env.E2E_SHOTS_DIR || 'e2e/shots/staff-with-login' })
const suffix = randomBytes(4).toString('hex')
const adminEmail = 'demo.admin@demo.flybirds.test'
const set = (selector, value) => b.evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return false; const proto = el instanceof HTMLSelectElement ? HTMLSelectElement.prototype : el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event(el instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true })); return true })()`)
const click = selector => b.evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return false; el.click(); return true })()`)
const field = name => `[aria-labelledby="staff-with-login-title"] [name="${name}"]`
const openForm = async () => { await b.go('/admin/staff'); await b.waitFor(`!!document.querySelector('[data-staff-create-login]')`, 15000); await click('[data-staff-create-login]'); return b.waitFor(`!!document.querySelector('#staff-with-login-title')`, 5000) }
const fillBase = async (role, code, email) => {
  await set(field('name_en'), `${role === 'teacher' ? 'New Teacher' : 'New Head'} ${suffix}`)
  await set(field('name_bn'), role === 'teacher' ? 'নতুন শিক্ষক' : 'নতুন প্রধান শিক্ষক')
  await set(field('employee_code'), code)
  await set(field('confirm_employee_code'), code)
  await set(field('designation'), role === 'teacher' ? 'Assistant Teacher' : 'Head Teacher')
  await set(field('joined_on'), '2026-01-01')
  await set(field('role'), role)
  await set(field('email'), email)
  await set('[aria-labelledby="staff-with-login-title"] textarea', 'Mathematics\nB.Ed')
}
const submit = async (code, email) => {
  await click('[aria-labelledby="staff-with-login-title"] button[type=submit]')
  check('confirmation shows employee and login identity', await b.waitFor(`document.querySelector('[role=alertdialog]')?.innerText.includes(${JSON.stringify(code)}) && document.querySelector('[role=alertdialog]')?.innerText.includes(${JSON.stringify(email)})`, 5000))
  await click('[role=alertdialog] button:last-child')
  check('one-time credential appears', await b.waitFor(`!!document.querySelector('#staff-created-title') && !!document.querySelector('[data-temporary-password]')`, 15000))
  return b.evaluate(`document.querySelector('[data-temporary-password]')?.innerText.trim()`)
}
const changeFirstPassword = async (email, temporary, next) => {
  await b.login(email, temporary)
  await b.go('/')
  check('first sign-in requires password change', await b.waitFor(`!!document.querySelector('#forced-title')`, 15000))
  await set('input[autocomplete=current-password]', temporary)
  await set('input[autocomplete=new-password]', next)
  // The second new-password input is the confirmation, regardless of layout.
  await b.evaluate(`(() => { const e = [...document.querySelectorAll('input[autocomplete=new-password]')][1]; if (!e) return false; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(e, ${JSON.stringify(next)}); e.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)
  await click('section[aria-labelledby="forced-title"] form button')
  return b.waitFor(`!!document.querySelector('.ds-role-head')`, 15000)
}

try {
  await b.viewport(375, 812, true)
  await b.login(adminEmail, demoPassword)
  check('admin has creation action', await openForm())
  check('teacher creation form fits 375px', !(await b.overflow()))
  const sessions = (await b.api('/academic/sessions?page=1&page_size=100')).body.data.items
  const classes = (await b.api('/academic/classes?page=1&page_size=100')).body.data.items
  const activeSession = sessions.find(item => item.is_active)
  let assignedClass, assignedSubject
  for (const item of classes.filter(item => item.is_active)) {
    const choices = (await b.api(`/academic/classes/${item.id}/subjects?page=1&page_size=100`)).body.data.items
    if (choices.length) { assignedClass = item; assignedSubject = choices[0]; break }
  }
  check('demo has an existing class subject for staff assignment', !!activeSession && !!assignedClass && !!assignedSubject)
  const teacherCode = `SWL${suffix.toUpperCase()}T`, teacherEmail = `staff-teacher-${suffix}@example.test`
  await fillBase('teacher', teacherCode, teacherEmail)
  await b.waitFor(`!!document.querySelector(${JSON.stringify(`${field('session_id')} option[value="${activeSession.id}"]`)}) && !!document.querySelector(${JSON.stringify(`${field('class_id')} option[value="${assignedClass.id}"]`)})`, 10000)
  await set(field('session_id'), activeSession.id)
  await set(field('class_id'), assignedClass.id)
  await b.waitFor(`!!document.querySelector(${JSON.stringify(`${field('subject_id')} option[value="${assignedSubject.subject_id}"]`)})`, 10000)
  await set(field('subject_id'), assignedSubject.subject_id)
  check('photo can be selected in browser', await b.evaluate(`(async () => { const input = document.querySelector('[aria-labelledby="staff-with-login-title"] input[type=file]'); const c = document.createElement('canvas'); c.width = c.height = 80; c.getContext('2d').fillStyle = '#267ca8'; c.getContext('2d').fillRect(0,0,80,80); const blob = await new Promise(resolve => c.toBlob(resolve, 'image/png')); const dt = new DataTransfer(); dt.items.add(new File([blob], 'staff.png', { type: 'image/png' })); input.files = dt.files; input.dispatchEvent(new Event('change', { bubbles: true })); return true })()`))
  check('selected photo preview appears', await b.waitFor(`!!document.querySelector('[aria-labelledby="staff-with-login-title"] img')`, 5000))
  const teacherTemporary = await submit(teacherCode, teacherEmail)
  check('temporary credential is generated once', typeof teacherTemporary === 'string' && teacherTemporary.length >= 12)
  const teacherRows = (await b.api(`/staff/employees?search=${teacherCode}&page=1&page_size=10`)).body.data.items
  const teacher = teacherRows.find(item => item.employee_code === teacherCode)
  check('teacher account, photo and profile are linked', !!teacher?.user_id && !!teacher?.photo_version)
  const assignments = (await b.api(`/staff/employees/${teacher.id}/assignments?page=1&page_size=10`)).body.data.items
  check('initial teaching assignment was saved', assignments.some(item => item.subject_id === assignedSubject.subject_id && item.class_id === assignedClass.id))
  check('password is absent from later profile reads', !(JSON.stringify((await b.api(`/staff/profiles/${teacher.id}`)).body).includes(teacherTemporary)))
  await click('[aria-labelledby="staff-created-title"] button:last-child')
  check('one-time password disappears after acknowledgement', !(await b.evaluate(`!!document.querySelector('[data-temporary-password]')`)))
  check('teacher can finish first sign-in', await changeFirstPassword(teacherEmail, teacherTemporary, `Chosen-teacher-${suffix}-2026`))
  check('teacher photo appears in header and dashboard', await b.waitFor(`!!document.querySelector('header .shell-avatar img') && !!document.querySelector('.ds-role-head img')`, 15000))
  await b.go('/staff/profiles/me')
  check('teacher profile banner uses the photo', await b.waitFor(`!!document.querySelector('.pr-hero img')`, 15000))

  await b.login(adminEmail, demoPassword)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'bn'); localStorage.setItem('flybirds.theme', 'dark')`)
  check('Bangla admin can open creation form', await openForm())
  check('Bangla dark form fits 375px', !(await b.overflow()))
  const headCode = `SWL${suffix.toUpperCase()}H`, headEmail = `staff-head-${suffix}@example.test`
  await fillBase('head_teacher', headCode, headEmail)
  const headTemporary = await submit(headCode, headEmail)
  check('head-teacher receives a different credential', headTemporary !== teacherTemporary)
  check('head teacher can finish first sign-in', await changeFirstPassword(headEmail, headTemporary, `Chosen-head-${suffix}-2026`))
  check('head teacher without photo uses initials', await b.waitFor(`!!document.querySelector('header .shell-avatar') && !document.querySelector('header .shell-avatar img')`, 15000))
  check('head teacher has no sideways scroll at 375px', !(await b.overflow()))
} catch (error) { check('script error', false, error.stack) }
finally {
  check('no browser JavaScript errors', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  r.save(process.env.E2E_RESULTS || 'e2e/shots/staff-with-login/results.json')
  await b.close()
}
