// Run against a fresh demo school after the other suites: see e2e/README.md.
// Checkpoint 5 in the browser:
//   1. One-click login on a student's profile, with its slip, and signing in
//      with the Student ID (which leads to the forced password change).
//   2. Logins for a whole section, printed eight to an A4 page.
//   3. Bulk admission from a spreadsheet: preview with problems, admit,
//      re-upload skips everyone, Bangla names survive.
//   4. The public school page below the sign-in form: sections, gallery
//      lightbox, public notices only, nothing personal, sign-in first.
import { launch, sleep, reporter } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readFileSync, writeFileSync, mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
const here = dirname(fileURLToPath(import.meta.url))
const pw = process.env.DEMO_PASSWORD
if (!pw) throw new Error('set DEMO_PASSWORD')
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:5173'
const r = reporter(), check = r.check
const b = await launch({ base, shots: join(here, 'shots', 'checkpoint5') })
const email = who => `demo.${who}@demo.flybirds.test`
const js = JSON.stringify
const stamp = Date.now().toString().slice(-6)
const AXE = readFileSync(join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js'), 'utf8')
const mainText = () => b.evaluate(`document.querySelector('main')?.innerText ?? document.body.innerText`)
const setValue = (selector, value) => b.evaluate(`(() => { const f = ${selector}; if (!f) return false; const proto = f instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, 'value').set.call(f, ${js(value)}); f.dispatchEvent(new Event(f instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true })); return true })()`)
const confirmDialog = label => b.evaluate(`(() => { const e = [...document.querySelectorAll('[role=alertdialog] button')].find(x => x.innerText.trim() === ${js(label)}); if (!e) return false; e.click(); return true })()`)
async function axe(scope) {
  if (!(await b.evaluate('typeof axe !== "undefined"'))) await b.evaluate(AXE)
  return b.evaluate(`axe.run(${scope}, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(v => v.id + ':' + v.nodes.length))`)
}
async function setFile(selector, path) {
  await b.send('DOM.enable')
  const { root } = await b.send('DOM.getDocument', { depth: -1 })
  const { nodeId } = await b.send('DOM.querySelector', { nodeId: root.nodeId, selector })
  await b.send('DOM.setFileInputFiles', { nodeId, files: [path] })
}
const dir = mkdtempSync(join(tmpdir(), 'fb-cp5-'))

try {
  await b.viewport(1366, 900)
  await b.login(email('admin'), pw)

  // ---------- 1. one-click login on the profile ----------
  const pupil = (await b.api('/students?page=1&page_size=1&status=active&without_login=true')).body.data.items[0]
  check('a demo student without a login exists', !!pupil, JSON.stringify(pupil ?? {}).slice(0, 120))
  await b.go(`/admin/students/${pupil.id}`)
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Create login')`, 15000)
  await b.clickText('Create login', 'main button')
  check('Create login shows a slip with the sign-in ID and a password', await b.waitFor(`!!document.querySelector('main [data-login-id]') && !!document.querySelector('main [data-temporary-password]')`, 15000))
  const loginId = await b.evaluate(`document.querySelector('main [data-login-id]').innerText.trim()`)
  const temporary = await b.evaluate(`document.querySelector('main [data-temporary-password]').innerText.trim()`)
  check('the sign-in ID is the Student ID or admission number', loginId === (pupil.student_code || pupil.admission_no) || loginId === pupil.admission_no, `${loginId} vs ${pupil.student_code}/${pupil.admission_no}`)
  const useBy = new Date(Date.now() + 14 * 864e5).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' })
  const expiryText = await b.evaluate(`document.querySelector('main [data-password-expires]')?.innerText ?? ''`)
  check('the slip says the password must be used within 14 days, in both languages', expiryText.includes(useBy) && /ব্যবহারের শেষ তারিখ/.test(await mainText()), `${expiryText} vs ${useBy}`)
  check('the slip has English and Bangla labels and a print button', /Sign-in ID/.test(await mainText()) && /সাইন-ইন আইডি/.test(await mainText()) && await b.evaluate(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Print slip')`))
  await b.shot('create-login-slip', true)
  await b.go(`/admin/students/${pupil.id}`)
  check('the profile then shows the sign-in ID instead of the button', await b.waitFor(`document.querySelector('main').innerText.includes(${js(loginId)}) && ![...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Create login')`, 15000))

  // Sign in with the Student ID through the form on the landing page.
  await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }); localStorage.setItem('flybirds.language', 'en') })()`)
  await b.go('/')
  await b.waitFor(`!!document.querySelector('#login-email')`, 15000)
  check('the sign-in field reads "Email or Student ID"', /Email or Student ID/.test(await b.evaluate(`document.querySelector('label[for=login-email]').innerText`)) && await b.evaluate(`document.querySelector('#login-email').type === 'text'`))
  await setValue(`document.querySelector('#login-email')`, loginId.toLowerCase()); await setValue(`document.querySelector('#login-password')`, temporary)
  await b.evaluate(`document.querySelector('form[aria-labelledby=sign-in-title] button[type=submit]').click()`)
  check('the Student ID signs in (any case) and asks for a new password', await b.waitFor(`document.querySelector('main h1')?.innerText === 'Choose your own password'`, 15000))
  check('the forced-change page names the sign-in ID, not a placeholder address', !(await mainText()).includes('no-email.invalid') && (await mainText()).includes(loginId))
  await b.login(email('admin'), pw)
  const wrong = await b.evaluate(`fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ identifier: ${js(loginId)}, password: 'Wrong-password-123' }) }).then(async r => [r.status, await r.text()])`)
  const unknown = await b.evaluate(`fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ identifier: 'NOBODY-${stamp}', password: 'Wrong-password-123' }) }).then(async r => [r.status, await r.text()])`)
  check('a wrong password and an unknown ID get the same answer', wrong[0] === 401 && wrong[1] === unknown[1])
  await b.login(email('admin'), pw)

  // ---------- 2. logins for a whole section ----------
  const sections = (await b.api(`/students?page=1&page_size=100&status=active&without_login=true`)).body.data.items
  const target = sections.find(s => s.section_id !== pupil.section_id) ?? sections[0]
  await b.go('/admin/students')
  check('the Students page links to student logins and bulk admission', await b.waitFor(`!!document.querySelector('main a[href="/admin/students/logins"]') && !!document.querySelector('main a[href="/admin/students/import"]')`, 15000))
  await b.go('/admin/students/logins')
  await b.waitFor(`document.querySelectorAll('main select')[0]?.options.length > 1`, 15000)
  await setValue(`document.querySelectorAll('main select')[0]`, target.session_id)
  await setValue(`document.querySelectorAll('main select')[1]`, target.class_id)
  await b.waitFor(`[...document.querySelectorAll('main select')[2].options].some(o => o.value === ${js(target.section_id)})`)
  await setValue(`document.querySelectorAll('main select')[2]`, target.section_id)
  check('the page says how many students have no login', await b.waitFor(`!!document.querySelector('[data-without-count]')`, 15000))
  const without = Number(await b.evaluate(`document.querySelector('[data-without-count]').dataset.withoutCount`))
  await b.clickText('Give logins to all students without one', 'main button')
  check('giving logins asks first', await b.waitFor(`/Create logins? for/.test(document.querySelector('[role=alertdialog]')?.innerText ?? '')`))
  await confirmDialog('Give logins to all students without one')
  check('one slip per student appears, passwords shown once', await b.waitFor(`document.querySelectorAll('main .login-slip').length === ${without}`, 30000), `${without} expected`)
  check('the print sheet holds the same slips, two across', await b.evaluate(`document.querySelectorAll('.login-slips-print .login-slip').length === ${without}`))
  const printLayout = await b.evaluate(`(() => { const s = [...document.styleSheets].flatMap(x => { try { return [...x.cssRules] } catch { return [] } }).map(r => r.cssText).join(' '); return /login-slips-print[^}]*grid-template-columns: 1fr 1fr/.test(s) })()`)
  check('slips print two across on A4', printLayout)
  await b.shot('section-logins', true)
  const audit = (await b.api('/audit/events?page=1&page_size=20&action=students.logins_created')).body.data
  check('the section run is audited without passwords', JSON.stringify(audit).includes(target.section_id) && !JSON.stringify(audit).includes(temporary))

  // ---------- 3. bulk admission ----------
  const cls = (await b.api(`/academic/classes?page=1&page_size=100`)).body.data.items.find(c => c.id === target.class_id)
  const sec = (await b.api(`/academic/sections?page=1&page_size=100&session_id=${target.session_id}&class_id=${target.class_id}`)).body.data.items.find(s => s.id === target.section_id)
  const head = 'admission_no,name_en,name_bn,class,section,date_of_birth,guardian_name,guardian_phone'
  // Siblings: the same new guardian (same phone) on every row.
  const rows = [1, 2, 3].map(n => `BULK-${stamp}-${n},Bulk Pupil ${n},বাল্ক শিক্ষার্থী ${n},${cls.name},${sec.name},2014-0${n}-1${n},Bulk Parent ${stamp},0191${stamp}7`)
  const badRow = `BULK-${stamp}-9,Bad Row,,No Such Class,A,31/02/2014,,`
  const bad = join(dir, 'with-problems.csv'), good = join(dir, 'students.csv')
  writeFileSync(bad, '﻿' + [head, ...rows, badRow].join('\n') + '\n')
  writeFileSync(good, '﻿' + [head, ...rows].join('\n') + '\n')
  await b.go('/admin/students/import')
  check('the bulk page offers both templates', await b.waitFor(`/Template \\(.xlsx\\)/.test(document.querySelector('main').innerText) && /Template \\(CSV\\)/.test(document.querySelector('main').innerText)`, 15000))
  const template = await b.api('/students/bulk-admission/template?format=csv')
  check('the CSV template downloads with Bangla guidance', template.status === 200 && template.body.includes('admission_no,name_en') && template.body.includes('ভর্তি'))
  await b.waitFor(`document.querySelector('main select')?.value !== ''`)
  await setValue(`document.querySelector('main select')`, target.session_id)
  await setFile('main input[type=file]', bad)
  check('the preview lists every row with its problems', await b.waitFor(`document.querySelectorAll('[data-bulk-line]').length === 4`, 20000))
  const badText = await mainText()
  check('problems are spelled out per row', /No class with this name/.test(badText) && /Date of birth is not a valid/.test(badText) && /1 row needs fixing/.test(badText))
  check('nothing can be admitted while a row has problems', !(await b.evaluate(`[...document.querySelectorAll('main button')].some(e => /^Admit \\d+ student/.test(e.innerText.trim()))`)))
  check('the sibling rows share one new guardian', /as row 2/.test(badText))
  await b.shot('bulk-preview-problems', true)
  await setFile('main input[type=file]', good)
  await b.waitFor(`document.querySelectorAll('[data-bulk-line]').length === 3 && [...document.querySelectorAll('[data-bulk-status]')].every(e => e.dataset.bulkStatus === 'ready')`, 20000)
  check('the Bangla names survive the upload', (await mainText()).includes('বাল্ক শিক্ষার্থী 1'))
  await b.evaluate(`[...document.querySelectorAll('main input[type=checkbox]')].find(e => /login/i.test(e.closest('label')?.innerText ?? ''))?.click()`)
  await b.clickText('Admit 3 students', 'main button')
  await b.waitFor(`!!document.querySelector('[role=alertdialog]')`)
  await confirmDialog('Admit 3 students')
  check('all three are admitted, with login slips', await b.waitFor(`/3 admitted/.test(document.querySelector('main').innerText) && document.querySelectorAll('main .login-slip').length === 3`, 30000))
  await b.shot('bulk-admitted', true)
  const stored = (await b.api(`/students?page=1&page_size=10&search=BULK-${stamp}`)).body.data
  check('the students are on record with their Bangla names', stored.total === 3 && stored.items.some(s => s.name_bn === 'বাল্ক শিক্ষার্থী 2'))
  await setFile('main input[type=file]', good)
  check('uploading the same file again skips everyone', await b.waitFor(`document.querySelectorAll('[data-bulk-status=skip]').length === 3 && /already admitted/.test(document.querySelector('main').innerText)`, 20000))
  check('bulk page: no accessibility violations', (await axe(`document.querySelector('main')`)).length === 0, (await axe(`document.querySelector('main')`)).join(','))

  // ---------- 4. the public school page ----------
  await b.go('/admin/school-settings')
  check('School Settings has the public page editor with the consent reminder', await b.waitFor(`/Public school page/.test(document.querySelector('main').innerText) && /guardians’ consent/.test(document.querySelector('main').innerText)`, 15000))
  const page = { about_en: `Green Valley teaches every child. ${stamp}`, about_bn: 'প্রতিটি শিশুর জন্য শিক্ষা।', founded_year: 1985, eiin: '123456', show_counts: true, head_name_en: 'Ayesha Rahman', head_name_bn: 'আয়েশা রহমান', head_message_en: 'Welcome to our school.', head_message_bn: 'আমাদের বিদ্যালয়ে স্বাগতম।', contact_address_en: 'Main Road, Sylhet', contact_address_bn: 'প্রধান সড়ক, সিলেট', contact_phone: '+880 1711-000000', contact_email: 'office@demo.flybirds.test', map_url: 'https://maps.example.com/?q=green-valley' }
  check('the administrator saves the public page', (await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: js(page) })).status === 200)
  const settings = (await b.api('/school/settings')).body.data
  check('the EIIN and school details are school settings', settings.eiin === '123456' && settings.founded_year === 1985 && settings.head_teacher_name_en === 'Ayesha Rahman' && settings.address_bn === 'প্রধান সড়ক, সিলেট' && settings.map_url === page.map_url, JSON.stringify(settings).slice(0, 300))
  const photo = await b.evaluate(`(async () => { const c = document.createElement('canvas'); c.width = 1200; c.height = 800; const x = c.getContext('2d'); const g = x.createLinearGradient(0, 0, 1200, 800); g.addColorStop(0, '#0f766e'); g.addColorStop(1, '#f59e0b'); x.fillStyle = g; x.fillRect(0, 0, 1200, 800); const blob = await new Promise(r => c.toBlob(r, 'image/jpeg', 0.9)); const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1]); const tok = (await (await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })).json()).data.access_token; const res = await fetch('/api/school/gallery', { method: 'POST', headers: { Authorization: 'Bearer ' + tok, 'Content-Type': 'image/jpeg' }, body: blob }); const body = await res.json(); await fetch('/api/school/gallery/' + body.data.id, { method: 'PATCH', headers: { Authorization: 'Bearer ' + tok, 'Content-Type': 'application/json' }, body: JSON.stringify({ caption_en: 'Our playground', caption_bn: 'আমাদের খেলার মাঠ' }) }); return res.status })()`)
  check('a gallery photo is added', photo === 201)
  const notice = async (title, show) => {
    const saved = (await b.api('/notices', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ title_en: title, title_bn: '', body_en: `Text of ${title}`, body_bn: '', priority: 'normal', audience_roles: ['guardian'], targets: [], expires_at: null, file_ids: [], show_public: show }) })).body.data
    await b.api(`/notices/${saved.id}/publish`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
    return saved
  }
  const pub = await notice(`Sports day ${stamp}`, true)
  await notice(`Staff only ${stamp}`, false)
  check('the notice keeps its public flag', pub.show_public === true)
  await b.go('/notices')
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Manage notices')`, 15000)
  await b.clickText('Manage notices', 'main button')
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'New notice')`, 15000)
  await b.clickText('New notice', 'main button')
  check('the notice form offers "Show on the public school page", off by default', await b.waitFor(`(() => { const l = [...document.querySelectorAll('main label')].find(x => /Show on the public school page/.test(x.innerText)); return !!l && !l.querySelector('input').checked })()`, 15000))

  await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }) })()`)
  await b.go('/')
  check('the sign-in form is on the first screen', await b.waitFor(`(() => { const f = document.querySelector('#login-email'); return f && f.getBoundingClientRect().bottom < window.innerHeight })()`, 15000))
  check('the public sections load below it', await b.waitFor(`!!document.querySelector('[data-public-page] #public-about') && !!document.querySelector('#public-head') && !!document.querySelector('#public-gallery') && !!document.querySelector('#public-notices') && !!document.querySelector('#public-contact')`, 15000))
  const landing = await b.text()
  check('about shows the founding year, EIIN and totals', landing.includes('1985') && landing.includes('123456') && /Students/i.test(landing) && /Teachers/i.test(landing))
  check('only the notice chosen for the public appears', landing.includes(`Sports day ${stamp}`) && !landing.includes(`Staff only ${stamp}`))
  check('contact uses a map link, not an embedded map', await b.evaluate(`!!document.querySelector('#public-contact ~ * a[href^="https://maps.example.com"], [data-public-page] a[href^="https://maps.example.com"]') && !document.querySelector('[data-public-page] iframe')`))
  check('nothing personal is on the public page', !landing.includes(pupil.name_en) && !landing.includes(pupil.admission_no) && !landing.includes('Bulk Pupil') && !/GPA|Total due/.test(landing))
  check('gallery thumbnails load lazily', await b.evaluate(`[...document.querySelectorAll('#public-gallery ~ * img, [data-public-page] ul img')].every(i => i.loading === 'lazy' && i.src.includes('size=thumb'))`))
  // The demo seed has its own gallery photos; open the one added here.
  await b.evaluate(`[...document.querySelectorAll('[data-public-page] ul button')].find(x => /Our playground/.test(x.getAttribute('aria-label'))).click()`)
  check('a photo opens in a lightbox with its caption', await b.waitFor(`!!document.querySelector('[role=dialog][aria-modal=true] img') && /Our playground/.test(document.querySelector('[role=dialog]').innerText)`))
  await b.key('Escape', 'Escape', 27)
  check('Escape closes the lightbox', await b.waitFor(`!document.querySelector('[role=dialog][aria-modal=true]')`))
  await b.evaluate(`document.querySelector('[data-public-page]').scrollIntoView()`)
  check('public page: no accessibility violations', (await axe(`document.querySelector('[data-public-page]')`)).length === 0, (await axe(`document.querySelector('[data-public-page]')`)).join(','))
  await b.shot('public-page', true)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'bn')`); await b.go('/')
  check('the public page reads in Bangla', await b.waitFor(`/বিদ্যালয় পরিচিতি/.test(document.body.innerText) && document.body.innerText.includes('আয়েশা রহমান')`, 15000))
  await b.viewport(375, 812, true); await b.go('/'); await b.waitFor(`!!document.querySelector('[data-public-page]')`, 15000); await sleep(800)
  check('public page at 375px: no sideways scroll', !(await b.overflow()))
  check('the sign-in form comes before the public sections on phones', await b.evaluate(`document.querySelector('#login-email').getBoundingClientRect().top < document.querySelector('[data-public-page]').getBoundingClientRect().top`))
  const footerText = () => b.evaluate(`document.querySelector('[data-public-footer]')?.innerText ?? ''`)
  await b.waitFor(`!!document.querySelector('[data-public-footer] a[href^="tel:"]')`, 15000)
  const bnFooter = await footerText()
  check('footer in Bangla at 375px: school, address, EIIN, contacts and links', ['গ্রিন ভ্যালি ডেমো স্কুল', 'প্রধান সড়ক, সিলেট', 'EIIN 123456', 'office@demo.flybirds.test', 'সাইন ইন', 'পাবলিক নোটিশ', 'যোগাযোগ', 'Fly_Birds দ্বারা পরিচালিত'].every(x => bnFooter.includes(x)) && /© [০-৯]{4} গ্রিন ভ্যালি ডেমো স্কুল/.test(bnFooter), bnFooter.replace(/\n/g, ' | '))
  await b.evaluate(`document.querySelector('[data-public-footer]').scrollIntoView()`); await sleep(300)
  check('footer at 375px: no sideways scroll', !(await b.overflow()))
  await b.shot('footer-bn-375', true)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
  await b.viewport(1366, 900); await b.go('/'); await b.waitFor(`!!document.querySelector('[data-public-footer] a[href^="tel:"]') && !!document.querySelector('#public-notices')`, 15000)
  const enFooter = await footerText()
  const year = new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Dhaka', year: 'numeric' }).format(new Date())
  check('footer: school, address, EIIN, phone, email, links, copyright and credit', ['Green Valley Demo School', 'Main Road, Sylhet', 'EIIN 123456', '+880 1711-000000', 'office@demo.flybirds.test', 'Sign in', 'Public notices', 'Contact', `© ${year} Green Valley Demo School`, 'Powered by Fly_Birds'].every(x => enFooter.includes(x)), enFooter.replace(/\n/g, ' | '))
  check('footer contacts are tel: and mailto: links', await b.evaluate(`!!document.querySelector('[data-public-footer] a[href="tel:+8801711000000"]') && !!document.querySelector('[data-public-footer] a[href="mailto:office@demo.flybirds.test"]')`))
  await b.evaluate(`[...document.querySelectorAll('[data-public-footer] a')].find(a => a.innerText.trim() === 'Public notices').click()`)
  check('"Public notices" in the footer scrolls to the notices', await b.waitFor(`Math.abs(document.querySelector('#public-notices').getBoundingClientRect().top) < 120`, 5000))
  await b.evaluate(`[...document.querySelectorAll('[data-public-footer] a')].find(a => a.innerText.trim() === 'Sign in').click()`)
  check('"Sign in" in the footer goes to the sign-in form', await b.waitFor(`document.activeElement?.id === 'login-email' && window.scrollY < 50`, 5000))
  check('footer: no accessibility violations (light)', (await axe(`document.querySelector('[data-public-footer]')`)).length === 0, (await axe(`document.querySelector('[data-public-footer]')`)).join(','))
  await b.evaluate(`localStorage.setItem('flybirds.theme', 'dark')`); await b.go('/'); await b.waitFor(`!!document.querySelector('[data-public-footer] a[href^="tel:"]')`, 15000)
  check('footer: no accessibility violations (dark)', await b.evaluate(`document.documentElement.dataset.theme === 'dark'`) && (await axe(`document.querySelector('[data-public-footer]')`)).length === 0, (await axe(`document.querySelector('[data-public-footer]')`)).join(','))
  await b.evaluate(`localStorage.removeItem('flybirds.theme')`)
  await b.go('/verify/not-a-real-token')
  check('the footer is on the other public pages too', await b.waitFor(`/Green Valley Demo School/.test(document.querySelector('[data-public-footer]')?.innerText ?? '') && !!document.querySelector('[data-public-footer] a[href^="mailto:"]')`, 15000))
  await b.evaluate(`[...document.querySelectorAll('[data-public-footer] a')].find(a => a.innerText.trim() === 'Contact').click()`)
  check('"Contact" from another page opens the landing page at the contact section', await b.waitFor(`location.pathname === '/' && !!document.querySelector('#public-contact') && Math.abs(document.querySelector('#public-contact').getBoundingClientRect().top) < 120`, 15000))
  // Empty details are left out of the footer.
  await b.login(email('admin'), pw)
  const bare = { ...page, eiin: '', contact_address_en: '', contact_address_bn: '', contact_phone: '', contact_email: '', map_url: '' }
  check('the details can be cleared', (await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: js(bare) })).status === 200)
  await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }) })()`)
  await b.go('/'); await b.waitFor(`!!document.querySelector('[data-public-page]') && !!document.querySelector('[data-public-footer]')`, 15000); await sleep(800)
  const bareFooter = await footerText()
  check('empty details are left out of the footer', bareFooter.includes('Green Valley Demo School') && !/EIIN|Main Road|@|Contact/.test(bareFooter) && !(await b.evaluate(`!!document.querySelector('[data-public-footer] a[href^="tel:"], [data-public-footer] a[href^="mailto:"], #footer-contact')`)), bareFooter.replace(/\n/g, ' | '))
  await b.login(email('admin'), pw)
  check('the details are restored', (await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: js(page) })).status === 200)
  await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)

  // ---------- 5. the phone top bar: every control has a name ----------
  for (const lang of ['en', 'bn']) {
    await b.login(email('admin'), pw, lang)
    await b.viewport(375, 812, true); await b.go('/dashboard'); await b.waitFor(`!!document.querySelector('header.shell-topbar')`, 15000); await sleep(800)
    const { nodes } = await b.send('Accessibility.getFullAXTree')
    const header = await b.evaluate(`[...document.querySelectorAll('header.shell-topbar a, header.shell-topbar button')].filter(e => e.offsetParent !== null).length`)
    const named = nodes.filter(n => !n.ignored && ['button', 'link'].includes(n.role?.value)).slice(0, 1 + header)
    const empty = named.filter(n => !(n.name?.value ?? '').trim()).length
    const names = named.map(n => n.name?.value ?? '')
    const want = lang === 'en' ? [/^Open menu$/, /^Home, /, /^Account menu for /] : [/^মেনু খুলুন$/, /^হোম, /, /-এর অ্যাকাউন্ট মেনু$/]
    check(`phone top bar (${lang}): the menu button, home link and account menu have names`, empty === 0 && want.every(w => names.some(n => w.test(n))), names.join(' | '))
  }
  await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
} catch (e) { check('script error', false, e.stack) } finally {
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  const csp = b.consoleErrors.filter(e => /Content Security Policy|Refused to/i.test(e))
  check('no Content-Security-Policy violations', csp.length === 0, csp.slice(0, 3).join(' | '))
  r.save(join(here, 'shots', 'checkpoint5.json')); b.close()
}
