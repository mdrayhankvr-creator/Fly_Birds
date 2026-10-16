package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academicsessions"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresAcademicSessionsHTTP(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; academic-session PostgreSQL integration test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal("migration was not idempotent:", err)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(nonce[:])
	password := suffix + "-SessionSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "session-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := authService.BootstrapUser(ctx, "session-reader-"+suffix+"@example.test", password, "head_teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id IN (?,?)", admin.ID, reader.ID).Error })
	var oldEnabled bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=1 AND module_key='academics'").Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("school_modules").Where("school_id=1 AND module_key='academics'").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=1 AND module_key='academics'").Update("enabled", oldEnabled).Error
	})
	sessionService := academicsessions.NewService(academicsessions.NewRepository(db))
	router := NewRouterWithAll(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, sessionService)
	adminCreds, err := authService.Login(ctx, admin.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	readerCreds, err := authService.Login(ctx, reader.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	adminHeader := map[string]string{"Authorization": "Bearer " + adminCreds.AccessToken}
	readerHeader := map[string]string{"Authorization": "Bearer " + readerCreds.AccessToken}
	check := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	check("GET", "/academic/sessions", "", nil, 401)
	check("POST", "/academic/sessions", `{"name":"2026","start_date":"2026-01-01","end_date":"2026-12-31"}`, readerHeader, 403)
	check("POST", "/academic/sessions", `{"name":"2026","start_date":"2026-02-30","end_date":"2026-12-31"}`, adminHeader, 400)
	check("POST", "/academic/sessions", `{"name":"2026","start_date":"2026-12-31","end_date":"2026-01-01"}`, adminHeader, 400)
	// Choose a distinct pair of valid years for concurrent test runs.
	year := 2050 + int(nonce[0])%100
	names := []string{fmt.Sprint(year), fmt.Sprintf("%d–%d", year, year+1)}
	created := make([]academicsessions.Session, 2)
	t.Cleanup(func() {
		for _, item := range created {
			if item.ID != "" {
				_ = db.Exec("DELETE FROM audit_events WHERE target_type='academic_session' AND target_id=?", item.ID).Error
				_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", item.ID).Error
			}
		}
	})
	for i, name := range names {
		input := academicsessions.Input{Name: name, StartDate: fmt.Sprintf("%d-01-01", year), EndDate: fmt.Sprintf("%d-12-31", year)}
		if i == 1 {
			input.StartDate = fmt.Sprintf("%d-07-01", year)
			input.EndDate = fmt.Sprintf("%d-06-30", year+1)
		}
		body, _ := json.Marshal(input)
		response := check("POST", "/academic/sessions", string(body), adminHeader, 201)
		var decoded struct {
			Data academicsessions.Session `json:"data"`
		}
		if err := json.Unmarshal(response, &decoded); err != nil {
			t.Fatal(err)
		}
		created[i] = decoded.Data
		if created[i].ID == "" || created[i].IsActive {
			t.Fatal("create must return an inactive UUID session")
		}
	}
	var generatedKey string
	if err := db.Table("academic_sessions").Select("name_key").Where("id=?", created[1].ID).Scan(&generatedKey).Error; err != nil || generatedKey != fmt.Sprintf("%d-%d", year, year+1) {
		t.Fatalf("generated normalized key=%q error=%v", generatedKey, err)
	}
	if err := db.Exec("INSERT INTO academic_sessions (name,start_date,end_date) VALUES (?,?,?)", fmt.Sprint(year+2), fmt.Sprintf("%d-12-31", year+2), fmt.Sprintf("%d-01-01", year+2)).Error; err == nil {
		t.Fatal("PostgreSQL accepted an end date before the start date")
	}
	duplicate, _ := json.Marshal(academicsessions.Input{Name: fmt.Sprintf("%d-%d", year, year+1), StartDate: fmt.Sprintf("%d-01-01", year), EndDate: fmt.Sprintf("%d-12-31", year+1)})
	check("POST", "/academic/sessions", string(duplicate), adminHeader, 409)
	check("PATCH", "/academic/sessions/"+created[0].ID, string(duplicate), adminHeader, 409)
	check("PATCH", "/academic/sessions/"+created[0].ID, `{"name":"2026","start_date":"2026-12-31","end_date":"2026-01-01"}`, adminHeader, 400)
	check("GET", "/academic/sessions/"+created[0].ID, "", readerHeader, 200)
	var pageResult struct {
		Data academicsessions.Page `json:"data"`
	}
	search := fmt.Sprintf("/academic/sessions?search=%d&page=1&page_size=1", year+1)
	if err := json.Unmarshal(check("GET", search, "", readerHeader, 200), &pageResult); err != nil {
		t.Fatal(err)
	}
	if pageResult.Data.Total != 1 || len(pageResult.Data.Items) != 1 || pageResult.Data.Items[0].ID != created[1].ID {
		t.Fatalf("search result: %#v", pageResult.Data)
	}
	if err := json.Unmarshal(check("GET", "/academic/sessions?page=2&page_size=1", "", readerHeader, 200), &pageResult); err != nil {
		t.Fatal(err)
	}
	if pageResult.Data.Total != 2 || len(pageResult.Data.Items) != 1 || pageResult.Data.Items[0].ID != created[0].ID {
		t.Fatalf("pagination result: %#v", pageResult.Data)
	}
	check("GET", "/academic/sessions?page=0", "", readerHeader, 400)
	check("GET", "/academic/sessions/current", "", readerHeader, 404)
	if err := db.Table("school_modules").Where("school_id=1 AND module_key='academics'").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/academic/sessions", "", readerHeader, 403)
	check("POST", "/academic/sessions", string(duplicate), adminHeader, 403)
	if err := db.Table("school_modules").Where("school_id=1 AND module_key='academics'").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, item := range created {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			codes <- perform(router, "PATCH", "/academic/sessions/"+id+"/activation", `{"is_active":true}`, nil, adminHeader).Code
		}(item.ID)
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent activation status %d", code)
		}
	}
	var activeCount int64
	if err := db.Table("academic_sessions").Where("is_active=true").Count(&activeCount).Error; err != nil || activeCount != 1 {
		t.Fatalf("active sessions=%d err=%v", activeCount, err)
	}
	var current struct {
		Data academicsessions.Session `json:"data"`
	}
	if err := json.Unmarshal(check("GET", "/academic/sessions/current", "", readerHeader, 200), &current); err != nil {
		t.Fatal(err)
	}
	otherID := created[0].ID
	if current.Data.ID == otherID {
		otherID = created[1].ID
	}
	if err := db.Table("academic_sessions").Where("id=?", otherID).Update("is_active", true).Error; err == nil {
		t.Fatal("PostgreSQL accepted a second active session")
	}
	check("PATCH", "/academic/sessions/"+current.Data.ID+"/activation", `{"is_active":false}`, adminHeader, 200)
	check("GET", "/academic/sessions/current", "", readerHeader, 404)
	update := fmt.Sprintf(`{"name":"%d","start_date":"%d-02-01","end_date":"%d-12-31"}`, year, year, year)
	check("PATCH", "/academic/sessions/"+created[0].ID, update, adminHeader, 200)
}
