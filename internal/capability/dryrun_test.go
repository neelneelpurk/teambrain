package capability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)
	s = s.WithDryRun(true)

	res, err := s.NewSkill("planned", "x")
	if err != nil {
		t.Fatalf("NewSkill dry-run: %v", err)
	}
	if !res.Changed || len(res.Created) == 0 {
		t.Fatal("dry-run should still report the would-be change")
	}
	if _, err := os.Stat(filepath.Join(dir, res.Path)); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write the skill, stat err = %v", err)
	}

	hookRes, err := s.NewHook(HookOptions{Name: "planned-hook", Event: "Stop"})
	if err != nil {
		t.Fatalf("NewHook dry-run: %v", err)
	}
	if !hookRes.Changed {
		t.Fatal("dry-run hook should report a change")
	}
	if _, err := os.Stat(filepath.Join(dir, "hooks", "planned-hook.sh")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write the hook script")
	}
	if _, err := os.Stat(filepath.Join(dir, ".teambrain.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write the manifest")
	}
}
