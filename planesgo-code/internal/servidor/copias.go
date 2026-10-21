package servidor

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// Copias de trabajo (git worktree) de los repositorios del proyecto: las que crea pgocode para las ramas de las
// sesiones, las de los subagentes (.claude/worktrees) y las del usuario. Se pueden quitar solo si no se pierde nada.

// copiaVista: una copia de trabajo con su estado y su origen.
type copiaVista struct {
	repo.Estado
	Origen    string `json:"origen"` // pgocode, subagente o usuario
	Sesion    string `json:"sesion,omitempty"`
	RamaNueva bool   `json:"rama_nueva,omitempty"`
	Quitable  bool   `json:"se_puede_quitar"`
	Pendiente string `json:"pendiente,omitempty"`
	Error     string `json:"error,omitempty"`
}

// reposDelProyecto: las carpetas principales de los repositorios de las carpetas del proyecto.
func (s *Servidor) reposDelProyecto(ctx context.Context, proyecto int) []string {
	var l []string
	for _, c := range append(s.Indice.Carpetas(false)[proyecto], s.Indice.Registradas(proyecto)...) {
		if p, err := repo.RepoPrincipal(ctx, c); err == nil && !slices.Contains(l, p) {
			l = append(l, p)
		}
	}
	return l
}

// origenCopia: quién creó la copia (pgocode si está en su registro; subagente si está en .claude/worktrees).
func (s *Servidor) origenCopia(v *copiaVista) {
	if c, ok := s.RegistroCopias.Busca(v.Carpeta); ok {
		v.Origen, v.Sesion, v.RamaNueva = "pgocode", c.Sesion, c.RamaNueva
		return
	}
	sep := string(filepath.Separator)
	if strings.Contains(v.Carpeta, sep+".claude"+sep+"worktrees"+sep) {
		v.Origen = "subagente"
		return
	}
	v.Origen = "usuario"
}

// proyectoCopias: GET /api/proyecto/copias?proyecto=N.
func (s *Servidor) proyectoCopias(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	copias := []copiaVista{}
	for _, p := range s.reposDelProyecto(r.Context(), id) {
		for _, c := range repo.ListaCopias(r.Context(), p) {
			e, err := repo.EstadoCopia(r.Context(), c)
			v := copiaVista{Estado: e}
			if err != nil {
				v.Carpeta, v.Principal, v.Error = c, p, err.Error()
			} else {
				v.Quitable, v.Pendiente = e.SePuedeQuitar(), e.Pendiente()
			}
			s.origenCopia(&v)
			copias = append(copias, v)
		}
	}
	sort.Slice(copias, func(i, j int) bool { return copias[i].Carpeta < copias[j].Carpeta })
	jsonOK(w, map[string]any{"copias": copias})
}

// proyectoQuitarCopia: POST /api/proyecto/quitar-copia {proyecto, carpeta, borrar_rama}. Solo copias de trabajo
// de los repositorios del proyecto y sin nada que se pierda (si no, 409 con el detalle).
func (s *Servidor) proyectoQuitarCopia(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto   int    `json:"proyecto"`
		Carpeta    string `json:"carpeta"`
		BorrarRama bool   `json:"borrar_rama"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	c := filepath.Clean(p.Carpeta)
	if p.Carpeta == "" || !filepath.IsAbs(c) {
		jsonError(w, http.StatusBadRequest, "la carpeta tiene que ser una ruta completa")
		return
	}
	e, err := repo.EstadoCopia(r.Context(), c)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.Indice.Pertenece(p.Proyecto, c) && !slices.Contains(s.reposDelProyecto(r.Context(), p.Proyecto), e.Principal) {
		jsonError(w, http.StatusForbidden, "esa copia no es de un repositorio del proyecto")
		return
	}
	if !e.Worktree {
		jsonError(w, http.StatusBadRequest, repo.ErrNoEsCopia.Error())
		return
	}
	if !e.SePuedeQuitar() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": "la copia tiene trabajo sin guardar (" + e.Pendiente() + "): no se quita", "estado": e})
		return
	}
	borrada, err := repo.QuitaCopia(r.Context(), c, p.BorrarRama)
	if _, st := os.Stat(c); err != nil && !errors.Is(st, os.ErrNotExist) {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	s.RegistroCopias.Quita(c)
	s.Indice.Olvida(p.Proyecto, c)
	res := map[string]any{"ok": true, "rama_borrada": borrada}
	if err != nil { // copia quitada, pero la rama se conserva
		res["aviso"] = err.Error()
	}
	log.Printf("copia de trabajo quitada: %s (rama %s, borrada: %v)", c, e.Rama, borrada)
	jsonOK(w, res)
}

// PodaCopias: git worktree prune en los repositorios de las copias registradas (al arrancar, en segundo plano).
func (s *Servidor) PodaCopias(ctx context.Context) {
	hechos := map[string]bool{}
	for _, c := range s.RegistroCopias.Lista() {
		if c.Repo != "" && !hechos[c.Repo] {
			hechos[c.Repo] = true
			repo.Prune(ctx, c.Repo)
		}
	}
}

// apuntaCopia guarda en el registro una copia de trabajo creada por pgocode.
func (s *Servidor) apuntaCopia(c repo.CopiaCreada) {
	if c.Repo == "" {
		return
	}
	if err := s.RegistroCopias.Apunta(c); err != nil {
		log.Printf("registro de copias de trabajo: %v", err)
	}
}
