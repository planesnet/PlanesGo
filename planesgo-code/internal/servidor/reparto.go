package servidor

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// Reparto en sesiones paralelas: cuando una petición lista varias partes independientes (llm.ParallelParts),
// la interfaz propone repartirlas en sesiones hijas, cada una en su propia copia de trabajo (git worktree) y en una
// rama «pgo/<madre>-<n>» creada desde la rama de la sesión madre. El servidor decide si procede (/propone), crea
// cada copia (/prepara) y une la rama de cada hija a la de la madre al acabar (/une). La interfaz lleva las sesiones
// (cada hija es una sesión de pgocode normal, con su turno y su imputación). Nada de esto llama a una IA de pago.

const (
	// maxHijas: cuántas hijas trabajan a la vez (las demás esperan en cola).
	maxHijas = 4
	// esperaReparto: segundos que el usuario tiene para decidir antes de que se reparta solo.
	esperaReparto = 8
)

// ocupadas: carpetas en las que hay ahora un agente trabajando (un chat en curso), con cuántos.
type ocupadas struct {
	mu sync.Mutex
	n  map[string]int
}

func (o *ocupadas) entra(c string) func() {
	c = filepath.Clean(c)
	o.mu.Lock()
	if o.n == nil {
		o.n = map[string]int{}
	}
	o.n[c]++
	o.mu.Unlock()
	return func() {
		o.mu.Lock()
		if o.n[c]--; o.n[c] <= 0 {
			delete(o.n, c)
		}
		o.mu.Unlock()
	}
}

func (o *ocupadas) hay(c string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.n[filepath.Clean(c)] > 0
}

// carpetaOcupada: hay un agente trabajando en esa carpeta.
func (s *Servidor) carpetaOcupada(c string) bool { return s.trabajando.hay(c) }

// bloqueoRepo: un candado por repositorio: las copias se crean y las ramas se unen de una en una.
func (s *Servidor) bloqueoRepo(principal string) *sync.Mutex {
	m, _ := s.bloqueosRepo.LoadOrStore(filepath.Clean(principal), &sync.Mutex{})
	return m.(*sync.Mutex)
}

// partesDe: las partes independientes de una petición (nil: no se reparte). Sustituible en las pruebas.
func (s *Servidor) partesDe(ctx context.Context, texto string) *llm.Request {
	if s.Partes != nil {
		return s.Partes(ctx, texto)
	}
	return llm.ParallelParts(ctx, texto)
}

// propuestaReparto: lo que contesta /api/reparto/propone.
type propuestaReparto struct {
	Partes     []string `json:"partes,omitempty"` // vacío: no se reparte
	Intro      string   `json:"intro,omitempty"`
	Outro      string   `json:"outro,omitempty"`
	RamaMadre  string   `json:"rama_madre,omitempty"`
	MadreSucia bool     `json:"madre_sucia,omitempty"` // las hijas parten de lo ya guardado: la unión esperará a que se guarde
	Max        int      `json:"max"`
	EsperaS    int      `json:"espera_s"`
	Motivo     string   `json:"motivo,omitempty"` // por qué no se reparte (para depurar)
}

// repartoPropone: POST /api/reparto/propone {proyecto, carpeta, texto, permisos}. Solo propone con una carpeta
// que sea la raíz de un repositorio git (o de una copia) en una rama, una sesión que edita y una petición con tres
// o más partes independientes.
func (s *Servidor) repartoPropone(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Texto    string `json:"texto"`
		Permisos string `json:"permisos"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	no := func(motivo string) {
		jsonOK(w, propuestaReparto{Max: maxHijas, EsperaS: esperaReparto, Motivo: motivo})
	}
	switch {
	case os.Getenv("PGOCODE_PARALELO") == "0":
		no("desactivado")
		return
	case p.Permisos != "editar" && p.Permisos != "auto":
		no("la sesión no edita")
		return
	case !filepath.IsAbs(p.Carpeta) || !s.Indice.Pertenece(p.Proyecto, p.Carpeta):
		no("carpeta que no es del proyecto")
		return
	}
	if _, _, err := repo.Principal(r.Context(), p.Carpeta); err != nil {
		no("la carpeta no es la raíz de un repositorio")
		return
	}
	rama := repo.RamaActual(r.Context(), p.Carpeta)
	if rama == "" {
		no("sin rama")
		return
	}
	partes := s.partesDe(r.Context(), p.Texto)
	if partes == nil || len(partes.Items) < 3 {
		no("no son partes independientes")
		return
	}
	limpia, _ := repo.Limpia(r.Context(), p.Carpeta)
	jsonOK(w, propuestaReparto{Partes: partes.Items, Intro: partes.Intro, Outro: partes.Outro, RamaMadre: rama, MadreSucia: !limpia, Max: maxHijas, EsperaS: esperaReparto})
}

// repartoPrepara: POST /api/reparto/prepara {proyecto, sesion, carpeta, n, hija}: crea la copia de trabajo de la
// hija n de la sesión madre (rama nueva «pgo/<madre>-<n>» desde la rama actual de la madre) y la registra. Repetirlo
// devuelve la misma copia.
func (s *Servidor) repartoPrepara(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Sesion   string `json:"sesion"`
		Carpeta  string `json:"carpeta"`
		N        int    `json:"n"`
		Hija     string `json:"hija"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if !filepath.IsAbs(p.Carpeta) || !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusForbidden, "esa carpeta no es del proyecto")
		return
	}
	rama, err := repo.RamaHija(p.Sesion, p.N)
	if err != nil || p.N > 99 {
		jsonError(w, http.StatusBadRequest, "sesión o número de parte no válidos")
		return
	}
	principal, err := repo.RepoPrincipal(r.Context(), p.Carpeta)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.DirRamas == "" {
		jsonError(w, http.StatusInternalServerError, "no hay carpeta para las copias de trabajo")
		return
	}
	dest := filepath.Join(s.DirRamas, filepath.Base(principal), strings.ReplaceAll(rama, "/", "_"))
	mu := s.bloqueoRepo(principal)
	mu.Lock()
	defer mu.Unlock()
	var c repo.CopiaCreada
	ramaMadre := repo.RamaActual(r.Context(), p.Carpeta)
	if e, err := repo.EstadoCopia(r.Context(), dest); err == nil && e.Worktree && e.Rama == rama { // ya estaba
		c = repo.CopiaCreada{Carpeta: dest, Rama: rama, Repo: principal, RamaNueva: true}
	} else if c, ramaMadre, err = repo.CopiaHija(r.Context(), p.Carpeta, rama, dest); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	c.Sesion, c.Proyecto = p.Hija, p.Proyecto
	s.apuntaCopia(c)
	if err := s.Indice.Registra(p.Proyecto, c.Carpeta); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("reparto: copia de la parte %d de %s en %s (rama %s, desde %s)", p.N, p.Sesion, c.Carpeta, c.Rama, ramaMadre)
	jsonOK(w, map[string]any{"carpeta": c.Carpeta, "rama": c.Rama, "rama_madre": ramaMadre})
}

// repartoUne: POST /api/reparto/une {proyecto, madre, rama_madre, hija, rama, n}: une la rama de la hija a la de la
// madre (repo.UneHija) si no hay un agente trabajando en ninguna de las dos copias; si no, queda pendiente.
func (s *Servidor) repartoUne(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto  int    `json:"proyecto"`
		Madre     string `json:"madre"`
		RamaMadre string `json:"rama_madre"`
		Hija      string `json:"hija"`
		Rama      string `json:"rama"`
		N         int    `json:"n"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	madre, hija := filepath.Clean(p.Madre), filepath.Clean(p.Hija)
	if !filepath.IsAbs(madre) || !filepath.IsAbs(hija) || !s.Indice.Pertenece(p.Proyecto, madre) {
		jsonError(w, http.StatusForbidden, "esa carpeta no es del proyecto")
		return
	}
	// Solo copias de reparto que pgocode ha creado para este proyecto.
	if c, ok := s.RegistroCopias.Busca(hija); !ok || c.Rama != p.Rama || c.Proyecto != p.Proyecto {
		jsonError(w, http.StatusForbidden, "esa copia no es de un reparto de pgocode")
		return
	}
	u, err := s.uneHija(r.Context(), p.Proyecto, madre, p.RamaMadre, hija, p.Rama, p.N)
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	jsonOK(w, u)
}

// uneHija: la unión de una hija, de una en una por repositorio y solo sin agentes trabajando en las dos copias.
func (s *Servidor) uneHija(ctx context.Context, proyecto int, madre, ramaMadre, hija, rama string, n int) (repo.Union, error) {
	principal, err := repo.RepoPrincipal(ctx, madre)
	if err != nil {
		return repo.Union{}, err
	}
	mu := s.bloqueoRepo(principal)
	mu.Lock()
	defer mu.Unlock()
	if s.carpetaOcupada(hija) {
		return repo.Union{Estado: "pendiente", Motivo: "la parte sigue trabajando"}, nil
	}
	if s.carpetaOcupada(madre) {
		return repo.Union{Estado: "pendiente", Motivo: "hay un agente trabajando en la copia de la sesión madre"}, nil
	}
	u, err := repo.UneHija(ctx, madre, hija, rama, ramaMadre, n)
	if err != nil {
		return u, err
	}
	if u.Quitada {
		s.RegistroCopias.Quita(hija)
		s.Indice.Olvida(proyecto, hija)
	}
	log.Printf("reparto: parte %d (%s): %s%s", n, rama, u.Estado, map[bool]string{true: " · copia quitada", false: ""}[u.Quitada])
	return u, nil
}
