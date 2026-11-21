package migrations

import "testing"

func TestLatestVersionMatchesEmbeddedMigrations(t *testing.T) {
	version, err := LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version < 1 {
		t.Fatalf("latest migration version=%d", version)
	}
}
