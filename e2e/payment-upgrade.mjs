// Upgrade check for the payment-safety migration (1401) on a disposable,
// demo-seeded school. Run PHASE=before against the old release, upgrade the
// same school (same Compose project and volumes), then run PHASE=after.
//
//   E2E_BASE_URL=http://localhost:8100 DEMO_PASSWORD=... PHASE=before node e2e/payment-upgrade.mjs
//   (rebuild and restart the school on the new release)
//   E2E_BASE_URL=http://localhost:8100 DEMO_PASSWORD=... PHASE=after node e2e/payment-upgrade.mjs
//
// "before" makes payments through the old mock bKash/Nagad checkout, which
// recorded them as real; "after" checks they are kept, labelled simulated,
// excluded from collections and can be voided. Never run against a real school.
import { randomUUID } from 'node:crypto'
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8100'
const password = process.env.DEMO_PASSWORD
const phase = process.env.PHASE
if (!password || !['before', 'after'].includes(phase)) throw new Error('set DEMO_PASSWORD and PHASE=before|after')
const snapshotFile = join(here, 'shots', 'payment-upgrade-snapshot.json')
const r = reporter()
const check = (name, ok, detail = '') => r.check(`[${phase}] ${name}`, ok, detail)
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
const [childA, childB] = (await guardian('GET', '/guardians/me/children')).data
const statementOf = async id => (await guardian('GET', `/fees/children/${id}`)).data
const balanceOf = async id => cents((await statementOf(id)).totals.balance)
const collections = async () => (await accountant('GET', '/fees/reports/collections')).data
const income = async () => { const c = await collections(); return cents((await accountant('GET', `/accounts/summary?from=${c.from}&to=${c.to}`)).data.income) }

if (phase === 'before') {
  // Unpaid months to pay against (the demo seed leaves both children paid up).
  for (const child of [childA, childB]) {
    if ((await balanceOf(child.id)) >= 100000) continue
    const structure = (await accountant('GET', `/fees/dues?student_id=${child.id}&page=1&page_size=1`)).data.items[0].structure_id
    for (let month = 11; month < 15; month++) {
      const period = `${2026 + Math.floor((month - 1) / 12)}-${String((month - 1) % 12 + 1).padStart(2, '0')}`
      await accountant('POST', '/fees/dues', { student_id: child.id, structure_id: structure, period })
    }
  }
  const mock = async (pay, body) => {
    const intent = await pay('POST', '/fees/online/intents', body)
    if (intent.status !== 201) throw new Error(`old checkout: HTTP ${intent.status} ${intent.code}`)
    const done = await pay('POST', `/fees/online/intents/${intent.data.id}/confirm`, { otp: '123456' })
    if (done.status !== 200) throw new Error(`old confirm: HTTP ${done.status} ${done.code}`)
    return done.data.payment
  }
  const legacy = [
    { child: childA.id, ...(await mock(guardian, { student_id: childA.id, provider: 'bkash', amount: 300 })) },
    { child: childB.id, ...(await mock(guardian, { student_id: childB.id, provider: 'nagad', amount: 200.5 })) },
  ]
  const own = (await student('GET', '/fees/me')).data
  if (own && own.totals.balance >= 75.25) legacy.push({ child: own.student.id, ...(await mock(student, { provider: 'bkash', amount: 75.25 })) })
  check('the old release recorded mock online payments as real payments', legacy.every(p => /^RCT-/.test(p.receipt_no) && p.source === 'online'), legacy.map(p => `${p.receipt_no} ${p.amount}`).join(', '))
  const counter = (await accountant('POST', '/fees/payments', { student_id: childB.id, amount: 99.99, method: 'cash', reference: '', note: 'upgrade check', paid_on: (await collections()).to, idempotency_key: randomUUID() })).data.payment
  check('a counter payment is recorded on the old release', /^RCT-/.test(counter.receipt_no), counter.receipt_no)
  const snapshot = { legacy: legacy.map(p => ({ id: p.id, child: p.child, amount: p.amount, receipt_no: p.receipt_no })), counter: { id: counter.id, amount: counter.amount },
    balances: { [childA.id]: await balanceOf(childA.id), [childB.id]: await balanceOf(childB.id) }, collections: await collections(), income: await income() }
  mkdirSync(dirname(snapshotFile), { recursive: true }); writeFileSync(snapshotFile, JSON.stringify(snapshot, null, 1))
  r.save(join(here, 'shots', 'payment-upgrade-before.json'))
  process.exit()
}

const before = JSON.parse(readFileSync(snapshotFile, 'utf8'))
const legacyCents = before.legacy.reduce((n, p) => n + cents(p.amount), 0)

// Financial history is kept: same rows, same balances, now labelled.
for (const p of before.legacy) {
  const receipt = (await accountant('GET', `/fees/payments/${p.id}`)).data
  check(`legacy payment ${p.receipt_no} is kept and labelled simulated`, receipt?.payment.receipt_no === p.receipt_no && cents(receipt.payment.amount) === cents(p.amount) && receipt.payment.simulated === true && !receipt.payment.voided_at, JSON.stringify({ simulated: receipt?.payment.simulated, amount: receipt?.payment.amount }))
}
const counterReceipt = (await accountant('GET', `/fees/payments/${before.counter.id}`)).data
check('the counter payment is unchanged and not simulated', counterReceipt.payment.simulated === false && cents(counterReceipt.payment.amount) === cents(before.counter.amount))
for (const child of [childA, childB]) {
  const s = await statementOf(child.id)
  const mine = before.legacy.filter(p => p.child === child.id).reduce((n, p) => n + cents(p.amount), 0)
  check(`${child.name_en ?? child.id}: the balance is not silently rewritten`, cents(s.totals.balance) === before.balances[child.id], `${before.balances[child.id] / 100} → ${s.totals.balance}`)
  check(`${child.name_en ?? child.id}: the unreviewed simulated amount is reported`, cents(s.totals.simulated_unreviewed) === mine, `${s.totals.simulated_unreviewed}`)
}

// Reports count real money only.
const report = await collections()
check('collections exclude the legacy simulated payments', cents(report.total) === cents(before.collections.total) - legacyCents && report.count === before.collections.count - before.legacy.length, `${before.collections.total} → ${report.total}`)
check('collections report the simulated payments separately', cents(report.simulated_total) === legacyCents && report.simulated_count === before.legacy.length, `${report.simulated_total} in ${report.simulated_count}`)
check('ledger income excludes the legacy simulated payments', (await income()) === before.income - legacyCents, `${before.income / 100} → ${(await income()) / 100}`)

// Online payment is off. Students hold the grant again (1750: families use
// the student's account), but the payment-safety switch still refuses them.
const studentTry = await student('POST', '/fees/online/intents', { provider: 'bkash', amount: 10 })
check('a student is refused: online payment is off', studentTry.status === 403 && studentTry.code === 'online_payments_disabled', `HTTP ${studentTry.status} ${studentTry.code}`)
const guardianTry = await guardian('POST', '/fees/online/intents', { student_id: childA.id, provider: 'bkash', amount: 10 })
check('a guardian is refused: online payment is off', guardianTry.status === 403 && guardianTry.code === 'online_payments_disabled', `HTTP ${guardianTry.status} ${guardianTry.code}`)
const enable = await admin('PUT', '/fees/online/settings', { simulation_enabled: true })
check('the administrator cannot switch simulation on in this installation', enable.status === 409, `HTTP ${enable.status} ${enable.code}`)

// Review: only an administrator voids, only simulated payments, with a reason.
const review = (await admin('GET', '/fees/payments?simulated=unreviewed&page=1&page_size=100')).data.items.map(p => p.id)
check('the review list holds exactly the legacy payments', review.length === before.legacy.length && before.legacy.every(p => review.includes(p.id)), `${review.length}`)
const target = before.legacy[0]
const byAccountant = await accountant('POST', `/fees/payments/${target.id}/void`, { reason: 'no money received' })
const noReason = await admin('POST', `/fees/payments/${target.id}/void`, { reason: '' })
const realVoid = await admin('POST', `/fees/payments/${before.counter.id}/void`, { reason: 'try' })
check('void is refused for an accountant, without a reason, and for a real payment', byAccountant.status === 403 && noReason.status === 400 && realVoid.status === 403, `${byAccountant.status} ${noReason.status} ${realVoid.status}`)
const beforeVoid = await balanceOf(target.child)
const voided = await admin('POST', `/fees/payments/${target.id}/void`, { reason: 'Old simulator: no money received' })
const again = await admin('POST', `/fees/payments/${target.id}/void`, { reason: 'twice' })
check('the administrator voids a legacy simulated payment once', voided.status === 200 && !!voided.data.voided_at && again.status === 409, `${voided.status} then ${again.status}`)
check('voiding makes exactly its amount owed again', (await balanceOf(target.child)) === beforeVoid + cents(target.amount), `${beforeVoid / 100} → ${(await balanceOf(target.child)) / 100}`)
const afterVoid = await collections()
check('voiding does not change real collections', cents(afterVoid.total) === cents(report.total) && afterVoid.count === report.count, `${afterVoid.total}`)

// Real counter payments still work, exactly.
const due = await balanceOf(childB.id), c0 = await collections()
const paid = await accountant('POST', '/fees/payments', { student_id: childB.id, amount: 50.05, method: 'bank', reference: 'TRX-UPG-1', note: 'after upgrade', paid_on: c0.to, idempotency_key: randomUUID() })
check('a counter payment after the upgrade is real and exact', paid.status === 201 && paid.data.payment.simulated === false && (await balanceOf(childB.id)) === due - 5005 && cents((await collections()).total) === cents(c0.total) + 5005, paid.data?.payment?.receipt_no)

// Browser: labels on the student's own fees page and receipt, and the admin review panel.
const b = await launch({ base, shots: join(here, 'shots'), port: 9471 })
await b.viewport(390, 844, true)
await b.login('demo.student@demo.flybirds.test', password)
const mine = (await student('GET', '/students/me')).data.id
// The student's own legacy payment that was not voided above.
const other = before.legacy.find(p => p.child === mine && p.id !== target.id)
await b.go('/student/fees'); await b.waitFor(`document.querySelector('main').innerText.includes('${other.receipt_no}')`)
const page = await b.evaluate(`document.querySelector('main').innerText`)
check("the student's fees page labels the legacy payment and warns about it", page.includes('SIMULATED — NO MONEY TRANSFERRED') && page.includes('were simulated'), 'label and warning')
await b.evaluate(`[...document.querySelectorAll('main tr')].find(tr => tr.innerText.includes('${other.receipt_no}')).querySelector('button').click()`)
await b.waitFor(`document.querySelector('main').innerText.includes('not a receipt')`)
const receipt = await b.evaluate(`document.querySelector('main').innerText`)
check('the receipt is titled a simulation record, not proof of payment', receipt.includes('Simulation record — not a receipt') && receipt.includes('not proof of payment'), 'simulation record')
await b.shot('payment-upgrade-student-receipt')
b.close()
const a = await launch({ base, shots: join(here, 'shots'), port: 9472 })
await a.viewport(1280, 900)
await a.login('demo.admin@demo.flybirds.test', password)
await a.go('/accounts/fees'); await a.waitFor(`[...document.querySelectorAll('main button')].some(x => x.innerText.trim() === 'Fee setup')`)
await a.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Fee setup').click()`)
await a.waitFor(`document.querySelector('main').innerText.includes('Simulated payments to review') && document.querySelector('main').innerText.includes('${other.receipt_no}')`)
const panel = await a.evaluate(`document.querySelector('main').innerText`)
check('the admin review panel lists the remaining legacy payments and says simulation is not allowed', panel.includes(other.receipt_no) && !panel.includes(target.receipt_no + '\\n') && panel.includes('does not allow simulation'), 'review panel')
await a.shot('payment-upgrade-admin-panel')
a.close()
await sleep(200)
r.save(join(here, 'shots', 'payment-upgrade-after.json'))
