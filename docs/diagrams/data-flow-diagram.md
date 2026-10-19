# Data Flow Diagram

Notation: rectangles are external entities, rounded boxes are processes,
cylinders are data stores. Arrows carry the data named on them.

Every request from the browser passes through the school's TLS edge and the
web server (nginx) to the Go API, which checks the session, permission and
module before any process touches a data store. Each school has its own
stack, so no data flows between schools.

## Level 0 — Context

```mermaid
flowchart LR
    staff["Staff<br/>admin, head teacher,<br/>teacher, accountant"]
    family["Students and guardians"]
    operator["Server operator"]
    gateway["Payment gateway<br/>(MOCK bKash / Nagad)"]
    sms["SMS provider<br/>(MOCK, nothing is sent)"]

    sys(["0. Fly_Birds school system"])

    staff -->|"credentials, school data, attendance,<br/>marks, fees, notices, diary"| sys
    sys -->|"dashboards, registers, reports,<br/>receipts, exports, backups"| staff
    family -->|"credentials, homework, online payment request"| sys
    sys -->|"attendance, results, fees, routine,<br/>diary, notices, notifications"| family
    operator -->|"deployment settings, first admin,<br/>password reset, restore archive"| sys
    sys -->|"health and readiness, logs"| operator
    sys -->|"simulated payment intent"| gateway
    gateway -->|"simulated confirmation"| sys
    sys -->|"queued SMS text"| sms
    sms -->|"mock delivery status"| sys
```

## Level 1 — Main processes

```mermaid
flowchart TB
    staff["Staff"]
    family["Students and guardians"]
    operator["Server operator"]
    gateway["MOCK payment gateway"]
    smsp["MOCK SMS provider"]

    p1(["1. Authenticate and authorise"])
    p2(["2. Configure school"])
    p3(["3. Manage academics and people"])
    p4(["4. Record attendance"])
    p5(["5. Exams and results"])
    p6(["6. Fees and accounts"])
    p7(["7. Notices, events and diary"])
    p8(["8. Notify"])
    p9(["9. Analytics and reports"])
    p10(["10. Backup, restore and audit"])

    d1[("D1 Users, roles,<br/>permissions, sessions")]
    d2[("D2 School settings<br/>and modules")]
    d3[("D3 Academic structure,<br/>students, guardians, staff")]
    d4[("D4 Attendance and routines")]
    d5[("D5 Exams, marks, results")]
    d6[("D6 Fees, payments, ledger")]
    d7[("D7 Notices, events, diary")]
    d8[("D8 Notifications and SMS queue")]
    d9[("D9 File storage<br/>(uploads volume)")]
    d10[("D10 Audit log")]
    d11[("D11 Backup archives<br/>(backups volume)")]

    staff & family -->|"email, password"| p1
    p1 <-->|"password hash, lockout,<br/>refresh and CSRF hashes"| d1
    p1 -->|"access token, cookies,<br/>permissions, navigation"| staff & family
    operator -->|"first admin, password reset"| p1

    staff -->|"name, logo, colours,<br/>modules, grading"| p2
    p2 <--> d2

    staff -->|"sessions, classes, sections,<br/>students, guardians, staff"| p3
    p3 <--> d3
    p3 -->|"student and guardian accounts"| d1

    staff -->|"present, absent, late"| p4
    d3 -->|"roster"| p4
    p4 <--> d4
    p4 -->|"absence events"| p8
    p4 -->|"attendance views"| family

    staff -->|"exams, marks"| p5
    d3 -->|"enrolment, subjects"| p5
    p5 <--> d5
    p5 -->|"published results"| family
    p5 -->|"result published"| p8

    staff -->|"structures, collections,<br/>expenses"| p6
    family -->|"online payment request"| p6
    p6 <-->|"intent, confirmation"| gateway
    p6 <--> d6
    p6 -->|"receipts, dues"| staff & family

    staff -->|"notices, events,<br/>diary, attachments"| p7
    family -->|"homework submissions"| p7
    p7 <--> d7
    p7 <-->|"files"| d9
    p7 -->|"published items"| p8

    p8 <--> d8
    d1 -->|"recipients, preferences"| p8
    p8 -->|"in-app notifications"| family & staff
    p8 -->|"queued SMS"| smsp

    d3 & d4 & d5 & d6 -->|"read only"| p9
    p9 -->|"dashboards, alerts,<br/>CSV and printable reports"| staff & family

    p1 & p2 & p3 & p4 & p5 & p6 & p7 -->|"who did what"| d10
    staff -->|"backup, verify, export"| p10
    d1 & d2 & d3 & d4 & d5 & d6 & d7 & d8 & d9 & d10 -->|"all tables and files"| p10
    p10 <-->|"tar.gz with SHA-256 manifest"| d11
    operator -->|"archive to restore<br/>(empty database only)"| p10
    d10 -->|"audit trail"| p10
```

## Notes

- **Access control (process 1)** runs before every other process: the API
  validates the 15-minute access token, checks the permission in
  `role_permissions` and the school's enabled modules, and records denied
  attempts in the audit log.
- **Scope rules:** teachers reach only the classes and subjects assigned to
  them; students only their own records; guardians only their linked
  children.
- **Simulations:** the payment gateway and the SMS provider are mocks.
  Payments are marked confirmed by the mock and SMS messages are marked
  sent without leaving the server. Neither is a live integration.
- **Restore** (process 10) refuses a database that already has users unless
  the operator explicitly forces it; see [OPERATIONS.md](../OPERATIONS.md).
