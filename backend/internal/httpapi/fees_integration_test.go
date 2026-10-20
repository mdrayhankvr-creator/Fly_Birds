package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/fees"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresFeeRoutesEnforceAccessAndMoneyRules(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var old bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=1 AND module_key='fees'").Scan(&old).Error; err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x78}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "M8TestPassword!" + suffix
	users := []auth.User{}
	var session, class, section, student, other, guardianRow string
	t.Cleanup(func() {
		ids := []string{}
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		students := []string{}
		for _, id := range []string{student, other} {
			if id != "" {
				students = append(students, id)
			}
		}
		_ = db.Exec("DELETE FROM role_permissions WHERE role_id='teacher' AND permission_id='fees:collect'").Error
		_ = db.Exec("UPDATE school_modules SET enabled=? WHERE school_id=1 AND module_key='fees'", old).Error
		if len(ids) > 0 {
			_ = db.Exec("DELETE FROM ledger_entries WHERE created_by IN ?", ids).Error
			_ = db.Exec("DELETE FROM online_payment_intents WHERE payer_user_id IN ?", ids).Error
		}
		if len(students) > 0 {
			for _, statement := range []string{
				"DELETE FROM fee_payment_allocations WHERE payment_id IN (SELECT id FROM fee_payments WHERE student_id IN ?)",
				"DELETE FROM fee_payments WHERE student_id IN ?",
				"DELETE FROM student_fees WHERE student_id IN ?",
				"DELETE FROM student_guardians WHERE student_id IN ?",
				"DELETE FROM students WHERE id IN ?",
			} {
				if err := db.Exec(statement, students).Error; err != nil {
					t.Errorf("cleanup %s: %v", statement, err)
				}
			}
		}
		if guardianRow != "" {
			_ = db.Exec("DELETE FROM guardians WHERE id=?", guardianRow).Error
		}
		if class != "" {
			_ = db.Exec("DELETE FROM fee_structures WHERE class_id=?", class).Error
		}
		if len(ids) > 0 {
			_ = db.Exec("DELETE FROM fee_categories WHERE created_by IN ?", ids).Error
		}
		if section != "" {
			_ = db.Exec("DELETE FROM academic_sections WHERE id=?", section).Error
			_ = db.Exec("DELETE FROM academic_classes WHERE id=?", class).Error
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", session).Error
		}
		for _, u := range users {
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id=?", u.ID).Error
			_ = db.Exec("DELETE FROM users WHERE id=?", u.ID).Error
		}
	})
	if err := db.Exec("UPDATE school_modules SET enabled=false WHERE school_id=1 AND module_key='fees'").Error; err != nil {
		t.Fatal(err)
	}
	login := func(role string) (auth.User, map[string]string) {
		u, err := authService.BootstrapUser(ctx, fmt.Sprintf("m8-%s-%d-%s@example.test", role, len(users), suffix), password, role)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
		credential, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		return u, map[string]string{"Authorization": "Bearer " + credential.AccessToken}
	}
	_, admin := login("admin")
	_, accountant := login("accountant")
	_, teacher := login("teacher")
	_, headTeacher := login("head_teacher")
	studentUser, studentAuth := login("student")
	guardianUser, guardianAuth := login("guardian")
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := db.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	session = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2182','2026-01-01','2199-12-31') RETURNING id")
	class = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class M8 HTTP "+suffix)
	section = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, class)
	student = insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,user_id) VALUES (?,?,?,?,?,?) RETURNING id", "M8H-1-"+suffix, "Paying Student", session, class, section, studentUser.ID)
	other = insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id) VALUES (?,?,?,?,?) RETURNING id", "M8H-2-"+suffix, "Other Student", session, class, section)
	guardianRow = insert("INSERT INTO guardians(name_en,user_id) VALUES ('Linked Guardian',?) RETURNING id", guardianUser.ID)
	if err := db.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship,is_primary) VALUES (?,?,'father',true)", other, guardianRow).Error; err != nil {
		t.Fatal(err)
	}

	// router is a production installation (no payment simulation); simRouter
	// is a disposable test installation that allows simulation.
	router := NewRouterWithFees(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fees.NewService(fees.NewRepository(db), nil))
	simRouter := NewRouterWithFees(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fees.NewService(fees.NewRepository(db), nil, fees.ServiceOptions{SimulationAllowed: true}))
	defer db.Exec("UPDATE payment_settings SET simulation_enabled=false WHERE id=1")
	active := router
	call := func(method, path, body string, headers map[string]string, want int) map[string]any {
		t.Helper()
		response := perform(active, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		var decoded map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)
		return decoded
	}
	data := func(body map[string]any) map[string]any { return body["data"].(map[string]any) }
	errorCode := func(body map[string]any) string { return body["error"].(map[string]any)["code"].(string) }
	key := func() string {
		var id string
		if err := db.Raw("SELECT gen_random_uuid()").Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	call("GET", "/fees/options", "", nil, 401)
	call("GET", "/fees/options", "", accountant, 403)
	if err := db.Exec("UPDATE school_modules SET enabled=true WHERE school_id=1 AND module_key='fees'").Error; err != nil {
		t.Fatal(err)
	}
	call("GET", "/fees/options", "", accountant, 200)
	for _, headers := range []map[string]string{teacher, headTeacher, studentAuth, guardianAuth} {
		call("GET", "/fees/dues", "", headers, 403)
		call("GET", "/accounts/summary", "", headers, 403)
		call("POST", "/fees/payments", `{}`, headers, 403)
	}
	call("GET", "/fees/me", "", accountant, 403)
	call("GET", "/fees/children/"+other, "", studentAuth, 403)

	code := "H" + suffix[len(suffix)-8:]
	category := data(call("POST", "/fees/categories", fmt.Sprintf(`{"code":%q,"name_en":"Tuition","name_bn":"বেতন","frequency":"monthly"}`, code), accountant, 201))
	call("POST", "/fees/categories", fmt.Sprintf(`{"code":%q,"name_en":"Again","frequency":"monthly"}`, code), accountant, 409)
	call("POST", "/fees/categories", `{"code":"a b","name_en":"Bad","frequency":"monthly"}`, accountant, 400)
	call("POST", "/fees/categories", `{"code":"XX","name_en":"Bad","frequency":"monthly","extra":1}`, accountant, 400)
	response := perform(router, "POST", "/fees/categories", "code=XX", nil, map[string]string{"Authorization": accountant["Authorization"], "Content-Type": "text/plain"})
	if response.Code != 415 {
		t.Fatalf("non-JSON body got %d", response.Code)
	}
	call("POST", "/fees/structures", fmt.Sprintf(`{"session_id":%q,"class_id":%q,"category_id":%q,"amount":1200,"due_day":5}`, session, class, category["id"]), accountant, 201)
	generated := data(call("POST", "/fees/dues/generate", fmt.Sprintf(`{"session_id":%q,"class_id":%q,"period":"2026-02"}`, session, class), accountant, 200))
	if generated["created"].(float64) != 2 {
		t.Fatalf("generated %+v", generated)
	}

	// A mistaken grant does not let a teacher collect money.
	if err := db.Exec("INSERT INTO role_permissions(role_id,permission_id) VALUES ('teacher','fees:collect') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	call("POST", "/fees/payments", fmt.Sprintf(`{"student_id":%q,"amount":100,"method":"cash","idempotency_key":%q}`, student, key()), teacher, 403)

	paymentKey := key()
	body := fmt.Sprintf(`{"student_id":%q,"amount":500,"method":"cash","idempotency_key":%q}`, student, paymentKey)
	first := data(call("POST", "/fees/payments", body, accountant, 201))
	replay := data(call("POST", "/fees/payments", body, accountant, 200))
	receiptNo := first["payment"].(map[string]any)["receipt_no"]
	if replay["payment"].(map[string]any)["receipt_no"] != receiptNo || replay["replayed"] != true || first["balance_after"].(float64) != 700 {
		t.Fatalf("receipt replay %+v / %+v", first, replay)
	}
	if code := errorCode(call("POST", "/fees/payments", fmt.Sprintf(`{"student_id":%q,"amount":700.01,"method":"cash","idempotency_key":%q}`, student, key()), accountant, 409)); code != "overpayment" {
		t.Fatalf("overpayment code %s", code)
	}
	call("POST", "/fees/payments", fmt.Sprintf(`{"student_id":%q,"amount":100,"method":"bank","reference":"BANK-%s","idempotency_key":%q}`, student, suffix, key()), accountant, 201)
	if code := errorCode(call("POST", "/fees/payments", fmt.Sprintf(`{"student_id":%q,"amount":100,"method":"bank","reference":"bank-%s","idempotency_key":%q}`, student, suffix, key()), admin, 409)); code != "duplicate_payment" {
		t.Fatalf("duplicate reference code %s", code)
	}
	call("POST", "/fees/payments", fmt.Sprintf(`{"student_id":%q,"amount":100,"method":"bkash","idempotency_key":%q}`, student, key()), accountant, 400)

	dues := data(call("GET", "/fees/dues?student_id="+other, "", accountant, 200))["items"].([]any)
	due := dues[0].(map[string]any)["id"].(string)
	call("POST", "/fees/dues/"+due+"/adjustments", `{"kind":"waiver","reason":"Hardship"}`, accountant, 403)
	adjusted := data(call("POST", "/fees/dues/"+due+"/adjustments", `{"kind":"discount","amount":200,"reason":"Sibling"}`, accountant, 200))
	if adjusted["balance"].(float64) != 1000 {
		t.Fatalf("discount %+v", adjusted)
	}

	// Student and guardian visibility.
	own := data(call("GET", "/fees/me", "", studentAuth, 200))
	if own["student"].(map[string]any)["id"] != student || own["totals"].(map[string]any)["balance"].(float64) != 600 {
		t.Fatalf("own statement %+v", own["totals"])
	}
	paymentID := first["payment"].(map[string]any)["id"].(string)
	call("GET", "/fees/me/payments/"+paymentID, "", studentAuth, 200)
	call("GET", "/fees/children/"+student, "", guardianAuth, 404)
	call("GET", "/fees/children/"+other+"/payments/"+paymentID, "", guardianAuth, 404)
	call("GET", "/fees/children/"+other, "", guardianAuth, 200)

	// Online payment is off in a production installation: refused server-side
	// for every role, and the administrator cannot switch simulation on.
	onlineBody := fmt.Sprintf(`{"student_id":%q,"provider":"bkash","amount":300}`, other)
	if code := errorCode(call("POST", "/fees/online/intents", onlineBody, guardianAuth, 403)); code != "online_payments_disabled" {
		t.Fatalf("default intent code %s", code)
	}
	call("POST", "/fees/online/intents", `{"provider":"nagad","amount":50}`, studentAuth, 403)
	if status := data(call("GET", "/fees/online/status", "", guardianAuth, 200)); status["mode"] != "off" || status["simulation_allowed"] != false {
		t.Fatalf("default status %+v", status)
	}
	if code := errorCode(call("PUT", "/fees/online/settings", `{"simulation_enabled":true}`, admin, 409)); code != "simulation_not_allowed" {
		t.Fatalf("enable without deployment permission: %s", code)
	}
	call("PUT", "/fees/online/settings", `{"simulation_enabled":true}`, accountant, 403)

	// A disposable test installation: the administrator switches simulation on.
	active = simRouter
	if status := data(call("GET", "/fees/online/status", "", admin, 200)); status["mode"] != "off" || status["simulation_allowed"] != true {
		t.Fatalf("test installation before enabling %+v", status)
	}
	if status := data(call("PUT", "/fees/online/settings", `{"simulation_enabled":true}`, admin, 200)); status["mode"] != "simulation" {
		t.Fatalf("enable simulation %+v", status)
	}
	call("POST", "/fees/online/intents", `{"provider":"nagad","amount":50}`, studentAuth, 403)
	call("POST", "/fees/online/intents", fmt.Sprintf(`{"student_id":%q,"provider":"bkash","amount":300}`, student), guardianAuth, 404)
	before := data(call("GET", "/fees/children/"+other, "", guardianAuth, 200))["totals"].(map[string]any)["balance"].(float64)
	paymentsBefore := data(call("GET", "/fees/payments?student_id="+other, "", accountant, 200))["total"].(float64)
	intent := data(call("POST", "/fees/online/intents", onlineBody, guardianAuth, 201))
	intentID := intent["id"].(string)
	if !strings.HasPrefix(intent["provider_ref"].(string), "SIMBK") {
		t.Fatalf("simulated reference %v", intent["provider_ref"])
	}
	call("POST", "/fees/online/intents/"+intentID+"/confirm", `{"otp":"123456"}`, studentAuth, 403)
	if code := errorCode(call("POST", "/fees/online/intents/"+intentID+"/confirm", `{"otp":"111111"}`, guardianAuth, 422)); code != "payment_declined" {
		t.Fatalf("declined code %s", code)
	}
	call("POST", "/fees/online/intents/"+intentID+"/confirm", `{"otp":"123456"}`, guardianAuth, 409)
	intent = data(call("POST", "/fees/online/intents", onlineBody, guardianAuth, 201))
	result := data(call("POST", "/fees/online/intents/"+intent["id"].(string)+"/confirm", `{"otp":"123456"}`, guardianAuth, 200))
	if result["simulated"] != true || result["balance"].(float64) != before || result["intent"].(map[string]any)["status"] != "simulated" || result["payment"] != nil {
		t.Fatalf("simulation result %+v", result)
	}
	again := data(call("POST", "/fees/online/intents/"+intent["id"].(string)+"/confirm", `{"otp":"123456"}`, guardianAuth, 200))
	if again["replayed"] != true {
		t.Fatalf("simulation replay %+v", again)
	}
	after := data(call("GET", "/fees/children/"+other, "", guardianAuth, 200))["totals"].(map[string]any)["balance"].(float64)
	paymentsAfter := data(call("GET", "/fees/payments?student_id="+other, "", accountant, 200))["total"].(float64)
	if after != before || paymentsAfter != paymentsBefore {
		t.Fatalf("simulation changed money: balance %v -> %v, payments %v -> %v", before, after, paymentsBefore, paymentsAfter)
	}
	// Switching simulation off blocks checkouts already started.
	pending := data(call("POST", "/fees/online/intents", onlineBody, guardianAuth, 201))
	call("PUT", "/fees/online/settings", `{"simulation_enabled":false}`, admin, 200)
	if code := errorCode(call("POST", "/fees/online/intents/"+pending["id"].(string)+"/confirm", `{"otp":"123456"}`, guardianAuth, 403)); code != "online_payments_disabled" {
		t.Fatalf("confirm after switching off: %s", code)
	}
	active = router

	// A simulated payment recorded before the payment-safety upgrade (as the
	// old simulator did): labelled, excluded from money totals, voidable.
	legacy := insert(`INSERT INTO fee_payments(student_id,amount,method,source,reference,paid_on,idempotency_key,received_by,simulated)
VALUES (?,300,'bkash','online',?,(now() AT TIME ZONE 'Asia/Dhaka')::date,gen_random_uuid(),?,true) RETURNING id`, other, "BKLEGACY"+strings.ToUpper(suffix), guardianUser.ID)
	if err := db.Exec(`INSERT INTO fee_payment_allocations(payment_id,student_fee_id,amount) VALUES (?,?,300)`, legacy, due).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE student_fees SET paid_amount=paid_amount+300 WHERE id=?`, due).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ledger_entries(entry_type,category,amount,entry_date,description,reference,payment_id,created_by) VALUES ('income','fees',300,(now() AT TIME ZONE 'Asia/Dhaka')::date,'Fee receipt (legacy simulation)','',?,?)`, legacy, guardianUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	flagged := data(call("GET", "/fees/children/"+other, "", guardianAuth, 200))
	if flagged["totals"].(map[string]any)["simulated_unreviewed"].(float64) != 300 {
		t.Fatalf("statement flag %+v", flagged["totals"])
	}
	legacyReceipt := data(call("GET", "/fees/children/"+other+"/payments/"+legacy, "", guardianAuth, 200))
	if legacyReceipt["payment"].(map[string]any)["simulated"] != true {
		t.Fatalf("legacy receipt not labelled %+v", legacyReceipt["payment"])
	}
	unreviewed := data(call("GET", "/fees/payments?simulated=unreviewed", "", accountant, 200))
	if unreviewed["total"].(float64) < 1 {
		t.Fatalf("unreviewed list %+v", unreviewed)
	}
	call("POST", "/fees/payments/"+legacy+"/void", `{"reason":"Simulated, no money received"}`, accountant, 403)
	call("POST", "/fees/payments/"+paymentID+"/void", `{"reason":"Not simulated"}`, admin, 403)
	call("POST", "/fees/payments/"+legacy+"/void", `{"reason":""}`, admin, 400)

	// Ledger and reports.
	call("POST", "/accounts/entries", `{"entry_type":"income","category":"fees","amount":10,"entry_date":"2026-01-01","description":"Bypass"}`, accountant, 400)
	entry := data(call("POST", "/accounts/entries", `{"entry_type":"expense","category":"supplies","amount":250,"entry_date":"2026-01-02","description":"Chalk"}`, accountant, 201))
	call("DELETE", "/accounts/entries/"+entry["id"].(string), "", accountant, 200)
	// Real money only: two counter payments (500 + 100). The legacy simulated
	// 300 is reported separately and is not income.
	collections := data(call("GET", "/fees/reports/collections?class_id="+class, "", accountant, 200))
	if collections["total"].(float64) != 600 || collections["count"].(float64) != 2 || collections["simulated_total"].(float64) != 300 || collections["simulated_count"].(float64) != 1 {
		t.Fatalf("collections %+v", collections)
	}
	todayDhaka := time.Now().In(fees.Dhaka).Format("2006-01-02")
	var realIncome, allIncome float64
	db.Raw("SELECT COALESCE(sum(amount),0) FROM ledger_entries WHERE entry_type='income' AND entry_date=(now() AT TIME ZONE 'Asia/Dhaka')::date AND NOT EXISTS (SELECT 1 FROM fee_payments p WHERE p.id=payment_id AND p.simulated)").Row().Scan(&realIncome)
	db.Raw("SELECT COALESCE(sum(amount),0) FROM ledger_entries WHERE entry_type='income' AND entry_date=(now() AT TIME ZONE 'Asia/Dhaka')::date").Row().Scan(&allIncome)
	summary := data(call("GET", "/accounts/summary?from="+todayDhaka+"&to="+todayDhaka, "", accountant, 200))
	if summary["income"].(float64) != realIncome || allIncome-realIncome < 300 {
		t.Fatalf("summary %+v (real %v, all %v)", summary, realIncome, allIncome)
	}
	// The administrator voids the simulated payment: the 300 is owed again.
	owedBefore := data(call("GET", "/fees/children/"+other, "", guardianAuth, 200))["totals"].(map[string]any)["balance"].(float64)
	voided := data(call("POST", "/fees/payments/"+legacy+"/void", `{"reason":"Simulated, no money received"}`, admin, 200))
	if voided["voided_at"] == nil || voided["simulated"] != true {
		t.Fatalf("void %+v", voided)
	}
	owedAfter := data(call("GET", "/fees/children/"+other, "", guardianAuth, 200))["totals"].(map[string]any)
	if owedAfter["balance"].(float64) != owedBefore+300 || owedAfter["simulated_unreviewed"].(float64) != 0 {
		t.Fatalf("void did not restore dues: %v -> %+v", owedBefore, owedAfter)
	}
	call("POST", "/fees/payments/"+legacy+"/void", `{"reason":"Again"}`, admin, 409)
	var voidAudits int64
	db.Raw("SELECT count(*) FROM audit_events WHERE action='fee.simulated_payment_voided' AND target_id=?", legacy).Row().Scan(&voidAudits)
	if voidAudits != 1 {
		t.Fatalf("void audit events %d", voidAudits)
	}
	// Overdue: 600 for the paying student and 1000 for the other (1200 less a
	// 200 discount); simulations and the voided simulated payment paid nothing.
	report := data(call("GET", "/fees/reports/dues?overdue_only=true&class_id="+class, "", accountant, 200))
	if report["total"].(float64) != 2 || report["total_overdue"].(float64) != 1600 {
		t.Fatalf("due report %+v", report)
	}
	call("GET", "/fees/reports/dues?overdue_only=maybe", "", accountant, 400)
	call("GET", "/fees/reports/collections?from=2026-10-05&to=2026-10-01", "", accountant, 400)
	call("GET", "/fees/payments?search="+suffix, "", headTeacher, 403)
	call("GET", "/fees/payments/"+paymentID, "", admin, 200)
}
