// Unit tests for checkpoint 3 rules: receipt amounts in words and the
// suggested login email of an admission.
// Run with: node --experimental-strip-types --test scripts/checkpoint3.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { amountInWordsBN, amountInWordsEN } from '../src/pages/amountWords.ts'
import { admissionLoginLocalPart } from '../src/pages/admissionModel.ts'

test('English amounts use crore, lakh and thousand', () => {
  assert.equal(amountInWordsEN(0), 'Taka Zero Only')
  assert.equal(amountInWordsEN(7), 'Taka Seven Only')
  assert.equal(amountInWordsEN(45), 'Taka Forty-Five Only')
  assert.equal(amountInWordsEN(1500), 'Taka One Thousand Five Hundred Only')
  assert.equal(amountInWordsEN(1500.5), 'Taka One Thousand Five Hundred and Fifty Paisa Only')
  assert.equal(amountInWordsEN(250000), 'Taka Two Lakh Fifty Thousand Only')
  assert.equal(amountInWordsEN(12345678.09), 'Taka One Crore Twenty-Three Lakh Forty-Five Thousand Six Hundred Seventy-Eight and Nine Paisa Only')
  assert.equal(amountInWordsEN(1_000_000_000), 'Taka One Hundred Crore Only')
})

test('Bangla amounts use the irregular names below one hundred', () => {
  assert.equal(amountInWordsBN(0), 'শূন্য টাকা মাত্র')
  assert.equal(amountInWordsBN(29), 'ঊনত্রিশ টাকা মাত্র')
  assert.equal(amountInWordsBN(99), 'নিরানব্বই টাকা মাত্র')
  assert.equal(amountInWordsBN(1500), 'এক হাজার পাঁচশত টাকা মাত্র')
  assert.equal(amountInWordsBN(1500.5), 'এক হাজার পাঁচশত টাকা ও পঞ্চাশ পয়সা মাত্র')
  assert.equal(amountInWordsBN(250000), 'দুই লক্ষ পঞ্চাশ হাজার টাকা মাত্র')
  assert.equal(amountInWordsBN(12345678), 'এক কোটি তেইশ লক্ষ পঁয়তাল্লিশ হাজার ছয়শত আটাত্তর টাকা মাত্র')
})

test('floating-point amounts round to the paisa', () => {
  assert.equal(amountInWordsEN(0.1 + 0.2), 'Taka Zero and Thirty Paisa Only')
  assert.equal(amountInWordsBN(19.999), 'বিশ টাকা মাত্র')
})

test('a login suggestion is made from the admission number', () => {
  assert.equal(admissionLoginLocalPart('DEMO-037'), 'demo-037')
  assert.equal(admissionLoginLocalPart('  ADM/2026/0041 '), 'adm-2026-0041')
  assert.equal(admissionLoginLocalPart('ভর্তি-12'), '12')
})
