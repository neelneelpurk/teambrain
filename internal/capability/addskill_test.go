package capability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddSkillInstallsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)
	content := []byte("---\nname: code-review\ndescription: Review code.\n---\nbody\n")

	res, err := s.AddSkill("code-review", content)
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}
	if !res.Changed || len(res.Created) != 1 {
		t.Fatalf("first AddSkill should create the skill: %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "skills", "code-review", "SKILL.md"))
	if string(got) != string(content) {
		t.Fatalf("installed content mismatch")
	}

	// Re-adding does not clobber an edited copy.
	edited := []byte("---\nname: code-review\ndescription: My edited version.\n---\nmine\n")
	if err := os.WriteFile(filepath.Join(dir, "skills", "code-review", "SKILL.md"), edited, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = s.AddSkill("code-review", content)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("re-adding an existing skill should report no change")
	}
	got, _ = os.ReadFile(filepath.Join(dir, "skills", "code-review", "SKILL.md"))
	if string(got) != string(edited) {
		t.Fatal("AddSkill must not clobber an existing (edited) skill")
	}
}
