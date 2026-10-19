// Minimal headless Chromium/Edge driver over the Chrome DevTools Protocol.
// No dependencies: Node 22's built-in fetch and WebSocket are enough.
import { spawn } from 'node:child_process'
import { existsSync, mkdtempSync, writeFileSync, mkdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

export const sleep = ms => new Promise(r => setTimeout(r, ms))

// E2E_BROWSER overrides the browser; otherwise the first installed one is used.
function browserPath() {
  const candidates = [process.env.E2E_BROWSER, 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', 'C:/Program Files/Google/Chrome/Application/chrome.exe',
    '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']
  const found = candidates.find(p => p && existsSync(p))
  if (!found) throw new Error('No Chromium-based browser found; set E2E_BROWSER')
  return found
}

export async function launch({ base, shots, port = 9341 }) {
  mkdirSync(shots, { recursive: true })
  const profile = mkdtempSync(join(tmpdir(), 'flybirds-e2e-'))
  const edge = spawn(browserPath(), ['--headless=new', '--no-sandbox', ...(process.env.E2E_INSECURE_TLS === '1' ? ['--ignore-certificate-errors'] : []), `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, '--no-first-run', '--disable-gpu', '--force-prefers-reduced-motion', 'about:blank'], { stdio: 'ignore' })
  let ok = false
  for (let i = 0; i < 60 && !ok; i++) { try { await fetch(`http://127.0.0.1:${port}/json/version`); ok = true } catch { await sleep(200) } }
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: 'PUT' })).json()
  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise(r => ws.addEventListener('open', r))
  let seq = 0
  const pending = new Map(), errors = [], consoleErrors = []
  ws.addEventListener('message', ev => {
    const msg = JSON.parse(ev.data)
    if (msg.id && pending.has(msg.id)) { const { resolve, reject } = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result) }
    if (msg.method === 'Runtime.exceptionThrown') errors.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text)
    if (msg.method === 'Runtime.consoleAPICalled' && msg.params.type === 'error') consoleErrors.push(msg.params.args.map(a => a.value ?? a.description).join(' '))
    if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') consoleErrors.push(msg.params.entry.text)
  })
  const send = (method, params = {}) => new Promise((resolve, reject) => { const id = ++seq; pending.set(id, { resolve, reject }); ws.send(JSON.stringify({ id, method, params })) })
  await send('Page.enable'); await send('Runtime.enable'); await send('Log.enable')
  const b = {
    errors, consoleErrors, send,
    async evaluate(expression) {
      const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
      return r.result.value
    },
    async waitFor(expr, timeout = 12000) {
      const end = Date.now() + timeout
      while (Date.now() < end) { try { if (await b.evaluate(`!!(${expr})`)) return true } catch { /* loading */ } await sleep(150) }
      return false
    },
    async go(path) { await send('Page.navigate', { url: base + path }); await sleep(350); await b.waitFor('document.readyState === "complete"') },
    base,
    text: () => b.evaluate('document.body.innerText'),
    async viewport(width, height, mobile = false) { await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile }) },
    async shot(name, full = true) { const r = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: full }); writeFileSync(join(shots, name + '.png'), Buffer.from(r.data, 'base64')) },
    overflow: () => b.evaluate('document.documentElement.scrollWidth > window.innerWidth + 1'),
    async login(email, password, lang = 'en') {
      await b.go('/login')
      await b.evaluate(`(async () => { localStorage.setItem('flybirds.language', '${lang}'); const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); if (csrf) await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }).catch(() => null); return true })()`)
      const status = await b.evaluate(`fetch('/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: ${JSON.stringify(email)}, password: ${JSON.stringify(password)} }) }).then(r => r.status)`)
      if (status !== 200) throw new Error(`login ${email}: ${status}`)
    },
    async api(path, init = {}) {
      return b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1]); const r = await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }); const tok = (await r.json()).data.access_token; const init = ${JSON.stringify(init)}; init.headers = { ...(init.headers || {}), Authorization: 'Bearer ' + tok }; const x = await fetch('/api${path}', init); return { status: x.status, body: (x.headers.get('content-type') || '').includes('json') ? await x.json() : await x.text() } })()`)
    },
    clickText: (label, selector = 'button, a') => b.evaluate(`(() => { const el = [...document.querySelectorAll(${JSON.stringify(selector)})].find(e => e.innerText.trim() === ${JSON.stringify(label)} && e.offsetParent !== null); if (!el) return false; el.click(); return true })()`),
    async key(key, code = key, keyCode = 0) {
      await send('Input.dispatchKeyEvent', { type: 'keyDown', key, code, windowsVirtualKeyCode: keyCode })
      await send('Input.dispatchKeyEvent', { type: 'keyUp', key, code, windowsVirtualKeyCode: keyCode })
    },
    // Each launch gets a throwaway browser profile (hundreds of MB once pages
    // have loaded); delete it when the browser exits so runs do not fill the disk.
    close() { edge.once('exit', () => rmSync(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 })); ws.close(); edge.kill() },
  }
  return b
}

export function reporter() {
  const results = []
  return {
    results,
    check(name, ok, detail = '') { results.push({ name, ok: !!ok, detail }); console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ' — ' + detail : ''}`) },
    save(path) { writeFileSync(path, JSON.stringify(results, null, 1)); const p = results.filter(r => r.ok).length; console.log(`\n${p} of ${results.length} checks passed`) },
  }
}
