# Student Profile and Double-Sided ID Card

Branch `feature/student-profile-duplex-id-card`, from `main` `7b3723f`. This extends M3 (student profiles and ID cards with QR codes). It does not change the guardian dashboard or account model.

## 1. Profile

New optional fields on `students`, all added by migration **1601**:

- student ID (`student_code`);
- gender and blood group;
- father's and mother's names in English and Bangla;
- present and permanent address;
- admission date.

Existing rows get empty values and keep working.

- **Guardian:** the guardian name, relationship and phone shown on the profile come from the existing primary guardian link. They are read, never copied, so guardian data is not duplicated.
- **Validation:**
  - fixed lists for gender and blood group;
  - length limits;
  - the admission date must be after 1900, not before the date of birth, and at most one year ahead;
  - `student_code` is unique, ignoring letter case.
- **Backward compatibility:** a create or update that omits the new fields leaves them unchanged. An empty string clears a field.
- **UI:**
  - The profile shows Personal, Parents and guardian, Address and Academic sections.
  - Empty fields read "Not provided".
  - The student form gains a "Profile details" group with "Same as present address".
  - Everything is in English and Bangla.

## 2. Card

Cards are portrait CR80 (54 × 85.6 mm) and drawn with inline styles. They keep the school colours in light and dark dashboard themes.

| Front | Back |
|---|---|
| Logo, school name, photo, name (English or Bangla) | School name, address, phone, email |
| Student ID, father, mother, class and section, roll | Usage note and return note (school-editable, defaults in both languages) |
| Valid until, signature and signatory title (optional) | Emergency contact (optional), verification QR (optional) |

- **Validity:** the card settings date, otherwise the end of the student's academic session.
- **Long names:** they step down a size and wrap to two lines. If the face would overflow, the photo gives up height so nothing is clipped.
- **Card settings (administrators):** validity, signatory titles, emergency phone, the four notes and the QR switch. The signature is a PNG or JPEG of at most 512 KiB, re-encoded by the server and served only to administrators.

## 3. Printing

The print page is at `/admin/students/:id/card` for one student and `/admin/students/cards` for one section; both are for administrators only.

- **ID card printer (CR80):** one face per page at 54 × 85.6 mm, front then back, for duplex card printers.
- **A4 sheets:**
  - up to nine cards per sheet; each sheet of fronts is followed by its backs;
  - backs are mirrored so each lands behind its front when the sheet is flipped on the long edge;
  - optional crop marks and a centred registration mark on both sides.
- **PDF:** the browser's "Save as PDF" makes the PDF. Printing waits until protected photos and the signature have loaded. Bulk printing covers at most 300 active students, ordered by roll.

## 4. Verification QR and security

- **QR content:** `<school address>/verify/<token>`, where the token is 22 random URL-safe characters stored on the student. It holds no student ID.
- **Reissuing:** "Reissue card" replaces the token, so the old card stops verifying.
- **Public check:** `/public/id-cards/:token` returns only the state (valid, expired or inactive), the validity date and the school's name and logo. It sends `noindex`, and returns 404 for unknown tokens or when the Students module is off.
- **QR attendance:** accepts the new link and the legacy `FB-STUDENT:<id>` on cards printed earlier.
- **Card origin:** the `origin` parameter is checked against a strict pattern; otherwise the request host is used.
- **Access:**

  | Who | Cards they can open |
  |---|---|
  | Administrators and head teachers | Any student's card |
  | Students | Their own card |
  | Guardians | Their linked children's cards |
  | Teachers | None; other students and unlinked guardians get 404 |

  Bulk printing, settings, signature and reissue need `students:manage` and the administrator role.

- **Audit:** settings, signature and reissue events are audited without tokens or personal data.

## 5. API

| Method | Path | Access |
|---|---|---|
| GET | `/students/:id/card` | `students:read`, own-record scope |
| GET | `/students/cards?section_id=` | `students:manage` + administrator |
| POST | `/students/:id/card/reissue` | `students:manage` + administrator |
| GET | `/students/card-settings` | `students:read` |
| PUT | `/students/card-settings` | `students:manage` + administrator |
| GET / PUT / DELETE | `/students/card-settings/signature` | `students:manage` + administrator |
| GET | `/public/id-cards/:token` | public, Students module on |

`GET /students/:id/card` keeps its keys (`student`, `qr_payload`, `qr_data_url`) and adds `valid_until`, `school` and `settings`. `qr_payload` is now the verification link (see §4).

## 6. Verification (disposable resources only)

- **Go:** all packages pass on a fresh database, and vet is clean. `TestPostgresStudentProfileAndIDCards` covers the profile fields, access per role, the public check, reissue, QR attendance and audit content. The M3 lifecycle test now expects the verification link in place of `FB-STUDENT:<id>`.
- **Frontend:** type-check, production build, i18n (2,474 keys in each language) and 11 unit tests pass.
- **Upgrade:** two schools were upgraded through migration 1601: from 1501 with 266 students, and from 1301 with 36 students. Student and guardian-link counts were unchanged.
- **Browser and PDF:** a QA script ran 24 checks, all passing. It covered:
  - profile editing;
  - "Not provided" on older records;
  - card settings and signature;
  - CR80 PDFs in English and Bangla;
  - A4 sheets in two brandings (long names, other colours and logo);
  - light and dark themes;
  - 390 px Bangla layouts;
  - axe WCAG 2 A/AA scans;
  - role access in the UI;
  - public verification and reissue.

  The PDFs were inspected with PyMuPDF:
  - CR80 pages measure 54.02 × 85.68 mm;
  - Inter and Noto Sans Bengali are embedded;
  - photo, signature, logo and QR images are present;
  - fronts and backs align within 0.1 mm;
  - light- and dark-theme PDFs match.
- **Regression:** the browser suite, white-label and payment-safety suites all pass, the last two on fresh fixtures.

## 7. Limitations

- Class and section names print as stored, since they have no Bangla variant.
- The PDF comes from the browser's print dialog; there is no server-side PDF.
- A printer's own margins or scaling can shift alignment. The print page tells users to choose no margins and 100 % scale.
- Bulk printing is per section, at most 300 students.
