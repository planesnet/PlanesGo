// e2e: prueba de humo de punta a punta de pgocode, sin Odoo real ni agentes (cero tokens).
//
// Compila cmd/odoo-prueba y cmd/pgocode en una carpeta temporal, los arranca en puertos libres con datos
// temporales (HOME incluido) y una raíz con un repositorio git del proyecto de prueba (999), y recorre la API:
// sesión, proyectos, lanzar y parar un programa, actividad humana, imputación y validación de tiempos.
// Al acabar (también si falla o se interrumpe) mata todo lo que ha arrancado. Sale con 1 ante cualquier fallo.
// Es para Linux (grupos de procesos y /proc), como el paquete de la app.
//
//	go run ./cmd/e2e            # o: make e2e
//	go run ./cmd/e2e -v         # con el registro del backend y del Odoo simulado
//	go run ./cmd/e2e -conservar # no borra la carpeta temporal (para mirar los datos)
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	proyecto    = 999          // el proyecto de cmd/odoo-prueba
	ordenInocua = "sleep 3017" // programa que se lanza y se para (el número lo distingue de otros «sleep»)
)

var (
	verboso   = flag.Bool("v", false, "muestra el registro del backend y del Odoo simulado")
	conservar = flag.Bool("conservar", false, "no borra la carpeta temporal al acabar")
)

// fallo corta la prueba: lo recoge prueba() para que se limpie todo antes de salir.
type fallo struct{ msg string }

func falla(formato string, a ...any) { panic(fallo{fmt.Sprintf(formato, a...)}) }

func paso(formato string, a ...any) { log.Printf("✓ "+formato, a...) }

func main() {
	log.SetFlags(log.Ltime)
	log.SetPrefix("e2e ")
	flag.Parse()
	os.Exit(prueba())
}

// entorno lleva lo que se arranca y cómo pararlo.
type entorno struct {
	tmp      string
	backend  *exec.Cmd
	odoo     *exec.Cmd
	puerto   string
	limpieza sync.Once

	mu         sync.Mutex
	compilando int // grupo del «go build» en curso (0: ninguno)
}

func prueba() (codigo int) {
	tmp, err := os.MkdirTemp("", "pgocode-e2e-")
	if err != nil {
		log.Print(err)
		return 1
	}
	e := &entorno{tmp: tmp}
	// Ctrl+C o SIGTERM: se limpia igual.
	senal := make(chan os.Signal, 1)
	signal.Notify(senal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-senal
		log.Print("interrumpida: limpiando…")
		e.limpia()
		os.Exit(1)
	}()
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(fallo)
			if !ok {
				f = fallo{fmt.Sprint(r)}
			}
			log.Printf("✗ FALLO: %s", f.msg)
			e.muestraRegistros()
			codigo = 1
		} else if *verboso {
			e.muestraRegistros()
		}
		e.limpia()
		if codigo == 0 {
			log.Print("e2e: todo bien")
		} else if *conservar {
			log.Printf("datos en %s", tmp)
		}
	}()
	e.ejecuta()
	return 0
}

// ---- preparación ----

func raizModulo() string {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" || strings.TrimSpace(string(out)) == os.DevNull {
		falla("no estoy dentro del módulo planesgo-code (go env GOMOD): %v", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

// compila en un grupo de procesos propio: si se interrumpe la prueba, limpia lo mata entero antes de borrar
// la carpeta temporal (si no, el enlazador podría volver a crearla).
func (e *entorno) compila(modulo, salida, paquete string, ldflags string) {
	args := []string{"build", "-trimpath", "-o", salida}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	cmd := exec.Command("go", append(args, paquete)...)
	cmd.Dir = modulo
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	e.mu.Lock()
	err := cmd.Start()
	if err == nil {
		e.compilando = cmd.Process.Pid
	}
	e.mu.Unlock()
	if err == nil {
		err = cmd.Wait()
	}
	e.mu.Lock()
	e.compilando = 0
	e.mu.Unlock()
	if err != nil {
		falla("compilando %s: %v\n%s", paquete, err, out.Bytes())
	}
}

// puertoLibre pide al sistema un puerto libre.
func puertoLibre() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		falla("puerto libre: %v", err)
	}
	defer ln.Close()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	return p
}

func ejecutaGit(dir string, args ...string) {
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=e2e", "-c", "user.email=e2e@prueba.invalid",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		falla("git %v: %v\n%s", args, err, out)
	}
}

// entornoLimpio: el del usuario sin lo que cambiaría el comportamiento de pgocode, con HOME en el temporal.
func (e *entorno) entornoLimpio(casa string) []string {
	var l []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k == "HOME" || strings.HasPrefix(k, "PGO_") || strings.HasPrefix(k, "ODOO_") || strings.HasPrefix(k, "PLANESGO") {
			continue
		}
		l = append(l, kv)
	}
	return append(l, "HOME="+casa)
}

func arranca(nombre, registro string, entorno []string, bin string, args ...string) *exec.Cmd {
	f, err := os.Create(registro)
	if err != nil {
		falla("%v", err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = f, f
	cmd.Env = entorno
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // grupo propio: se mata entero al acabar
	if err := cmd.Start(); err != nil {
		falla("arrancando %s: %v", nombre, err)
	}
	go func() { cmd.Wait(); f.Close() }()
	return cmd
}

// espera repite cond hasta que se cumple o pasa max.
func espera(max time.Duration, cond func() bool) bool {
	for fin := time.Now().Add(max); ; time.Sleep(100 * time.Millisecond) {
		if cond() {
			return true
		}
		if time.Now().After(fin) {
			return false
		}
	}
}

// ---- cliente de la API ----

type cliente struct {
	base string
	http *http.Client
}

// pide hace la petición (con X-PGO en las escrituras) y devuelve el estado y el cuerpo.
func (c *cliente) pide(metodo, ruta string, cuerpo any) (int, []byte) {
	var r io.Reader
	if cuerpo != nil {
		b, _ := json.Marshal(cuerpo)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, c.base+ruta, r)
	if err != nil {
		falla("%v", err)
	}
	if metodo != http.MethodGet {
		req.Header.Set("X-PGO", "1")
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		falla("%s %s: %v", metodo, ruta, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// exige pide y comprueba el estado; si destino no es nil, decodifica el JSON.
func (c *cliente) exige(metodo, ruta string, cuerpo any, estado int, destino any) []byte {
	cod, b := c.pide(metodo, ruta, cuerpo)
	if cod != estado {
		falla("%s %s: estado %d, quiero %d: %s", metodo, ruta, cod, estado, strings.TrimSpace(string(b)))
	}
	if destino != nil {
		if err := json.Unmarshal(b, destino); err != nil {
			falla("%s %s: respuesta no válida (%v): %s", metodo, ruta, err, b)
		}
	}
	return b
}

// ---- la prueba ----

func (e *entorno) ejecuta() {
	t0 := time.Now()
	modulo := raizModulo()
	bin := filepath.Join(e.tmp, "bin")
	e.compila(modulo, filepath.Join(bin, "odoo-prueba"), "./cmd/odoo-prueba", "")
	e.compila(modulo, filepath.Join(bin, "pgocode-backend"), "./cmd/pgocode", "-X main.version=e2e")
	paso("compilados odoo-prueba y pgocode (%s)", time.Since(t0).Round(time.Millisecond))

	// Raíz con un repositorio git del proyecto de prueba; HOME y datos de pgocode, en el temporal.
	casa := filepath.Join(e.tmp, "casa")
	datos := filepath.Join(e.tmp, "datos")
	repo := filepath.Join(e.tmp, "raiz", "proyecto-prueba")
	for _, d := range []string{casa, datos, repo} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			falla("%v", err)
		}
	}
	cfg := fmt.Sprintf(`{"odoo_project_id": %d, "odoo_project_name": "PRUEBA PGOCODE"}`+"\n", proyecto)
	if err := os.WriteFile(filepath.Join(repo, ".planesgo.json"), []byte(cfg), 0o644); err != nil {
		falla("%v", err)
	}
	ejecutaGit(repo, "init", "-q")
	ejecutaGit(repo, "add", ".")
	ejecutaGit(repo, "commit", "-q", "-m", "inicio")

	ent := e.entornoLimpio(casa)
	po := puertoLibre()
	e.odoo = arranca("odoo-prueba", filepath.Join(e.tmp, "odoo.log"), ent, filepath.Join(bin, "odoo-prueba"),
		"-addr", "127.0.0.1:"+po, "-proyecto", fmt.Sprint(proyecto))
	e.puerto = puertoLibre()
	e.backend = arranca("pgocode", filepath.Join(e.tmp, "backend.log"), ent, filepath.Join(bin, "pgocode-backend"),
		"-addr", "127.0.0.1:"+e.puerto, "-odoo", "http://127.0.0.1:"+po, "-db", "prueba",
		"-raices", filepath.Join(e.tmp, "raiz"), "-studio", "", "-actualizaciones", "", "-datos", datos, "-navegador")

	jar, _ := cookiejar.New(nil)
	c := &cliente{base: "http://127.0.0.1:" + e.puerto, http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
	sano := func(url string) bool {
		r, err := http.Get(url)
		if err != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode < 500
	}
	if !espera(30*time.Second, func() bool { return sano("http://127.0.0.1:" + po + "/lineas") }) {
		falla("el Odoo simulado no arranca")
	}
	if !espera(30*time.Second, func() bool { return sano(c.base + "/healthz") }) {
		falla("el backend no responde en /healthz")
	}
	paso("backend en %s y Odoo simulado en :%s", c.base, po)

	// Sesión.
	if cod, _ := c.pide("GET", "/api/yo", nil); cod != http.StatusUnauthorized {
		falla("/api/yo sin sesión: %d, quiero 401", cod)
	}
	if cod, _ := c.pide("POST", "/api/login", map[string]string{"login": "prueba"}); cod != http.StatusBadRequest {
		falla("login sin contraseña: %d, quiero 400", cod)
	}
	var yo struct{ Nombre, Login string }
	c.exige("POST", "/api/login", map[string]string{"login": "prueba", "password": "prueba"}, 200, &yo)
	c.exige("GET", "/api/yo", nil, 200, &yo)
	if yo.Login != "prueba" {
		falla("/api/yo: %+v", yo)
	}
	paso("sesión iniciada como %q", yo.Nombre)

	// Proyectos: el 999 con la carpeta del repositorio de prueba.
	var ps []struct {
		ID       int      `json:"id"`
		Nombre   string   `json:"nombre"`
		Carpetas []string `json:"carpetas"`
	}
	c.exige("GET", "/api/proyectos", nil, 200, &ps)
	var carpetas []string
	for _, p := range ps {
		if p.ID == proyecto {
			carpetas = p.Carpetas
		}
	}
	if len(carpetas) != 1 || carpetas[0] != repo {
		falla("el proyecto %d no tiene la carpeta %s: %+v", proyecto, repo, ps)
	}
	paso("proyecto %d con su carpeta", proyecto)

	// Programa inocuo: se lanza, se ve en curso, se para y no queda vivo.
	if cod, _ := c.pide("POST", "/api/procesos/lanzar", map[string]any{"proyecto": proyecto, "carpeta": e.tmp, "nombre": "fuera", "orden": "true"}); cod != http.StatusForbidden {
		falla("lanzar en una carpeta de fuera del proyecto: %d, quiero 403", cod)
	}
	c.exige("POST", "/api/procesos/lanzar", map[string]any{"proyecto": proyecto, "carpeta": repo, "nombre": "e2e", "orden": ordenInocua}, 200, nil)
	estado := func() string {
		var l struct {
			Procesos []struct{ Nombre, Estado string } `json:"procesos"`
		}
		c.exige("GET", fmt.Sprintf("/api/procesos?proyecto=%d", proyecto), nil, 200, &l)
		for _, p := range l.Procesos {
			if p.Nombre == "e2e" {
				return p.Estado
			}
		}
		return ""
	}
	if !espera(10*time.Second, func() bool { return estado() == "curso" && len(e.programas()) > 0 }) {
		falla("el programa no está en curso (estado %q, procesos %v)", estado(), e.programas())
	}
	paso("programa lanzado (%s)", ordenInocua)
	c.exige("POST", "/api/procesos/parar", map[string]any{"nombre": "e2e"}, 200, nil)
	if !espera(15*time.Second, func() bool { return estado() == "parado" && len(e.programas()) == 0 }) {
		falla("el programa no se para (estado %q, procesos vivos %v)", estado(), e.programas())
	}
	paso("programa parado y sin procesos vivos")

	// Actividad humana en una sesión del proyecto: abre un periodo de imputación.
	sesion := "e2e-" + fmt.Sprint(time.Now().UnixNano())
	c.exige("POST", "/api/actividad", map[string]any{"sesion": sesion, "titulo": "Prueba de humo e2e", "proyecto": proyecto, "humana": true}, http.StatusNoContent, nil)
	var imp struct {
		Hoy      []json.RawMessage `json:"hoy"`
		Abiertos []json.RawMessage `json:"abiertos"`
	}
	c.exige("GET", "/api/imputacion", nil, 200, &imp)
	if len(imp.Abiertos) == 0 {
		falla("tras la actividad humana no hay ningún periodo abierto")
	}
	paso("imputación: %d abiertos, %d líneas hoy", len(imp.Abiertos), len(imp.Hoy))

	// Validación del registro de tiempos: sin errores.
	var val struct {
		Fichero string            `json:"fichero"`
		Sucesos int               `json:"sucesos"`
		Errores *int              `json:"errores"`
		Avisos  []json.RawMessage `json:"avisos"`
	}
	espera(10*time.Second, func() bool {
		c.exige("GET", "/api/imputacion/validar", nil, 200, &val)
		return val.Sucesos > 0
	})
	if val.Sucesos == 0 || val.Errores == nil {
		falla("el registro de tiempos está vacío (%s)", val.Fichero)
	}
	if *val.Errores != 0 {
		b := c.exige("GET", "/api/imputacion/validar", nil, 200, nil)
		falla("la validación de tiempos tiene %d errores: %s", *val.Errores, b)
	}
	paso("tiempos validados: %d sucesos, 0 errores, %d avisos", val.Sucesos, len(val.Avisos))

	// Cierre de sesión.
	c.exige("POST", "/api/logout", nil, 200, nil)
	if cod, _ := c.pide("GET", "/api/yo", nil); cod != http.StatusUnauthorized {
		falla("/api/yo tras cerrar sesión: %d, quiero 401", cod)
	}
	paso("sesión cerrada")

	// Apagado limpio (cierra los periodos) y validación del registro completo con la propia orden de pgocode.
	if !e.paraBackend(15 * time.Second) {
		falla("el backend no se cierra con SIGTERM")
	}
	if val.Fichero != "" {
		out, err := exec.Command(filepath.Join(bin, "pgocode-backend"), "-valida-tiempos", val.Fichero).CombinedOutput()
		if err != nil {
			falla("pgocode -valida-tiempos tras el cierre: %v\n%s", err, out)
		}
	}
	paso("backend cerrado y registro de tiempos válido (%s)", time.Since(t0).Round(time.Millisecond))
}

// programas: pids de la orden inocua lanzada por este backend (se reconoce por PGO_API en su entorno).
// Solo en Linux (/proc); en otros sistemas no se comprueba.
func (e *entorno) programas() []int {
	if runtime.GOOS != "linux" || e.puerto == "" {
		return nil
	}
	l, _ := filepath.Glob("/proc/[0-9]*")
	var pids []int
	marca := []byte("PGO_API=http://127.0.0.1:" + e.puerto + "\x00")
	orden := []byte(strings.ReplaceAll(ordenInocua, " ", "\x00"))
	for _, d := range l {
		cmd, err := os.ReadFile(filepath.Join(d, "cmdline"))
		if err != nil || !bytes.Contains(cmd, orden) {
			continue
		}
		env, err := os.ReadFile(filepath.Join(d, "environ"))
		if err != nil || !bytes.Contains(env, marca) {
			continue
		}
		var pid int
		fmt.Sscan(filepath.Base(d), &pid)
		pids = append(pids, pid)
	}
	return pids
}

// paraBackend manda SIGTERM al backend y espera a que salga.
func (e *entorno) paraBackend(max time.Duration) bool {
	if e.backend == nil || e.backend.Process == nil {
		return true
	}
	syscall.Kill(e.backend.Process.Pid, syscall.SIGTERM)
	return espera(max, func() bool { return !vivo(e.backend.Process.Pid) })
}

func vivo(pid int) bool {
	// Kill(pid, 0) también da bien con un zombi: se mira el estado en /proc si se puede.
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if i := bytes.LastIndexByte(b, ')'); i > 0 && i+2 < len(b) && b[i+2] == 'Z' {
			return false
		}
	}
	return true
}

// limpia mata todo lo arrancado (grupos enteros) y borra el temporal. Se puede llamar varias veces.
func (e *entorno) limpia() {
	e.limpieza.Do(func() {
		e.mu.Lock()
		if g := e.compilando; g != 0 {
			syscall.Kill(-g, syscall.SIGKILL)
			espera(5*time.Second, func() bool { return syscall.Kill(-g, 0) != nil })
		}
		e.mu.Unlock()
		if e.backend != nil && e.backend.Process != nil {
			if !e.paraBackend(10 * time.Second) {
				log.Print("el backend no se cierra: SIGKILL")
			}
			syscall.Kill(-e.backend.Process.Pid, syscall.SIGKILL)
		}
		for _, pid := range e.programas() { // por si el backend murió sin parar sus programas
			syscall.Kill(-pid, syscall.SIGKILL)
			syscall.Kill(pid, syscall.SIGKILL)
		}
		if e.odoo != nil && e.odoo.Process != nil {
			syscall.Kill(-e.odoo.Process.Pid, syscall.SIGKILL)
		}
		if !*conservar {
			if err := os.RemoveAll(e.tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("no se pudo borrar %s: %v", e.tmp, err)
			}
		}
	})
}

// muestraRegistros enseña el final de los registros tras un fallo (o enteros con -v).
func (e *entorno) muestraRegistros() {
	for _, n := range []string{"backend.log", "odoo.log"} {
		b, err := os.ReadFile(filepath.Join(e.tmp, n))
		if err != nil {
			continue
		}
		l := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if !*verboso && len(l) > 40 {
			l = l[len(l)-40:]
		}
		fmt.Fprintf(os.Stderr, "---- %s ----\n%s\n", n, strings.Join(l, "\n"))
	}
}
