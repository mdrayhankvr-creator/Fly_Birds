# Use Case Diagram

Actors and use cases come from the role-permission matrix the API enforces
(`role_permissions`, seeded by the migrations). Each permission is checked on
every request, and a use case also requires its module to be enabled for the
school. Mermaid has no native UML use-case notation, so actors are drawn as
boxes and use cases as rounded ovals inside the system boundary.

## Administrator

```mermaid
flowchart LR
    admin["👤 Administrator"]

    subgraph FB["Fly_Birds (one school)"]
        direction TB
        a1(["Sign in, sign out, view own profile"])
        a2(["Manage school settings, branding and modules"])
        a3(["Manage users, roles and permissions"])
        a4(["Manage sessions, classes, sections and subjects"])
        a5(["Manage students and guardians"])
        a6(["Manage staff, leave and salary records"])
        a7(["Take student attendance"])
        a8(["Record and report teacher attendance"])
        a9(["Manage class routines"])
        a10(["Manage exams, grading and publish results"])
        a11(["Manage fees, collections and the ledger"])
        a12(["Publish notices and events"])
        a13(["Oversee the class diary"])
        a14(["View analytics, finance analytics and reports"])
        a15(["Back up, verify and export data"])
        a16(["Review the audit log and system health"])
    end

    admin --- a1 & a2 & a3 & a4 & a5 & a6 & a7 & a8
    admin --- a9 & a10 & a11 & a12 & a13 & a14 & a15 & a16
```

## Head teacher, teacher and accountant

```mermaid
flowchart LR
    head["👤 Head Teacher"]
    teacher["👤 Teacher"]
    accountant["👤 Accountant"]

    subgraph FB["Fly_Birds (one school)"]
        direction TB
        u1(["Sign in, sign out, view own profile"])
        u2(["Take student attendance"])
        u3(["Record and report teacher attendance"])
        u4(["Publish notices and events"])
        u5(["Write class diary and review homework"])
        u6(["View analytics, alerts and reports"])
        u7(["Request leave"])
        u8(["Enter exam marks"])
        u9(["Manage fee structures and student fees"])
        u10(["Collect fees and issue receipts"])
        u11(["Manage income and expense ledger"])
        u12(["View finance analytics"])
    end

    head --- u1 & u2 & u3 & u4 & u5 & u6 & u7
    teacher --- u1 & u2 & u5 & u6 & u7 & u8
    accountant --- u1 & u9 & u10 & u11 & u12
```

Notes:
- The head teacher reads (but does not change) academic structure, students,
  guardians, staff, routines and exams.
- A teacher's analytics, attendance and marks entry are limited to the
  classes and subjects assigned to them.
- The administrator also manages the class diary (`diary:manage`), for
  example reviewing every teacher's entries.

## Students and guardians

```mermaid
flowchart LR
    student["👤 Student"]
    guardian["👤 Guardian"]
    gateway["⚙ Payment gateway (MOCK bKash / Nagad)"]

    subgraph FB["Fly_Birds (one school)"]
        direction TB
        s1(["Sign in and view own profile"])
        s2(["View attendance"])
        s3(["View routine"])
        s4(["View published results and GPA"])
        s5(["View fees, dues and receipts"])
        s6(["Pay fees online"])
        s7(["Read class diary and homework"])
        s8(["Submit homework"])
        s9(["Read notices, events and notifications"])
        s10(["View attendance and result summary"])
        s11(["Choose a child"])
    end

    student --- s1 & s2 & s3 & s4 & s5 & s6 & s7 & s8 & s9 & s10
    guardian --- s1 & s11 & s2 & s3 & s4 & s5 & s6 & s7 & s9 & s10
    s6 -.->|"simulated payment"| gateway
```

Students see only their own records; guardians see only the children linked
to them. Online payment goes through a **mock** provider: no real bKash or
Nagad account is charged until a real gateway is integrated.

## Operator (server command line)

```mermaid
flowchart LR
    operator["👤 Server operator"]
    subgraph FB["Fly_Birds deployment"]
        direction TB
        o1(["Install and upgrade a school stack"])
        o2(["Create the first administrator (create-user)"])
        o3(["Reset a password and unlock an account (reset-password)"])
        o4(["Restore a backup into an empty database (restore-backup)"])
        o5(["Seed a demo school for training (seed-demo)"])
    end
    operator --- o1 & o2 & o3 & o4 & o5
```

See [OPERATIONS.md](../OPERATIONS.md) for each operator procedure.
