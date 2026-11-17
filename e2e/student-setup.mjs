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
  check('planned bilingual names and identifiers are explicitly synthetic', plan.students.every(student => student.test_marker === 'TEST' && student.name_en.startsWith('TEST ') && student.name_bn.startsWith('টেস্ট ') && ['male', 'female'].includes(student.gender) && student.login_id === student.admission_no))
  check('planned names are diverse', new Set(plan.students.map(student => student.name_en)).size === plan.students.length)
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
  check('credential download requires an export passphrase', await b.evaluate(`document.querySelector('[data-setup-download]')?.disabled === true && !!document.querySelector('input[type=password]')`))
  const firstCredential = await b.evaluate(`({ id: document.querySelector('[data-login-id]')?.innerText.trim(), password: document.querySelector('[data-temporary-password]')?.innerText.trim() })`)
  check('credential slip contains a supported sign-in ID', !!firstCredential.id && !!firstCredential.password)
  await b.evaluate(`(() => {
    const blobs = new Map(), create = URL.createObjectURL, click = HTMLAnchorElement.prototype.click
    window.__setupDownloads = []
    URL.createObjectURL = function(blob) { const url = create.call(URL, blob); blobs.set(url, blob); return url }
    HTMLAnchorElement.prototype.click = function() {
      if (this.download?.startsWith('TEST-student-credentials')) {
        window.__setupDownloads.push({ name: this.download, blob: blobs.get(this.href) }); return
      }
      return click.call(this)
    }
    window.__setupPassphrase = Array.from(crypto.getRandomValues(new Uint8Array(32)), byte => String.fromCharCode(65 + byte % 26)).join('')
    const input = document.querySelector('input[type=password]')
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, window.__setupPassphrase)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  check('passphrase enables encrypted download', await b.waitFor(`document.querySelector('[data-setup-download]')?.disabled === false`, 5000))
  await b.evaluate(`document.querySelector('[data-setup-download]').click()`)
  check('one-time export is an encrypted file with no readable password', await b.waitFor(`window.__setupDownloads?.length === 1`, 15000) && await b.evaluate(`(async () => {
    const { name, blob } = window.__setupDownloads[0], bytes = new Uint8Array(await blob.arrayBuffer())
    return name.endsWith('.fbcred') && new TextDecoder().decode(bytes.slice(0, 24)).startsWith('FLYBIRDS-CREDENTIALS-V1') &&
      !new TextDecoder().decode(bytes).includes(document.querySelector('[data-temporary-password]').innerText.trim())
  })()`))
  await b.evaluate(`(() => {
    const saved = window.__setupDownloads[0]
    const transfer = new DataTransfer(); transfer.items.add(new File([saved.blob], saved.name))
    const file = document.querySelector('input[type=file]'); file.files = transfer.files; file.dispatchEvent(new Event('change', { bubbles: true }))
    const passphrase = document.querySelectorAll('input[type=password]')[1]
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(passphrase, window.__setupPassphrase)
    passphrase.dispatchEvent(new Event('input', { bubbles: true }))
  })()`)
  check('saved file can be opened locally', await b.waitFor(`[...document.querySelectorAll('button')].some(button => button.textContent.includes('Unlock and download CSV') && !button.disabled)`, 5000))
  await b.clickText('Unlock and download CSV')
  check('unlock restores the full credential CSV without sending it to the server', await b.waitFor(`window.__setupDownloads?.length === 2`, 15000) && await b.evaluate(`(async () => {
    const { name, blob } = window.__setupDownloads[1], csv = await blob.text()
    return name.endsWith('.csv') && csv.includes('Academic session') && csv.includes('Student (BN)') &&
      csv.includes(document.querySelector('[data-temporary-password]').innerText.trim())
  })()`))
  const firstStudent = (await b.api(`/students?section_id=${plan.sections[0].section_id}&page=1&page_size=100`)).body.data.items.find(item => item.name_en.startsWith('TEST '))
  check('created profile has paired name, gender and no invented contacts', !!firstStudent && firstStudent.name_bn.startsWith('টেস্ট ') && ['male', 'female'].includes(firstStudent.gender) && !firstStudent.present_address && !firstStudent.father_name_en && !firstStudent.mother_name_en)
  await b.go(`/admin/students/${firstStudent.id}`)
  check('student profile shows its TEST indicator', await b.waitFor(`document.body.innerText.includes('TEST record')`, 15000))
  await b.go('/admin/students/setup')
  check('lost export recovery UI is available', await b.waitFor(`document.body.innerText.includes('Open a saved credential file') && document.body.innerText.includes('Open user accounts')`, 15000))
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
