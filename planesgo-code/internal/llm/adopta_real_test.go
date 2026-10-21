package llm

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"planesgo-code/internal/proceso"
)

func TestAdoptaReal(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_ADOPTA") == "" {
		t.Skip()
	}
	var claude Proveedor
	for _, p := range Agentes() {
		if a, ok := p.(*Agente); ok && a.Clave == "claude-code" {
			claude = p
		}
	}
	d := t.TempDir()
	os.WriteFile(d+"/index.html", []byte("hola"), 0o644)
	inicio := time.Now()
	var ad []Adoptado
	err := claude.Chat(context.Background(), Peticion{Carpeta: d, Permisos: "auto", Modelo: "haiku",
		Mensajes: []Mensaje{{Rol: "user", Texto: "Arranca con la herramienta Bash y run_in_background=true la orden `python3 -m http.server 8977`, y responde solo OK sin esperar a que termine."}}},
		func(e Evento) { ad = append(ad, e.Adoptados...) })
	t.Logf("turno en %s, error %v, adoptados %+v", time.Since(inicio).Round(time.Second), err, ad)
	if err != nil || len(ad) != 1 {
		t.Fatalf("adoptados: %+v", ad)
	}
	defer proceso.SenalGrupo(ad[0].Grupo, true)
	time.Sleep(time.Second)
	r, err := http.Get("http://127.0.0.1:8977/")
	if err != nil {
		t.Fatalf("el servidor no sigue vivo: %v", err)
	}
	r.Body.Close()
	t.Logf("sigue vivo (grupo %d), salida en %s", ad[0].Grupo, ad[0].Fichero)
}
