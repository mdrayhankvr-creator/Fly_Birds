package database

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// With database/sql defaults only two idle connections survived a burst, so
// the next burst opened new PostgreSQL backends and re-planned every query.
func TestPostgresPoolKeepsConnectionsBetweenBursts(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if got := sqlDB.Stats().MaxOpenConnections; got != maxOpenConns {
		t.Fatalf("max open connections %d, want %d", got, maxOpenConns)
	}
	burst := func() {
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := db.WithContext(ctx).Exec("SELECT pg_sleep(0.05)").Error; err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
	}
	burst()
	if idle := sqlDB.Stats().Idle; idle < 10 {
		t.Fatalf("pool kept %d idle connections after a 10-way burst", idle)
	}
	opened := sqlDB.Stats().OpenConnections
	burst()
	if now := sqlDB.Stats().OpenConnections; now != opened {
		t.Fatalf("second burst changed open connections from %d to %d", opened, now)
	}
}
