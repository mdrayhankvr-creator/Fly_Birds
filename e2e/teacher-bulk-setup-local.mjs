// Isolated teacher setup browser runner. No live stack or stored credentials.
import { spawn, spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createServer } from 'node:net'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const backend = join(root, 'backend'), frontend = join(root, 'frontend')
const suffix = randomBytes(6).toString('hex')
const role = `m15_teacher_browser_${suffix}`, database = `teacher_browser_test_${suffix}`
const dbPassword = randomBytes(32).toString('hex'), demoPassword = `LocalTest!${randomBytes(18).toString('hex')}`
const bin = join(tmpdir(), `flybirds-teacher-setup-${suffix}`)
const exe = name => join(bin, process.platform === 'win32' ? `${name}.exe` : name)
const dbURL = `postgresql://${role}:${dbPassword}@127.0.0.1:55415/${database}?sslmode=disable`
const docker = (args, input = '') => {
  const result = spawnSync('docker', args, { input, encoding: 'utf8', windowsHide: true })
  if (result.status !== 0) throw new Error(`disposable Docker command failed: ${args.slice(0, 3).join(' ')}`)
  return result.stdout.trim()
}
const sql = (name, statement) => docker(['exec', '-i', 'm15-testdb', 'sh', '-c', `psql -U "$POSTGRES_USER" -d ${name} -v ON_ERROR_STOP=1 -q`], statement)
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
  throw new Error(`isolated process did not become ready at ${url}`)
}
function start(name, args, cwd, env) {
  const child = spawn(name, args, { cwd, env, stdio: 'ignore', windowsHide: true })
  child.once('error', () => {})
  return child
}

let api, vite, createdRole = false
try {
  if (docker(['port', 'm15-testdb', '5432']) !== '127.0.0.1:55415') throw new Error('m15-testdb must publish only on 127.0.0.1:55415')
  await free(18085); await free(15174)
  sql('postgres', `CREATE ROLE ${role} LOGIN PASSWORD '${dbPassword}' SUPERUSER;`)
  createdRole = true
  sql('postgres', `CREATE DATABASE ${database} OWNER ${role};`)
  mkdirSync(bin, { recursive: true })
  run('go', ['build', '-o', exe('seed-demo'), './cmd/seed-demo'], backend, process.env)
  run('go', ['build', '-o', exe('api'), './cmd/api'], backend, process.env)
  const env = { ...process.env, DATABASE_URL: dbURL, DEMO_PASSWORD: demoPassword, JWT_SIGNING_KEY_B64: randomBytes(48).toString('base64'), AUTH_COOKIE_SECURE: 'false', PORT: '18085', NOTIFY_INTERVAL: '0', FILE_STORAGE_DIR: join(bin, 'uploads'), BACKUP_DIR: join(bin, 'backups'), GIN_MODE: 'release' }
  run(exe('seed-demo'), ['-confirm'], backend, env)
  // Fixture accounts exist only inside this newly created disposable database.
  sql(database, `INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES
    ('mdrayhankvr@gmail.com','fixture-unusable','admin','Protected fixture',true),
    ('2023331031@student.sust.edu','fixture-unusable','admin','Protected fixture',true);`)
  api = start(exe('api'), [], backend, env)
  await ready('http://127.0.0.1:18085/readyz')
  vite = start(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '15174', '--strictPort'], frontend, { ...process.env, VITE_API_PROXY_TARGET: 'http://127.0.0.1:18085' })
  await ready('http://127.0.0.1:15174/')
  const browser = spawn(process.execPath, [join(root, 'e2e', 'teacher-bulk-setup.mjs')], { cwd: root, env: { ...process.env, TEACHER_SETUP_E2E_BASE_URL: 'http://127.0.0.1:15174', DEMO_PASSWORD: demoPassword, E2E_SHOTS_DIR: join(bin, 'shots'), E2E_RESULTS: join(bin, 'results.json') }, stdio: 'inherit', windowsHide: true })
  const code = await new Promise((ok, fail) => { browser.once('error', fail); browser.once('exit', ok) })
  if (code !== 0) throw new Error(`isolated browser checks failed with exit ${code}`)
} finally {
  if (vite && !vite.killed) vite.kill()
  if (api && !api.killed) api.kill()
  if (createdRole) sql('postgres', `ALTER ROLE ${role} NOLOGIN;`)
}
