package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ambatumove/core/internal/config"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

func TestVerifyBackupRejectsSchemaNewerThanBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := storage.OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := migrations.LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO schema_migrations(version,name,checksum,applied_at) VALUES(?,?,?,?)`, latest+1, "future", "checksum", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	err = verifyBackup([]string{"--path", path})
	if err == nil || !strings.Contains(err.Error(), "newer than this binary supports") {
		t.Fatalf("verifyBackup error=%v, want a schema compatibility rejection", err)
	}
}

func TestSchemaVersionReportsAppliedMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.sqlite")
	db, err := storage.OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err = storage.ApplyMigrations(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	latest, err := migrations.LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = writer
	callErr := schemaVersion(config.Config{DatabasePath: path})
	_ = writer.Close()
	os.Stdout = previousStdout
	if callErr != nil {
		t.Fatal(callErr)
	}
	output, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte(strconv.Itoa(latest) + "\n"); !bytes.Equal(output, want) {
		t.Fatalf("schema-version output=%q, want %q", output, want)
	}
}

func TestAdminReferenceImportDryRunAndExportCommands(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "admin.sqlite")
	cfg, err := config.Load(func(key string) (string, bool) {
		if key == "DATABASE_PATH" {
			return dbPath, true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "equipment.json")
	document := `[{"id":"hex_dumbbell","label":"Hex Dumbbell","category":"free_weight","active":true}]`
	if err := os.WriteFile(inputPath, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := admin(cfg, []string{"equipment", "import", "--file", inputPath, "--dry-run"}); err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	db, err := storage.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM equipment WHERE id='hex_dumbbell'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("CLI dry-run wrote %d rows (err=%v)", count, err)
	}
	_ = db.Close()
	if err := admin(cfg, []string{"equipment", "import", "--file", inputPath}); err != nil {
		t.Fatalf("import command: %v", err)
	}
	exportPath := filepath.Join(dir, "equipment-export.json")
	if err := admin(cfg, []string{"equipment", "export", "--file", exportPath}); err != nil {
		t.Fatalf("export command: %v", err)
	}
	exported, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exported), `"id": "hex_dumbbell"`) {
		t.Fatalf("export omitted imported record: %s", exported)
	}
}
