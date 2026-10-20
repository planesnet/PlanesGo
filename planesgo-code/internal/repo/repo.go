// Package repo da la información de git de la carpeta local de un proyecto (tipo de hosting, repositorio,
// rama y ramas) y prepara copias de trabajo por rama (git worktree) para que una sesión trabaje en otra rama
// sin tocar la carpeta del usuario.
package repo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Tipos de proyecto (hosting).
const (
	Local    = "local"
	OdooSH   = "odoo.sh"
	AutoPyme = "autopyme.com"
)

// Info describe la carpeta local de un proyecto.
type Info struct {
	Tipo        string   `json:"tipo"`
	Repositorio string   `json:"repositorio,omitempty"` // owner/repo (o el nombre de la carpeta si no hay remoto)
	Remoto      string   `json:"remoto,omitempty"`
	Carpeta     string   `json:"carpeta"`
	Rama        string   `json:"rama,omitempty"`
	Ramas       []string `json:"ramas"`
}

var ramaValida = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Lee la información de git de la carpeta (si no es un repositorio, solo carpeta y tipo local).
func Lee(ctx context.Context, carpeta string) Info {
	i := Info{Tipo: Local, Carpeta: carpeta, Ramas: []string{}}
	if _, err := git(ctx, carpeta, "rev-parse", "--is-inside-work-tree"); err != nil {
		return i
	}
	i.Rama, _ = git(ctx, carpeta, "branch", "--show-current")
	if r, err := git(ctx, carpeta, "remote", "get-url", "origin"); err == nil && r != "" {
		i.Remoto = r
		i.Repositorio = nombreRepositorio(r)
	} else if raiz, err := git(ctx, carpeta, "rev-parse", "--show-toplevel"); err == nil {
		i.Repositorio = filepath.Base(raiz)
	}
	vistas := map[string]bool{}
	if refs, err := git(ctx, carpeta, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes/origin"); err == nil {
		for _, r := range strings.Split(refs, "\n") {
			r = strings.TrimPrefix(strings.TrimSpace(r), "origin/")
			if r != "" && r != "HEAD" && r != "origin" && !vistas[r] {
				vistas[r] = true
				i.Ramas = append(i.Ramas, r)
			}
		}
	}
	sort.Strings(i.Ramas)
	i.Tipo = tipo(carpeta)
	return i
}

// nombreRepositorio: git@github.com:planesnet/madpla.git o https://github.com/planesnet/madpla -> planesnet/madpla.
func nombreRepositorio(remoto string) string {
	r := strings.TrimSuffix(strings.TrimRight(remoto, "/"), ".git")
	if i := strings.Index(r, "://"); i >= 0 {
		r = r[i+3:]
		if j := strings.Index(r, "/"); j >= 0 {
			r = r[j+1:]
		}
	} else if i := strings.Index(r, ":"); i >= 0 {
		r = r[i+1:]
	}
	return r
}

// tipo deduce el hosting del perfil PSF del proyecto (.psf/proyecto.yml): la URL de sus entornos.
func tipo(carpeta string) string {
	b, err := os.ReadFile(filepath.Join(carpeta, ".psf", "proyecto.yml"))
	if err != nil {
		return Local
	}
	s := strings.ToLower(string(b))
	switch {
	case strings.Contains(s, "odoo.sh") || strings.Contains(s, ".odoo.com"):
		return OdooSH
	case strings.Contains(s, "autopyme"):
		return AutoPyme
	}
	return Local
}

// CarpetaDeRama devuelve dónde trabajar en una rama: la propia carpeta si ya está en ella o, si no, una copia
// de trabajo (git worktree) en base/<repositorio>/<rama>, creada si hace falta. La carpeta del usuario no se toca.
func CarpetaDeRama(ctx context.Context, carpeta, rama, base string) (string, error) {
	if rama == "" {
		return carpeta, nil
	}
	if !ramaValida.MatchString(rama) || strings.Contains(rama, "..") || strings.HasPrefix(rama, "-") {
		return "", errors.New("nombre de rama no válido")
	}
	actual, _ := git(ctx, carpeta, "branch", "--show-current")
	if actual == rama {
		return carpeta, nil
	}
	raiz, err := git(ctx, carpeta, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("la carpeta no es un repositorio git")
	}
	dir := filepath.Join(base, filepath.Base(raiz), strings.ReplaceAll(rama, "/", "_"))
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	// Rama local, o rama remota con seguimiento.
	if _, err := git(ctx, raiz, "show-ref", "--verify", "--quiet", "refs/heads/"+rama); err == nil {
		_, err = git(ctx, raiz, "worktree", "add", "--", dir, rama)
		if err != nil {
			return "", errors.New("no se pudo preparar la copia de la rama " + rama + " (¿está abierta en otra copia de trabajo?)")
		}
		return dir, nil
	}
	if _, err := git(ctx, raiz, "worktree", "add", "--track", "-b", rama, "--", dir, "origin/"+rama); err != nil {
		return "", errors.New("no se pudo preparar la copia de la rama " + rama)
	}
	return dir, nil
}
