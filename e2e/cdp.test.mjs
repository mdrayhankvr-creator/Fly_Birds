// Browser startup and connection handling of cdp.mjs. Run: node --test e2e/cdp.test.mjs
// Most cases use testdata/fake-browser.mjs, so they need no browser; the last
// one starts the real browser and is skipped when none is installed.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { existsSync, mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { after, beforeEach, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { browserPath, launch } from './cdp.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const fake = join(here, 'testdata', 'fake-browser.mjs')
const work = mkdtempSync(join(tmpdir(), 'cdp-test-'))
const shots = join(work, 'shots')
const pidFile = join(work, 'pids')
after(() => rmSync(work, { recursive: true, force: true }))

const profiles = () => new Set(readdirSync(tmpdir()).filter(name => name.startsWith('flybirds-e2e-')))
const alive = pid => { try { process.kill(pid, 0); return true } catch { return false } }
const fakePids = () => existsSync(pidFile) ? readFileSync(pidFile, 'utf8').trim().split('\n').map(Number) : []
const withFake = (mode, options = {}) => {
  process.env.FAKE_BROWSER_MODE = mode
  process.env.FAKE_BROWSER_PIDFILE = pidFile
  return launch({ base: 'about:blank', shots, browser: process.execPath, browserArgs: [fake], ...options })
}
let before
beforeEach(() => { rmSync(pidFile, { force: true }); before = profiles() })
// Every case must leave no fake browser running and no new profile directory.
async function assertCleanedUp() {
  for (let i = 0; i < 50 && (fakePids().some(alive) || [...profiles()].some(p => !before.has(p))); i++) await new Promise(r => setTimeout(r, 100))
  assert.deepEqual(fakePids().filter(alive), [], 'fake browser processes left running')
  assert.deepEqual([...profiles()].filter(p => !before.has(p)), [], 'browser profiles left behind')
}

test('a browser that starts slowly is waited for within the limit', async () => {
  const b = await withFake('delay:1500', { timeoutMs: 10000 })
  assert.equal(await b.evaluate('1 + 1'), 2)
  await b.close()
  await assertCleanedUp()
})

test('a browser that exits at startup fails fast with its own error output', async () => {
  const started = Date.now()
  await assert.rejects(withFake('crash', { attempts: 2 }), error => {
    assert.match(error.message, /Browser did not start/)
    assert.match(error.message, /attempt 1: exited during startup \(code 3\)/)
    assert.match(error.message, /attempt 2: exited during startup \(code 3\)/)
    assert.match(error.message, /cannot open display/)
    return true
  })
  assert.ok(Date.now() - started < 5000, 'crash was not detected promptly')
  assert.equal(fakePids().length, 2)
  await assertCleanedUp()
})

test('a browser that never becomes ready times out and is stopped', async () => {
  const started = Date.now()
  await assert.rejects(withFake('hang', { timeoutMs: 800, attempts: 1 }), /did not become ready within 800 ms/)
  assert.ok(Date.now() - started < 7000, 'readiness was not bounded')
  assert.equal(fakePids().length, 1)
  await assertCleanedUp()
})

test('a failed start is retried once', async () => {
  const marker = join(work, 'fail-once')
  rmSync(marker, { force: true })
  const b = await withFake(`fail-once:${marker}`, { attempts: 2 })
  assert.equal(await b.evaluate('1 + 1'), 2)
  assert.equal(fakePids().length, 2)
  await b.close()
  await assertCleanedUp()
})

test('a lost connection fails commands instead of hanging', async () => {
  // The three startup commands are answered, then the socket drops.
  const b = await withFake('drop:3')
  await assert.rejects(b.evaluate('1 + 1'), /browser connection closed/)
  await assert.rejects(b.send('Page.enable'), /browser connection closed/)
  await b.close()
  await assertCleanedUp()
})

test('a connection lost during startup stops the run', async () => {
  await assert.rejects(withFake('drop:1', { attempts: 1 }), /browser connection closed/)
  await assertCleanedUp()
})

test('a missing executable is reported clearly', async () => {
  await assert.rejects(launch({ base: 'about:blank', shots, browser: join(work, 'no-such-browser'), attempts: 1 }), /could not start: spawn .*ENOENT/)
  const saved = process.env.E2E_BROWSER
  process.env.E2E_BROWSER = join(work, 'no-such-browser')
  try { assert.throws(() => browserPath(), /E2E_BROWSER does not exist/) } finally {
    if (saved === undefined) delete process.env.E2E_BROWSER; else process.env.E2E_BROWSER = saved
  }
})

let installed = null
try { installed = browserPath() } catch { /* no browser on this machine */ }
test('the real browser starts repeatedly even when the legacy port is taken', { skip: !installed && 'no Chromium-based browser installed' }, async () => {
  const squatter = createServer((req, res) => res.end('{"not":"a browser"}'))
  await new Promise(resolve => squatter.listen(9345, '127.0.0.1', resolve))
  try {
    for (let run = 1; run <= 3; run++) {
      const b = await launch({ base: 'about:blank', shots, port: 9345 })
      assert.match(await b.evaluate('navigator.userAgent'), /Headless/, `run ${run} reached another process`)
      const pid = b.browserPid
      await b.close()
      assert.equal(alive(pid), false, `run ${run} left its browser running`)
      assert.equal(existsSync(b.profileDir), false, `run ${run} left its profile`)
    }
  } finally {
    squatter.close()
  }
})
