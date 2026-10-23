// Package selector elige la skill adecuada para una petición con embeddings (en local con Ollama y
// embeddinggemma; si Ollama no está, en Vertex AI con gemini-embedding-001):
// cada skill tiene un vector de su nombre y descripción (calculado una vez y guardado en disco); la petición se
// convierte en otro vector y se comparan por similitud de coseno. Así pgocode sabe qué skill usar antes de llamar
// al agente, en unos milisegundos y sin gastar tokens del agente.
package selector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"planesgo-code/internal/google"
	"planesgo-code/internal/skills"
)

const (
	modeloVertex = "gemini-embedding-001"
	modeloLocal  = "embeddinggemma"
	region       = "europe-west1"
	dimensiones  = 256
	ollama       = "http://127.0.0.1:11434"
)

// Motor es quien calcula los vectores: su nombre (va en la huella: los vectores de dos motores no se mezclan)
// y su umbral (calibrado con las skills reales para cada modelo).
type Motor struct {
	Nombre string
	Umbral float64
	Embebe func(ctx context.Context, texto, tarea string) ([]float32, error)
}

// Candidato es una skill con su parecido a la petición (0…1).
type Candidato struct {
	Nombre     string  `json:"nombre"`
	Ruta       string  `json:"ruta"`
	Puntuacion float64 `json:"puntuacion"`
}

type vector struct {
	Huella string    `json:"huella"` // sha256 de nombre + descripción: si cambia, se recalcula
	V      []float32 `json:"v"`
}

// Selector guarda los vectores de las skills (en Fichero) y calcula los de las peticiones.
type Selector struct {
	Fichero string
	HTTP    *http.Client
	// Embebe se puede sustituir en las pruebas (motor «prueba»).
	Embebe func(ctx context.Context, texto, tarea string) ([]float32, error)
	// SoloVertex: no usar Ollama aunque esté.
	SoloVertex bool

	mu       sync.Mutex
	vectores map[string]vector // por ruta de SKILL.md
	cargado  bool
}

func (s *Selector) carga() {
	if s.cargado {
		return
	}
	s.cargado, s.vectores = true, map[string]vector{}
	if b, err := os.ReadFile(s.Fichero); err == nil {
		json.Unmarshal(b, &s.vectores)
	}
}

func (s *Selector) guarda() {
	if s.Fichero == "" {
		return
	}
	b, _ := json.Marshal(s.vectores)
	tmp := s.Fichero + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, s.Fichero)
	}
}

func huella(motor string, k skills.Skill) string {
	h := sha256.Sum256([]byte(motor + "\x00" + k.Nombre + "\x00" + k.Descripcion))
	return hex.EncodeToString(h[:8])
}

// MotorActual: la IA local si responde y tiene el modelo; si no, Vertex AI.
func (s *Selector) MotorActual(ctx context.Context) Motor {
	if s.Embebe != nil {
		return Motor{"prueba", 0.5, s.Embebe}
	}
	if !s.SoloVertex && ollamaListo(ctx) {
		return Motor{localMotorName(), UmbralLocal, func(ctx context.Context, t, tarea string) ([]float32, error) { return Ollama(ctx, s.HTTP, t, tarea) }}
	}
	return Motor{"vertex/" + modeloVertex, UmbralVertex, func(ctx context.Context, t, tarea string) ([]float32, error) { return Vertex(ctx, s.HTTP, t, tarea) }}
}

// Umbrales calibrados con las skills reales (peticiones con skill clara frente a peticiones sin ninguna).
const (
	UmbralVertex = 0.74
	UmbralLocal  = 0.33
)

func textoSkill(k skills.Skill) string { return k.Nombre + ": " + k.Descripcion }

// Prepara calcula los vectores que falten (skills nuevas o cambiadas), varios a la vez.
func (s *Selector) Prepara(ctx context.Context, l []skills.Skill) error {
	return s.prepara(ctx, s.MotorActual(ctx), l)
}

func (s *Selector) prepara(ctx context.Context, m Motor, l []skills.Skill) error {
	s.mu.Lock()
	s.carga()
	var falta []skills.Skill
	for _, k := range l {
		if k.Rota || k.Descripcion == "" {
			continue
		}
		if v, ok := s.vectores[k.Ruta]; !ok || v.Huella != huella(m.Nombre, k) {
			falta = append(falta, k)
		}
	}
	s.mu.Unlock()
	if len(falta) == 0 {
		return nil
	}
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		primer error
		sem    = make(chan struct{}, 8)
	)
	for _, k := range falta {
		wg.Add(1)
		go func(k skills.Skill) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			v, err := m.Embebe(ctx, textoSkill(k), "RETRIEVAL_DOCUMENT")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if primer == nil {
					primer = err
				}
				return
			}
			s.mu.Lock()
			s.vectores[k.Ruta] = vector{huella(m.Nombre, k), v}
			s.mu.Unlock()
		}(k)
	}
	wg.Wait()
	s.mu.Lock()
	s.guarda()
	s.mu.Unlock()
	return primer
}

// Elige ordena las skills por parecido a la petición (solo las que tienen vector) y dice el motor usado.
func (s *Selector) Elige(ctx context.Context, peticion string, l []skills.Skill) ([]Candidato, Motor, error) {
	m := s.MotorActual(ctx)
	if err := s.prepara(ctx, m, l); err != nil && len(s.vectores) == 0 {
		return nil, m, err
	}
	q, err := m.Embebe(ctx, peticion, "RETRIEVAL_QUERY")
	if err != nil {
		return nil, m, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var c []Candidato
	for _, k := range l {
		if v, ok := s.vectores[k.Ruta]; ok && v.Huella == huella(m.Nombre, k) {
			c = append(c, Candidato{k.Nombre, k.Ruta, coseno(q, v.V)})
		}
	}
	sort.Slice(c, func(i, j int) bool { return c[i].Puntuacion > c[j].Puntuacion })
	return c, m, nil
}

func coseno(a, b []float32) float64 {
	var p, na, nb float64
	for i := range a {
		if i >= len(b) {
			break
		}
		p += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return p / math.Sqrt(na*nb)
}

// ollamaListo: la IA local (la configurada o, si no responde, el Ollama del equipo) responde y tiene el modelo de
// embeddings (se comprueba como mucho cada 30 s).
func ollamaListo(ctx context.Context) bool {
	_, e := activa(ctx)
	return e.Ready
}

// Ollama calcula un embedding con la IA local (localai.go: por defecto el Ollama del equipo con embeddinggemma),
// con sus prefijos de búsqueda (consulta / documento). En Ollama el modelo se queda cargado 30 minutos.
func Ollama(ctx context.Context, cli *http.Client, texto, tarea string) ([]float32, error) {
	if cli == nil {
		cli = &http.Client{Timeout: 60 * time.Second}
	}
	if len(texto) > 6000 {
		texto = texto[:6000]
	}
	if tarea == "RETRIEVAL_QUERY" {
		texto = "task: search result | query: " + texto
	} else {
		texto = "title: none | text: " + texto
	}
	return embedLocal(ctx, cli, texto)
}

// Precarga deja el modelo local cargado (la primera llamada tarda unos segundos).
func (s *Selector) Precarga(ctx context.Context) {
	if m := s.MotorActual(ctx); m.Nombre == localMotorName() {
		m.Embebe(ctx, "hola", "RETRIEVAL_QUERY")
	}
}

// Vertex calcula un embedding con gemini-embedding-001 en Vertex AI.
func Vertex(ctx context.Context, cli *http.Client, texto, tarea string) ([]float32, error) {
	t, err := google.Token()
	if err != nil {
		return nil, fmt.Errorf("sin credenciales de Google (gcloud auth application-default login): %w", err)
	}
	if cli == nil {
		cli = &http.Client{Timeout: 20 * time.Second}
	}
	if len(texto) > 6000 {
		texto = texto[:6000]
	}
	cuerpo, _ := json.Marshal(map[string]any{
		"instances":  []map[string]string{{"content": texto, "task_type": tarea}},
		"parameters": map[string]any{"outputDimensionality": dimensiones},
	})
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict", region, google.Proyecto(), region, modeloVertex)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(cuerpo))
	req.Header.Set("Authorization", "Bearer "+t)
	req.Header.Set("Content-Type", "application/json")
	res, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var r struct {
		Predictions []struct {
			Embeddings struct {
				Values []float32 `json:"values"`
			} `json:"embeddings"`
		} `json:"predictions"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("embeddings: %s", res.Status)
	}
	if r.Error != nil {
		return nil, errors.New("embeddings: " + r.Error.Message)
	}
	if len(r.Predictions) == 0 {
		return nil, errors.New("embeddings: respuesta vacía")
	}
	return r.Predictions[0].Embeddings.Values, nil
}
