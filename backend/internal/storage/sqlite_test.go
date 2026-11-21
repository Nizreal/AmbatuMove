package storage

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestOpenSQLiteAppliesPragmasToPooledConnections(t *testing.T) {
	db, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()

	connections := make([]interface{ Close() error }, 0, 2)
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	for i := 0; i < 2; i++ {
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatalf("db.Conn() error = %v", err)
		}
		var foreignKeys, busyTimeout, synchronous int
		if err := conn.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(context.Background(), "PRAGMA synchronous").Scan(&synchronous); err != nil {
			t.Fatal(err)
		}
		var journalMode string
		if err := conn.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 || busyTimeout != 5000 || synchronous != 2 || journalMode != "wal" {
			t.Fatalf("connection %d pragmas: fk=%d busy=%d sync=%d journal=%s", i, foreignKeys, busyTimeout, synchronous, journalMode)
		}
		connections = append(connections, conn)
	}
}

func testMigrations(sql string) fstest.MapFS {
	return fstest.MapFS{"0001_initial.sql": &fstest.MapFile{Data: []byte(sql)}}
}

func TestMigrateAppliesMigrationAndVerifiesChecksum(t *testing.T) {
	db, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "migrations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := ApplyMigrations(context.Background(), db, testMigrations("CREATE TABLE sample (id INTEGER PRIMARY KEY);")); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if err := ApplyMigrations(context.Background(), db, testMigrations("CREATE TABLE sample (id INTEGER PRIMARY KEY);")); err != nil {
		t.Fatalf("repeat ApplyMigrations() error = %v", err)
	}
	if err := ApplyMigrations(context.Background(), db, testMigrations("CREATE TABLE altered (id INTEGER PRIMARY KEY);")); err == nil {
		t.Fatal("ApplyMigrations() accepted changed checksum")
	}
}
