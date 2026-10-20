// Package llm reúne los agentes con los que se trabaja desde el chat: programas que se lanzan en la carpeta del
// proyecto (Claude Code, OpenCode, Gemini CLI…). El chat no sabe con cuál habla: todos cumplen Proveedor.
package llm

import (
	"context"
	"sort"
)

// Mensaje es un turno de la conversación. Rol: "user" o "assistant".
type Mensaje struct {
	Rol   string `json:"rol"`
	Texto string `json:"texto"`
}

// Peticion es lo que el chat pide a un proveedor.
type Peticion struct {
	Modelo   string
	Sistema  string
	Mensajes []Mensaje
	// Carpeta del proyecto: obligatoria para los agentes, que trabajan sobre ella.
	Carpeta string
	// SesionAgente permite a un agente continuar su propia conversación (p. ej. claude --resume).
	SesionAgente string
	// Permisos del agente en la sesión: "lectura" (por defecto), "editar" o "auto" (ejecuta comandos con el
	// control de seguridad del agente).
	Permisos string
	// Esfuerzo de razonamiento del agente (uno de los de su Info.Esfuerzos; vacío: el suyo por defecto).
	Esfuerzo string
	// Entorno: variables que se añaden al del agente (PGO_API, PGO_TOKEN… para la orden «pgo»).
	Entorno []string
}

// Evento es un trozo de la respuesta, igual para todos los agentes (la interfaz común de pgocode): texto,
// razonamiento, una herramienta (al empezar y al terminar, con el mismo id), el consumo del turno o la sesión
// del agente para continuarla después.
type Evento struct {
	Texto        string       `json:"texto,omitempty"`
	Pensamiento  string       `json:"pensamiento,omitempty"`
	Herramienta  *Herramienta `json:"herramienta,omitempty"`
	Uso          *Uso         `json:"uso,omitempty"`
	SesionAgente string       `json:"sesion_agente,omitempty"`
}

// Herramienta es una llamada del agente a una herramienta (leer, editar, ejecutar…).
type Herramienta struct {
	ID      string `json:"id"`
	Nombre  string `json:"nombre"`
	Resumen string `json:"resumen,omitempty"` // lo principal de la entrada: fichero, orden, patrón…
	Entrada string `json:"entrada,omitempty"` // la entrada completa (JSON), recortada
	Salida  string `json:"salida,omitempty"`  // el resultado, recortado
	Estado  string `json:"estado"`            // curso | ok | error
}

// Uso es el consumo de un turno.
type Uso struct {
	Entrada  int     `json:"entrada"` // tokens de entrada (incluida la caché)
	Salida   int     `json:"salida"`  // tokens de salida
	Coste    float64 `json:"coste,omitempty"`
	Segundos float64 `json:"segundos,omitempty"`
}

// Info describe un proveedor para el selector del chat.
type Info struct {
	ID          string   `json:"id"`
	Nombre      string   `json:"nombre"`
	Tipo        string   `json:"tipo"` // "api" o "agente"
	Disponible  bool     `json:"disponible"`
	Motivo      string   `json:"motivo,omitempty"` // por qué no está disponible
	Modelos     []string `json:"modelos"`
	PorDefecto  string   `json:"por_defecto"`
	NecesitaDir bool     `json:"necesita_carpeta"`
	Esfuerzos   []string `json:"esfuerzos,omitempty"` // niveles de esfuerzo que admite (vacío: ninguno)
	Modos       bool     `json:"modos,omitempty"`     // admite los modos de permisos (lectura, editar, auto)
}

// Proveedor habla con un modelo o agente. ID no hace llamadas de red; Info tampoco bloquea (la lista de
// modelos se carga en segundo plano).
type Proveedor interface {
	ID() string
	Info(ctx context.Context) Info
	Chat(ctx context.Context, p Peticion, emite func(Evento)) error
}

// Registro guarda los proveedores por id.
type Registro struct {
	prov map[string]Proveedor
}

// NuevoRegistro crea el registro con los proveedores dados.
func NuevoRegistro(ps ...Proveedor) *Registro {
	r := &Registro{prov: map[string]Proveedor{}}
	for _, p := range ps {
		r.prov[p.ID()] = p
	}
	return r
}

// Busca devuelve el proveedor con ese id.
func (r *Registro) Busca(id string) (Proveedor, bool) {
	p, ok := r.prov[id]
	return p, ok
}

// Lista devuelve la información de todos los proveedores: primero los disponibles.
func (r *Registro) Lista(ctx context.Context) []Info {
	l := make([]Info, 0, len(r.prov))
	for _, p := range r.prov {
		l = append(l, p.Info(ctx))
	}
	sort.Slice(l, func(i, j int) bool {
		if l[i].Disponible != l[j].Disponible {
			return l[i].Disponible
		}
		if l[i].Tipo != l[j].Tipo {
			return l[i].Tipo < l[j].Tipo
		}
		return l[i].Nombre < l[j].Nombre
	})
	return l
}
