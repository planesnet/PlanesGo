package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

// Calibración de la vigencia por proyecto, dentro de pgocode y sin que nadie marque nada: con el historial de las
// sesiones del proyecto (las que tienen recolector), sus etiquetas automáticas (calibrate.go: usada, no usada,
// podada de más) y los embeddings locales ya guardados por los recolectores (los que falten, con Ollama local;
// nunca IA de pago), se buscan los pesos que más retiran con las podadas de más por debajo del 2 %. En segundo
// plano, en un hilo propio (uno para todos los proyectos, de uno en uno), cada calibEvery turnos de las sesiones del
// proyecto. Se guarda en los datos de pgocode (poda/calibracion.json: carpeta del proyecto → pesos), nunca en el
// repositorio del usuario. Sin historial suficiente, los valores generales (defaultVig).

const (
	// calibEvery: cada cuántos turnos del proyecto se vuelve a calibrar (el primero también).
	calibEvery = 5
	// Historial mínimo para calibrar: piezas etiquetadas y retiradas con los valores generales (con menos, cada
	// podada de más pesa demasiado y los pesos saldrían de unas pocas piezas).
	calibMinPieces  = 150
	calibMinCleared = 30
	// calibMaxSessions: las sesiones más recientes del proyecto que se usan.
	calibMaxSessions = 12
)

// calibEntry: la calibración de un proyecto (y los turnos que lleva).
type calibEntry struct {
	CalibrationInfo
	Turns int `json:"turnos"`
}

var (
	muVigParams sync.Mutex
	// calibStore: la calibración de cada carpeta de proyecto (cargada de disco la primera vez).
	calibStore    map[string]*calibEntry
	calibStoreDir string // la carpeta de la que se cargó (en las pruebas cambia)
	calibQueue    = make(chan string, 64)
	calibQueued   = map[string]bool{}
	calibOnce     sync.Once
	// calibEnabled: la calibración en segundo plano (en las pruebas, apagada salvo las suyas).
	calibEnabled = true
)

func calibFile() string { return filepath.Join(pruneDirFunc(), "calibracion.json") }

// calibLoaded: el almacén, cargado (con muVigParams cogido).
func calibLoaded() map[string]*calibEntry {
	if calibStore == nil || calibStoreDir != pruneDirFunc() {
		calibStore, calibStoreDir = map[string]*calibEntry{}, pruneDirFunc()
		if b, err := os.ReadFile(calibFile()); err == nil {
			json.Unmarshal(b, &calibStore)
		}
	}
	return calibStore
}

// calibSave guarda el almacén (con muVigParams cogido).
func calibSave() {
	if err := os.MkdirAll(pruneDirFunc(), 0o700); err != nil {
		return
	}
	b, _ := json.Marshal(calibStore)
	tmp := calibFile() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, calibFile())
	}
}

// vigParamsFor: los pesos de la vigencia del proyecto (los generales si no está calibrado o no hay carpeta).
func vigParamsFor(folder string) vigParams {
	if folder == "" {
		return defaultVig
	}
	muVigParams.Lock()
	defer muVigParams.Unlock()
	if e := calibLoaded()[folder]; e != nil && !e.General && e.At > 0 {
		return e.Params
	}
	return defaultVig
}

// calibrationInfo: lo que ve la interfaz de la calibración del proyecto (nil sin carpeta).
func calibrationInfo(folder string) *CalibrationInfo {
	if folder == "" {
		return nil
	}
	muVigParams.Lock()
	defer muVigParams.Unlock()
	if e := calibLoaded()[folder]; e != nil && e.At > 0 {
		ci := e.CalibrationInfo
		if ci.General {
			ci.Params = defaultVig
		}
		return &ci
	}
	return &CalibrationInfo{Params: defaultVig, General: true}
}

// params: los pesos de la sesión del recolector (los de su proyecto).
func (c *pruneCollector) params() vigParams { return vigParamsFor(c.state.Folder) }

// noteTurn: un turno acabado en una sesión del proyecto: el primero y cada calibEvery, se recalibra en segundo
// plano.
func noteTurn(folder string) {
	if folder == "" || !calibEnabled {
		return
	}
	muVigParams.Lock()
	st := calibLoaded()
	e := st[folder]
	if e == nil {
		e = &calibEntry{}
		st[folder] = e
	}
	e.Turns++
	due := e.Turns%calibEvery == 1
	calibSave()
	if due && !calibQueued[folder] {
		select {
		case calibQueue <- folder:
			calibQueued[folder] = true
		default:
		}
	}
	muVigParams.Unlock()
	if due {
		calibOnce.Do(func() { go calibWorker() })
	}
}

// calibWorker: el hilo de la calibración (de uno en uno, sin prisa).
func calibWorker() {
	for folder := range calibQueue {
		ctx, cancela := context.WithTimeout(context.Background(), 20*time.Minute)
		if err := calibrateProject(ctx, folder); err != nil {
			depuracion.Log("poda", "calibrar %s: %v", folder, err)
		}
		cancela()
		muVigParams.Lock()
		delete(calibQueued, folder)
		muVigParams.Unlock()
	}
}

// calibSession: una sesión del proyecto con su recolector (su clave y los vectores que guardó).
type calibSession struct {
	Key     string
	Vectors map[string][]float32
	mod     time.Time
}

// projectSessions: las sesiones con recolector del proyecto (las más recientes primero).
func projectSessions(folder string) []calibSession {
	l, _ := filepath.Glob(filepath.Join(pruneDirFunc(), "*.json"))
	var out []calibSession
	for _, f := range l {
		key := strings.TrimSuffix(filepath.Base(f), ".json")
		if !sesionValida.MatchString(key) || strings.HasPrefix(key, "chat-") {
			continue // las conversaciones de pgocode (Antigravity) no guardan su historial
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var st pruneState
		if json.Unmarshal(b, &st) != nil || st.Folder != folder {
			continue
		}
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		out = append(out, calibSession{Key: key, Vectors: st.Vectors, mod: fi.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].mod.After(out[j].mod) })
	return out[:min(len(out), calibMaxSessions)]
}

// calibLoad: todo el historial de una sesión del agente, con sus compactaciones (en las pruebas, se sustituye).
var calibLoad = func(ctx context.Context, folder, key string) ([]*ocMessage, error) {
	if idSesionOC.MatchString(key) {
		var a *Agente
		for _, x := range Agentes() {
			if ag, ok := x.(*Agente); ok && ag.Clave == "opencode" {
				a = ag
			}
		}
		if a == nil {
			return nil, fmt.Errorf("sin OpenCode")
		}
		return loadOpenCodeSession(ctx, a, Peticion{Carpeta: folder}, key)
	}
	return loadClaudeHistory(key)
}

// loadClaudeHistory: el historial de Claude Code con lo anterior a su última compactación delante (como en
// OpenCode: el mensaje de la compactación y su resumen), con identificadores de mensaje en orden.
func loadClaudeHistory(key string) ([]*ocMessage, error) {
	s, err := loadClaude(key)
	if err != nil {
		return nil, err
	}
	msgs := append([]*ocMessage(nil), s.Before...)
	if len(s.Before) > 0 {
		comp := &ocMessage{Role: "user"}
		comp.Parts = []*ocPart{newPart(key+":compactacion", "", `{"type":"compaction"}`, "")}
		msgs = append(msgs, comp, &ocMessage{Role: "assistant", Summary: true})
	}
	msgs = append(msgs, s.Win...)
	for i, m := range msgs {
		m.ID = fmt.Sprintf("m%07d", i)
		if m.Summary {
			m.Parent = msgs[i-1].ID
		}
	}
	return msgs, nil
}

// calibVectorsFile: los vectores que calcula la calibración (los que no guardó ningún recolector: «lo que se está
// haciendo ahora» en cada punto simulado), para no volver a calcularlos.
func calibVectorsFile() string { return filepath.Join(pruneDirFunc(), "calibracion-vectores.json") }

// calibrateProject: calibra los pesos del proyecto con el historial de sus sesiones y lo guarda.
func calibrateProject(ctx context.Context, folder string) error {
	ses := projectSessions(folder)
	extra := map[string][]float32{}
	if b, err := os.ReadFile(calibVectorsFile()); err == nil {
		json.Unmarshal(b, &extra)
	}
	nuevos := 0
	var hs []*calibHistory
	for _, s := range ses {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msgs, err := calibLoad(ctx, folder, s.Key)
		if err != nil || len(msgs) == 0 {
			continue
		}
		vec := func(k, texto, tarea string) []float32 {
			if v, ok := s.Vectors[k]; ok {
				return v
			}
			if v, ok := extra[k]; ok {
				return v
			}
			v, err := pruneEmbed(ctx, texto, tarea) // Ollama local
			if err != nil {
				return nil
			}
			extra[k] = v
			nuevos++
			return v
		}
		hs = append(hs, buildHistory(msgs, vec))
	}
	if nuevos > 0 {
		if b, err := json.Marshal(extra); err == nil {
			os.WriteFile(calibVectorsFile(), b, 0o600)
		}
	}
	base := simulateAll(hs, defaultVig)
	ci := CalibrationInfo{Params: defaultVig, General: true, At: pruneNow().UnixMilli(), Pieces: base.Labeled, Sessions: len(hs),
		Cleared: base.Cleared, Tokens: base.Tokens, TokenSteps: base.TokenSteps, Over: base.Over,
		BaseCleared: base.Cleared, BaseTokens: base.Tokens, BaseTokenStep: base.TokenSteps, BaseOver: base.Over}
	if base.Labeled >= calibMinPieces && base.Cleared >= calibMinCleared {
		k, t := searchParams(hs, defaultVig)
		ci.Params, ci.General = k, false
		ci.Cleared, ci.Tokens, ci.TokenSteps, ci.Over = t.Cleared, t.Tokens, t.TokenSteps, t.Over
	}
	muVigParams.Lock()
	st := calibLoaded()
	e := st[folder]
	if e == nil {
		e = &calibEntry{}
		st[folder] = e
	}
	e.CalibrationInfo = ci
	calibSave()
	muVigParams.Unlock()
	depuracion.Log("poda", "calibrado %s: %d sesiones, %d piezas, pesos %+v (generales: %v), %d retiradas y %d de más (con los generales, %d y %d)",
		folder, len(hs), ci.Pieces, ci.Params, ci.General, ci.Cleared, ci.Over, ci.BaseCleared, ci.BaseOver)
	return nil
}

// CalibrationInfo: lo que ve la interfaz de la calibración del proyecto (la ventanita del hilo de limpieza).
type CalibrationInfo struct {
	Params   vigParams `json:"pesos"`
	General  bool      `json:"generales,omitempty"` // sin calibrar (poco historial): los valores generales
	At       int64     `json:"fecha,omitempty"`     // cuándo se calibró (ms)
	Pieces   int       `json:"piezas,omitempty"`    // piezas etiquetadas
	Sessions int       `json:"sesiones,omitempty"`
	// Con los pesos del proyecto (y, para comparar, con los generales): piezas retiradas, tokens retirados,
	// tokens·paso fuera y podadas de más (relecturas por poda de más).
	Cleared       int   `json:"retiradas,omitempty"`
	Tokens        int   `json:"tokens,omitempty"`
	TokenSteps    int64 `json:"tokens_paso,omitempty"`
	Over          int   `json:"de_mas,omitempty"`
	BaseCleared   int   `json:"retiradas_generales,omitempty"`
	BaseTokens    int   `json:"tokens_generales,omitempty"`
	BaseTokenStep int64 `json:"tokens_paso_generales,omitempty"`
	BaseOver      int   `json:"de_mas_generales,omitempty"`
}
