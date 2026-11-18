import assert from 'node:assert/strict'
import { test } from 'node:test'
import { credentialCSV, decryptCredentialList, encryptCredentialList, teacherCredentialCSV } from '../src/pages/studentCredentialExport.ts'

test('one-time credential file is encrypted and decrypts with its passphrase', async () => {
  const csv = credentialCSV([{
    student_name: 'TEST Arif Rahman', name_bn: 'টেস্ট আরিফ রহমান', class_name: 'Six', section_name: '=unsafe',
    roll_no: 1, admission_no: 'TEST-1', login_id: 'TEST-1', initial_password: 'example-only-never-used',
    password_expires_at: '2026-11-19T00:00:00Z', student_id: '1', user_id: '2',
  }], '2026')
  assert.match(csv, /Academic session/)
  assert.match(csv, /টেস্ট আরিফ রহমান/)
  assert.match(csv, /"'=unsafe"/)
  const encrypted = await encryptCredentialList(csv, 'a long owner chosen passphrase')
  const bytes = Buffer.from(await encrypted.arrayBuffer())
  assert.equal(bytes.includes(Buffer.from('example-only-never-used')), false)
  assert.equal(await decryptCredentialList(encrypted, 'a long owner chosen passphrase'), csv)
  await assert.rejects(decryptCredentialList(encrypted, 'an incorrect passphrase'))
  await assert.rejects(encryptCredentialList(csv, 'too short'))
  await assert.rejects(decryptCredentialList(new Blob(['invalid']), 'a long owner chosen passphrase'))
})

test('teacher export contains only the required fields and stays encrypted', async () => {
  const csv = teacherCredentialCSV([{ name_en: 'TEST Farhana Rahman', name_bn: 'টেস্ট ফারহানা রহমান', employee_code: 'TEACHER-03', role: 'teacher', login_id: 'test.teacher-03@flybirds.test', initial_password: 'example-only-unused', password_expires_at: '2026-11-19T00:00:00Z' }])
  assert.match(csv, /Employee code/)
  assert.match(csv, /TEACHER-03/)
  assert.match(csv, /example-only-unused/)
  const encrypted = await encryptCredentialList(csv, 'a separately chosen owner passphrase')
  assert.equal(Buffer.from(await encrypted.arrayBuffer()).includes(Buffer.from('example-only-unused')), false)
  assert.equal(await decryptCredentialList(encrypted, 'a separately chosen owner passphrase'), csv)
  await assert.rejects(decryptCredentialList(encrypted, 'a wrong owner passphrase'))
})
