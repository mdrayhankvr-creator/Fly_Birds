package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// Overlapping sessions must not hide the active one: attendance pages choose
// their default session from this list.
func TestPostgresAttendanceSessionsListActiveFirst(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('teacher','attendance:teachers_read_self') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:6])
	year := 5000 + int(binary.BigEndian.Uint16(random[6:])%4000)
	password := suffix + "-SessionSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "attendance-sessions-"+suffix+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	var oldEnabled bool
	if err = db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	var previousActive []string
	if err = db.Table("academic_sessions").Where("is_active").Pluck("id", &previousActive).Error; err != nil {
		t.Fatal(err)
	}
	var activeID, inactiveID string
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM academic_sessions WHERE id IN ?", []string{activeID, inactiveID}).Error
		if len(previousActive) > 0 {
			_ = db.Table("academic_sessions").Where("id IN ?", previousActive).Update("is_active", true).Error
		}
		_ = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
		_ = db.Exec("DELETE FROM users WHERE id=?", teacher.ID).Error
	})
	if err = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Table("academic_sessions").Where("is_active").Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	// The inactive session starts later but also contains the active session's later dates.
	if err = db.Raw("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES (?,'2026-01-01','2026-12-31',true) RETURNING id", fmt.Sprintf("%04d", year)).Scan(&activeID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,'2026-07-01','2027-06-30') RETURNING id", fmt.Sprintf("%04d-%04d", year, year+1)).Scan(&inactiveID).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithBatchServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, attendance.NewService(attendance.NewRepository(db)))
	login, err := authService.Login(ctx, teacher.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	r := perform(router, "GET", "/attendance/sessions?page=1&page_size=100", "", nil, map[string]string{"Authorization": "Bearer " + login.AccessToken})
	if r.Code != 200 {
		t.Fatalf("sessions got %d: %s", r.Code, r.Body.String())
	}
	var body struct {
		Data attendance.Page[attendance.Session] `json:"data"`
	}
	if err = json.Unmarshal(r.Body.Bytes(), &body); err != nil || len(body.Data.Items) < 2 {
		t.Fatalf("sessions %+v %v", body, err)
	}
	if first := body.Data.Items[0]; first.ID != activeID || !first.IsActive {
		t.Fatalf("active session is not listed first: %+v", body.Data.Items)
	}
	for _, item := range body.Data.Items {
		if item.ID == inactiveID && item.IsActive {
			t.Fatalf("inactive session reported active: %+v", item)
		}
	}
}
