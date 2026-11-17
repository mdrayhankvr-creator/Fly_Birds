# Student-wise fourth-subject policy

This policy is **off by default for every class**. Migration 1814 adds a class policy record only when an administrator saves one; it does not change school settings, subject mappings, existing student choices, marks, or published results. Other schools keep their previous optional-subject behavior until they opt in.

## Current behavior and change

Previously, `academic_class_subjects.is_fourth_subject` was the whole class's role. Before any student had a choice, every class fourth subject applied to everyone. After the first choice, only each student's explicit fourth choice applied. The existing group mapping further limited group subjects. The GPA engine already caps at 5.00, adds only the optional grade point above 2.00, and ignores optional failure for pass/fail.

With a class policy enabled, an existing explicit `student_subject_choices` row wins. Otherwise, male and female students receive the administrator-configured fourth subject **only when its class and group mappings make it eligible**. Unknown or other gender remains unresolved. The other gender option is not a main subject and is absent from rosters, marks, results and new transcripts. An approved Science pair can have one chosen fourth subject and the other main subject for that student's chosen group; the pair only applies when both subjects are mapped to that group. The existing one-choice-per-student/session key still limits the student to one fourth subject.

Before activation, the system checks explicit choices across all sessions of the class. If an older choice is no longer eligible for that student's group, it reports the problem and refuses activation until an administrator reviews the choice. The policy editor also previews unsaved default assignments and unresolved students without writing choices.

The exam setup → Optional subjects screen shows effective defaults, explicit choices, unresolved students, eligible choices, published-result warnings, and class policy controls. Saving a choice after a policy class has published result versions requires explicit acknowledgement. The stored published snapshot remains unchanged; staff must use the existing result review, approval and versioned correction flow before a changed result is published. Changing policy configuration after any result version for the class is blocked.

While a policy is enabled, changing its referenced class or group mappings and deactivating its referenced subjects or swap group is blocked. The same protection applies to mappings and subjects used by explicit student choices, and group changes cannot leave an explicit choice ineligible. Disable the policy before editing those mappings when no result versions exist. Once result versions exist, changes to policy configuration or referenced mappings require a separately reviewed correction path. The Agriculture/Home Science defaults cannot also be used as the Science swap pair.

## Required administrator review

1. In Academic structure, ensure Agriculture Studies and Home Science are mapped as fourth-subject options for each intended class. Class 10 currently lacks optional mappings; the policy cannot be enabled there until approved mappings are added. The policy editor accepts school-specific subject IDs, not hard-coded codes.
2. Class 6 currently marks Religion optional. Review the applicable religion course and change its approved mapping to required before enabling the policy. If multiple courses are recognized as Religion (code `111` or an English name containing `Religion`), the system refuses policy activation until student-specific religion applicability is designed and approved. Administrators must also review any school-specific religion course names or codes that this check cannot recognize. The policy does not assign every religion course to every student.
3. For Classes 9–10, verify group-specific mappings. To enable the Biology/Higher Mathematics swap, select the Science group and two fourth-mapped subjects that both belong to that group. If the school's approved combination does not require both, leave the swap group empty.
4. Review existing choices and published results before enabling a policy. Configuration never overwrites an existing choice. Confirm each unresolved student's choice individually; do not infer gender from names.

No bulk admission, teacher import, timetable or credential operation is part of this change. No live configuration is applied by this branch.
