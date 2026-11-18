// Self-contained local runner for class-routine-planner.mjs. It creates a
// uniquely named disposable database in m15-testdb and disables its login
// when done. It never connects to a school stack or reads stored credentials.
import { spawn, spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createServer } from 'node:net'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const backend = join(root, 'backend')
const frontend = join(root, 'frontend')
const suffix = randomBytes(6).toString('hex')
const role = `m15_browser_${suffix}`
const database = `flybirds_browser_test_${suffix}`
const dbPassword = randomBytes(32).toString('hex')
const demoPassword = `LocalTest!${randomBytes(18).toString('hex')}`
const bin = join(tmpdir(), `flybirds-planner-${suffix}`)
const exe = name => join(bin, process.platform === 'win32' ? `${name}.exe` : name)
const dbURL = `postgresql://${role}:${dbPassword}@127.0.0.1:55415/${database}?sslmode=disable`
const docker = (args, input = '') => {
  const result = spawnSync('docker', args, { input, encoding: 'utf8', windowsHide: true })
  if (result.status !== 0) throw new Error(`disposable Docker command failed: ${args.slice(0, 3).join(' ')}`)
  return result.stdout.trim()
}
const sql = statement => docker(['exec', '-i', 'm15-testdb', 'sh', '-c', 'psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 -q'], statement)
const run = (name, args, cwd, env) => {
  const result = spawnSync(name, args, { cwd, env, encoding: 'utf8', windowsHide: true, maxBuffer: 8 * 1024 * 1024 })
  if (result.status !== 0) throw new Error(`isolated ${name} ${args[0]} failed`)
}
async function free(port) {
  const server = createServer()
  await new Promise((ok, fail) => server.once('error', fail).listen(port, '127.0.0.1', ok))
  await new Promise(ok => server.close(ok))
}
async function ready(url) {
  for (let i = 0; i < 90; i++) {
    try { if ((await fetch(url, { signal: AbortSignal.timeout(1000) })).ok) return } catch { /* starting */ }
    await new Promise(ok => setTimeout(ok, 500))
  }
  throw new Error(`isolated local process did not become ready at ${url}`)
}
function start(name, args, cwd, env) {
  const child = spawn(name, args, { cwd, env, stdio: 'ignore', windowsHide: true })
  child.once('error', () => {})
  return child
}
function stop(child) { if (child && !child.killed) child.kill() }

let api, vite, createdRole = false
try {
  const publishedPort = docker(['port', 'm15-testdb', '5432'])
  if (!publishedPort.split(/\r?\n/).some(line => line === '127.0.0.1:55415')) throw new Error('m15-testdb must publish PostgreSQL only on 127.0.0.1:55415')
  await free(18084); await free(15173)
  sql(`CREATE ROLE ${role} LOGIN PASSWORD '${dbPassword}' SUPERUSER;`)
  createdRole = true
  sql(`CREATE DATABASE ${database} OWNER ${role};`)
  mkdirSync(bin, { recursive: true })
  run('go', ['build', '-o', exe('seed-demo'), './cmd/seed-demo'], backend, process.env)
  run('go', ['build', '-o', exe('api'), './cmd/api'], backend, process.env)
  const backendEnv = { ...process.env, DATABASE_URL: dbURL, DEMO_PASSWORD: demoPassword, JWT_SIGNING_KEY_B64: randomBytes(48).toString('base64'), AUTH_COOKIE_SECURE: 'false', PORT: '18084', NOTIFY_INTERVAL: '0', FILE_STORAGE_DIR: join(bin, 'uploads'), BACKUP_DIR: join(bin, 'backups'), GIN_MODE: 'release' }
  run(exe('seed-demo'), ['-confirm'], backend, backendEnv)
  api = start(exe('api'), [], backend, backendEnv)
  await ready('http://127.0.0.1:18084/readyz')
  vite = start(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '15173', '--strictPort'], frontend, { ...process.env, VITE_API_PROXY_TARGET: 'http://127.0.0.1:18084' })
  await ready('http://127.0.0.1:15173/')
  const browser = spawn(process.execPath, [join(root, 'e2e', 'class-routine-planner.mjs')], { cwd: root, env: { ...process.env, ROUTINE_PLANNER_E2E_BASE_URL: 'http://127.0.0.1:15173', DEMO_PASSWORD: demoPassword }, stdio: 'inherit', windowsHide: true })
  const code = await new Promise((ok, fail) => { browser.once('error', fail); browser.once('exit', ok) })
  if (code !== 0) throw new Error(`isolated browser checks failed with exit ${code}`)
} finally {
  stop(vite); stop(api)
  if (createdRole) sql(`ALTER ROLE ${role} NOLOGIN;`)
}
