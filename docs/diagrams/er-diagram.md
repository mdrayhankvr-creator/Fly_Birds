# Entity-Relationship Diagram

Generated from the migrated PostgreSQL schema (migration `1201`, 61 tables)
by reading its foreign keys. Each school runs its own database, so there is
no `school_id` column: one database holds exactly one school.

The diagram is split by domain so each part stays readable. To keep the lines
meaningful, it leaves out the `created_by`, `updated_by`, `recorded_by`,
`entered_by` and `reviewed_by` references to `users`, which almost every
business table carries for accountability. `schema_migrations` and
`demo_records` (bookkeeping for the migration runner and the demo seed) have
no relationships.

Notation: `||--o{` is one to zero-or-more; `|o--o|` is zero-or-one to
zero-or-one. Composite foreign keys (for example a student's section must
belong to the student's session and class) appear as one relationship.

## 1. Identity, access and school settings

```mermaid
erDiagram
    roles ||--o{ users : "has"
    roles ||--o{ role_permissions : "grants"
    permissions ||--o{ role_permissions : "granted by"
    users ||--o{ auth_sessions : "signs in with"
    users ||--o{ audit_events : "acts in"
    users ||--o{ notifications : "receives"
    users ||--o{ notification_preferences : "chooses"
    users ||--o{ sms_messages : "is sent"
    users ||--o{ file_objects : "uploads"
    users ||--o{ backup_runs : "starts"
    school_settings ||--o{ school_modules : "enables"
    module_definitions ||--o{ school_modules : "defines"
    grading_schemes ||--o{ school_settings : "used by"
    grading_schemes ||--o{ grading_bands : "has"

    users {
        uuid id PK
        text email UK
        text password_hash
        text role FK
        bool is_active
        int failed_login_attempts
        timestamptz locked_until
    }
    roles {
        text id PK
        text name
    }
    permissions {
        text id PK
        text description
    }
    role_permissions {
        text role_id FK
        text permission_id FK
    }
    auth_sessions {
        uuid id PK
        uuid user_id FK
        bytea refresh_hash
        bytea csrf_hash
        timestamptz expires_at
        timestamptz revoked_at
    }
    audit_events {
        uuid id PK
        uuid actor_user_id FK
        text action
        text target_type
        text outcome
        jsonb metadata
    }
    school_settings {
        smallint id PK
        text name_en
        text name_bn
        text logo_url
        text primary_color
        text grading_scheme_id FK
    }
    school_modules {
        smallint school_id FK
        text module_key FK
        bool enabled
    }
    module_definitions {
        text key PK
        bool is_core
        text_array depends_on
    }
    grading_schemes {
        text id PK
        text name
    }
    grading_bands {
        text scheme_id FK
        text grade
        int min_marks
        numeric gpa
    }
    backup_runs {
        uuid id PK
        text status
        text sha256
        int schema_version
    }
    file_objects {
        uuid id PK
        text storage_key
        text purpose
        text sha256
    }
```

## 2. Academic structure, people and staff

```mermaid
erDiagram
    academic_sessions ||--o{ academic_sections : "runs"
    academic_classes ||--o{ academic_sections : "divided into"
    academic_classes ||--o{ academic_class_subjects : "teaches"
    academic_subjects ||--o{ academic_class_subjects : "taught in"
    academic_sections ||--o{ students : "enrols"
    academic_sessions ||--o{ students : "enrols"
    academic_classes ||--o{ students : "enrols"
    users |o--o| students : "student account"
    users |o--o| guardians : "guardian account"
    students ||--o{ student_guardians : "has"
    guardians ||--o{ student_guardians : "cares for"
    students ||--o| student_photos : "has"
    users |o--o| employees : "staff account"
    employees ||--o{ teacher_assignments : "assigned"
    academic_class_subjects ||--o{ teacher_assignments : "taught by"
    academic_sessions ||--o{ teacher_assignments : "in"
    employees ||--o{ employee_leaves : "requests"
    employees ||--o{ employee_salary_records : "paid"

    academic_sessions {
        uuid id PK
        text name
        date start_date
        date end_date
        bool is_active
    }
    academic_classes {
        uuid id PK
        text name
        int display_order
    }
    academic_sections {
        uuid id PK
        uuid session_id FK
        uuid class_id FK
        text name
    }
    academic_subjects {
        uuid id PK
        text code UK
        text name_en
        text name_bn
    }
    academic_class_subjects {
        uuid id PK
        uuid class_id FK
        uuid subject_id FK
        bool is_fourth_subject
    }
    students {
        uuid id PK
        text admission_no UK
        text name_en
        text name_bn
        uuid session_id FK
        uuid class_id FK
        uuid section_id FK
        int roll_no
        uuid user_id FK
    }
    guardians {
        uuid id PK
        text name_en
        text phone
        uuid user_id FK
    }
    student_guardians {
        uuid student_id FK
        uuid guardian_id FK
        text relationship
        bool is_primary
    }
    employees {
        uuid id PK
        text employee_code UK
        text kind
        text designation
        uuid user_id FK
    }
    teacher_assignments {
        uuid id PK
        uuid employee_id FK
        uuid session_id FK
        uuid class_id FK
        uuid subject_id FK
    }
```

## 3. Attendance and routines

```mermaid
erDiagram
    students ||--o{ student_attendance : "marked in"
    academic_sections ||--o{ student_attendance : "register of"
    student_attendance ||--o| attendance_absence_events : "raises"
    students ||--o{ attendance_absence_events : "about"
    employees ||--o{ teacher_attendance : "marked in"
    academic_sessions ||--o{ teacher_attendance : "in"
    academic_sections ||--o{ class_routines : "timetable of"
    teacher_assignments ||--o{ class_routines : "scheduled as"
    employees ||--o{ class_routines : "teaches"

    student_attendance {
        uuid id PK
        uuid student_id FK
        uuid section_id FK
        date attendance_date
        text status
        text source
    }
    attendance_absence_events {
        uuid id PK
        uuid attendance_id FK
        uuid student_id FK
        text state
    }
    teacher_attendance {
        uuid id PK
        uuid employee_id FK
        date attendance_date
        text status
    }
    class_routines {
        uuid id PK
        uuid section_id FK
        uuid subject_id FK
        uuid teacher_id FK
        int day_of_week
        int start_minute
        int end_minute
        text room
    }
```

## 4. Exams and results

```mermaid
erDiagram
    academic_sections ||--o{ exams : "sits"
    exam_routines ||--o{ exams : "schedules"
    academic_sections ||--o{ exam_routines : "for"
    exam_routines ||--o{ exam_routine_slots : "has"
    academic_class_subjects ||--o{ exam_routine_slots : "examined in"
    exams ||--o{ exam_subjects : "covers"
    academic_class_subjects ||--o{ exam_subjects : "examined as"
    exam_subjects ||--o{ exam_marks : "scored in"
    students ||--o{ exam_marks : "earns"
    exams ||--o{ exam_results : "produces"
    students ||--o{ exam_results : "receives"

    exams {
        uuid id PK
        text name
        text exam_type
        uuid section_id FK
        uuid routine_id FK
        text status
        timestamptz published_at
    }
    exam_routines {
        uuid id PK
        text name
        uuid section_id FK
        bool is_published
    }
    exam_routine_slots {
        uuid id PK
        uuid routine_id FK
        uuid subject_id FK
        date exam_date
    }
    exam_subjects {
        uuid id PK
        uuid exam_id FK
        uuid subject_id FK
        numeric full_marks
        numeric pass_marks
    }
    exam_marks {
        uuid id PK
        uuid exam_subject_id FK
        uuid student_id FK
        numeric marks
        text grade
        numeric grade_point
    }
    exam_results {
        uuid exam_id FK
        uuid student_id FK
        numeric gpa
        bool passed
        int class_position
    }
```

## 5. Fees and accounts

```mermaid
erDiagram
    fee_categories ||--o{ fee_structures : "priced in"
    academic_classes ||--o{ fee_structures : "charged to"
    academic_sessions ||--o{ fee_structures : "for"
    fee_structures ||--o{ student_fees : "bills"
    students ||--o{ student_fees : "owes"
    student_fees ||--o{ fee_adjustments : "adjusted by"
    students ||--o{ fee_concessions : "granted"
    fee_categories ||--o{ fee_concessions : "on"
    students ||--o{ fee_payments : "pays"
    fee_payments ||--o{ fee_payment_allocations : "split into"
    student_fees ||--o{ fee_payment_allocations : "settled by"
    fee_payments ||--o{ ledger_entries : "posted as"
    students ||--o{ online_payment_intents : "for"
    users ||--o{ online_payment_intents : "started by"
    fee_payments |o--o| online_payment_intents : "completes"

    fee_categories {
        uuid id PK
        text code UK
        text frequency
    }
    fee_structures {
        uuid id PK
        uuid category_id FK
        uuid class_id FK
        numeric amount
        smallint due_day
    }
    student_fees {
        uuid id PK
        uuid student_id FK
        uuid structure_id FK
        date period
        numeric amount
        numeric balance
    }
    fee_payments {
        uuid id PK
        text receipt_no UK
        uuid student_id FK
        numeric amount
        text method
        uuid idempotency_key UK
    }
    fee_payment_allocations {
        uuid payment_id FK
        uuid student_fee_id FK
        numeric amount
    }
    ledger_entries {
        uuid id PK
        text entry_type
        text category
        numeric amount
        uuid payment_id FK
    }
    online_payment_intents {
        uuid id PK
        text provider "mock bkash or nagad"
        text status
        uuid payment_id FK
    }
```

## 6. Notices, events and class diary

```mermaid
erDiagram
    notices ||--o{ notice_targets : "aimed at"
    academic_classes ||--o{ notice_targets : "receives"
    academic_sections ||--o{ notice_targets : "receives"
    notices ||--o{ notice_attachments : "has"
    file_objects ||--o{ notice_attachments : "stored as"
    school_events ||--o{ school_event_targets : "aimed at"
    academic_sections ||--o{ school_event_targets : "receives"
    academic_sections ||--o{ class_diaries : "written for"
    academic_class_subjects ||--o{ class_diaries : "about"
    users ||--o{ class_diaries : "written by"
    class_diaries ||--o{ diary_attachments : "has"
    file_objects ||--o{ diary_attachments : "stored as"
    class_diaries ||--o{ homework_submissions : "answered by"
    students ||--o{ homework_submissions : "submits"
    homework_submissions ||--o{ homework_submission_files : "has"
    file_objects ||--o{ homework_submission_files : "stored as"

    notices {
        uuid id PK
        text title_en
        text title_bn
        text priority
        text_array audience_roles
        text status
    }
    school_events {
        uuid id PK
        text kind
        date starts_on
        date ends_on
    }
    class_diaries {
        uuid id PK
        uuid section_id FK
        uuid subject_id FK
        uuid teacher_user_id FK
        date diary_date
        text homework
        text status
    }
    homework_submissions {
        uuid id PK
        uuid diary_id FK
        uuid student_id FK
        text status
    }
```

Singleton settings tables `analytics_settings` and `diary_settings` hold one
row each and reference only the user who last changed them.
