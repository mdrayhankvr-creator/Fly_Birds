package database

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// OpenWhenReady opens the database, trying again every second until it
// accepts connections or ctx ends. When Docker starts, its restart policy
// starts the API and PostgreSQL together, without the Compose start order,
// so the first tries can be refused while PostgreSQL is still starting.
// waiting is called once, with the first error, when the first try fails.
func OpenWhenReady(ctx context.Context, dsn string, waiting func(error)) (*gorm.DB, error) {
	for attempt := 0; ; attempt++ {
		tryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		db, err := Open(tryCtx, dsn)
		cancel()
		if err == nil {
			return db, nil
		}
		if attempt == 0 && waiting != nil {
			waiting(err)
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(time.Second):
		}
	}
}
