package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academicsessions"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/analytics"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/classteacher"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/config"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/diary"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/fees"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/httpapi"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notices"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notify"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/promotion"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var authService *auth.Service
	var authorization *rbac.Service
	var schoolService *school.Service
	var userService *users.Service
	var auditService *audit.Service
	var dashboardService *dashboard.Service
	var academicService *academic.Service
	var sectionService *academic.SectionService
	var peopleService *people.Service
	var sessionService *academicsessions.Service
	var staffService *staff.Service
	var attendanceService *attendance.Service
	var studentAttendanceService *attendance.StudentService
	var routineService *routine.Service
	var examService *exams.Service
	var feeService *fees.Service
	var comm *httpapi.Communication
	var adminServices *httpapi.AdminServices
	var analyticsService *analytics.Service
	var promotionService *promotion.Service
	var classTeacherService *classteacher.Service
	var documentService *documents.Service
	if cfg.DatabaseURL != "" {
		// Wait up to a minute for PostgreSQL: when Docker starts, both start
		// together and the database may still be starting up.
		openCtx, cancelOpen := context.WithTimeout(context.Background(), time.Minute)
		defer cancelOpen()
		db, err := database.OpenWhenReady(openCtx, cfg.DatabaseURL, func(err error) {
			logger.Warn("waiting for the database", "error", err.Error())
		})
		if err != nil {
			return err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		httpapi.SetReadiness(sqlDB.PingContext)
		// Migrations get their own, longer deadline: a new school's first
		// start applies every migration, which on a slow or busy server can
		// take longer than connecting does.
		migrateCtx, cancelMigrate := context.WithTimeout(context.Background(), cfg.MigrationTimeout)
		started := time.Now()
		err = database.Migrate(migrateCtx, db)
		cancelMigrate()
		if err != nil {
			return err
		}
		logger.Info("migrations applied", "duration", time.Since(started).Round(time.Millisecond).String())
		connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		authService, err = auth.NewService(auth.NewRepository(db), cfg.JWTSigningKey)
		if err != nil {
			return err
		}
		if cfg.DemoQuickLogin {
			authService.EnableDemoQuickLogin(true)
			logger.Warn("DEMO_QUICK_LOGIN is on: one-click sign-in for seed-demo accounts from localhost; never use this for a real school")
		}
		authorization = rbac.NewService(rbac.NewRepository(db))
		schoolService = school.NewService(school.NewRepository(db))
		userService = users.NewService(users.NewRepository(db))
		auditService = audit.NewService(audit.NewRepository(db))
		dashboardService = dashboard.NewService(dashboard.NewRepository(db))
		var academicSchemaReady bool
		if err := db.WithContext(connectCtx).Raw("SELECT to_regclass('public.academic_class_subjects') IS NOT NULL").Scan(&academicSchemaReady).Error; err != nil {
			return err
		}
		if academicSchemaReady {
			academicService = academic.NewService(academic.NewRepository(db))
			sectionService = academic.NewSectionService(academic.NewSectionRepository(db))
			peopleService = people.NewService(people.NewRepository(db))
		}
		sessionService = academicsessions.NewService(academicsessions.NewRepository(db))
		staffService = staff.NewService(staff.NewRepository(db))
		attendanceService = attendance.NewService(attendance.NewRepository(db))
		studentAttendanceService = attendance.NewStudentService(attendance.NewStudentRepository(db))
		routineService = routine.NewService(routine.NewRepository(db))
		examService = exams.NewService(exams.NewRepository(db))
		feeService = fees.NewService(fees.NewRepository(db), fees.SimulationGateway{}, fees.ServiceOptions{SimulationAllowed: cfg.PaymentSimulationAllowed})
		store, err := files.NewLocalStore(cfg.FileStorageDir)
		if err != nil {
			return fmt.Errorf("file storage: %w", err)
		}
		comm = &httpapi.Communication{
			Notices: notices.NewService(notices.NewRepository(db)),
			Notify:  notify.NewService(notify.NewRepository(db), notify.MockSMS{}),
			Diary:   diary.NewService(diary.NewRepository(db)),
			Files:   files.NewService(db, store),
		}
		backups, err := admin.NewBackups(db, cfg.BackupDir, cfg.FileStorageDir, cfg.BackupRetention)
		if err != nil {
			return fmt.Errorf("backup storage: %w", err)
		}
		adminServices = &httpapi.AdminServices{Exporter: admin.NewExporter(db), Backups: backups, Overview: admin.NewOverviewReader(db, backups)}
		analyticsService = analytics.NewService(analytics.NewRepository(db))
		promotionService = promotion.NewService(db)
		classTeacherService = classteacher.NewService(db)
		// Result documents need the Typst renderer and its fonts; without
		// them the document routes answer 503 and everything else works.
		var renderer documents.Renderer
		if typst, err := documents.NewTypst(cfg.TypstBin, cfg.DocumentFontsDir, 20*time.Second, 2); err == nil {
			renderer = typst
		} else {
			logger.Warn("result documents are off", "reason", err)
		}
		if results := examService.Results(); results != nil {
			documentService = documents.NewService(db, results, store, renderer, logger)
			documentService.BaseURL = cfg.PublicBaseURL
			documentService.Logo = documents.NewLogoFetcher().Fetch
		}
		// Routine PDFs (E8) use the same renderer.
		if routineService.Term != nil && documentService.RoutineAvailable() {
			routineService.Term.Documents = documentService
		}
		// Admit cards too.
		if routineService.Admit != nil && documentService.RoutineAvailable() {
			routineService.Admit.Documents = documentService
		}
		logger.Info("postgres connected")
	} else {
		logger.Warn("DATABASE_URL is unset; starting without postgres")
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpapi.NewRouterWithDocuments(authService, cfg.AuthCookieSecure, authorization, schoolService, userService, auditService, dashboardService, sessionService, academicService, sectionService, peopleService, staffService, attendanceService, studentAttendanceService, routineService, examService, feeService, comm, adminServices, analyticsService, promotionService, documentService, classTeacherService),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if adminServices != nil && cfg.BackupInterval > 0 {
		go scheduledBackups(ctx, logger, adminServices.Backups, cfg.BackupInterval)
	}
	if documentService.Available() {
		go documentService.Run(ctx, 2*time.Second)
	}
	if comm != nil && cfg.NotifyInterval > 0 {
		go backgroundWork(ctx, logger, comm, cfg.NotifyInterval)
	}
	if feeService != nil {
		go lateFines(ctx, logger, feeService, 30*time.Minute)
	}

	serveResult := make(chan error, 1)
	go func() { serveResult <- server.ListenAndServe() }()
	logger.Info("http server listening", "port", cfg.Port)

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// backgroundWork dispatches notifications (which also hands queued SMS to the
// mock provider) and removes abandoned uploads on a fixed interval.
func backgroundWork(ctx context.Context, logger *slog.Logger, comm *httpapi.Communication, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCtx, cancel := context.WithTimeout(ctx, interval)
			if result, err := comm.Notify.Dispatch(runCtx); err != nil {
				logger.Error("notification dispatch failed", "error", err)
			} else if !result.Skipped && !result.Disabled {
				logger.Info("notifications dispatched", "created", result.Created, "withdrawn", result.Withdrawn, "sms_sent_mock", result.SMSSentMock, "sms_failed", result.SMSFailed)
			}
			if removed, err := comm.Files.Cleanup(runCtx); err != nil {
				logger.Error("upload cleanup failed", "error", err)
			} else if removed > 0 {
				logger.Info("abandoned uploads removed", "count", removed)
			}
			cancel()
		}
	}
}

// lateFines charges the late fines that fell due, at startup and then on a
// fixed interval. Payments and statements also charge a student's fines
// first, so the counter never waits for this pass.
func lateFines(ctx context.Context, logger *slog.Logger, service *fees.Service, interval time.Duration) {
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if result, err := service.ApplyLateFines(runCtx); err != nil {
			logger.Error("late fines failed", "error", err)
		} else if result.Charged > 0 {
			logger.Info("late fines charged", "dues", result.Charged, "amount", result.Amount)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// scheduledBackups takes an automatic backup every interval; retention pruning
// happens after each successful archive.
func scheduledBackups(ctx context.Context, logger *slog.Logger, backups *admin.Backups, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, err := backups.Create(ctx, nil, "scheduled")
			if err != nil {
				logger.Error("scheduled backup failed", "error", err)
				continue
			}
			logger.Info("scheduled backup completed", "id", run.ID, "size_bytes", run.SizeBytes)
		}
	}
}
