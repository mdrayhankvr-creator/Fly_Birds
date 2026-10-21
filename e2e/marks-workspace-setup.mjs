// Prepares a disposable, demo-seeded school for e2e/marks-workspace.mjs: one
// large section (56 students), 42 in every other section the demo teacher
// teaches (200+ students in all), and a draft exam per section with the
// teacher's subject. It adds fictional students; never run it on a real school.
//
//   E2E_BASE_URL=http://localhost:8104 DEMO_PASSWORD=... EXAM_NAME="E4 Marks Test" node e2e/marks-workspace-setup.mjs sheets.json
//
// Run it again with a new EXAM_NAME for a fresh set of draft sheets.
import { writeFileSync } from 'node:fs'
const base = process.env.E2E_BASE_URL, password = process.env.DEMO_PASSWORD
const login = async who => (await (await fetch(`${base}/api/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: `demo.${who}@demo.flybirds.test`, password }) })).json()).data.access_token
const client = token => async (method, path, body) => {
  const r = await fetch(`${base}/api${path}`, { method, headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) }, body: body ? JSON.stringify(body) : undefined })
  const j = await r.json().catch(() => ({}))
  if (r.status >= 300) throw new Error(`${method} ${path}: ${r.status} ${JSON.stringify(j.error)}`)
  return j.data
}
const admin = client(await login('admin')), teacher = client(await login('teacher'))
const first = ['Arif', 'Nusrat', 'Tanvir', 'Sadia', 'Rakib', 'Farzana', 'Imran', 'Mitu', 'Shakil', 'Jannat', 'Rafiq', 'Sumaiya', 'Hasan', 'Tania', 'Mahin', 'Anika']
const last = ['Rahman', 'Akter', 'Hossain', 'Islam', 'Chowdhury', 'Saha', 'Ahmed', 'Begum', 'Khan', 'Das']
const assignments = (await teacher('GET', '/staff/me/assignments?page=1&page_size=100')).items
const sheets = []
let added = 0, n = 0
for (const a of assignments) {
  const sections = (await admin('GET', `/academic/sections?page=1&page_size=100&session_id=${a.session_id}&class_id=${a.class_id}`)).items.filter(s => s.is_active)
  for (const [si, section] of sections.entries()) {
    const existing = (await admin('GET', `/students?page=1&page_size=100&session_id=${a.session_id}&class_id=${a.class_id}&section_id=${section.id}`)).items
    const target = sheets.length === 0 ? 56 : 42
    let roll = Math.max(0, ...existing.map(s => s.roll_no || 0))
    for (let i = existing.length; i < target; i++) {
      n++; roll++
      await admin('POST', '/students', { admission_no: `E4-${String(n).padStart(4, '0')}`, name_en: `${first[n % first.length]} ${last[(n * 7) % last.length]}`, name_bn: '', date_of_birth: '', status: 'active', session_id: a.session_id, class_id: a.class_id, section_id: section.id, roll_no: roll, user_id: '' })
      added++
    }
    const exam = await admin('POST', '/exams', { name: process.env.EXAM_NAME || 'E4 Marks Test', exam_type: 'class_test', session_id: a.session_id, class_id: a.class_id, section_id: section.id, routine_id: null })
    const withSubject = await admin('POST', `/exams/${exam.id}/subjects`, { subject_id: a.subject_id, full_marks: 50, pass_marks: 17 })
    const sheet = withSubject.subjects.find(s => s.subject_id === a.subject_id)
    sheets.push({ exam_id: exam.id, sheet_id: sheet.id, class: a.class_name, section: section.name, students: Math.max(target, existing.length), first: si === 0 && sheets.length === 0 })
  }
}
const total = sheets.reduce((s, x) => s + x.students, 0)
console.log(`added ${added} students; ${sheets.length} sheets for demo.teacher; ${total} students across them`)
for (const s of sheets) console.log(`  ${s.class} ${s.section}: ${s.students} students`)
writeFileSync(process.argv[2], JSON.stringify(sheets, null, 1))
