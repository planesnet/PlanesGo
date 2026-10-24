package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Planes Software Factory (PSF): the psf/issue-<N> branch of an issue, in its own worktree, starting from the
// remote default branch. It is the same folder the chat uses for that branch (CopiaDeRama), so the agent just
// works there.

// PSFBranch: the work branch of an issue.
func PSFBranch(issue int) string { return fmt.Sprintf("psf/issue-%d", issue) }

// PSFSpecPath: where the issue spec lives inside the repository.
func PSFSpecPath(issue int) string { return fmt.Sprintf(".psf/specs/issue-%d-spec.md", issue) }

// gitSlow: like git, with a longer timeout (fetch and push go over the network).
func gitSlow(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := execGit(ctx, dir, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("git %s: %s", args[0], s)
	}
	return s, nil
}

// PSFWorkspace: the issue worktree, its branch and the base branch.
type PSFWorkspace struct {
	CopiaCreada
	Base string `json:"base"`
}

// PreparePSFBranch gets the issue worktree ready: the psf/issue-<N> branch from origin/<target> (the remote
// default branch when target is empty), or the existing one, local or remote; and, while it has no commits of its
// own, an empty commit pushed to the remote so GitHub accepts the draft PR. It never switches the branch of the
// user's folder. base is where the worktrees live.
func PreparePSFBranch(ctx context.Context, folder string, issue int, target, base string) (PSFWorkspace, error) {
	if issue <= 0 {
		return PSFWorkspace{}, errors.New("issue no válida")
	}
	branch := PSFBranch(issue)
	root, err := git(ctx, folder, "rev-parse", "--show-toplevel")
	if err != nil {
		return PSFWorkspace{}, errors.New("la carpeta no es un repositorio git")
	}
	mainDir, err := RepoPrincipal(ctx, root)
	if err != nil {
		return PSFWorkspace{}, err
	}
	def := target
	if def == "" {
		def = RamaDefecto(ctx, mainDir)
	} else if !ramaValida.MatchString(def) || strings.Contains(def, "..") || strings.HasPrefix(def, "-") {
		return PSFWorkspace{}, errors.New("la rama de destino no es válida")
	}
	if def == "" {
		return PSFWorkspace{}, errors.New("no se sabe cuál es la rama principal del repositorio")
	}
	if _, err := gitSlow(ctx, mainDir, "fetch", "--quiet", "origin", def); err != nil {
		return PSFWorkspace{}, fmt.Errorf("la rama de destino «%s» no está en el remoto: %v", def, err)
	}
	// New branches start at origin/<default>; an existing one (local or remote) is reused as it is.
	if _, err := git(ctx, mainDir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		gitSlow(ctx, mainDir, "fetch", "--quiet", "origin", branch) // it may not exist: not an error
		if _, err := git(ctx, mainDir, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch); err != nil {
			if _, err := git(ctx, mainDir, "branch", "--no-track", branch, "origin/"+def); err != nil {
				return PSFWorkspace{}, fmt.Errorf("no se pudo crear la rama %s", branch)
			}
		}
	}
	ws := PSFWorkspace{Base: def}
	c, err := CopiaDeRama(ctx, folder, branch, base)
	if err != nil {
		return ws, err
	}
	ws.CopiaCreada = c
	ws.Rama = branch
	dir := c.Carpeta
	if err := os.MkdirAll(filepath.Join(dir, ".psf", "specs"), 0o755); err != nil {
		return ws, err
	}
	if n, _ := git(ctx, dir, "rev-list", "--count", "origin/"+def+"..HEAD"); n == "0" {
		if _, err := gitSlow(ctx, dir, "commit", "--allow-empty", "-m", fmt.Sprintf("chore(issue-%d): initialize PSF workspace", issue)); err != nil {
			return ws, err
		}
	}
	if err := pushBranch(ctx, dir, branch); err != nil {
		return ws, err
	}
	return ws, nil
}

// FetchPSFBase brings the latest origin/<base> into the repository (worktrees share it), so an agent without network
// (e.g. in a sandbox) can merge it to solve a conflict.
func FetchPSFBase(ctx context.Context, folder, base string) error {
	if base == "" || !ramaValida.MatchString(base) || strings.Contains(base, "..") || strings.HasPrefix(base, "-") {
		return errors.New("rama base no válida")
	}
	mainDir, err := RepoPrincipal(ctx, folder)
	if err != nil {
		return err
	}
	_, err = gitSlow(ctx, mainDir, "fetch", "--quiet", "origin", base)
	return err
}

// pushBranch pushes the branch when it has commits the remote lacks (or is not on the remote yet).
func pushBranch(ctx context.Context, dir, branch string) error {
	if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch); err == nil {
		if n, _ := git(ctx, dir, "rev-list", "--count", "origin/"+branch+"..HEAD"); n == "0" {
			return nil
		}
	}
	_, err := gitSlow(ctx, dir, "push", "-u", "origin", branch)
	return err
}

// PSFSync: what the Builder did, already committed and pushed.
type PSFSync struct {
	Committed bool     `json:"committed"` // there were uncommitted changes and they got committed
	Diff      string   `json:"diff"`      // git diff origin/<base>...HEAD (without the spec)
	DiffSize  int      `json:"diff_size"` // size of the whole diff (Diff may be truncated)
	Files     []string `json:"files"`
	Commits   int      `json:"commits"`
}

// SyncPSF commits whatever is pending in the issue worktree (with that message), pushes it and returns the diff
// against the base branch, truncated to maxDiff characters.
func SyncPSF(ctx context.Context, dir, branch, base, message string, maxDiff int) (PSFSync, error) {
	var r PSFSync
	if current, _ := git(ctx, dir, "branch", "--show-current"); current != branch {
		return r, fmt.Errorf("la carpeta no está en la rama %s", branch)
	}
	status, err := git(ctx, dir, "status", "--porcelain")
	if err != nil {
		return r, errors.New("no se ha podido leer el estado de la subcarpeta")
	}
	if status != "" {
		if _, err := git(ctx, dir, "add", "-A"); err != nil {
			return r, errors.New("git add ha fallado")
		}
		if _, err := gitSlow(ctx, dir, "commit", "-m", message); err != nil {
			return r, err
		}
		r.Committed = true
	}
	if err := pushBranch(ctx, dir, branch); err != nil {
		return r, err
	}
	span := "origin/" + base + "...HEAD"
	noSpecs := ":(exclude).psf/specs"
	diff, err := git(ctx, dir, "diff", span, "--", ".", noSpecs)
	if err != nil {
		return r, errors.New("no se ha podido calcular la diff")
	}
	r.DiffSize = len(diff)
	r.Diff = diff
	if maxDiff > 0 && len(diff) > maxDiff {
		r.Diff = strings.ToValidUTF8(diff[:maxDiff], "") + fmt.Sprintf("\n… (diff recortada: %d de %d caracteres)", maxDiff, len(diff))
	}
	if f, err := git(ctx, dir, "diff", "--name-only", span, "--", ".", noSpecs); err == nil && f != "" {
		r.Files = strings.Split(f, "\n")
	}
	if n, err := git(ctx, dir, "rev-list", "--count", "origin/"+base+"..HEAD"); err == nil {
		r.Commits, _ = strconv.Atoi(n)
	}
	return r, nil
}

// WritePSFSpec saves the spec in the issue worktree, commits it when it changed and pushes it.
func WritePSFSpec(ctx context.Context, dir, branch string, issue int, text string) error {
	if current, _ := git(ctx, dir, "branch", "--show-current"); current != branch {
		return fmt.Errorf("la carpeta no está en la rama %s", branch)
	}
	rel := PSFSpecPath(issue)
	f := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(f, []byte(strings.TrimSpace(text)+"\n"), 0o644); err != nil {
		return err
	}
	if _, err := git(ctx, dir, "add", "--", rel); err != nil {
		return errors.New("git add de la especificación ha fallado")
	}
	if status, _ := git(ctx, dir, "status", "--porcelain", "--", rel); status != "" {
		if _, err := gitSlow(ctx, dir, "commit", "-m", fmt.Sprintf("docs(issue-%d): PSF specification", issue), "--", rel); err != nil {
			return err
		}
	}
	return pushBranch(ctx, dir, branch)
}

// RemovePSFWorktree removes the worktree of an issue branch once its PR is merged (git worktree remove, never
// forced). Its work is already on the base branch (squash merge), so the commits no remote has any more are not
// lost; it refuses with uncommitted changes or stashes. The local branch is kept (deleting it would need -D).
// It returns the removed folder («» if there was no worktree for the branch).
func RemovePSFWorktree(ctx context.Context, folder string, issue int) (string, error) {
	dir, ok := Copias(ctx, folder)[PSFBranch(issue)]
	if !ok {
		return "", nil
	}
	e, err := EstadoCopia(ctx, dir)
	if err != nil {
		return "", err
	}
	if !e.Worktree {
		return "", ErrNoEsCopia
	}
	if e.Cambios > 0 || e.Stash > 0 {
		return "", fmt.Errorf("la subcarpeta tiene trabajo sin guardar (%s): no se quita", e.Pendiente())
	}
	Prune(ctx, e.Principal)
	if out, err := gitSalida(ctx, e.Principal, "worktree", "remove", "--", e.Carpeta); err != nil {
		return "", fmt.Errorf("git no ha quitado la subcarpeta: %s", out)
	}
	return e.Carpeta, nil
}
