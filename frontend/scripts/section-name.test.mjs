import { test } from 'node:test'
import assert from 'node:assert/strict'
import { sectionDisplayName } from '../src/components/sectionName.ts'

test('an existing section label is never repeated in either language', () => {
  assert.equal(sectionDisplayName('Section A', 'en'), 'Section A')
  assert.equal(sectionDisplayName('Section A', 'bn'), 'Section A')
  assert.equal(sectionDisplayName('শাখা ক', 'bn'), 'শাখা ক')
  assert.equal(sectionDisplayName('ক শাখা', 'bn'), 'ক শাখা')
})

test('an unlabelled section gets the selected language label', () => {
  assert.equal(sectionDisplayName('A', 'en'), 'Section A')
  assert.equal(sectionDisplayName('ক', 'bn'), 'শাখা ক')
})
