// Review captures of Modern Bold on the disposable fbqa-cp1b demo school.
// Two seeded gallery photos are used temporarily if the seed has no slides.
import { launch, sleep } from './cdp.mjs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { writeFileSync } from 'node:fs'

const shots = process.env.E2E_SHOTS_DIR || join(dirname(fileURLToPath(import.meta.url)), 'shots', 'public-polish')
const pw = process.env.DEMO_PASSWORD
if (!pw) throw new Error('set DEMO_PASSWORD')
const b = await launch({ base: process.env.E2E_BASE_URL || 'http://127.0.0.1:8092', shots })
const added = []
const index = ['# Modern Bold public page', '', 'Fresh fbqa-cp1b demo school. Landing captures show the full page; sign-in captures show the viewport.', '']

try {
  await b.login('demo.admin@demo.flybirds.test', pw)
  const slides = (await b.api('/school/slides')).body.data
  const gallery = (await b.api('/school/public-page/edit')).body.data.gallery
  for (let i = slides.length; i < 2 && i < gallery.length; i++) {
    const result = await b.evaluate(`(async () => {
      const image = await (await fetch('/api/school/gallery/${gallery[i].id}/image')).blob()
      const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1])
      const token = (await (await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })).json()).data.access_token
      const response = await fetch('/api/school/slides', { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': image.type }, body: image })
      return { status: response.status, body: await response.json() }
    })()`)
    if (result.status !== 201) throw new Error(`slide upload failed: ${result.status}`)
    added.push(result.body.data.id)
  }
  await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }) })()`)

  for (const width of [375, 1440]) for (const lang of ['en', 'bn']) {
    await b.viewport(width, width === 375 ? 812 : 900, width === 375)
    await b.evaluate(`localStorage.setItem('flybirds.language', '${lang}'); localStorage.setItem('flybirds.theme', 'light')`)
    await b.go('/')
    if (!(await b.waitFor(`document.querySelector('.landing[data-public-style="modern_bold"]') && document.querySelector('.landing-hero img')?.complete && document.querySelector('[data-public-page]')`, 15000))) throw new Error('public page did not load')
    await b.evaluate(`(async () => { const images = [...document.querySelectorAll('[data-public-page] img')]; images.forEach(img => { img.loading = 'eager' }); await Promise.all(images.map(img => img.decode().catch(() => null))) })()`)
    await b.evaluate(`scrollTo(0, 0)`)
    const landingName = `landing-${width}-${lang}`
    await b.shot(landingName, true)
    index.push(`- [Landing · ${width}px · ${lang.toUpperCase()}](${landingName}.png)`)

    await b.evaluate(`document.querySelector('[data-public-sign-in]').click()`)
    if (!(await b.waitFor(`!!document.querySelector('[data-sign-in-overlay] #login-email')`, 5000))) throw new Error('sign-in card did not open')
    const signInName = `sign-in-${width}-${lang}`
    await b.shot(signInName, false)
    index.push(`- [Sign-in card · ${width}px · ${lang.toUpperCase()}](${signInName}.png)`)
  }
  const trapped = await b.evaluate(`(() => {
    const modal = document.querySelector('[data-sign-in-overlay] [role="dialog"]')
    const items = [...modal.querySelectorAll('button:not(:disabled), input:not(:disabled), a[href]')].filter(x => x.getClientRects().length)
    items[items.length - 1].focus()
    items[items.length - 1].dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    const forward = document.activeElement === items[0]
    items[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    return forward && document.activeElement === items[items.length - 1]
  })()`)
  if (!trapped) throw new Error('sign-in focus did not wrap in both directions')

  await b.viewport(375, 812, true)
  await b.go('/')
  await b.waitFor(`document.querySelectorAll('[data-slide-dots] button').length >= 2`, 5000)
  const swiped = await b.evaluate(`(() => {
    const hero = document.querySelector('.landing-hero')
    const before = [...document.querySelectorAll('[data-slide-dots] button')].findIndex(x => x.getAttribute('aria-current') === 'true')
    const start = new Touch({ identifier: 1, target: hero, clientX: 280, clientY: 360 })
    const end = new Touch({ identifier: 1, target: hero, clientX: 75, clientY: 355 })
    hero.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, touches: [start], changedTouches: [start] }))
    hero.dispatchEvent(new TouchEvent('touchend', { bubbles: true, touches: [], changedTouches: [end] }))
    return before
  })()`)
  if (!(await b.waitFor(`[...document.querySelectorAll('[data-slide-dots] button')].findIndex(x => x.getAttribute('aria-current') === 'true') !== ${swiped}`, 3000))) throw new Error('phone swipe did not advance the slide')
  await b.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'no-preference' }] })
  await b.go('/')
  await b.waitFor(`!matchMedia('(prefers-reduced-motion: reduce)').matches && document.querySelectorAll('[data-slide-dots] button').length >= 2`, 5000)
  const beforeAuto = await b.evaluate(`[...document.querySelectorAll('[data-slide-dots] button')].findIndex(x => x.getAttribute('aria-current') === 'true')`)
  await sleep(7500)
  if (!(await b.evaluate(`[...document.querySelectorAll('[data-slide-dots] button')].findIndex(x => x.getAttribute('aria-current') === 'true') !== ${beforeAuto}`))) throw new Error('slideshow did not advance automatically')
  index.push('', 'Verified: focus wraps within sign-in; phone swipe and automatic slideshow advance work. Reduced-motion pause is checked in public-design.mjs.')
  writeFileSync(join(shots, 'index.md'), index.join('\n') + '\n')
} finally {
  if (added.length) {
    await b.login('demo.admin@demo.flybirds.test', pw)
    for (const id of added) await b.api(`/school/slides/${id}`, { method: 'DELETE' })
  }
  await b.close()
}
