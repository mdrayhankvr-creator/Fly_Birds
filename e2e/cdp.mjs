// Minimal headless Chromium/Edge driver over the Chrome DevTools Protocol.
// No dependencies: Node 22's built-in fetch and WebSocket are enough.
import { spawn } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

export const sleep = ms => new Promise(r => setTimeout(r, ms))

// E2E_BROWSER overrides the browser; otherwise the first installed one is used.
// A set but missing E2E_BROWSER is an error, not a silent fallback.
export function browserPath() {
  if (process.env.E2E_BROWSER) {
    if (!existsSync(process.env.E2E_BROWSER)) throw new Error(`E2E_BROWSER does not exist: ${process.env.E2E_BROWSER}`)
    return process.env.E2E_BROWSER
  }
  const candidates = ['C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', 'C:/Program Files/Google/Chrome/Application/chrome.exe',
    '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']
  const found = candidates.find(p => existsSync(p))
  if (!found) throw new Error('No Chromium-based browser found; set E2E_BROWSER')
  return found
}

const positive = (value, fallback) => Number(value) > 0 ? Number(value) : fallback

// Starts the browser, retrying a failed start once by default. Each attempt is
// bounded; a browser that cannot start stops the run with its own error output
// instead of letting checks run against nothing.
//   timeoutMs (E2E_BROWSER_TIMEOUT_MS, 30000): readiness limit per attempt.
//   attempts (E2E_BROWSER_ATTEMPTS, 2): total starts before giving up.
// `port` is accepted for compatibility but unused: the browser picks a free
// port and reports it in its own profile, so a stale browser or another
// process on a fixed port can never answer for this one.
export async function launch({ base, shots, port: _unusedPort, browser, browserArgs = [], timeoutMs = positive(process.env.E2E_BROWSER_TIMEOUT_MS, 30000), attempts = positive(process.env.E2E_BROWSER_ATTEMPTS, 2) }) {
  mkdirSync(shots, { recursive: true })
  const executable = browser || browserPath()
  const failures = []
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      return await connect(await startBrowser(executable, browserArgs, timeoutMs), base, shots, timeoutMs)
    } catch (error) {
      failures.push(`attempt ${attempt}: ${error.message}`)
      if (attempt < attempts) console.warn(`warning: browser start failed (${error.message}); retrying`)
    }
  }
  throw new Error(`Browser did not start (${executable}).\n${failures.join('\n')}`)
}

async function startBrowser(executable, browserArgs, timeoutMs) {
  const profile = mkdtempSync(join(tmpdir(), 'flybirds-e2e-'))
  const flags = ['--headless=new', '--no-sandbox', ...(process.env.E2E_INSECURE_TLS === '1' ? ['--ignore-certificate-errors'] : []), '--remote-debugging-port=0', `--user-data-dir=${profile}`, '--no-first-run', '--disable-gpu', '--force-prefers-reduced-motion', 'about:blank']
  const child = spawn(executable, [...browserArgs, ...flags], { stdio: ['ignore', 'ignore', 'pipe'] })
  const state = { child, profile, stderr: '', exit: null, spawnError: null }
  child.stderr.on('data', chunk => { state.stderr = (state.stderr + chunk).slice(-2000) })
  child.once('error', error => { state.spawnError = error })
  child.once('exit', (code, signal) => { state.exit = { code, signal } })
  const deadline = Date.now() + timeoutMs
  try {
    while (Date.now() < deadline) {
      if (state.spawnError) throw new Error(`could not start: ${state.spawnError.message}`)
      if (state.exit) throw new Error(`exited during startup (code ${state.exit.code}${state.exit.signal ? `, signal ${state.exit.signal}` : ''})${detail(state)}`)
      // Chrome writes the chosen port here once DevTools is listening.
      const active = readPort(join(profile, 'DevToolsActivePort'))
      if (active) {
        try {
          const version = await fetch(`http://127.0.0.1:${active}/json/version`, { signal: AbortSignal.timeout(2000) })
          if (version.ok) { state.port = active; return state }
        } catch { /* not answering yet */ }
      }
      await sleep(100)
    }
    throw new Error(`did not become ready within ${timeoutMs} ms${detail(state)}`)
  } catch (error) {
    await stopBrowser(state)
    throw error
  }
}

function readPort(file) {
  try { return Number(readFileSync(file, 'utf8').split('\n')[0]) || 0 } catch { return 0 }
}

function detail(state) {
  const text = state.stderr.trim().split('\n').slice(-5).join('\n')
  return text ? `; browser output:\n${text}` : ''
}

// Stops only the browser this run started, then deletes its profile.
async function stopBrowser(state) {
  if (!state.exit && !state.spawnError) {
    const exited = new Promise(resolve => state.child.once('exit', resolve))
    state.child.kill()
    await Promise.race([exited, sleep(5000)])
  }
  await removeProfile(state.profile)
}

async function connect(state, base, shots, timeoutMs) {
  const ws = await openTarget(state, timeoutMs).catch(async error => { await stopBrowser(state); throw error })
  let seq = 0, closed = null
  const pending = new Map(), errors = [], consoleErrors = []
  // Fail outstanding and later commands instead of hanging if the browser goes away.
  const disconnect = reason => {
    if (closed) return
    closed = new Error(`browser connection closed: ${reason}${detail(state)}`)
    for (const { reject } of pending.values()) reject(closed)
    pending.clear()
  }
  ws.addEventListener('close', () => disconnect('DevTools socket closed'))
  state.child.once('exit', (code, signal) => disconnect(`browser exited (code ${code}${signal ? `, signal ${signal}` : ''})`))
  // Never leave this run's browser behind if the script stops without close().
  const killOnExit = () => { if (!state.exit) state.child.kill() }
  process.once('exit', killOnExit)
  ws.addEventListener('message', ev => {
    const msg = JSON.parse(ev.data)
    if (msg.id && pending.has(msg.id)) { const { resolve, reject } = pending.get(msg.id); pending.delete(msg.id); msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result) }
    if (msg.method === 'Runtime.exceptionThrown') errors.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text)
    if (msg.method === 'Runtime.consoleAPICalled' && msg.params.type === 'error') consoleErrors.push(msg.params.args.map(a => a.value ?? a.description).join(' '))
    if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') consoleErrors.push(msg.params.entry.text)
  })
  const send = (method, params = {}) => new Promise((resolve, reject) => {
    if (closed) return reject(closed)
    const id = ++seq; pending.set(id, { resolve, reject }); ws.send(JSON.stringify({ id, method, params }))
  })
  try {
    await send('Page.enable'); await send('Runtime.enable'); await send('Log.enable')
  } catch (error) {
    process.removeListener('exit', killOnExit)
    ws.close()
    await stopBrowser(state)
    throw error
  }
  const b = {
    errors, consoleErrors, send,
    browserPid: state.child.pid, profileDir: state.profile,
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
    // have loaded); it is deleted once the browser exits so runs do not fill
    // the disk. Callers may await the returned promise; existing callers do not.
    close() {
      process.removeListener('exit', killOnExit)
      disconnect('closed by the test')
      ws.close()
      return stopBrowser(state)
    },
  }
  return b
}

// Opens a page target on this run's browser and its DevTools socket, both bounded.
async function openTarget(state, timeoutMs) {
  const response = await fetch(`http://127.0.0.1:${state.port}/json/new?about:blank`, { method: 'PUT', signal: AbortSignal.timeout(timeoutMs) })
  if (!response.ok) throw new Error(`could not open a page: HTTP ${response.status}`)
  const target = await response.json()
  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => { ws.close(); reject(new Error(`DevTools socket did not open within ${timeoutMs} ms`)) }, timeoutMs)
    ws.addEventListener('open', () => { clearTimeout(timer); resolve() }, { once: true })
    ws.addEventListener('error', () => { clearTimeout(timer); reject(new Error('DevTools socket failed to open')) }, { once: true })
  })
  return ws
}

// Chrome's helper processes can keep writing to the profile for a moment after
// the main process exits, so retry; cleanup must never fail a test run.
async function removeProfile(dir) {
  for (let attempt = 0; attempt < 20; attempt++) {
    try { rmSync(dir, { recursive: true, force: true }); return } catch { await sleep(250) }
  }
  console.warn(`warning: could not remove browser profile ${dir}`)
}

export function reporter() {
  const results = []
  return {
    results,
    check(name, ok, detail = '') { results.push({ name, ok: !!ok, detail }); console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ' — ' + detail : ''}`) },
    save(path) { writeFileSync(path, JSON.stringify(results, null, 1)); const p = results.filter(r => r.ok).length; console.log(`\n${p} of ${results.length} checks passed`); if (p < results.length) process.exitCode = 1 },
  }
}
