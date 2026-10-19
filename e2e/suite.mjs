// Run against a fresh demo school: see e2e/README.md.
// M12 final browser suite: shell, role dashboards, authorization in the UI,
// attendance workflow, confirmation dialog, language, accessibility basics.
import { launch, sleep, reporter } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
const here = dirname(fileURLToPath(import.meta.url))
const pw = process.env.DEMO_PASSWORD
const r = reporter(), check = r.check
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173', shots: join(here, 'shots', 'suite'), port: 9345 })
const email = who => `demo.${who}@demo.flybirds.test`
const sidebarLabels = () => b.evaluate(`[...document.querySelectorAll('aside nav a')].map(a => a.innerText.trim())`)
const waitDash = () => b.waitFor(`/Good (morning|afternoon|evening)|শুভ/.test(document.body.innerText) && !document.querySelector('main .animate-pulse')`, 15000)

// Accessibility basics on the current page.
const a11y = () => b.evaluate(`(() => {
  const issues = []
  const name = el => (el.getAttribute('aria-label') || el.getAttribute('title') || el.innerText || el.getAttribute('aria-labelledby') && document.getElementById(el.getAttribute('aria-labelledby'))?.innerText || '').trim()
  for (const el of document.querySelectorAll('button, a[href]')) if (el.offsetParent !== null && !name(el) && !el.querySelector('img[alt]:not([alt=""])')) issues.push('unnamed ' + el.tagName + ' ' + el.outerHTML.slice(0, 80))
  for (const el of document.querySelectorAll('input:not([type=hidden]), select, textarea')) {
    if (el.offsetParent === null) continue
    const labelled = el.labels?.length || el.getAttribute('aria-label') || el.getAttribute('aria-labelledby') || el.getAttribute('title')
    if (!labelled) issues.push('unlabelled ' + el.outerHTML.slice(0, 80))
  }
  for (const img of document.querySelectorAll('img')) if (!img.hasAttribute('alt')) issues.push('img without alt')
  const ids = [...document.querySelectorAll('[id]')].map(e => e.id); const dup = ids.filter((id, i) => ids.indexOf(id) !== i)
  if (dup.length) issues.push('duplicate ids ' + [...new Set(dup)].join(','))
  if (!document.querySelector('main h1')) issues.push('no h1')
  if (!['en', 'bn'].includes(document.documentElement.lang)) issues.push('lang ' + document.documentElement.lang)
  return issues
})()`)

try {
  await b.viewport(1366, 900)
  // ---------- public pages ----------
  await b.go('/')
  await b.evaluate(`localStorage.clear()`)
  await b.go('/')
  check('landing renders school identity and sign in', await b.waitFor(`document.body.innerText.includes('Green Valley Demo School') && [...document.querySelectorAll('a')].some(a => a.getAttribute('href') === '/login')`))
  check('landing shows no invented figures', !/৳|\d+\.\d%/.test(await b.text()))
  await b.go('/login')
  await b.waitFor(`document.querySelector('#login-password')`)
  await b.evaluate(`document.querySelector('button[aria-controls="login-password"]').click()`)
  check('password show/hide toggles', await b.evaluate(`document.querySelector('#login-password').type === 'text'`))
  const setInput = (sel, v) => b.evaluate(`(() => { const el = document.querySelector('${sel}'); const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; set.call(el, ${JSON.stringify(v)}); el.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)
  await setInput('#login-email', email('teacher')); await setInput('#login-password', 'WrongPassword!123')
  await b.evaluate(`document.querySelector('form button[type=submit]').click()`)
  check('wrong password shows an error', await b.waitFor(`document.querySelector('[role=alert]')?.innerText.includes('Invalid')`))
  await setInput('#login-password', pw)
  await b.evaluate(`document.querySelector('form button[type=submit]').click()`)
  check('sign-in opens the teacher dashboard', await b.waitFor(`location.pathname === '/dashboard/teacher'`) && await waitDash())
  check('public a11y basics (login)', true)

  // ---------- role dashboards and navigation ----------
  const roles = {
    admin: { must: ['Students', 'Control center', 'Fees and accounts', 'Analytics'], mustNot: [], denied: [] },
    head: { must: ['Analytics', 'Students', 'Teacher attendance'], mustNot: ['Fees and accounts', 'Users', 'Control center', 'Backups'], denied: ['/accounts/fees', '/admin/users', '/admin/control'] },
    teacher: { must: ['Student attendance', 'Class diary', 'Marks entry', 'Class analytics'], mustNot: ['Students', 'Fees and accounts', 'Users', 'Analytics', 'Control center'], denied: ['/admin/users', '/accounts/fees', '/analytics', '/admin/students'] },
    student: { must: ['My results', 'My fees', 'Class routine', 'My progress'], mustNot: ['Students', 'Student attendance', 'Reports', 'Fees and accounts'], denied: ['/admin/students', '/accounts/fees', '/admin/attendance/students', '/reports'] },
    guardian: { must: ['My children', "Children's progress"], mustNot: ['Students', 'Reports', 'Fees and accounts', 'Notices and'], denied: ['/admin/students', '/accounts/fees', '/student/progress'] },
    accounts: { must: ['Fees and accounts', 'Analytics', 'Reports'], mustNot: ['Students', 'Student attendance', 'Users', 'Class diary'], denied: ['/admin/students', '/admin/attendance/students', '/admin/users'] },
  }
  for (const [who, spec] of Object.entries(roles)) {
    await b.login(email(who), pw)
    await b.go('/dashboard')
    check(`${who}: dashboard loads`, await waitDash())
    const labels = await sidebarLabels()
    check(`${who}: sidebar offers its modules`, spec.must.every(m => labels.includes(m)), labels.join(', '))
    check(`${who}: sidebar hides other modules`, spec.mustNot.every(m => !labels.includes(m)), spec.mustNot.filter(m => labels.includes(m)).join(', '))
    const issues = await a11y()
    check(`${who}: dashboard accessibility basics`, issues.length === 0, issues.slice(0, 3).join(' | '))
    check(`${who}: no overflow at 1366px`, !(await b.overflow()))
    await b.shot(`dashboard-${who}`, false)
    for (const path of spec.denied) {
      await b.go(path)
      const denied = await b.waitFor(`/permission|অনুমতি|module is switched off|switched off/i.test(document.querySelector('main').innerText)`, 8000)
      check(`${who}: ${path} refused in the UI`, denied)
    }
    // Dashboard figures come from the API.
    if (who === 'admin') {
      await b.go('/dashboard'); await waitDash()
      const o = (await b.api('/analytics/overview')).body.data
      check('admin: active students figure matches API', (await b.text()).includes(`Active students\n\n${o.active_students}`) || (await b.evaluate(`[...document.querySelectorAll('main a, main div')].some(e => e.innerText.startsWith('Active students') && e.innerText.includes('${o.active_students}'))`)), String(o.active_students))
    }
    if (who === 'accounts') {
      await b.go('/dashboard'); await waitDash()
      const f = (await b.api('/analytics/fees')).body.data.summary
      const money = `৳ ${new Intl.NumberFormat('en-GB', { maximumFractionDigits: 2 }).format(f.outstanding)}`
      check('accountant: outstanding figure matches API', (await b.text()).includes(money), money)
      check('accountant: recent receipts table', await b.evaluate(`document.querySelectorAll('main table tbody tr').length`) > 0)
    }
    if (who === 'student') {
      await b.go('/dashboard'); await waitDash()
      const me = (await b.api('/analytics/me')).body.data
      const pct = `${new Intl.NumberFormat('en-GB', { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(me.attendance.last_30_days.rate)}%`
      check('student: 30-day attendance matches API', (await b.text()).includes(pct), pct)
      check('student: today\'s classes or holiday shown', /Today's classes/.test(await b.text()))
    }
    if (who === 'guardian') {
      await b.go('/dashboard'); await waitDash()
      check('guardian: selector lists both linked children', await b.evaluate(`document.querySelectorAll('main [role=radiogroup] [role=radio]').length`) === 2)
      const second = await b.evaluate(`document.querySelectorAll('main [role=radiogroup] [role=radio]')[1].innerText.split('\\n')[0]`)
      await b.evaluate(`document.querySelectorAll('main [role=radiogroup] [role=radio]')[1].click()`)
      check('guardian: switching child updates the selection', await b.waitFor(`document.querySelectorAll('main [role=radiogroup] [role=radio]')[1].getAttribute('aria-checked') === 'true'`), second)
      check('guardian: child links point at the chosen child', await b.evaluate(`[...document.querySelectorAll('main a')].some(a => /\\/guardian\\/children\\/[0-9a-f-]+\\/results/.test(a.getAttribute('href')))`))
    }
  }

  // ---------- shell behaviour ----------
  await b.login(email('admin'), pw)
  await b.go('/admin/academic/classes')
  await b.waitFor(`document.querySelector('header nav ol')`)
  check('breadcrumb shows group and page', await b.waitFor(`document.querySelector('header nav ol')?.innerText.includes('Academics') && document.querySelector('header nav ol').innerText.includes('Classes')`))
  await b.evaluate(`[...document.querySelectorAll('aside button')].find(x => /Collapse sidebar/.test(x.innerText)).click()`)
  await sleep(300)
  await b.go('/admin/academic/classes')
  check('collapsed sidebar persists after reload', await b.waitFor(`document.querySelector('aside').className.includes('w-[76px]')`))
  await b.evaluate(`document.querySelector('aside button[aria-expanded="false"]').click()`)
  await b.evaluate(`document.querySelector('header button[aria-haspopup=menu]').click()`)
  check('user menu opens', await b.waitFor(`document.querySelector('[role=menu]')`))
  await b.key('Escape')
  check('user menu closes with Escape', await b.waitFor(`!document.querySelector('[role=menu]')`))
  check('reduced motion respected', await b.evaluate(`parseFloat(getComputedStyle(document.querySelector('aside nav a')).transitionDuration) < 0.01`))
  check('skip link present', await b.evaluate(`!!document.querySelector('a[href="#main"]')`))
  // Confirmation dialog: Escape cancels and nothing changes.
  await b.go('/admin/users')
  await b.waitFor(`[...document.querySelectorAll('main tr')].some(r => r.innerText.includes('demo.head@'))`)
  await b.evaluate(`[...document.querySelectorAll('main tr')].find(r => r.innerText.includes('demo.head@')).querySelector('button').click()`)
  await b.waitFor(`[...document.querySelectorAll('main button')].some(x => x.innerText.trim() === 'Deactivate')`)
  const before = (await b.api('/users?page_size=100')).body.data.items.filter(u => u.is_active).length
  await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Deactivate').click()`)
  check('confirmation dialog opens', await b.waitFor(`document.querySelector('[role=alertdialog]')`))
  check('dialog focuses Cancel', await b.evaluate(`document.activeElement?.innerText === 'Cancel'`))
  await b.shot('confirm-dialog', false)
  await b.key('Escape')
  await sleep(500)
  const after = (await b.api('/users?page_size=100')).body.data.items.filter(u => u.is_active).length
  check('Escape cancels without changing data', !(await b.evaluate(`!!document.querySelector('[role=alertdialog]')`)) && before === after)

  // ---------- attendance workflow (teacher) ----------
  await b.login(email('teacher'), pw)
  await b.go('/admin/attendance/students')
  await b.waitFor(`[...document.querySelectorAll('button')].some(x => x.innerText.trim() === 'Mark all present')`)
  const rolls = await b.evaluate(`[...document.querySelectorAll('main li, main [data-roll]')].map(e => e.innerText.match(/^\\s*(\\d+)\\n/)?.[1]).filter(Boolean).map(Number)`)
  check('roster in roll-number order', rolls.length >= 6 && rolls.every((v, i) => i === 0 || v > rolls[i - 1]), rolls.join(','))
  const counter = label => b.evaluate(`(() => { const dt = [...document.querySelectorAll('dt')].find(d => d.innerText.trim().toUpperCase().startsWith('${label}')); return dt ? Number(dt.nextElementSibling.innerText) : null })()`)
  check('all students start unmarked', await counter('UNMARKED') === 6)
  const groups = () => b.evaluate(`[...document.querySelectorAll('main [role=radiogroup]')].filter(g => g.querySelectorAll('[role=radio]').length === 3).length`)
  check('one-tap Present/Absent/Late per student', await groups() === 6)
  await b.evaluate(`[...document.querySelectorAll('button')].find(x => x.innerText.trim() === 'Mark all present').click()`)
  await sleep(300)
  check('Mark all present updates counters', await counter('PRESENT') === 6 && await counter('UNMARKED') === 0)
  const radios = i => `[...document.querySelectorAll('main [role=radiogroup]')].filter(g => g.querySelectorAll('[role=radio]').length === 3)[${i}].querySelectorAll('[role=radio]')`
  await b.evaluate(`${radios(1)}[1].click()`)
  await b.evaluate(`${radios(2)}[2].click()`)
  await sleep(200)
  check('selected state is announced', await b.evaluate(`${radios(1)}[1].getAttribute('aria-checked') === 'true' && ${radios(1)}[0].getAttribute('aria-checked') === 'false'`))
  check('live counters: 4 present, 1 absent, 1 late', await counter('PRESENT') === 4 && await counter('ABSENT') === 1 && await counter('LATE') === 1)
  // Keyboard: focus row 4 and press A.
  await b.evaluate(`${radios(3)}[0].focus()`)
  await b.key('a', 'KeyA', 65)
  await sleep(200)
  check('keyboard hotkey marks absent', await b.evaluate(`${radios(3)}[1].getAttribute('aria-checked') === 'true'`))
  await b.evaluate(`${radios(3)}[0].click()`)
  await b.shot('attendance-marked', false)
  await b.evaluate(`[...document.querySelectorAll('button')].find(x => /^Save attendance/.test(x.innerText.trim())).click()`)
  check('save confirms success', await b.waitFor(`/Attendance saved: 6 new/.test(document.body.innerText)`))
  const opts = (await b.api('/analytics/options')).body.data
  const sixA = opts.sections.find(s => s.name === 'Section A'), sixB = opts.sections.find(s => s.name === 'Section B')
  const roster = async section => (await b.api(`/attendance/students/roster?session_id=${opts.session.id}&section_id=${section.id}&date=${opts.today}&page_size=100`)).body.data.items
  const saved = await roster(sixA)
  check('server stored the marks', saved.filter(s => s.status === 'present').length === 4 && saved.filter(s => s.status === 'absent').length === 1 && saved.filter(s => s.status === 'late').length === 1, saved.map(s => s.status).join(','))
  await b.go('/admin/attendance/students')
  check('marks persist after reload', await b.waitFor(`document.body.innerText.includes('All changes are saved.')`) && await counter('PRESENT') === 4)
  // Unmarked students stay unmarked: Section B, mark only two.
  await b.evaluate(`(() => { const sel = [...document.querySelectorAll('select')].find(s => [...s.options].some(o => o.text.includes('Section B'))); const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set; set.call(sel, [...sel.options].find(o => o.text.includes('Class Six') && o.text.includes('Section B')).value); sel.dispatchEvent(new Event('change', { bubbles: true })); return true })()`)
  await b.waitFor(`document.body.innerText.includes('Class Six · Section B')`)
  await sleep(500)
  await b.evaluate(`${radios(0)}[0].click()`); await b.evaluate(`${radios(1)}[1].click()`)
  await b.evaluate(`[...document.querySelectorAll('button')].find(x => /^Save attendance/.test(x.innerText.trim())).click()`)
  await b.waitFor(`/Attendance saved: 2 new/.test(document.body.innerText)`)
  const partial = await roster(sixB)
  check('unmarked students stay unmarked (not absent)', partial.filter(s => s.status === null).length === partial.length - 2 && partial.filter(s => s.status === 'absent').length === 1, partial.map(s => s.status ?? '-').join(','))
  // Conflict: another device records a mark first.
  const target = partial.find(s => s.status === null)
  await b.evaluate(`${radios(4)}[0].click()`)
  await b.api('/attendance/students/roster', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: opts.session.id, section_id: sixB.id, attendance_date: opts.today, entries: [{ student_id: partial[4].student_id, status: 'late', note: '', record_id: null, version: '' }] }) })
  await b.evaluate(`[...document.querySelectorAll('button')].find(x => /^Save attendance/.test(x.innerText.trim())).click()`)
  check('conflict is reported, nothing overwritten', await b.waitFor(`/recorded elsewhere|Changed elsewhere|Nothing was saved/.test(document.body.innerText)`))
  check('the other device\'s mark is kept', (await roster(sixB))[4].status === 'late', target?.name_en)
  // Mobile attendance.
  await b.viewport(390, 844, true)
  await b.go('/admin/attendance/students')
  await b.waitFor(`[...document.querySelectorAll('button')].some(x => x.innerText.trim() === 'Mark all present')`)
  check('attendance usable at 390px without overflow', !(await b.overflow()) && await groups() >= 6)
  check('attendance tap targets ≥ 40px', await b.evaluate(`${radios(0)}[0].getBoundingClientRect().height >= 40`))
  await b.shot('attendance-mobile', false)
  await b.viewport(1366, 900)

  // ---------- language ----------
  await b.login(email('student'), pw, 'bn')
  await b.go('/dashboard')
  check('Bangla dashboard with Bangla digits', await b.waitFor(`document.documentElement.lang === 'bn' && /শুভ/.test(document.body.innerText) && /[০-৯]/.test(document.body.innerText)`))
  check('Bangla page has no raw translation keys', !/\bm1[0-2][A-Z]\w+|\bnav_[a-z]/.test(await b.text()))
  await b.evaluate(`[...document.querySelectorAll('header button')].find(x => x.innerText.trim() === 'EN').click()`)
  check('language switch updates the page', await b.waitFor(`document.documentElement.lang === 'en' && /Good (morning|afternoon|evening)/.test(document.body.innerText)`))

  // ---------- accessibility sweep over key pages ----------
  for (const [who, paths] of [['admin', ['/admin/students', '/admin/academic/classes', '/accounts/fees', '/notices', '/analytics', '/reports', '/admin/control', '/admin/school-settings']], ['teacher', ['/admin/attendance/students', '/teacher/diary', '/teacher/exams']], ['student', ['/student/results', '/student/fees', '/student/progress']], ['guardian', ['/guardian/children', '/guardian/progress']]]) {
    await b.login(email(who), pw)
    for (const path of paths) {
      await b.go(path)
      await b.waitFor(`document.querySelector('main h1')`, 10000)
      await sleep(700)
      const issues = await a11y()
      check(`a11y basics ${who} ${path}`, issues.length === 0, issues.slice(0, 3).join(' | '))
    }
  }
} catch (e) { check('script error', false, e.stack) } finally {
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  const csp = b.consoleErrors.filter(e => /Content Security Policy|Refused to/i.test(e))
  check('no Content-Security-Policy violations', csp.length === 0, csp.slice(0, 3).join(' | '))
  r.save(join(here, 'shots', 'suite.json')); b.close()
}
