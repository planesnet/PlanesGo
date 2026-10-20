// Package selector elige la skill adecuada para una petición con embeddings (Vertex AI, gemini-embedding-001):
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
	modelo      = "gemini-embedding-001"
	region      = "europe-west1"
	dimensiones = 256
)

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
	// Embebe se puede sustituir en las pruebas.
	Embebe func(ctx context.Context, texto, tarea string) ([]float32, error)

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

func huella(k skills.Skill) string {
	h := sha256.Sum256([]byte(modelo + "\x00" + k.Nombre + "\x00" + k.Descripcion))
	return hex.EncodeToString(h[:8])
}

func textoSkill(k skills.Skill) string { return k.Nombre + ": " + k.Descripcion }

// Prepara calcula los vectores que falten (skills nuevas o cambiadas), varios a la vez.
func (s *Selector) Prepara(ctx context.Context, l []skills.Skill) error {
	s.mu.Lock()
	s.carga()
	var falta []skills.Skill
	for _, k := range l {
		if k.Rota || k.Descripcion == "" {
			continue
		}
		if v, ok := s.vectores[k.Ruta]; !ok || v.Huella != huella(k) {
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
			v, err := s.embebe(ctx, textoSkill(k), "RETRIEVAL_DOCUMENT")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if primer == nil {
					primer = err
				}
				return
			}
			s.mu.Lock()
			s.vectores[k.Ruta] = vector{huella(k), v}
			s.mu.Unlock()
		}(k)
	}
	wg.Wait()
	s.mu.Lock()
	s.guarda()
	s.mu.Unlock()
	return primer
}

// Elige ordena las skills por parecido a la petición (solo las que tienen vector).
func (s *Selector) Elige(ctx context.Context, peticion string, l []skills.Skill) ([]Candidato, error) {
	if err := s.Prepara(ctx, l); err != nil && len(s.vectores) == 0 {
		return nil, err
	}
	q, err := s.embebe(ctx, peticion, "RETRIEVAL_QUERY")
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var c []Candidato
	for _, k := range l {
		if v, ok := s.vectores[k.Ruta]; ok && v.Huella == huella(k) {
			c = append(c, Candidato{k.Nombre, k.Ruta, coseno(q, v.V)})
		}
	}
	sort.Slice(c, func(i, j int) bool { return c[i].Puntuacion > c[j].Puntuacion })
	return c, nil
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

func (s *Selector) embebe(ctx context.Context, texto, tarea string) ([]float32, error) {
	if s.Embebe != nil {
		return s.Embebe(ctx, texto, tarea)
	}
	return Vertex(ctx, s.HTTP, texto, tarea)
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
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict", region, google.Proyecto(), region, modelo)
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
