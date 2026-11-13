// Run against a fresh demo school: see e2e/README.md.
// Narrow phones: no page may scroll sideways. Signs in as every demo role (and
// stays signed out for the public pages), visits every page it can reach from
// the menu and from links on those pages (one page per route pattern), opens
// every tab, and measures at each width (default 360px; WIDTHS=320,360,375)
// in each language (default English and Bangla; LANGS=en).
//
// A page fails when the document is wider than the screen, or when any
// element sticks out past the right edge without a scrolling or clipping
// parent to hold it. The second test ignores overflow hidden on html and body,
// which phones do not always honour.
import { launch, reporter, sleep } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
const here = dirname(fileURLToPath(import.meta.url))
const pw = process.env.DEMO_PASSWORD
const widths = (process.env.WIDTHS || '360').split(',').map(Number)
const langs = (process.env.LANGS || 'en,bn').split(',')
const only = process.env.ROLES ? process.env.ROLES.split(',') : null
const r = reporter(), check = r.check
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173', shots: join(here, 'shots', 'narrow-phones') })
const email = who => `demo.${who}@demo.flybirds.test`
const roles = ['admin', 'head', 'teacher', 'accounts', 'student', 'guardian'].filter(x => !only || only.includes(x))

// One page per route pattern: ids become :id.
const pattern = path => path.split('?')[0].replace(/\/[0-9a-f]{8}-[0-9a-f-]{27,}(?=\/|$)/gi, '/:id').replace(/\/\d+(?=\/|$)/g, '/:n')
const skip = path => /^\/(api|login|verify)\b|logout|\.pdf$/i.test(path)

const settle = async () => {
  await b.waitFor(`document.readyState === 'complete' && !document.querySelector('main .animate-pulse, main [aria-busy=true]')`, 12000).catch(() => null)
  await sleep(250)
}

// The elements that stick out past the right edge, deepest first.
const measure = width => b.evaluate(`(() => {
  const W = ${width}
  const fixed = el => { for (let p = el; p && p !== document.body; p = p.parentElement) if (getComputedStyle(p).position === 'fixed') return true; return false }
  const held = el => {
    for (let p = el.parentElement; p && p !== document.body && p !== document.documentElement; p = p.parentElement) {
      if (getComputedStyle(p).overflowX !== 'visible' && p.getBoundingClientRect().right <= W + 1) return true
    }
    return false
  }
  const out = []
  for (const el of document.body.querySelectorAll('*')) {
    const r = el.getBoundingClientRect()
    if (r.width <= 1 || r.height <= 1 || r.right <= W + 1) continue
    if (getComputedStyle(el).visibility === 'hidden' || fixed(el) || held(el)) continue
    out.push(el)
  }
  const deepest = out.filter(el => !out.some(o => o !== el && el.contains(o)))
  const name = el => {
    const bits = []
    for (let e = el; e && e !== document.body && bits.length < 4; e = e.parentElement) bits.unshift(e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + (e.getAttribute('data-testid') ? '[' + e.getAttribute('data-testid') + ']' : ''))
    return bits.join('>') + ' +' + Math.round(el.getBoundingClientRect().right - W) + 'px "' + (el.innerText || el.getAttribute('aria-label') || '').trim().slice(0, 40).replace(/\\s+/g, ' ') + '"'
  }
  return { doc: document.documentElement.scrollWidth - W, culprits: deepest.slice(0, 5).map(name) }
})()`)

async function checkHere(label) {
  const bad = []
  for (const w of widths) {
    await b.viewport(w, 760, true); await sleep(200)
    const m = await measure(w)
    if (m.doc > 1 || m.culprits.length) bad.push(`${w}px: doc +${m.doc}px ${m.culprits.join(' ; ')}`)
  }
  return bad
}

async function tabsHere() {
  return b.evaluate(`[...document.querySelectorAll('main [role=tab]')].filter(t => t.offsetParent !== null && t.getAttribute('aria-selected') !== 'true').map(t => t.innerText.trim()).filter(Boolean)`)
}

async function visit(path, role, lang, seen, queue, failures) {
  await b.viewport(widths[0], 760, true)
  await b.go(path); await settle()
  const here = await b.evaluate('location.pathname + location.search')
  if (here.startsWith('/login') && path !== '/login') return
  const label = `${role} ${lang} ${path}`
  for (const x of await checkHere(label)) failures.push(`${label} ${x}`)
  // Each tab in turn.
  for (const tab of await tabsHere()) {
    const clicked = await b.evaluate(`(() => { const t = [...document.querySelectorAll('main [role=tab]')].find(t => t.innerText.trim() === ${JSON.stringify(tab)}); if (!t) return false; t.click(); return true })()`)
    if (!clicked) continue
    await settle()
    for (const x of await checkHere(label)) failures.push(`${label} [${tab}] ${x}`)
  }
  // Links to more pages, one per pattern.
  const links = await b.evaluate(`[...document.querySelectorAll('a[href^="/"]')].filter(a => !a.hasAttribute('download')).map(a => a.getAttribute('href'))`)
  for (const l of links) { const p = pattern(l); if (!skip(l) && !seen.has(p)) { seen.add(p); queue.push(l) } }
}

async function crawl(role, lang) {
  const failures = [], seen = new Set(), queue = []
  if (role === 'public') {
    await b.go('/'); await b.evaluate(`localStorage.setItem('flybirds.language', '${lang}')`)
    for (const p of ['/', '/login', '/verify/not-a-real-token', '/no-such-page']) { seen.add(pattern(p)); queue.push(p) }
  } else {
    await b.login(email(role), pw, lang)
    // The menu: the sidebar on wide screens holds every page of the role.
    await b.viewport(1366, 900); await b.go('/dashboard'); await settle()
    const menu = await b.evaluate(`[...document.querySelectorAll('aside nav a[href^="/"]')].map(a => a.getAttribute('href'))`)
    for (const p of ['/dashboard', ...menu, '/account', '/notifications']) if (!seen.has(pattern(p))) { seen.add(pattern(p)); queue.push(p) }
  }
  let pages = 0
  while (queue.length && pages < 120) { const p = queue.shift(); pages++; if (role === 'public' || !skip(p)) await visit(p, role, lang, seen, queue, failures) }
  // The phone menu open.
  if (role !== 'public') {
    await b.viewport(widths[0], 760, true); await b.go('/dashboard'); await settle()
    const opened = await b.evaluate(`(() => { const m = [...document.querySelectorAll('header button')].find(x => /menu|মেনু/i.test(x.getAttribute('aria-label') || '')); if (!m) return false; m.click(); return true })()`)
    if (opened) { await sleep(400); for (const x of await checkHere('menu')) failures.push(`${role} ${lang} menu open ${x}`) }
  }
  return { failures, pages }
}

try {
  for (const lang of langs) {
    for (const role of ['public', ...roles]) {
      const { failures, pages } = await crawl(role, lang)
      check(`${role} (${lang}): ${pages} pages at ${widths.join(', ')}px, none scroll sideways`, failures.length === 0, failures.slice(0, 12).join(' || ') || `${pages} pages`)
      if (failures.length > 12) console.log(failures.slice(12).join('\n'))
    }
  }
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
} catch (e) {
  check('narrow phone run completed', false, e.stack)
} finally {
  r.save(join(here, 'shots', 'narrow-phones-results.json'))
  await b.close()
}
