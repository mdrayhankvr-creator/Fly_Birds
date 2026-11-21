// Run only against a newly seeded, disposable local stack on port 15173.
// The script creates a room, a draft and a published routine in that test DB.
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { launch, reporter, sleep } from './cdp.mjs'

const base = process.env.ROUTINE_PLANNER_E2E_BASE_URL
const password = process.env.DEMO_PASSWORD
const url = base && new URL(base)
if (!url || url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.port !== '15173' || url.pathname !== '/' || !password) {
  throw new Error('Use a disposable local 127.0.0.1:15173 stack and set DEMO_PASSWORD')
}
const report = reporter()
const check = (name, ok) => report.check(name, ok)
const json = value => JSON.stringify(value)
const login = await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: json({ email: 'demo.admin@demo.flybirds.test', password }) })
if (!login.ok) throw new Error(`disposable demo login failed: ${login.status}`)
const token = (await login.json()).data.access_token
async function api(method, path, body) {
  const response = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? json(body) : undefined })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${data.error?.code ?? ''}`)
  return data.data
}

const sessions = await api('GET', '/academic/sessions?page=1&page_size=100')
const session = sessions.items.find(item => item.is_active)
if (!session) throw new Error('disposable demo has no active session')
const empty = { session_id: session.id, config: { start_time: '', end_time: '', lunch_start: '', lunch_minutes: 45, period_minutes: 0, transition_minutes: 0, days: [], availability_confirmed: false, frequencies: [], unavailable: [] }, slots: [] }
let initial = await api('POST', '/routines/class-planner/preview', empty)
check('incomplete preview reports missing inputs', !initial.can_publish && initial.issues.some(issue => issue.code === 'invalid_day_config'))
const assignment = initial.catalogue.assignments.find(item => initial.catalogue.sections.some(section => section.class_id === item.class_id))
if (!assignment) throw new Error('disposable demo has no assigned teacher for an active class')
const sections = initial.catalogue.sections.filter(section => section.class_id === assignment.class_id)
const roomName = `Planner test room ${Date.now().toString(36)}`
await api('POST', '/routines/rooms', { name: roomName, name_bn: 'পরীক্ষা কক্ষ', kind: 'classroom', capacity: 60, is_active: true })
const times = [['08:00', '08:45'], ['08:45', '09:30'], ['09:30', '10:15'], ['10:15', '11:00'], ['12:00', '12:45'], ['12:45', '13:30']]
if (sections.length > times.length * 5) throw new Error('disposable fixture has more sections than the test grid can hold')
const plan = {
  session_id: session.id,
  config: { start_time: '08:00', end_time: '14:00', lunch_start: '11:15', lunch_minutes: 45, period_minutes: 45, transition_minutes: 0, days: [0, 1, 2, 3, 4], availability_confirmed: true,
    frequencies: initial.catalogue.subjects.map(subject => ({ class_id: subject.class_id, subject_id: subject.id, periods_per_week: subject.class_id === assignment.class_id && subject.id === assignment.subject_id ? 1 : 0 })), unavailable: [] },
  slots: sections.map((section, index) => ({ session_id: session.id, class_id: section.class_id, section_id: section.id, subject_id: assignment.subject_id, teacher_id: assignment.teacher_id, day_of_week: Math.floor(index / times.length), start_time: times[index % times.length][0], end_time: times[index % times.length][1], room: roomName })),
}
const ready = await api('POST', '/routines/class-planner/preview', plan)
if (!ready.can_publish) throw new Error(`isolated plan rejected: ${ready.issues.map(issue => issue.code).join(', ')}`)
await api('PUT', '/routines/class-planner/draft', { plan, preview_digest: ready.digest })

const b = await launch({ base, shots: join(tmpdir(), `flybirds-planner-browser-${Date.now()}`) })
try {
  await b.login('demo.admin@demo.flybirds.test', password)
  await b.go('/admin/routines?tab=class_planner')
  check('planner opens for administrator', await b.waitFor(`document.body.innerText.includes('Class routine planner')`))
  check('saved draft loads', await b.waitFor(`document.body.innerText.includes('Draft version 1')`))
  await b.clickText('Preview without saving')
  check('browser shows valid preview and 45-minute lunch', await b.waitFor(`document.body.innerText.includes('Ready to publish') && document.body.innerText.includes('45 minutes')`))
  check('combined and parallel roster editor is available', await b.waitFor(`document.body.innerText.includes('Combined and parallel lessons') && document.body.innerText.includes('Explicit student roster')`))
  const setSelect = async (index, value) => {
    const ok = await b.evaluate(`(() => {
      const section = [...document.querySelectorAll('section')].find(node => node.querySelector('h3')?.textContent === 'Place lessons')
      const el = section?.querySelectorAll('select')[${index}]
      if (!el || ![...el.options].some(option => option.value === ${json(value)})) return false
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(el, ${json(value)})
      el.dispatchEvent(new Event('change', { bubbles: true }))
      return true
    })()`)
    if (!ok) throw new Error(`manual lesson select ${index} could not be set`)
    await sleep(100)
  }
  await setSelect(0, plan.slots[0].section_id)
  await setSelect(1, assignment.subject_id)
  await setSelect(2, assignment.teacher_id)
  await setSelect(3, `${plan.slots[0].day_of_week}|${plan.slots[0].start_time}|${plan.slots[0].end_time}`)
  await setSelect(4, roomName)
  check('manual lesson can be placed', await b.clickText('Add lesson'))
  await b.clickText('Preview without saving')
  check('browser displays overlap warning', await b.waitFor(`document.body.innerText.includes('A teacher, section or room overlaps.')`))
  await b.go('/admin/routines?tab=class_planner')
  check('saved plan restores after unsaved conflict', await b.waitFor(`document.body.innerText.includes('Draft version 1')`))
  await b.clickText('Preview without saving')
  check('valid draft preview restores', await b.waitFor(`document.body.innerText.includes('Ready to publish')`))
  await b.clickText('Publish version')
  check('publish requires browser confirmation', await b.waitFor(`!!document.querySelector('[role=alertdialog]') && document.querySelector('#confirm-body')?.textContent.includes('replace')`))
  await b.evaluate(`document.querySelector('[role=alertdialog] button')?.click()`)
  const stillDraft = await api('GET', `/routines/class-planner/draft?session_id=${session.id}`)
  check('cancelling confirmation leaves draft unpublished', stillDraft.state === 'draft')
  await b.clickText('Publish version')
  await b.waitFor(`!!document.querySelector('[role=alertdialog]')`)
  await b.evaluate(`document.querySelectorAll('[role=alertdialog] button')[1]?.click()`)
  check('confirmed publication completes in disposable school', await b.waitFor(`document.body.innerText.includes('Version 1 published.')`))
  const current = await api('GET', `/routines/classes?session_id=${session.id}&page=1&page_size=100`)
  check('published rows match manual plan', current.total === plan.slots.length)
  await b.go('/admin/exams/setup')
  check('Religion Education setup tab is available', await b.waitFor(`document.body.innerText.includes('Religion Education')`))
  await b.clickText('Religion Education')
  check('Admin course roster opens with privacy guidance', await b.waitFor(`document.body.innerText.includes('Student Religion Education courses') && document.body.innerText.includes('does not record religious affiliation')`))
  await b.viewport(375, 812, true)
  check('course setup fits a 375px screen', !(await b.overflow()))
  await b.go('/admin/routines?tab=class_planner')
  check('planner fits a 375px screen', await b.waitFor(`document.body.innerText.includes('Class routine planner')`) && !(await b.overflow()))
  await b.evaluate(`localStorage.setItem('flybirds.language', 'bn')`)
  await b.go('/admin/routines?tab=class_planner')
  check('Bangla planner opens on a phone', await b.waitFor(`document.body.innerText.includes('শ্রেণি রুটিন পরিকল্পনাকারী')`) && !(await b.overflow()))
} finally {
  await b.close()
}
report.save(join(tmpdir(), `flybirds-planner-browser-result-${Date.now()}.json`))
