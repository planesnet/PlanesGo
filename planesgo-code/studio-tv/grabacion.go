package studiotv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Grabación en el propio estudio: el directo y la grabación van separados. Quien mira decide si se graba
// (por defecto, no). Mientras se graba, el último fotograma del canal se escribe a ffmpeg a 10 imágenes por
// segundo constantes (una pantalla quieta cuesta casi nada en H.264) y los pasos que llegan en el estado se
// guardan como marcadores. Cada grabación deja <dir>/<canal>/<id>.mp4 y <id>.json.

const fpsGrabacion = 10

var idGrabacionValido = regexp.MustCompile(`^\d{8}-\d{6}$`)

// Marca es un marcador del vídeo: segundo y texto del paso.
type Marca struct {
	T   float64 `json:"t"`
	Txt string  `json:"txt"`
}

// Grabacion describe un vídeo guardado.
type Grabacion struct {
	ID         string    `json:"id"`
	Canal      string    `json:"canal"`
	Inicio     time.Time `json:"inicio"`
	Dur        float64   `json:"dur"`
	Bytes      int64     `json:"bytes"`
	Marcadores []Marca   `json:"marcadores"`
}

type grabadora struct {
	g      Grabacion
	mp4    string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	salida bytes.Buffer

	mu     sync.Mutex
	ultimo []byte
	paso   string
	parar  chan struct{}
	hecho  chan struct{}
	err    error
}

// GrabarEn activa la grabación guardando los vídeos en dir. Sin llamarla, el estudio solo emite en directo.
func (s *Studio) GrabarEn(dir string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("grabación desactivada: falta ffmpeg")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.mu.Lock()
	s.dirGrab = dir
	s.mu.Unlock()
	return nil
}

// empiezaGrabacion arranca la grabación del canal (que tiene que existir). Se llama con s.mu tomado.
func (s *Studio) empiezaGrabacionLocked(nombre string) (*grabadora, error) {
	if s.dirGrab == "" {
		return nil, errNoGraba
	}
	c, ok := s.canales[nombre]
	if !ok {
		return nil, errNoEncontrado
	}
	if c.rec != nil {
		return c.rec, nil
	}
	dir := filepath.Join(s.dirGrab, nombre)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	inicio := s.ahora()
	id := inicio.Format("20060102-150405")
	r := &grabadora{g: Grabacion{ID: id, Canal: nombre, Inicio: inicio, Marcadores: []Marca{}},
		mp4: filepath.Join(dir, id+".mp4"), ultimo: c.frame, parar: make(chan struct{}), hecho: make(chan struct{})}
	r.cmd = exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "image2pipe", "-c:v", "mjpeg", "-framerate", fmt.Sprint(fpsGrabacion), "-i", "-",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", "-c:v", "libx264", "-preset", "medium", "-tune", "stillimage",
		"-crf", "30", "-pix_fmt", "yuv420p", "-movflags", "+faststart", r.mp4)
	r.cmd.Stderr = &r.salida
	in, err := r.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	r.stdin = in
	if err := r.cmd.Start(); err != nil {
		return nil, err
	}
	if p := pasoDe(c.estado); p != "" {
		r.paso = p
		r.g.Marcadores = append(r.g.Marcadores, Marca{T: 0, Txt: p})
	}
	c.rec = r
	go r.bucle()
	return r, nil
}

// bucle escribe el último fotograma a ritmo constante hasta que se para; luego cierra el vídeo y su ficha.
func (r *grabadora) bucle() {
	defer close(r.hecho)
	tic := time.NewTicker(time.Second / fpsGrabacion)
	defer tic.Stop()
	escrito := 0
	for {
		select {
		case <-r.parar:
			r.stdin.Close()
			r.err = r.cmd.Wait()
			r.g.Dur = float64(escrito) / fpsGrabacion
			if escrito == 0 {
				os.Remove(r.mp4)
				r.err = errors.New("no llegó ninguna imagen mientras se grababa")
				return
			}
			if r.err != nil {
				r.err = fmt.Errorf("ffmpeg: %v %s", r.err, strings.TrimSpace(r.salida.String()))
				return
			}
			if st, err := os.Stat(r.mp4); err == nil {
				r.g.Bytes = st.Size()
			}
			b, _ := json.MarshalIndent(r.g, "", " ")
			r.err = os.WriteFile(strings.TrimSuffix(r.mp4, ".mp4")+".json", b, 0o644)
			return
		case <-tic.C:
			r.mu.Lock()
			f := r.ultimo
			r.mu.Unlock()
			if f == nil {
				continue
			}
			if _, err := r.stdin.Write(f); err != nil {
				r.err = err
				<-r.parar
				r.stdin.Close()
				r.cmd.Wait()
				return
			}
			escrito++
		}
	}
}

func (r *grabadora) frame(b []byte) {
	r.mu.Lock()
	r.ultimo = b
	r.mu.Unlock()
}

// estado añade un marcador cada vez que cambia el paso.
func (r *grabadora) estado(b []byte, ahora time.Time) {
	p := pasoDe(b)
	r.mu.Lock()
	defer r.mu.Unlock()
	if p != "" && p != r.paso {
		r.paso = p
		r.g.Marcadores = append(r.g.Marcadores, Marca{T: float64(ahora.Sub(r.g.Inicio).Milliseconds()) / 1000, Txt: p})
	}
}

func pasoDe(estado []byte) string {
	var e struct {
		Paso string `json:"paso"`
	}
	json.Unmarshal(estado, &e)
	return e.Paso
}

// paraGrabacion detiene la grabación del canal y espera a que el vídeo quede cerrado.
func (s *Studio) paraGrabacion(nombre string) (*Grabacion, error) {
	s.mu.Lock()
	c, ok := s.canales[nombre]
	if !ok || c.rec == nil {
		s.mu.Unlock()
		return nil, errNoGrabando
	}
	r := c.rec
	c.rec = nil
	s.mu.Unlock()
	close(r.parar)
	<-r.hecho
	if r.err != nil {
		return nil, r.err
	}
	return &r.g, nil
}

// Cierra detiene todas las grabaciones en curso (al apagar el estudio), para que los vídeos queden bien.
func (s *Studio) Cierra() {
	s.mu.Lock()
	var nombres []string
	for n, c := range s.canales {
		if c.rec != nil {
			nombres = append(nombres, n)
		}
	}
	s.mu.Unlock()
	for _, n := range nombres {
		s.paraGrabacion(n)
	}
}

// grabaciones lista los vídeos de un canal, el más reciente primero.
func (s *Studio) grabaciones(nombre string) ([]Grabacion, error) {
	s.mu.Lock()
	dir := s.dirGrab
	s.mu.Unlock()
	if dir == "" {
		return nil, errNoGraba
	}
	fichas, _ := filepath.Glob(filepath.Join(dir, nombre, "*.json"))
	l := []Grabacion{}
	for _, f := range fichas {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var g Grabacion
		if json.Unmarshal(b, &g) == nil && idGrabacionValido.MatchString(g.ID) {
			l = append(l, g)
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].ID > l[j].ID })
	return l, nil
}

// rutaGrabacion devuelve el MP4 de una grabación (id validado: no se sale del directorio).
func (s *Studio) rutaGrabacion(nombre, id string) (string, error) {
	s.mu.Lock()
	dir := s.dirGrab
	s.mu.Unlock()
	if dir == "" {
		return "", errNoGraba
	}
	if !idGrabacionValido.MatchString(id) {
		return "", errNoEncontrado
	}
	p := filepath.Join(dir, nombre, id+".mp4")
	if _, err := os.Stat(p); err != nil {
		return "", errNoEncontrado
	}
	return p, nil
}
