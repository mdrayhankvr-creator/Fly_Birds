// Run against a fresh demo school: see e2e/README.md.
// White-label check: several brand colours and the dark theme, measuring the
// rendered contrast of brand-coloured text and fills.
import { launch, sleep, reporter } from './cdp.mjs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
const here = dirname(fileURLToPath(import.meta.url))
const r = reporter()
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173', shots: join(here, 'shots', 'whitelabel'), port: 9344 })
const contrastJs = `(() => {
  const parse = c => { const m = c.match(/[\\d.]+/g).map(Number); return c.startsWith('color(') ? m.slice(0, 3).map(v => v * 255) : m.slice(0, 3) }
  const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
  const bgOf = el => { while (el) { const c = getComputedStyle(el).backgroundColor; if (!/rgba\\(0, 0, 0, 0\\)|transparent/.test(c)) return parse(c); el = el.parentElement } return [255, 255, 255] }
  const pair = el => el ? Math.round(ratio(parse(getComputedStyle(el).color), bgOf(el)) * 100) / 100 : null
  return { nav: pair(document.querySelector('[aria-current="page"]')), button: pair([...document.querySelectorAll('a,button')].find(e => getComputedStyle(e).backgroundColor === getComputedStyle(document.documentElement).getPropertyValue('--brand-primary').trim() || e.className.includes('bg-[var(--brand-primary)]') || e.classList.contains('btn-primary'))),
    link: pair([...document.querySelectorAll('a')].find(e => e.className.includes('text-[var(--brand-ink)]'))) }
})()`
try {
  await b.viewport(1366, 900)
  await b.login('demo.admin@demo.flybirds.test', process.env.DEMO_PASSWORD)
  const original = (await b.api('/school/settings')).body.data
  const themes = [['blue', '#1D4ED8', 'light'], ['teal', '#0F766E', 'light'], ['purple', '#7C3AED', 'light'], ['amber', '#B45309', 'light'], ['yellow', '#FACC15', 'light'], ['dark', '#2563EB', 'dark']]
  try {
    for (const [name, color, theme] of themes) {
      const body = { name_en: original.name_en, name_bn: original.name_bn, logo_url: original.logo_url, address: original.address, phone: original.phone, email: original.email, preferred_language: original.preferred_language,
        primary_color: color, secondary_color: original.secondary_color, dashboard_theme: theme, grading_scheme_id: original.grading_scheme_id }
      const put = await b.api('/school/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      if (name === 'yellow') { r.check('server refuses a brand colour below 4.5:1 on white', put.status === 400, String(put.status)); continue }
      if (put.status !== 200) { r.check(`save ${name} theme`, false, JSON.stringify(put.body)); continue }
      original.updated_at = put.body.data.updated_at
      await b.go('/dashboard')
      await b.waitFor(`document.body.innerText.includes('Good') && getComputedStyle(document.documentElement).getPropertyValue('--brand-primary').trim().toLowerCase() === '${color.toLowerCase()}'`)
      await sleep(1200)
      const c = await b.evaluate(contrastJs)
      r.check(`${name}: active navigation text ≥ 4.5:1`, c.nav >= 4.5, `ratio ${c.nav}`)
      r.check(`${name}: primary button text ≥ 4.5:1`, c.button >= 4.5, `ratio ${c.button}`)
      r.check(`${name}: brand link text ≥ 4.5:1`, c.link === null || c.link >= 4.5, `ratio ${c.link}`)
      r.check(`${name}: no overflow`, !(await b.overflow()))
      await b.shot(`dashboard-${name}`, false)
      if (name === 'amber' || name === 'dark') { await b.go('/admin/attendance/students'); await sleep(1500); await b.shot(`attendance-${name}`, false) }
    }
  } finally {
    const restore = { name_en: original.name_en, name_bn: original.name_bn, logo_url: original.logo_url, address: original.address, phone: original.phone, email: original.email, preferred_language: original.preferred_language,
      primary_color: '#1D4ED8', secondary_color: original.secondary_color, dashboard_theme: 'light', grading_scheme_id: original.grading_scheme_id }
    const res = await b.api('/school/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(restore) })
    r.check('restore original branding', res.status === 200)
  }
} catch (e) { r.check('script', false, e.stack) } finally { r.check('no JavaScript exceptions', b.errors.length === 0, b.errors.join(' | ')); r.save(join(here, 'shots', 'whitelabel.json')); b.close() }
