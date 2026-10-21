package servidor

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/sesiones"
)

// Resumen de imputación de un proyecto y una tarea para el marcador de la barra, leído de Odoo (todas las líneas
// del usuario: pgocode, PlanesGo o puestas a mano). La interfaz le suma lo pendiente del cronómetro (lo que pgocode
// lleva y aún no ha escrito), así que el marcador avanza al segundo.
type resumenImputacion struct {
	TareaID   int     `json:"tarea_id"`
	HoyH      float64 `json:"hoy_h"` // hoy en la tarea (Odoo)
	HoyM      float64 `json:"hoy_m"`
	TotalH    float64 `json:"total_h"` // total en el proyecto (Odoo)
	TotalM    float64 `json:"total_m"`
	Calculado string  `json:"calculado"`
}

var (
	muResumen    sync.Mutex
	cacheResumen = map[string]struct {
		r  resumenImputacion
		en time.Time
	}{}
)

func (s *Servidor) imputacionResumen(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	q := r.URL.Query()
	proyecto, _ := strconv.Atoi(q.Get("proyecto"))
	tareaID, _ := strconv.Atoi(q.Get("tarea_id"))
	tarea := strings.TrimSpace(q.Get("tarea"))
	if tarea == "" || tarea == "pgocode - Sesión IA" {
		tarea = "Desarrollo"
	}
	if _, ok := s.proyecto(w, r, ses, proyecto); !ok {
		return
	}
	clave := fmt.Sprintf("%d|%d|%d|%s", ses.UID, proyecto, tareaID, tarea)
	muResumen.Lock()
	if c, ok := cacheResumen[clave]; ok && time.Since(c.en) < time.Minute {
		muResumen.Unlock()
		jsonOK(w, c.r)
		return
	}
	muResumen.Unlock()
	ctx, cancela := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancela()
	res := resumenImputacion{TareaID: tareaID, Calculado: time.Now().Format(time.RFC3339)}
	// La tarea, si no se sabe su id (sin crearla).
	if res.TareaID == 0 {
		var l []struct {
			ID int `json:"id"`
		}
		dom := []any{[]any{"project_id", "=", proyecto}, []any{"name", "=", tarea}}
		if s.Odoo.CallKW(ctx, ses.OdooSesion, "project.task", "search_read", []any{dom}, map[string]any{"fields": []string{"id"}, "limit": 1}, &l) == nil && len(l) > 0 {
			res.TareaID = l[0].ID
		}
	}
	type linea struct {
		Horas  float64 `json:"unit_amount"`
		Modelo any     `json:"ai_model"`
		Marca  any     `json:"ai_session_id"`
		Fecha  string  `json:"date"`
		Task   any     `json:"task_id"`
	}
	campos := []string{"unit_amount", "ai_model", "ai_session_id", "date", "task_id"}
	var l []linea
	dom := []any{[]any{"user_id", "=", ses.UID}, []any{"project_id", "=", proyecto}}
	err := s.Odoo.CallKW(ctx, ses.OdooSesion, "account.analytic.line", "search_read", []any{dom}, map[string]any{"fields": campos}, &l)
	if err != nil { // Odoo sin los campos de IA: solo horas
		var l2 []struct {
			Horas float64 `json:"unit_amount"`
			Fecha string  `json:"date"`
			Task  any     `json:"task_id"`
		}
		if err2 := s.Odoo.CallKW(ctx, ses.OdooSesion, "account.analytic.line", "search_read", []any{dom},
			map[string]any{"fields": []string{"unit_amount", "date", "task_id"}}, &l2); err2 != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		for _, x := range l2 {
			l = append(l, linea{Horas: x.Horas, Fecha: x.Fecha, Task: x.Task})
		}
	}
	hoy := time.Now().Format("2006-01-02")
	for _, x := range l {
		modelo, _ := x.Modelo.(string)
		maquina := strings.TrimSpace(modelo) != ""
		if maquina {
			res.TotalM += x.Horas
		} else {
			res.TotalH += x.Horas
		}
		if x.Fecha == hoy {
			if t, ok := x.Task.([]any); ok && len(t) > 0 {
				if id, _ := t[0].(float64); int(id) == res.TareaID && res.TareaID > 0 {
					if maquina {
						res.HoyM += x.Horas
					} else {
						res.HoyH += x.Horas
					}
				}
			}
		}
	}
	muResumen.Lock()
	cacheResumen[clave] = struct {
		r  resumenImputacion
		en time.Time
	}{res, time.Now()}
	muResumen.Unlock()
	jsonOK(w, res)
}
