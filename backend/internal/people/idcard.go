package people

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"regexp"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Double-sided student ID cards: school-wide card settings, card data for one
// student or a whole section, reissue, and public verification.

type CardSettings struct {
	ValidUntil       string `json:"valid_until"`
	SignatoryTitleEN string `json:"signatory_title_en"`
	SignatoryTitleBN string `json:"signatory_title_bn"`
	HasSignature     bool   `json:"has_signature"`
	EmergencyPhone   string `json:"emergency_phone"`
	UsageNoteEN      string `json:"usage_note_en"`
	UsageNoteBN      string `json:"usage_note_bn"`
	ReturnNoteEN     string `json:"return_note_en"`
	ReturnNoteBN     string `json:"return_note_bn"`
	ShowQR           bool   `json:"show_qr"`
}
type CardSettingsInput struct {
	ValidUntil       string `json:"valid_until"`
	SignatoryTitleEN string `json:"signatory_title_en"`
	SignatoryTitleBN string `json:"signatory_title_bn"`
	EmergencyPhone   string `json:"emergency_phone"`
	UsageNoteEN      string `json:"usage_note_en"`
	UsageNoteBN      string `json:"usage_note_bn"`
	ReturnNoteEN     string `json:"return_note_en"`
	ReturnNoteBN     string `json:"return_note_bn"`
	ShowQR           bool   `json:"show_qr"`
}

// CardSchool is the school identity printed on cards; it is already public
// branding plus the contact details the school chose to print.
type CardSchool struct {
	NameEN         string `json:"name_en"`
	NameBN         string `json:"name_bn"`
	LogoURL        string `json:"logo_url"`
	Address        string `json:"address"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
}
type Card struct {
	Student    Student `json:"student"`
	ValidUntil string  `json:"valid_until"`
	// Token is placed in the verification QR by the handler; it is never
	// returned on its own.
	Token string `json:"-"`
}

// CardVerification is everything a public scan of a card reveals: no name,
// photo, identifier or contact detail of the student.
type CardVerification struct {
	State        string `json:"state"`
	ValidUntil   string `json:"valid_until"`
	SchoolNameEN string `json:"school_name_en"`
	SchoolNameBN string `json:"school_name_bn"`
	LogoURL      string `json:"logo_url"`
}

var (
	cardToken  = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	cardPhone  = regexp.MustCompile(`^[+0-9() -]{0,32}$`)
	maxSection = 300
)

// IsCardToken reports whether s has the shape of a card verification token.
func IsCardToken(s string) bool { return cardToken.MatchString(s) }

// newCardToken is 128 random bits from gen_random_uuid, base64url without padding (22 characters).
const newCardToken = `translate(rtrim(encode(decode(replace(gen_random_uuid()::text,'-',''),'hex'),'base64'),'='),'+/','-_')`

func (s *Service) CardSettings(ctx context.Context) (CardSettings, error) {
	return s.repo.CardSettings(ctx)
}
func (s *Service) SaveCardSettings(ctx context.Context, a Actor, in CardSettingsInput) (CardSettings, error) {
	in.SignatoryTitleEN, in.SignatoryTitleBN, in.EmergencyPhone = strings.TrimSpace(in.SignatoryTitleEN), strings.TrimSpace(in.SignatoryTitleBN), strings.TrimSpace(in.EmergencyPhone)
	in.UsageNoteEN, in.UsageNoteBN, in.ReturnNoteEN, in.ReturnNoteBN = strings.TrimSpace(in.UsageNoteEN), strings.TrimSpace(in.UsageNoteBN), strings.TrimSpace(in.ReturnNoteEN), strings.TrimSpace(in.ReturnNoteBN)
	if !validID(a.ID) || !validText(in.SignatoryTitleEN, 80, false) || !validText(in.SignatoryTitleBN, 160, false) || !cardPhone.MatchString(in.EmergencyPhone) ||
		!validText(in.UsageNoteEN, 400, false) || !validText(in.UsageNoteBN, 800, false) || !validText(in.ReturnNoteEN, 400, false) || !validText(in.ReturnNoteBN, 800, false) {
		return CardSettings{}, ErrInvalid
	}
	if in.ValidUntil != "" {
		if _, err := time.Parse("2006-01-02", in.ValidUntil); err != nil {
			return CardSettings{}, ErrInvalid
		}
	}
	return s.repo.SaveCardSettings(ctx, a, in)
}

// SaveSignature accepts a PNG or JPEG of at most 512 KiB and re-encodes it,
// so nothing but pixels is stored.
func (s *Service) SaveSignature(ctx context.Context, a Actor, p Photo) error {
	if !validID(a.ID) || len(p.Content) < 1 || len(p.Content) > 524288 {
		return ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(p.Content))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 2000 || config.Height > 2000 || (format == "jpeg" && p.ContentType != "image/jpeg") || (format == "png" && p.ContentType != "image/png") || (format != "jpeg" && format != "png") {
		return ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(p.Content))
	if err != nil {
		return ErrInvalid
	}
	var clean bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&clean, decoded, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&clean, decoded)
	}
	if err != nil || clean.Len() > 524288 {
		return ErrInvalid
	}
	p.Content = clean.Bytes()
	return s.repo.SaveSignature(ctx, a, &p)
}
func (s *Service) DeleteSignature(ctx context.Context, a Actor) error {
	if !validID(a.ID) {
		return ErrInvalid
	}
	return s.repo.SaveSignature(ctx, a, nil)
}
func (s *Service) Signature(ctx context.Context, a Actor) (Photo, error) {
	if !validID(a.ID) {
		return Photo{}, ErrInvalid
	}
	return s.repo.Signature(ctx, a)
}
func (s *Service) StudentCard(ctx context.Context, a Actor, id string) (Card, error) {
	if !validID(a.ID) || !validID(id) {
		return Card{}, ErrInvalid
	}
	return s.repo.StudentCard(ctx, a, id)
}
func (s *Service) SectionCards(ctx context.Context, a Actor, sectionID string) ([]Card, error) {
	if !validID(a.ID) || !validID(sectionID) {
		return nil, ErrInvalid
	}
	return s.repo.SectionCards(ctx, a, sectionID)
}
func (s *Service) ReissueCard(ctx context.Context, a Actor, id string) error {
	if !validID(a.ID) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.ReissueCard(ctx, a, id)
}
func (s *Service) CardSchool(ctx context.Context) (CardSchool, error) {
	return s.repo.CardSchool(ctx)
}
func (s *Service) VerifyCard(ctx context.Context, token string) (CardVerification, error) {
	if !IsCardToken(token) {
		return CardVerification{}, ErrNotFound
	}
	return s.repo.VerifyCard(ctx, token, time.Now().UTC().Format("2006-01-02"))
}

const cardSettingsColumns = "COALESCE(to_char(valid_until,'YYYY-MM-DD'),'') AS valid_until,signatory_title_en,signatory_title_bn,(signature IS NOT NULL) AS has_signature,emergency_phone,usage_note_en,usage_note_bn,return_note_en,return_note_bn,show_qr"

func (r *GormRepository) CardSettings(ctx context.Context) (CardSettings, error) {
	var v CardSettings
	err := r.db.WithContext(ctx).Table("id_card_settings").Select(cardSettingsColumns).Where("id=1").Take(&v).Error
	return v, err
}
func (r *GormRepository) SaveCardSettings(ctx context.Context, a Actor, in CardSettingsInput) (CardSettings, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE id_card_settings SET valid_until=NULLIF(?,'')::date,signatory_title_en=?,signatory_title_bn=?,emergency_phone=?,usage_note_en=?,usage_note_bn=?,return_note_en=?,return_note_bn=?,show_qr=?,updated_at=now() WHERE id=1",
			in.ValidUntil, in.SignatoryTitleEN, in.SignatoryTitleBN, in.EmergencyPhone, in.UsageNoteEN, in.UsageNoteBN, in.ReturnNoteEN, in.ReturnNoteBN, in.ShowQR).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "id_card.settings_saved", "id_card_settings", "1", map[string]any{"show_qr": in.ShowQR, "valid_until": in.ValidUntil})
	})
	if err != nil {
		return CardSettings{}, databaseError(err)
	}
	return r.CardSettings(ctx)
}

// SaveSignature replaces the signature, or removes it when p is nil.
func (r *GormRepository) SaveSignature(ctx context.Context, a Actor, p *Photo) error {
	return databaseError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		var contentType, content any
		if p != nil {
			contentType, content = p.ContentType, p.Content
		}
		if err := tx.Exec("UPDATE id_card_settings SET signature_type=?,signature=?,updated_at=now() WHERE id=1", contentType, content).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "id_card.signature_saved", "id_card_settings", "1", map[string]any{"removed": p == nil})
	}))
}

// Signature is readable only by administrators who print cards.
func (r *GormRepository) Signature(ctx context.Context, a Actor) (Photo, error) {
	if err := requireAdmin(r.db.WithContext(ctx), a, "students:manage"); err != nil {
		return Photo{}, err
	}
	var p Photo
	err := r.db.WithContext(ctx).Table("id_card_settings").Select("signature_type AS content_type,signature AS content").Where("id=1 AND signature IS NOT NULL").Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Photo{}, ErrNotFound
	}
	return p, err
}
func (r *GormRepository) CardSchool(ctx context.Context) (CardSchool, error) {
	var v CardSchool
	err := r.db.WithContext(ctx).Table("school_settings").Select("name_en,name_bn,logo_url,address,phone,email,primary_color,secondary_color").Where("id=1").Take(&v).Error
	return v, err
}

type cardRow struct {
	ID         string
	ValidUntil string
	CardToken  string
}

// cardRows issues missing tokens and returns validity and tokens for the given students.
func cardRows(tx *gorm.DB, ids []string) (map[string]cardRow, error) {
	if err := tx.Exec("UPDATE students SET card_token="+newCardToken+" WHERE id IN ? AND card_token IS NULL", ids).Error; err != nil {
		return nil, err
	}
	var rows []cardRow
	err := tx.Raw("SELECT s.id,to_char(COALESCE(cs.valid_until,a.end_date),'YYYY-MM-DD') AS valid_until,s.card_token FROM students s JOIN academic_sessions a ON a.id=s.session_id CROSS JOIN id_card_settings cs WHERE cs.id=1 AND s.id IN ?", ids).Scan(&rows).Error
	out := make(map[string]cardRow, len(rows))
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, err
}

// StudentCard applies the same scope as reading the student: staff, the
// student, or a currently linked guardian.
func (r *GormRepository) StudentCard(ctx context.Context, a Actor, id string) (Card, error) {
	student, err := r.GetStudent(ctx, a, id)
	if err != nil {
		return Card{}, err
	}
	rows, err := cardRows(r.db.WithContext(ctx), []string{student.ID})
	if err != nil {
		return Card{}, err
	}
	row := rows[student.ID]
	return Card{Student: student, ValidUntil: row.ValidUntil, Token: row.CardToken}, nil
}

// SectionCards returns cards for the active students of one section, for bulk printing by administrators.
func (r *GormRepository) SectionCards(ctx context.Context, a Actor, sectionID string) ([]Card, error) {
	db := r.db.WithContext(ctx)
	if err := requireAdmin(db, a, "students:manage"); err != nil {
		return nil, err
	}
	var students []Student
	if err := studentBase(db).Select(studentColumns).Where("s.section_id=? AND s.status='active'", sectionID).Order("s.roll_no NULLS LAST,s.name_en,s.id").Limit(maxSection).Scan(&students).Error; err != nil {
		return nil, err
	}
	out := make([]Card, 0, len(students))
	if len(students) == 0 {
		return out, nil
	}
	ids := make([]string, len(students))
	for i, s := range students {
		ids[i] = s.ID
	}
	var guardians []struct {
		StudentID string
		GuardianSummary
	}
	if err := db.Table("student_guardians AS sg").Joins("JOIN guardians AS g ON g.id=sg.guardian_id").Select("sg.student_id,g.name_en,g.name_bn,sg.relationship,g.phone").Where("sg.student_id IN ? AND sg.is_primary", ids).Scan(&guardians).Error; err != nil {
		return nil, err
	}
	primary := map[string]GuardianSummary{}
	for _, g := range guardians {
		primary[g.StudentID] = g.GuardianSummary
	}
	rows, err := cardRows(db, ids)
	if err != nil {
		return nil, err
	}
	for _, s := range students {
		if g, ok := primary[s.ID]; ok {
			g := g
			s.PrimaryGuardian = &g
		}
		out = append(out, Card{Student: s, ValidUntil: rows[s.ID].ValidUntil, Token: rows[s.ID].CardToken})
	}
	return out, nil
}

// ReissueCard replaces the student's card token; the old card's QR stops verifying.
func (r *GormRepository) ReissueCard(ctx context.Context, a Actor, id string) error {
	return databaseError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		res := tx.Exec("UPDATE students SET card_token="+newCardToken+",updated_at=now() WHERE id=?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &a.ID, "student.card_reissued", "student", id, nil)
	}))
}
func (r *GormRepository) VerifyCard(ctx context.Context, token, today string) (CardVerification, error) {
	var row struct {
		Status     string
		ValidUntil string
	}
	db := r.db.WithContext(ctx)
	err := db.Raw("SELECT s.status,to_char(COALESCE(cs.valid_until,a.end_date),'YYYY-MM-DD') AS valid_until FROM students s JOIN academic_sessions a ON a.id=s.session_id CROSS JOIN id_card_settings cs WHERE cs.id=1 AND s.card_token=?", token).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CardVerification{}, ErrNotFound
	}
	if err != nil {
		return CardVerification{}, err
	}
	school, err := r.CardSchool(ctx)
	if err != nil {
		return CardVerification{}, err
	}
	state := "valid"
	if row.Status != "active" {
		state = "inactive"
	} else if row.ValidUntil < today {
		state = "expired"
	}
	return CardVerification{State: state, ValidUntil: row.ValidUntil, SchoolNameEN: school.NameEN, SchoolNameBN: school.NameBN, LogoURL: school.LogoURL}, nil
}
