package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ExcludeLocally adds entry («.pgocode/», «.pgo.json»…) to the repository's .git/info/exclude, the local list of
// ignored files: git does not show what pgocode keeps in a project folder, and nothing changes in the repository
// itself. Outside a repository, nothing happens.
func ExcludeLocally(dir, entry string) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	f := filepath.Join(gitDir, "info", "exclude")
	b, _ := os.ReadFile(f)
	if slices.Contains(strings.Split(string(b), "\n"), entry) {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		fmt.Fprintln(fh)
	}
	fmt.Fprintln(fh, entry)
}

// Tracked: whether file (inside a repository) is in git.
func Tracked(file string) bool {
	return exec.Command("git", "-C", filepath.Dir(file), "ls-files", "--error-unmatch", "--", filepath.Base(file)).Run() == nil
}
