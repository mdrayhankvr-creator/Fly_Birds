// Run after suite.mjs and daily-tasks.mjs on the fresh seeded fbqa-cp1b stack.
import { launch, reporter, sleep } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:5173'
const shots = process.env.E2E_SHOTS_DIR || join(here, 'shots', 'diary-comments')
const pw = process.env.DEMO_PASSWORD
const r = reporter(), check = r.check
const b = await launch({ base, shots, port: 9374 })
const js = JSON.stringify
const email = role => `demo.${role}@demo.flybirds.test`
const question = `Could you explain the fractions exercise? ${Date.now().toString().slice(-6)}`
const answer = 'Start with a common denominator, then add the numerators.'
const thread = id => `details[data-diary-comments="${id}"]`
const fill = (selector, value) => b.evaluate(`(() => { const e = document.querySelector(${js(selector)}); if (!e) return false; e.focus(); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(e, ${js(value)}); e.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)
const openThread = async id => {
  if (!await b.waitFor(`!!document.querySelector(${js(thread(id))})`, 15000)) return false
  await b.evaluate(`document.querySelector(${js(thread(id))}).open = true`)
  return b.waitFor(`document.querySelector(${js(thread(id))})?.open && (!!document.querySelector(${js(thread(id) + ' textarea')}) || !!document.querySelector(${js(thread(id) + ' li')}))`, 10000)
}
const shotThread = async (name, id) => { await b.evaluate(`document.querySelector(${js(thread(id))}).scrollIntoView({ block: 'center' })`); await sleep(250); await b.shot(name, false) }

try {
  await b.viewport(375, 812, true)
  await b.login(email('student'), pw)
  const feed = (await b.api('/diary/me?page=1&page_size=20')).body.data.items
  check('demo student has a published diary', feed.length > 0)
  const id = feed[0].id
  await b.go('/student/diary')
  check('student thread is under the entry', await b.waitFor(`!!document.querySelector(${js(thread(id))})`, 15000))
  check('student can open the thread', await openThread(id))
  check('student can ask a question', await fill(`${thread(id)} textarea`, question))
  await b.evaluate(`document.querySelector(${js(thread(id))})?.querySelector('form button')?.click()`)
  check('student question appears in the thread', await b.waitFor(`document.querySelector(${js(thread(id))})?.innerText.includes(${js(question)})`))
  const questionId = (await b.api(`/diary/${id}/comments`)).body.data.find(c => c.body === question)?.id
  check('student question is stored under this diary', !!questionId)

  await b.login(email('teacher'), pw)
  await b.go('/teacher/diary')
  check('teacher list shows the diary', await b.waitFor(`!!document.querySelector('[data-diary-thread="${id}"]')`, 15000))
  check('teacher list has an unread badge', await b.evaluate(`document.querySelector('[data-diary-thread="${id}"]')?.innerText.includes('unread')`))
  check('teacher can open the section thread', await openThread(id))
  check('teacher sees the student question', await b.waitFor(`document.querySelector(${js(thread(id))})?.innerText.includes(${js(question)})`))
  check('teacher opens reply on this question', await b.evaluate(`(() => { const row = [...document.querySelectorAll(${js(thread(id) + ' li')})].find(e => e.querySelector(':scope > p')?.textContent.trim() === ${js(question)}); const button = row?.querySelector('button'); if (!button) return false; button.click(); return true })()`))
  check('teacher can type a reply', await fill(`${thread(id)} textarea`, answer))
  await b.evaluate(`document.querySelector(${js(thread(id))})?.querySelector('form button')?.click()`)
  check('reply belongs to this question', await b.waitFor(`document.querySelector(${js(thread(id))})?.innerText.includes(${js(answer)})`) && (await b.api(`/diary/${id}/comments`)).body.data.some(c => c.parent_id === questionId && c.body === answer))
  check('teacher thread fits 375px', !(await b.overflow()))
  await shotThread('teacher-en-375', id)
  await b.viewport(360, 780, true)
  check('teacher thread fits 360px', !(await b.overflow()))
  await b.viewport(375, 812, true)

  await b.login(email('student'), pw)
  await b.go('/student/diary')
  check('student thread reopens after sign-in', await openThread(id))
  check('student sees the reply to their question', await b.waitFor(`document.querySelector(${js(thread(id))})?.innerText.includes(${js(answer)})`) && (await b.api(`/diary/${id}/comments`)).body.data.some(c => c.parent_id === questionId && c.body === answer))
  const inbox = (await b.api('/notifications?page=1&page_size=50')).body.data.items
  check('student received a diary reply notification', inbox.some(n => n.category === 'diary' && n.title_en === 'Teacher replied to your diary question'))
  check('student thread fits 375px', !(await b.overflow()))
  await shotThread('student-en-375', id)
  await b.viewport(360, 780, true)
  check('student thread fits 360px', !(await b.overflow()))
  await b.viewport(375, 812, true)

  await b.evaluate(`localStorage.setItem('flybirds.theme', 'dark')`)
  await b.login(email('teacher'), pw, 'bn')
  await b.go('/teacher/diary')
  await b.waitFor(`!!document.querySelector(${js(thread(id))})`)
  await openThread(id)
  check('Bangla teacher thread heading', await b.evaluate(`document.querySelector(${js(thread(id) + ' summary')})?.innerText.includes('প্রশ্ন ও উত্তর')`))
  check('teacher dark mode is active', await b.evaluate(`document.documentElement.dataset.theme === 'dark'`))
  check('Bangla teacher thread fits 375px', !(await b.overflow()))
  await shotThread('teacher-bn-dark-375', id)

  await b.login(email('student'), pw, 'bn')
  await b.go('/student/diary')
  await openThread(id)
  check('Bangla student thread heading', await b.evaluate(`document.querySelector(${js(thread(id) + ' summary')})?.innerText.includes('প্রশ্ন ও উত্তর')`))
  check('student dark mode is active', await b.evaluate(`document.documentElement.dataset.theme === 'dark'`))
  check('Bangla student thread fits 375px', !(await b.overflow()))
  await shotThread('student-bn-dark-375', id)
} catch (error) {
  check('script error', false, error.stack)
} finally {
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  r.save(join(shots, 'results.json'))
  await b.close()
}
