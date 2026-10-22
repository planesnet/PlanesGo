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
	// Adjuntos: ficheros que el usuario adjunta al mensaje (rutas dentro de la carpeta de la sesión).
	Adjuntos []string
	// Entorno: variables que se añaden al del agente (PGO_API, PGO_TOKEN… para la orden «pgo»).
	Entorno []string
	// Autorizadas: carpetas de fuera del proyecto que el usuario ha autorizado en la sesión. El agente no puede
	// salir de su carpeta salvo a estas (ahorra tokens y evita que recorra el equipo).
	Autorizadas []string
	// MCP: servidores MCP de Claude Code que la sesión quiere además de «pgo» (por defecto, ninguno: cada uno
	// arranca su proceso y añade sus herramientas a cada turno).
	MCP []string
	// Idioma del usuario (su nombre, p. ej. «español»): el agente responde en él.
	Idioma string
	// Traza: la sesión de pgocode y el turno (para encontrar en depuracion.log lo de cada sesión).
	Traza string
	// AlInyectar: si el agente admite mensajes a mitad de turno (Claude Code), se llama con la función que se los
	// pasa mientras su entrada está abierta, y con nil cuando se cierra (acaba su respuesta).
	AlInyectar func(enviar func(texto string) error)
	// Aparte: una pregunta que no entra en la conversación. Va a una copia de la sesión del agente (Claude:
	// --fork-session sin guardarla; OpenCode: --fork y se borra al acabar), en solo lectura.
	Aparte bool
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
	Tarea        *Tarea       `json:"tarea,omitempty"`
	// Adoptados: programas que el agente dejó en marcha (en segundo plano) y que pgocode se queda al acabar
	// su respuesta, para no esperarlos ni gastar otro turno (siguen en Programas y en la barra de ejecución).
	Adoptados []Adoptado `json:"adoptados,omitempty"`
	// Vivos: al acabar el turno, los grupos de procesos de este agente que siguen en marcha (solo los suyos).
	Vivos *int `json:"-"`
}

// Adoptado: un programa del agente que sigue en marcha en pgocode.
type Adoptado struct {
	Nombre  string `json:"nombre"`
	Orden   string `json:"orden"`
	Fichero string `json:"-"` // donde escribe su salida
	Grupo   int    `json:"-"` // su grupo de procesos
	Marca   string `json:"-"` // PGO_EJECUCION de la ejecución del agente (la heredan sus procesos)
	Tarea   string `json:"tarea,omitempty"`
}

// Fondo: una orden que el agente tiene en segundo plano.
type Fondo struct {
	ID, Orden, Titulo, Fichero string
}

// FinConFondo: el agente ha acabado su respuesta y solo espera a órdenes en segundo plano (p. ej. el programa
// que se está desarrollando, en marcha para probarlo). pgocode termina el turno y se queda esas órdenes.
type FinConFondo struct{ Ordenes []Fondo }

func (f *FinConFondo) Error() string { return "fin del turno con órdenes en segundo plano" }

// Tarea: lo que un agente hace aparte de la conversación principal, con el mismo concepto en todos los agentes:
// un subagente, una tarea en segundo plano (una orden que sigue corriendo) o su lista de tareas. Se emite varias
// veces con el mismo ID según avanza (lo que llega de nuevo sustituye a lo anterior; lo vacío se conserva).
type Tarea struct {
	ID           string     `json:"id"`
	Tipo         string     `json:"tipo,omitempty"`        // subagente | segundo_plano | lista
	Herramienta  string     `json:"herramienta,omitempty"` // la herramienta que la creó: las suyas llevan Padre = esto
	Padre        string     `json:"padre,omitempty"`       // tarea dentro de la que se lanzó (subagente que lanza otra)
	Titulo       string     `json:"titulo,omitempty"`
	Subtipo      string     `json:"subtipo,omitempty"` // general-purpose, explore, research…
	Estado       string     `json:"estado,omitempty"`  // curso | ok | error | parada
	SegundoPlano bool       `json:"segundo_plano,omitempty"`
	Progreso     string     `json:"progreso,omitempty"` // qué está haciendo (última herramienta, tokens, duración)
	Salida       string     `json:"salida,omitempty"`   // resumen final, recortado
	Detalle      string     `json:"detalle,omitempty"`  // fichero con el detalle completo (salida, transcript)
	SesionHija   string     `json:"sesion_hija,omitempty"`
	Elementos    []Elemento `json:"elementos,omitempty"` // Tipo «lista»
}

// Elemento de una lista de tareas del agente.
type Elemento struct {
	Texto  string `json:"texto"`
	Estado string `json:"estado"` // pendiente | curso | hecho
}

// Herramienta es una llamada del agente a una herramienta (leer, editar, ejecutar…).
type Herramienta struct {
	ID      string `json:"id"`
	Nombre  string `json:"nombre"`
	Resumen string `json:"resumen,omitempty"` // lo principal de la entrada: fichero, orden, patrón…
	Entrada string `json:"entrada,omitempty"` // la entrada completa (JSON), recortada
	Salida  string `json:"salida,omitempty"`  // el resultado, recortado
	Estado  string `json:"estado"`            // curso | ok | error
	Skill   string `json:"skill,omitempty"`   // la skill que el agente carga con esta herramienta
	Padre   string `json:"padre,omitempty"`   // herramienta (de subagente) dentro de la que se usa
	// Imágenes que devuelve la herramienta (capturas del navegador, imágenes leídas): las guarda el servidor como
	// evidencias y por el chat solo viajan sus rutas (Capturas).
	Imagenes []Imagen `json:"-"`
	Capturas []string `json:"capturas,omitempty"`
}

// Imagen devuelta por una herramienta.
type Imagen struct {
	Tipo  string // image/png, image/jpeg…
	Datos []byte
}

// Uso es el consumo de un turno.
type Uso struct {
	Entrada  int     `json:"entrada"` // tokens de entrada (incluida la caché)
	Salida   int     `json:"salida"`  // tokens de salida
	Coste    float64 `json:"coste,omitempty"`
	Segundos float64 `json:"segundos,omitempty"`
	// Contexto: los tokens que se enviaron en la última llamada al modelo (todo el historial): lo que costará el
	// siguiente mensaje, sin caché si la conversación lleva un rato parada.
	Contexto int `json:"contexto,omitempty"`
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
	Inyecta     bool     `json:"inyecta,omitempty"`   // admite mensajes durante el turno (los lee antes de acabarlo)
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
