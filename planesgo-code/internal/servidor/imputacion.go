package servidor

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/sesiones"
)

// imputacionEstado: partes abiertos y cerrados hoy por pgocode.
func (s *Servidor) imputacionEstado(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	hoy, ab := s.Imputacion.Estado()
	jsonOK(w, map[string]any{"hoy": hoy, "abiertos": ab})
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
