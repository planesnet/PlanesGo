package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Copias de trabajo (git worktree): su estado y cómo quitarlas sin perder nada. Nunca se fuerza nada: ni
// «worktree remove --force» ni «branch -D».

// Prune olvida las copias de trabajo cuya carpeta ya no existe (git worktree prune).
func Prune(ctx context.Context, repoDir string) error {
	_, err := git(ctx, repoDir, "worktree", "prune")
	return err
}

// Estado de una copia de trabajo.
type Estado struct {
	Carpeta   string `json:"carpeta"`
	Principal string `json:"principal"` // carpeta principal del repositorio
	Rama      string `json:"rama"`      // vacía si está en un commit suelto
	Worktree  bool   `json:"worktree"`  // es una copia de trabajo (no la carpeta principal)
	Cambios   int    `json:"cambios"`   // ficheros con cambios sin commit (incluidos los sin seguimiento)
	Stash     int    `json:"stash"`     // cambios guardados con «git stash» en esta rama
	SinSubir  int    `json:"sin_subir"` // commits que no están en ningún remoto ni en la rama por defecto
	Defecto   string `json:"defecto"`   // rama por defecto del repositorio
	Fusionada bool   `json:"fusionada"` // la rama ya está en la rama por defecto
	// SinCommits: la rama no tiene commits propios (recién creada: su registro solo dice «Created from»). Entonces
	// «fusionada» engaña: no se ha unido nada, no hay nada que unir.
	SinCommits bool `json:"sin_commits,omitempty"`
}

// SePuedeQuitar: es una copia de trabajo y quitarla no pierde nada.
func (e Estado) SePuedeQuitar() bool {
	return e.Worktree && e.Cambios == 0 && e.Stash == 0 && e.SinSubir == 0
}

// Pendiente describe lo que se perdería al quitar la copia (vacío si nada).
func (e Estado) Pendiente() string {
	var l []string
	if e.Cambios > 0 {
		l = append(l, fmt.Sprintf("%d %s con cambios sin commit", e.Cambios, map[bool]string{true: "fichero", false: "ficheros"}[e.Cambios == 1]))
	}
	if e.SinSubir > 0 {
		l = append(l, fmt.Sprintf("%d %s sin subir", e.SinSubir, map[bool]string{true: "commit", false: "commits"}[e.SinSubir == 1]))
	}
	if e.Stash > 0 {
		l = append(l, fmt.Sprintf("%d %s con stash", e.Stash, map[bool]string{true: "cambio guardado", false: "cambios guardados"}[e.Stash == 1]))
	}
	return strings.Join(l, ", ")
}

// ErrNoEsCopia: la carpeta no es una copia de trabajo (git worktree) de un repositorio.
var ErrNoEsCopia = errors.New("esa carpeta no es una subcarpeta de rama (git worktree): no se quita")

// Principal devuelve la carpeta principal del repositorio de dir y si dir es la raíz de una copia de trabajo
// (y no la principal). Error si dir no es la raíz de un repositorio o copia.
func Principal(ctx context.Context, dir string) (principal string, worktree bool, err error) {
	raiz, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false, errors.New("la carpeta no es un repositorio git")
	}
	if !mismaCarpeta(raiz, dir) {
		return "", false, errors.New("la carpeta está dentro de un repositorio, pero no es su raíz")
	}
	comun, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", false, err
	}
	propio, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return "", false, err
	}
	principal = filepath.Dir(comun)
	return principal, !mismaCarpeta(comun, propio) && !mismaCarpeta(principal, raiz), nil
}

// RepoPrincipal devuelve la carpeta principal del repositorio en el que está dir (aunque sea una copia de
// trabajo o una subcarpeta).
func RepoPrincipal(ctx context.Context, dir string) (string, error) {
	comun, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", errors.New("la carpeta no es un repositorio git")
	}
	return filepath.Dir(comun), nil
}

func mismaCarpeta(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// RamaDefecto: la rama por defecto del repositorio (la del HEAD de origin; si no, main o master locales).
func RamaDefecto(ctx context.Context, dir string) string {
	if r, err := git(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && r != "" {
		return strings.TrimPrefix(r, "origin/")
	}
	for _, r := range []string{"main", "master"} {
		if _, err := git(ctx, dir, "show-ref", "--verify", "--quiet", "refs/heads/"+r); err == nil {
			return r
		}
		if _, err := git(ctx, dir, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+r); err == nil {
			return r
		}
	}
	return ""
}

// EstadoCopia mira qué tiene una carpeta de repositorio que se perdería al quitarla.
func EstadoCopia(ctx context.Context, dir string) (Estado, error) {
	e := Estado{Carpeta: filepath.Clean(dir)}
	principal, wt, err := Principal(ctx, dir)
	if err != nil {
		return e, err
	}
	e.Principal, e.Worktree = principal, wt
	e.Rama, _ = git(ctx, dir, "branch", "--show-current")
	st, err := git(ctx, dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return e, errors.New("no se ha podido leer el estado de la subcarpeta")
	}
	if st != "" {
		e.Cambios = len(strings.Split(st, "\n"))
	}
	if e.Rama != "" {
		if l, err := git(ctx, dir, "stash", "list", "--format=%gs"); err == nil && l != "" {
			for _, s := range strings.Split(l, "\n") {
				if strings.HasPrefix(s, "WIP on "+e.Rama+":") || strings.HasPrefix(s, "On "+e.Rama+":") {
					e.Stash++
				}
			}
		}
	}
	e.Defecto = RamaDefecto(ctx, dir)
	// Lo que ya está en la rama por defecto (remota o local) no se pierde.
	var defectos []string
	for _, r := range []string{"refs/remotes/origin/" + e.Defecto, "refs/heads/" + e.Defecto} {
		if e.Defecto == "" {
			break
		}
		if _, err := git(ctx, dir, "show-ref", "--verify", "--quiet", r); err == nil {
			defectos = append(defectos, r)
			if e.Rama != e.Defecto {
				if _, err := git(ctx, dir, "merge-base", "--is-ancestor", "HEAD", r); err == nil {
					e.Fusionada = true
				}
			}
		}
	}
	if e.Rama != "" && e.Rama != e.Defecto {
		e.SinCommits = sinCommitsPropios(ctx, dir, e.Rama)
	}
	args := []string{"rev-list", "--count", "HEAD", "--not"}
	if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "@{u}"); err == nil {
		args = append(args, "@{u}")
	} else {
		args = append(args, "--remotes")
	}
	args = append(args, defectos...)
	if n, err := git(ctx, dir, args...); err == nil {
		e.SinSubir, _ = strconv.Atoi(n)
	} else if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		return e, errors.New("no se han podido contar los commits sin subir")
	}
	return e, nil
}

// sinCommitsPropios: el registro (reflog) de la rama solo tiene su creación: nadie ha hecho commit en ella.
func sinCommitsPropios(ctx context.Context, dir, rama string) bool {
	l, err := git(ctx, dir, "reflog", "show", "--format=%gs", "refs/heads/"+rama, "--")
	if err != nil || strings.TrimSpace(l) == "" {
		return false
	}
	for _, x := range strings.Split(l, "\n") {
		if !strings.HasPrefix(x, "branch: Created from") {
			return false
		}
	}
	return true
}

// QuitaCopia quita una copia de trabajo (git worktree remove, sin forzar) si no tiene nada que se pierda y, si
// borrarRama y la rama ya está fusionada en la de por defecto, también la rama (git branch -d, nunca -D).
// Nunca toca la carpeta principal del repositorio. Devuelve si se ha borrado la rama.
func QuitaCopia(ctx context.Context, dir string, borrarRama bool) (bool, error) {
	e, err := EstadoCopia(ctx, dir)
	if err != nil {
		return false, err
	}
	if !e.Worktree {
		return false, ErrNoEsCopia
	}
	if !e.SePuedeQuitar() {
		return false, fmt.Errorf("la subcarpeta tiene trabajo sin guardar (%s): no se quita", e.Pendiente())
	}
	Prune(ctx, e.Principal)
	if out, err := gitSalida(ctx, e.Principal, "worktree", "remove", "--", e.Carpeta); err != nil {
		return false, fmt.Errorf("git no ha quitado la subcarpeta: %s", out)
	}
	if !borrarRama || !e.Fusionada || e.Rama == "" || e.Rama == e.Defecto || !ramaValida.MatchString(e.Rama) || strings.HasPrefix(e.Rama, "-") {
		return false, nil
	}
	if out, err := gitSalida(ctx, e.Principal, "branch", "-d", e.Rama); err != nil {
		return false, fmt.Errorf("subcarpeta quitada, pero la rama «%s» se conserva: %s", e.Rama, out)
	}
	return true, nil
}

// gitSalida: como git, pero devuelve también el mensaje de error de git.
func gitSalida(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := execGit(ctx, dir, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ListaCopias devuelve las carpetas de las copias de trabajo del repositorio (sin la principal ni las que git
// ya no encuentra).
func ListaCopias(ctx context.Context, repoDir string) []string {
	out, err := git(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	var l []string
	for i, b := range strings.Split(out, "\n\n") {
		dir, prunable := "", false
		for _, x := range strings.Split(b, "\n") {
			if strings.HasPrefix(x, "worktree ") {
				dir = strings.TrimPrefix(x, "worktree ")
			} else if strings.HasPrefix(x, "prunable") {
				prunable = true
			}
		}
		if i == 0 || dir == "" || prunable { // la primera es la principal
			continue
		}
		l = append(l, dir)
	}
	return l
}

// CopiaCreada: una copia de trabajo que ha creado pgocode.
type CopiaCreada struct {
	Carpeta   string    `json:"carpeta"`
	Rama      string    `json:"rama"`
	Repo      string    `json:"repo"`       // carpeta principal del repositorio
	RamaNueva bool      `json:"rama_nueva"` // la rama la creó pgocode
	Sesion    string    `json:"sesion,omitempty"`
	Proyecto  int       `json:"proyecto,omitempty"`
	Fecha     time.Time `json:"fecha"`
}

// Registro de las copias de trabajo que crea pgocode (JSON en el directorio de datos). Sin fichero no guarda nada.
type Registro struct {
	Fichero string
	mu      sync.Mutex
}

func (r *Registro) lee() []CopiaCreada {
	var l []CopiaCreada
	if b, err := os.ReadFile(r.Fichero); err == nil {
		json.Unmarshal(b, &l)
	}
	return l
}

func (r *Registro) escribe(l []CopiaCreada) error {
	sort.Slice(l, func(i, j int) bool { return l[i].Carpeta < l[j].Carpeta })
	b, _ := json.MarshalIndent(l, "", " ")
	if err := os.MkdirAll(filepath.Dir(r.Fichero), 0o755); err != nil {
		return err
	}
	tmp := r.Fichero + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.Fichero)
}

// Apunta una copia creada (sustituye la que hubiera en esa carpeta).
func (r *Registro) Apunta(c CopiaCreada) error {
	if r == nil || r.Fichero == "" {
		return nil
	}
	c.Carpeta = filepath.Clean(c.Carpeta)
	if c.Fecha.IsZero() {
		c.Fecha = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	l := []CopiaCreada{c}
	for _, x := range r.lee() {
		if x.Carpeta != c.Carpeta {
			l = append(l, x)
		}
	}
	return r.escribe(l)
}

// Quita del registro la copia de esa carpeta.
func (r *Registro) Quita(carpeta string) error {
	if r == nil || r.Fichero == "" {
		return nil
	}
	carpeta = filepath.Clean(carpeta)
	r.mu.Lock()
	defer r.mu.Unlock()
	var l []CopiaCreada
	for _, x := range r.lee() {
		if x.Carpeta != carpeta {
			l = append(l, x)
		}
	}
	return r.escribe(l)
}

// Lista las copias registradas.
func (r *Registro) Lista() []CopiaCreada {
	if r == nil || r.Fichero == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lee()
}

// Busca la copia registrada en esa carpeta.
func (r *Registro) Busca(carpeta string) (CopiaCreada, bool) {
	carpeta = filepath.Clean(carpeta)
	for _, x := range r.Lista() {
		if x.Carpeta == carpeta {
			return x, true
		}
	}
	return CopiaCreada{}, false
}
