package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Reparto de una petición en sesiones paralelas: cada «hija» trabaja en su propia copia de trabajo (git worktree),
// en una rama nueva «pgo/<madre>-<n>» creada a partir de la rama actual de la madre. Al acabar, su rama se une
// (git merge --no-ff) a la de la madre y se quita la copia. Nunca se hace push, reset --hard, checkout forzado ni se
// borra una rama con cambios sin unir.

const prefijoRamaHija = "pgo/"

var idMadre = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// RamaHija es el nombre de la rama de la hija n (desde 1) de la sesión madre.
func RamaHija(madre string, n int) (string, error) {
	if !idMadre.MatchString(madre) || n < 1 {
		return "", errors.New("identificador de sesión no válido")
	}
	return fmt.Sprintf("%s%s-%d", prefijoRamaHija, madre, n), nil
}

func ramaDeHijaValida(rama string) bool {
	return strings.HasPrefix(rama, prefijoRamaHija) && ramaValida.MatchString(rama) && !strings.Contains(rama, "..") && !strings.HasSuffix(rama, "/")
}

// RamaActual: la rama de la carpeta (vacía si está en un commit suelto).
func RamaActual(ctx context.Context, dir string) string {
	r, _ := git(ctx, dir, "branch", "--show-current")
	return r
}

// Limpia: la carpeta no tiene ningún cambio ni fichero sin seguimiento ni una unión a medias.
func Limpia(ctx context.Context, dir string) (bool, error) {
	st, err := git(ctx, dir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, errors.New("no se ha podido leer el estado de la subcarpeta")
	}
	if st != "" {
		return false, nil
	}
	if _, err := git(ctx, dir, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
		return false, nil
	}
	return true, nil
}

// identidad: si git no tiene autor configurado, uno propio solo para esa orden (los commits de pgocode no deben
// fallar por eso; no se toca la configuración de nadie).
func identidad(ctx context.Context, dir string) []string {
	n, _ := git(ctx, dir, "config", "user.name")
	e, _ := git(ctx, dir, "config", "user.email")
	var a []string
	if n == "" {
		a = append(a, "-c", "user.name=pgocode")
	}
	if e == "" {
		a = append(a, "-c", "user.email=pgocode@localhost")
	}
	return a
}

// CopiaHija crea la copia de trabajo de una hija: una carpeta nueva con una rama nueva a partir de la rama actual
// de madreDir (sin consultar remotos). La carpeta no puede existir y la rama no puede existir ya. Devuelve la
// copia, con la rama de la madre de la que sale.
func CopiaHija(ctx context.Context, madreDir, rama, carpeta string) (CopiaCreada, string, error) {
	if !ramaDeHijaValida(rama) {
		return CopiaCreada{}, "", errors.New("nombre de rama no válido")
	}
	carpeta = filepath.Clean(carpeta)
	if !filepath.IsAbs(carpeta) {
		return CopiaCreada{}, "", errors.New("la carpeta tiene que ser una ruta completa")
	}
	principal, err := RepoPrincipal(ctx, madreDir)
	if err != nil {
		return CopiaCreada{}, "", err
	}
	ramaMadre := RamaActual(ctx, madreDir)
	if ramaMadre == "" {
		return CopiaCreada{}, "", errors.New("la copia de la sesión no está en ninguna rama (commit suelto): no se puede repartir")
	}
	if _, err := os.Stat(carpeta); err == nil {
		return CopiaCreada{}, "", errors.New("la subcarpeta ya existe")
	}
	if err := os.MkdirAll(filepath.Dir(carpeta), 0o755); err != nil {
		return CopiaCreada{}, "", err
	}
	Prune(ctx, principal)
	if _, err := git(ctx, madreDir, "show-ref", "--verify", "--quiet", "refs/heads/"+rama); err == nil {
		return CopiaCreada{}, "", fmt.Errorf("la rama «%s» ya existe", rama)
	}
	if out, err := gitSalida(ctx, madreDir, "worktree", "add", "-b", rama, "--", carpeta); err != nil {
		return CopiaCreada{}, "", fmt.Errorf("no se ha podido crear la subcarpeta: %s", out)
	}
	return CopiaCreada{Carpeta: carpeta, Rama: rama, Repo: principal, RamaNueva: true}, ramaMadre, nil
}

// Resultado de unir la rama de una hija a la de la madre.
type Union struct {
	Estado   string   `json:"estado"`             // unida, sin_cambios, conflicto, pendiente o error
	Motivo   string   `json:"motivo,omitempty"`   // por qué queda pendiente o ha fallado
	Ficheros []string `json:"ficheros,omitempty"` // los que chocan (conflicto)
	Commit   bool     `json:"commit,omitempty"`   // se hizo un commit automático de lo que la hija dejó sin commitear
	Quitada  bool     `json:"quitada"`            // la copia de la hija se ha quitado
	Rama     bool     `json:"rama_borrada"`       // y su rama también
	Aviso    string   `json:"aviso,omitempty"`    // la copia o la rama se conservan (por qué)
}

// UneHija une la rama de la hija n a la rama actual de la madre (que tiene que seguir siendo ramaMadre) con
// «git merge --no-ff», solo si el árbol de la madre está limpio. Antes, lo que la hija dejó sin commitear se
// guarda en un commit en su rama («pgocode: parte N — <su texto>»). Con conflicto, «git merge --abort»: la madre queda como
// estaba y la rama de la hija se conserva. Si la unión va bien, se quita la copia de la hija (y su rama).
// El que llama garantiza que no hay un agente trabajando en ninguna de las dos carpetas y que no se une a la vez en
// el mismo repositorio.
// parte: el texto de la parte, para los mensajes de los commits (puede ir vacío).
func UneHija(ctx context.Context, madreDir, hijaDir, rama, ramaMadre string, n int, parte string) (Union, error) {
	if !ramaDeHijaValida(rama) {
		return Union{}, errors.New("nombre de rama no válido")
	}
	h, err := EstadoCopia(ctx, hijaDir)
	if err != nil {
		return Union{}, err
	}
	if !h.Worktree {
		return Union{}, ErrNoEsCopia
	}
	if h.Rama != rama {
		return Union{}, fmt.Errorf("la subcarpeta está en la rama «%s», no en «%s»", h.Rama, rama)
	}
	pm, err := RepoPrincipal(ctx, madreDir)
	if err != nil || !mismaCarpeta(pm, h.Principal) {
		return Union{}, errors.New("la subcarpeta de la hija no es del repositorio de la madre")
	}
	var u Union
	// Lo que dejó la hija sin commitear va a su rama (así no se pierde, ni se une a medias).
	if h.Cambios > 0 {
		if out, err := gitSalida(ctx, hijaDir, "add", "-A"); err != nil {
			return Union{}, fmt.Errorf("no se han podido guardar los cambios de la parte %d: %s", n, out)
		}
		args := append(identidad(ctx, hijaDir), "commit", "-q", "-m", MensajeParte(n, parte))
		if out, err := gitSalida(ctx, hijaDir, args...); err != nil {
			return Union{}, fmt.Errorf("no se han podido guardar los cambios de la parte %d: %s", n, out)
		}
		u.Commit = true
	}
	if actual := RamaActual(ctx, madreDir); actual != ramaMadre || ramaMadre == "" {
		u.Estado, u.Motivo = "pendiente", fmt.Sprintf("la copia de la sesión madre ya no está en la rama «%s»", ramaMadre)
		return u, nil
	}
	if ok, err := Limpia(ctx, madreDir); err != nil {
		return u, err
	} else if !ok {
		u.Estado, u.Motivo = "pendiente", "la copia de la sesión madre tiene cambios sin guardar"
		return u, nil
	}
	if _, err := git(ctx, madreDir, "merge-base", "--is-ancestor", rama, "HEAD"); err == nil {
		u.Estado = "sin_cambios" // ya está en la rama de la madre (o no hizo nada)
	} else {
		args := append(identidad(ctx, madreDir), "merge", "--no-ff", "--no-edit", "-m", fmt.Sprintf("pgocode: une la parte %d (%s)", n, rama)+resumenParte(parte), rama)
		if out, err := gitSalida(ctx, madreDir, args...); err != nil {
			fich, _ := git(ctx, madreDir, "diff", "--name-only", "--diff-filter=U")
			if _, e := git(ctx, madreDir, "rev-parse", "-q", "--verify", "MERGE_HEAD"); e == nil {
				if o, e := gitSalida(ctx, madreDir, "merge", "--abort"); e != nil {
					return u, fmt.Errorf("la unión de «%s» ha chocado y no se ha podido deshacer: %s", rama, o)
				}
			}
			if fich == "" {
				u.Estado, u.Motivo = "error", out
				return u, nil
			}
			u.Estado, u.Ficheros = "conflicto", strings.Split(fich, "\n")
			return u, nil
		}
		u.Estado = "unida"
	}
	u.Quitada, u.Rama, err = QuitaUnida(ctx, hijaDir, madreDir)
	if err != nil {
		u.Aviso = err.Error()
	}
	return u, nil
}

// MensajeParte: el mensaje del commit de lo que una parte dejó sin commitear: «pgocode: parte 2 — añade el cupón
// VERANO» (sin su texto, «pgocode: parte 2»).
func MensajeParte(n int, parte string) string {
	return fmt.Sprintf("pgocode: parte %d", n) + resumenParte(parte)
}

// resumenParte: « — <la primera línea de la parte>» recortada, o nada.
func resumenParte(parte string) string {
	l := strings.Join(strings.Fields(strings.SplitN(strings.TrimSpace(parte), "\n", 2)[0]), " ")
	l = strings.TrimRight(l, ".:;, ")
	if l == "" {
		return ""
	}
	if r := []rune(l); len(r) > 72 {
		l = strings.TrimSpace(string(r[:71])) + "…"
	}
	return " — " + l
}

// QuitaUnida quita la copia de trabajo de una hija cuya rama ya está unida a la actual de la madre (git worktree
// remove, sin forzar) y borra su rama (git branch -d, nunca -D). Si la copia tiene cambios o cambios guardados con
// stash, o la rama no está unida, no toca nada y lo dice.
func QuitaUnida(ctx context.Context, hijaDir, madreDir string) (quitada, ramaBorrada bool, err error) {
	h, err := EstadoCopia(ctx, hijaDir)
	if err != nil {
		return false, false, err
	}
	if !h.Worktree {
		return false, false, ErrNoEsCopia
	}
	if h.Cambios > 0 || h.Stash > 0 {
		return false, false, fmt.Errorf("la subcarpeta se conserva: tiene trabajo sin guardar (%s)", h.Pendiente())
	}
	if h.Rama == "" || !ramaDeHijaValida(h.Rama) {
		return false, false, errors.New("la subcarpeta se conserva: no es una rama de reparto")
	}
	if _, err := git(ctx, madreDir, "merge-base", "--is-ancestor", h.Rama, "HEAD"); err != nil {
		return false, false, fmt.Errorf("la subcarpeta se conserva: su rama «%s» no está unida a la de la madre", h.Rama)
	}
	Prune(ctx, h.Principal)
	if out, err := gitSalida(ctx, madreDir, "worktree", "remove", "--", h.Carpeta); err != nil {
		return false, false, fmt.Errorf("la subcarpeta se conserva: git no la ha quitado: %s", out)
	}
	if out, err := gitSalida(ctx, madreDir, "branch", "-d", h.Rama); err != nil {
		return true, false, fmt.Errorf("subcarpeta quitada, pero la rama «%s» se conserva: %s", h.Rama, out)
	}
	return true, true, nil
}
