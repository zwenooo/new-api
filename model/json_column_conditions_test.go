package model

import (
	"strings"
	"testing"

	"one-api/common"
)

func TestJSONColumnConditions(t *testing.T) {
	prevMySQL := common.UsingMySQL
	prevPostgres := common.UsingPostgreSQL
	t.Cleanup(func() {
		common.UsingMySQL = prevMySQL
		common.UsingPostgreSQL = prevPostgres
	})

	common.UsingMySQL = false
	common.UsingPostgreSQL = true
	if got := jsonColumnIsEmptyCondition("allowed_groups"); !strings.Contains(got, "::text = '[]'") {
		t.Fatalf("expected postgres JSON empty condition, got %q", got)
	}
	if got := jsonColumnIsNotEmptyCondition("allowed_groups"); !strings.Contains(got, "::text <> '[]'") {
		t.Fatalf("expected postgres JSON non-empty condition, got %q", got)
	}

	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	if got := jsonColumnIsEmptyCondition("allowed_groups"); !strings.Contains(got, "allowed_groups = ''") {
		t.Fatalf("expected sqlite-compatible JSON empty condition, got %q", got)
	}
	if got := jsonColumnIsNotEmptyCondition("allowed_groups"); !strings.Contains(got, "allowed_groups <> ''") {
		t.Fatalf("expected sqlite-compatible JSON non-empty condition, got %q", got)
	}
}
