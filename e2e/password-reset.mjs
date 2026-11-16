// "Forgot password" end to end, with a fake mail relay.
//
// A tiny SMTP server runs in this process and keeps whatever the backend
// sends it, so the test reads the emailed link exactly as a person would and
// then uses it in the browser. Nothing real is emailed and no real account is
// touched: the test makes a throwaway user of its own.
//
// The stack must be started with the relay pointed at this process, which is
// what deploy/m15b1.env does:
//
//   SMTP_HOST=host.docker.internal   SMTP_PORT=2525
//   SMTP_SECURITY=none               SMTP_FROM=noreply@m15b1.test
//   PUBLIC_BASE_URL=http://localhost:8121
//
//   DEMO_PASSWORD=...  E2E_BASE_URL=http://localhost:8121  node e2e/password-reset.mjs
//   SMTP_SINK_PORT=2525 changes the port; E2E_SHOTS=<folder> the screenshots.
import { createServer } from 'node:net'
import { join, dirname } from 'node:path'
import { mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const shots = process.env.E2E_SHOTS || join(here, 'shots', 'password-reset')
mkdirSync(shots, { recursive: true })
const pw = process.env.DEMO_PASSWORD
const port = Number(process.env.SMTP_SINK_PORT || 2525)
const r = reporter(), check = r.check
const email = who => `demo.${who}@demo.flybirds.test`
const js = JSON.stringify
// React reads the value through its own descriptor, so setting .value
// directly is not noticed; call the native setter, as the other suites do.
const fill = (selector, value) => b.evaluate(`(() => { const el = document.querySelector(${js(selector)}); if (!el) return false; el.focus(); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, ${js(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)

// ---------------------------------------------------------------------------
// A fake relay. Only what Go's net/smtp client needs, in the order it needs it.

const inbox = []
const sink = createServer(socket => {
  let buffer = ''
  let data = false
  let body = ''
  socket.setEncoding('utf8')
  socket.write('220 sink.test ESMTP ready\r\n')
  socket.on('data', chunk => {
    if (data) {
      body += chunk
      const end = body.indexOf('\r\n.\r\n')
      if (end < 0) return
      inbox.push(body.slice(0, end))
      body = ''
      data = false
      socket.write('250 2.0.0 queued\r\n')
      return
    }
    buffer += chunk
    let line
    while ((line = nextLine()) !== null) {
      const verb = line.split(' ')[0].toUpperCase()
      if (verb === 'EHLO') socket.write('250-sink.test\r\n250 8BITMIME\r\n')
      else if (verb === 'HELO') socket.write('250 sink.test\r\n')
      else if (verb === 'MAIL' || verb === 'RCPT' || verb === 'RSET' || verb === 'NOOP') socket.write('250 2.1.0 ok\r\n')
      else if (verb === 'DATA') { data = true; socket.write('354 end with <CRLF>.<CRLF>\r\n'); body = buffer; buffer = ''; return }
      else if (verb === 'QUIT') { socket.write('221 2.0.0 bye\r\n'); socket.end() }
      else socket.write('502 5.5.1 not implemented\r\n')
    }
  })
  socket.on('error', () => socket.destroy())
  function nextLine() {
    const cut = buffer.indexOf('\r\n')
    if (cut < 0) return null
    const out = buffer.slice(0, cut)
    buffer = buffer.slice(cut + 2)
    return out
  }
})
await new Promise((resolve, reject) => {
  sink.once('error', reject)
  sink.listen(port, '0.0.0.0', resolve)
})
console.log(`fake relay listening on 0.0.0.0:${port}`)

// waitForMail waits for one more message than we had, so each request's own
// message is the one that is read.
async function waitForMail(had, timeout = 20000) {
  const end = Date.now() + timeout
  while (Date.now() < end) {
    if (inbox.length > had) return inbox[inbox.length - 1]
    await sleep(200)
  }
  return ''
}

const b = await launch({ base: process.env.E2E_BASE_URL || 'http://localhost:8121', shots })
const must = async (promise, what) => {
  const res = await promise
  if (res.status >= 300) throw new Error(`${what}: ${res.status} ${JSON.stringify(res.body).slice(0, 200)}`)
  return res.body.data
}
const signIn = (identifier, password) => b.evaluate(`(async () => {
  const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || '')
  if (csrf) await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }).catch(() => null)
  const res = await fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ identifier: ${JSON.stringify(identifier)}, password: ${JSON.stringify(password)} }) })
  const body = await res.json().catch(() => ({}))
  return { status: res.status, code: body.error && body.error.code, must: !!(body.data && body.data.user && body.data.user.must_change_password) }
})()`)
const signOut = () => b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); if (csrf) await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }).catch(() => null); return true })()`)

try {
  // -------------------------------------------------------------------------
  // 1. The relay is configured, so the school offers the feature.

  await b.go('/login')
  const offered = await b.evaluate(`(async () => { const r = await fetch('/api/auth/password/forgot'); const body = await r.json(); return body.data && body.data.enabled === true })()`)
  check('the server reports that it can email a reset link', offered === true, String(offered))
  // The sign-in page asks this on every visit, so it must not share the
  // strict limit that protects sending email: if it did, the link would
  // vanish from the page as soon as a few people opened it.
  const repeated = await b.evaluate(`(async () => {
    const codes = []
    for (let i = 0; i < 12; i++) { const r = await fetch('/api/auth/password/forgot'); codes.push(r.status) }
    return codes.join(',')
  })()`)
  check('asking whether the feature exists is not rate limited', !repeated.includes('429'), repeated)

  // -------------------------------------------------------------------------
  // 2. A throwaway account to recover. A user an administrator creates starts
  //    with a temporary password it must replace, which the reset clears.

  const stamp = String(Date.now()).slice(-9)
  const mine = `reset.${stamp}@example.test`
  await b.login(email('admin'), pw)
  const created = await must(b.api('/users', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: mine, display_name: 'Reset Test Teacher', role: 'teacher', password: 'Throwaway-first-password-26!' }),
  }), 'create a throwaway user')
  check('a throwaway account exists to recover', !!created.id, `${mine} ${created.id || ''}`)
  // An administrator's reset leaves a temporary password the account must
  // replace. Recovering by email has to settle that too, not sit on top of it.
  await must(b.api(`/users/${created.id}/password-reset`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }), 'administrator reset')
  const forced = await signIn(mine, 'Throwaway-first-password-26!')
  check('the administrator reset left a password the account must replace', forced.status === 401, `status ${forced.status}`)
  await signOut()

  // -------------------------------------------------------------------------
  // 3. The sign-in page offers the link, and the office line stays.

  await b.go('/login')
  await b.waitFor(`document.querySelector('#login-email')`, 15000)
  const linkText = await b.evaluate(`(() => { const el = [...document.querySelectorAll('a[href="/forgot-password"]')].find(e => e.offsetParent !== null); return el ? el.innerText.trim() : '' })()`)
  check('the sign-in page offers "Forgot your password?"', linkText === 'Forgot your password?', linkText)
  const keepsOffice = await b.evaluate(`document.body.innerText.includes('Contact your school office')`)
  check('the school office is still named as the other way in', keepsOffice === true, String(keepsOffice))

  // -------------------------------------------------------------------------
  // 4. Asking for a link, in the browser, on a phone.

  await b.viewport(375, 760, true)
  await b.go('/forgot-password')
  await b.waitFor(`document.querySelector('#forgot-email')`, 15000)
  await b.shot('forgot-en-375')
  const had = inbox.length
  check('the address is typed into the form', (await fill('#forgot-email', mine)) === true)
  await b.clickText('Send the reset link')
  const confirmed = await b.waitFor(`document.body.innerText.includes('the link is on its way')`, 20000)
  check('the form confirms without saying whether the address has an account', confirmed === true)
  await b.shot('forgot-sent-375')

  // -------------------------------------------------------------------------
  // 5. The message: bilingual, addressed to the account, with one link.

  const message = await waitForMail(had)
  check('the relay received one message', message !== '', `${inbox.length} message(s)`)
  check('it is addressed to the account', message.includes(`To: ${mine}`), message.split('\r\n').find(l => l.startsWith('To:')) || '')
  check('the subject is an encoded word or plain ASCII', /\r?\nSubject: \S/.test(message) || message.startsWith('Subject:'), '')
  for (const [what, needle] of [['English', 'Choose a new password'], ['Bangla', 'নতুন পাসওয়ার্ড দিন'], ['the expiry', '60 minutes']]) {
    check(`the message carries ${what}`, message.includes(needle))
  }
  const links = [...message.matchAll(/http:\/\/localhost:8121\/reset-password\?token=([A-Za-z0-9_%-]+)/g)].map(m => m[1])
  const token = decodeURIComponent(links[0] || '')
  check('the message carries a reset link', token.length >= 40, `${links.length} occurrence(s), ${token.length} characters`)

  // An address with no account gets the same answer and no message.
  const before = inbox.length
  const silent = await b.evaluate(`(async () => {
    const r = await fetch('/api/auth/password/forgot', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'nobody.${stamp}@example.test' }) })
    return { status: r.status, body: await r.text() }
  })()`)
  await sleep(1500)
  check('an address with no account gets the same answer', silent.status === 202 && silent.body.includes('"sent":true'), `${silent.status} ${silent.body}`)
  check('and no message is sent for it', inbox.length === before, `${inbox.length - before} extra message(s)`)

  // -------------------------------------------------------------------------
  // 6. Using the link.

  await b.go(`/reset-password?token=${encodeURIComponent(token)}`)
  const form = await b.waitFor(`document.querySelector('#reset-password')`, 20000)
  check('the link opens the new-password form', form === true)
  const named = await b.evaluate(`document.body.innerText.includes('Reset Test Teacher')`)
  check('the form says whose account it is', named === true, String(named))
  await b.shot('reset-en-375')

  const fillBoth = async (one, two) => {
    await fill('#reset-password', one)
    await fill('#reset-password-again', two)
  }
  const chosen = 'Reset-chosen-password-2026!'
  await fillBoth(chosen, 'Something-else-entirely-26!')
  await b.clickText('Save the new password')
  const mismatch = await b.waitFor(`document.body.innerText.includes('The two passwords are not the same')`, 8000)
  check('two different passwords are refused', mismatch === true)

  await fillBoth(chosen, chosen)
  await b.clickText('Save the new password')
  const saved = await b.waitFor(`document.body.innerText.includes('Your new password is saved')`, 20000)
  check('the new password is saved and every device signed out', saved === true)
  await b.shot('reset-done-375')

  // -------------------------------------------------------------------------
  // 7. What the reset is worth: the password works, the forced change is
  //    gone, and the link does not work again.

  const now = await signIn(mine, chosen)
  check('the account signs in with the new password', now.status === 200, `status ${now.status} ${now.code || ''}`)
  check('and is not asked to change it again', now.must === false, String(now.must))
  await signOut()

  await b.go(`/reset-password?token=${encodeURIComponent(token)}`)
  const dead = await b.waitFor(`document.body.innerText.includes('This link no longer works')`, 20000)
  check('the same link does not work a second time', dead === true)
  await b.shot('reset-used-375')

  const nonsense = await b.evaluate(`(async () => { const r = await fetch('/api/auth/password/reset?token=not-a-token'); const body = await r.json(); return { status: r.status, code: body.error && body.error.code } })()`)
  check('a link that was never issued answers the same error', nonsense.status === 400 && nonsense.code === 'invalid_reset_token', `${nonsense.status} ${nonsense.code}`)

  // -------------------------------------------------------------------------
  // 8. Bangla, and the page reached without a link at all.

  await b.evaluate(`localStorage.setItem('flybirds.language', 'bn')`)
  await b.go('/forgot-password')
  await b.waitFor(`document.querySelector('#forgot-email')`, 15000)
  const bangla = await b.evaluate(`document.body.innerText`)
  check('the Bangla page asks for the address in Bangla', bangla.includes('ইমেইল ঠিকানা') && bangla.includes('পাসওয়ার্ড নতুন করে দিন'), bangla.slice(0, 60).replace(/\n/g, ' '))
  check('no page scrolls sideways at 375px', (await b.overflow()) === false)
  await b.shot('forgot-bn-375')

  await b.go('/reset-password')
  const noToken = await b.waitFor(`document.body.innerText.includes('এই ঠিকানায় কোনো লিংক নেই')`, 15000)
  check('the reset page without a link says so and offers a new one', noToken === true)

  await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
  await b.viewport(1440, 900)
  const issues = await b.evaluate(`window.__consoleErrors ? window.__consoleErrors.length : 0`)
  check('no console errors on the reset pages', !issues, String(issues))

  // -------------------------------------------------------------------------
  // 9. Put the throwaway account out of use; it is not part of the school.

  await b.login(email('admin'), pw)
  const suspended = await b.api(`/users/${created.id}/status`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ is_active: false }) })
  check('the throwaway account is suspended afterwards', suspended.status === 200, `status ${suspended.status}`)
} finally {
  r.save(join(shots, 'password-reset-checks.json'))
  await b.close()
  sink.close()
}
