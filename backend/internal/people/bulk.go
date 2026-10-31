package people

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"gorm.io/gorm"
)

// Bulk admission from a spreadsheet (checkpoint 5). Upload, preview every row
// with its problems, then admit: all rows in one transaction or none. Rows
// already admitted (same admission number and name) are skipped, so the same
// file can be uploaded again safely, and a retried request with the same
// idempotency key answers with the first result.

const MaxBulkRows = 1000

var (
	ErrBulkHasErrors   = errors.New("some rows have problems; nothing was admitted")
	ErrBulkMissingCols = errors.New("required columns are missing")
	ErrBulkEmpty       = errors.New("the file has no student rows")
	ErrBulkKeyReused   = errors.New("this upload key already admitted a different file")
)

// BulkRow is one spreadsheet row as the preview shows it.
type BulkRow struct {
	Line        int           `json:"line"`
	AdmissionNo string        `json:"admission_no"`
	NameEN      string        `json:"name_en"`
	NameBN      string        `json:"name_bn"`
	ClassName   string        `json:"class_name"`
	SectionName string        `json:"section_name"`
	RollNo      *int          `json:"roll_no"`
	Status      string        `json:"status"` // ready, skip (already admitted) or error
	Errors      []string      `json:"errors"`
	Guardian    *BulkGuardian `json:"guardian,omitempty"`
	LoginEmail  string        `json:"login_email,omitempty"`
}

// BulkGuardian says what happens to a row's guardian: a new record, a link
// to one already on record (matched by phone or email), or the same new
// guardian as an earlier row (siblings).
type BulkGuardian struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Action       string `json:"action"` // new, existing or same_as_row
	ExistingName string `json:"existing_name,omitempty"`
	SameAsLine   int    `json:"same_as_line,omitempty"`
}

type BulkPreview struct {
	Rows           []BulkRow `json:"rows"`
	Ready          int       `json:"ready"`
	Skipped        int       `json:"skipped"`
	Errors         int       `json:"errors"`
	MissingColumns []string  `json:"missing_columns"`
	UnknownColumns []string  `json:"unknown_columns"`
	FileSHA256     string    `json:"file_sha256"`
}

type BulkResult struct {
	Admitted int            `json:"admitted"`
	Skipped  int            `json:"skipped"`
	Logins   []StudentLogin `json:"logins"`
	Replayed bool           `json:"replayed"`
}

// BulkError carries the preview when an admit is refused for row problems.
type BulkError struct{ Preview BulkPreview }

func (e *BulkError) Error() string        { return ErrBulkHasErrors.Error() }
func (e *BulkError) Is(target error) bool { return target == ErrBulkHasErrors }

// bulkEntry is a ready row: what to write.
type bulkEntry struct {
	row          int // index into the preview rows
	student      StudentInput
	guardian     *GuardianInput
	guardianID   string // existing guardian to link
	sameAs       int    // index of the earlier entry whose new guardian to share; -1 if none
	relationship string
	loginEmail   string
}

var columnAliases = map[string]string{
	"admission_number": "admission_no", "admission": "admission_no", "student_code": "student_id",
	"roll_no": "roll", "roll_number": "roll", "dob": "date_of_birth", "birth_date": "date_of_birth",
	"class_name": "class", "section_name": "section", "name": "name_en", "student_name": "name_en",
	"email": "login_email", "student_email": "login_email",
}

func headerKey(s string) string {
	k := strings.ToLower(strings.TrimSpace(s))
	k = strings.NewReplacer(" ", "_", "-", "_", ".", "").Replace(k)
	if alias, ok := columnAliases[k]; ok {
		return alias
	}
	return k
}

// parseBulkDate accepts 2014-03-21, 21/03/2014, 21-03-2014 and spreadsheet
// date serials (days since 1899-12-30).
func parseBulkDate(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "2006/01/02"} {
		if d, err := time.Parse(layout, s); err == nil {
			return d.Format("2006-01-02"), true
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 1 && f < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(f)).Format("2006-01-02"), true
	}
	return "", false
}

var genders = map[string]string{"male": "male", "m": "male", "boy": "male", "ছেলে": "male", "female": "female", "f": "female", "girl": "female", "মেয়ে": "female", "other": "other"}

// PreviewBulk checks a spreadsheet without writing anything.
func (s *Service) PreviewBulk(ctx context.Context, a Actor, sessionID string, data []byte) (BulkPreview, error) {
	if !validID(a.ID) || !validID(sessionID) {
		return BulkPreview{}, ErrInvalid
	}
	repo, ok := s.repo.(bulkRepository)
	if !ok {
		return BulkPreview{}, ErrNotFound
	}
	rows, err := ReadSheet(data)
	if err != nil {
		return BulkPreview{}, err
	}
	preview, _, err := repo.CheckBulk(ctx, a, sessionID, rows)
	if err != nil {
		return preview, err // with the missing columns, when that is the problem
	}
	preview.FileSHA256 = fileHash(data)
	return preview, nil
}

// AdmitBulk admits every ready row of a spreadsheet in one transaction, or
// nothing if any row has a problem. createLogins gives each new student a
// login; the slips come back once, in this response.
func (s *Service) AdmitBulk(ctx context.Context, a Actor, sessionID, key string, createLogins bool, data []byte) (BulkResult, error) {
	if !validID(a.ID) || !validID(sessionID) || !validID(key) {
		return BulkResult{}, ErrInvalid
	}
	repo, ok := s.repo.(bulkRepository)
	if !ok {
		return BulkResult{}, ErrNotFound
	}
	key = strings.ToLower(key)
	hash := fileHash(data)
	if done, found, err := repo.FindBulk(ctx, a, key); err != nil || found {
		if err == nil && done.hash != hash {
			return BulkResult{}, ErrBulkKeyReused
		}
		return BulkResult{Admitted: done.admitted, Skipped: done.skipped, Logins: []StudentLogin{}, Replayed: true}, err
	}
	rows, err := ReadSheet(data)
	if err != nil {
		return BulkResult{}, err
	}
	// Passwords are made and hashed before the transaction, in parallel.
	var logins []newLogin
	if createLogins {
		if logins, err = prepareLogins(len(rows)); err != nil {
			return BulkResult{}, err
		}
	}
	return repo.AdmitBulk(ctx, a, bulkRequest{sessionID: sessionID, key: key, hash: hash, rows: rows, logins: logins})
}

func fileHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// prepareLogins makes n temporary passwords and their hashes.
func prepareLogins(n int) ([]newLogin, error) {
	out := make([]newLogin, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < max(1, runtime.NumCPU()); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i], errs[i] = prepareLogin("", "")
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type bulkRequest struct {
	sessionID, key, hash string
	rows                 [][]string
	logins               []newLogin
}

type bulkRecord struct {
	hash              string
	admitted, skipped int
}

type bulkRepository interface {
	CheckBulk(ctx context.Context, a Actor, sessionID string, rows [][]string) (BulkPreview, []bulkEntry, error)
	FindBulk(ctx context.Context, a Actor, key string) (bulkRecord, bool, error)
	AdmitBulk(ctx context.Context, a Actor, in bulkRequest) (BulkResult, error)
}

func (r *GormRepository) CheckBulk(ctx context.Context, a Actor, sessionID string, rows [][]string) (BulkPreview, []bulkEntry, error) {
	db := r.db.WithContext(ctx)
	if err := requireAdmin(db, a, "students:manage"); err != nil {
		return BulkPreview{}, nil, err
	}
	return checkBulkTx(db, sessionID, rows)
}

// checkBulkTx validates every row against the file and the database.
func checkBulkTx(tx *gorm.DB, sessionID string, rows [][]string) (BulkPreview, []bulkEntry, error) {
	preview := BulkPreview{Rows: []BulkRow{}, MissingColumns: []string{}, UnknownColumns: []string{}}
	if len(rows) < 2 {
		return preview, nil, ErrBulkEmpty
	}
	known := map[string]bool{}
	for _, c := range BulkColumns {
		known[c.Key] = true
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		k := headerKey(h)
		if k == "" {
			continue
		}
		if !known[k] {
			preview.UnknownColumns = append(preview.UnknownColumns, h)
			continue
		}
		if _, dup := col[k]; !dup {
			col[k] = i
		}
	}
	for _, k := range []string{"admission_no", "name_en", "class", "section"} {
		if _, ok := col[k]; !ok {
			preview.MissingColumns = append(preview.MissingColumns, k)
		}
	}
	if len(preview.MissingColumns) > 0 {
		return preview, nil, ErrBulkMissingCols
	}
	data := rows[1:]
	if len(data) > MaxBulkRows {
		return preview, nil, ErrSheetTooBig
	}
	var sessions int64
	if err := tx.Table("academic_sessions").Where("id = ?", sessionID).Count(&sessions).Error; err != nil {
		return preview, nil, err
	}
	if sessions == 0 {
		return preview, nil, ErrNotFound
	}

	// What the database already holds, fetched once.
	var classes []struct{ ID, Name string }
	if err := tx.Table("academic_classes").Select("id, name").Scan(&classes).Error; err != nil {
		return preview, nil, err
	}
	classByName := map[string]string{}
	for _, c := range classes {
		classByName[strings.ToLower(c.Name)] = c.ID
	}
	var sections []struct{ ID, ClassID, Name string }
	if err := tx.Table("academic_sections").Select("id, class_id, name").Where("session_id = ?", sessionID).Scan(&sections).Error; err != nil {
		return preview, nil, err
	}
	sectionByName := map[string]string{}
	for _, s := range sections {
		sectionByName[s.ClassID+"|"+strings.ToLower(s.Name)] = s.ID
	}
	cell := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}
	admissionNos, emails := []string{}, []string{}
	for _, row := range data {
		if v := strings.ToLower(cell(row, "admission_no")); v != "" {
			admissionNos = append(admissionNos, v)
		}
		if v := strings.ToLower(cell(row, "login_email")); v != "" {
			emails = append(emails, v)
		}
	}
	existing := map[string]string{} // lower(admission_no) -> name_en
	if len(admissionNos) > 0 {
		var found []struct{ AdmissionNo, NameEN string }
		if err := tx.Table("students").Select("lower(admission_no) AS admission_no, name_en").Where("lower(admission_no) IN ?", admissionNos).Scan(&found).Error; err != nil {
			return preview, nil, err
		}
		for _, f := range found {
			existing[f.AdmissionNo] = f.NameEN
		}
	}
	takenRolls := map[string]bool{}
	var rolls []struct {
		SectionID string
		RollNo    int
	}
	if err := tx.Table("students").Select("section_id, roll_no").Where("session_id = ? AND roll_no IS NOT NULL", sessionID).Scan(&rolls).Error; err != nil {
		return preview, nil, err
	}
	for _, r := range rolls {
		takenRolls[fmt.Sprintf("%s|%d", r.SectionID, r.RollNo)] = true
	}
	takenEmails := map[string]bool{}
	if len(emails) > 0 {
		var found []string
		if err := tx.Table("users").Where("lower(email) IN ?", emails).Pluck("lower(email)", &found).Error; err != nil {
			return preview, nil, err
		}
		for _, e := range found {
			takenEmails[e] = true
		}
	}

	var entries []bulkEntry
	seenAdmission, seenRoll, seenEmail := map[string]bool{}, map[string]bool{}, map[string]bool{}
	newGuardians := map[string]int{} // phone digits or email -> entry index
	matchCache := map[string][]GuardianMatch{}
	today := time.Now().UTC().Format("2006-01-02")
	for i, row := range data {
		out := BulkRow{Line: i + 2, Status: "ready", Errors: []string{}}
		fail := func(code string) { out.Errors = append(out.Errors, code) }
		out.AdmissionNo, out.NameEN, out.NameBN = cell(row, "admission_no"), cell(row, "name_en"), cell(row, "name_bn")
		out.ClassName, out.SectionName = cell(row, "class"), cell(row, "section")
		v := StudentInput{AdmissionNo: out.AdmissionNo, NameEN: out.NameEN, NameBN: out.NameBN, Status: "active", SessionID: sessionID}
		if out.AdmissionNo == "" {
			fail("missing_admission_no")
		}
		if out.NameEN == "" {
			fail("missing_name_en")
		}
		switch {
		case out.ClassName == "":
			fail("missing_class")
		case classByName[strings.ToLower(out.ClassName)] == "":
			fail("class_not_found")
		default:
			v.ClassID = classByName[strings.ToLower(out.ClassName)]
			switch {
			case out.SectionName == "":
				fail("missing_section")
			case sectionByName[v.ClassID+"|"+strings.ToLower(out.SectionName)] == "":
				fail("section_not_found")
			default:
				v.SectionID = sectionByName[v.ClassID+"|"+strings.ToLower(out.SectionName)]
			}
		}
		// Already admitted by an earlier upload: skipped, not an error.
		skip := false
		if out.AdmissionNo != "" {
			k := strings.ToLower(out.AdmissionNo)
			if seenAdmission[k] {
				fail("duplicate_admission_no_in_file")
			}
			seenAdmission[k] = true
			if name, ok := existing[k]; ok {
				if strings.EqualFold(strings.TrimSpace(name), out.NameEN) {
					skip = true
				} else {
					fail("admission_no_taken")
				}
			}
		}
		if roll := cell(row, "roll"); roll != "" {
			n, err := strconv.Atoi(strings.TrimSuffix(roll, ".0"))
			if err != nil || n < 1 || n > 999999 {
				fail("bad_roll")
			} else {
				out.RollNo, v.RollNo = &n, &n
				if v.SectionID != "" {
					k := fmt.Sprintf("%s|%d", v.SectionID, n)
					if seenRoll[k] {
						fail("duplicate_roll_in_file")
					}
					seenRoll[k] = true
					if takenRolls[k] && !skip {
						fail("roll_taken")
					}
				}
			}
		}
		if d, ok := parseBulkDate(cell(row, "date_of_birth")); !ok || (d != "" && d > today) {
			fail("bad_date_of_birth")
		} else {
			v.DateOfBirth = d
		}
		if d, ok := parseBulkDate(cell(row, "admission_date")); !ok {
			fail("bad_admission_date")
		} else {
			if d == "" {
				d = today
			}
			v.AdmissionDate = &d
		}
		if g := strings.ToLower(cell(row, "gender")); g != "" {
			if norm, ok := genders[g]; ok {
				v.Gender = &norm
			} else {
				fail("bad_gender")
			}
		}
		if bg := strings.ToUpper(strings.ReplaceAll(cell(row, "blood_group"), " ", "")); bg != "" {
			if bloodGroups[bg] {
				v.BloodGroup = &bg
			} else {
				fail("bad_blood_group")
			}
		}
		for _, f := range []struct {
			field **string
			key   string
		}{{&v.StudentCode, "student_id"}, {&v.FatherNameEN, "father_name_en"}, {&v.FatherNameBN, "father_name_bn"}, {&v.MotherNameEN, "mother_name_en"}, {&v.MotherNameBN, "mother_name_bn"}, {&v.PresentAddress, "present_address"}, {&v.PermanentAddress, "permanent_address"}} {
			value := cell(row, f.key)
			*f.field = &value
		}
		if email := strings.ToLower(cell(row, "login_email")); email != "" {
			if e, err := auth.NormalizeEmail(email); err != nil || auth.IsPlaceholderEmail(e) {
				fail("bad_login_email")
			} else {
				out.LoginEmail = e
				if seenEmail[e] {
					fail("duplicate_login_email_in_file")
				}
				seenEmail[e] = true
				if takenEmails[e] && !skip {
					fail("login_email_taken")
				}
			}
		}
		entry := bulkEntry{student: v, sameAs: -1, loginEmail: out.LoginEmail}
		// Guardian: new, an existing match, or shared with an earlier row.
		gName, gPhone, gEmail := cell(row, "guardian_name"), cell(row, "guardian_phone"), strings.ToLower(cell(row, "guardian_email"))
		if gName != "" || gPhone != "" || gEmail != "" || cell(row, "guardian_name_bn") != "" {
			rel := cell(row, "guardian_relationship")
			if rel == "" {
				rel = "Guardian"
			}
			out.Guardian = &BulkGuardian{Name: gName, Relationship: rel, Action: "new"}
			g, err := normalizeGuardian(GuardianInput{NameEN: gName, NameBN: cell(row, "guardian_name_bn"), Phone: gPhone, Email: gEmail})
			switch {
			case gName == "":
				fail("missing_guardian_name")
			case gPhone == "" && gEmail == "":
				fail("missing_guardian_contact")
			case err != nil || !validText(rel, 60, true):
				fail("bad_guardian")
			default:
				digits := PhoneDigits(g.Phone)
				if g.Phone != "" && len(digits) < 10 {
					fail("bad_guardian_phone")
					break
				}
				keys := []string{}
				if digits != "" {
					keys = append(keys, "p:"+digits[max(0, len(digits)-10):])
				}
				if g.Email != "" {
					keys = append(keys, "e:"+g.Email)
				}
				cacheKey := strings.Join(keys, "|")
				matches, cached := matchCache[cacheKey]
				if !cached {
					if matches, err = guardianMatches(tx, digits, g.Email); err != nil {
						return preview, nil, err
					}
					matchCache[cacheKey] = matches
				}
				entry.relationship = rel
				switch {
				case len(matches) > 1:
					fail("guardian_ambiguous")
				case len(matches) == 1:
					out.Guardian.Action, out.Guardian.ExistingName = "existing", matches[0].NameEN
					entry.guardianID = matches[0].ID
				default:
					shared := -1
					for _, k := range keys {
						if idx, ok := newGuardians[k]; ok {
							shared = idx
							break
						}
					}
					if shared >= 0 {
						out.Guardian.Action, out.Guardian.SameAsLine = "same_as_row", preview.Rows[entries[shared].row].Line
						entry.sameAs = shared
					} else {
						entry.guardian = &g
					}
				}
			}
		}
		if len(out.Errors) == 0 && !skip {
			if _, err := normalizeStudent(v); err != nil {
				fail("bad_row")
			}
		}
		switch {
		case len(out.Errors) > 0:
			out.Status = "error"
			preview.Errors++
		case skip:
			out.Status = "skip"
			out.Guardian = nil
			preview.Skipped++
		default:
			preview.Ready++
			entry.row = len(preview.Rows)
			if entry.guardian != nil {
				digits := PhoneDigits(entry.guardian.Phone)
				if digits != "" {
					newGuardians["p:"+digits[max(0, len(digits)-10):]] = len(entries)
				}
				if entry.guardian.Email != "" {
					newGuardians["e:"+entry.guardian.Email] = len(entries)
				}
			}
			entries = append(entries, entry)
		}
		preview.Rows = append(preview.Rows, out)
	}
	return preview, entries, nil
}

func (r *GormRepository) FindBulk(ctx context.Context, a Actor, key string) (bulkRecord, bool, error) {
	db := r.db.WithContext(ctx)
	if err := requireAdmin(db, a, "students:manage"); err != nil {
		return bulkRecord{}, false, err
	}
	var rows []struct {
		FileSHA256        string
		Admitted, Skipped int
	}
	if err := db.Table("bulk_admissions").Select("file_sha256, admitted, skipped").Where("idempotency_key = ?", key).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return bulkRecord{}, false, err
	}
	return bulkRecord{hash: rows[0].FileSHA256, admitted: rows[0].Admitted, skipped: rows[0].Skipped}, true, nil
}

var errBulkKeyTaken = errors.New("bulk admission key already used")

func (r *GormRepository) AdmitBulk(ctx context.Context, a Actor, in bulkRequest) (BulkResult, error) {
	out := BulkResult{Logins: []StudentLogin{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		if len(in.logins) > 0 {
			if err := requireAdmin(tx, a, "users:create"); err != nil {
				return err
			}
		}
		if err := setActor(tx, a); err != nil {
			return err
		}
		// Uploads run one at a time, so two at once cannot both pass the
		// checks and then collide; a retry with the same key finds the first.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('bulk-admission', 1756))").Error; err != nil {
			return err
		}
		var done int64
		if err := tx.Table("bulk_admissions").Where("idempotency_key = ?", in.key).Count(&done).Error; err != nil {
			return err
		}
		if done > 0 {
			return errBulkKeyTaken
		}
		preview, entries, err := checkBulkTx(tx, in.sessionID, in.rows)
		if err != nil {
			return err
		}
		if preview.Errors > 0 {
			preview.FileSHA256 = in.hash
			return &BulkError{Preview: preview}
		}
		for _, e := range entries {
			if e.guardian != nil || e.guardianID != "" {
				if err := requireAdmin(tx, a, "guardians:manage"); err != nil {
					return err
				}
				break
			}
		}
		created := make([]string, len(entries)) // new guardian per entry
		guardiansCreated, guardiansLinked := 0, 0
		for i, e := range entries {
			studentID, err := saveStudentTx(tx, "", e.student)
			if err != nil {
				return admissionError(err)
			}
			if err := audit.Record(tx, &a.ID, "student.admitted", "student", studentID, map[string]any{"source": "import", "login_created": len(in.logins) > 0, "line": preview.Rows[e.row].Line}); err != nil {
				return err
			}
			guardianID := e.guardianID
			switch {
			case e.guardian != nil:
				g := e.guardian
				if err := tx.Raw("INSERT INTO guardians (name_en,name_bn,phone,email,address) VALUES (?,?,?,?,'') RETURNING id", g.NameEN, g.NameBN, g.Phone, g.Email).Scan(&guardianID).Error; err != nil {
					return err
				}
				if err := audit.Record(tx, &a.ID, "guardian.saved", "guardian", guardianID, map[string]any{"created": true, "source": "import"}); err != nil {
					return err
				}
				created[i] = guardianID
				guardiansCreated++
			case e.sameAs >= 0:
				guardianID = created[e.sameAs]
				guardiansLinked++
			case guardianID != "":
				guardiansLinked++
			}
			if guardianID != "" {
				if err := tx.Exec("INSERT INTO student_guardians (student_id,guardian_id,relationship,is_primary) VALUES (?,?,?,true)", studentID, guardianID, e.relationship).Error; err != nil {
					return err
				}
			}
			if len(in.logins) > 0 {
				one := in.logins[i]
				one.StudentID, one.Email = studentID, e.loginEmail
				login, err := createStudentLoginTx(tx, a.ID, one, "import")
				if err != nil {
					return err
				}
				out.Logins = append(out.Logins, login)
			}
		}
		out.Admitted, out.Skipped = len(entries), preview.Skipped
		if err := tx.Exec("INSERT INTO bulk_admissions (idempotency_key,file_sha256,session_id,admitted,skipped,logins,created_by) VALUES (?,?,?,?,?,?,?)", in.key, in.hash, in.sessionID, out.Admitted, out.Skipped, len(out.Logins), a.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "students.bulk_admitted", "session", in.sessionID, map[string]any{"admitted": out.Admitted, "skipped": out.Skipped, "logins": len(out.Logins), "guardians_created": guardiansCreated, "guardians_linked": guardiansLinked})
	})
	if errors.Is(err, errBulkKeyTaken) {
		done, _, ferr := r.FindBulk(ctx, a, in.key)
		if ferr != nil {
			return BulkResult{}, ferr
		}
		if done.hash != in.hash {
			return BulkResult{}, ErrBulkKeyReused
		}
		return BulkResult{Admitted: done.admitted, Skipped: done.skipped, Logins: []StudentLogin{}, Replayed: true}, nil
	}
	if err != nil {
		var bulk *BulkError
		if errors.As(err, &bulk) || errors.Is(err, ErrLoginTaken) || errors.Is(err, ErrNoLoginID) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrBulkEmpty) || errors.Is(err, ErrBulkMissingCols) || errors.Is(err, ErrSheetTooBig) || errors.Is(err, ErrNotFound) {
			return BulkResult{}, err
		}
		return BulkResult{}, admissionError(err)
	}
	return out, nil
}
