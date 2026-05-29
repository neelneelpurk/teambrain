// Package testutil provides shared test helpers: golden-file assertions and a
// global -update flag. Importing it from a _test.go file registers the flag, so
// `go test ./... -update` rewrites every golden fixture in one pass.
package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// update is set by `go test -update` to rewrite golden files instead of
// asserting against them.
var update = flag.Bool("update", false, "update golden files")

// Update reports whether golden files should be rewritten.
func Update() bool { return *update }

// AssertGolden compares got against the contents of goldenPath. With -update it
// writes got to goldenPath and passes. The path is created (with parents) on
// update so new fixtures are easy to add.
func AssertGolden(t *testing.T, goldenPath string, got []byte) {
	t.Helper()

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test -update` to create it)", goldenPath, err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Fatalf("golden mismatch for %s (-want +got):\n%s", goldenPath, diff)
	}
}

// AssertGoldenString is AssertGolden for string payloads.
func AssertGoldenString(t *testing.T, goldenPath, got string) {
	t.Helper()
	AssertGolden(t, goldenPath, []byte(got))
}
