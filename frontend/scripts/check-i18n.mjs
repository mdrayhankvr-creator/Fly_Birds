// Fails when a static t('key') call has no English or Bangla translation.
// i18next renders a missing key as raw text, so these bugs are easy to miss.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const languages = ['en', 'bn']
const files = dir => readdirSync(dir).flatMap(name => {
  const path = join(dir, name)
  return statSync(path).isDirectory() ? files(path) : [path]
})

// A call with a `count` option resolves to key_<plural category> (i18next JSON v4),
// so it needs every category the language's plural rules can select.
function translated(defined, language, key, plural) {
  if (defined.has(key)) return true
  if (!plural) return false
  const categories = new Intl.PluralRules(language).resolvedOptions().pluralCategories
  return categories.every(category => defined.has(`${key}_${category}`))
}

export function checkTranslations(src) {
  const sources = files(src).filter(path => /\.(ts|tsx)$/.test(path))
  const translationFiles = sources.filter(path => /Translations\.ts$|[\\/]i18n\.ts$/.test(path))
  const keys = Object.fromEntries(languages.map(language => [language, new Set()]))
  const keyPattern = /(?:^|[\s{,])([A-Za-z][A-Za-z0-9_]*)\s*:\s*['"`]/g
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
    for (const match of text.matchAll(/\bt\(\s*'([A-Za-z][A-Za-z0-9]*)'\s*(?:,\s*\{([^{}]*)\})?\s*[,)]/g)) {
      const plural = /\bcount\b/.test(match[2] ?? '')
      for (const language of languages) {
        if (!translated(keys[language], language, match[1], plural)) missing.push(`${path.slice(src.length)}: ${match[1]} (${language})`)
      }
    }
  }
  return { missing: [...new Set(missing)], keys }
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const { missing, keys } = checkTranslations(fileURLToPath(new URL('../src/', import.meta.url)))
  if (missing.length) {
    console.error(`Missing translations:\n${missing.join('\n')}`)
    process.exit(1)
  }
  console.log(`i18n check passed: ${keys.en.size} English and ${keys.bn.size} Bangla keys`)
}
