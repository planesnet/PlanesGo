package servidor

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"planesgo-code/internal/sesiones"
)

// Planes (modo plan): el agente redacta un plan sin tocar nada; el usuario lo revisa, lo edita o pide cambios, y
// al final lo ejecuta o lo descarta. Cada plan es un Markdown con una cabecera de datos, guardado en el directorio
// local del proyecto: <directorio>/.pgocode/planes/ (fuera de git).

// Plan: la cabecera y, al leerlo, el texto.
type Plan struct {
	Ruta       string `json:"ruta"`
	Numero     int    `json:"numero"` // número del plan en el proyecto (#1, #2…), para nombrarlo en el chat
	Titulo     string `json:"titulo"`
	Proyecto   int    `json:"proyecto"`
	Sesion     string `json:"sesion,omitempty"`
	Agente     string `json:"agente,omitempty"`
	Estado     string `json:"estado"` // borrador | ejecutado | descartado
	Creado     string `json:"creado,omitempty"`
	Modificado string `json:"modificado,omitempty"`
	Texto      string `json:"texto,omitempty"`
}

var (
	estadosPlan  = map[string]bool{"borrador": true, "ejecutado": true, "descartado": true}
	noSlug       = regexp.MustCompile(`[^a-z0-9]+`)
	clavesPlan   = []string{"numero", "titulo", "proyecto", "sesion", "agente", "estado", "creado"}
	errFueraPlan = errors.New("ese plan no está en la carpeta de planes del proyecto")
)

// dirPlanes: la carpeta de planes del directorio del proyecto. El directorio tiene que ser una carpeta del proyecto
// o contener alguna (el directorio principal, donde viven las carpetas de las ramas). Sin directorio en los
// ajustes del proyecto, el principal por defecto: el que contiene el repositorio.
func (s *Servidor) dirPlanes(r *http.Request, proyecto int, directorio string) (string, bool) {
	if directorio == "" && r != nil {
		if repo := s.repoProyectoDir(r, proyecto); repo != "" {
			directorio = filepath.Dir(repo)
		}
	}
	d := filepath.Clean(directorio)
	if directorio == "" || !filepath.IsAbs(d) {
		return "", false
	}
	carpetas := append(append([]string(nil), s.Indice.Carpetas(false)[proyecto]...), s.Indice.Registradas(proyecto)...)
	for _, c := range carpetas {
		c = filepath.Clean(c)
		if c == d || strings.HasPrefix(c, d+string(filepath.Separator)) {
			return filepath.Join(d, ".pgocode", "planes"), true
		}
	}
	return "", false
}

// slug: nombre de fichero a partir del título (sin acentos ni signos).
func slug(t string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
	x := strings.Trim(noSlug.ReplaceAllString(r.Replace(strings.ToLower(t)), "-"), "-")
	if len(x) > 50 {
		x = strings.Trim(x[:50], "-")
	}
	if x == "" {
		x = "plan"
	}
	return x
}

// componePlan: cabecera YAML sencilla (clave: valor) y el texto.
func componePlan(p Plan) string {
	var b strings.Builder
	b.WriteString("---\n")
	v := map[string]string{"numero": numeroTexto(p.Numero), "titulo": p.Titulo, "proyecto": strconv.Itoa(p.Proyecto), "sesion": p.Sesion, "agente": p.Agente, "estado": p.Estado, "creado": p.Creado}
	for _, k := range clavesPlan {
		if v[k] != "" {
			b.WriteString(k + ": " + strings.ReplaceAll(v[k], "\n", " ") + "\n")
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(p.Texto) + "\n")
	return b.String()
}

// leePlan: separa la cabecera del texto. Un fichero sin cabecera es un plan en borrador.
func leePlan(contenido string) Plan {
	p := Plan{Estado: "borrador", Texto: contenido}
	if !strings.HasPrefix(contenido, "---\n") {
		return p
	}
	cab, resto, ok := strings.Cut(contenido[4:], "\n---\n")
	if !ok {
		return p
	}
	p.Texto = strings.TrimLeft(resto, "\n")
	for _, l := range strings.Split(cab, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "numero":
			p.Numero, _ = strconv.Atoi(v)
		case "titulo":
			p.Titulo = v
		case "proyecto":
			p.Proyecto, _ = strconv.Atoi(v)
		case "sesion":
			p.Sesion = v
		case "agente":
			p.Agente = v
		case "estado":
			if estadosPlan[v] {
				p.Estado = v
			}
		case "creado":
			p.Creado = v
		}
	}
	return p
}

// rutaPlan: comprueba que la ruta es un .md dentro de la carpeta de planes.
func rutaPlan(dir, ruta string) (string, error) {
	r := filepath.Clean(ruta)
	if filepath.Dir(r) != dir || filepath.Ext(r) != ".md" {
		return "", errFueraPlan
	}
	return r, nil
}

// planesGuarda: crea un plan (sin ruta) o lo reescribe; un estado vacío conserva el que tenía.
func (s *Servidor) planesGuarda(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Plan
		Directorio string `json:"directorio"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	dir, ok := s.dirPlanes(r, p.Proyecto, p.Directorio)
	if !ok {
		jsonError(w, http.StatusBadRequest, "ese directorio no es del proyecto")
		return
	}
	if p.Estado != "" && !estadosPlan[p.Estado] {
		jsonError(w, http.StatusBadRequest, "estado de plan no válido")
		return
	}
	pl := p.Plan
	ahora := time.Now()
	if pl.Ruta == "" {
		if strings.TrimSpace(pl.Texto) == "" {
			jsonError(w, http.StatusBadRequest, "el plan está vacío")
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, c := range s.Indice.Registradas(p.Proyecto) {
			excluyeDeGit(c)
		}
		pl.Ruta = filepath.Join(dir, ahora.Format("2006-01-02-1504")+"-"+slug(pl.Titulo)+".md")
		for i := 2; ; i++ {
			if _, err := os.Stat(pl.Ruta); os.IsNotExist(err) {
				break
			}
			pl.Ruta = filepath.Join(dir, ahora.Format("2006-01-02-1504")+"-"+slug(pl.Titulo)+"-"+strconv.Itoa(i)+".md")
		}
		pl.Creado = ahora.Format(time.RFC3339)
		pl.Numero = siguienteNumero(dir, p.Proyecto)
		if pl.Estado == "" {
			pl.Estado = "borrador"
		}
	} else {
		ruta, err := rutaPlan(dir, pl.Ruta)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		b, err := os.ReadFile(ruta)
		if err != nil {
			jsonError(w, http.StatusNotFound, "no existe ese plan")
			return
		}
		viejo := leePlan(string(b))
		viejo.Ruta = ruta
		if pl.Titulo != "" {
			viejo.Titulo = pl.Titulo
		}
		if pl.Estado != "" {
			viejo.Estado = pl.Estado
		}
		if pl.Texto != "" {
			viejo.Texto = pl.Texto
		}
		if pl.Sesion != "" {
			viejo.Sesion = pl.Sesion
		}
		if pl.Agente != "" {
			viejo.Agente = pl.Agente
		}
		viejo.Proyecto = p.Proyecto
		if viejo.Numero == 0 {
			viejo.Numero = siguienteNumero(dir, p.Proyecto)
		}
		pl = viejo
	}
	pl.Proyecto = p.Proyecto
	if err := os.WriteFile(pl.Ruta, []byte(componePlan(pl)), 0o644); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pl.Modificado = ahora.Format(time.RFC3339)
	jsonOK(w, pl)
}

// planesLista: los planes del proyecto, el más reciente primero (sin el texto).
func (s *Servidor) planesLista(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	q := r.URL.Query()
	id, _ := strconv.Atoi(q.Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	dir, ok := s.dirPlanes(r, id, q.Get("directorio"))
	if !ok {
		jsonError(w, http.StatusBadRequest, "ese directorio no es del proyecto")
		return
	}
	numera(dir, id)
	ents, _ := os.ReadDir(dir)
	l := []Plan{}
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		f := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		p := leePlan(string(b))
		// El directorio principal puede ser compartido por varios proyectos: solo los de este.
		if p.Proyecto != 0 && p.Proyecto != id {
			continue
		}
		p.Ruta, p.Texto = f, ""
		if p.Titulo == "" {
			p.Titulo = strings.TrimSuffix(e.Name(), ".md")
		}
		if st, err := e.Info(); err == nil {
			p.Modificado = st.ModTime().Format(time.RFC3339)
		}
		l = append(l, p)
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Modificado > l[j].Modificado })
	jsonOK(w, l)
}

// planesLee: un plan con su texto.
func (s *Servidor) planesLee(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	q := r.URL.Query()
	id, _ := strconv.Atoi(q.Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	dir, ok := s.dirPlanes(r, id, q.Get("directorio"))
	if !ok {
		jsonError(w, http.StatusBadRequest, "ese directorio no es del proyecto")
		return
	}
	ruta, err := rutaPlan(dir, q.Get("ruta"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := os.ReadFile(ruta)
	if err != nil {
		jsonError(w, http.StatusNotFound, "no existe ese plan")
		return
	}
	p := leePlan(string(b))
	p.Ruta = ruta
	if st, err := os.Stat(ruta); err == nil {
		p.Modificado = st.ModTime().Format(time.RFC3339)
	}
	jsonOK(w, p)
}

// planesBorra: borra un plan, solo si está descartado (los demás se descartan antes).
func (s *Servidor) planesBorra(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto   int    `json:"proyecto"`
		Directorio string `json:"directorio"`
		Ruta       string `json:"ruta"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	dir, ok := s.dirPlanes(r, p.Proyecto, p.Directorio)
	if !ok {
		jsonError(w, http.StatusBadRequest, "ese directorio no es del proyecto")
		return
	}
	ruta, err := rutaPlan(dir, p.Ruta)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := os.ReadFile(ruta)
	if err != nil {
		jsonError(w, http.StatusNotFound, "no existe ese plan")
		return
	}
	if leePlan(string(b)).Estado != "descartado" {
		jsonError(w, http.StatusBadRequest, "solo se pueden borrar los planes descartados")
		return
	}
	if err := os.Remove(ruta); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func numeroTexto(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// planesDe: los planes del proyecto en la carpeta (con su texto), del más antiguo al más reciente.
func planesDe(dir string, proyecto int) []Plan {
	ents, _ := os.ReadDir(dir)
	var l []Plan
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		f := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		p := leePlan(string(b))
		if p.Proyecto != 0 && p.Proyecto != proyecto {
			continue
		}
		p.Ruta = f
		if p.Creado == "" {
			if st, err := e.Info(); err == nil {
				p.Creado = st.ModTime().Format(time.RFC3339)
			}
		}
		l = append(l, p)
	}
	sort.SliceStable(l, func(i, j int) bool { return l[i].Creado < l[j].Creado })
	return l
}

// siguienteNumero: el número del próximo plan del proyecto. Los números no se reutilizan (un plan borrado deja su
// número sin usar, para que «plan N» en el chat nunca se confunda): se guarda el último dado en .contador-<proyecto>.
func siguienteNumero(dir string, proyecto int) int {
	n := 0
	for _, p := range planesDe(dir, proyecto) {
		n = max(n, p.Numero)
	}
	f := filepath.Join(dir, ".contador-"+strconv.Itoa(proyecto))
	if b, err := os.ReadFile(f); err == nil {
		if c, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			n = max(n, c)
		}
	}
	os.WriteFile(f, []byte(strconv.Itoa(n+1)+"\n"), 0o644)
	return n + 1
}

// numera: los planes de antes de la numeración reciben su número (por orden de creación) y se guarda.
func numera(dir string, proyecto int) {
	n := 0
	l := planesDe(dir, proyecto)
	for _, p := range l {
		n = max(n, p.Numero)
	}
	for _, p := range l {
		if p.Numero == 0 {
			n++
			p.Numero = n
			p.Proyecto = proyecto
			os.WriteFile(p.Ruta, []byte(componePlan(p)), 0o644)
		}
	}
}
