package school

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// The public school page (checkpoint 5): what anyone may read below the
// sign-in form. The administrator writes every word of it in English and
// Bangla; each section appears only when it has content. Nothing about an
// individual student (results, fees, records) is ever part of it: the only
// figures are optional totals of active students and teachers.

const (
	MaxGalleryPhotos   = 48
	MaxGalleryUpload   = 8 << 20
	galleryEdge        = 1600 // longest side of a gallery photo
	galleryThumbEdge   = 480  // longest side of its grid copy
	maxGalleryStored   = 1 << 20
	maxGalleryThumb    = 128 << 10
	headPhotoEdge      = 480
	maxHeadPhotoStored = 512 << 10
)

var (
	ErrGalleryLimit = errors.New("the gallery is full")
	eiinPattern     = regexp.MustCompile(`^([0-9]{6})?$`)
)

// PublicPage is the page as the administrator edits it. The school details
// in it (founded year, EIIN, head teacher's name, contact address, phone,
// email and map link) are school settings (migration 1759): the editor shows
// and saves the same values as GET and PUT /school/settings, under these
// names: founded_year, eiin, head_teacher_name_en/bn, address, address_bn,
// phone, email and map_url.
type PublicPage struct {
	DesignStyle      string `json:"design_style"`
	PassRate         *int   `json:"pass_rate"`
	AboutEN          string `json:"about_en"`
	AboutBN          string `json:"about_bn"`
	FoundedYear      *int   `json:"founded_year"`
	EIIN             string `json:"eiin"`
	ShowCounts       bool   `json:"show_counts"`
	HeadNameEN       string `json:"head_name_en"`
	HeadNameBN       string `json:"head_name_bn"`
	HeadMessageEN    string `json:"head_message_en"`
	HeadMessageBN    string `json:"head_message_bn"`
	HeadPhotoVersion string `json:"head_photo_version"`
	ContactAddressEN string `json:"contact_address_en"`
	ContactAddressBN string `json:"contact_address_bn"`
	ContactPhone     string `json:"contact_phone"`
	ContactEmail     string `json:"contact_email"`
	MapURL           string `json:"map_url"`
}

// PublicPageInput is everything but the photo, which has its own upload.
type PublicPageInput struct {
	DesignStyle      string `json:"design_style"`
	PassRate         *int   `json:"pass_rate"`
	AboutEN          string `json:"about_en"`
	AboutBN          string `json:"about_bn"`
	FoundedYear      *int   `json:"founded_year"`
	EIIN             string `json:"eiin"`
	ShowCounts       bool   `json:"show_counts"`
	HeadNameEN       string `json:"head_name_en"`
	HeadNameBN       string `json:"head_name_bn"`
	HeadMessageEN    string `json:"head_message_en"`
	HeadMessageBN    string `json:"head_message_bn"`
	ContactAddressEN string `json:"contact_address_en"`
	ContactAddressBN string `json:"contact_address_bn"`
	ContactPhone     string `json:"contact_phone"`
	ContactEmail     string `json:"contact_email"`
	MapURL           string `json:"map_url"`
}

type GalleryPhoto struct {
	ID        string `json:"id"`
	Position  int    `json:"position"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Version   string `json:"version"`
	CaptionEN string `json:"caption_en"`
	CaptionBN string `json:"caption_bn"`
}

type PublicNotice struct {
	ID        string `json:"id"`
	TitleEN   string `json:"title_en"`
	TitleBN   string `json:"title_bn"`
	BodyEN    string `json:"body_en"`
	BodyBN    string `json:"body_bn"`
	Priority  string `json:"priority"`
	PublishAt string `json:"publish_at"`
}

// PublicView is what anyone may read. A nil section has no content and is
// not shown.
type PublicView struct {
	DesignStyle string         `json:"design_style"`
	About       *PublicAbout   `json:"about"`
	Head        *PublicHead    `json:"head"`
	Gallery     []GalleryPhoto `json:"gallery"`
	Notices     []PublicNotice `json:"notices"`
	Contact     *PublicContact `json:"contact"`
}
type PublicAbout struct {
	PassRate    *int   `json:"pass_rate,omitempty"`
	AboutEN     string `json:"about_en"`
	AboutBN     string `json:"about_bn"`
	FoundedYear *int   `json:"founded_year,omitempty"`
	EIIN        string `json:"eiin,omitempty"`
	Students    *int64 `json:"students,omitempty"`
	Teachers    *int64 `json:"teachers,omitempty"`
}
type PublicHead struct {
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	MessageEN    string `json:"message_en"`
	MessageBN    string `json:"message_bn"`
	PhotoVersion string `json:"photo_version,omitempty"`
}
type PublicContact struct {
	AddressEN string `json:"address_en"`
	AddressBN string `json:"address_bn"`
	Email     string `json:"email"`
	MapURL    string `json:"map_url"`
}

type publicRepository interface {
	PublicPage(ctx context.Context) (PublicPage, error)
	PublicCounts(ctx context.Context) (int64, int64, error)
	PublicNotices(ctx context.Context) ([]PublicNotice, error)
	Gallery(ctx context.Context) ([]GalleryPhoto, error)
	SavePublicPage(ctx context.Context, actor string, in PublicPageInput) (PublicPage, error)
	SetHeadPhoto(ctx context.Context, actor string, content []byte, version string) error
	HeadPhoto(ctx context.Context) (SlideImage, error)
	AddGalleryPhoto(ctx context.Context, actor string, full, thumb []byte, width, height int, version string) (GalleryPhoto, error)
	UpdateGalleryCaption(ctx context.Context, actor, id string, caption SlideCaption) (GalleryPhoto, error)
	DeleteGalleryPhoto(ctx context.Context, actor, id string) error
	GalleryImage(ctx context.Context, id string, thumb bool) (SlideImage, error)
}

func (s *Service) public() (publicRepository, error) {
	repo, ok := s.repo.(publicRepository)
	if !ok {
		return nil, ErrNotFound
	}
	return repo, nil
}

// PublicView assembles the page for anyone, leaving out empty sections.
func (s *Service) PublicView(ctx context.Context) (PublicView, error) {
	repo, err := s.public()
	if err != nil {
		return PublicView{}, err
	}
	p, err := repo.PublicPage(ctx)
	if err != nil {
		return PublicView{}, err
	}
	out := PublicView{DesignStyle: p.DesignStyle, Gallery: []GalleryPhoto{}, Notices: []PublicNotice{}}
	if p.AboutEN != "" || p.AboutBN != "" || p.FoundedYear != nil || p.EIIN != "" || p.ShowCounts || p.PassRate != nil {
		about := &PublicAbout{AboutEN: p.AboutEN, AboutBN: p.AboutBN, FoundedYear: p.FoundedYear, EIIN: p.EIIN, PassRate: p.PassRate}
		if p.ShowCounts {
			students, teachers, err := repo.PublicCounts(ctx)
			if err != nil {
				return PublicView{}, err
			}
			about.Students, about.Teachers = &students, &teachers
		}
		out.About = about
	}
	if p.HeadMessageEN != "" || p.HeadMessageBN != "" {
		out.Head = &PublicHead{NameEN: p.HeadNameEN, NameBN: p.HeadNameBN, MessageEN: p.HeadMessageEN, MessageBN: p.HeadMessageBN, PhotoVersion: p.HeadPhotoVersion}
	}
	if p.ContactAddressEN != "" || p.ContactAddressBN != "" || p.ContactEmail != "" || p.MapURL != "" {
		out.Contact = &PublicContact{AddressEN: p.ContactAddressEN, AddressBN: p.ContactAddressBN, Email: p.ContactEmail, MapURL: p.MapURL}
	}
	if out.Gallery, err = repo.Gallery(ctx); err != nil {
		return PublicView{}, err
	}
	if out.Notices, err = repo.PublicNotices(ctx); err != nil {
		return PublicView{}, err
	}
	return out, nil
}

// PublicPageForEdit returns every field for the administrator's editor.
func (s *Service) PublicPageForEdit(ctx context.Context) (PublicPage, []GalleryPhoto, error) {
	repo, err := s.public()
	if err != nil {
		return PublicPage{}, nil, err
	}
	p, err := repo.PublicPage(ctx)
	if err != nil {
		return PublicPage{}, nil, err
	}
	gallery, err := repo.Gallery(ctx)
	return p, gallery, err
}

// validText accepts text of at most max characters; lines are allowed when
// multiline, other control characters never.
func validPublicText(value string, max int, multiline bool) bool {
	if utf8.RuneCountInString(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return false
		}
	}
	return true
}

func (s *Service) SavePublicPage(ctx context.Context, actor string, in PublicPageInput) (PublicPage, error) {
	repo, err := s.public()
	if err != nil {
		return PublicPage{}, err
	}
	trim := func(v *string) { *v = strings.TrimSpace(strings.ReplaceAll(*v, "\r\n", "\n")) }
	for _, v := range []*string{&in.AboutEN, &in.AboutBN, &in.EIIN, &in.HeadNameEN, &in.HeadNameBN, &in.HeadMessageEN, &in.HeadMessageBN, &in.ContactAddressEN, &in.ContactAddressBN, &in.ContactPhone, &in.ContactEmail, &in.MapURL} {
		trim(v)
	}
	in.ContactEmail = strings.ToLower(in.ContactEmail)
	if in.DesignStyle == "" {
		in.DesignStyle = "modern_bold"
	}
	ok := actorID.MatchString(actor) &&
		(in.DesignStyle == "modern_bold" || in.DesignStyle == "classic" || in.DesignStyle == "minimal") &&
		(in.PassRate == nil || (*in.PassRate >= 0 && *in.PassRate <= 100)) &&
		validPublicText(in.AboutEN, 3000, true) && validPublicText(in.AboutBN, 3000, true) &&
		validPublicText(in.HeadMessageEN, 3000, true) && validPublicText(in.HeadMessageBN, 3000, true) &&
		validLine(in.HeadNameEN, 160) && validLine(in.HeadNameBN, 160) &&
		// The details below are school settings, checked by the same rules.
		validPublicText(in.ContactAddressEN, 500, true) && validPublicText(in.ContactAddressBN, 500, true) &&
		(in.ContactPhone == "" || (phoneChars.MatchString(in.ContactPhone) && len(in.ContactPhone) <= 20)) &&
		eiinPattern.MatchString(in.EIIN) && validMapURL(in.MapURL) &&
		(in.FoundedYear == nil || (*in.FoundedYear >= 1800 && *in.FoundedYear <= 2100))
	if !ok {
		return PublicPage{}, ErrInvalid
	}
	if in.ContactEmail != "" {
		if parsed, err := mail.ParseAddress(in.ContactEmail); err != nil || parsed.Address != in.ContactEmail || len(in.ContactEmail) > 254 {
			return PublicPage{}, ErrInvalid
		}
	}
	return repo.SavePublicPage(ctx, actor, in)
}

// decodeUpload checks an uploaded JPEG or PNG and decodes it.
func decodeUpload(contentType string, data []byte, maxBytes int) (image.Image, error) {
	if len(data) < 1 || len(data) > maxBytes {
		return nil, ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format == "jpeg" && contentType != "image/jpeg") || (format == "png" && contentType != "image/png") || (format != "jpeg" && format != "png") {
		return nil, ErrInvalid
	}
	if config.Width < 200 || config.Height < 200 || config.Width > 8000 || config.Height > 8000 {
		return nil, ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	return decoded, nil
}

func encodeJPEG(img image.Image, quality, limit int) ([]byte, error) {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil || out.Len() > limit {
		return nil, ErrInvalid
	}
	return out.Bytes(), nil
}

// SetHeadPhoto stores the head teacher's photo, cropped square and resized.
func (s *Service) SetHeadPhoto(ctx context.Context, actor, contentType string, data []byte) (PublicPage, error) {
	repo, err := s.public()
	if err != nil {
		return PublicPage{}, err
	}
	if !actorID.MatchString(actor) {
		return PublicPage{}, ErrInvalid
	}
	decoded, err := decodeUpload(contentType, data, MaxGalleryUpload)
	if err != nil {
		return PublicPage{}, err
	}
	b := decoded.Bounds()
	side := min(b.Dx(), b.Dy())
	square := scaleDown(decoded, image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, b.Min.X+(b.Dx()-side)/2+side, b.Min.Y+(b.Dy()-side)/2+side), headPhotoEdge, headPhotoEdge)
	content, err := encodeJPEG(square, 84, maxHeadPhotoStored)
	if err != nil {
		return PublicPage{}, err
	}
	sum := sha256.Sum256(content)
	if err := repo.SetHeadPhoto(ctx, actor, content, hex.EncodeToString(sum[:6])); err != nil {
		return PublicPage{}, err
	}
	return repo.PublicPage(ctx)
}

func (s *Service) DeleteHeadPhoto(ctx context.Context, actor string) (PublicPage, error) {
	repo, err := s.public()
	if err != nil {
		return PublicPage{}, err
	}
	if !actorID.MatchString(actor) {
		return PublicPage{}, ErrInvalid
	}
	if err := repo.SetHeadPhoto(ctx, actor, nil, ""); err != nil {
		return PublicPage{}, err
	}
	return repo.PublicPage(ctx)
}

func (s *Service) HeadPhoto(ctx context.Context) (SlideImage, error) {
	repo, err := s.public()
	if err != nil {
		return SlideImage{}, err
	}
	return repo.HeadPhoto(ctx)
}

// AddGalleryPhoto stores a photo at most 1600px on its longest side, and a
// 480px copy for the grid. Re-encoding drops metadata such as location.
func (s *Service) AddGalleryPhoto(ctx context.Context, actor, contentType string, data []byte) (GalleryPhoto, error) {
	repo, err := s.public()
	if err != nil {
		return GalleryPhoto{}, err
	}
	if !actorID.MatchString(actor) {
		return GalleryPhoto{}, ErrInvalid
	}
	decoded, err := decodeUpload(contentType, data, MaxGalleryUpload)
	if err != nil {
		return GalleryPhoto{}, err
	}
	b := decoded.Bounds()
	fw, fh := fit(b.Dx(), b.Dy(), galleryEdge)
	full, err := encodeJPEG(scaleDown(decoded, b, fw, fh), 80, maxGalleryStored)
	if err != nil {
		return GalleryPhoto{}, err
	}
	tw, th := fit(b.Dx(), b.Dy(), galleryThumbEdge)
	thumb, err := encodeJPEG(scaleDown(decoded, b, tw, th), 74, maxGalleryThumb)
	if err != nil {
		return GalleryPhoto{}, err
	}
	sum := sha256.Sum256(full)
	return repo.AddGalleryPhoto(ctx, actor, full, thumb, fw, fh, hex.EncodeToString(sum[:8]))
}

func (s *Service) UpdateGalleryCaption(ctx context.Context, actor, id string, caption SlideCaption) (GalleryPhoto, error) {
	repo, err := s.public()
	if err != nil {
		return GalleryPhoto{}, err
	}
	caption.CaptionEN, caption.CaptionBN = strings.TrimSpace(caption.CaptionEN), strings.TrimSpace(caption.CaptionBN)
	if !actorID.MatchString(actor) || !actorID.MatchString(id) || !validLine(caption.CaptionEN, 140) || !validLine(caption.CaptionBN, 140) {
		return GalleryPhoto{}, ErrInvalid
	}
	return repo.UpdateGalleryCaption(ctx, actor, id, caption)
}

func (s *Service) DeleteGalleryPhoto(ctx context.Context, actor, id string) error {
	repo, err := s.public()
	if err != nil {
		return err
	}
	if !actorID.MatchString(actor) || !actorID.MatchString(id) {
		return ErrInvalid
	}
	return repo.DeleteGalleryPhoto(ctx, actor, id)
}

func (s *Service) GalleryImage(ctx context.Context, id string, thumb bool) (SlideImage, error) {
	repo, err := s.public()
	if err != nil {
		return SlideImage{}, err
	}
	if !actorID.MatchString(id) {
		return SlideImage{}, ErrNotFound
	}
	return repo.GalleryImage(ctx, id, thumb)
}

// fit scales w×h down (never up) so the longest side is at most edge.
func fit(w, h, edge int) (int, int) {
	if w <= edge && h <= edge {
		return w, h
	}
	if w >= h {
		return edge, max(1, h*edge/w)
	}
	return max(1, w*edge/h), edge
}

// scaleDown draws the src rectangle r of img into a w×h image, averaging the
// source pixels under each output pixel and flattening transparency onto
// white.
func scaleDown(img image.Image, r image.Rectangle, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	rw, rh := r.Dx(), r.Dy()
	for oy := 0; oy < h; oy++ {
		sy0, sy1 := r.Min.Y+oy*rh/h, r.Min.Y+(oy+1)*rh/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for ox := 0; ox < w; ox++ {
			sx0, sx1 := r.Min.X+ox*rw/w, r.Min.X+(ox+1)*rw/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var cr, cg, cb, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					pr, pg, pb, pa := img.At(x, y).RGBA()
					white := 0xffff - uint64(pa)
					cr += uint64(pr) + white
					cg += uint64(pg) + white
					cb += uint64(pb) + white
					n++
				}
			}
			i := dst.PixOffset(ox, oy)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(cr/n>>8), uint8(cg/n>>8), uint8(cb/n>>8), 0xff
		}
	}
	return dst
}

// The page's own text from school_public_page; the school details from
// school_settings.
const publicPageColumns = "p.design_style, p.pass_rate, p.about_en, p.about_bn, s.founded_year, s.eiin, p.show_counts, s.head_teacher_name_en AS head_name_en, s.head_teacher_name_bn AS head_name_bn, p.head_message_en, p.head_message_bn, p.head_photo_version, s.address AS contact_address_en, s.address_bn AS contact_address_bn, s.phone AS contact_phone, s.email AS contact_email, s.map_url"
const galleryColumns = "id::text AS id, position, width, height, version, caption_en, caption_bn"

func (r *GormRepository) PublicPage(ctx context.Context) (PublicPage, error) {
	var out PublicPage
	err := r.db.WithContext(ctx).Table("school_settings AS s").Joins("JOIN school_public_page AS p ON p.id = 1").Select(publicPageColumns).Where("s.id = ?", SingleSchoolID).Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PublicPage{}, nil
	}
	return out, err
}

func (r *GormRepository) PublicCounts(ctx context.Context) (int64, int64, error) {
	var students, teachers int64
	db := r.db.WithContext(ctx)
	if err := db.Table("students").Where("status = 'active'").Count(&students).Error; err != nil {
		return 0, 0, err
	}
	if err := db.Table("employees").Where("kind = 'teacher' AND is_active").Count(&teachers).Error; err != nil {
		return 0, 0, err
	}
	return students, teachers, nil
}

// PublicNotices lists the live notices their authors chose to show publicly:
// title and text only, the newest six, each body cut to 1000 characters.
func (r *GormRepository) PublicNotices(ctx context.Context) ([]PublicNotice, error) {
	out := []PublicNotice{}
	err := r.db.WithContext(ctx).Table("notices").
		Select("id::text AS id, title_en, title_bn, left(body_en, 1000) AS body_en, left(body_bn, 1000) AS body_bn, priority, to_char(publish_at AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') AS publish_at").
		Where("show_public AND status = 'published' AND publish_at <= now() AND (expires_at IS NULL OR expires_at > now())").
		Order("publish_at DESC, id").Limit(6).Scan(&out).Error
	return out, err
}

func (r *GormRepository) Gallery(ctx context.Context) ([]GalleryPhoto, error) {
	out := []GalleryPhoto{}
	err := r.db.WithContext(ctx).Table("school_gallery").Select(galleryColumns).Order("position").Scan(&out).Error
	return out, err
}

func (r *GormRepository) SavePublicPage(ctx context.Context, actor string, in PublicPageInput) (PublicPage, error) {
	err := r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE school_public_page SET design_style=?, pass_rate=?, about_en=?, about_bn=?, show_counts=?, head_message_en=?, head_message_bn=?, updated_by=?, updated_at=now() WHERE id = 1`,
			in.DesignStyle, in.PassRate, in.AboutEN, in.AboutBN, in.ShowCounts, in.HeadMessageEN, in.HeadMessageBN, actor).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE school_settings SET founded_year=?, eiin=?, head_teacher_name_en=?, head_teacher_name_bn=?, address=?, address_bn=?, phone=?, email=?, map_url=?, updated_at=now() WHERE id = ?`,
			in.FoundedYear, in.EIIN, in.HeadNameEN, in.HeadNameBN, in.ContactAddressEN, in.ContactAddressBN, in.ContactPhone, in.ContactEmail, in.MapURL, SingleSchoolID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.public_page_saved", "school", "1", map[string]any{"show_counts": in.ShowCounts, "settings_fields": []string{"address", "address_bn", "email", "eiin", "founded_year", "head_teacher_name_bn", "head_teacher_name_en", "map_url", "phone"}})
	})
	if err != nil {
		return PublicPage{}, err
	}
	return r.PublicPage(ctx)
}

func (r *GormRepository) SetHeadPhoto(ctx context.Context, actor string, content []byte, version string) error {
	return r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE school_public_page SET head_photo = ?, head_photo_version = ?, updated_by = ?, updated_at = now() WHERE id = 1", content, version, actor).Error; err != nil {
			return err
		}
		action := "school.head_photo_saved"
		if content == nil {
			action = "school.head_photo_removed"
		}
		return audit.Record(tx, &actor, action, "school", "1", nil)
	})
}

func (r *GormRepository) HeadPhoto(ctx context.Context) (SlideImage, error) {
	var row struct {
		Content []byte
		Version string
	}
	if err := r.db.WithContext(ctx).Table("school_public_page").Select("head_photo AS content, head_photo_version AS version").Where("id = 1").Scan(&row).Error; err != nil {
		return SlideImage{}, err
	}
	if len(row.Content) == 0 {
		return SlideImage{}, ErrNotFound
	}
	return SlideImage{ContentType: "image/jpeg", Content: row.Content, Version: row.Version}, nil
}

func (r *GormRepository) AddGalleryPhoto(ctx context.Context, actor string, full, thumb []byte, width, height int, version string) (GalleryPhoto, error) {
	var out GalleryPhoto
	err := r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("school_gallery").Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxGalleryPhotos {
			return ErrGalleryLimit
		}
		var id string
		if err := tx.Raw("INSERT INTO school_gallery (position, content, thumb, width, height, version, created_by) VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id::text", count, full, thumb, width, height, version, actor).Scan(&id).Error; err != nil {
			return err
		}
		if err := tx.Table("school_gallery").Select(galleryColumns).Where("id = ?", id).Scan(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.gallery_photo_added", "school_gallery", id, map[string]any{"width": width, "height": height, "bytes": len(full)})
	})
	return out, err
}

func (r *GormRepository) UpdateGalleryCaption(ctx context.Context, actor, id string, caption SlideCaption) (GalleryPhoto, error) {
	var out GalleryPhoto
	err := r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		res := tx.Exec("UPDATE school_gallery SET caption_en = ?, caption_bn = ? WHERE id = ?", caption.CaptionEN, caption.CaptionBN, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Table("school_gallery").Select(galleryColumns).Where("id = ?", id).Scan(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.gallery_caption_updated", "school_gallery", id, nil)
	})
	return out, err
}

func (r *GormRepository) DeleteGalleryPhoto(ctx context.Context, actor, id string) error {
	return r.slideWrite(ctx, actor, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM school_gallery WHERE id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Exec("UPDATE school_gallery AS g SET position = o.rn - 1 FROM (SELECT id, row_number() OVER (ORDER BY position) AS rn FROM school_gallery) AS o WHERE g.id = o.id").Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.gallery_photo_removed", "school_gallery", id, nil)
	})
}

func (r *GormRepository) GalleryImage(ctx context.Context, id string, thumb bool) (SlideImage, error) {
	column := "content"
	if thumb {
		column = "thumb"
	}
	var row struct {
		Content []byte
		Version string
	}
	res := r.db.WithContext(ctx).Table("school_gallery").Select(column+" AS content, version").Where("id = ?", id).Scan(&row)
	if res.Error != nil {
		return SlideImage{}, res.Error
	}
	if res.RowsAffected == 0 {
		return SlideImage{}, ErrNotFound
	}
	return SlideImage{ContentType: "image/jpeg", Content: row.Content, Version: row.Version}, nil
}
