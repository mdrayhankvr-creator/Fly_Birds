// Run against a disposable demo database. Checks real PageHeader geometry at
// desktop, tablet and phone widths without changing student records.
import { launch, reporter } from './cdp.mjs'

const password = process.env.DEMO_PASSWORD
if (!password) throw new Error('DEMO_PASSWORD is required')
const report = reporter(), check = report.check
const browser = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:8092', shots: process.env.E2E_SHOTS_DIR || 'e2e/shots/students-header-layout' })

const geometry = `(() => {
  const header = document.querySelector('main header.ds-page-head')
  const title = header?.querySelector('h1'), description = title?.nextElementSibling
  const links = [...(header?.querySelectorAll('a[href^="/admin/students/"]') || [])]
  if (!header || !title || !description) return null
  const bounds = header.getBoundingClientRect(), text = title.parentElement.getBoundingClientRect()
  const lineHeight = parseFloat(getComputedStyle(title).lineHeight)
  const actionsInside = links.every(link => {
    const rect = link.getBoundingClientRect(), middle = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
    return rect.width > 0 && rect.left >= bounds.left - 1 && rect.right <= bounds.right + 1 &&
      rect.top >= bounds.top - 1 && rect.bottom <= bounds.bottom + 1 && middle?.closest('a') === link
  })
  return { textWidth: text.width, descriptionWidth: description.getBoundingClientRect().width,
    titleLines: title.getBoundingClientRect().height / lineHeight, actions: links.length,
    actionRows: new Set(links.map(link => Math.round(link.getBoundingClientRect().top))).size,
    actionsInside, pageOverflow: document.documentElement.scrollWidth > innerWidth + 1 }
})()`

try {
  await browser.login('demo.admin@demo.flybirds.test', password)
  for (const language of ['en', 'bn']) {
    await browser.evaluate(`localStorage.setItem('flybirds.language', ${JSON.stringify(language)})`)
    for (const width of [1440, 768, 390]) {
      await browser.viewport(width, 900, width < 640)
      await browser.go('/admin/students')
      check(`${language} ${width}: Students header and all five actions load`, await browser.waitFor(`document.querySelectorAll('main header.ds-page-head a[href^="/admin/students/"]').length === 5`, 15000))
      const layout = await browser.evaluate(geometry)
      check(`${language} ${width}: heading and description have readable width`, !!layout && layout.textWidth >= (width === 390 ? 180 : 300) && layout.descriptionWidth >= (width === 390 ? 180 : 300) && layout.titleLines <= 2.1, JSON.stringify(layout))
      check(`${language} ${width}: all actions are visible and clickable`, layout?.actions === 5 && layout.actionsInside, JSON.stringify(layout))
      check(`${language} ${width}: actions wrap and page does not scroll sideways`, !!layout && (width === 1440 || layout.actionRows >= 2) && !layout.pageOverflow, JSON.stringify(layout))
      if (process.env.E2E_SHOTS_DIR && language === 'en') await browser.shot(`students-${width}`, false)
    }
  }
  await browser.viewport(768, 900)
  await browser.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
  await browser.go('/admin/exams')
  check('another shared header remains readable', await browser.waitFor(`!!document.querySelector('main header.ds-page-head h1')`, 15000) && await browser.evaluate(`document.querySelector('main header.ds-page-head h1').parentElement.getBoundingClientRect().width > 200`))
} catch (error) { check('script error', false, error.stack) }
finally {
  check('no browser JavaScript errors', browser.errors.length === 0, browser.errors.slice(0, 3).join(' | '))
  report.save(process.env.E2E_RESULTS || 'e2e/shots/students-header-layout/results.json')
  await browser.close()
}
