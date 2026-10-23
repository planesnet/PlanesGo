package servidor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/memory"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

// fakeAgent: un agente que guarda el mensaje que recibe y responde con una orden de pruebas que funciona (y, si se
// le pide, con una compactación).
type fakeAgent struct {
	got     []string
	compact bool
}

func (f *fakeAgent) ID() string { return "falso" }
func (f *fakeAgent) Info(context.Context) llm.Info {
	return llm.Info{ID: "falso", Nombre: "Falso", Tipo: "agente", Disponible: true, Modelos: []string{"m"}, PorDefecto: "m", NecesitaDir: true}
}
func (f *fakeAgent) Chat(_ context.Context, p llm.Peticion, emite func(llm.Evento)) error {
	f.got = append(f.got, p.Mensajes[len(p.Mensajes)-1].Texto+p.Memory) // el agente la pone al final (agentes.go)
	emite(llm.Evento{Herramienta: &llm.Herramienta{ID: "t1", Nombre: "bash", Resumen: "cd x && go test ./... 2>&1 | tail -5", Estado: "curso"}})
	emite(llm.Evento{Herramienta: &llm.Herramienta{ID: "t1", Estado: "ok", Salida: "ok"}})
	if f.compact {
		emite(llm.Evento{Texto: "(compactado)", Compacted: true})
	}
	emite(llm.Evento{Texto: "hecho", SesionAgente: "ag1"})
	return nil
}

func TestProjectMemoryInChat(t *testing.T) {
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "p1")
	os.Mkdir(dir, 0o755)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Skipf("sin git: %v %s", err, out)
	}
	os.WriteFile(filepath.Join(dir, ".planesgo.json"), []byte(`{"odoo_project_id": 1}`), 0o644)
	ag := &fakeAgent{}
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Indice: proyectos.NuevoIndice([]string{raiz}),
		Proveedores: llm.NuevoRegistro(ag)}
	h := s.Handler()
	ses, _ := s.Sesiones.Crea(1, "t", "t", "x")
	ses.MarcaProyecto(1, time.Now().Add(time.Hour))
	pide := func(ruta string, cuerpo map[string]any) string {
		b, _ := json.Marshal(cuerpo)
		r := httptest.NewRequest("POST", ruta, strings.NewReader(string(b)))
		r.Host = "127.0.0.1:7801"
		r.Header.Set("X-PGO", "1")
		r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", ruta, w.Code, w.Body)
		}
		return w.Body.String()
	}
	chat := func(sesionAgente, texto string) string {
		return pide("/api/chat", map[string]any{"proyecto": 1, "proveedor": "falso", "carpeta": dir, "sesion_agente": sesionAgente,
			"mensajes": []map[string]string{{"rol": "user", "texto": texto + "\n\n[pgocode: nota añadida]"}}, "texto_usuario": texto})
	}
	waitMemory := func(want string) {
		for range 100 {
			if strings.Contains(memory.Load(dir), want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("la memoria no tiene %q:\n%s", want, memory.Load(dir))
	}

	// 1. Sin memoria: nada que añadir. Lo que dice el usuario y la orden de pruebas que ha funcionado, a la memoria.
	out := chat("", "Arregla la factura. El puerto es 3005.")
	if strings.Contains(ag.got[0], "Memoria del proyecto") || strings.Contains(out, `"memoria"`) {
		t.Fatalf("sin memoria no se añade nada: %q", ag.got[0])
	}
	waitMemory("El puerto es 3005")
	waitMemory("pruebas: `go test ./...`")
	if strings.Contains(memory.Load(dir), "nota añadida") || strings.Contains(memory.Load(dir), "Arregla") {
		t.Fatalf("solo lo durable de lo que escribió el usuario:\n%s", memory.Load(dir))
	}

	// 2. Sesión nueva del agente: con la memoria, al final del mensaje.
	out = chat("", "Sigue con lo de ayer")
	if g := ag.got[1]; !strings.HasPrefix(g, "Sigue con lo de ayer") || !strings.Contains(g, "- El puerto es 3005") || !strings.HasSuffix(g, "[Fin de la memoria del proyecto]") {
		t.Fatalf("sesión nueva sin memoria: %q", g)
	}
	if !strings.Contains(out, `"memoria":{"motivo":"sesión nueva del agente"`) {
		t.Fatalf("la interfaz no lo sabe: %s", out)
	}

	// 3. La misma sesión: sin memoria (no en cada turno).
	chat("ag1", "Y ahora el total")
	if strings.Contains(ag.got[2], "Memoria del proyecto") {
		t.Fatalf("en cada turno no: %q", ag.got[2])
	}

	// 4. Tras «Vaciar contexto»: otra vez, una sola vez.
	viejo := vaciado
	defer func() { vaciado = viejo }()
	vaciado = func(context.Context, string, string, string, string) (*llm.PruneReport, error) {
		return &llm.PruneReport{}, nil
	}
	pide("/api/chat/vigencia?barrer=1&vaciar=1", map[string]any{"proyecto": 1, "agente": "opencode", "sesion_agente": "ag1", "carpeta": dir})
	chat("ag1", "Sigue")
	chat("ag1", "Sigue más")
	if !strings.Contains(ag.got[3], "Memoria del proyecto") || strings.Contains(ag.got[4], "Memoria del proyecto") {
		t.Fatalf("tras vaciar, en el siguiente mensaje y solo en ese: %q / %q", ag.got[3], ag.got[4])
	}

	// 5. Tras una compactación: en el turno siguiente.
	ag.compact = true
	chat("ag1", "Compacta")
	ag.compact = false
	chat("ag1", "Después de compactar")
	if strings.Contains(ag.got[5], "Memoria del proyecto") || !strings.Contains(ag.got[6], "Memoria del proyecto") {
		t.Fatalf("tras compactar, en el siguiente: %q / %q", ag.got[5], ag.got[6])
	}

	// 6. Ver, corregir y borrar desde la interfaz.
	r := httptest.NewRequest("GET", "/api/memoria?proyecto=1&carpeta="+dir, nil)
	r.Host = "127.0.0.1:7801"
	r.Header.Set("X-PGO", "1")
	r.AddCookie(&http.Cookie{Name: cookie, Value: ses.Token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var v memoryView
	if json.Unmarshal(w.Body.Bytes(), &v); !strings.Contains(v.Texto, "El puerto es 3005") || v.Tokens == 0 || v.Max != memory.MaxTokens {
		t.Fatalf("ver: %s", w.Body)
	}
	pide("/api/memoria", map[string]any{"proyecto": 1, "carpeta": dir, "texto": "## Mía\n- El puerto es 4000\n"})
	chat("", "Hola")
	if !strings.Contains(ag.got[7], "- El puerto es 4000") || strings.Contains(ag.got[7], "3005") {
		t.Fatalf("la corrección del usuario: %q", ag.got[7])
	}
	pide("/api/memoria", map[string]any{"proyecto": 1, "carpeta": dir, "texto": ""})
	if memory.Load(dir) != "" {
		t.Fatal("no se ha borrado")
	}
}

func TestWithoutRelayMemory(t *testing.T) {
	in := "[Memoria de trabajo: pgocode…]\n\n## Decisiones\n- Usuario: nunca X\n\n[Fin de la memoria de trabajo]\n\nEl puerto es 3005"
	if got := strings.TrimSpace(withoutRelayMemory(in)); got != "El puerto es 3005" {
		t.Fatalf("%q", got)
	}
}
