package capability

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/testutil"
)

const foreignSettings = `{
  "permissions": {
    "allow": ["Bash(go test:*)"]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          { "type": "command", "command": "existing-formatter.sh", "timeout": 30 }
        ]
      }
    ]
  }
}
`

func TestMergeHookIntoEmpty(t *testing.T) {
	t.Parallel()

	reg := HookRegistration{
		Event:   "PostToolUse",
		Matcher: "Edit|Write",
		Command: "$CLAUDE_PROJECT_DIR/.claude/hooks/fmt.sh",
	}
	out, changed, err := MergeHook(nil, reg)
	if err != nil {
		t.Fatalf("MergeHook: %v", err)
	}
	if !changed {
		t.Fatal("merging into empty settings should report a change")
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "settings", "merge_empty.golden"), out)
}

func TestMergeHookPreservesForeignContent(t *testing.T) {
	t.Parallel()

	reg := HookRegistration{
		Event:   "PostToolUse",
		Matcher: "Write",
		Command: "$CLAUDE_PROJECT_DIR/.claude/hooks/notify.sh",
	}
	out, changed, err := MergeHook([]byte(foreignSettings), reg)
	if err != nil {
		t.Fatalf("MergeHook: %v", err)
	}
	if !changed {
		t.Fatal("expected a change")
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "settings", "merge_preserves_foreign.golden"), out)

	// The foreign permissions block, the foreign hook, and its unknown "timeout"
	// field must all survive.
	s := string(out)
	for _, must := range []string{"permissions", "Bash(go test:*)", "existing-formatter.sh", "timeout", "notify.sh"} {
		if !strings.Contains(s, must) {
			t.Errorf("merged output lost %q", must)
		}
	}
	// And it must still be valid JSON.
	var sink map[string]any
	if err := json.Unmarshal(out, &sink); err != nil {
		t.Fatalf("merged output is not valid JSON: %v", err)
	}
}

func TestMergeHookIsIdempotent(t *testing.T) {
	t.Parallel()

	reg := HookRegistration{Event: "Stop", Command: "$CLAUDE_PROJECT_DIR/.claude/hooks/done.sh"}
	once, _, err := MergeHook(nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	twice, changed, err := MergeHook(once, reg)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("merging the same hook twice should report no change")
	}
	if string(once) != string(twice) {
		t.Fatalf("idempotent merge changed bytes:\n%s\n---\n%s", once, twice)
	}
}

func TestUnmergeHookRemovesOnlyTarget(t *testing.T) {
	t.Parallel()

	// Start from foreign settings, add our hook, then remove it.
	reg := HookRegistration{Event: "PostToolUse", Matcher: "Write", Command: "ours.sh"}
	withOurs, _, err := MergeHook([]byte(foreignSettings), reg)
	if err != nil {
		t.Fatal(err)
	}
	out, changed, err := UnmergeHook(withOurs, "ours.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("unmerge should report a change")
	}
	s := string(out)
	if strings.Contains(s, "ours.sh") {
		t.Error("our hook should be gone")
	}
	if !strings.Contains(s, "existing-formatter.sh") || !strings.Contains(s, "permissions") {
		t.Error("foreign content must be preserved on unmerge")
	}

	// Removing a non-existent command is a no-op.
	_, changed, err = UnmergeHook(out, "ghost.sh")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("removing a missing hook should report no change")
	}
}
