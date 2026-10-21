// Package depuracion escribe un registro detallado (depuracion.log) para analizar qué pasa con los agentes: la
// orden lanzada, el entorno añadido, cada línea de salida tal cual, el error estándar, el código de salida y los
// tiempos. Es un fichero local (0600) que rota al llegar a 5 MB (se guardan los 5 anteriores). Los secretos
// conocidos (tokens, claves de API, cookies, contraseñas) se tapan antes de escribir.
package depuracion

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxTam     = 5 << 20
	anteriores = 5
)

var (
	mu      sync.Mutex
	fichero string
	f       *os.File
	tam     int64
)

// secretos: lo que nunca debe quedar en el registro (se cambia por «***»).
var secretos = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(PGO_TOKEN[A-Z_]*|[A-Z0-9_]*(API_?KEY|SECRET|TOKEN|PASSWORD|PASSWD))=([^\s"']+)`),
	regexp.MustCompile(`(?i)("(password|passwd|api_key|apikey|token|secret|session_id|odoo_sesion)"\s*:\s*")([^"]+)`),
	regexp.MustCompile(`(?i)\b(bearer|session_id=)\s*([A-Za-z0-9._~+/=-]{12,})`),
	regexp.MustCompile(`\b(sk-ant-[A-Za-z0-9_-]{10,}|sk-[A-Za-z0-9]{20,}|AIza[0-9A-Za-z_-]{30,}|gh[pousr]_[A-Za-z0-9]{30,}|ya29\.[0-9A-Za-z_-]{20,})`),
}

// Tapa los secretos conocidos de un texto.
func Tapa(t string) string {
	t = secretos[0].ReplaceAllString(t, "$1=***")
	t = secretos[1].ReplaceAllString(t, "$1***")
	t = secretos[2].ReplaceAllString(t, "$1 ***")
	return secretos[3].ReplaceAllString(t, "***")
}

// Activa empieza a escribir en ruta.
func Activa(ruta string) error {
	mu.Lock()
	defer mu.Unlock()
	fichero = ruta
	return abre()
}

func abre() error {
	var err error
	f, err = os.OpenFile(fichero, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err == nil {
		if st, e := f.Stat(); e == nil {
			tam = st.Size()
		}
	}
	return err
}

// rota: depuracion.log → .1 → … → .5 (el más viejo se borra).
func rota() {
	f.Close()
	os.Remove(fmt.Sprintf("%s.%d", fichero, anteriores))
	for i := anteriores - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", fichero, i), fmt.Sprintf("%s.%d", fichero, i+1))
	}
	os.Rename(fichero, fichero+".1")
	if abre() != nil {
		f = nil
	}
}

// Activo dice si se está registrando.
func Activo() bool {
	mu.Lock()
	defer mu.Unlock()
	return f != nil
}

// Log añade una línea: fecha, categoría y texto.
func Log(cat, formato string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return
	}
	if tam > maxTam {
		if rota(); f == nil {
			return
		}
	}
	n, _ := fmt.Fprintf(f, "%s [%s] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), cat, Tapa(fmt.Sprintf(formato, args...)))
	tam += int64(n)
}

// Escritor: un io.Writer que va al registro con esa categoría (para el log del backend: inicios de sesión,
// programas lanzados, actualizaciones…, que si no se pierden en la salida de Electron).
func Escritor(cat string) *escritor { return &escritor{cat} }

type escritor struct{ cat string }

func (e *escritor) Write(b []byte) (int, error) {
	Log(e.cat, "%s", strings.TrimRight(string(b), "\n"))
	return len(b), nil
}
