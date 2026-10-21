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

	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/sesiones"
)

// Los agentes que lanza pgocode llevan PLANESGO_GESTOR=pgocode y PlanesGo no imputa en ellos: lo hace pgocode.
// Las sesiones de los agentes fuera de pgocode (en el mismo equipo) imputan por su cuenta, salvo que se marque
// este interruptor opcional: con ~/.planesgo/gestor = «pgocode» PlanesGo se duerme también en ellas y se cierran
// los partes que tuvieran abiertos.

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
	// Desactivado se guarda como «agentes» (no se borra): así pgocode no lo vuelve a activar al arrancar.
	valor := "agentes\n"
	if pgocode {
		valor = "pgocode\n"
	}
	if err := os.MkdirAll(filepath.Dir(ficheroGestor()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(ficheroGestor(), []byte(valor), 0o644); err != nil {
		return err
	}
	if pgocode {
		go cierraPartesAgentes()
	}
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
	hoy, ab := s.Imputacion.Estado()
	jsonOK(w, map[string]any{"hoy": hoy, "abiertos": ab, "gestor_pgocode": GestorPgocode()})
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

// imputacionValidar: valida el registro de tiempos de un día (hoy si no se dice).
func (s *Servidor) imputacionValidar(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Imputacion == nil || s.Imputacion.Tiempos == nil {
		jsonError(w, http.StatusServiceUnavailable, "sin registro de tiempos")
		return
	}
	hoy := time.Now().Format("2006-01-02")
	dia := r.URL.Query().Get("dia")
	if dia == "" {
		dia = hoy
	}
	if _, err := time.Parse("2006-01-02", dia); err != nil {
		jsonError(w, http.StatusBadRequest, "día no válido (AAAA-MM-DD)")
		return
	}
	f, err := os.Open(s.Imputacion.Tiempos.Fichero(dia))
	if err != nil {
		jsonOK(w, map[string]any{"dia": dia, "fichero": s.Imputacion.Tiempos.Fichero(dia), "sucesos": 0, "avisos": []any{}, "lineas": []any{}, "sesiones": []any{}})
		return
	}
	defer f.Close()
	inf, err := imputacion.Valida(f, dia == hoy)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"dia": dia, "fichero": f.Name(), "sucesos": inf.Sucesos, "errores": inf.Errores, "avisos": inf.Avisos,
		"lineas": inf.Lineas, "sesiones": inf.Sesiones})
}

// imputacionUnificar: suma a las líneas del día las líneas sueltas de pgocode de menos de 5 minutos y las borra.
func (s *Servidor) imputacionUnificar(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	res, err := s.Imputacion.UnificaSueltas(r.Context(), ses.UID, ses.OdooSesion)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.Eventos.Emite(map[string]string{"tipo": "imputacion"})
	jsonOK(w, res)
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
