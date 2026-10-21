// Package repo da la información de git de la carpeta local de un proyecto (tipo de hosting, repositorio,
// rama y ramas) y prepara copias de trabajo por rama (git worktree) para que una sesión trabaje en otra rama
// sin tocar la carpeta del usuario.
package repo

import (
	"context"
	"errors"
	"fmt"
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
	Git         bool     `json:"git"` // la carpeta es (o está dentro de) un repositorio git
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
	i.Git = true
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

// Copias devuelve en qué carpeta está abierta cada rama (git worktree list): la principal y las copias de trabajo.
func Copias(ctx context.Context, carpeta string) map[string]string {
	m := map[string]string{}
	out, err := git(ctx, carpeta, "worktree", "list", "--porcelain")
	if err != nil {
		return m
	}
	dir := ""
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "worktree "):
			dir = strings.TrimPrefix(l, "worktree ")
		case strings.HasPrefix(l, "branch refs/heads/"):
			m[strings.TrimPrefix(l, "branch refs/heads/")] = dir
		}
	}
	return m
}

// PreparaRama deja lista la carpeta de una rama: si ya está abierta allí, nada; si no existe (o está vacía), crea
// una copia de trabajo (git worktree) del repositorio en esa rama (local, remota o nueva desde la actual).
// Nunca cambia de rama una carpeta que ya tenga otra.
func PreparaRama(ctx context.Context, repoDir, rama, carpeta string) error {
	if !ramaValida.MatchString(rama) || strings.Contains(rama, "..") || strings.HasPrefix(rama, "-") {
		return errors.New("nombre de rama no válido")
	}
	if st, err := os.Stat(carpeta); err == nil {
		if !st.IsDir() {
			return errors.New("ya existe un fichero con ese nombre")
		}
		if _, err := git(ctx, carpeta, "rev-parse", "--is-inside-work-tree"); err == nil {
			actual, _ := git(ctx, carpeta, "branch", "--show-current")
			if actual != rama {
				return fmt.Errorf("esa carpeta ya es un repositorio en la rama «%s»", actual)
			}
			return nil
		}
		if ents, _ := os.ReadDir(carpeta); len(ents) > 0 {
			return errors.New("la carpeta no está vacía y no es un repositorio git")
		}
	} else if err := os.MkdirAll(filepath.Dir(carpeta), 0o755); err != nil {
		return err
	}
	if donde, ok := Copias(ctx, repoDir)[rama]; ok {
		return fmt.Errorf("la rama «%s» ya está abierta en %s", rama, donde)
	}
	git(ctx, repoDir, "fetch", "--quiet", "origin", rama)
	if _, err := git(ctx, repoDir, "show-ref", "--verify", "--quiet", "refs/heads/"+rama); err == nil {
		_, err = git(ctx, repoDir, "worktree", "add", "--", carpeta, rama)
		return err
	}
	if _, err := git(ctx, repoDir, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+rama); err == nil {
		_, err = git(ctx, repoDir, "worktree", "add", "--track", "-b", rama, "--", carpeta, "origin/"+rama)
		return err
	}
	_, err := git(ctx, repoDir, "worktree", "add", "-b", rama, "--", carpeta)
	return err
}

// RemotoSSH: un repositorio en formato SSH (git@servidor:organizacion/repositorio.git o ssh://…).
var RemotoSSH = regexp.MustCompile(`^(git@[A-Za-z0-9.-]+:[A-Za-z0-9._/-]+|ssh://[A-Za-z0-9._@-]+(:[0-9]+)?/[A-Za-z0-9._/-]+)$`)

// Clona el repositorio (SSH) en la carpeta, que tiene que no existir o estar vacía (nunca pisa ficheros), y deja
// la rama pedida: la del remoto si existe; si no, una nueva desde la principal. Sin preguntas por terminal: si la
// clave SSH no vale, falla con el mensaje de git.
func Clona(ctx context.Context, remoto, rama, carpeta string) error {
	if !RemotoSSH.MatchString(remoto) || strings.Contains(remoto, "..") {
		return errors.New("el repositorio tiene que ir en formato SSH, p. ej. git@github.com:organizacion/repositorio.git")
	}
	if rama != "" && (!ramaValida.MatchString(rama) || strings.Contains(rama, "..") || strings.HasPrefix(rama, "-")) {
		return errors.New("nombre de rama no válido")
	}
	if st, err := os.Stat(carpeta); err == nil {
		if !st.IsDir() {
			return errors.New("ya existe un fichero con ese nombre")
		}
		if ents, _ := os.ReadDir(carpeta); len(ents) > 0 {
			return errors.New("la carpeta tiene ficheros y no es un repositorio: para clonar, elige una carpeta nueva o vacía")
		}
	} else if err := os.MkdirAll(filepath.Dir(carpeta), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--", remoto, carpeta)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("no se ha podido clonar %s: %s", remoto, strings.TrimSpace(string(out)))
	}
	if rama == "" {
		return nil
	}
	if actual, _ := git(ctx, carpeta, "branch", "--show-current"); actual == rama {
		return nil
	}
	if _, err := git(ctx, carpeta, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+rama); err == nil {
		_, err = git(ctx, carpeta, "checkout", "--track", "-b", rama, "origin/"+rama)
		return err
	}
	_, err := git(ctx, carpeta, "checkout", "-b", rama)
	return err
}
