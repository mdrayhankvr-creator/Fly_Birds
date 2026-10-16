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
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/config"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/httpapi"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
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
	var sessionService *academicsessions.Service
	var staffService *staff.Service
	var attendanceService *attendance.Service
	if cfg.DatabaseURL != "" {
		connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		db, err := database.Open(connectCtx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		if err := database.Migrate(connectCtx, db); err != nil {
			return err
		}
		authService, err = auth.NewService(auth.NewRepository(db), cfg.JWTSigningKey)
		if err != nil {
			return err
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
		}
		sessionService = academicsessions.NewService(academicsessions.NewRepository(db))
		staffService = staff.NewService(staff.NewRepository(db))
		attendanceService = attendance.NewService(attendance.NewRepository(db))
		logger.Info("postgres connected")
	} else {
		logger.Warn("DATABASE_URL is unset; starting without postgres")
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpapi.NewRouterWithBatchServices(authService, cfg.AuthCookieSecure, authorization, schoolService, userService, auditService, dashboardService, sessionService, academicService, staffService, attendanceService),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
