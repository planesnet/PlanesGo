// Package depuracion escribe un registro detallado (depuracion.log) para analizar qué pasa con los agentes: la
// orden lanzada, el entorno añadido, cada línea de salida tal cual, el error estándar, el código de salida y los
// tiempos. Es un fichero local (0600) que rota al llegar a 5 MB.
package depuracion

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const maxTam = 5 << 20

var (
	mu      sync.Mutex
	fichero string
	f       *os.File
)

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
	return err
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
	if st, err := f.Stat(); err == nil && st.Size() > maxTam {
		f.Close()
		os.Rename(fichero, fichero+".1")
		if abre() != nil {
			f = nil
			return
		}
	}
	fmt.Fprintf(f, "%s [%s] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), cat, fmt.Sprintf(formato, args...))
}
