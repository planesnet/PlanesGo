package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubfolderActions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
	base := t.TempDir()
	run := func(dir string, a ...string) string {
		t.Helper()
		cmd := exec.Command("git", a...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@b", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@b")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", a, out)
		}
		return strings.TrimSpace(string(out))
	}
	remote := filepath.Join(base, "remote.git")
	run(base, "init", "-q", "--bare", "-b", "master", remote)
	main := filepath.Join(base, "project")
	run(base, "clone", "-q", remote, main)
	for _, a := range [][]string{{"commit", "-q", "--allow-empty", "-m", "x"}, {"push", "-q", "origin", "master"}, {"remote", "set-head", "origin", "master"}} {
		run(main, a...)
	}
	ctx := context.Background()
	sub := filepath.Join(base, "sub")
	run(main, "worktree", "add", "-q", "-b", "feature", sub)

	os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644)
	txt, err := WorktreeChanges(ctx, sub)
	if err != nil || !strings.Contains(txt, "a.txt") || !strings.Contains(txt, "Rama feature") {
		t.Fatalf("changes: %v\n%s", err, txt)
	}
	run(sub, "add", "a.txt")
	run(sub, "commit", "-q", "-m", "a")
	if err := PushWorktree(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if run(main, "ls-remote", "origin", "feature") == "" {
		t.Error("the branch did not reach the remote")
	}
	if err := PushWorktree(ctx, main); err == nil {
		t.Error("the main folder is not a subfolder")
	}

	// Uncommitted work: Quitar refuses, Descartar removes it and keeps the branch.
	os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0o644)
	if _, err := QuitaCopia(ctx, sub, false); err == nil {
		t.Fatal("a subfolder with work must not be removed without forcing")
	}
	if err := DiscardWorktree(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) || run(main, "branch", "--list", "feature") == "" {
		t.Error("discard: the folder must go and the branch stay")
	}

	// A subfolder deleted by hand is an orphan until it is forgotten.
	orphan := filepath.Join(base, "orphan")
	run(main, "worktree", "add", "-q", "-b", "other", orphan)
	os.RemoveAll(orphan)
	if l := OrphanWorktrees(ctx, main); len(l) != 1 || l[0].Dir != orphan || l[0].Branch != "other" {
		t.Fatalf("orphans: %v", l)
	}
	Prune(ctx, main)
	if l := OrphanWorktrees(ctx, main); len(l) != 0 {
		t.Errorf("forgotten: %v", l)
	}
}
