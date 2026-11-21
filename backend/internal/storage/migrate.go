package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var migrationName = regexp.MustCompile(`^([0-9]{4})_[a-z0-9_]+\.sql$`)

type migration struct {
	version  int64
	name     string
	sql      string
	checksum string
}

func ApplyMigrations(ctx context.Context, db *sql.DB, migrationFS fs.FS) error {
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	migrations := make([]migration, 0, len(files))
	var previous int64
	for _, name := range files {
		match := migrationName.FindStringSubmatch(name)
		if match == nil {
			return fmt.Errorf("invalid migration filename %q; expected NNNN_name.sql", name)
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version <= previous {
			return fmt.Errorf("migration versions must be strictly increasing: %q", name)
		}
		body, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}
		digest := sha256.Sum256(body)
		migrations = append(migrations, migration{version: version, name: name, sql: string(body), checksum: hex.EncodeToString(digest[:])})
		previous = version
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("initialize migration tracking table: %w", err)
	}

	for _, item := range migrations {
		if err := applyOne(ctx, conn, item); err != nil {
			return err
		}
	}
	return nil
}

func applyOne(ctx context.Context, conn *sql.Conn, item migration) error {
	if _, err := conn.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		return fmt.Errorf("begin migration %s: %w", item.name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	var existing string
	err := conn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = ?", item.version).Scan(&existing)
	if err == nil {
		if existing != item.checksum {
			return fmt.Errorf("migration %s checksum mismatch: applied=%s current=%s", item.name, existing, item.checksum)
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return fmt.Errorf("finish migration check %s: %w", item.name, err)
		}
		committed = true
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check migration %s: %w", item.name, err)
	}
	if _, err := conn.ExecContext(ctx, item.sql); err != nil {
		return fmt.Errorf("execute migration %s: %w", item.name, err)
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES(?, ?, ?, ?)", item.version, item.name, item.checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record migration %s: %w", item.name, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit migration %s: %w", item.name, err)
	}
	committed = true
	return nil
}
