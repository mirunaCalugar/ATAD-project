package main

import (
	"strings"
	"testing"

	"atad-project/internal/version"
)

// The marking script reads these fields. If this test fails, the contract is
// broken and the build is not gradeable.
func TestVersionReportsContractFields(t *testing.T) {
	var b strings.Builder
	if err := version.WriteJSON(&b); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	for _, field := range []string{`"app"`, `"version"`, `"commit"`, `"built_at"`} {
		if !strings.Contains(b.String(), field) {
			t.Errorf("version output is missing %s: %s", field, b.String())
		}
	}
}

// Replace this with tests for your own logic. Aim for the behaviour that would
// break silently, not for the lines that are easy to reach.
func TestYourLogic(t *testing.T) {
	t.Skip("write your own tests here")
}
