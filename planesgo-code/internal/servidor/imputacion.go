package servidor

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"planesgo-code/internal/sesiones"
)

// Interruptor «PlanesGo en los agentes»: con ~/.planesgo/gestor = «pgocode», PlanesGo (hooks, plugins y
// planesgo-mcp) se queda dormido también en las sesiones que los agentes ejecutan por su cuenta; la imputación
// la hace pgocode. Al activarlo se cierran los partes que esas sesiones tuvieran abiertos.

func ficheroGestor() string {
	casa, _ := os.UserHomeDir()
	return filepath.Join(casa, ".planesgo", "gestor")
}

// GestorPgocode dice si pgocode lleva la imputación.
func GestorPgocode() bool {
	b, err := os.ReadFile(ficheroGestor())
	return err == nil && strings.TrimSpace(string(b)) == "pgocode"
}

// PonGestor activa (pgocode lleva la imputación) o desactiva el interruptor.
func PonGestor(pgocode bool) error {
	if !pgocode {
		err := os.Remove(ficheroGestor())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ficheroGestor()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(ficheroGestor(), []byte("pgocode\n"), 0o644); err != nil {
		return err
	}
	go cierraPartesAgentes()
	return nil
}

// cierraPartesAgentes: el hook de PlanesGo cierra e imputa los partes abiertos por las sesiones de los agentes.
func cierraPartesAgentes() {
	casa, _ := os.UserHomeDir()
	hook := filepath.Join(casa, ".claude", "hooks", "planesgo_claude_hook.py")
	if _, err := os.Stat(hook); err != nil {
		return
	}
	ctx, cancela := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancela()
	exec.CommandContext(ctx, "python3", hook, "cierra-partes").Run()
}

// imputacionEstado: partes abiertos y cerrados hoy por pgocode, y el interruptor.
func (s *Servidor) imputacionEstado(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	ab, ce := s.Imputacion.Estado()
	jsonOK(w, map[string]any{"abiertos": ab, "cerrados": ce, "gestor_pgocode": GestorPgocode()})
}

func (s *Servidor) imputacionGestor(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Pgocode bool `json:"pgocode"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if err := PonGestor(p.Pgocode); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Eventos.Emite(map[string]string{"tipo": "imputacion"})
	jsonOK(w, map[string]bool{"gestor_pgocode": GestorPgocode()})
}

// proyectoTareas: tareas abiertas del proyecto en Odoo (para elegir a cuál imputa una sesión).
func (s *Servidor) proyectoTareas(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	var l []struct {
		ID     int    `json:"id"`
		Nombre string `json:"name"`
	}
	dominio := []any{[]any{"project_id", "=", id}}
	if err := s.Odoo.CallKW(r.Context(), ses.OdooSesion, "project.task", "search_read", []any{dominio},
		map[string]any{"fields": []string{"id", "name"}, "limit": 300, "order": "write_date desc"}, &l); err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(l))
	for _, t := range l {
		out = append(out, map[string]any{"id": t.ID, "nombre": t.Nombre})
	}
	jsonOK(w, out)
}
