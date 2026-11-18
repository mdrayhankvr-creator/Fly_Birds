// Public design and privacy on the disposable fbqa-cp1b demo school.
// Makes 24 review screenshots in E2E_SHOTS_DIR, then restores the page,
// logo, slideshow and temporary fifth gallery photo.
import { launch, reporter, sleep } from './cdp.mjs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readFileSync, writeFileSync } from 'node:fs'

const here = dirname(fileURLToPath(import.meta.url))
const shots = process.env.E2E_SHOTS_DIR || join(here, 'shots', 'public-design')
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8092'
const pw = process.env.DEMO_PASSWORD
if (!pw) throw new Error('set DEMO_PASSWORD')
const r = reporter(), check = r.check
const b = await launch({ base, shots })
const axeSource = readFileSync(join(here, '..', 'frontend', 'node_modules', 'axe-core', 'axe.min.js'), 'utf8')
const styles = ['modern_bold', 'classic', 'minimal']
const labels = { modern_bold: 'Modern Bold', classic: 'Classic', minimal: 'Minimal' }
const headings = ['# Public school page design screenshots', '', 'QA stack: fbqa-cp1b at 127.0.0.1:8092.', 'Each image is a full-page capture at the named viewport width.', '']
let originalPage, originalSettings, originalLogo, addedSlideIds = [], addedGalleryId
const editable = page => { const { head_photo_version: _photo, ...input } = page; return input }

async function admin() { await b.login('demo.admin@demo.flybirds.test', pw); await b.go('/admin/school-settings') }
async function logout() {
  await b.evaluate(`(async () => { const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1] || ''); await fetch('/api/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': csrf } }) })()`)
}
async function axe() {
  if (!(await b.evaluate('typeof axe !== "undefined"'))) await b.evaluate(axeSource)
  return b.evaluate(`axe.run(document.body, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa'] } }).then(x => x.violations.map(v => v.id + ':' + v.nodes.length))`)
}
async function saveStyle(style, viaEditor = false) {
  if (viaEditor) {
    await b.go('/admin/school-settings')
    if (!(await b.waitFor(`!!document.querySelector('input[name="public-design-style"][value="${style}"]')`, 15000))) throw new Error('design editor did not load')
    await b.evaluate(`(() => { const input = document.querySelector('input[name="public-design-style"][value="${style}"]'); input.click(); input.closest('form').requestSubmit() })()`)
    if (!(await b.waitFor(`document.querySelector('section[aria-labelledby="public-page-editor"]')?.innerText.includes('Public page saved.')`, 15000))) throw new Error(`editor did not save ${style}`)
  } else {
    const result = await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...editable(originalPage), design_style: style, pass_rate: 92 }) })
    if (result.status !== 200) throw new Error(`save ${style}: ${result.status}`)
  }
  const saved = (await b.api('/school/public-page/edit')).body.data.page
  check(`${labels[style]} persists on the server`, saved.design_style === style)
  await b.go('/admin/school-settings')
  check(`${labels[style]} persists after editor reload`, await b.waitFor(`!!document.querySelector('input[name="public-design-style"][value="${style}"]:checked')`, 15000))
}

try {
  await b.viewport(1440, 900)
  await admin()
  originalSettings = (await b.api('/school/settings')).body.data
  originalPage = (await b.api('/school/public-page/edit')).body.data.page
  check('existing school defaults to Modern Bold', originalPage.design_style === 'modern_bold')
  check('four bilingual feature cards have defaults', originalPage.features?.length === 4 && originalPage.features.every(f => f.title_en && f.title_bn && f.description_en && f.description_bn))
  if (originalSettings.logo_url?.startsWith('/api/school/logo?')) {
    originalLogo = await b.evaluate(`fetch('/api/school/logo').then(async r => { if (!r.ok) return null; const a = new Uint8Array(await r.arrayBuffer()); let text = ''; for (let i = 0; i < a.length; i += 8192) text += String.fromCharCode(...a.slice(i, i + 8192)); return { kind: r.headers.get('content-type'), bytes: btoa(text) } })`)
  }

  // A school-coloured logo, with transparent edges and two distinct swatches.
  const uploaded = await b.evaluate(`(async () => {
    const c = document.createElement('canvas'); c.width = 512; c.height = 512
    const x = c.getContext('2d'); x.clearRect(0, 0, 512, 512)
    x.fillStyle = '#075985'; x.beginPath(); x.arc(256, 256, 228, 0, Math.PI*2); x.fill()
    x.fillStyle = '#f59e0b'; x.fillRect(108, 235, 296, 88)
    const blob = await new Promise(resolve => c.toBlob(resolve, 'image/png'))
    const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1])
    const token = (await (await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })).json()).data.access_token
    return fetch('/api/school/logo', { method: 'PUT', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'image/png' }, body: blob }).then(r => r.status)
  })()`)
  check('logo upload for palette succeeds', uploaded === 200, String(uploaded))
  const branding = (await b.api('/school/branding')).body.data
  check('two logo colours are stored', /^#[\da-f]{6}$/i.test(branding.logo_palette_primary) && /^#[\da-f]{6}$/i.test(branding.logo_palette_accent) && branding.logo_palette_primary !== branding.logo_palette_accent)

  // A newly seeded demo school has gallery art but no slideshow. Use two of
  // its own photos temporarily so the captures exercise the full photo hero.
  const slides = (await b.api('/school/slides')).body.data
  const gallery = (await b.api('/school/public-page/edit')).body.data.gallery
  // A plain CI seed has four photos, exactly the preview limit. Add one
  // temporary photo so See all must expand the grid, then remove it below.
  if (gallery.length <= 4) {
    if (!gallery.length) throw new Error('the demo school has no gallery photo to extend')
    const result = await b.evaluate(`(async () => {
      const image = await (await fetch('/api/school/gallery/${gallery[0].id}/image')).blob()
      const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1])
      const token = (await (await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })).json()).data.access_token
      const response = await fetch('/api/school/gallery', { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': image.type }, body: image })
      if (response.status !== 201) return { status: response.status }
      const body = await response.json()
      const caption = await fetch('/api/school/gallery/' + body.data.id, { method: 'PATCH', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: JSON.stringify({ caption_en: 'School life', caption_bn: 'বিদ্যালয়ের জীবন' }) })
      return { status: response.status, id: body.data.id, captionStatus: caption.status }
    })()`)
    if (result.status === 201) addedGalleryId = result.id
    if (result.status !== 201 || result.captionStatus !== 200) throw new Error(`could not prepare fifth gallery photo: ${result.status}/${result.captionStatus}`)
  }
  for (let i = slides.length; i < 2 && i < gallery.length; i++) {
    const result = await b.evaluate(`(async () => {
      const image = await (await fetch('/api/school/gallery/${gallery[i].id}/image')).blob()
      const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1])
      const token = (await (await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })).json()).data.access_token
      const response = await fetch('/api/school/slides', { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': image.type }, body: image })
      return { status: response.status, body: await response.json() }
    })()`)
    if (result.status === 201) addedSlideIds.push(result.body.data.id)
    check('school slideshow photo is used in the hero', result.status === 201, String(result.status))
  }

  for (const style of styles) await saveStyle(style, true)
  await saveStyle('modern_bold', true)
  if (!(await b.waitFor(`!!document.querySelector('[data-feature-card="0"] [data-feature-title-en]') && !!document.querySelector('[data-public-field="activities_en"]')`, 15000))) throw new Error('modern feature editor did not load')
  await b.evaluate(`(() => {
    const set = (selector, value) => { const el = document.querySelector(selector); const prototype = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(prototype, 'value').set.call(el, value); el.dispatchEvent(new Event('input', { bubbles: true })) }
    set('[data-feature-card="0"] [data-feature-title-en]', 'Thoughtful learning')
    set('[data-public-field="activities_en"]', 'Classroom learning\\nCreative activities')
    document.querySelector('section[aria-labelledby="public-page-editor"] form').requestSubmit()
  })()`)
  if (!(await b.waitFor(`document.querySelector('section[aria-labelledby="public-page-editor"]')?.innerText.includes('Public page saved.')`, 15000))) throw new Error('modern editor fields did not save')
  const edited = (await b.api('/school/public-page/edit')).body.data.page
  check('feature title and activities are editable in the admin form', edited.features[0].title_en === 'Thoughtful learning' && edited.activities_en.includes('Creative activities'))
  // Give the stats a value entered by the school, for every design screenshot.
  const page = { ...editable(originalPage), features: edited.features, pass_rate: 92,
    activities_en: 'Classroom learning\nCreative activities', activities_bn: 'শ্রেণিকক্ষে পাঠ\nসৃজনশীল কার্যক্রম',
    footer_description_en: 'A place to learn and grow together.', footer_description_bn: 'একসঙ্গে শেখা ও বেড়ে ওঠার স্থান।',
    contact_phone: '+880 1711-000000',
    social_facebook: 'https://facebook.com/flybirds.demo', privacy_policy_en: 'We use school information only for education.', privacy_policy_bn: 'বিদ্যালয়ের তথ্য শুধু শিক্ষার কাজে ব্যবহার করা হয়।',
    terms_en: 'Use this public page with care.', terms_bn: 'এই পাবলিক পাতাটি দায়িত্বের সঙ্গে ব্যবহার করুন.' }
  for (const style of styles) {
    const result = await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...page, design_style: style }) })
    if (result.status !== 200) throw new Error(`prepare ${style}: ${result.status}`)
    await logout()
    for (const theme of ['light', 'dark']) for (const lang of ['en', 'bn']) {
      await b.evaluate(`localStorage.setItem('flybirds.theme', ${JSON.stringify(theme)}); localStorage.setItem('flybirds.language', ${JSON.stringify(lang)})`)
      for (const width of [1440, 375]) {
        await b.viewport(width, width === 375 ? 812 : 900, width === 375)
        await b.go('/')
        if (!(await b.waitFor(`document.querySelector('[data-public-page]')?.dataset.publicStyle === '${style}'`, 15000))) throw new Error(`public ${style} did not load`)
        await b.evaluate(`(async () => { const images = [...document.querySelectorAll('[data-public-page] img')]; images.forEach(img => { img.loading = 'eager' }); await Promise.all(images.map(img => img.decode().catch(() => null))); scrollTo(0, 0) })()`)
        await sleep(350)
        const name = `${style}-${width}-${lang}-${theme}`
        await b.shot(name, true)
        headings.push(`- [${labels[style]} · ${width}px · ${lang.toUpperCase()} · ${theme}](${name}.png)`)
        check(`${name}: no sideways scroll`, !(await b.overflow()))
        check(`${name}: footer stays navy with readable links and current credit`, await b.evaluate(`(() => { const f = document.querySelector('[data-public-footer]'); const css = getComputedStyle(f); const expected = ${JSON.stringify(lang === 'bn' ? 'ফ্লাই বার্ডস-এর নির্মাণে · প্রকৌশল ও উন্নয়নে রায়হান ও জয়' : 'Developed by Fly Birds · Engineered by Rayhan & Joy')}; return css.backgroundColor === 'rgb(20, 34, 56)' && f.textContent.includes(expected) && f.querySelectorAll('nav').length === 2 && !!f.querySelector('a[href="/login"]') })()`))
        const violations = await axe()
        check(`${name}: axe WCAG A/AA`, violations.length === 0, violations.join(', '))
      }
    }
    for (const width of [320, 360, 375, 768]) {
      await b.viewport(width, 812, true); await b.go('/'); await b.waitFor(`!!document.querySelector('[data-public-page]')`, 15000)
      check(`${labels[style]} at ${width}px does not scroll sideways`, !(await b.overflow()))
    }
    const publicData = await b.evaluate(`fetch('/api/school/public-page').then(r => r.json()).then(x => x.data)`)
    const serialized = JSON.stringify(publicData).toLowerCase()
    check(`${labels[style]} public API exposes only school contact phone`, publicData.contact?.phone === '+880 1711-000000' && !/("[^\"]*(?:contact_phone|nid|guardian|marks)[^\"]*"\s*:)/i.test(serialized))
    check(`${labels[style]} footer links to configured school phone`, await b.evaluate(`document.querySelector('[data-public-footer] a[href="tel:+8801711000000"]')?.textContent === '+880 1711-000000'`))
    check(`${labels[style]} footer section links resolve on the public page`, await b.evaluate(`[...document.querySelectorAll('.school-footer a[href^="/#"]')].every(a => !!document.getElementById(a.hash.slice(1)))`))
    if (style === 'modern_bold') {
      await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
      await b.go('/')
      await b.waitFor(`!!document.querySelector('[data-public-page]')`, 15000)
      check('Modern Bold keeps sign-in off the landing page and removes hero buttons', await b.evaluate(`!document.querySelector('#login-email, .pd-hero-actions') && !!document.querySelector('[data-public-sign-in]')`))
      check('Modern Bold photo fills the first screen with only dot controls', await b.evaluate(`(() => { const hero = document.querySelector('.landing-hero').getBoundingClientRect(); return hero.height >= innerHeight - 70 && document.querySelectorAll('[data-slide-dots] button').length >= 2 && !document.querySelector('[data-slide-controls]') })()`))
      await b.evaluate(`document.querySelector('[data-public-sign-in]').click()`)
      check('header sign-in opens a modal with the full identifier field', await b.waitFor(`!!document.querySelector('[data-sign-in-overlay] [role="dialog"] #login-email') && document.querySelector('label[for="login-email"]')?.innerText.trim() === 'Email, phone or Student ID'`))
      check('sign-in modal focuses the identifier field', await b.evaluate(`document.activeElement?.id === 'login-email'`))
      await b.key('Escape', 'Escape', 27)
      check('Escape closes sign-in', await b.waitFor(`!document.querySelector('[data-sign-in-overlay]') && location.pathname === '/'`))
      check('Modern Bold has sticky navigation and four feature cards', await b.evaluate(`(() => { const h = document.querySelector('[data-public-header]'); return h && getComputedStyle(h).position === 'sticky' && document.querySelectorAll('.pd-feature-card').length === 4 && !!document.querySelector('#public-results') })()`))
      check('About uses two uploaded gallery photos and editable activities', await b.evaluate(`document.querySelectorAll('.pd-about-photos img').length === 2 && document.querySelectorAll('.pd-activities li').length === 2`))
      check('four-column footer uses configured contacts and social link without payment methods', await b.evaluate(`document.querySelectorAll('.school-footer-main > section, .school-footer-main > nav').length === 4 && document.querySelectorAll('.school-footer-socials a').length === 1 && !!document.querySelector('.school-footer a[href="tel:+8801711000000"]') && !document.querySelector('[data-payment-methods]')`))
      check('footer uses configured description and map without placeholder routes', await b.evaluate(`(() => { const f = document.querySelector('.school-footer'); const map = ${JSON.stringify(page.map_url)}; return f.textContent.includes('A place to learn and grow together.') && (map ? !![...f.querySelectorAll('a')].find(a => a.href === map) : !f.querySelector('a[href*="maps."]')) && !f.querySelector('a[href="#"], a[href="/notices"], a[href="/academic-calendar"], a[href="/admissions"]') })()`))
      check('privacy and terms links appear only when their pages have content', await b.evaluate(`document.querySelectorAll('.school-footer a[href="/privacy-policy"], .school-footer a[href="/terms"]').length === 2`))
      await b.evaluate(`document.querySelector('#public-gallery .pd-pill')?.click()`)
      check('gallery See all reveals full grid', await b.waitFor(`document.querySelectorAll('#public-gallery .pd-gallery-grid li').length >= 5`, 5000))
      await b.evaluate(`document.querySelector('#public-gallery .pd-gallery-grid button')?.click()`)
      check('gallery lightbox opens', await b.waitFor(`!!document.querySelector('[role="dialog"][aria-modal="true"]')`, 5000))
      await b.evaluate(`document.querySelector('[role="dialog"] button')?.click()`)
      await b.evaluate(`document.querySelector('.pd-menu-button')?.click()`)
      check('mobile hamburger reveals six public links', await b.waitFor(`document.querySelectorAll('#pd-mobile-menu .pd-nav-link').length === 6`, 5000))
      await b.evaluate(`document.querySelector('.pd-menu-button')?.click()`)
      await b.evaluate(`localStorage.setItem('flybirds.language', 'en')`)
      await b.go('/privacy-policy')
      check('admin-editable privacy page is published without HTML injection', await b.evaluate(`document.querySelector('.pd-legal')?.innerText.includes('school information') && !document.querySelector('.pd-legal script')`))
      await b.go('/terms')
      check('admin-editable terms page is published', await b.evaluate(`document.querySelector('.pd-legal')?.innerText.includes('public page with care')`))
      await b.go('/')
      await b.waitFor(`!!document.querySelector('[data-public-page]')`, 15000)
      const motion = await b.evaluate(`(() => { const el = document.querySelector('#public-gallery img') || document.querySelector('[data-public-page]'); return { reduced: matchMedia('(prefers-reduced-motion: reduce)').matches, duration: parseFloat(getComputedStyle(el).transitionDuration), hidden: getComputedStyle(document.querySelector('[data-public-page] .public-section')).opacity === '0' } })()`)
      check('reduced motion disables reveal and transitions', motion.reduced && motion.duration < 0.01 && !motion.hidden, JSON.stringify(motion))
      const slideBefore = await b.evaluate(`document.querySelector('[data-slide-dots] [aria-current="true"]')?.getAttribute('aria-label')`)
      await sleep(7300)
      check('reduced motion disables slideshow auto-slide', await b.evaluate(`document.querySelector('[data-slide-dots] [aria-current="true"]')?.getAttribute('aria-label')`) === slideBefore)
      const applied = await b.evaluate(`(() => ({ raw: document.documentElement.dataset.logoPalette, primary: getComputedStyle(document.documentElement).getPropertyValue('--public-primary').trim() }))()`)
      check('uploaded logo palette is applied to the public design', applied.raw?.toLowerCase() === branding.logo_palette_primary.toLowerCase() && applied.primary !== originalSettings.primary_color)
      const ratios = await b.evaluate(`(() => {
        const css = getComputedStyle(document.documentElement)
        const lum = hex => { const c = [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16) / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4); return c[0] * .2126 + c[1] * .7152 + c[2] * .0722 }
        const ratio = (a, b) => (Math.max(lum(a), lum(b)) + .05) / (Math.min(lum(a), lum(b)) + .05)
        return { light: ratio(css.getPropertyValue('--public-primary').trim(), '#ffffff'), dark: ratio(css.getPropertyValue('--public-light').trim(), '#101827') }
      })()`)
      check('logo shades meet WCAG AA on light and dark surfaces', ratios.light >= 4.5 && ratios.dark >= 4.5, JSON.stringify(ratios))
    }
    await admin()
  }
  const emptyFooter = { ...page, design_style: 'modern_bold', contact_address_en: '', contact_address_bn: '', contact_email: '', contact_phone: '', map_url: '', privacy_policy_en: '', privacy_policy_bn: '', terms_en: '', terms_bn: '' }
  check('school can clear optional footer details', (await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(emptyFooter) })).status === 200)
  await logout(); await b.go('/')
  await b.waitFor(`!!document.querySelector('[data-public-footer] .school-footer-heading')`, 15000)
  check('empty contacts and legal pages leave no dead footer blocks or links', await b.evaluate(`!document.querySelector('.school-footer-contact, .school-footer a[href^="tel:"], .school-footer a[href^="mailto:"], .school-footer a[href="/privacy-policy"], .school-footer a[href="/terms"]')`))
} catch (error) { check('script', false, error.stack) } finally {
  try {
    if (originalPage) {
      await admin()
      await b.api('/school/public-page', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(editable(originalPage)) })
      for (const id of addedSlideIds) await b.api(`/school/slides/${id}`, { method: 'DELETE' })
      if (addedGalleryId && (await b.api(`/school/gallery/${addedGalleryId}`, { method: 'DELETE' })).status !== 200) throw new Error('could not remove the temporary gallery photo')
      if (originalLogo) {
        await b.evaluate(`(async () => { const bytes = Uint8Array.from(atob(${JSON.stringify(originalLogo.bytes)}), c => c.charCodeAt(0)); const csrf = decodeURIComponent((document.cookie.match(/flybirds_csrf=([^;]+)/)||[])[1]); const token = (await (await fetch('/api/auth/refresh', { method: 'POST', headers: { 'X-CSRF-Token': csrf } })).json()).data.access_token; return fetch('/api/school/logo', { method: 'PUT', headers: { Authorization: 'Bearer ' + token, 'Content-Type': ${JSON.stringify(originalLogo.kind)} }, body: bytes }).then(r => r.status) })()`)
      } else {
        await b.api('/school/logo', { method: 'DELETE' })
        if (originalSettings.logo_url) {
          const fields = (({ name_en, name_bn, logo_url, address, phone, email, preferred_language, primary_color, secondary_color, dashboard_theme, grading_scheme_id }) => ({ name_en, name_bn, logo_url, address, phone, email, preferred_language, primary_color, secondary_color, dashboard_theme, grading_scheme_id }))(originalSettings)
          await b.api('/school/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(fields) })
        }
      }
    }
  } catch (error) { check('restore QA page and logo', false, error.message) }
  check('no JavaScript exceptions', b.errors.length === 0, b.errors.slice(0, 3).join(' | '))
  writeFileSync(join(shots, 'index.md'), headings.join('\n') + '\n')
  r.save(join(shots, 'results.json'))
  b.close()
}
