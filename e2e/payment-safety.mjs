// Payment-safety checks against a running, demo-seeded disposable school.
//
//   E2E_BASE_URL=http://localhost:8098 DEMO_PASSWORD=... PAYMENT_MODE=off node e2e/payment-safety.mjs
//   E2E_BASE_URL=http://localhost:8099 DEMO_PASSWORD=... PAYMENT_MODE=simulation node e2e/payment-safety.mjs
//
// PAYMENT_MODE=off: a production installation (PAYMENT_SIMULATION_ALLOWED
// false). PAYMENT_MODE=simulation: a test installation that allows
// simulation; the script switches it on and off itself. Never run against a
// real school: it records counter payments.
import { randomUUID } from 'node:crypto'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
const mode = process.env.PAYMENT_MODE ?? 'off'
if (!password || !['off', 'simulation'].includes(mode)) throw new Error('set DEMO_PASSWORD and PAYMENT_MODE=off|simulation')
const r = reporter()
const check = (name, ok, detail = '') => r.check(`[${mode}] ${name}`, ok, detail)
const cents = v => Math.round(v * 100)

async function signIn(role) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${role}@demo.flybirds.test`, password }) })
  if (res.status !== 200) throw new Error(`sign-in ${role}: ${res.status}`)
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
const [admin, accountant, guardian, student] = [await signIn('admin'), await signIn('accounts'), await signIn('guardian'), await signIn('student')]
const children = (await guardian('GET', '/guardians/me/children')).data
const [childA, childB] = children
const balanceOf = async id => cents((await guardian('GET', `/fees/children/${id}`)).data.totals.balance)
const paymentCount = async id => (await accountant('GET', `/fees/payments?student_id=${id}&page=1&page_size=100`)).data.total
const collections = async () => (await accountant('GET', '/fees/reports/collections')).data
check('the guardian has two children with separate fee accounts', children.length >= 2 && childA.id !== childB.id, `${children.length} children`)

// The demo seed leaves both children fully paid; give each one an unpaid
// month of its own tuition so there is something to (not) pay.
async function ensureDue(id) {
  if ((await balanceOf(id)) > 0) return
  const structure = (await accountant('GET', `/fees/dues?student_id=${id}&page=1&page_size=1`)).data.items[0].structure_id
  for (let month = 11; month < 40; month++) {
    const period = `${2026 + Math.floor((month - 1) / 12)}-${String((month - 1) % 12 + 1).padStart(2, '0')}`
    const res = await accountant('POST', '/fees/dues', { student_id: id, structure_id: structure, period })
    if (res.status === 201) return
    if (res.status !== 409) throw new Error(`assign due: HTTP ${res.status} ${res.code}`)
  }
  throw new Error('no free period to assign a due')
}
await ensureDue(childA.id); await ensureDue(childB.id)

// Students never hold online payment.
const studentIntent = await student('POST', '/fees/online/intents', { provider: 'bkash', amount: 10 })
check('a student cannot start an online payment (403)', studentIntent.status === 403, `HTTP ${studentIntent.status} ${studentIntent.code ?? ''}`)

if (mode === 'off') {
  const status = await guardian('GET', '/fees/online/status')
  check('online payment is off', status.data?.mode === 'off', JSON.stringify(status.data))
  const intent = await guardian('POST', '/fees/online/intents', { student_id: childA.id, provider: 'bkash', amount: 100 })
  check('a guardian is refused online payment (online_payments_disabled)', intent.status === 403 && intent.code === 'online_payments_disabled', `HTTP ${intent.status} ${intent.code}`)
  const forged = await guardian('POST', `/fees/online/intents/${randomUUID()}/confirm`, { otp: '123456' })
  check('a forged confirmation is refused', forged.status === 403, `HTTP ${forged.status} ${forged.code}`)
  const enable = await admin('PUT', '/fees/online/settings', { simulation_enabled: true })
  check('the administrator cannot switch simulation on in a production installation', enable.status === 409 && enable.code === 'simulation_not_allowed', `HTTP ${enable.status} ${enable.code}`)
} else {
  const enable = await admin('PUT', '/fees/online/settings', { simulation_enabled: true })
  check('the administrator switches simulation on in a test installation', enable.status === 200 && enable.data.mode === 'simulation', JSON.stringify(enable.data))
  const accountantEnable = await accountant('PUT', '/fees/online/settings', { simulation_enabled: true })
  check('an accountant cannot change the simulation switch', accountantEnable.status === 403, `HTTP ${accountantEnable.status}`)
  const beforeA = await balanceOf(childA.id), beforeB = await balanceOf(childB.id), paymentsA = await paymentCount(childA.id), collectedBefore = await collections()
  const intent = await guardian('POST', '/fees/online/intents', { student_id: childA.id, provider: 'bkash', amount: Math.min(500, beforeA / 100) })
  check('a simulated checkout uses a SIM reference, never a provider-style transaction ID', intent.status === 201 && /^SIMBK/.test(intent.data.provider_ref), intent.data?.provider_ref)
  const wrong = await guardian('POST', `/fees/online/intents/${intent.data.id}/confirm`, { otp: '000000' })
  check('a wrong code declines the simulation', wrong.status === 422, `HTTP ${wrong.status} ${wrong.code}`)
  const second = await guardian('POST', '/fees/online/intents', { student_id: childA.id, provider: 'nagad', amount: 100 })
  const stranger = await student('POST', `/fees/online/intents/${second.data.id}/confirm`, { otp: '123456' })
  check('another user cannot confirm the checkout', [403, 404].includes(stranger.status), `HTTP ${stranger.status}`)
  const confirms = await Promise.all(Array.from({ length: 5 }, () => guardian('POST', `/fees/online/intents/${second.data.id}/confirm`, { otp: '123456' })))
  check('five concurrent confirmations all return the same simulation', confirms.every(c => c.status === 200 && c.data.simulated === true && c.data.intent.id === second.data.id), confirms.map(c => c.status).join(','))
  check('the result says SIMULATED — NO MONEY TRANSFERRED and carries no receipt', confirms[0].data.message.includes('NO MONEY TRANSFERRED') && !confirms[0].data.payment, confirms[0].data.message)
  const replay = await guardian('POST', `/fees/online/intents/${second.data.id}/confirm`, { otp: '123456' })
  check('a replayed confirmation is a no-op', replay.status === 200 && replay.data.replayed === true, `replayed ${replay.data?.replayed}`)
  check('simulation does not reduce the balance of either child', (await balanceOf(childA.id)) === beforeA && (await balanceOf(childB.id)) === beforeB, `${beforeA / 100} / ${beforeB / 100}`)
  check('simulation records no payment or receipt', (await paymentCount(childA.id)) === paymentsA, `${paymentsA} payments before and after`)
  const collectedAfter = await collections()
  check('simulation adds nothing to the collection report', cents(collectedAfter.total) === cents(collectedBefore.total) && collectedAfter.count === collectedBefore.count, `${collectedBefore.total} → ${collectedAfter.total}`)
  const pending = await guardian('POST', '/fees/online/intents', { student_id: childB.id, provider: 'bkash', amount: 50 })
  const off = await admin('PUT', '/fees/online/settings', { simulation_enabled: false })
  const late = await guardian('POST', `/fees/online/intents/${pending.data.id}/confirm`, { otp: '123456' })
  check('switching simulation off blocks a checkout already started', off.status === 200 && late.status === 403 && late.code === 'online_payments_disabled', `HTTP ${late.status} ${late.code}`)
  await admin('PUT', '/fees/online/settings', { simulation_enabled: true })
}

// Counter payments: real money, still recorded with exact arithmetic and receipts.
const before = await balanceOf(childB.id), collectedBefore = await collections()
const key = randomUUID()
const pay = body => accountant('POST', '/fees/payments', { student_id: childB.id, amount: 123.45, method: 'cash', reference: '', note: 'payment-safety check', paid_on: collectedBefore.to, idempotency_key: key, ...body })
const first = await pay({}), again = await pay({})
check('a counter payment is recorded with a numbered receipt', first.status === 201 && /^RCT-\d{8}$/.test(first.data.payment.receipt_no) && first.data.payment.simulated === false, first.data?.payment?.receipt_no)
check('retrying the same counter payment returns the same receipt', again.status === 200 && again.data.payment.receipt_no === first.data.payment.receipt_no, `${again.status} ${again.data?.payment?.receipt_no}`)
check('the counter payment reduces the balance by exactly 123.45', (await balanceOf(childB.id)) === before - 12345, `${before / 100} → ${(await balanceOf(childB.id)) / 100}`)
const collectedAfter = await collections()
check('the collection report rises by exactly 123.45', cents(collectedAfter.total) - cents(collectedBefore.total) === 12345, `${collectedBefore.total} → ${collectedAfter.total}`)
const register = (await accountant('GET', '/fees/payments?page=1&page_size=100')).data.items
const realSum = register.filter(p => !p.simulated && p.paid_on >= collectedAfter.from && p.paid_on <= collectedAfter.to).reduce((n, p) => n + cents(p.amount), 0)
check('the collection report equals the sum of real payments in the period', realSum === cents(collectedAfter.total), `${realSum / 100} = ${collectedAfter.total}`)

// Browser: the guardian page offers online payment only in simulation, labelled.
const b = await launch({ base, shots: join(here, 'shots'), port: 9470 })
await b.viewport(390, 844, true)
await b.login('demo.guardian@demo.flybirds.test', password)
await b.go(`/guardian/children/${childA.id}/fees`); await sleep(1200)
const page = await b.evaluate(`document.querySelector('main').innerText`)
if (mode === 'off') check('the guardian fees page shows no online payment', !/Test online payment|Pay with bKash/i.test(page), 'no pay section')
else check('the guardian fees page labels online payment SIMULATED — NO MONEY TRANSFERRED', page.includes('SIMULATED — NO MONEY TRANSFERRED'), 'label present')
await b.shot(`payment-safety-guardian-${mode}`)
b.close()

if (mode === 'simulation') await admin('PUT', '/fees/online/settings', { simulation_enabled: false })
r.save(join(here, 'shots', `payment-safety-${mode}.json`))
