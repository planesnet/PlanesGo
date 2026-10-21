// Package skills hace el inventario de las skills que ven los agentes (Claude Code, OpenCode y Antigravity),
// como lo hace cada uno: una skill es una carpeta con SKILL.md, cuya cabecera (name y description) es lo único
// que el agente carga siempre; el cuerpo solo se lee cuando decide usarla, porque la descripción encaja con lo
// que se le pide. De ahí el coste fijo (descripciones, en cada conversación) y el coste al usarla (el cuerpo).
package skills

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Skill es una skill instalada.
type Skill struct {
	Nombre      string   `json:"nombre"`
	Descripcion string   `json:"descripcion"`
	Ruta        string   `json:"ruta"`        // SKILL.md
	Ambito      string   `json:"ambito"`      // personal | proyecto | plugin | sincronizada
	Origen      string   `json:"origen"`      // carpeta de la que viene (p. ej. ~/.claude/skills)
	Agentes     []string `json:"agentes"`     // claude-code | opencode | antigravity
	TokensFijo  int      `json:"tokens_fijo"` // nombre + descripción: se cargan siempre
	TokensUso   int      `json:"tokens_uso"`  // el cuerpo de SKILL.md: al usarla
	Ficheros    int      `json:"ficheros"`    // otros ficheros de la skill (se leen solo si hacen falta)
	Avisos      []string `json:"avisos"`
	Plugin      string   `json:"plugin,omitempty"`
	Rota        bool     `json:"rota,omitempty"`       // enlace roto o carpeta sin SKILL.md: el agente no la ve
	Sustituida  bool     `json:"sustituida,omitempty"` // la hace pgocode: no se sugiere ni la usan sus agentes
}

// Sustituidas: skills cuyo trabajo hace pgocode por su cuenta (la imputación de horas en Odoo). Siguen
// instaladas para las sesiones de los agentes fuera de pgocode.
var Sustituidas = []string{"pgo-imputacion"}

// tokens: estimación habitual (unos 4 caracteres por token).
func tokens(s string) int { return (utf8.RuneCountInString(s) + 3) / 4 }

type ubicacion struct {
	dir, ambito string
	agentes     []string
	patron      string // relativo a dir
	plugin      string
}

// ubicaciones: dónde busca cada agente, de lo más concreto (proyecto) a lo más general.
func ubicaciones(carpeta string) []ubicacion {
	casa, _ := os.UserHomeDir()
	cc, oc, ag := "claude-code", "opencode", "antigravity"
	var l []ubicacion
	// Proyecto: .claude/skills desde la carpeta hasta la raíz del repositorio (Claude Code y OpenCode).
	if carpeta != "" {
		for d := carpeta; ; d = filepath.Dir(d) {
			l = append(l, ubicacion{filepath.Join(d, ".claude", "skills"), "proyecto", []string{cc, oc}, "*/SKILL.md", ""})
			if _, err := os.Stat(filepath.Join(d, ".git")); err == nil || filepath.Dir(d) == d || d == casa {
				break
			}
		}
		l = append(l,
			ubicacion{filepath.Join(carpeta, ".opencode", "skill"), "proyecto", []string{oc}, "**/SKILL.md", ""},
			ubicacion{filepath.Join(carpeta, ".opencode", "skills"), "proyecto", []string{oc}, "**/SKILL.md", ""},
			ubicacion{filepath.Join(carpeta, ".agents", "skills"), "proyecto", []string{ag}, "*/SKILL.md", ""},
			ubicacion{filepath.Join(carpeta, "_agents", "skills"), "proyecto", []string{ag}, "*/SKILL.md", ""},
		)
	}
	l = append(l,
		ubicacion{filepath.Join(casa, ".claude", "skills"), "personal", []string{cc, oc}, "*/SKILL.md", ""},
		ubicacion{filepath.Join(casa, ".claude", "skills", "synced"), "sincronizada", []string{cc}, "*/*/SKILL.md", ""},
		ubicacion{filepath.Join(casa, ".config", "opencode", "skill"), "personal", []string{oc}, "**/SKILL.md", ""},
		ubicacion{filepath.Join(casa, ".config", "opencode", "skills"), "personal", []string{oc}, "**/SKILL.md", ""},
		ubicacion{filepath.Join(casa, ".gemini", "config", "skills"), "personal", []string{ag}, "*/SKILL.md", ""},
		ubicacion{filepath.Join(casa, ".agents", "skills"), "personal", []string{ag}, "*/SKILL.md", ""},
	)
	// Plugins activados de Claude Code (enabledPlugins en ~/.claude/settings.json).
	var cfg struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if b, err := os.ReadFile(filepath.Join(casa, ".claude", "settings.json")); err == nil {
		json.Unmarshal(b, &cfg)
	}
	for id, on := range cfg.EnabledPlugins {
		nombre, mercado, ok := strings.Cut(id, "@")
		if !on || !ok {
			continue
		}
		vers, _ := filepath.Glob(filepath.Join(casa, ".claude", "plugins", "cache", mercado, nombre, "*"))
		sort.Strings(vers)
		if len(vers) > 0 {
			l = append(l, ubicacion{filepath.Join(vers[len(vers)-1], "skills"), "plugin", []string{cc}, "*/SKILL.md", nombre})
		}
	}
	return l
}

// glob con «**» (cualquier profundidad) o el patrón de filepath.Glob.
func busca(dir, patron string) []string {
	if !strings.Contains(patron, "**") {
		m, _ := filepath.Glob(filepath.Join(dir, patron))
		return m
	}
	var m []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "SKILL.md" {
			m = append(m, p)
		}
		return nil
	})
	return m
}

// Inventario: todas las skills que ven los agentes trabajando en carpeta. Una misma carpeta de skills vista
// por varios agentes sale una vez, con todos ellos.
func Inventario(carpeta string) []Skill {
	porRuta := map[string]*Skill{}
	var orden []string
	for _, u := range ubicaciones(carpeta) {
		// Enlaces rotos y carpetas sin SKILL.md: el agente no las ve, pero conviene saberlo.
		if u.patron == "*/SKILL.md" {
			ents, _ := os.ReadDir(u.dir)
			for _, e := range ents {
				p := filepath.Join(u.dir, e.Name())
				if strings.HasPrefix(e.Name(), ".") || e.Name() == "synced" {
					continue
				}
				if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err == nil || porRuta[p] != nil {
					continue
				}
				aviso := "Carpeta sin SKILL.md: no es una skill"
				if st, err := os.Stat(p); err != nil {
					destino, _ := os.Readlink(p)
					aviso = "Enlace roto (apunta a " + destino + "): la skill no existe"
				} else if !st.IsDir() {
					continue
				}
				porRuta[p] = &Skill{Nombre: e.Name(), Ruta: p, Ambito: u.ambito, Origen: u.dir, Agentes: append([]string(nil), u.agentes...),
					Avisos: []string{aviso}, Rota: true}
				orden = append(orden, p)
			}
		}
		for _, f := range busca(u.dir, u.patron) {
			real, err := filepath.EvalSymlinks(f)
			if err != nil {
				real = f
			}
			if s := porRuta[real]; s != nil {
				for _, a := range u.agentes {
					if !contiene(s.Agentes, a) {
						s.Agentes = append(s.Agentes, a)
					}
				}
				continue
			}
			s := lee(f)
			s.Ambito, s.Origen, s.Plugin = u.ambito, u.dir, u.plugin
			s.Agentes = append([]string(nil), u.agentes...)
			porRuta[real] = &s
			orden = append(orden, real)
		}
	}
	l := make([]Skill, 0, len(orden))
	nombres := map[string]int{}
	for _, r := range orden {
		l = append(l, *porRuta[r])
		if !porRuta[r].Rota {
			nombres[porRuta[r].Nombre]++
		}
	}
	for i := range l {
		if contiene(Sustituidas, l[i].Nombre) {
			l[i].Sustituida = true
			l[i].Avisos = append(l[i].Avisos, "La imputación la hace pgocode: no se sugiere ni la usan los agentes de pgocode")
		}
		if !l[i].Rota && nombres[l[i].Nombre] > 1 {
			l[i].Avisos = append(l[i].Avisos, "Hay otra skill con el mismo nombre: el agente puede usar una u otra")
		}
	}
	return l
}

func contiene(l []string, x string) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

// lee interpreta SKILL.md: la cabecera YAML (name, description; también en bloque «>» o «|») y el cuerpo.
func lee(f string) Skill {
	s := Skill{Ruta: f, Agentes: []string{}, Avisos: []string{}}
	b, err := os.ReadFile(f)
	if err != nil {
		s.Nombre = filepath.Base(filepath.Dir(f))
		s.Avisos = append(s.Avisos, "No se puede leer SKILL.md")
		return s
	}
	cab, cuerpo := Cabecera(string(b))
	s.Nombre, s.Descripcion = cab["name"], cab["description"]
	carpeta := filepath.Base(filepath.Dir(f))
	if s.Nombre == "" {
		s.Nombre = carpeta
		s.Avisos = append(s.Avisos, "Sin «name» en la cabecera: se usa el nombre de la carpeta")
	} else if s.Nombre != carpeta {
		s.Avisos = append(s.Avisos, "El nombre («"+s.Nombre+"») no coincide con la carpeta («"+carpeta+"»)")
	}
	switch d := strings.ToLower(s.Descripcion); {
	case d == "":
		s.Avisos = append(s.Avisos, "Sin descripción: el agente no sabrá cuándo usarla")
	case utf8.RuneCountInString(s.Descripcion) > 1024:
		s.Avisos = append(s.Avisos, "Descripción de más de 1024 caracteres: se carga en todas las conversaciones")
	case !dicesCuando(d):
		s.Avisos = append(s.Avisos, "La descripción no dice cuándo usarla («Úsala cuando…»): el agente acierta menos")
	}
	s.TokensFijo = tokens(s.Nombre + ": " + s.Descripcion)
	s.TokensUso = tokens(cuerpo)
	if s.TokensUso > 5000 {
		s.Avisos = append(s.Avisos, "Cuerpo muy largo: conviene pasar el detalle a ficheros aparte que se lean solo si hacen falta")
	}
	filepath.WalkDir(filepath.Dir(f), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && p != f {
			s.Ficheros++
		}
		return nil
	})
	return s
}

func dicesCuando(d string) bool {
	for _, k := range []string{"úsala", "usala", "úsalo", "usar cuando", "cuando ", "use when", "use this", "use it", "when ", "si el usuario", "si se pide", "para "} {
		if strings.Contains(d, k) {
			return true
		}
	}
	return false
}

// Cabecera separa la cabecera YAML de un SKILL.md (solo claves de primer nivel con texto) y el cuerpo.
func Cabecera(t string) (map[string]string, string) {
	m := map[string]string{}
	t = strings.TrimPrefix(t, "\xef\xbb\xbf")
	if !strings.HasPrefix(t, "---") {
		return m, t
	}
	resto := t[3:]
	fin := strings.Index(resto, "\n---")
	if fin < 0 {
		return m, t
	}
	yaml, cuerpo := resto[:fin], resto[fin+4:]
	if i := strings.IndexByte(cuerpo, '\n'); i >= 0 {
		cuerpo = cuerpo[i+1:]
	}
	sc := bufio.NewScanner(strings.NewReader(yaml))
	clave, bloque := "", false
	var acum []string
	cierra := func() {
		if clave != "" && bloque {
			m[clave] = strings.TrimSpace(strings.Join(acum, " "))
		}
		clave, bloque, acum = "", false, nil
	}
	for sc.Scan() {
		l := sc.Text()
		if bloque && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.TrimSpace(l) == "") {
			acum = append(acum, strings.TrimSpace(l))
			continue
		}
		cierra()
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if v == ">" || v == "|" || v == ">-" || v == "|-" || v == "" {
			clave, bloque = k, true
			continue
		}
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			if v[0] == '"' {
				var x string
				if json.Unmarshal([]byte(v), &x) == nil {
					v = x
				} else {
					v = v[1 : len(v)-1]
				}
			} else {
				v = strings.ReplaceAll(v[1:len(v)-1], "''", "'")
			}
		}
		m[k] = v
	}
	cierra()
	return m, cuerpo
}

// DeRuta dice qué skill es un fichero que un agente lee (…/<skill>/SKILL.md), o "".
func DeRuta(p string) string {
	if filepath.Base(p) != "SKILL.md" {
		return ""
	}
	return filepath.Base(filepath.Dir(p))
}
