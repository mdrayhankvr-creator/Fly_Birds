package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresM10AdminControls(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	for range 2 {
		if err := database.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	var applied int64
	if db.Table("schema_migrations").Where("version = 1001").Count(&applied); applied != 1 {
		t.Fatal("M10 migration 1001 not applied")
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x7a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "M10TestPassword!" + suffix
	userIDs := []string{}
	guardianIDs := []string{}
	modules := map[string]bool{}
	var moduleRows []struct {
		ModuleKey string
		Enabled   bool
	}
	db.Raw("SELECT module_key, enabled FROM school_modules WHERE school_id = 1").Scan(&moduleRows)
	for _, m := range moduleRows {
		modules[m.ModuleKey] = m.Enabled
	}
	var teacherGrants []string
	db.Raw("SELECT permission_id FROM role_permissions WHERE role_id = 'teacher' ORDER BY permission_id").Scan(&teacherGrants)
	backupDir, uploadsDir := t.TempDir(), t.TempDir()
	t.Cleanup(func() {
		for key, on := range modules {
			_ = db.Exec("UPDATE school_modules SET enabled = ? WHERE school_id = 1 AND module_key = ?", on, key).Error
		}
		_ = db.Exec("DELETE FROM role_permissions WHERE role_id = 'teacher'").Error
		for _, p := range teacherGrants {
			_ = db.Exec("INSERT INTO role_permissions(role_id, permission_id) VALUES ('teacher', ?)", p).Error
		}
		_ = db.Exec("DELETE FROM backup_runs WHERE started_by IN ? OR started_by IS NULL", userIDs).Error
		if len(guardianIDs) > 0 {
			_ = db.Exec("DELETE FROM guardians WHERE id IN ?", guardianIDs).Error
		}
		if len(userIDs) > 0 {
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN ?", userIDs).Error
			_ = db.Exec("DELETE FROM users WHERE id IN ?", userIDs).Error
		}
	})
	type actor struct {
		user    auth.User
		headers map[string]string
	}
	login := func(role string) actor {
		u, err := authService.BootstrapUser(ctx, fmt.Sprintf("m10-%s-%d-%s@example.test", role, len(userIDs), suffix), password, role)
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		c, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		return actor{u, map[string]string{"Authorization": "Bearer " + c.AccessToken}}
	}
	adm, head, accountant, teacher := login("admin"), login("head_teacher"), login("accountant"), login("teacher")

	backups, err := admin.NewBackups(db, backupDir, uploadsDir, 10)
	if err != nil {
		t.Fatal(err)
	}
	services := &AdminServices{Exporter: admin.NewExporter(db), Backups: backups, Overview: admin.NewOverviewReader(db, backups)}
	auditService := audit.NewService(audit.NewRepository(db))
	router := NewRouterWithAdmin(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, auditService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, services)
	raw := func(method, path, body string, a actor, want int) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, a.headers)
		if r.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		return r.Body.Bytes()
	}
	call := func(method, path, body string, a actor, want int) map[string]any {
		t.Helper()
		var out map[string]any
		_ = json.Unmarshal(raw(method, path, body, a, want), &out)
		return out
	}
	data := func(m map[string]any) map[string]any { return m["data"].(map[string]any) }
	items := func(m map[string]any) []any { return data(m)["items"].([]any) }
	errorCode := func(m map[string]any) string { return m["error"].(map[string]any)["code"].(string) }

	// Prohibited operations: every control-center route refuses non-admin
	// roles, including ones that hold some management permissions.
	adminRoutes := [][2]string{
		{"GET", "/admin/overview"}, {"GET", "/admin/exports"}, {"GET", "/admin/exports/users"},
		{"GET", "/admin/backups"}, {"POST", "/admin/backups"}, {"GET", "/permissions/matrix"},
		{"GET", "/roles/teacher/permissions/history"}, {"GET", "/audit/events"}, {"GET", "/audit/actions"},
		{"PUT", "/school/modules/diary"},
	}
	for _, who := range []actor{head, accountant, teacher} {
		for _, route := range adminRoutes {
			body := ""
			if route[0] == "PUT" {
				body = `{"enabled":false}`
			}
			raw(route[0], route[1], body, who, 403)
		}
	}
	for _, route := range adminRoutes[:5] {
		raw(route[0], route[1], "", actor{headers: map[string]string{}}, 401)
	}
	var runs int64
	db.Table("backup_runs").Where("started_by IN ?", userIDs).Count(&runs)
	if runs != 0 {
		t.Fatal("a refused backup request created a run")
	}
	// Refusals by signed-in users are audited with the route but no body.
	denied := call("GET", "/audit/events?outcome=denied&actor="+accountant.user.ID+"&page_size=100", "", adm, 200)
	if got := len(items(denied)); got != len(adminRoutes) {
		t.Fatalf("accountant denied events = %d, want %d", got, len(adminRoutes))
	}
	first := items(denied)[0].(map[string]any)
	if first["action"] != "admin.access_denied" || first["actor_email"] != accountant.user.Email || first["actor_role"] != "accountant" {
		t.Fatalf("denied event shape: %v", first)
	}
	if !strings.HasPrefix(first["target_id"].(string), "PUT /school/modules") && !strings.Contains(first["target_id"].(string), "/") {
		t.Fatalf("denied target: %v", first["target_id"])
	}

	// Audit viewer filters and validation.
	for _, bad := range []string{"outcome=maybe", "from=2026-02-30", "from=2026-05-02&to=2026-05-01", "search=" + strings.Repeat("x", 101), "actor=nope"} {
		raw("GET", "/audit/events?"+bad, "", adm, 400)
	}
	today := time.Now().In(time.FixedZone("Asia/Dhaka", 6*3600)).Format(time.DateOnly)
	searched := call("GET", "/audit/events?outcome=denied&search="+url.QueryEscape(strings.ToUpper(head.user.Email))+"&from="+today+"&to="+today+"&page_size=100", "", adm, 200)
	if len(items(searched)) != len(adminRoutes) {
		t.Fatalf("search by actor email found %d", len(items(searched)))
	}
	raw("GET", "/audit/events?search=%25%25%25&outcome=denied&actor="+teacher.user.ID, "", adm, 200)
	if escaped := call("GET", "/audit/events?search=%25&actor="+teacher.user.ID, "", adm, 200); len(items(escaped)) != 0 {
		t.Fatal("search wildcard was not escaped")
	}

	// Overview reflects the database.
	overview := data(call("GET", "/admin/overview", "", adm, 200))
	if overview["schema_version"].(float64) < 1001 || overview["denied_last_day"].(float64) < float64(3*len(adminRoutes)) || overview["modules_total"].(float64) < 10 {
		t.Fatalf("overview: %v", overview)
	}

	// Exports: CSV with BOM, Bangla intact, formulas neutralised, no secrets.
	insertGuardian := func(name, nameBN, phone string) {
		var id string
		if err := db.Raw("INSERT INTO guardians(name_en, name_bn, phone) VALUES (?, ?, ?) RETURNING id", name, nameBN, phone).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		guardianIDs = append(guardianIDs, id)
	}
	insertGuardian("=HYPERLINK(\"http://evil\")"+suffix, "অভিভাবক "+suffix, "+8801711000000")
	list := data(call("GET", "/admin/exports", "", adm, 200))
	for _, d := range list["items"].([]any) {
		row := d.(map[string]any)
		if row["ranged"] == true && row["rows"].(float64) != -1 {
			t.Fatalf("ranged dataset counted without range: %v", row)
		}
	}
	r := perform(router, "GET", "/admin/exports/guardians", "", nil, adm.headers)
	if r.Code != 200 || r.Header().Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(r.Header().Get("Content-Disposition"), "attachment") || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("export response %d %v", r.Code, r.Header())
	}
	body := r.Body.String()
	if !strings.HasPrefix(body, "\xEF\xBB\xBF") || !strings.Contains(body, "'=HYPERLINK") || !strings.Contains(body, "অভিভাবক "+suffix) || !strings.Contains(body, "'+8801711000000") {
		t.Fatalf("guardian export content: %.300s", body)
	}
	users := string(raw("GET", "/admin/exports/users", "", adm, 200))
	if strings.Contains(users, "$argon2") || strings.Contains(strings.ToLower(users), "password") || !strings.Contains(users, adm.user.Email) {
		t.Fatal("users export leaks password hashes or misses users")
	}
	raw("GET", "/admin/exports/fee_payments", "", adm, 400)
	raw("GET", "/admin/exports/fee_payments?from=2025-01-01&to=2026-12-31", "", adm, 400)
	raw("GET", "/admin/exports/password_hashes", "", adm, 404)
	auditCSV := string(raw("GET", "/admin/exports/audit_log?from="+today+"&to="+today, "", adm, 200))
	if !strings.Contains(auditCSV, "admin.access_denied") {
		t.Fatal("audit export misses today's denied events")
	}
	var exported int64
	db.Table("audit_events").Where("action = 'data.exported' AND actor_user_id = ?", adm.user.ID).Count(&exported)
	if exported != 3 {
		t.Fatalf("data.exported events = %d, want 3", exported)
	}

	// Backups: create, refuse a concurrent run, verify, download, restore.
	upload := []byte("m10 attachment " + suffix)
	key := strings.Repeat("ab", 16)
	if err := os.MkdirAll(filepath.Join(uploadsDir, "ab"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadsDir, "ab", key), upload, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(uploadsDir, "ab", "partial.tmp"), []byte("x"), 0o600)
	run := data(call("POST", "/admin/backups", "", adm, 201))
	if run["status"] != "completed" || run["file_count"].(float64) != 1 || run["schema_version"].(float64) < 1001 {
		t.Fatalf("backup run: %v", run)
	}
	runID, fileName := run["id"].(string), run["file_name"].(string)
	var busyID string
	if err := db.Raw("INSERT INTO backup_runs(status, trigger, started_by) VALUES ('running', 'manual', ?) RETURNING id", adm.user.ID).Row().Scan(&busyID); err != nil {
		t.Fatal(err)
	}
	if got := call("POST", "/admin/backups", "", adm, 409); errorCode(got) != "backup_running" {
		t.Fatalf("concurrent backup: %v", got)
	}
	_ = db.Exec("DELETE FROM backup_runs WHERE id = ?", busyID).Error
	if v := data(call("POST", "/admin/backups/"+runID+"/verify", "", adm, 200)); v["ok"] != true {
		t.Fatalf("verify: %v", v)
	}
	d := perform(router, "GET", "/admin/backups/"+runID+"/download", "", nil, adm.headers)
	sum := sha256.Sum256(d.Body.Bytes())
	if d.Code != 200 || d.Header().Get("Content-Type") != "application/gzip" || hex.EncodeToString(sum[:]) != run["sha256"] {
		t.Fatalf("download %d %s", d.Code, d.Header())
	}
	archive := filepath.Join(backupDir, fileName)
	manifest, entries, err := admin.ReadArchive(archive, nil)
	if err != nil || manifest.Format != admin.ManifestFormat || entries != len(manifest.Tables)+len(manifest.Uploads) {
		t.Fatalf("archive: %v %d %v", err, entries, manifest)
	}
	for _, tbl := range manifest.Tables {
		if tbl.Name == "auth_sessions" {
			t.Fatal("auth sessions were backed up")
		}
	}
	tampered := filepath.Join(t.TempDir(), fileName)
	original, _ := os.ReadFile(archive)
	broken := bytes.Clone(original)
	broken[len(broken)/2] ^= 0xff
	_ = os.WriteFile(tampered, broken, 0o600)
	if _, _, err := admin.ReadArchive(tampered, nil); err == nil {
		t.Fatal("tampered archive passed verification")
	}
	list2 := data(call("GET", "/admin/backups", "", adm, 200))
	if list2["retention"].(float64) != 10 || len(list2["items"].([]any)) == 0 {
		t.Fatalf("backup list: %v", list2)
	}

	// Restore drill into a separate, freshly migrated database.
	restoreName := "flybirds_m10_restore_" + suffix[len(suffix)-9:]
	if err := db.Exec("CREATE DATABASE " + restoreName).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP DATABASE IF EXISTS " + restoreName + " WITH (FORCE)").Error })
	parsed, _ := url.Parse(dsn)
	parsed.Path = "/" + restoreName
	target, err := database.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	targetSQL, _ := target.DB()
	if err := database.Migrate(ctx, target); err != nil {
		t.Fatal(err)
	}
	restoredUploads := t.TempDir()
	result, err := admin.Restore(ctx, target, archive, restoredUploads, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploads != 1 || result.Tables != len(manifest.Tables)-1 {
		t.Fatalf("restore result %+v", result)
	}
	for _, tbl := range []string{"users", "guardians", "role_permissions", "school_modules", "audit_events", "backup_runs"} {
		var want, got int64
		for _, mt := range manifest.Tables {
			if mt.Name == tbl {
				want = mt.Rows
			}
		}
		target.Table(tbl).Count(&got)
		if got != want || got == 0 && tbl != "backup_runs" {
			t.Fatalf("restored %s rows %d, manifest %d", tbl, got, want)
		}
	}
	var restoredName string
	target.Raw("SELECT name_bn FROM guardians WHERE id = ?", guardianIDs[0]).Scan(&restoredName)
	if restoredName != "অভিভাবক "+suffix {
		t.Fatalf("restored Bangla name %q", restoredName)
	}
	if got, _ := os.ReadFile(filepath.Join(restoredUploads, "ab", key)); !bytes.Equal(got, upload) {
		t.Fatal("restored attachment differs")
	}
	if _, err := admin.Restore(ctx, target, archive, "", false); !errors.Is(err, admin.ErrNotEmpty) {
		t.Fatalf("restore over data without force: %v", err)
	}
	if err := target.Exec("INSERT INTO schema_migrations(version) VALUES (999999)").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Restore(ctx, target, archive, "", true); !errors.Is(err, admin.ErrSchemaMismatch) {
		t.Fatalf("restore across schema versions: %v", err)
	}
	_ = targetSQL.Close()

	// Retention keeps only the newest archives.
	small, err := admin.NewBackups(db, backupDir, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := small.Create(ctx, &adm.user.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	var kept int64
	db.Table("backup_runs").Where("status = 'completed' AND deleted_at IS NULL").Count(&kept)
	if _, err := os.Stat(archive); kept != 1 || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retention kept %d, old archive err %v", kept, err)
	}
	raw("GET", "/admin/backups/"+runID+"/download", "", adm, 404)
	latest := items(call("GET", "/admin/backups", "", adm, 200))[0].(map[string]any)
	raw("DELETE", "/admin/backups/"+latest["id"].(string), "", adm, 200)
	raw("DELETE", "/admin/backups/"+latest["id"].(string), "", adm, 404)
	raw("DELETE", "/admin/backups/not-a-uuid", "", adm, 404)
	for _, action := range []string{"backup.created", "backup.verified", "backup.downloaded", "backup.pruned", "backup.deleted"} {
		var n int64
		db.Table("audit_events").Where("action = ? AND actor_user_id = ?", action, adm.user.ID).Count(&n)
		if n == 0 {
			t.Fatalf("missing audit %s", action)
		}
	}

	// Permission editor: matrix, protected grants, stale saves, history.
	matrix := data(call("GET", "/permissions/matrix", "", adm, 200))
	if len(matrix["roles"].([]any)) != 6 || len(matrix["protected"].(map[string]any)["admin"].([]any)) != 2 {
		t.Fatalf("matrix: %v", matrix["roles"])
	}
	grants := matrix["grants"].(map[string]any)["teacher"].([]any)
	current := []string{}
	for _, g := range grants {
		current = append(current, g.(string))
	}
	marshal := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	next := append(append([]string{}, current...), "data:export")
	stale := call("PUT", "/roles/teacher/permissions", marshal(map[string]any{"ids": next, "expected_ids": current[1:]}), adm, 409)
	if errorCode(stale) != "permissions_changed" {
		t.Fatalf("stale save: %v", stale)
	}
	call("PUT", "/roles/teacher/permissions", marshal(map[string]any{"ids": next, "expected_ids": current}), adm, 200)
	history := items(call("GET", "/roles/teacher/permissions/history?page_size=5", "", adm, 200))
	h0 := history[0].(map[string]any)
	if len(history) == 0 || marshal(h0["added"]) != `["data:export"]` || len(h0["removed"].([]any)) != 0 {
		t.Fatalf("history: %v", history)
	}
	// The teacher now holds data:export but is still not an administrator.
	raw("GET", "/admin/exports/users", "", teacher, 403)
	call("PUT", "/roles/admin/permissions", `{"ids":["auth:read_self"]}`, adm, 409)

	// Module management: dependencies and cascade.
	if err := db.Exec("UPDATE school_modules SET enabled = true WHERE school_id = 1 AND module_key IN ('academics','students','staff','attendance','fees')").Error; err != nil {
		t.Fatal(err)
	}
	blocked := call("PUT", "/school/modules/students", `{"enabled":false}`, adm, 409)
	if errorCode(blocked) != "module_dependencies" || blocked["error"].(map[string]any)["kind"] != "required_by" {
		t.Fatalf("blocked disable: %v", blocked)
	}
	call("PUT", "/school/modules/students", `{"enabled":false,"cascade":true}`, adm, 200)
	enabled := func(key string) bool {
		var on bool
		db.Raw("SELECT enabled FROM school_modules WHERE school_id = 1 AND module_key = ?", key).Scan(&on)
		return on
	}
	if enabled("students") || enabled("attendance") || enabled("fees") || !enabled("staff") {
		t.Fatal("cascade disable did not switch dependents only")
	}
	if needs := call("PUT", "/school/modules/attendance", `{"enabled":true}`, adm, 409); needs["error"].(map[string]any)["kind"] != "requires" {
		t.Fatalf("enable without prerequisites: %v", needs)
	}
	call("PUT", "/school/modules/attendance", `{"enabled":true,"cascade":true}`, adm, 200)
	if !enabled("students") || !enabled("attendance") || enabled("fees") {
		t.Fatal("cascade enable did not switch prerequisites only")
	}
	var via int64
	db.Raw("SELECT count(*) FROM audit_events WHERE actor_user_id = ? AND target_type = 'module' AND metadata->>'via' IS NOT NULL", adm.user.ID).Scan(&via)
	if via < 3 { // attendance and fees switched off with students, students back on with attendance
		t.Fatalf("cascaded module changes audited %d", via)
	}
	actions := call("GET", "/audit/actions", "", adm, 200)
	if !strings.Contains(marshal(actions), "data.exported") {
		t.Fatal("audit actions misses data.exported")
	}
}
