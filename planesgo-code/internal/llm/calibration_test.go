package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// longSession: una sesión larga de prueba: n pasos, cada uno lee un fichero distinto (salida grande); cada cuatro
// pasos, el agente edita el fichero que leyó dos pasos antes (lo usa) y, de vez en cuando, uno muy viejo (que habría
// que releer si se retiró).
func longSession(pref string, n int) []*ocMessage {
	s := &sesionPrueba{}
	s.user("Instrucciones")
	s.assistant(text("Vale."))
	s.user("Revisa todos los módulos del proyecto y corrige los fallos de cada uno")
	for i := 0; i < n; i++ {
		f := fmt.Sprintf("/p/%s_mod%03d.go", pref, i)
		var b strings.Builder
		for j := 0; j < 150; j++ {
			fmt.Fprintf(&b, "%d: func %s_Funcion%03d_%d() { return valorDelModulo_%03d }\n", j+1, pref, i, j, i)
		}
		rd := tool("read", map[string]any{"filePath": f, "offset": 1, "limit": 150}, b.String())
		rd["callID"] = fmt.Sprintf("call_%s_%d", pref, i)
		partes := []map[string]any{rd}
		if i%4 == 3 {
			g := fmt.Sprintf("/p/%s_mod%03d.go", pref, i-2)
			partes = append(partes, tool("edit", map[string]any{"filePath": g, "oldString": "a", "newString": "b"}, "ok"))
		}
		if i%25 == 24 {
			g := fmt.Sprintf("/p/%s_mod%03d.go", pref, i-20)
			partes = append(partes, tool("edit", map[string]any{"filePath": g, "oldString": "a", "newString": "b"}, "ok"))
		}
		s.assistant(partes...)
	}
	s.assistant(text("Hecho."))
	for i, m := range s.msgs {
		m.Time = vigT0.Add(time.Duration(i) * time.Minute).UnixMilli()
		m.ID = fmt.Sprintf("msg_%s_%05d", pref, i)
	}
	return s.msgs
}

func TestCalibrateProject(t *testing.T) {
	drainCollectors()
	dir := t.TempDir()
	viejoDir, viejoLoad, viejoEmb, viejoNow := pruneDirFunc, calibLoad, pruneEmbed, pruneNow
	defer func() { pruneDirFunc, calibLoad, pruneEmbed, pruneNow = viejoDir, viejoLoad, viejoEmb, viejoNow }()
	pruneDirFunc = func() string { return dir }
	pruneNow = func() time.Time { return vigT0 }
	pruneEmbed = func(context.Context, string, string) ([]float32, error) { return nil, os.ErrNotExist }
	carpeta := "/home/x/proyecto"
	hist := map[string][]*ocMessage{}
	for i, k := range []string{"ses_cala", "ses_calb", "ses_calc"} {
		hist[k] = longSession(fmt.Sprint(i), 70)
		b, _ := json.Marshal(pruneState{Folder: carpeta})
		os.WriteFile(filepath.Join(dir, k+".json"), b, 0o600)
	}
	b, _ := json.Marshal(pruneState{Folder: "/otra"})
	os.WriteFile(filepath.Join(dir, "ses_otra.json"), b, 0o600)
	calibLoad = func(_ context.Context, folder, key string) ([]*ocMessage, error) {
		if folder != carpeta || hist[key] == nil {
			t.Errorf("sesión de otro proyecto: %s %s", folder, key)
		}
		return hist[key], nil
	}
	if ci := calibrationInfo(carpeta); !ci.General || ci.Params != defaultVig {
		t.Fatalf("sin calibrar: %+v", ci)
	}
	if err := calibrateProject(context.Background(), carpeta); err != nil {
		t.Fatal(err)
	}
	ci := calibrationInfo(carpeta)
	if ci.General || ci.Sessions != 3 || ci.Pieces < calibMinPieces || ci.At != vigT0.UnixMilli() {
		t.Fatalf("calibración: %+v", ci)
	}
	if ci.Over > int(calibMaxOver*float64(ci.Cleared)) || ci.TokenSteps < ci.BaseTokenStep && ci.BaseOver <= int(calibMaxOver*float64(ci.BaseCleared)) {
		t.Fatalf("la calibración cumple y no retira menos que los generales si estos cumplen: %+v", ci)
	}
	if vigParamsFor(carpeta) != ci.Params || vigParamsFor("/otra") != defaultVig {
		t.Fatal("pesos por proyecto")
	}
	// Se guarda en los datos de pgocode y se vuelve a cargar.
	muVigParams.Lock()
	calibStore = nil
	muVigParams.Unlock()
	if vigParamsFor(carpeta) != ci.Params {
		t.Fatal("no se guardó")
	}
	if _, err := os.Stat(filepath.Join(dir, "calibracion.json")); err != nil {
		t.Fatal(err)
	}
	// El recolector de una sesión del proyecto usa sus pesos y los manda a la interfaz.
	c := &pruneCollector{key: "ses_cala"}
	c.load()
	if c.params() != ci.Params || c.reportNow(vigT0).Calibration == nil {
		t.Fatal("recolector")
	}
}

func TestCalibrationTooLittleHistory(t *testing.T) {
	dir := t.TempDir()
	viejoDir, viejoLoad := pruneDirFunc, calibLoad
	defer func() { pruneDirFunc, calibLoad = viejoDir, viejoLoad }()
	pruneDirFunc = func() string { return dir }
	viejoEmb := pruneEmbed
	defer func() { pruneEmbed = viejoEmb }()
	pruneEmbed = func(context.Context, string, string) ([]float32, error) { return nil, os.ErrNotExist }
	b, _ := json.Marshal(pruneState{Folder: "/p"})
	os.WriteFile(filepath.Join(dir, "ses_poco.json"), b, 0o600)
	calibLoad = func(context.Context, string, string) ([]*ocMessage, error) { return conversacion().msgs, nil }
	if err := calibrateProject(context.Background(), "/p"); err != nil {
		t.Fatal(err)
	}
	if ci := calibrationInfo("/p"); !ci.General || ci.At == 0 || ci.Params != defaultVig || vigParamsFor("/p") != defaultVig {
		t.Fatalf("con poco historial, los generales: %+v", ci)
	}
}

func TestNoteTurnSchedule(t *testing.T) {
	dir := t.TempDir()
	viejoDir := pruneDirFunc
	defer func() { pruneDirFunc = viejoDir }()
	pruneDirFunc = func() string { return dir }
	calibEnabled = true
	defer func() { calibEnabled = false }()
	// Sin hilo: solo la cola (se vacía a mano).
	calibOnce.Do(func() {})
	var colas []int
	for i := 1; i <= 11; i++ {
		noteTurn("/q")
		select {
		case f := <-calibQueue:
			colas = append(colas, i)
			muVigParams.Lock()
			delete(calibQueued, f)
			muVigParams.Unlock()
		default:
		}
	}
	if fmt.Sprint(colas) != "[1 6 11]" {
		t.Fatalf("turnos en que se calibra: %v", colas)
	}
}

// Lo que decide la simulación es lo mismo que decide planPruneK (con los mismos pesos): en cada punto de barrido,
// cada candidato sale con unos pesos si y solo si sale en el plan de esa ventana.
func TestCalibCandMatchesPlan(t *testing.T) {
	msgs := longSession("x", 40)
	h := buildHistory(msgs, nil)
	for _, k := range []vigParams{defaultVig, {AgeK: 0.3, StepK: 2, TimeK: 0, RelK: 10, RelPivot: 0.5}, {StepK: 0.4}} {
		comprobados := 0
		for _, pt := range h.Points {
			// La ventana del punto: hasta su paso (entre pasos, incluido; al empezar el turno, antes del mensaje).
			n, fin := 0, 0
			for i, m := range msgs {
				if m.Role == "assistant" {
					n++
				}
				if n == pt.Step {
					fin = i + 1
					if !pt.Mid {
						for fin < len(msgs) && msgs[fin].Role == "assistant" {
							fin++
						}
					}
					break
				}
			}
			young := pruneYoung
			if pt.Mid {
				young++
			}
			prefix := msgs[:fin]
			plan := planPruneK(pruneWindow(prefix), nil, young, time.UnixMilli(prefix[len(prefix)-1].Time), k)
			out := map[string]bool{}
			for _, pv := range plan.Pieces {
				out[pv.PartID] = pv.Out
			}
			for _, c := range pt.Cands {
				if c.out(k) != out[c.P.ID] {
					t.Fatalf("pesos %+v, paso %d: %s sale %v en la simulación y %v en el plan", k, pt.Step, c.P.ID, c.out(k), out[c.P.ID])
				}
				comprobados++
			}
		}
		if comprobados == 0 {
			t.Fatal("sin candidatos")
		}
	}
}
