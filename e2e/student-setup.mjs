// Run only against a fresh disposable fbqa-cp1b demo database. Do not save
// screenshots or logs containing one-time student credentials.
import { launch, reporter } from './cdp.mjs'

const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('DEMO_PASSWORD is required')
const r = reporter(), check = r.check
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:8092', shots: process.env.E2E_SHOTS_DIR || 'e2e/shots/student-setup' })

try {
  await b.viewport(375, 812, true)
  await b.login('demo.admin@demo.flybirds.test', password)
  const sessions = (await b.api('/academic/sessions?page=1&page_size=100')).body.data.items
  const active = sessions.find(item => item.is_active)
  check('demo has an active session', !!active)
  const first = await b.api(`/students/synthetic-setup/preview?session_id=${active.id}`)
  check('dry run returns a student-only plan', first.status === 200 && first.body.data?.to_add > 0 && first.body.data?.students.length === first.body.data?.to_add)
  const plan = first.body.data
  check('plan uses 15 minus existing per active section', plan.sections.every(section => section.to_add === 15 - section.existing) && plan.sections.reduce((sum, section) => sum + section.to_add, 0) === plan.to_add)
  check('all planned names and identifiers are explicitly synthetic', plan.students.every(student => student.test_marker === 'TEST' && student.name_en.startsWith('TEST STUDENT TEST-') && student.login_id === student.admission_no))
  await b.go('/admin/students/setup')
  check('admin setup page opens at phone width', await b.waitFor(`!!document.querySelector('button') && document.body.innerText.includes('Synthetic student setup')`, 15000))
  check('preview page has no sideways scroll', !(await b.overflow()))
  check('active session loaded before preview', await b.waitFor(`[...document.querySelectorAll('select')].some(select => select.value === ${JSON.stringify(active.id)}) && [...document.querySelectorAll('button')].some(button => button.textContent.includes('Refresh dry-run preview') && !button.disabled)`, 15000))
  check('preview button works', await b.clickText('Refresh dry-run preview'))
  check('UI shows fresh plan before apply', await b.waitFor(`!!document.querySelector('[data-setup-plan]') && !!document.querySelector('[data-setup-apply]')`, 15000))
  const key = await b.evaluate(`document.querySelector('[data-setup-apply]')?.dataset.setupKey`)
  check('client has a stable retry key', /^[0-9a-f-]{36}$/i.test(key))
  check('plan shows incomplete personal data warning', await b.evaluate(`document.body.innerText.includes('no real personal or guardian details')`))
  check('plan fits 375px', !(await b.overflow()))
  await b.evaluate(`document.querySelector('[data-setup-apply]').click()`)
  check('explicit confirmation appears', await b.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000))
  await b.evaluate(`document.querySelector('[role=alertdialog] button:last-child').click()`)
  check('creation produces one-time printable credentials', await b.waitFor(`!!document.querySelector('[data-setup-download]') && document.querySelectorAll('[data-temporary-password]').length >= ${plan.to_add}`, 90000))
  check('success page fits 375px', !(await b.overflow()))
  const retry = await b.api('/students/synthetic-setup', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: plan.session_id, fingerprint: plan.fingerprint, idempotency_key: key, confirm: true }) })
  check('retry returns no password or new account', retry.status === 200 && retry.body.data?.replayed === true && retry.body.data?.admitted === plan.to_add && retry.body.data?.logins.length === 0)
  let exact = true
  for (const section of plan.sections) {
    const response = await b.api(`/students?section_id=${section.section_id}&page=1&page_size=100`)
    if (response.status !== 200 || response.body.data?.total !== 15) exact = false
  }
  check('every active section now has exactly 15 students', exact)
  check('download action is offered before dismissal', await b.evaluate(`!!document.querySelector('[data-setup-download]') && !!document.querySelector('a[aria-disabled=true]')`))
  const firstCredential = await b.evaluate(`({ id: document.querySelector('[data-login-id]')?.innerText.trim(), password: document.querySelector('[data-temporary-password]')?.innerText.trim() })`)
  check('credential slip contains a supported sign-in ID', !!firstCredential.id && !!firstCredential.password)
  await b.login(firstCredential.id, firstCredential.password)
  await b.go('/dashboard')
  check('new student must change the temporary password at first sign-in', await b.waitFor(`!!document.querySelector('#forced-title')`, 15000))
  await b.login('demo.admin@demo.flybirds.test', password)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'bn'); localStorage.setItem('flybirds.theme', 'dark')`)
  await b.go('/admin/students/setup')
  check('Bangla dark setup page opens', await b.waitFor(`document.body.innerText.includes('পরীক্ষামূলক শিক্ষার্থী সেটআপ')`, 15000))
  check('Bangla dark page fits 375px', !(await b.overflow()))
} catch (error) { check('script error', false, error.stack) }
finally {
  check('no browser JavaScript errors', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  r.save(process.env.E2E_RESULTS || 'e2e/shots/student-setup/results.json')
  await b.close()
}
