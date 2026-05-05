package model

import (
	"strings"
	"testing"
)

func TestPostgresCompatPreMigrationSQLIncludesLongTextDomain(t *testing.T) {
	joined := strings.Join(postgresCompatPreMigrationSQL(), "\n")
	if !strings.Contains(joined, "typname = 'longtext'") {
		t.Fatalf("expected longtext type guard in compat SQL, got %q", joined)
	}
	if !strings.Contains(joined, "CREATE DOMAIN longtext AS text") {
		t.Fatalf("expected longtext domain creation in compat SQL, got %q", joined)
	}
}
