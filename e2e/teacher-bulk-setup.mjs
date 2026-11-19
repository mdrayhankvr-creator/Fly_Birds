// Mutates only the runner's newly seeded disposable database on 127.0.0.1:15174.
import { launch, reporter } from './cdp.mjs'

const base = process.env.TEACHER_SETUP_E2E_BASE_URL
const password = process.env.DEMO_PASSWORD
const url = base && new URL(base)
if (!url || url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.port !== '15174' || url.pathname !== '/' || !password) {
  throw new Error('Use the disposable local teacher setup runner on 127.0.0.1:15174')
}
const report = reporter()
const check = (name, ok) => report.check(name, ok)
const browser = await launch({ base, shots: process.env.E2E_SHOTS_DIR })
try {
  await browser.viewport(375, 812, true)
  await browser.login('demo.admin@demo.flybirds.test', password)
  const response = await browser.api('/staff/synthetic-setup/preview')
  const plan = response.body.data
  check('Admin preview counts existing staff and creates only the shortfall', response.status === 200 && plan.existing > 0 && plan.to_add === 15 - plan.existing && plan.candidates.length === plan.to_add)
  check('candidates have TEST names and unique identifiers', plan.candidates.every(item => item.test_marker === 'TEST' && item.name_en.startsWith('TEST ') && item.name_bn.startsWith('টেস্ট ') && item.role === 'teacher') && new Set(plan.candidates.map(item => item.employee_code)).size === plan.to_add)
  await browser.go('/admin/staff/synthetic-setup')
  check('Admin page opens at phone width', await browser.waitFor(`!!document.querySelector('[data-teacher-preview]')`, 15000) && !(await browser.overflow()))
  await browser.evaluate(`document.querySelector('[data-teacher-preview]').click()`)
  check('preview shows the shortfall before apply', await browser.waitFor(`document.querySelectorAll('[data-teacher-candidates] > *').length === ${plan.to_add} && !!document.querySelector('[data-teacher-apply]')`, 15000))
  const key = await browser.evaluate(`document.querySelector('[data-teacher-apply]').dataset.teacherKey`)
  check('retry key is stable in the current preview', /^[0-9a-f-]{36}$/i.test(key))
  check('apply requires a separate passphrase', await browser.evaluate(`document.querySelector('[data-teacher-apply]').disabled && document.querySelectorAll('input[type=password]').length >= 2`))
  await browser.evaluate(`(() => {
    const blobs = new Map(), create = URL.createObjectURL, click = HTMLAnchorElement.prototype.click
    window.__teacherDownloads = []
    URL.createObjectURL = function(blob) { const url = create.call(URL, blob); blobs.set(url, blob); return url }
    HTMLAnchorElement.prototype.click = function() {
      if (this.download?.startsWith('TEST-teacher-credentials')) { window.__teacherDownloads.push({ name: this.download, blob: blobs.get(this.href) }); return }
      return click.call(this)
    }
    window.__teacherPassphrase = Array.from(crypto.getRandomValues(new Uint8Array(32)), byte => String.fromCharCode(65 + byte % 26)).join('')
    for (const input of [...document.querySelectorAll('input[type=password]')].slice(0, 2)) {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, window.__teacherPassphrase)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }
  })()`)
  check('passphrase enables apply', await browser.waitFor(`document.querySelector('[data-teacher-apply]')?.disabled === false`, 5000))
  await browser.evaluate(`document.querySelector('[data-teacher-apply]').click()`)
  check('explicit confirmation appears', await browser.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000))
  await browser.evaluate(`document.querySelector('[role=alertdialog] button.btn-secondary').click()`)
  check('cancel creates no teachers', (await browser.api('/staff/synthetic-setup/preview')).body.data.existing === plan.existing)
  await browser.evaluate(`document.querySelector('[data-teacher-apply]').click()`)
  check('confirmation appears again', await browser.waitFor(`!!document.querySelector('[role=alertdialog]')`, 5000))
  await browser.evaluate(`document.querySelector('[role=alertdialog] button.btn-primary').click()`)
  check('encrypted one-time download is offered', await browser.waitFor(`window.__teacherDownloads?.length === 1 && !!document.querySelector('[data-teacher-download]')`, 90000))
  check('file decrypts locally and contains every new login', await browser.evaluate(`(async () => {
    const { name, blob } = window.__teacherDownloads[0]
    const bytes = new Uint8Array(await blob.arrayBuffer())
    const encryptedText = new TextDecoder().decode(bytes)
    const { decryptCredentialList } = await import('/src/pages/studentCredentialExport.ts')
    const csv = await decryptCredentialList(blob, window.__teacherPassphrase)
    return name.endsWith('.fbcred') && encryptedText.startsWith('FLYBIRDS-CREDENTIALS-V1') && !encryptedText.includes('TEACHER-') &&
      csv.includes('Employee code') && ${JSON.stringify(plan.candidates.map(item => item.login_id))}.every(id => csv.includes(id))
  })()`))
  const retry = await browser.api('/staff/synthetic-setup', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ fingerprint: plan.fingerprint, idempotency_key: key, confirm: true }) })
  check('retry makes no more teachers and returns no passwords', retry.status === 200 && retry.body.data?.replayed === true && retry.body.data?.credentials.length === 0 && (await browser.api('/staff/synthetic-setup/preview')).body.data.existing === 15)
  check('mobile success view has no sideways scroll', !(await browser.overflow()))
  const firstCredential = await browser.evaluate(`(async () => {
    const { decryptCredentialList } = await import('/src/pages/studentCredentialExport.ts')
    const csv = await decryptCredentialList(window.__teacherDownloads[0].blob, window.__teacherPassphrase)
    const cells = csv.trim().split(/\\r?\\n/)[1].split(',').map(cell => cell.slice(1, -1))
    return { login: cells[4], password: cells[5] }
  })()`)
  await browser.login(firstCredential.login, firstCredential.password)
  await browser.go('/dashboard')
  check('new teacher must change the temporary password at first sign-in', await browser.waitFor(`!!document.querySelector('#forced-title')`, 15000))
  await browser.login('demo.teacher@demo.flybirds.test', password)
  const denied = await browser.api('/staff/synthetic-setup/preview')
  check('teacher cannot access Admin setup preview', denied.status === 403)
} catch { check('script error', false) }
finally {
  check('no browser JavaScript errors', browser.errors.length === 0)
  report.save(process.env.E2E_RESULTS || 'e2e/shots/teacher-bulk-setup/results.json')
  await browser.close()
}
