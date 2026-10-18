import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, sep } from 'node:path'
import { test } from 'node:test'
import { checkTranslations } from './check-i18n.mjs'

function fixture(translations, page) {
  const root = mkdtempSync(join(tmpdir(), 'i18n-'))
  mkdirSync(join(root, 'pages'))
  writeFileSync(join(root, 'demoTranslations.ts'), translations)
  writeFileSync(join(root, 'pages', 'Demo.tsx'), page)
  try {
    return checkTranslations(root + sep).missing
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
}
const both = (en, bn) => `export const demo = {\n en: { ${en} },\n bn: { ${bn} }\n}\n`

test('plain keys must exist in both languages', () => {
  assert.deepEqual(fixture(both("title:'Title'", "title:'শিরোনাম'"), "t('title')"), [])
  assert.deepEqual(fixture(both("title:'Title'", "other:'x'"), "t('title')"), ['pages' + sep + 'Demo.tsx: title (bn)'])
})

test('a count call accepts complete plural forms', () => {
  const plural = both("items_one:'{{count}} item',items_other:'{{count}} items'", "items_one:'{{count}}টি',items_other:'{{count}}টি'")
  assert.deepEqual(fixture(plural, "t('items', { count: n })"), [])
})

test('a count call fails when a plural form is missing', () => {
  const partial = both("items_one:'{{count}} item',items_other:'{{count}} items'", "items_one:'{{count}}টি'")
  assert.deepEqual(fixture(partial, "t('items', { count: n })"), ['pages' + sep + 'Demo.tsx: items (bn)'])
})

test('plural forms do not satisfy a call without count', () => {
  const plural = both("items_one:'a',items_other:'b'", "items_one:'a',items_other:'b'")
  assert.equal(fixture(plural, "t('items')").length, 2)
})
