import type { StudentSetupResult } from '../api/studentSetup'

// The export is encrypted locally. The server sends credentials only once and
// neither the passphrase nor the plaintext is persisted by this page.
const header = new TextEncoder().encode('FLYBIRDS-CREDENTIALS-V1\n')
const iterations = 310_000

function cell(value: string | number) {
  const text = String(value)
  // Class and section names are school controlled; prevent CSV formulas.
  const safe = /^\s*[=+@-]/.test(text) ? `'${text}` : text
  return `"${safe.replaceAll('"', '""')}"`
}

export function credentialCSV(logins: StudentSetupResult['logins'], session: string) {
  const rows = [['Student (EN)', 'Student (BN)', 'Academic session', 'Class', 'Section', 'Roll', 'Admission number', 'Sign-in ID', 'Initial password', 'Expires at'],
    ...logins.map(login => [login.student_name, login.name_bn, session, login.class_name, login.section_name, login.roll_no ?? '', login.admission_no, login.login_id, login.initial_password, login.password_expires_at])]
  return '\ufeff' + rows.map(row => row.map(cell).join(',')).join('\r\n')
}

async function keyFor(passphrase: string, salt: Uint8Array, usage: KeyUsage) {
  const base = await crypto.subtle.importKey('raw', new TextEncoder().encode(passphrase), 'PBKDF2', false, ['deriveKey'])
  return crypto.subtle.deriveKey({ name: 'PBKDF2', salt: salt as BufferSource, iterations, hash: 'SHA-256' }, base,
    { name: 'AES-GCM', length: 256 }, false, [usage])
}

export async function encryptCredentialList(csv: string, passphrase: string): Promise<Blob> {
  if (passphrase.length < 16 || passphrase.length > 256) throw new Error('invalid passphrase length')
  const salt = crypto.getRandomValues(new Uint8Array(16))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  const key = await keyFor(passphrase, salt, 'encrypt')
  const ciphertext = new Uint8Array(await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(csv)))
  return new Blob([header, salt, iv, ciphertext], { type: 'application/octet-stream' })
}

export async function decryptCredentialList(file: Blob, passphrase: string): Promise<string> {
  if (file.size < header.length + 16 + 12 + 16 || file.size > 2_000_000) throw new Error('invalid credential file')
  const data = new Uint8Array(await file.arrayBuffer())
  if (!header.every((byte, index) => data[index] === byte)) throw new Error('invalid credential file')
  const salt = data.slice(header.length, header.length + 16)
  const iv = data.slice(header.length + 16, header.length + 28)
  const key = await keyFor(passphrase, salt, 'decrypt')
  const plaintext = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, data.slice(header.length + 28))
  return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(plaintext)
}
