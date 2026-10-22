package imputacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// odooMemoria: un Odoo en memoria con tareas y líneas de parte, y dominios como los de Odoo (=, =like, in, >=, |).
type odooMemoria struct {
	mu     sync.Mutex
	sig    int
	tareas map[int]map[string]any
	lineas map[int]map[string]any
}

func nuevoOdooMemoria() *odooMemoria {
	return &odooMemoria{sig: 1000, tareas: map[int]map[string]any{}, lineas: map[int]map[string]any{}}
}

// cumple: el registro cumple el dominio (notación polaca de Odoo: «|» y «&» delante de sus dos operandos).
func cumple(v map[string]any, dom []any) bool {
	var eval func(i int) (bool, int)
	eval = func(i int) (bool, int) {
		switch c := dom[i].(type) {
		case string: // operador
			a, j := eval(i + 1)
			b, k := eval(j)
			if c == "|" {
				return a || b, k
			}
			return a && b, k
		case []any:
			campo, op, val := c[0].(string), c[1].(string), c[2]
			x := v[campo]
			if l, ok := x.([]any); ok && len(l) > 0 { // many2one: [id, nombre]
				x = l[0]
			}
			sx, sval := fmt.Sprint(x), fmt.Sprint(val)
			switch op {
			case "=":
				return sx == sval, i + 1
			case "=like":
				return strings.HasPrefix(sx, strings.TrimSuffix(sval, "%")), i + 1
			case "!=":
				return sx != sval, i + 1
			case "<", "<=", ">", ">=":
				c := compara(x, val)
				return map[string]bool{"<": c < 0, "<=": c <= 0, ">": c > 0, ">=": c >= 0}[op], i + 1
			case "ilike":
				return strings.Contains(strings.ToLower(sx), strings.ToLower(sval)), i + 1
			case "in":
				for _, y := range val.([]any) {
					if fmt.Sprint(y) == sx {
						return true, i + 1
					}
				}
				return false, i + 1
			}
			panic("operador no previsto: " + op)
		}
		panic(fmt.Sprintf("dominio no previsto: %v", dom[i]))
	}
	ok, i := true, 0
	for i < len(dom) { // lo que queda arriba va unido con «&»
		var r bool
		r, i = eval(i)
		ok = ok && r
	}
	return ok
}

func (o *odooMemoria) CallKW(_ context.Context, ses, modelo, metodo string, args []any, kw map[string]any, res any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if ses == "" {
		return errors.New("sin sesión")
	}
	tabla := o.lineas
	if modelo == "project.task" {
		tabla = o.tareas
	}
	var r any = true
	switch metodo {
	case "fields_get":
		r = map[string]any{"ai_model": map[string]any{}, "ai_session_id": map[string]any{}, "ai_tokens_input": map[string]any{},
			"ai_tokens_output": map[string]any{}, "ai_tokens_total": map[string]any{}, "ai_cost": map[string]any{}}
	case "create":
		o.sig++
		v := map[string]any{}
		for k, x := range args[0].(map[string]any) {
			v[k] = x
		}
		tabla[o.sig] = v
		r = o.sig
	case "write":
		for _, id := range args[0].([]int) {
			if tabla[id] == nil {
				return errors.New("MissingError: Record does not exist or has been deleted")
			}
			for k, x := range args[1].(map[string]any) {
				tabla[id][k] = x
			}
		}
	case "unlink":
		for _, id := range args[0].([]int) {
			delete(tabla, id)
		}
	case "search_read":
		l := []any{}
		for id, v := range tabla {
			if !cumple(v, args[0].([]any)) {
				continue
			}
			x := map[string]any{"id": id}
			for k, val := range v {
				x[k] = val
			}
			if t, ok := v["task_id"]; ok {
				x["task_id"] = []any{t, fmt.Sprint(o.tareas[toInt(t)]["name"])}
			}
			l = append(l, x)
			if lim, ok := kw["limit"].(int); ok && len(l) >= lim {
				break
			}
		}
		r = l
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, res)
}

// compara dos valores de un dominio: como números si lo son los dos; si no, como texto.
func compara(a, b any) int {
	fa, oka := numero(a)
	fb, okb := numero(b)
	if oka && okb {
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return 0
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}

func numero(x any) (float64, bool) {
	switch v := x.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

func toInt(x any) int {
	switch v := x.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// Todos los escenarios de imputación con el Odoo en memoria (los mismos que cmd/imputacion-prueba ejecuta contra
// el Odoo de verdad).
func TestEscenarios(t *testing.T) {
	for _, e := range Escenarios() {
		t.Run(e.Clave+" "+e.Nombre, func(t *testing.T) {
			b := Banco{Odoo: nuevoOdooMemoria(), Sesion: "ses", UID: 12, Proyecto: 900, NombreProy: "PRUEBAS PLANESGO",
				Dia: time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local), Dir: t.TempDir()}
			lineas, fallos := b.Ejecuta(e)
			for _, f := range fallos {
				t.Error(f)
			}
			if t.Failed() {
				for _, l := range lineas {
					t.Log("en Odoo: " + l.String() + " — " + l.Nombre)
				}
			}
		})
	}
}
