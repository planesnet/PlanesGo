// Package actualizacion mantiene Planes Go Code al día con lo que publica PlanesGo.
//
// Cada minuto consulta <URL>/version.json. Si la huella del paquete (sha256) no es la instalada, lo descarga,
// comprueba la huella y lo deja preparado en <DirPreparado>/nuevo. Cuando la app lleva un rato sin hacer nada
// (sin chats ni agentes en curso, sin directo ni grabación en el estudio local), llama a AlAplicar: el backend
// guarda las sesiones y sale con el código SalidaActualizar, y la app de escritorio cambia los ficheros y se
// vuelve a abrir. Si la app no está instalada desde un paquete (desarrollo), no hace nada.
package actualizacion

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SalidaActualizar es el código de salida con el que el backend pide a la app de escritorio que aplique la
// actualización preparada y vuelva a abrirse.
const SalidaActualizar = 75

const (
	maxPaquete  = 200 << 20
	quietoAntes = 30 * time.Second // tiempo seguido sin actividad antes de actualizar
)

// Publicada es lo que anuncia PlanesGo en version.json.
type Publicada struct {
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Bytes    int64  `json:"bytes"`
	Archivo  string `json:"archivo"`
	Electron string `json:"electron"`
}

// Estado es lo que ve la interfaz.
type Estado struct {
	Version  string    `json:"version"`         // instalada
	Estado   string    `json:"estado"`          // desactivada | al-dia | descargando | pendiente | aplicando | manual | error
	Nueva    string    `json:"nueva,omitempty"` // versión preparada o disponible
	Error    string    `json:"error,omitempty"`
	Revisado time.Time `json:"revisado,omitempty"`
	Pausada  bool      `json:"pausada"` // el usuario ha pausado las actualizaciones automáticas
}

// Actualizador revisa, descarga y aplica las actualizaciones.
type Actualizador struct {
	URL          string        // p. ej. https://planesgo.autopyme.com/install/pgocode
	DirApp       string        // carpeta de la app instalada (donde está pgocode-backend)
	DirPreparado string        // dónde se deja la versión nueva (…/actualizacion)
	Electron     string        // versión de Electron en marcha (si el paquete pide otra, hay que reinstalar)
	Intervalo    time.Duration // cada cuánto se revisa (60 s)
	Ocupado      func() bool   // ¿hay algo en curso?
	AlAplicar    func()        // guarda el estado y sale con SalidaActualizar
	HTTP         *http.Client

	mu        sync.Mutex
	estado    Estado
	instalado string // sha256 del paquete instalado
	buscar    chan struct{}
	aplicar   chan struct{}
}

// Nuevo prepara el actualizador leyendo la versión y la huella instaladas en dirApp.
func Nuevo(url, dirApp, dirPreparado, version string) *Actualizador {
	a := &Actualizador{URL: strings.TrimRight(url, "/"), DirApp: dirApp, DirPreparado: dirPreparado,
		Intervalo: time.Minute, HTTP: &http.Client{Timeout: 5 * time.Minute}, buscar: make(chan struct{}, 1), aplicar: make(chan struct{}, 1)}
	a.estado.Version = version
	b, err := os.ReadFile(filepath.Join(dirApp, "paquete.sha256"))
	a.instalado = strings.TrimSpace(string(b))
	switch {
	case a.URL == "":
		a.estado.Estado, a.estado.Error = "desactivada", "sin servidor de actualizaciones"
	case err != nil || a.instalado == "" || a.instalado == "desarrollo":
		a.estado.Estado, a.estado.Error = "desactivada", "versión de desarrollo (no instalada desde un paquete)"
	default:
		a.estado.Estado = "al-dia"
	}
	if _, err := os.Stat(filepath.Join(dirPreparado, "pausada")); err == nil {
		a.estado.Pausada = true
	}
	return a
}

// Estado devuelve una copia del estado actual.
func (a *Actualizador) Estado() Estado {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.estado
}

// Pausa activa o quita la pausa de las actualizaciones automáticas (se recuerda en DirPreparado/pausada).
// Con la pausa se siguen descargando, pero solo se instalan si el usuario lo pide.
func (a *Actualizador) Pausa(p bool) {
	f := filepath.Join(a.DirPreparado, "pausada")
	if p {
		os.MkdirAll(a.DirPreparado, 0o755)
		os.WriteFile(f, nil, 0o600)
	} else {
		os.Remove(f)
	}
	a.pon(func(e *Estado) { e.Pausada = p })
}

// BuscarYa pide una revisión inmediata.
// AplicarYa instala ya la versión preparada (si la hay), sin esperar a que la app esté parada.
func (a *Actualizador) AplicarYa() bool {
	if a.Estado().Estado != "pendiente" {
		return false
	}
	select {
	case a.aplicar <- struct{}{}:
	default:
	}
	return true
}

func (a *Actualizador) BuscarYa() {
	select {
	case a.buscar <- struct{}{}:
	default:
	}
}

func (a *Actualizador) pon(f func(*Estado)) {
	a.mu.Lock()
	f(&a.estado)
	a.mu.Unlock()
}

// Funciona revisa cada Intervalo y aplica cuando toca. Termina al cancelar ctx.
func (a *Actualizador) Funciona(ctx context.Context) {
	if a.Estado().Estado == "desactivada" {
		return
	}
	revisa := time.NewTimer(10 * time.Second) // primera revisión poco después de arrancar
	defer revisa.Stop()
	mira := time.NewTicker(5 * time.Second)
	defer mira.Stop()
	var quietoDesde time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.buscar:
			a.revisa(ctx)
		case <-a.aplicar:
			// El usuario la pide ya (pulsando el aviso): se aplica aunque haya algo en curso.
			if a.Estado().Estado == "pendiente" && a.AlAplicar != nil {
				a.pon(func(e *Estado) { e.Estado = "aplicando" })
				a.AlAplicar()
				return
			}
		case <-revisa.C:
			a.revisa(ctx)
			revisa.Reset(a.Intervalo)
		case <-mira.C:
			if a.Estado().Estado != "pendiente" {
				quietoDesde = time.Time{}
				continue
			}
			if a.Estado().Pausada || (a.Ocupado != nil && a.Ocupado()) {
				quietoDesde = time.Time{}
				continue
			}
			if quietoDesde.IsZero() {
				quietoDesde = time.Now()
			}
			if time.Since(quietoDesde) >= quietoAntes && a.AlAplicar != nil {
				a.pon(func(e *Estado) { e.Estado = "aplicando" })
				a.AlAplicar()
				return
			}
		}
	}
}

// revisa consulta la versión publicada y, si es otra, la descarga y la deja preparada.
func (a *Actualizador) revisa(ctx context.Context) {
	if e := a.Estado().Estado; e == "aplicando" || e == "descargando" {
		return
	}
	p, err := a.publicada(ctx)
	a.pon(func(e *Estado) { e.Revisado = time.Now() })
	if err != nil {
		a.pon(func(e *Estado) { e.Error = err.Error() }) // se mantiene el estado; se vuelve a intentar
		return
	}
	if p.SHA256 == a.instalado {
		a.pon(func(e *Estado) { e.Estado, e.Nueva, e.Error = "al-dia", "", "" })
		return
	}
	if a.Electron != "" && p.Electron != "" && p.Electron != a.Electron {
		a.pon(func(e *Estado) {
			e.Estado, e.Nueva = "manual", p.Version
			e.Error = fmt.Sprintf("la versión %s necesita Electron %s: hay que reinstalar Planes Go Code", p.Version, p.Electron)
		})
		return
	}
	if b, err := os.ReadFile(filepath.Join(a.DirPreparado, "nuevo", "paquete.sha256")); err == nil && strings.TrimSpace(string(b)) == p.SHA256 {
		a.pon(func(e *Estado) { e.Estado, e.Nueva, e.Error = "pendiente", p.Version, "" })
		return
	}
	a.pon(func(e *Estado) { e.Estado, e.Nueva, e.Error = "descargando", p.Version, "" })
	if err := a.prepara(ctx, p); err != nil {
		a.pon(func(e *Estado) { e.Estado, e.Error = "error", err.Error() })
		return
	}
	a.pon(func(e *Estado) { e.Estado, e.Error = "pendiente", "" })
}

func (a *Actualizador) publicada(ctx context.Context) (Publicada, error) {
	var p Publicada
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/version.json", nil)
	if err != nil {
		return p, err
	}
	r, err := a.HTTP.Do(req)
	if err != nil {
		return p, fmt.Errorf("no se pudo consultar las actualizaciones: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return p, fmt.Errorf("servidor de actualizaciones: %s", r.Status)
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&p); err != nil {
		return p, fmt.Errorf("version.json no válido: %w", err)
	}
	if len(p.SHA256) != 64 || p.Archivo == "" || strings.ContainsAny(p.Archivo, "/\\") {
		return p, errors.New("version.json incompleto")
	}
	return p, nil
}

// prepara descarga el paquete, comprueba la huella y lo extrae en DirPreparado/nuevo.
func (a *Actualizador) prepara(ctx context.Context, p Publicada) error {
	if err := os.MkdirAll(a.DirPreparado, 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/"+p.Archivo, nil)
	if err != nil {
		return err
	}
	r, err := a.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("descarga: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("descarga: %s", r.Status)
	}
	tmp, err := os.CreateTemp(a.DirPreparado, "paquete-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r.Body, maxPaquete+1))
	tmp.Close()
	if err != nil {
		return fmt.Errorf("descarga: %w", err)
	}
	if n > maxPaquete {
		return errors.New("paquete demasiado grande")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != p.SHA256 {
		return fmt.Errorf("la huella del paquete no coincide (%s…): no se instala", got[:12])
	}
	destino := filepath.Join(a.DirPreparado, "nuevo")
	parcial := destino + ".parcial"
	os.RemoveAll(parcial)
	if err := extrae(tmp.Name(), parcial); err != nil {
		os.RemoveAll(parcial)
		return err
	}
	if _, err := os.Stat(filepath.Join(parcial, "pgocode-backend")); err != nil {
		os.RemoveAll(parcial)
		return errors.New("el paquete no trae pgocode-backend")
	}
	if err := os.WriteFile(filepath.Join(parcial, "paquete.sha256"), []byte(p.SHA256+"\n"), 0o644); err != nil {
		return err
	}
	os.RemoveAll(destino)
	return os.Rename(parcial, destino)
}

// extrae un .tar.gz en dir sin dejar salir nada de dir (sin rutas absolutas, «..» ni enlaces).
func extrae(archivo, dir string) error {
	f, err := os.Open(archivo)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		nombre := filepath.Clean(h.Name)
		if nombre == "." {
			continue
		}
		if filepath.IsAbs(nombre) || nombre == ".." || strings.HasPrefix(nombre, ".."+string(filepath.Separator)) {
			return fmt.Errorf("ruta no permitida en el paquete: %s", h.Name)
		}
		ruta := filepath.Join(dir, nombre)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(ruta, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
				return err
			}
			modo := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				modo = 0o755
			}
			out, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, modo)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(tr, maxPaquete))
			out.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("tipo de fichero no permitido en el paquete: %s", h.Name)
		}
	}
}
