package database

import (
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"testing"
	"time"
)

// A database that never answers is retried until the deadline, then the
// last error is returned.
func TestOpenWhenReadyGivesUpAtTheDeadline(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // nothing listens there now: every try is refused
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	waited := 0
	started := time.Now()
	db, err := OpenWhenReady(ctx, "postgres://u:p@"+addr+"/db?sslmode=disable&connect_timeout=1", func(error) { waited++ })
	if err == nil || db != nil {
		t.Fatal("opened a database that is not there")
	}
	if took := time.Since(started); took < 2*time.Second || took > 8*time.Second {
		t.Fatalf("gave up after %s, want about the 2.5 s deadline", took)
	}
	if waited != 1 {
		t.Fatalf("waiting reported %d times, want once", waited)
	}
}

// A database that starts accepting connections later is opened: for 1.5 s
// nothing listens on the address, then a proxy forwards to the test database.
func TestPostgresOpenWhenReadyWaitsForTheDatabase(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	target := u.Host
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u.Host = l.Addr().String()
	l.Close()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		select {
		case <-stop:
			return
		case <-time.After(1500 * time.Millisecond):
		}
		proxy, err := net.Listen("tcp", u.Host)
		if err != nil {
			return
		}
		go func() { <-stop; proxy.Close() }()
		for {
			in, err := proxy.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp", target)
			if err != nil {
				in.Close()
				continue
			}
			go func() { io.Copy(out, in); out.Close() }()
			go func() { io.Copy(in, out); in.Close() }()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	waited := false
	started := time.Now()
	db, err := OpenWhenReady(ctx, u.String(), func(error) { waited = true })
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	if !waited || time.Since(started) < time.Second {
		t.Fatalf("opened after %s without waiting (%v)", time.Since(started), waited)
	}
}
