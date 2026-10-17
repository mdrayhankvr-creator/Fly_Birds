// Fails when a static t('key') call has no English or Bangla translation.
// i18next renders a missing key as raw text, so these bugs are easy to miss.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const src = fileURLToPath(new URL('../src/', import.meta.url))
const files = dir => readdirSync(dir).flatMap(name => {
  const path = join(dir, name)
  return statSync(path).isDirectory() ? files(path) : [path]
})
const sources = files(src).filter(path => /\.(ts|tsx)$/.test(path))
const translationFiles = sources.filter(path => /Translations\.ts$|[\\/]i18n\.ts$/.test(path))

const keys = { en: new Set(), bn: new Set() }
const keyPattern = /(?:^|[\s{,])([A-Za-z][A-Za-z0-9]*)\s*:\s*['"`]/g
for (const path of translationFiles) {
  const text = readFileSync(path, 'utf8')
  const bn = text.search(/\bbn\s*:\s*\{/)
  if (bn < 0) throw new Error(`${path}: no bn block`)
  for (const [language, block] of [['en', text.slice(0, bn)], ['bn', text.slice(bn)]]) {
    for (const match of block.matchAll(keyPattern)) keys[language].add(match[1])
  }
}

const missing = []
for (const path of sources) {
  if (translationFiles.includes(path)) continue
  const text = readFileSync(path, 'utf8')
  for (const match of text.matchAll(/\bt\(\s*'([A-Za-z][A-Za-z0-9]*)'\s*[,)]/g)) {
    for (const language of ['en', 'bn']) {
      if (!keys[language].has(match[1])) missing.push(`${path.slice(src.length)}: ${match[1]} (${language})`)
    }
  }
}
if (missing.length) {
  console.error(`Missing translations:\n${[...new Set(missing)].join('\n')}`)
  process.exit(1)
}
console.log(`i18n check passed: ${keys.en.size} English and ${keys.bn.size} Bangla keys`)
