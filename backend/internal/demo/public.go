package demo

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"gorm.io/gorm"
)

// The demo school's public page: an introduction, school details, the head
// teacher's message and photo, a small gallery and one public notice, so a
// fresh preview looks like a school's real front page. It runs only while the
// school is the demo school (the seed's own name), and it fills a field only
// while that field is empty: anything an administrator has written stays.
// The photos are drawn here, so no real person or child appears in them.

const demoSchoolName = "Green Valley Demo School"

// isDemoSchool reports whether the school still carries the demo's name. A
// real school, or a demo school an administrator has renamed, gets no public
// content from the seed.
func (s *seeder) isDemoSchool() (bool, error) {
	var demo bool
	err := s.db.WithContext(s.ctx).Raw("SELECT EXISTS (SELECT 1 FROM school_settings WHERE id = 1 AND name_en = ?)", demoSchoolName).Row().Scan(&demo)
	return demo, err
}

var galleryScenes = []struct {
	ref, en, bn string
	draw        func() *image.RGBA
}{
	{"campus", "Our school building", "আমাদের বিদ্যালয় ভবন", drawCampus},
	{"library", "The library", "পাঠাগার", drawLibrary},
	{"science", "Science fair projects", "বিজ্ঞান মেলার প্রকল্প", drawScience},
	{"field", "The playing field", "খেলার মাঠ", drawField},
}

func (s *seeder) publicPage() error {
	demo, err := s.isDemoSchool()
	if err != nil || !demo {
		return err
	}
	db := s.db.WithContext(s.ctx)
	// School details live in the settings; each is set only while empty.
	if err := db.Exec(`UPDATE school_settings SET
    eiin = CASE WHEN eiin = '' THEN '123456' ELSE eiin END,
    founded_year = COALESCE(founded_year, 1987),
    address = CASE WHEN address = '' THEN '12 College Road, Sylhet 3100' ELSE address END,
    address_bn = CASE WHEN address_bn = '' THEN '১২ কলেজ রোড, সিলেট ৩১০০' ELSE address_bn END,
    phone = CASE WHEN phone = '' THEN '+880 1700-000000' ELSE phone END,
    email = CASE WHEN email = '' THEN ? ELSE email END,
    head_teacher_name_en = CASE WHEN head_teacher_name_en = '' THEN 'Shahana Begum' ELSE head_teacher_name_en END,
    head_teacher_name_bn = CASE WHEN head_teacher_name_bn = '' THEN 'শাহানা বেগম' ELSE head_teacher_name_bn END,
    updated_at = now()
WHERE id = 1 AND name_en = ? AND (eiin = '' OR founded_year IS NULL OR address = '' OR address_bn = '' OR phone = '' OR email = '' OR head_teacher_name_en = '' OR head_teacher_name_bn = '')`, "office@"+Domain, demoSchoolName).Error; err != nil {
		return err
	}
	// The introduction (with the totals) and the head teacher's message.
	if err := db.Exec(`UPDATE school_public_page SET about_en = ?, about_bn = ?, show_counts = true, updated_by = ?, updated_at = now()
WHERE id = 1 AND about_en = '' AND about_bn = ''`, aboutEN, aboutBN, s.admin).Error; err != nil {
		return err
	}
	if err := db.Exec(`UPDATE school_public_page SET head_message_en = ?, head_message_bn = ?, updated_by = ?, updated_at = now()
WHERE id = 1 AND head_message_en = '' AND head_message_bn = ''`, headMessageEN, headMessageBN, s.admin).Error; err != nil {
		return err
	}
	service := school.NewService(school.NewRepository(s.db))
	var photo string
	if err := db.Raw("SELECT head_photo_version FROM school_public_page WHERE id = 1").Row().Scan(&photo); err != nil {
		return err
	}
	if photo == "" {
		data, err := pngBytes(drawPortrait())
		if err != nil {
			return err
		}
		if _, err := service.SetHeadPhoto(s.ctx, s.admin, "image/png", data); err != nil {
			return fmt.Errorf("head teacher photo: %w", err)
		}
		s.report.Created["head_photo"]++
	}
	for _, scene := range galleryScenes {
		if _, err := s.ensure("gallery", scene.ref, func(tx *gorm.DB) (string, error) {
			data, err := pngBytes(scene.draw())
			if err != nil {
				return "", err
			}
			svc := school.NewService(school.NewRepository(tx))
			added, err := svc.AddGalleryPhoto(s.ctx, s.admin, "image/png", data)
			if err != nil {
				return "", err
			}
			_, err = svc.UpdateGalleryCaption(s.ctx, s.admin, added.ID, school.SlideCaption{CaptionEN: scene.en, CaptionBN: scene.bn})
			return added.ID, err
		}); err != nil {
			return err
		}
	}
	_, err = s.ensure("notice", "public-admissions", func(tx *gorm.DB) (string, error) {
		return scan(tx, `INSERT INTO notices (title_en, title_bn, body_en, body_bn, priority, audience_roles, status, publish_at, show_public, created_by, updated_by)
VALUES (?, ?, ?, ?, 'normal', '{head_teacher,teacher,student,guardian,accountant}', 'published', now(), true, ?, ?) RETURNING id`,
			"Admissions open for the new session", "নতুন শিক্ষাবর্ষে ভর্তি চলছে",
			"Admission forms for Class Six to Class Eight are available at the school office from Sunday to Thursday, 9 am to 1 pm. Please bring the birth certificate and two photographs.",
			"ষষ্ঠ থেকে অষ্টম শ্রেণির ভর্তি ফরম রবিবার থেকে বৃহস্পতিবার সকাল ৯টা থেকে দুপুর ১টা পর্যন্ত বিদ্যালয়ের অফিসে পাওয়া যাবে। জন্মনিবন্ধন সনদ ও দুই কপি ছবি সঙ্গে আনুন।",
			s.admin, s.admin)
	})
	return err
}

const aboutEN = `Green Valley Demo School is a co-educational secondary school for Classes Six to Ten. Small classes, caring teachers and a calm campus help every student learn with confidence.

Besides the national curriculum, students take part in debate, science fairs, sports and cultural programmes throughout the year.`

const aboutBN = `গ্রিন ভ্যালি ডেমো স্কুল ষষ্ঠ থেকে দশম শ্রেণির একটি সহশিক্ষা মাধ্যমিক বিদ্যালয়। ছোট শ্রেণিকক্ষ, যত্নশীল শিক্ষক ও শান্ত পরিবেশ প্রত্যেক শিক্ষার্থীকে আত্মবিশ্বাসের সঙ্গে শিখতে সাহায্য করে।

জাতীয় শিক্ষাক্রমের পাশাপাশি সারা বছর বিতর্ক, বিজ্ঞান মেলা, খেলাধুলা ও সাংস্কৃতিক অনুষ্ঠানে শিক্ষার্থীরা অংশ নেয়।`

const headMessageEN = `Welcome to Green Valley. Our aim is simple: every child should feel known, safe and able to do well. We work closely with families, so please visit us, talk to the class teachers and follow your child's progress through the year.`

const headMessageBN = `গ্রিন ভ্যালিতে আপনাকে স্বাগতম। আমাদের লক্ষ্য সহজ: প্রত্যেক শিশু যেন নিজেকে পরিচিত, নিরাপদ ও সফল হওয়ার যোগ্য মনে করে। আমরা পরিবারের সঙ্গে মিলে কাজ করি; বিদ্যালয়ে আসুন, শ্রেণি শিক্ষকের সঙ্গে কথা বলুন এবং সারা বছর সন্তানের অগ্রগতি দেখুন।`

// ---- drawing ----

func pngBytes(img image.Image) ([]byte, error) {
	var out bytes.Buffer
	err := png.Encode(&out, img)
	return out.Bytes(), err
}

func rgb(hex uint32) color.RGBA {
	return color.RGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 255}
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

func rect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	b := img.Bounds()
	for y := max(y0, b.Min.Y); y < min(y1, b.Max.Y); y++ {
		for x := max(x0, b.Min.X); x < min(x1, b.Max.X); x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// gradient fills rows y0..y1 from top to bottom colour.
func gradient(img *image.RGBA, y0, y1 int, top, bottom color.RGBA) {
	for y := y0; y < y1; y++ {
		rect(img, 0, y, img.Bounds().Dx(), y+1, mix(top, bottom, float64(y-y0)/float64(max(1, y1-y0-1))))
	}
}

func ellipse(img *image.RGBA, cx, cy, rx, ry float64, c color.RGBA) {
	for y := int(cy - ry); y <= int(cy+ry); y++ {
		for x := int(cx - rx); x <= int(cx+rx); x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			if dx*dx+dy*dy <= 1 && image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// polygon fills a convex or simple polygon by scanlines.
func polygon(img *image.RGBA, c color.RGBA, pts ...[2]float64) {
	minY, maxY := pts[0][1], pts[0][1]
	for _, p := range pts {
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	for y := int(minY); y <= int(maxY); y++ {
		fy := float64(y) + 0.5
		xs := []float64{}
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			if (a[1] <= fy && b[1] > fy) || (b[1] <= fy && a[1] > fy) {
				xs = append(xs, a[0]+(fy-a[1])/(b[1]-a[1])*(b[0]-a[0]))
			}
		}
		for i := 0; i+1 < len(xs); i += 2 {
			lo, hi := math.Min(xs[i], xs[i+1]), math.Max(xs[i], xs[i+1])
			rect(img, int(lo), y, int(hi)+1, y+1, c)
		}
	}
}

func tree(img *image.RGBA, x, ground, size float64) {
	rect(img, int(x-size*0.08), int(ground-size*0.9), int(x+size*0.08), int(ground), rgb(0x6b4a2b))
	ellipse(img, x, ground-size*1.15, size*0.55, size*0.5, rgb(0x2f7d32))
	ellipse(img, x-size*0.25, ground-size*0.95, size*0.4, size*0.35, rgb(0x388e3c))
	ellipse(img, x+size*0.28, ground-size*1.0, size*0.38, size*0.34, rgb(0x2e7031))
}

const sceneW, sceneH = 1200, 800

func drawCampus() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sceneW, sceneH))
	gradient(img, 0, 560, rgb(0x7cb8e8), rgb(0xd8ecf7))
	gradient(img, 560, sceneH, rgb(0x6aa84f), rgb(0x4e8a3a))
	ellipse(img, 1010, 120, 55, 55, rgb(0xfff3c4))
	// The building: two floors, a roof band and a central entrance block.
	rect(img, 170, 250, 1030, 590, rgb(0xf1e2c4))
	rect(img, 150, 228, 1050, 256, rgb(0xa0472f))
	rect(img, 500, 190, 700, 590, rgb(0xe8d3ad))
	polygon(img, rgb(0x8f3d27), [2]float64{480, 196}, [2]float64{600, 120}, [2]float64{720, 196})
	rect(img, 170, 410, 1030, 422, rgb(0xd9c39c))
	for floor := 0; floor < 2; floor++ {
		y := 290 + floor*150
		for _, x := range []int{205, 300, 395, 745, 840, 935} {
			rect(img, x, y, x+62, y+80, rgb(0x5d7d9a))
			rect(img, x+29, y, x+33, y+80, rgb(0xf1e2c4))
			rect(img, x, y+38, x+62, y+42, rgb(0xf1e2c4))
		}
	}
	rect(img, 555, 470, 645, 590, rgb(0x6b4a2b))
	rect(img, 598, 470, 602, 590, rgb(0x4f3520))
	rect(img, 540, 250, 660, 300, rgb(0x1d4ed8))
	rect(img, 548, 258, 652, 292, rgb(0xf8fafc))
	// Flag.
	rect(img, 300, 560, 820, 600, rgb(0xcfc6b4))
	rect(img, 116, 300, 122, 600, rgb(0x9ca3af))
	rect(img, 122, 300, 212, 354, rgb(0x006a4e))
	ellipse(img, 160, 327, 17, 17, rgb(0xf42a41))
	for i, x := range []float64{60, 1110, 1170} {
		tree(img, x, 610+float64(i)*8, 120)
	}
	return img
}

func drawLibrary() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sceneW, sceneH))
	gradient(img, 0, 600, rgb(0xf3e7d3), rgb(0xe9d8bc))
	rect(img, 0, 600, sceneW, sceneH, rgb(0xa47148))
	for x := 0; x < sceneW; x += 120 {
		rect(img, x, 600, x+3, sceneH, rgb(0x8c5e3a))
	}
	books := []color.RGBA{rgb(0xb91c1c), rgb(0x1d4ed8), rgb(0x15803d), rgb(0xca8a04), rgb(0x7e22ce), rgb(0x0f766e), rgb(0xc2410c), rgb(0x334155)}
	seed := uint32(7)
	next := func(n int) int { seed = seed*1103515245 + 12345; return int(seed>>16) % n }
	for _, shelf := range [][2]int{{60, 560}, {640, 1140}} {
		rect(img, shelf[0]-16, 90, shelf[1]+16, 560, rgb(0x7a4b2a))
		for row := 0; row < 4; row++ {
			base := 200 + row*112
			rect(img, shelf[0], base-100, shelf[1], base, rgb(0x5a361d))
			for x := shelf[0] + 6; x < shelf[1]-30; {
				w, h := 18+next(16), 60+next(34)
				rect(img, x, base-h, x+w, base, books[next(len(books))])
				rect(img, x+3, base-h+10, x+w-3, base-h+14, rgb(0xf5e6c8))
				x += w + 2
			}
			rect(img, shelf[0]-16, base, shelf[1]+16, base+12, rgb(0x7a4b2a))
		}
	}
	// A reading table with an open book and a lamp.
	rect(img, 300, 610, 900, 640, rgb(0x6b4226))
	rect(img, 330, 640, 352, 760, rgb(0x5a361d))
	rect(img, 848, 640, 870, 760, rgb(0x5a361d))
	polygon(img, rgb(0xfafaf9), [2]float64{520, 608}, [2]float64{600, 596}, [2]float64{600, 610}, [2]float64{520, 612})
	polygon(img, rgb(0xf1f5f9), [2]float64{600, 596}, [2]float64{680, 608}, [2]float64{680, 612}, [2]float64{600, 610})
	rect(img, 780, 520, 788, 610, rgb(0x334155))
	polygon(img, rgb(0x15803d), [2]float64{752, 520}, [2]float64{816, 520}, [2]float64{800, 490}, [2]float64{768, 490})
	return img
}

func drawScience() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sceneW, sceneH))
	gradient(img, 0, 560, rgb(0xdff1ee), rgb(0xcbe7e2))
	rect(img, 0, 560, sceneW, sceneH, rgb(0x94a3b8))
	// Blackboard with chalk diagrams.
	rect(img, 180, 70, 1020, 360, rgb(0x6b4a2b))
	rect(img, 196, 86, 1004, 344, rgb(0x1f4d3a))
	chalk := rgb(0xe7efe9)
	ellipse(img, 340, 210, 70, 70, chalk)
	ellipse(img, 340, 210, 64, 64, rgb(0x1f4d3a))
	ellipse(img, 340, 210, 16, 16, chalk)
	ellipse(img, 410, 170, 10, 10, chalk)
	for i := 0; i < 6; i++ {
		rect(img, 520, 140+i*30, 520+180-i*18, 144+i*30, chalk)
	}
	polygon(img, chalk, [2]float64{800, 290}, [2]float64{880, 140}, [2]float64{960, 290})
	polygon(img, rgb(0x1f4d3a), [2]float64{812, 284}, [2]float64{880, 156}, [2]float64{948, 284})
	// Project tables with flasks and a small model.
	for _, tb := range [][2]int{{80, 560}, {640, 1120}} {
		rect(img, tb[0], 520, tb[1], 548, rgb(0xf8fafc))
		rect(img, tb[0], 548, tb[1], 640, rgb(0x1d4ed8))
		rect(img, tb[0]+10, 548, tb[1]-10, 556, rgb(0x1e40af))
	}
	liquids := []color.RGBA{rgb(0x16a34a), rgb(0xdc2626), rgb(0x2563eb), rgb(0xf59e0b)}
	for i, x := range []float64{170, 290, 410} {
		polygon(img, rgb(0xe2e8f0), [2]float64{x - 14, 420}, [2]float64{x + 14, 420}, [2]float64{x + 14, 450}, [2]float64{x + 44, 520}, [2]float64{x - 44, 520}, [2]float64{x - 14, 450})
		polygon(img, liquids[i], [2]float64{x - 28, 482}, [2]float64{x + 28, 482}, [2]float64{x + 44, 520}, [2]float64{x - 44, 520})
	}
	// A wind turbine model.
	rect(img, 876, 430, 884, 520, rgb(0x475569))
	rect(img, 840, 506, 920, 520, rgb(0x334155))
	for _, a := range []float64{-1.571, 0.524, 2.618} {
		polygon(img, rgb(0x64748b), [2]float64{880, 430}, [2]float64{880 + 62*math.Cos(a) - 7*math.Sin(a), 430 + 62*math.Sin(a) + 7*math.Cos(a)}, [2]float64{880 + 62*math.Cos(a), 430 + 62*math.Sin(a)})
	}
	ellipse(img, 880, 430, 9, 9, rgb(0x334155))
	rect(img, 980, 470, 1060, 520, rgb(0xfde68a))
	rect(img, 990, 480, 1050, 488, rgb(0x92400e))
	rect(img, 990, 496, 1036, 502, rgb(0x92400e))
	return img
}

func drawField() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sceneW, sceneH))
	gradient(img, 0, 330, rgb(0x5aa6e0), rgb(0xcfe7f7))
	ellipse(img, 240, 110, 90, 30, rgb(0xf8fafc))
	ellipse(img, 300, 95, 70, 34, rgb(0xf8fafc))
	ellipse(img, 880, 150, 110, 28, rgb(0xf1f5f9))
	for i := 0; i < 14; i++ {
		tree(img, float64(30+i*90), 345, 70+float64((i*37)%30))
	}
	for y := 330; y < sceneH; y += 40 {
		c := rgb(0x4f9a3a)
		if (y/40)%2 == 0 {
			c = rgb(0x58a843)
		}
		rect(img, 0, y, sceneW, y+40, c)
	}
	white := rgb(0xf8fafc)
	polygon(img, white, [2]float64{100, 760}, [2]float64{1100, 760}, [2]float64{1060, 420}, [2]float64{140, 420})
	polygon(img, rgb(0x4f9a3a), [2]float64{108, 754}, [2]float64{1092, 754}, [2]float64{1054, 426}, [2]float64{146, 426})
	rect(img, 597, 420, 603, 760, white)
	ellipse(img, 600, 590, 92, 60, white)
	ellipse(img, 600, 590, 86, 55, rgb(0x58a843))
	// A goal at each end, outside the touchline.
	for _, x := range []int{62, 1084} {
		rect(img, x, 540, x+54, 546, white)
		rect(img, x, 540, x+6, 640, white)
		rect(img, x+48, 540, x+54, 640, white)
	}
	ellipse(img, 690, 640, 14, 14, white)
	ellipse(img, 690, 640, 6, 6, rgb(0x1f2937))
	return img
}

// drawPortrait is an illustration, not a photograph of anyone.
func drawPortrait() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 600, 600))
	gradient(img, 0, 600, rgb(0xdbeafe), rgb(0xbfdbfe))
	ellipse(img, 300, 640, 230, 210, rgb(0x0f766e))
	polygon(img, rgb(0x115e59), [2]float64{170, 470}, [2]float64{300, 600}, [2]float64{230, 600}, [2]float64{120, 520})
	rect(img, 268, 330, 332, 440, rgb(0xc68a5e))
	ellipse(img, 300, 250, 120, 135, rgb(0x2b2118))
	ellipse(img, 300, 265, 95, 115, rgb(0xd29a6c))
	ellipse(img, 300, 175, 104, 52, rgb(0x2b2118))
	ellipse(img, 262, 262, 9, 7, rgb(0x2b2118))
	ellipse(img, 338, 262, 9, 7, rgb(0x2b2118))
	rect(img, 278, 322, 322, 327, rgb(0x9a4a3a))
	polygon(img, rgb(0xc68a5e), [2]float64{252, 430}, [2]float64{348, 430}, [2]float64{300, 490})
	return img
}
