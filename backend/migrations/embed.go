package migrations

import (
	"embed"
	"fmt"
	"regexp"
	"strconv"
)

// FS contains the forward-only SQL migrations shipped with the service.
//
//go:embed *.sql
var FS embed.FS

var migrationFile = regexp.MustCompile(`^([0-9]{4})_[a-z0-9_]+\.sql$`)

// LatestVersion returns the highest forward migration version compiled into this binary.
func LatestVersion() (int, error) {
	files, err := FS.ReadDir(".")
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, file := range files {
		match := migrationFile.FindStringSubmatch(file.Name())
		if match == nil {
			return 0, fmt.Errorf("invalid embedded migration filename %q", file.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, err
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		return 0, fmt.Errorf("no embedded SQL migrations found")
	}
	return latest, nil
}
