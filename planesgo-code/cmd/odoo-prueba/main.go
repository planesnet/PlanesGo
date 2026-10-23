// odoo-prueba: un Odoo simulado para probar pgocode de punta a punta sin un Odoo real (ciclos de prueba con la
// interfaz y los agentes de verdad). Acepta cualquier usuario y contraseña, ofrece un proyecto de prueba y guarda
// en memoria las tareas y las líneas de horas que imputa pgocode (se ven en /lineas). Como el de verdad, call_kw
// exige la cookie de una sesión abierta (aleatoria en cada entrada): sin ella, «sesión caducada».
//
//	go run ./cmd/odoo-prueba -addr 127.0.0.1:8611 -proyecto 999
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

var (
	mu      sync.Mutex
	sig     = 100
	tareas  = map[int]map[string]any{}
	lineas  = map[int]map[string]any{}
	proyID  int
	proyNom string
	// sesiones abiertas con /web/session/authenticate
	sesiones = map[string]bool{}
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8611", "dirección de escucha")
	flag.IntVar(&proyID, "proyecto", 999, "id del proyecto de prueba")
	flag.StringVar(&proyNom, "nombre", "PRUEBA PGOCODE", "nombre del proyecto de prueba")
	flag.Parse()
	http.HandleFunc("/web/session/authenticate", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Params struct {
				Login string `json:"login"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		b := make([]byte, 16)
		rand.Read(b)
		ses := hex.EncodeToString(b)
		mu.Lock()
		sesiones[ses] = true
		mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: ses, Path: "/"})
		responde(w, map[string]any{"uid": 2, "name": "Usuario de prueba", "username": p.Params.Login})
	})
	http.HandleFunc("/web/session/destroy", func(w http.ResponseWriter, r *http.Request) { responde(w, true) })
	http.HandleFunc("/web/image/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	http.HandleFunc("/web/dataset/call_kw/", llamada)
	http.HandleFunc("/lineas", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"tareas": tareas, "lineas": lineas})
	})
	log.Printf("Odoo de prueba en http://%s (proyecto %d «%s»)", *addr, proyID, proyNom)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func responde(w http.ResponseWriter, res any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "result": res})
}

func llamada(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Params struct {
			Model  string         `json:"model"`
			Method string         `json:"method"`
			Args   []any          `json:"args"`
			Kwargs map[string]any `json:"kwargs"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m, metodo, args := p.Params.Model, p.Params.Method, p.Params.Args
	mu.Lock()
	defer mu.Unlock()
	if c, err := r.Cookie("session_id"); err != nil || !sesiones[c.Value] {
		log.Printf("rechazado sin sesión: %s.%s", m, metodo)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": 100,
			"message": "Odoo Session Expired", "data": map[string]any{"name": "odoo.http.SessionExpiredException", "message": "Session expired"}}})
		return
	}
	switch {
	case m == "project.project" && metodo == "search_read":
		// El de prueba y otro más (para probar el cambio de proyecto).
		l := []any{map[string]any{"id": proyID, "name": proyNom, "partner_id": []any{1, "Cliente de prueba"}, "user_id": []any{2, "Usuario de prueba"}},
			map[string]any{"id": proyID + 1, "name": "PRUEBA OTRO", "partner_id": []any{1, "Cliente de prueba"}, "user_id": []any{2, "Usuario de prueba"}}}
		if id := idDominio(args); id > 0 {
			for _, x := range l {
				if x.(map[string]any)["id"] == id {
					l = []any{x}
				}
			}
		}
		responde(w, l)
	case m == "project.task" && metodo == "search_read":
		l := []any{}
		for id, t := range tareas {
			if coincide(t, args) {
				l = append(l, conID(id, t))
			}
		}
		responde(w, l)
	case metodo == "create":
		vals, _ := args[0].(map[string]any)
		sig++
		if m == "project.task" {
			tareas[sig] = vals
		} else {
			lineas[sig] = vals
		}
		responde(w, sig)
	case m == "account.analytic.line" && metodo == "write":
		for _, id := range ids(args) {
			if l := lineas[id]; l != nil {
				vals, _ := args[1].(map[string]any)
				for k, v := range vals {
					l[k] = v
				}
			}
		}
		responde(w, true)
	case m == "account.analytic.line" && metodo == "search_read":
		l := []any{}
		for id, x := range lineas {
			if coincide(x, args) {
				l = append(l, conID(id, x))
			}
		}
		responde(w, l)
	case m == "account.analytic.line" && metodo == "unlink":
		for _, id := range ids(args) {
			delete(lineas, id)
		}
		responde(w, true)
	case metodo == "fields_get":
		responde(w, map[string]any{"name": map[string]any{}, "unit_amount": map[string]any{}, "ai_model": map[string]any{},
			"ai_tokens_input": map[string]any{}, "ai_tokens_output": map[string]any{}, "ai_tokens_total": map[string]any{}, "ai_cost": map[string]any{}})
	default:
		log.Printf("sin simular: %s.%s", m, metodo)
		responde(w, []any{})
	}
}

func conID(id int, x map[string]any) map[string]any {
	c := map[string]any{"id": id}
	for k, v := range x {
		c[k] = v
	}
	return c
}

func ids(args []any) []int {
	var l []int
	if len(args) == 0 {
		return l
	}
	switch v := args[0].(type) {
	case []any:
		for _, x := range v {
			if f, ok := x.(float64); ok {
				l = append(l, int(f))
			}
		}
	case float64:
		l = append(l, int(v))
	}
	return l
}

// coincide: un dominio sencillo de Odoo (condiciones «=» y «=like», unidas por «y»; el resto se ignora).
func coincide(x map[string]any, args []any) bool {
	if len(args) == 0 {
		return true
	}
	dominio, _ := args[0].([]any)
	for _, c := range dominio {
		t, ok := c.([]any)
		if !ok || len(t) != 3 {
			continue
		}
		campo, _ := t[0].(string)
		op, _ := t[1].(string)
		v := fmt.Sprint(x[campo])
		if l, ok := x[campo].([]any); ok && len(l) > 0 {
			v = fmt.Sprint(l[0])
		}
		quiere := fmt.Sprint(t[2])
		switch op {
		case "=":
			if v != quiere && !(quiere == "true" && v == "<nil>") {
				return false
			}
		case "=like":
			if !strings.HasPrefix(v, strings.TrimSuffix(quiere, "%")) {
				return false
			}
		}
	}
	return true
}

// idDominio: el id de un dominio con («id», «=», N), si lo lleva.
func idDominio(args []any) int {
	if len(args) == 0 {
		return 0
	}
	d, _ := args[0].([]any)
	for _, c := range d {
		if t, ok := c.([]any); ok && len(t) == 3 && t[0] == "id" {
			if f, ok := t[2].(float64); ok {
				return int(f)
			}
		}
	}
	return 0
}
