package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Subfolders (git worktrees) beyond listing and removing them: the ones whose folder no longer exists, their
// changes, pushing their branch and discarding them. Nothing here touches the main folder of the repository, and
// only DiscardWorktree loses work (the interface asks first, listing what would be lost).

// Orphan: a subfolder git still knows whose folder no longer exists (deleted by hand), with its branch.
type Orphan struct{ Dir, Branch string }

// OrphanWorktrees lists the orphan subfolders of a repository.
func OrphanWorktrees(ctx context.Context, repoDir string) []Orphan {
	out, err := git(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	var l []Orphan
	for _, b := range strings.Split(out, "\n\n") {
		o, prunable := Orphan{}, false
		for _, x := range strings.Split(b, "\n") {
			switch {
			case strings.HasPrefix(x, "worktree "):
				o.Dir = strings.TrimPrefix(x, "worktree ")
			case strings.HasPrefix(x, "branch refs/heads/"):
				o.Branch = strings.TrimPrefix(x, "branch refs/heads/")
			case strings.HasPrefix(x, "prunable"):
				prunable = true
			}
		}
		if o.Dir != "" && prunable {
			l = append(l, o)
		}
	}
	return l
}

// WorktreeChanges: what the subfolder has: its uncommitted files and the commits and files of its branch against
// the default branch (plain text, to show as it is).
func WorktreeChanges(ctx context.Context, dir string) (string, error) {
	e, err := EstadoCopia(ctx, dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Rama %s · %s\n", e.Rama, dir)
	if st, _ := git(ctx, dir, "status", "--short"); st != "" {
		fmt.Fprintf(&b, "\nSin commit:\n%s\n", st)
	} else {
		b.WriteString("\nSin cambios por hacer commit.\n")
	}
	if e.Defecto != "" && e.Rama != e.Defecto {
		base := e.Defecto
		if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+base); err == nil {
			base = "origin/" + base
		}
		if log, _ := git(ctx, dir, "log", "--oneline", "-20", base+"..HEAD"); log != "" {
			fmt.Fprintf(&b, "\nCommits de la rama (frente a %s):\n%s\n", base, log)
		}
		if stat, _ := git(ctx, dir, "diff", "--stat", base+"...HEAD"); stat != "" {
			fmt.Fprintf(&b, "\nFicheros:\n%s\n", stat)
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// PushWorktree pushes the subfolder branch to origin. Never the default branch (pushing it can deploy).
func PushWorktree(ctx context.Context, dir string) error {
	e, err := EstadoCopia(ctx, dir)
	if err != nil {
		return err
	}
	switch {
	case !e.Worktree:
		return ErrNoEsCopia
	case e.Rama == "":
		return errors.New("la subcarpeta no está en ninguna rama")
	case e.Rama == e.Defecto:
		return fmt.Errorf("«%s» es la rama principal: no se sube desde aquí", e.Rama)
	}
	_, err = gitSlow(ctx, dir, "push", "-u", "origin", e.Rama)
	return err
}

// DiscardWorktree removes the subfolder even with uncommitted work (git worktree remove --force): for a stuck
// subfolder whose work is no longer wanted. The branch is kept, so its commits are not lost.
func DiscardWorktree(ctx context.Context, dir string) error {
	e, err := EstadoCopia(ctx, dir)
	if err != nil {
		return err
	}
	if !e.Worktree {
		return ErrNoEsCopia
	}
	Prune(ctx, e.Principal)
	if out, err := gitSalida(ctx, e.Principal, "worktree", "remove", "--force", "--", e.Carpeta); err != nil {
		return fmt.Errorf("git no ha quitado la subcarpeta: %s", out)
	}
	return nil
}
