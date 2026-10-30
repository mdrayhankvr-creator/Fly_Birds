// Run against a fresh demo school after suite.mjs and whitelabel.mjs: see e2e/README.md.
// Checkpoint 3: the three daily office tasks, end to end in the browser.
//   1. One-page admission (admin): defaults, duplicate-guardian offer, one
//      transaction, the printable slip with the initial password, a refused
//      admission leaving nothing behind, 375px.
//   2. Fee counter (accountant): search by guardian phone and roll, dues
//      oldest first, part and full payment in a few clicks, a double submit
//      recording once, receipts with both names and the amount in words,
//      keyboard use, 375px.
//   3. Quick attendance (teacher, 375px): everyone present by default, tap to
//      mark absent or late, the unsaved state, one Save, and the optimistic
//      lock keeping another device's mark.
import { launch, sleep, reporter } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
const here = dirname(fileURLToPath(import.meta.url))
const pw = process.env.DEMO_PASSWORD
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:5173'
const r = reporter(), check = r.check
const b = await launch({ base, shots: join(here, 'shots', 'daily'), port: 9347 })
const email = who => `demo.${who}@demo.flybirds.test`
const js = JSON.stringify
const stamp = Date.now().toString().slice(-6)
const setValue = (selector, value) => b.evaluate(`(() => { const f = ${selector}; if (!f) return false; f.focus(); const proto = f.tagName === 'SELECT' ? HTMLSelectElement.prototype : f.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, 'value').set.call(f, ${js(value)}); f.dispatchEvent(new Event(f.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true })); return true })()`)
// A visible form field by its label's own text; 'last:' takes the last match.
const field = (label, tag = 'input,select,textarea') => `[...document.querySelectorAll('main label')].filter(l => l.offsetParent)[${js(label)}.startsWith('last:') ? 'findLast' : 'find'](l => (l.firstChild?.textContent ?? '').trim() === ${js(label)}.replace(/^last:/, ''))?.querySelector(${js(tag)})`
const fill = (label, value) => setValue(field(label), value)
const choose = async (label, text) => {
  await b.waitFor(`[...(${field(label, 'select')})?.options ?? []].some(o => o.text.includes(${js(text)}))`)
  return b.evaluate(`(() => { const s = ${field(label, 'select')}; const o = [...s.options].find(o => o.text.includes(${js(text)})); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, o.value); s.dispatchEvent(new Event('change', { bubbles: true })); return true })()`)
}
const click = text => b.evaluate(`(() => { const e = [...document.querySelectorAll('main button, main a')].find(e => e.offsetParent && !e.disabled && e.innerText.trim().replace(/\\s+/g, ' ') === ${js(text)}); if (!e) return false; e.click(); return true })()`)
const collectButton = `document.querySelector('form[aria-labelledby=counter-collect] button:not([type=button])')`
const heading = text => b.waitFor(`document.querySelector('main h1')?.innerText.trim() === ${js(text)}`, 15000)

try {
  await b.viewport(1366, 900)
  // ---------- 1. one-page admission ----------
  await b.login(email('admin'), pw)
  const existing = (await b.api('/guardians?page=1&page_size=100')).body.data.items.find(g => g.phone)
  await b.go('/admin/students')
  await heading('Students')
  check('the student list offers "Admit a student"', await click('Admit a student'))
  check('the admission page opens', await heading('Admit a student') && await b.waitFor(`document.querySelector('main select')?.options.length > 0`))
  check('the current session is chosen', await b.evaluate(`!!${field('Academic session', 'select')}?.value`))
  await choose('Class', 'Class Seven'); await choose('Section', 'Section B')
  const sectionId = await b.evaluate(`${field('Section', 'select')}.value`)
  const defaults = (await b.api(`/students/admission/defaults?section_id=${sectionId}`)).body.data
  check('the next free roll is suggested', await b.waitFor(`document.getElementById('admit-roll').value === ${js(String(defaults.next_roll))}`), `roll ${defaults.next_roll}`)
  const suggestedNo = await b.evaluate(`document.getElementById('admit-admission').value`)
  check('the next admission number is suggested', !!suggestedNo && suggestedNo === defaults.admission_no, suggestedNo)
  check('a login email is suggested from it', await b.evaluate(`document.getElementById('admit-login').value`) === `${suggestedNo.toLowerCase()}@demo.flybirds.test`)
  check('profile details are folded away', await b.evaluate(`!document.querySelector('main details').open`))
  // A refused admission (the login email is taken) leaves nothing behind.
  await fill('Name in English', `Daily Test ${stamp}`)
  await fill('Name in Bangla', 'দৈনিক পরীক্ষা')
  await fill('Guardian name in English', `Uncle ${stamp}`)
  await setValue(`document.getElementById('admit-login')`, email('teacher'))
  await click('Admit student')
  check('a taken login email is reported at the field', await b.waitFor(`/already used/.test(document.getElementById('admit-login').closest('label').innerText)`))
  const left = (await b.api(`/students?page=1&page_size=10&search=${encodeURIComponent(suggestedNo)}`)).body.data.total
  const leftGuardians = (await b.api(`/guardians?page=1&page_size=10&search=${encodeURIComponent('Uncle ' + stamp)}`)).body.data.total
  check('the refused admission wrote no student and no guardian', left === 0 && leftGuardians === 0, `${left} students, ${leftGuardians} guardians`)
  await setValue(`document.getElementById('admit-login')`, `${suggestedNo.toLowerCase()}@demo.flybirds.test`)
  // The guardian on record is offered instead of a duplicate.
  await fill('Guardian name in English', `Father ${stamp}`)
  await fill('Phone', existing.phone.replace(/^0/, '+880 '))
  check('a guardian with the same phone is offered', await b.waitFor(`document.querySelector('main').innerText.includes('Already on record') && document.querySelector('main').innerText.includes(${js(existing.name_en)})`))
  check('linking the guardian on record', await click('Use this guardian') && await b.waitFor(`document.querySelector('main').innerText.includes('will be linked, not copied')`))
  await click('Add a second guardian')
  await fill('last:Guardian name in English', `Mother ${stamp}`)
  await fill('last:Phone', `0199${stamp}9`)
  await b.shot('admission-form', true)
  const guardiansBefore = (await b.api(`/guardians?page=1&page_size=100&search=${encodeURIComponent(existing.phone)}`)).body.data.total
  await click('Admit student')
  check('the admission slip appears', await b.waitFor(`!!document.querySelector('.admission-slip')`, 15000))
  const password = await b.evaluate(`document.querySelector('[data-initial-password]')?.innerText ?? ''`)
  const slip = await b.evaluate(`document.querySelector('.admission-slip')?.innerText ?? ''`)
  check('the slip shows the student, both guardians and the login', slip.includes(`Daily Test ${stamp}`) && slip.includes('দৈনিক পরীক্ষা') && slip.includes(existing.name_en) && slip.includes(`Mother ${stamp}`) && slip.includes(`${suggestedNo.toLowerCase()}@demo.flybirds.test`))
  check('the initial password is shown once, with a warning', /^[A-Za-z2-9]{4}-[A-Za-z2-9]{4}-[A-Za-z2-9]{4}$/.test(password) && slip.includes('shown only now'))
  await b.shot('admission-slip', true)
  const admitted = (await b.api(`/students?page=1&page_size=10&search=${encodeURIComponent(suggestedNo)}`)).body.data.items[0]
  const links = admitted ? (await b.api(`/students/${admitted.id}/guardians`)).body.data : []
  check('the server holds the student, the login and both guardians', !!admitted?.user_id && admitted.roll_no === defaults.next_roll && links.length === 2 && links.some(l => l.guardian_id === existing.id && l.is_primary))
  check('the guardian on record was not copied', (await b.api(`/guardians?page=1&page_size=100&search=${encodeURIComponent(existing.phone)}`)).body.data.total === guardiansBefore)
  const signIn = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `${suggestedNo.toLowerCase()}@demo.flybirds.test`, password }) })
  check('the new student signs in with the slip password', signIn.status === 200 && (await signIn.json()).data?.user?.role === 'student')
  // The next admission keeps the placement and moves the suggestions on.
  await click('Admit another')
  check('"Admit another" keeps the class and suggests the next roll', await b.waitFor(`${field('Section', 'select')}?.value === ${js(sectionId)} && document.getElementById('admit-roll').value === ${js(String(defaults.next_roll + 1))}`))
  await b.viewport(375, 812, true)
  await b.go('/admin/students/admit'); await heading('Admit a student'); await sleep(800)
  check('admission at 375px: no sideways scroll', !(await b.overflow()))
  check('admission at 375px: the save button is a large target', await b.evaluate(`[...document.querySelectorAll('main button')].find(e => e.innerText.trim() === 'Admit student').getBoundingClientRect().height >= 44`))
  await b.shot('admission-375', true)
  await b.viewport(1366, 900)

  // ---------- 2. fee counter ----------
  const due = (await b.api('/fees/dues?page=1&page_size=100&status=outstanding')).body.data.items
  let target = null, counterPhone = ''
  for (const d of due.filter(d => d.balance >= 200)) {
    const phone = (await b.api(`/students/${d.student_id}/guardians`)).body.data.find(l => l.phone)?.phone
    if (phone) { target = d; counterPhone = phone; break }
  }
  check('demo data: a student with a fee due and a guardian phone', !!target && !!counterPhone, target?.student_name)
  await b.login(email('accounts'), pw)
  await b.go('/accounts/fees')
  check('the counter opens with the search focused', await b.waitFor(`document.activeElement?.id === 'counter-search'`))
  await setValue(`document.getElementById('counter-search')`, counterPhone.slice(-6))
  check('search by the last digits of the guardian phone finds the student', await b.waitFor(`[...document.querySelectorAll('#counter-results [role=option]')].some(o => o.innerText.includes(${js(target.student_name)}))`))
  await b.evaluate(`[...document.querySelectorAll('#counter-results [role=option]')].find(o => o.innerText.includes(${js(target.student_name)})).click()`)
  const statement = async () => (await b.api(`/fees/students/${target.student_id}/statement`)).body.data
  const before = await statement()
  check('the student opens with the amount ready for Enter', await b.waitFor(`document.activeElement?.id === 'counter-amount'`))
  const dates = await b.evaluate(`[...document.querySelectorAll('main ul li label')].map(l => (l.innerText.match(/\\d{4}-\\d{2}-\\d{2}/) || [''])[0])`)
  check('dues are listed oldest first', dates.length > 0 && dates.every((d, i) => i === 0 || d >= dates[i - 1]), dates.join(','))
  check('the total due is shown', (await b.text()).includes('Total due'))
  // Part payment by bKash: the reference is required.
  await setValue(`document.getElementById('counter-amount')`, '100')
  await click('bKash')
  check('bKash needs its transaction ID before collecting', await b.evaluate(`${collectButton}.disabled`))
  await fill('Transaction ID', `TRX${stamp}`)
  await click('Collect ৳ 100.00')
  check('a part payment prints a receipt', await b.waitFor(`!!document.querySelector('.fee-receipt')`, 15000))
  const receipt = await b.evaluate(`document.querySelector('.fee-receipt').innerText`)
  check('the receipt names the student in English and Bangla', receipt.includes(target.student_name) && (!target.student_name_bn || receipt.includes(target.student_name_bn)))
  check('the receipt gives the amount in words in both languages', receipt.includes('Taka One Hundred Only') && receipt.includes('একশত টাকা মাত্র'))
  check('the print button has the focus', await b.evaluate(`document.activeElement?.innerText.trim() === 'Print receipt'`))
  await b.shot('counter-receipt', true)
  // A double submit records one payment.
  await click('Back to dues'); await b.waitFor(`!!document.getElementById('counter-amount')`)
  await setValue(`document.getElementById('counter-amount')`, '50')
  await click('Cash')
  const paidBefore = (await statement()).payments.length
  await b.evaluate(`(() => { const btn = ${collectButton}; btn.click(); btn.click(); btn.form.requestSubmit() })()`)
  await b.waitFor(`!!document.querySelector('.fee-receipt')`, 15000); await sleep(800)
  check('a double submit records one payment', (await statement()).payments.length === paidBefore + 1)
  // Full payment: an empty amount collects everything due.
  await click('Back to dues'); await b.waitFor(`!!document.getElementById('counter-amount')`)
  const left2 = (await statement()).totals.balance
  await click('Cash')
  const fullLabel = await b.evaluate(`${collectButton}.innerText.trim()`)
  check('the full due is offered when the amount is empty', fullLabel.replace(/,/g, '').includes(left2.toFixed(2)), fullLabel)
  await b.evaluate(`${collectButton}.click()`)
  await b.waitFor(`!!document.querySelector('.fee-receipt')`, 15000); await sleep(500)
  const after = await statement()
  check('a full payment settles the account', after.totals.balance === 0 && after.payments.length === before.payments.length + 3, `${after.totals.balance}`)
  // Keyboard: Esc goes back to the search; a roll number finds the student.
  await b.key('Escape', 'Escape', 27)
  check('Esc returns to the search with the focus', await b.waitFor(`document.activeElement?.id === 'counter-search'`))
  const roll = (await b.api(`/fees/students?page=1&page_size=1&search=${encodeURIComponent(before.student.admission_no)}`)).body.data.items[0]?.roll_no
  await setValue(`document.getElementById('counter-search')`, String(roll))
  check('a roll number lists that roll first', await b.waitFor(`/Roll ${roll}\\b/.test(document.querySelector('#counter-results [role=option]')?.innerText ?? '')`))
  await b.key('Enter', 'Enter', 13)
  check('Enter picks the highlighted student', await b.waitFor(`!!document.getElementById('counter-amount') || /Nothing is outstanding|nothing outstanding/i.test(document.querySelector('main').innerText)`))
  await b.viewport(375, 812, true)
  await b.go('/accounts/fees'); await b.waitFor(`!!document.getElementById('counter-search')`)
  await setValue(`document.getElementById('counter-search')`, target.admission_no)
  await b.waitFor(`!!document.querySelector('#counter-results [role=option]')`)
  await b.evaluate(`document.querySelector('#counter-results [role=option]').click()`); await sleep(1200)
  check('counter at 375px: no sideways scroll', !(await b.overflow()))
  await b.shot('counter-375', true)
  await b.viewport(1366, 900)

  // ---------- 3. quick attendance (teacher, phone) ----------
  await b.login(email('teacher'), pw)
  await b.viewport(375, 812, true)
  await b.go('/admin/attendance/students')
  check('the attendance page links to quick attendance', await b.waitFor(`[...document.querySelectorAll('main a')].some(a => a.innerText.trim() === 'Quick attendance')`))
  // A section of the teacher's with nothing marked today.
  const opts = (await b.api('/analytics/options')).body.data
  let sevenA = null
  for (const s of (await b.api(`/attendance/students/sections?session_id=${opts.session.id}`)).body.data) {
    const items = (await b.api(`/attendance/students/roster?session_id=${opts.session.id}&section_id=${s.id}&date=${opts.today}&page_size=100`)).body.data.items
    if (items.length >= 4 && items.every(i => !i.status)) { sevenA = s; break }
  }
  check('demo data: a section with no attendance today', !!sevenA)
  const sectionLabel = `${sevenA.class_name} · ${sevenA.name}`
  await click('Quick attendance')
  await heading('Quick attendance')
  await choose('Class and section', sectionLabel)
  await b.waitFor(`document.querySelectorAll('main li button').length > 0`)
  const rows = () => b.evaluate(`[...document.querySelectorAll('main li button')].map(e => e.innerText.split('\\n').map(s => s.trim()).filter(Boolean).pop())`)
  const initial = await rows()
  check('everyone starts present', initial.length >= 3 && initial.every(s => s === 'Present'), initial.join(','))
  check('the unsaved state is clear before saving', (await b.text()).includes(`${initial.length} not saved yet`))
  check('rows are large touch targets (≥ 56px)', await b.evaluate(`[...document.querySelectorAll('main li button')].every(e => e.getBoundingClientRect().height >= 56)`))
  check('quick attendance at 375px: no sideways scroll', !(await b.overflow()))
  await b.evaluate(`document.querySelectorAll('main li button')[1].click()`)
  await b.evaluate(`(() => { const e = document.querySelectorAll('main li button')[2]; e.click(); e.click() })()`)
  await sleep(200)
  const marked = await rows()
  check('a tap marks absent, a second tap late', marked[1] === 'Absent' && marked[2] === 'Late' && marked[0] === 'Present', marked.slice(0, 3).join(','))
  check('the counts follow', (await b.text()).includes(`${initial.length - 2} present`) && (await b.text()).includes('1 absent') && (await b.text()).includes('1 late'))
  await b.shot('quick-attendance-375', true)
  // Another device records a mark first: the save is refused and that mark kept.
  const server = async () => (await b.api(`/attendance/students/roster?session_id=${opts.session.id}&section_id=${sevenA.id}&date=${opts.today}&page_size=100`)).body.data.items
  const fresh = await server()
  await b.api('/attendance/students/roster', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ session_id: opts.session.id, section_id: sevenA.id, attendance_date: opts.today, entries: [{ student_id: fresh[3].student_id, status: 'late', note: '', record_id: null, version: '' }] }) })
  await b.evaluate(`[...document.querySelectorAll('main button')].find(e => /^Save \\(/.test(e.innerText.trim())).click()`)
  check('a mark made elsewhere is reported', await b.waitFor(`/recorded elsewhere|Changed elsewhere|Nothing was saved/i.test(document.querySelector('main').innerText)`))
  check('nothing was overwritten', (await server()).filter(s => s.status).length === 1)
  await click('Reload roster')
  await b.waitFor(`!document.querySelector('main [role=alert]')`); await sleep(800)
  const reloaded = await rows()
  check('reloading keeps this device\'s marks and shows the other one', reloaded[1] === 'Absent' && reloaded[2] === 'Late' && reloaded[3] === 'Late', reloaded.slice(0, 4).join(','))
  await b.evaluate(`[...document.querySelectorAll('main button')].find(e => /^Save \\(/.test(e.innerText.trim())).click()`)
  check('one Save records the class', await b.waitFor(`/Attendance saved for \\d+ students/.test(document.querySelector('main').innerText)`))
  const saved = await server()
  check('the server holds present by default, the absent and both late marks', saved.every(s => s.status) && saved[1].status === 'absent' && saved[2].status === 'late' && saved[3].status === 'late' && saved.filter(s => s.status === 'present').length === saved.length - 3, saved.map(s => s.status).join(','))
  check('after saving: "All saved"', (await b.text()).includes('All saved'))
  await b.go('/admin/attendance/students/quick'); await heading('Quick attendance')
  await choose('Class and section', sectionLabel); await sleep(1200)
  check('a reload shows the saved marks', (await rows())[1] === 'Absent' && (await b.text()).includes('All saved'))
  await b.viewport(1366, 900)
} catch (e) { check('script error', false, e.stack) } finally {
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  const csp = b.consoleErrors.filter(e => /Content Security Policy|Refused to/i.test(e))
  check('no Content-Security-Policy violations', csp.length === 0, csp.slice(0, 3).join(' | '))
  r.save(join(here, 'shots', 'daily.json')); b.close()
}
