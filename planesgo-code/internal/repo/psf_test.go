package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparePSFBranch(t *testing.T) {
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
	user := filepath.Join(base, "project")
	run(base, "clone", "-q", remote, user)
	for _, a := range [][]string{{"config", "user.email", "a@b"}, {"config", "user.name", "a"}, {"commit", "-q", "--allow-empty", "-m", "x"}, {"push", "-q", "origin", "master"}, {"remote", "set-head", "origin", "master"}} {
		run(user, a...)
	}
	// The user is on another branch: the issue branch still starts from origin/master, without touching their folder.
	run(user, "checkout", "-q", "-b", "work")
	ctx := context.Background()
	branches := filepath.Join(base, "branches")
	ws, err := PreparePSFBranch(ctx, user, 7, "", branches)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Rama != "psf/issue-7" || ws.Base != "master" || ws.Carpeta != filepath.Join(branches, "project", "psf_issue-7") {
		t.Fatalf("worktree: %+v", ws)
	}
	if c, _ := CarpetaDeRama(ctx, user, "psf/issue-7", branches); c != ws.Carpeta {
		t.Errorf("the chat would use another folder: %s", c)
	}
	if run(user, "branch", "--show-current") != "work" {
		t.Error("switched the branch of the user folder")
	}
	if n := run(ws.Carpeta, "rev-list", "--count", "origin/master..origin/psf/issue-7"); n != "1" {
		t.Errorf("initial commit pushed: %s", n)
	}
	// The second time reuses the worktree and adds no commits.
	if ws2, err := PreparePSFBranch(ctx, user, 7, "", branches); err != nil || ws2.Carpeta != ws.Carpeta {
		t.Fatalf("second time: %v %+v", err, ws2)
	}
	if n := run(ws.Carpeta, "rev-list", "--count", "origin/master..HEAD"); n != "1" {
		t.Errorf("commits after repeating: %s", n)
	}

	// Spec and Builder work: commit, push and diff without the spec.
	if err := WritePSFSpec(ctx, ws.Carpeta, ws.Rama, 7, "# Spec"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(ws.Carpeta, "hello.go"), []byte("package hello\n"), 0o644)
	r, err := SyncPSF(ctx, ws.Carpeta, ws.Rama, "master", "feat(issue-7): implement spec", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Committed || len(r.Files) != 1 || r.Files[0] != "hello.go" || !strings.Contains(r.Diff, "package hello") || r.Commits != 3 {
		t.Errorf("sync: %+v", r)
	}
	if run(ws.Carpeta, "status", "--porcelain") != "" || run(ws.Carpeta, "rev-list", "--count", "origin/psf/issue-7..HEAD") != "0" {
		t.Error("something left uncommitted or unpushed")
	}
	if r, _ := SyncPSF(ctx, ws.Carpeta, ws.Rama, "master", "x", 10); r.Committed || !strings.Contains(r.Diff, "recortada") {
		t.Errorf("no changes and truncated: %+v", r)
	}
	if _, err := SyncPSF(ctx, user, ws.Rama, "master", "x", 0); err == nil {
		t.Error("must not commit on another branch")
	}

	// A chosen merge branch: the issue branch starts from it; a branch missing on the remote is an error.
	run(user, "push", "-q", "origin", "work")
	ws8, err := PreparePSFBranch(ctx, user, 8, "work", branches)
	if err != nil || ws8.Base != "work" {
		t.Fatalf("target branch: %v %+v", err, ws8)
	}
	if run(ws8.Carpeta, "merge-base", "HEAD", "origin/work") != run(user, "rev-parse", "origin/work") {
		t.Error("the issue branch does not start from the target branch")
	}
	if _, err := PreparePSFBranch(ctx, user, 9, "no-such-branch", branches); err == nil {
		t.Error("a target branch missing on the remote must fail")
	}
}
