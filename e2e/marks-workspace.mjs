// E4 marks workspace on a disposable, demo-seeded school prepared with
// realistic sheets by e2e/marks-workspace-setup.mjs: keyboard entry,
// autosave, validation, paste, conflicts, offline and recovery, submission,
// review, access, layouts, languages, themes, branding and accessibility.
//
//   E2E_BASE_URL=http://localhost:8104 DEMO_PASSWORD=... SHEETS=sheets.json node e2e/marks-workspace.mjs
//
// SHEETS lists {exam_id, sheet_id, students} for demo.teacher's draft sheets,
// the first with 50+ students. It writes marks; never run it on a real school.
import { readFileSync, existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, reporter, sleep } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL ?? 'http://localhost:8081'
const password = process.env.DEMO_PASSWORD
const sheets = JSON.parse(readFileSync(process.env.SHEETS, 'utf8'))
if (!password || sheets.length < 3) throw new Error('set DEMO_PASSWORD and SHEETS (3+ sheets)')
const r = reporter()
const check = (name, ok, detail = '') => r.check(name, ok, detail)
const metrics = {}

async function api(who) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${who}@demo.flybirds.test`, password }) })
  const token = (await res.json()).data.access_token
  return async (method, path, body) => {
    const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
    const json = await response.json().catch(() => ({}))
    return { status: response.status, data: json.data, code: json.error?.code }
  }
}
const [teacherAPI, adminAPI, studentAPI, guardianAPI] = await Promise.all(['teacher', 'admin', 'student', 'guardian'].map(api))
const server = async s => (await teacherAPI('GET', `/exams/assigned/${s.exam_id}/sheets/${s.sheet_id}`)).data
const [big, second, third] = sheets
const sheetPath = s => `/teacher/exams/${s.exam_id}/sheets/${s.sheet_id}`

// In-page helpers: count save requests and their latency; collect slow input events.
const instrument = `(() => {
  if (window.__mw) return true
  window.__mw = { saves: 0, latencies: [], events: [], api: 0, apiFailed: 0 }
  const f = window.fetch
  window.fetch = async (...args) => {
    const req = args[1] || {}
    const isSave = String(args[0]).includes('/marks') && req.method === 'PATCH'
    const isAPI = String(args[0]).includes('/api/')
    const start = performance.now()
    let ok = false
    try { const response = await f(...args); ok = response.ok; return response } finally {
      if (isSave) { window.__mw.saves++; window.__mw.latencies.push(performance.now() - start) }
      if (isAPI) { window.__mw.api++; if (!ok) window.__mw.apiFailed++ }
    }
  }
  window.__mw.processing = []
  try { new PerformanceObserver(list => { for (const e of list.getEntries()) if (['keydown', 'keyup', 'input', 'keypress'].includes(e.name)) { window.__mw.events.push(e.duration); window.__mw.processing.push(e.processingEnd - e.processingStart) } }).observe({ type: 'event', durationThreshold: 16 }) } catch {}
  return true
})()`
const marksInputs = `[...document.querySelectorAll('main table tbody input[inputmode="decimal"]')]`
const saveState = `document.querySelector('[data-save-state]')`
const allSaved = `(${saveState}?.innerText || '').includes('All changes saved')`

const b = await launch({ base, shots: join(here, 'shots'), port: 9476 })
await b.viewport(1440, 900)
await b.login('demo.teacher@demo.flybirds.test', password)

// ---- 1. Queue → sheet ----------------------------------------------------
await b.go('/teacher/exams')
await b.waitFor(`[...document.querySelectorAll('main a')].some(a => a.getAttribute('href') === ${JSON.stringify(sheetPath(big))})`, 15000)
const queue = await b.evaluate(`document.querySelector('main').innerText`)
check('the queue lists the teacher\'s draft sheets as work to do', sheets.every(s => queue.includes(s.section)) && /To do\s*\d+/.test(queue), 'queue')
let clicks = 0
await b.evaluate(`[...document.querySelectorAll('main a')].find(a => a.getAttribute('href') === ${JSON.stringify(sheetPath(big))}).click()`); clicks++
await b.waitFor(`${marksInputs}.length === ${big.students}`, 15000)
await b.evaluate(instrument)
check('opening a sheet takes one click from the queue and shows every student', (await b.evaluate(`${marksInputs}.length`)) === big.students, `${big.students} rows, ${clicks} click`)
const headers = await b.evaluate(`[document.querySelectorAll('main table thead th[scope=col]').length, document.querySelectorAll('main table tbody th[scope=row]').length]`)
check('screen readers get a table with column headers and a row header per student', headers[0] >= 5 && headers[1] === big.students, JSON.stringify(headers))
await b.shot('mw-desktop-empty', false)

// ---- 2. Keyboard entry of the whole sheet ----------------------------------
const want = Array.from({ length: big.students }, (_, i) => (i * 7 + 13) % 51)
await b.evaluate(`${marksInputs}[0].focus()`); clicks++
await sleep(500)
await b.evaluate(`window.__mw.events = []; window.__mw.processing = []`) // only the typing itself
let keystrokes = 0
const t0 = Date.now()
for (const value of want) {
  await b.send('Input.insertText', { text: String(value) }); keystrokes += String(value).length
  await b.key('Enter', 'Enter', 13); keystrokes++
}
const typed = Date.now() - t0
await b.waitFor(allSaved, 20000)
const saveDone = Date.now() - t0
const m1 = await b.evaluate(`window.__mw`)
let rows = (await server(big)).rows
check('every typed mark is saved to the right student', rows.every((row, i) => row.marks === want[i] && row.status === 'present'), `${rows.filter((row, i) => row.marks === want[i]).length}/${big.students}`)
const pct = (list, q) => { const sorted = [...list].sort((x, y) => x - y); return sorted.length ? Math.round(sorted[Math.min(sorted.length - 1, Math.floor(q * sorted.length))]) : 0 }
metrics.keyboardEntry = { students: big.students, clicks, keystrokes, typingSeconds: +(typed / 1000).toFixed(1), allSavedSeconds: +(saveDone / 1000).toFixed(1), saveRequests: m1.saves, apiRequests: m1.api, apiErrors: m1.apiFailed, saveLatencyMs: { median: pct(m1.latencies, 0.5), max: Math.round(Math.max(...m1.latencies)) }, inputEventsOver16ms: m1.events.length, inputEventMs: { p50: pct(m1.events, 0.5), p95: pct(m1.events, 0.95), max: Math.round(Math.max(0, ...m1.events)) }, handlerMs: { p95: pct(m1.processing, 0.95), max: Math.round(Math.max(0, ...m1.processing)) } }
check('autosave batches the edits (far fewer requests than marks)', m1.saves > 0 && m1.saves < big.students / 2, `${m1.saves} requests for ${big.students} marks`)
check('keystroke handling stays fast (handlers under 50 ms; 95 % of keystrokes painted within 200 ms)', metrics.keyboardEntry.handlerMs.max <= 50 && metrics.keyboardEntry.inputEventMs.p95 <= 200, JSON.stringify({ event: metrics.keyboardEntry.inputEventMs, handler: metrics.keyboardEntry.handlerMs }))

// ---- 3. Shortcuts, Escape, validation ------------------------------------
await b.evaluate(`${marksInputs}[2].focus()`)
await b.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'a', code: 'KeyA', text: 'a' }); await b.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'a', code: 'KeyA' })
await b.waitFor(allSaved, 10000)
rows = (await server(big)).rows
check('the A key marks a student absent', rows[2].status === 'absent' && rows[2].marks === null, rows[2].status)
await b.evaluate(`${marksInputs}[3].focus(); ${marksInputs}[3].select()`)
await b.send('Input.insertText', { text: '12' })
await b.key('Escape', 'Escape', 27)
const reverted = await b.evaluate(`${marksInputs}[3].value`)
await sleep(1000)
rows = (await server(big)).rows
check('Escape reverts an unsaved edit', reverted === String(want[3]) && rows[3].marks === want[3], `${reverted}`)
await b.evaluate(`${marksInputs}[4].focus(); ${marksInputs}[4].select()`)
await b.send('Input.insertText', { text: '60' })
await b.waitFor(`document.querySelector('main').innerText.includes('Above the full marks')`, 8000)
const invalid = await b.evaluate(`(() => { const i = ${marksInputs}[4]; return { aria: i.getAttribute('aria-invalid'), described: !!document.getElementById(i.getAttribute('aria-describedby') || 'x') } })()`)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Needs attention').click()`)
await sleep(300)
const attention = await b.evaluate(`${marksInputs}.length`)
check('an invalid mark is flagged on its cell, announced, and found by the filter', invalid.aria === 'true' && invalid.described && attention === 1, `${attention} row`)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'All').click()`)
await sleep(200)
await b.evaluate(`${marksInputs}[4].focus(); ${marksInputs}[4].select()`)
await b.send('Input.insertText', { text: '৪৫' }) // Bangla digits
await b.waitFor(allSaved, 10000)
rows = (await server(big)).rows
check('a Bangla-digit mark replaces the invalid one and saves as 45', rows[4].marks === 45, `${rows[4].marks}`)

// ---- 4. Repeated edits of one cell --------------------------------------
const before = await b.evaluate(`window.__mw.saves`)
await b.evaluate(`${marksInputs}[5].focus()`)
for (const v of ['1', '10', '20', '30', '41']) { await b.evaluate(`${marksInputs}[5].select()`); await b.send('Input.insertText', { text: v }); await sleep(80) }
await b.waitFor(allSaved, 10000)
rows = (await server(big)).rows
const repeatSaves = (await b.evaluate(`window.__mw.saves`)) - before
check('rapid repeated edits save the last value in one or two requests', rows[5].marks === 41 && repeatSaves <= 2, `${repeatSaves} request(s)`)

// ---- 5. Conflict: someone else changed the cell -------------------------
const row6 = rows[6]
await adminAPI('PATCH', `/exams/${big.exam_id}/marks`, { entries: [{ exam_subject_id: big.sheet_id, student_id: row6.student_id, status: 'present', marks: 33, reason: '', row_version: row6.row_version }] })
await b.evaluate(`${marksInputs}[6].focus(); ${marksInputs}[6].select()`)
await b.send('Input.insertText', { text: '44' })
await b.waitFor(`document.querySelector('main').innerText.includes('Changed by someone else to 33')`, 10000)
rows = (await server(big)).rows
check('a stale edit becomes a visible conflict with the server value; nothing is overwritten', rows[6].marks === 33, `server ${rows[6].marks}`)
await b.shot('mw-conflict', false)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Use theirs').click()`)
await sleep(400)
check('"Use theirs" adopts the authoritative value', (await b.evaluate(`${marksInputs}[6].value`)) === '33', 'resolved')

// ---- 6. Offline: kept, then saved when the network returns --------------
await b.send('Network.enable')
await b.send('Network.emulateNetworkConditions', { offline: true, latency: 0, downloadThroughput: -1, uploadThroughput: -1 })
await b.evaluate(`${marksInputs}[7].focus(); ${marksInputs}[7].select()`)
await b.send('Input.insertText', { text: '39' })
await b.waitFor(`${saveState}?.dataset.saveState === 'offline'`, 10000)
const offlineText = await b.evaluate(`${saveState}.innerText`)
await b.send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 })
await b.evaluate(`window.dispatchEvent(new Event('online'))`)
await b.waitFor(allSaved, 20000)
rows = (await server(big)).rows
check('offline edits are kept, shown, and saved when the connection returns', offlineText.includes('Offline') && rows[7].marks === 39, offlineText)

// ---- 7. Saves failing, then a reload: recovery ----------------------------
// Only the save endpoint is blocked, so saves fail while the page reloads.
await b.send('Network.setBlockedURLs', { urls: ['*/api/exams/*/marks'] })
await b.evaluate(`${marksInputs}[8].focus(); ${marksInputs}[8].select()`)
await b.send('Input.insertText', { text: '28.5' })
await b.waitFor(`${saveState}?.dataset.saveState === 'offline'`, 10000)
const stored = await b.evaluate(`Object.keys(sessionStorage).filter(k => k.startsWith('flybirds.marks.v1.')).map(k => sessionStorage.getItem(k))`)
const leakFree = stored.length === 1 && stored[0].includes('28.5') && !/[A-Z][a-z]+ [A-Z][a-z]+/.test(stored[0]) && !stored[0].includes('@')
await b.send('Page.enable')
void b.send('Page.reload', { ignoreCache: false })
await sleep(800)
const warned = await b.send('Page.handleJavaScriptDialog', { accept: true }).then(() => true).catch(() => false) // the "leave site?" prompt
await b.waitFor(`document.querySelector('main') && document.querySelector('main').innerText.includes('unsaved change')`, 15000)
const banner = await b.evaluate(`document.querySelector('main').innerText.match(/\\d+ unsaved changes? from [0-9:]+/)?.[0] || ''`)
const serverBefore = (await server(big)).rows[8].marks
await b.send('Network.setBlockedURLs', { urls: [] })
await b.evaluate(instrument)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Restore them').click()`)
await b.waitFor(allSaved, 15000)
rows = (await server(big)).rows
check('after a reload the unsaved edit is offered back, restored and saved', banner.startsWith('1 unsaved') && serverBefore !== 28.5 && rows[8].marks === 28.5, banner)
check('recovery keeps ids and values only (no names) in this tab\'s sessionStorage', leakFree, `${stored.length} entry`)
check('the browser asked before leaving with an unsaved edit', warned, 'beforeunload')

// ---- 8. Exempt by the administrator: locked for the teacher --------------
rows = (await server(second)).rows
const exemptRow = rows[1]
await adminAPI('PATCH', `/exams/${second.exam_id}/marks`, { entries: [{ exam_subject_id: second.sheet_id, student_id: exemptRow.student_id, status: 'exempt', marks: null, reason: 'Medical certificate', row_version: exemptRow.row_version }] })

// ---- 9. Paste by admission number (shuffled, with bad lines) -------------
await b.go(sheetPath(second))
await b.waitFor(`${marksInputs}.length === ${second.students}`, 15000)
await b.evaluate(instrument)
rows = (await server(second)).rows
const pasteWant = new Map(rows.map((row, i) => [row.student_id, (i * 3 + 20) % 51]))
const lines = [...rows].reverse().map(row => `${row.admission_no}\t${row.student_name}\t${pasteWant.get(row.student_id)}`)
lines.splice(3, 0, 'DEMO-9999\tStranger\t40', `${rows[5].admission_no}\tAgain\t12`)
lines[10] = lines[10].replace(/\t\d+$/, '\t55')
const tsv = ['Admission\tName\tMarks', ...lines].join('\n')
const pasteStart = Date.now()
await b.evaluate(`(() => { const dt = new DataTransfer(); dt.setData('text/plain', ${JSON.stringify(tsv)}); const i = ${marksInputs}[0]; i.focus(); i.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true })) })()`)
await b.waitFor(`document.querySelector('[role=dialog]')?.innerText.includes('Matched to students by admission number')`, 8000)
const preview = await b.evaluate(`document.querySelector('[role=dialog]').innerText`)
const summary = preview.match(/(\d+) to apply · (\d+) rejected · (\d+) skipped/)
await b.shot('mw-paste-preview', false)
await b.evaluate(`document.querySelector('[role=dialog] button[data-primary]').click()`)
await b.waitFor(`document.querySelector('main').innerText.includes(' saved.')`, 15000)
const pasteMs = Date.now() - pasteStart
rows = (await server(second)).rows
const exempted = rows.find(row => row.student_id === exemptRow.student_id)
const badLine = rows.find(row => lines[10].startsWith(row.admission_no + '\t'))
const duplicated = rows[5].student_id
const mapped = rows.filter(row => ![exemptRow.student_id, badLine.student_id, duplicated].includes(row.student_id)).every(row => row.marks === pasteWant.get(row.student_id))
  && rows.find(row => row.student_id === duplicated).marks === null
check('a shuffled paste maps every mark to the right student by admission number', mapped, `${rows.length} students`)
check('the preview rejects the stranger, both lines of a duplicated student, the above-full mark and the exempt student', summary && Number(summary[2]) === 5, summary?.[0])
check('the exempt student keeps the administrator\'s status', exempted.status === 'exempt', exempted.status)
const pasteSaves = await b.evaluate(`window.__mw.saves`)
const pasteAPI = await b.evaluate(`[window.__mw.api, window.__mw.apiFailed]`)
check('no request failed while typing or pasting (error rate 0)', m1.apiFailed === 0 && pasteAPI[1] === 0, `typing ${m1.apiFailed}/${m1.api}, paste ${pasteAPI[1]}/${pasteAPI[0]}`)
metrics.paste = { students: second.students, pasteToSavedSeconds: +(pasteMs / 1000).toFixed(1), saveRequests: pasteSaves, apiRequests: pasteAPI[0], apiErrors: pasteAPI[1], applied: Number(summary?.[1]), rejected: Number(summary?.[2]) }
const lockedText = await b.evaluate(`[...document.querySelectorAll('main table tbody tr')].find(tr => tr.innerText.includes(${JSON.stringify(exemptRow.admission_no)})).innerText`)
check('the exempt row tells the teacher it is set by the administrator', lockedText.includes('Set by the administrator'), 'locked row')

// Paste by position is refused while a filter hides rows.
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Missing').click()`)
await sleep(200)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Paste marks').click()`)
await b.evaluate(`(() => { const ta = document.querySelector('[role=dialog] textarea'); const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set; set.call(ta, '10\\n20\\n30'); ta.dispatchEvent(new Event('input', { bubbles: true })) })()`)
await b.evaluate(`document.querySelector('[role=dialog] button[data-primary]').click()`)
await b.waitFor(`document.querySelector('[role=dialog]')?.innerText.includes('Clear the search and the filter')`, 5000)
check('paste by position is refused while rows are filtered', true, 'refused')
await b.key('Escape', 'Escape', 27)
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'All')?.click()`)

// ---- 10. Submit with explanation, then read-only ----------------------------
await b.go(sheetPath(big))
await b.waitFor(`${marksInputs}.length === ${big.students}`, 15000)
const submitState = await b.evaluate(`(() => { const btn = [...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Submit for review'); return { disabled: btn.disabled, why: document.getElementById('submit-why')?.innerText || '' } })()`)
check('the complete sheet can be submitted', !submitState.disabled, submitState.why || 'enabled')
await b.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Submit for review').click()`)
await b.waitFor(`[...document.querySelectorAll('button')].filter(x => x.innerText.trim() === 'Submit for review').length === 2`, 5000)
await b.evaluate(`[...document.querySelectorAll('button')].filter(x => x.innerText.trim() === 'Submit for review').pop().click()`)
await b.waitFor(`document.querySelector('main').innerText.includes('read-only until the head teacher')`, 10000)
const ro = await b.evaluate(`({ disabled: ${marksInputs}.every(i => i.disabled), text: document.querySelector('main').innerText })`)
check('after submission the sheet is read-only and says why', ro.disabled && ro.text.includes('Submitted'), 'read-only')
await b.go(sheetPath(third))
await b.waitFor(`${marksInputs}.length === ${third.students}`, 15000)
const blocked = await b.evaluate(`({ disabled: [...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Submit for review').disabled, why: document.getElementById('submit-why')?.innerText })`)
check('an incomplete sheet explains why it cannot be submitted', blocked.disabled && /still needs? marks/.test(blocked.why), blocked.why)
b.close()

// ---- 11. Head teacher reviews; students and guardians are kept out --------
const h = await launch({ base, shots: join(here, 'shots'), port: 9477 })
await h.viewport(1440, 900)
await h.login('demo.head@demo.flybirds.test', password)
await h.go(`/admin/exams/${big.exam_id}/sheets/${big.sheet_id}`)
await h.waitFor(`document.querySelector('main').innerText.includes('Approve')`, 15000)
const headView = await h.evaluate(`({ text: document.querySelector('main').innerText, inputs: ${marksInputs}.filter(i => !i.disabled).length })`)
check('the head teacher sees a read-only sheet with review actions', headView.inputs === 0 && headView.text.includes('Return to teacher'), 'review only')
await h.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'Approve').click()`)
await h.waitFor(`[...document.querySelectorAll('button')].filter(x => x.innerText.trim() === 'Approve').length === 2`, 5000)
await h.evaluate(`[...document.querySelectorAll('button')].filter(x => x.innerText.trim() === 'Approve').pop().click()`)
await h.waitFor(`document.querySelector('main').innerText.includes('Approved.')`, 10000)
check('the head teacher approves the submitted sheet', (await server(big)).sheet.status === 'approved', 'approved')
h.close()
const studentWS = await studentAPI('GET', `/exams/assigned/${big.exam_id}/sheets/${big.sheet_id}`)
const guardianWS = await guardianAPI('GET', `/exams/${big.exam_id}/sheets/${big.sheet_id}`)
const studentQueue = await studentAPI('GET', '/exams/my-sheets')
check('students and guardians cannot read sheets or the work queue', studentWS.status === 403 && guardianWS.status === 403 && studentQueue.status === 403, `${studentWS.status} ${guardianWS.status} ${studentQueue.status}`)
const sb = await launch({ base, shots: join(here, 'shots'), port: 9478 })
await sb.login('demo.student@demo.flybirds.test', password)
await sb.go(sheetPath(third))
await sb.waitFor(`document.querySelector('main')`, 8000)
await sleep(800)
check('a student opening a sheet URL is denied', /permission|denied|not allowed/i.test(await sb.evaluate(`document.querySelector('main').innerText`)), 'denied page')
sb.close()

// ---- 12. Layouts, languages, themes, branding, accessibility -------------
const { updated_at: _ignored, ...settings } = (await adminAPI('GET', '/school/settings')).data
const originalColour = settings.primary_color
const putSettings = async patch => { const res = await adminAPI('PUT', '/school/settings', { ...settings, ...patch }); if (res.status !== 200) throw new Error(`school settings: ${res.status} ${res.code}`) }
const v = await launch({ base, shots: join(here, 'shots'), port: 9479 })
const axePath = join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js')
const contrast = `(() => {
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  const rgb = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = '#000'; ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const opaque = c => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = c; ctx.fillRect(0, 0, 1, 1); return ctx.getImageData(0, 0, 1, 1).data[3] > 250 }
  const bg = el => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (c && opaque(c)) return c } return getComputedStyle(document.body).backgroundColor }
  const ratio = el => { const a = lum(rgb(getComputedStyle(el).color)), b = lum(rgb(bg(el))); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05) }
  const sel = 'main h1, main p, main span, main td, main th, main button, main a, main select, main input, main label'
  const low = [...document.querySelectorAll(sel)].filter(el => el.offsetParent !== null && el.innerText?.trim() && !el.closest('[disabled]') && !el.disabled).map(el => [el, ratio(el), parseFloat(getComputedStyle(el).fontSize), getComputedStyle(el).fontWeight])
    .filter(([, r, size, weight]) => r < ((size >= 24 || (size >= 18.66 && Number(weight) >= 700)) ? 3 : 4.5))
  return low.slice(0, 5).map(([el, r]) => el.innerText.trim().slice(0, 30) + ' ' + r.toFixed(2))
})()`
const runAxe = async () => {
  if (!existsSync(axePath)) return null
  await v.evaluate(readFileSync(axePath, 'utf8'))
  return v.evaluate(`axe.run(document.querySelector('main'), { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(r => r.violations.map(x => x.id + ':' + x.nodes.length))`)
}
for (const [label, width, height, mobile, lang, theme, colour] of [
  ['desktop-en-light', 1440, 900, false, 'en', 'light', '#1d4ed8'],
  ['tablet-en-light', 768, 1024, true, 'en', 'light', '#0f766e'],
  ['mobile-bn-light', 390, 844, true, 'bn', 'light', '#0f766e'],
  ['desktop-bn-dark', 1440, 900, false, 'bn', 'dark', '#b45309'],
  ['mobile-en-dark', 390, 844, true, 'en', 'dark', '#1d4ed8'],
]) {
  await putSettings({ primary_color: colour, dashboard_theme: theme })
  await v.viewport(width, height, mobile)
  await v.login('demo.teacher@demo.flybirds.test', password, lang)
  await v.go(sheetPath(third))
  await v.waitFor(`document.querySelectorAll('main input[inputmode="decimal"]').length >= ${third.students}`, 15000)
  await sleep(600)
  const state = await v.evaluate(`({ overflow: document.documentElement.scrollWidth > window.innerWidth + 1, theme: document.documentElement.dataset.theme, h1: document.querySelector('main h1').innerText, table: !!document.querySelector('main table'), brand: getComputedStyle(document.documentElement).getPropertyValue('--brand-primary').trim() })`)
  const low = await v.evaluate(contrast)
  const axe = await runAxe()
  check(`${label}: no horizontal overflow, ${theme} theme, ${width < 640 ? 'list' : 'grid'} layout, school colour applied`, !state.overflow && state.theme === theme && (width < 640 ? !state.table : state.table) && state.brand.length > 0, JSON.stringify(state))
  check(`${label}: WCAG AA text contrast`, low.length === 0, low.join(' | ') || 'ok')
  if (axe) check(`${label}: axe finds no WCAG 2.1 AA violations`, axe.length === 0, axe.join(', ') || 'clean')
  await v.shot(`mw-${label}`, false)
}
await putSettings({ primary_color: originalColour, dashboard_theme: settings.dashboard_theme })
// One student at a time on a phone.
await v.viewport(390, 844, true)
await v.login('demo.teacher@demo.flybirds.test', password, 'en')
await v.go(sheetPath(third))
await v.waitFor(`document.querySelectorAll('main input[inputmode="decimal"]').length >= 3`, 15000)
await v.evaluate(`[...document.querySelectorAll('main button')].find(x => x.innerText.trim() === 'One at a time').click()`)
await sleep(300)
const one = await v.evaluate(`({ inputs: [...document.querySelectorAll('main input[inputmode="decimal"]')].filter(i => i.offsetParent !== null).length, text: document.querySelector('main').innerText })`)
await v.shot('mw-mobile-one-at-a-time', false)
check('one-at-a-time mode shows a single large entry on a phone', one.inputs === 1 && /1 of \d+/.test(one.text), `${one.inputs} input`)
v.close()

console.log('\nMETRICS ' + JSON.stringify(metrics))
r.save(join(here, 'shots', 'marks-workspace.json'))
