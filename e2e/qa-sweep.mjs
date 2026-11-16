// Visit every sidebar destination for each seeded role at desktop and phone
// widths. QA_SWEEP_MATRIX=1 also checks every page in EN/BN and light/dark.
// Run against a fresh demo seed with DEMO_PASSWORD and E2E_BASE_URL set.
import { launch, sleep } from './cdp.mjs'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const out = process.env.QA_SWEEP_OUTPUT || join(root, 'shots', 'qa-sweep')
mkdirSync(out, { recursive: true })
const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('DEMO_PASSWORD is required')
const roles = (process.env.QA_SWEEP_ROLES || 'admin,head,teacher,class_teacher,accounts,student').split(',')
const matrix = process.env.QA_SWEEP_MATRIX === '1'
const variants = matrix
  ? [{ lang: 'en', theme: 'light' }, { lang: 'bn', theme: 'light' }, { lang: 'en', theme: 'dark' }, { lang: 'bn', theme: 'dark' }]
  : [{ lang: 'en', theme: 'light' }]
const browser = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173', shots: out })
const findings = []
const pages = []
let checks = 0

const settle = async () => {
  await browser.waitFor(`document.readyState === 'complete' && ![...document.querySelectorAll('main .animate-pulse, main [aria-busy=true]')].some(el => !el.closest('details:not([open])') && el.getClientRects().length > 0)`, 12000)
  await browser.waitFor(`(document.querySelector('main')?.innerText.trim().length || 0) > 15`, 3000)
  await sleep(300)
}
const state = (lang, theme) => browser.evaluate(`(() => {
  localStorage.setItem('flybirds.language', ${JSON.stringify(lang)})
  localStorage.setItem('flybirds.theme', ${JSON.stringify(theme)})
  return true
})()`)
const inspect = () => browser.evaluate(String.raw`(() => {
  const main = document.querySelector('main') || document.body
  const text = main.innerText || ''
  const raw = [...new Set((text.match(/\b(?:m\d+|c\d+|p\d+|ux|pr|e\d+|staff|studentAttendance|attendance|fee|school|admin|auth|login)[A-Z][A-Za-z0-9_]*\b|\{\{[^}]+\}\}/g) || []))].slice(0, 8)
  const width = window.innerWidth
  const overflow = document.documentElement.scrollWidth - width
  const heading = main.querySelector('h1, h2')?.innerText.trim() || ''
  const empty = text.trim().length < 15
  const links = [...main.querySelectorAll('a[href]')].filter(a => a.offsetParent !== null).map(a => ({ text: a.innerText.trim(), href: a.getAttribute('href') })).slice(0, 40)
  const buttons = [...main.querySelectorAll('button')].filter(b => b.offsetParent !== null && !b.disabled).map(b => b.innerText.trim() || b.getAttribute('aria-label') || '').filter(Boolean).slice(0, 40)
  return { path: location.pathname, heading, raw, overflow, empty, links, buttons, excerpt: text.trim().slice(0, 150) }
})()`)

async function visit(role, path, width, variant) {
  await browser.viewport(width, width === 375 ? 812 : 900, width === 375)
  browser.errors.length = 0
  browser.consoleErrors.length = 0
  browser.networkErrors.length = 0
  const start = Date.now()
  await browser.evaluate(`(() => {
    if (location.pathname === ${JSON.stringify(path)}) return
    const link = [...document.querySelectorAll('a[href]')].find(a => a.getAttribute('href') === ${JSON.stringify(path)})
    if (link) link.click()
    else { history.pushState({}, '', ${JSON.stringify(path)}); dispatchEvent(new PopStateEvent('popstate')) }
  })()`)
  await browser.waitFor(path === '/dashboard'
    ? `location.pathname === '/dashboard' || location.pathname.startsWith('/dashboard/')`
    : `location.pathname === ${JSON.stringify(path)}`, 12000)
  await settle()
  const ms = Date.now() - start
  const data = await inspect()
  const context = { role, path, width, ...variant }
  const issues = []
  if (data.path.startsWith('/login')) issues.push('redirected to login')
  if (data.overflow > 1) issues.push(`sideways scroll +${data.overflow}px`)
  if (data.raw.length) issues.push(`raw keys: ${data.raw.join(', ')}`)
  if (data.empty) issues.push('empty page without content')
  if (ms > 3000) issues.push(`slow page ${ms}ms`)
  for (const error of browser.errors) issues.push(`JS: ${error.slice(0, 200)}`)
  for (const error of browser.consoleErrors) {
    if (error.startsWith('Failed to load resource:') && browser.networkErrors.length) continue
    issues.push(`console: ${error.slice(0, 200)}`)
  }
  for (const error of browser.networkErrors) {
    const url = new URL(error.url)
    issues.push(`HTTP ${error.status} ${url.pathname}${url.search}`)
  }
  findings.push(...issues.map(issue => ({ ...context, issue, blocking: !issue.startsWith('slow page') && !issue.startsWith('empty page') })))
  pages.push({ ...context, ms, heading: data.heading, excerpt: data.excerpt, links: data.links, buttons: data.buttons, issues: issues.length })
  checks++
  const tag = `${role} ${width} ${variant.lang}/${variant.theme} ${path}`
  const status = issues.some(issue => !issue.startsWith('slow page') && !issue.startsWith('empty page')) ? 'FAIL' : issues.length ? 'NOTE' : 'PASS'
  console.log(`${status} ${tag} ${ms}ms${issues.length ? ' — ' + issues.join(' | ') : ''}`)
  return data
}

try {
  if (roles.includes('class_teacher')) {
    await browser.login('demo.admin@demo.flybirds.test', password)
    const sessions = await browser.api('/academic/sessions?page=1&page_size=100')
    const session = sessions.body.data.items.find(x => x.is_active)
    const sections = await browser.api(`/academic/class-teachers?session_id=${session.id}`)
    const teachers = await browser.api('/staff/employees?page=1&page_size=100&kind=teacher&status=active')
    const teacher = teachers.body.data.items.find(x => x.employee_code === 'DEMO-T01')
    const section = sections.body.data.find(x => x.employee_id === teacher?.id) || sections.body.data.find(x => !x.employee_id)
    if (!section || !teacher) throw new Error('Could not prepare a demo class teacher')
    if (section.employee_id !== teacher.id) {
      const assigned = await browser.api(`/academic/sections/${section.section_id}/class-teacher`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ employee_id: teacher.id }),
      })
      if (assigned.status !== 200) throw new Error(`Class teacher setup: HTTP ${assigned.status}`)
    }
  }
  for (const role of roles) {
    await browser.viewport(1440, 900)
    await browser.login(`demo.${role === 'class_teacher' ? 'teacher' : role}@demo.flybirds.test`, password)
    await browser.go('/dashboard')
    await settle()
    const menu = await browser.evaluate(`[...document.querySelectorAll('aside nav a[href^="/"]')].map(a => a.getAttribute('href'))`)
    const paths = [...new Set(['/dashboard', ...menu, '/account', '/notifications'])]
    console.log(`${role}: ${paths.length} sidebar destinations`)
    for (const variant of variants) {
      await state(variant.lang, variant.theme)
      await browser.go('/dashboard')
      await settle()
      for (const path of paths) {
        for (const width of [1440, 375]) await visit(role, path, width, variant)
      }
    }
  }
} catch (error) {
  findings.push({ issue: `sweep aborted: ${error.stack}`, blocking: true })
  console.error(error)
} finally {
  writeFileSync(join(out, 'results.json'), JSON.stringify({ checks, pages, findings }, null, 2))
  console.log(`${checks} page visits, ${findings.length} findings`)
  await browser.close()
}
if (findings.some(x => x.blocking)) process.exitCode = 1
