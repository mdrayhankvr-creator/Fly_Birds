// Run against a fresh demo school: see e2e/README.md.
// UX round 2: the uploaded school logo, students' own photos with approval
// by the class teacher (reject with a reason, then approve), the photo
// setting, and login slips printed eight to an A4 page.
//
// It changes the disposable demo school: it uploads a logo (removed again at
// the end), makes demo.teacher the class teacher of the demo student's
// section (restored at the end), approves a photo for the demo student and
// creates one student login.
import { launch, reporter, sleep } from './cdp.mjs'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { deflateSync } from 'node:zlib'
const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:5173'
const pw = process.env.DEMO_PASSWORD
const r = reporter(), check = r.check
const shots = join(here, 'shots', 'ux-round2')
mkdirSync(shots, { recursive: true })
const b = await launch({ base, shots })
const email = who => `demo.${who}@demo.flybirds.test`
const js = v => JSON.stringify(v)

// API calls as a role, outside the browser.
async function api(role) {
  const res = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: js({ email: email(role), password: pw }) })
  if (res.status !== 200) throw new Error(`sign-in ${role}: ${res.status}`)
  const token = (await res.json()).data.access_token
  const call = async (method, path, body) => {
    const x = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? js(body) : undefined })
    const j = await x.json().catch(() => ({}))
    return { status: x.status, data: j.data, code: j.error?.code }
  }
  // raw: a file's status and a fingerprint of its bytes.
  call.raw = async path => { const x = await fetch(`${base}/api${path}`, { headers: { Authorization: `Bearer ${token}` } }); const buf = Buffer.from(await x.arrayBuffer()); return `${x.status}:${buf.length}:${buf.subarray(0, 64).toString('hex')}` }
  return call
}

// Test pictures: a photo and a logo with a transparent background.
function png(w, h, pixel) {
  const crc = buf => { let c, t = []; for (let n = 0; n < 256; n++) { c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; t[n] = c >>> 0 } let x = 0xffffffff; for (const v of buf) x = t[(x ^ v) & 255] ^ (x >>> 8); return (x ^ 0xffffffff) >>> 0 }
  const chunk = (type, data) => { const len = Buffer.alloc(4); len.writeUInt32BE(data.length); const td = Buffer.concat([Buffer.from(type), data]); const c = Buffer.alloc(4); c.writeUInt32BE(crc(td)); return Buffer.concat([len, td, c]) }
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 6
  const rows = []
  for (let y = 0; y < h; y++) { const row = Buffer.alloc(1 + w * 4); for (let x = 0; x < w; x++) row.set(pixel(x, y), 1 + x * 4); rows.push(row) }
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(Buffer.concat(rows))), chunk('IEND', Buffer.alloc(0))])
}
const photoPath = join(shots, 'photo.png'), logoPath = join(shots, 'logo.png')
writeFileSync(photoPath, png(800, 600, (x, y) => { const d = (x - 400) ** 2 / 1.4 + (y - 280) ** 2; return d < 160 ** 2 ? [222, 184, 150, 255] : [220, 232, 240, 255] }))
writeFileSync(logoPath, png(600, 300, (x, y) => (x - 150) ** 2 + (y - 150) ** 2 < 130 ** 2 ? [15, 118, 110, 255] : [0, 0, 0, 0]))

async function setFile(selector, path) {
  const { root } = await b.send('DOM.getDocument', { depth: -1 })
  const { nodeId } = await b.send('DOM.querySelector', { nodeId: root.nodeId, selector })
  if (!nodeId) throw new Error(`no ${selector}`)
  await b.send('DOM.setFileInputFiles', { nodeId, files: [path] })
}
const text = () => b.evaluate(`document.querySelector('main')?.innerText ?? ''`)
const has = s => b.evaluate(`(document.querySelector('main')?.innerText ?? '').includes(${js(s)})`)
const click = selector => b.evaluate(`(() => { const el = document.querySelector(${js(selector)}); if (!el || el.disabled) return false; el.click(); return true })()`)
const clickText = (label, scope = 'main') => b.evaluate(`(() => { const el = [...document.querySelectorAll(${js(scope + ' button')})].find(e => e.offsetParent !== null && e.innerText.trim() === ${js(label)}); if (!el || el.disabled) return false; el.click(); return true })()`)

const admin = await api('admin'), student = await api('student'), teacher = await api('teacher')
const me = (await student('GET', '/students/me')).data
const settingsBefore = (await admin('GET', '/school/settings')).data
const assignments = (await admin('GET', `/academic/class-teachers?session_id=${me.session_id}`)).data ?? []
// The photo in use, as a fingerprint (unchanged until a photo is approved).
const photoInUse = () => student.raw(`/students/${me.id}/photo`)
const ctBefore = assignments.find(s => s.section_id === me.section_id)?.employee_id ?? null
let created = null
try {
  await b.viewport(1366, 900)

  // --- The logo ---------------------------------------------------------
  await b.login(email('admin'), pw)
  await b.go('/admin/school-settings')
  await b.waitFor(`!!document.querySelector('[data-logo-input]')`, 20000)
  check('settings: the student photo setting is shown and on', await b.evaluate(`document.querySelector('[data-photo-setting]')?.checked === true`))
  await setFile('[data-logo-input]', logoPath)
  await b.waitFor(`(document.querySelector('[data-logo-field]')?.innerText ?? '').includes('Uploaded logo')`, 20000).catch(() => null)
  const branding = await (await fetch(`${base}/api/school/branding`)).json()
  const logoURL = branding.data.logo_url
  const served = await fetch(`${base}${logoURL}`)
  const bytes = Buffer.from(await served.arrayBuffer())
  check('logo: uploaded from settings, logo_url points at it', /^\/api\/school\/logo\?v=[0-9a-f]{12}$/.test(logoURL), logoURL)
  check('logo: served publicly as a PNG (transparency kept), resized to 512 px', served.status === 200 && served.headers.get('content-type') === 'image/png' && bytes.readUInt32BE(16) === 512, `${served.status} ${served.headers.get('content-type')} ${bytes.length ? bytes.readUInt32BE(16) : 0}px`)
  await b.shot('settings-logo', false)
  await b.go('/')
  await b.waitFor(`[...document.querySelectorAll('img')].some(i => i.getAttribute('src') === ${js(logoURL)} && i.complete && i.naturalWidth > 0)`, 15000).catch(() => null)
  check('logo: the public pages show it', await b.evaluate(`[...document.querySelectorAll('img')].some(i => i.getAttribute('src') === ${js(logoURL)} && i.naturalWidth > 0)`))

  // --- Student photo: send, reject with a reason ---------------------------
  const emp = (await teacher('GET', '/staff/me')).data
  const assigned = await admin('PUT', `/academic/sections/${me.section_id}/class-teacher`, { employee_id: emp.id })
  check('setup: demo.teacher is the class teacher of the student\'s section', assigned.status === 200, `${assigned.status} ${assigned.code ?? ''}`)

  const photoBefore = await photoInUse()
  await b.login(email('student'), pw)
  await b.go('/student/me')
  await b.waitFor(`!!document.querySelector('[data-my-photo] [data-photo-input]')`, 20000)
  await setFile('[data-photo-input]', photoPath)
  await b.waitFor(`!!document.querySelector('[data-photo-cropper] canvas') && !!document.querySelector('[data-photo-submit]')`, 15000)
  check('student: the crop step shows the photo guidance', await b.evaluate(`(document.querySelector('[data-photo-cropper]')?.innerText ?? '').includes('For a good photo')`))
  // Zoom in a little and move with the keyboard, as a student would.
  await b.evaluate(`(() => { const r = document.querySelector('[data-photo-cropper] input[type=range]'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(r, '1.4'); r.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)
  await b.shot('student-crop', false)
  await click('[data-photo-submit]')
  await b.waitFor(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('Waiting for approval')`, 15000).catch(() => null)
  check('student: the photo waits for approval', await b.evaluate(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('Waiting for approval')`))
  check('student: the photo in use is unchanged before approval', (await photoInUse()) === photoBefore, `${photoBefore} -> ${await photoInUse()}`)

  await b.login(email('teacher'), pw)
  await b.go('/dashboard')
  await b.waitFor(`[...document.querySelectorAll('aside nav a')].length > 0`, 15000)
  check('teacher: the menu has Photo approvals', await b.evaluate(`[...document.querySelectorAll('aside nav a')].some(a => a.getAttribute('href') === '/photo-reviews')`))
  await b.go('/photo-reviews')
  await b.waitFor(`!!document.querySelector('[data-photo-review="${me.id}"] img')`, 20000).catch(() => null)
  check('teacher: the class teacher sees the photo', await b.evaluate(`!!document.querySelector('[data-photo-review="${me.id}"] img')`))
  await b.shot('teacher-review', false)
  await clickText('Reject', `[data-photo-review="${me.id}"]`)
  await sleep(300)
  await clickText('Please use a plain, light background.', `[data-photo-review="${me.id}"]`)
  await sleep(200)
  await clickText('Reject photo', `[data-photo-review="${me.id}"]`)
  await b.waitFor(`(document.querySelector('main')?.innerText ?? '').includes('was rejected')`, 15000).catch(() => null)
  check('teacher: rejects with a reason', await has('was rejected'))

  await b.login(email('student'), pw)
  await b.go('/student/me')
  await b.waitFor(`!!document.querySelector('[data-photo-rejected]')`, 20000).catch(() => null)
  check('student: sees the rejection and its reason', await b.evaluate(`(document.querySelector('[data-photo-rejected]')?.innerText ?? '').includes('Please use a plain, light background.')`))
  await setFile('[data-photo-input]', photoPath)
  await b.waitFor(`!!document.querySelector('[data-photo-submit]')`, 15000)
  await click('[data-photo-submit]')
  await b.waitFor(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('Waiting for approval')`, 15000).catch(() => null)
  check('student: a new photo waits again', await b.evaluate(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('Waiting for approval')`))

  // --- Approval by the administrator --------------------------------------
  await b.login(email('admin'), pw)
  await b.go('/photo-reviews')
  await b.waitFor(`!!document.querySelector('[data-photo-review="${me.id}"]')`, 20000).catch(() => null)
  await clickText('Approve', `[data-photo-review="${me.id}"]`)
  await b.waitFor(`(document.querySelector('main')?.innerText ?? '').includes('is approved and in use')`, 15000).catch(() => null)
  check('admin: approves it', await has('is approved and in use'))
  const state = (await student('GET', '/students/me/photo-submission')).data
  const inUse = await student('GET', `/students/${me.id}/photo`)
  check('the approved photo is the student\'s photo (profile, ID card, marksheet, admit card read it)', state.status === 'approved' && state.has_photo && inUse.status === 200, `${state.status} ${state.has_photo} ${inUse.status}`)
  await b.login(email('student'), pw)
  await b.go('/student/me')
  await b.waitFor(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('Approved')`, 15000).catch(() => null)
  check('student: sees it approved', await b.evaluate(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('Approved')`))

  // --- The setting turned off ------------------------------------------------
  const { updated_at: _u, ...settingsNow } = (await admin('GET', '/school/settings')).data
  for (const k of ['eiin', 'founded_year', 'address_bn', 'map_url', 'head_teacher_name_en', 'head_teacher_name_bn']) delete settingsNow[k]
  await admin('PUT', '/school/settings', { ...settingsNow, student_photo_uploads: false })
  await b.go('/student/me')
  await b.waitFor(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('turned photo uploads off')`, 15000).catch(() => null)
  check('setting off: the student cannot upload and is told why', await b.evaluate(`(document.querySelector('[data-my-photo]')?.innerText ?? '').includes('turned photo uploads off') && !document.querySelector('[data-photo-input]')`))
  await admin('PUT', '/school/settings', { ...settingsNow, student_photo_uploads: true })

  // --- Login slips, eight to an A4 page ---------------------------------------
  const without = (await admin('GET', '/students?without_login=true&page=1&page_size=1')).data?.items?.[0]
  if (!without) throw new Error('no student without a login in the demo school')
  created = without.id
  await b.login(email('admin'), pw)
  await b.go(`/admin/students/${without.id}`)
  await b.waitFor(`[...document.querySelectorAll('main button')].some(e => e.innerText.trim() === 'Create login')`, 20000)
  await clickText('Create login')
  await b.waitFor(`!!document.querySelector('[data-slip-layout]')`, 20000)
  await b.evaluate(`(() => { const s = document.querySelector('[data-slip-layout]'); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, 'eight'); s.dispatchEvent(new Event('change', { bubbles: true })); return true })()`)
  await b.waitFor(`!!document.querySelector('.login-slips-eight .login-slips-page')`, 5000)
  // Print ten slips (the one made here, copied): two A4 pages, none cut.
  await b.evaluate(`(() => { const root = document.querySelector('.login-slips-eight'); const slip = root.querySelector('.login-slip'); const page = root.querySelector('.login-slips-page'); for (let i = 0; i < 7; i++) page.appendChild(slip.cloneNode(true)); const second = page.cloneNode(false); for (let i = 0; i < 2; i++) second.appendChild(slip.cloneNode(true)); root.appendChild(second); return true })()`)
  await b.send('Emulation.setEmulatedMedia', { media: 'print' })
  const fit = await b.evaluate(`(() => { const mm = 96 / 25.4; return [...document.querySelectorAll('.login-slips-eight .login-slip')].map(s => ({ h: s.getBoundingClientRect().height / mm, over: s.scrollHeight > s.clientHeight + 1 })) })()`)
  await b.send('Emulation.setEmulatedMedia', { media: '' })
  const pdf = await b.send('Page.printToPDF', { preferCSSPageSize: true, printBackground: true })
  const raw = Buffer.from(pdf.data, 'base64')
  writeFileSync(join(shots, 'slips-eight.pdf'), raw)
  const pages = (raw.toString('latin1').match(/\/Type\s*\/Page[^s]/g) || []).length
  check('slips: ten compact slips print on two A4 pages', pages === 2, `${pages} pages`)
  check('slips: every compact slip fits its 67 mm cell', fit.length === 10 && fit.every(f => f.h <= 67.5 && !f.over), fit.map(f => `${f.h.toFixed(1)}${f.over ? '!' : ''}`).join(' '))

  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
} catch (e) {
  check('ux round 2 run completed', false, e.stack)
} finally {
  // Put the school back: the logo, the class teacher, the setting.
  try {
    await admin('DELETE', '/school/logo')
    if (settingsBefore.logo_url && !settingsBefore.logo_url.startsWith('/api/school/logo')) {
      const { updated_at: _u, ...s } = (await admin('GET', '/school/settings')).data
      for (const k of ['eiin', 'founded_year', 'address_bn', 'map_url', 'head_teacher_name_en', 'head_teacher_name_bn']) delete s[k]
      await admin('PUT', '/school/settings', { ...s, logo_url: settingsBefore.logo_url, student_photo_uploads: settingsBefore.student_photo_uploads })
    }
    await admin('PUT', `/academic/sections/${me.section_id}/class-teacher`, { employee_id: ctBefore })
  } catch (e) { console.log('cleanup:', e.message) }
  r.save(join(here, 'shots', 'ux-round2-results.json'))
  await b.close()
}
