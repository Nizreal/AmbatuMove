package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	maxOpenConnections = 4
	maxIdleConnections = 2
)

func OpenSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path must not be empty")
	}
	if path != ":memory:" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve database path: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		path = absPath
	}

	uriPath := filepath.ToSlash(path)
	rawURIPath := ""
	if len(uriPath) >= 2 && uriPath[1] == ':' {
		uriPath = "/" + uriPath
		rawURIPath = "/" + uriPath[1:2] + "%3A" + uriPath[3:]
	}
	fileURL := (&url.URL{Scheme: "file", Path: uriPath, RawPath: rawURIPath}).String()
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(FULL)")
	db, err := sql.Open("sqlite", fileURL+"?"+query.Encode())
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConnections)
	db.SetMaxIdleConns(maxIdleConnections)
	db.SetConnMaxLifetime(0)

	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to SQLite: %w", err)
	}
	defer conn.Close()
	var journalMode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable SQLite WAL: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		_ = db.Close()
		return nil, fmt.Errorf("SQLite refused WAL mode (reported %q)", journalMode)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping SQLite: %w", err)
	}
	return db, nil
}
